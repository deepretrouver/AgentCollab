package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/idgen"
)

const maxAgentTasksPerRoot = 8
const agentCallbackTitlePrefix = "Agent 回传："

var groupMentionPattern = regexp.MustCompile(`(?i)^\s*(?:(?:请|让|麻烦|请让)\s*)?(?:@(?:全体|everyone|all)|全体(?:\s*agents?)?|所有(?:的)?(?:\s*agents?|人|成员)|全部(?:\s*agents?|成员)?|大家|各位|每个(?:人|agent))`)
var coordinatorFanoutPattern = regexp.MustCompile(`(?i)(?:召集|组织|邀请|让|叫).{0,12}(?:大家|全体|各位|所有(?:的)?(?:\s*agents?|人|成员)|全部(?:\s*agents?|成员))`)

type routingAgent struct {
	ID              domain.AgentID
	DisplayName     string
	Type            string
	ResponseMode    domain.AgentResponseMode
	RolePrompt      string
	MaxAutoTurns    int
	ReplyDelaySecs  int
	OwnerUserID     domain.UserID
	InvocationScope domain.AgentInvocationScope
	RunnerVersion   string
}

type routedTaskInput struct {
	WorkspaceID     domain.WorkspaceID
	RoomID          domain.RoomID
	RootMessageID   domain.MessageID
	SourceMessageID domain.MessageID
	ParentTaskID    *domain.TaskID
	CreatorKind     domain.TaskCreatorKind
	CreatorID       string
	Target          routingAgent
	SourceBody      string
	Depth           int
	CreatedAt       time.Time
	IsCallback      bool
	ExecutionSource domain.TaskExecutionSource
}

func roomRoutingAgentsTx(ctx context.Context, tx *sql.Tx, roomID domain.RoomID) ([]routingAgent, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT a.id, COALESCE(NULLIF(a.source_display_name, ''), a.display_name), a.type, COALESCE(s.response_mode, 'mention'), COALESCE(s.role_prompt, ''),
	       COALESCE(s.max_auto_turns, 4), COALESCE(s.reply_delay_seconds, 0), a.owner_user_id, p.invocation_scope, r.version
FROM room_agents ra
JOIN agents a ON a.id = ra.agent_id
JOIN runners r ON r.id = a.runner_id
JOIN agent_access_policies p ON p.agent_id = a.id
LEFT JOIN room_agent_settings s ON s.room_id = ra.room_id AND s.agent_id = ra.agent_id
WHERE ra.room_id = ? AND a.status != 'unavailable' AND p.archived = 0
ORDER BY ra.added_at, a.id`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	agents := make([]routingAgent, 0)
	for rows.Next() {
		var agent routingAgent
		if err := rows.Scan(&agent.ID, &agent.DisplayName, &agent.Type, &agent.ResponseMode, &agent.RolePrompt, &agent.MaxAutoTurns, &agent.ReplyDelaySecs, &agent.OwnerUserID, &agent.InvocationScope, &agent.RunnerVersion); err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	return agents, rows.Err()
}

func canUserInvokeAgent(userID domain.UserID, agent routingAgent) bool {
	return agent.OwnerUserID == userID || (agent.InvocationScope == domain.AgentInvocationRoomMembers && runnerSupportsScopedCapabilities(agent.RunnerVersion))
}

func invokableAgents(userID domain.UserID, agents []routingAgent) []routingAgent {
	result := make([]routingAgent, 0, len(agents))
	for _, agent := range agents {
		if canUserInvokeAgent(userID, agent) {
			result = append(result, agent)
		}
	}
	return result
}

func validMentionIDs(requested []domain.AgentID, agents []routingAgent, userID domain.UserID) []domain.AgentID {
	allowed := make(map[domain.AgentID]bool, len(agents))
	for _, agent := range agents {
		allowed[agent.ID] = agent.ResponseMode != domain.AgentResponseMuted && canUserInvokeAgent(userID, agent)
	}
	seen := make(map[domain.AgentID]bool)
	result := make([]domain.AgentID, 0, len(requested))
	for _, id := range requested {
		if allowed[id] && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

// expandGroupMention turns an explicit group address into structured Agent IDs.
// The original message body is still stored and rendered unchanged.
func expandGroupMention(mentions []domain.AgentID, body string, agents []routingAgent, userID domain.UserID) []domain.AgentID {
	if len(mentions) > 0 || !groupMentionPattern.MatchString(body) {
		return mentions
	}
	result := make([]domain.AgentID, 0, len(agents))
	for _, agent := range agents {
		if agent.ResponseMode != domain.AgentResponseMuted && canUserInvokeAgent(userID, agent) {
			result = append(result, agent.ID)
		}
	}
	return result
}

// requestsCoordinatorFanout recognizes a user asking one coordinator to bring
// the rest of the room into the work. It intentionally differs from an
// explicit group address: the user's message can begin with @Coordinator.
func requestsCoordinatorFanout(body string) bool {
	_, brainstorm := brainstormTopic(body)
	return brainstorm || coordinatorFanoutPattern.MatchString(body)
}

func brainstormTopic(body string) (string, bool) {
	trimmed := strings.TrimSpace(body)
	if strings.EqualFold(trimmed, "/brainstorm") {
		return "", true
	}
	if len(trimmed) <= len("/brainstorm") || !strings.EqualFold(trimmed[:len("/brainstorm")], "/brainstorm") {
		return "", false
	}
	if next := trimmed[len("/brainstorm")]; next != ' ' && next != '\t' && next != '\n' && next != '\r' {
		return "", false
	}
	return strings.TrimSpace(trimmed[len("/brainstorm"):]), true
}

func selectUserMessageTargets(mentions []domain.AgentID, agents []routingAgent, userID domain.UserID) []routingAgent {
	if len(mentions) > 0 {
		return selectExplicitMessageTargets(mentions, agents, userID)
	}
	result := make([]routingAgent, 0)
	for _, agent := range agents {
		if (agent.ResponseMode == domain.AgentResponseCoordinator || agent.ResponseMode == domain.AgentResponseHumanAll) && canUserInvokeAgent(userID, agent) {
			result = append(result, agent)
		}
	}
	return result
}

func selectExplicitMessageTargets(mentions []domain.AgentID, agents []routingAgent, userID domain.UserID) []routingAgent {
	wanted := make(map[domain.AgentID]bool, len(mentions))
	for _, id := range mentions {
		wanted[id] = true
	}
	result := make([]routingAgent, 0, len(mentions))
	for _, agent := range agents {
		if wanted[agent.ID] && agent.ResponseMode != domain.AgentResponseMuted && canUserInvokeAgent(userID, agent) {
			result = append(result, agent)
		}
	}
	return result
}

func createRoutedTaskTx(ctx context.Context, tx *sql.Tx, input routedTaskInput, agents []routingAgent) error {
	taskID, err := idgen.New("task")
	if err != nil {
		return err
	}
	attemptID, err := idgen.New("attempt")
	if err != nil {
		return err
	}
	contextText, err := recentRoomContextTx(ctx, tx, input.RoomID, input.SourceMessageID, agents)
	if err != nil {
		return err
	}
	directory := make([]string, 0, len(agents))
	for _, agent := range agents {
		if agent.ID != input.Target.ID && agent.ResponseMode != domain.AgentResponseMuted {
			directory = append(directory, "@"+agent.DisplayName+" ("+agent.Type+")")
		}
	}
	title := "群聊点名：" + input.Target.DisplayName
	if input.CreatorKind == domain.TaskCreatorAgent {
		title = "Agent 接力：" + input.Target.DisplayName
	}
	if input.IsCallback {
		title = agentCallbackTitlePrefix + input.Target.DisplayName
	}
	workItemID, err := ensureWorkItemForRootTx(ctx, tx, input.WorkspaceID, input.RoomID, input.RootMessageID, title, input.CreatedAt)
	if err != nil {
		return err
	}
	if err := validateTaskExecutionSourceTx(ctx, tx, input.ExecutionSource, workItemID, input.Target.ID); err != nil {
		return err
	}
	prompt := fmt.Sprintf("你是 AgentCollab 群聊中的 %s（%s）。\n", input.Target.DisplayName, input.Target.Type)
	if input.Target.ResponseMode == domain.AgentResponseCoordinator {
		prompt += "你是本群的主协调 Agent：优先做需求澄清、任务拆解和专家分诊；需要专家执行时，用单独一行 @专家完整显示名 交接。当用户说“召集大家/全体/所有人”时，必须给每一位可协作 Agent 各写一行明确分工，不得只点名其中一位。平台会等所有直接专家任务结束后一次性回送全部结果，再由你汇总交付；在全部结果返回前不要冒充最终总结。\n"
	}
	if topic, brainstorm := brainstormTopic(input.SourceBody); brainstorm && topic != "" {
		prompt += "这是一次群聊级专题讨论，议题是：“" + topic + "”。你现在负责开题：界定问题、提出分析框架和参与者分工，不要在专家尚未回传前给出最终结论。平台会自动召集群内其他非静默 Agent，等待他们全部交付后再次唤醒你完成最终总结。\n"
	}
	if input.IsCallback {
		prompt += "这是平台在所有直接专家任务结束后触发的汇总回调。下方包含全部专家交付与失败信息；现在给出最终综合结论，明确每位专家的贡献、共识、分歧和下一步。\n"
	} else if input.CreatorKind == domain.TaskCreatorAgent {
		prompt += "这是一位协作 Agent 交给你的子任务。完成后直接返回结论，平台会自动把结果回送给委派者。\n"
	}
	if strings.TrimSpace(input.Target.RolePrompt) != "" {
		prompt += "你的群内角色：" + strings.TrimSpace(input.Target.RolePrompt) + "\n"
	}
	prompt += "请理解上下文后直接完成当前消息要求，并返回可在群聊中展示的结果。最终输出必须包含已完成的发现或结论及其依据；禁止用“我会/我将/接下来/下一步再”这类过程计划代替交付。\n\n最近群聊上下文：\n" + contextText + "\n\n当前消息：\n" + input.SourceBody
	if len(directory) > 0 {
		prompt += "\n\n可协作 Agent：" + strings.Join(directory, "、") + "。只有确实需要对方继续工作时，才在单独一行写 @完整显示名 并说明交接任务；普通提及不要放在行首。"
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO tasks (id, work_item_id, workspace_id, room_id, parent_task_id, creator_kind, creator_id, target_agent_id, title, prompt,
                   acceptance_criteria, execution_source, state, current_attempt, root_message_id, source_message_id, collaboration_depth, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'queued', 1, ?, ?, ?, ?, ?)`, taskID, workItemID, input.WorkspaceID, input.RoomID, input.ParentTaskID,
		input.CreatorKind, input.CreatorID, input.Target.ID, title, prompt,
		"回答必须结合当前会话上下文，并交付明确结论、核心依据和可执行建议；不得只返回调查计划或过程状态。", input.ExecutionSource, input.RootMessageID, input.SourceMessageID, input.Depth,
		formatTime(input.CreatedAt), formatTime(input.CreatedAt))
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO task_attempts (id, task_id, attempt_no, state, created_at) VALUES (?, ?, 1, 'queued', ?)`, attemptID, taskID, formatTime(input.CreatedAt)); err != nil {
		return err
	}
	return syncWorkItemStateTx(ctx, tx, workItemID, input.CreatedAt)
}

func validateTaskExecutionSourceTx(ctx context.Context, tx *sql.Tx, source domain.TaskExecutionSource, workItemID domain.WorkItemID, targetAgentID domain.AgentID) error {
	if !source.Valid() {
		return fmt.Errorf("%w: invalid execution source", auth.ErrInvalidInput)
	}
	if source == domain.TaskExecutionOfficial {
		var count int
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM work_items wi
JOIN project_step_workspaces psw ON psw.step_id = wi.plan_step_id
WHERE wi.id = ? AND psw.last_commit != ''`, workItemID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("%w: 当前步骤尚未上传团队公共底稿", auth.ErrInvalidInput)
		}
		return nil
	}
	if source != domain.TaskExecutionResponsibleLocal {
		return nil
	}
	var count int
	err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM work_items wi
JOIN project_step_local_bindings b ON b.step_id = wi.plan_step_id
JOIN agents a ON a.id = ? AND a.runner_id = b.runner_id AND a.owner_user_id = b.user_id
JOIN project_step_agent_responsibles ar ON ar.step_id = wi.plan_step_id AND ar.agent_id = a.id
WHERE wi.id = ?`, targetAgentID, workItemID).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("%w: 目标 Agent 尚未绑定这个步骤的负责人本机目录", auth.ErrInvalidInput)
	}
	return nil
}

func recentRoomContextTx(ctx context.Context, tx *sql.Tx, roomID domain.RoomID, through domain.MessageID, agents []routingAgent) (string, error) {
	var throughSequence uint64
	if err := tx.QueryRowContext(ctx, `SELECT sequence FROM messages WHERE message_id = ? AND room_id = ?`, through, roomID).Scan(&throughSequence); err != nil {
		return "", err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT sender_kind, sender_id, body
FROM messages
WHERE room_id = ? AND sequence <= ? AND kind != 'task'
ORDER BY sequence DESC LIMIT 25`, roomID, throughSequence)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	names := make(map[string]string, len(agents))
	for _, agent := range agents {
		names[string(agent.ID)] = agent.DisplayName
	}
	lines := make([]string, 0, 25)
	for rows.Next() {
		var senderKind domain.SenderKind
		var senderID, body string
		if err := rows.Scan(&senderKind, &senderID, &body); err != nil {
			return "", err
		}
		if isConversationBoundary(body) {
			break
		}
		if len(body) > 1200 {
			body = body[:1200] + "…"
		}
		name := "群成员"
		if senderKind == domain.MessageSenderAgent {
			name = names[senderID]
			if name == "" {
				name = "Agent"
			}
		}
		lines = append(lines, name+"："+body)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	for left, right := 0, len(lines)-1; left < right; left, right = left+1, right-1 {
		lines[left], lines[right] = lines[right], lines[left]
	}
	if len(lines) == 0 {
		return "这是本会话的第一条消息。", nil
	}
	return strings.Join(lines, "\n\n"), nil
}

func mentionedAgentIDs(body string, agents []routingAgent) []domain.AgentID {
	result := make([]domain.AgentID, 0)
	for _, agent := range agents {
		if agent.ResponseMode == domain.AgentResponseMuted {
			continue
		}
		pattern := `(?mi)^\s*@` + regexp.QuoteMeta(agent.DisplayName) + `(?:\s|[：:，,。.!！?？]|$)`
		if regexp.MustCompile(pattern).MatchString(body) {
			result = append(result, agent.ID)
		}
	}
	return result
}

func isConversationBoundary(body string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(body))
	if strings.HasPrefix(trimmed, "/new ") {
		return true
	}
	switch trimmed {
	case "/new", "/clear", "/restart", "/reset":
		return true
	default:
		return false
	}
}
