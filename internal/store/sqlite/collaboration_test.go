package sqlite

import (
	"reflect"
	"testing"

	"agentcollab.local/agentcollab/internal/domain"
)

func TestExpandGroupMention(t *testing.T) {
	agents := []routingAgent{
		{ID: "codex", OwnerUserID: "owner", ResponseMode: domain.AgentResponseMention},
		{ID: "hermes", OwnerUserID: "owner", ResponseMode: domain.AgentResponseCoordinator},
		{ID: "workbuddy", OwnerUserID: "owner", ResponseMode: domain.AgentResponseMention},
		{ID: "muted", OwnerUserID: "owner", ResponseMode: domain.AgentResponseMuted},
	}
	want := []domain.AgentID{"codex", "hermes", "workbuddy"}
	for _, body := range []string{
		"每个人都介绍一下自己 一句话",
		"请大家分别汇报进展",
		"@全体 介绍一下自己",
		"@all agents report status",
	} {
		if got := expandGroupMention(nil, body, agents, "owner"); !reflect.DeepEqual(got, want) {
			t.Fatalf("expandGroupMention(%q) = %v, want %v", body, got, want)
		}
	}
}

func TestExpandGroupMentionPreservesExplicitTargets(t *testing.T) {
	agents := []routingAgent{
		{ID: "codex", OwnerUserID: "owner", ResponseMode: domain.AgentResponseMention},
		{ID: "hermes", OwnerUserID: "owner", ResponseMode: domain.AgentResponseMention},
	}
	want := []domain.AgentID{"codex"}
	if got := expandGroupMention(want, "大家一起回答", agents, "owner"); !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit targets changed: got %v, want %v", got, want)
	}
	if got := expandGroupMention(nil, "总结大家刚才的回答", agents, "owner"); len(got) != 0 {
		t.Fatalf("non-address group phrase routed unexpectedly: %v", got)
	}
}

func TestRequestsCoordinatorFanout(t *testing.T) {
	for _, body := range []string{
		"@Hermes Chengong 请你召集大家一起评估这个技能",
		"@Lead 组织全体 agents 分工调研",
		"请协调人让所有成员参与复核",
		"/brainstorm 评估股票技能的实盘效果",
	} {
		if !requestsCoordinatorFanout(body) {
			t.Fatalf("requestsCoordinatorFanout(%q) = false, want true", body)
		}
	}
	for _, body := range []string{
		"@Hermes Chengong 请你单独评估",
		"请总结大家刚才的回答",
	} {
		if requestsCoordinatorFanout(body) {
			t.Fatalf("requestsCoordinatorFanout(%q) = true, want false", body)
		}
	}
}

func TestBrainstormTopic(t *testing.T) {
	for input, want := range map[string]string{
		"/brainstorm 评估股票技能":  "评估股票技能",
		"/BRAINSTORM   新产品方案": "新产品方案",
		"/brainstorm":         "",
	} {
		got, ok := brainstormTopic(input)
		if !ok || got != want {
			t.Fatalf("brainstormTopic(%q) = %q, %v; want %q, true", input, got, ok, want)
		}
	}
	if _, ok := brainstormTopic("/brainstorming 不是命令"); ok {
		t.Fatal("brainstorming prefix must not be treated as command")
	}
}
