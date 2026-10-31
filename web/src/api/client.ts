export interface User {
  id: string;
  username: string;
  display_name: string;
	avatar_url?: string;
	bio?: string;
	current_focus?: string;
  role: "admin" | "member";
  workspace_role?: "owner" | "admin" | "member";
  room_role?: "owner" | "admin" | "member";
}

export interface Workspace {
  id: string;
  name: string;
  owner_user_id: string;
	role?: "owner" | "admin" | "member";
  created_at: string;
  updated_at: string;
}

export interface Room {
  id: string;
  workspace_id: string;
  name: string;
  created_at: string;
  pinned?: boolean;
  pinned_at?: string;
  plan_step_id?: string;
}

export interface Message {
  id: string;
  workspace_id: string;
  room_id: string;
  sequence: number;
  sender_kind: "user" | "agent" | "system";
  sender_id: string;
  kind: "chat" | "system" | "task" | "progress" | "delivery";
  body: string;
  mention_agent_ids?: string[];
  reply_to_message_id?: string;
  root_message_id?: string;
  metadata?: {
    task_id?: string;
    task_state?: Task["state"];
    task_title?: string;
    task_source?: "structured" | "legacy";
    plan_step_id?: string;
    [key: string]: unknown;
  };
  attachments?: Attachment[];
  created_at: string;
}

export interface Attachment {
  id: string;
  workspace_id: string;
  room_id: string;
  uploader_user_id: string;
  message_id?: string;
  filename: string;
  content_type: string;
  size_bytes: number;
  sha256: string;
  download_url?: string;
  created_at: string;
}

export interface KnowledgeAsset {
  id: string;
  workspace_id: string;
  room_id?: string;
  scope: "workspace" | "room";
  uploader_user_id: string;
  uploader_name?: string;
  filename: string;
  content_type: string;
  size_bytes: number;
  sha256: string;
  download_url: string;
  can_delete: boolean;
  created_at: string;
}

export interface KnowledgeCollection {
  assets: KnowledgeAsset[];
  directory: string;
}

export interface Agent {
  id: string;
  display_name: string;
  type: string;
  owner_user_id: string;
  owner_name: string;
  device_id: string;
  device_name: string;
  os: string;
  arch: string;
  channel: string;
  transport?: "native" | "acp";
  catalog_tier?: "native" | "preset" | "custom";
  protocol_version?: number;
  permission_mode: "admin" | "restricted";
  status: "online" | "offline" | "busy" | "unavailable";
  imported: boolean;
  available: boolean;
	version: string;
	runner_version?: string;
	runner_last_seen_at?: string;
  queued_task_count?: number;
  running_task_count?: number;
  native_session_ready?: boolean;
  executable: string;
  native_history: string;
  skills?: Array<{ name: string; description?: string; source?: string }>;
  resources?: Array<{ name: string; description?: string; access: string }>;
  response_mode: "mention" | "coordinator" | "human_all" | "muted";
  reply_delay_seconds: number;
  role_prompt: string;
  max_auto_turns: number;
	visibility?: "private" | "workspace";
	invocation_scope?: "owner_only" | "room_members";
	shared_skills?: string[];
	can_manage?: boolean;
	can_moderate?: boolean;
	can_invoke?: boolean;
}

export interface Task {
  id: string;
  work_item_id?: string;
  plan_step_id?: string;
  execution_source?: TaskExecutionSource;
  workspace_id: string;
  room_id: string;
  parent_task_id?: string;
  root_message_id?: string;
  source_message_id?: string;
  collaboration_depth: number;
  creator_kind: "user" | "agent";
  creator_id: string;
  target_agent_id: string;
  title: string;
  prompt: string;
  acceptance_criteria: string;
  state: "queued" | "dispatched" | "running" | "delivered" | "failed" | "blocked" | "cancelled";
  current_attempt: number;
  created_at: string;
  updated_at: string;
}

export type WorkItemStatus = "backlog" | "ready" | "in_progress" | "review" | "done" | "cancelled";
export type WorkItemPriority = "low" | "normal" | "high" | "urgent";
export type ProjectPlanStatus = "planning" | "active" | "completed" | "cancelled";
export type ProjectStepStatus = "planned" | "in_progress" | "review" | "done" | "cancelled";
export type TaskExecutionSource = "official" | "responsible_local" | "none";

export interface WorkItemReview {
  branch: string;
  base_commit: string;
  head_commit: string;
  mergeable: boolean;
  file_count: number;
  additions: number;
  deletions: number;
  changed_files: Array<{ path: string; status: string; additions: number; deletions: number }>;
}

export interface ProjectStep {
  id: string;
  workspace_id: string;
  parent_step_id?: string;
  title: string;
  description: string;
  weight: number;
  status: ProjectStepStatus;
  sort_order: number;
  progress: number;
  linked_item_count: number;
  responsible_user_ids: string[];
  responsible_agent_ids: string[];
  workspace?: ProjectStepWorkspace;
  local_bindings?: ProjectStepLocalBinding[];
  created_at: string;
  updated_at: string;
}

export interface ProjectStepLocalBinding {
  step_id: string;
  user_id: string;
  runner_id: string;
  device_id: string;
  device_name: string;
  directory_label: string;
  baseline_commit?: string;
  runner_status: "online" | "offline" | "revoked";
  last_seen_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ProjectStepWorkspace {
  step_id: string;
  source_name: string;
  file_count: number;
  total_bytes: number;
  revision: number;
  last_commit: string;
  updated_by_user_id?: string;
  created_at: string;
  updated_at: string;
}

export interface ProjectPlan {
  workspace_id: string;
  objective: string;
  status: ProjectPlanStatus;
  progress: number;
  can_manage: boolean;
  steps: ProjectStep[];
  created_at?: string;
  updated_at?: string;
}

export interface ProjectRepository {
  initialized: boolean;
  default_branch: string;
  storage_path: string;
  transport: string;
  clone_url: string;
  main_protected: boolean;
}

export interface WorkItem {
  id: string;
  workspace_id: string;
  room_id: string;
  root_message_id?: string;
  plan_step_id?: string;
  git_branch?: string;
  merge_status: "none" | "open" | "merged" | "conflict";
  merged_commit?: string;
  branch_commit?: string;
  review_agent_id?: string;
  verification_agent_id?: string;
  review_task_id?: string;
  verification_task_id?: string;
  ai_review_status?: "none" | "pending" | "passed" | "changes_requested" | "failed";
  ai_review_summary?: string;
  verification_status?: "none" | "pending" | "passed" | "failed";
  verification_summary?: string;
  kind: "task" | "discussion" | "brainstorm";
  title: string;
  description: string;
  status: WorkItemStatus;
  priority: WorkItemPriority;
  owner_user_id?: string;
  due_at?: string;
  accepted_at?: string;
  task_count: number;
  active_task_count: number;
  delivered_task_count: number;
  failed_task_count: number;
  target_agent_ids: string[];
  created_at: string;
  updated_at: string;
}

export interface RunnerInvite {
  id: string;
  code: string;
  workspace_id: string;
  owner_user_id: string;
  expires_at: string;
  hub_urls: string[];
}

export interface HumanInvite {
  id: string;
  code: string;
  workspace_id: string;
  room_id: string;
  role: "admin" | "member";
  expires_at: string;
  hub_urls: string[];
}

export interface RunnerReleaseManifest {
  version: string;
  published_at?: string;
  packages: Array<{
    platform: string;
    filename: string;
    size_bytes: number;
    sha256: string;
    download_url: string;
  }>;
}

interface ErrorBody {
  error?: {
    code?: string;
    message?: string;
  };
}

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body && !(init.body instanceof FormData)) headers.set("Content-Type", "application/json");
  const response = await fetch(path, {
    ...init,
    headers,
    credentials: "same-origin",
  });
  if (!response.ok) {
    let body: ErrorBody = {};
    try {
      body = (await response.json()) as ErrorBody;
    } catch {
      // A proxy or unavailable Hub may return a non-JSON error.
    }
    throw new ApiError(
      response.status,
      body.error?.code ?? "request_failed",
      body.error?.message ?? `请求失败 (${response.status})`,
    );
  }
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

function workspaceRelativePath(file: File) {
  const candidate = (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name;
  const segments = candidate.split("/").filter(Boolean);
  return segments.length > 1 ? segments.slice(1).join("/") : segments[0] || file.name;
}

async function uploadWorkspaceDirectory<T>(path: string, files: File[]) {
  const body = new FormData();
  const selected = files.filter((file) => !workspaceRelativePath(file).split("/").some((segment) => [".git", ".agentcollab", "node_modules", ".DS_Store"].includes(segment)));
  const first = selected[0] as (File & { webkitRelativePath?: string }) | undefined;
  const sourceName = first?.webkitRelativePath?.split("/").filter(Boolean)[0] || "本地目录";
  body.append("source_name", sourceName);
  for (const file of selected) {
    body.append("files", file, file.name);
    body.append("paths", workspaceRelativePath(file));
  }
  return request<T>(path, { method: "POST", body });
}

export const api = {
	runnerRelease: () => request<RunnerReleaseManifest>("/downloads/runner/manifest"),
  login: (username: string, password: string) =>
    request<User>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  register: (input: { username: string; display_name: string; password: string }) =>
    request<User>("/api/v1/auth/register", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  registerWithInvite: (code: string, input: { username: string; display_name: string; password: string }) =>
    request<{ user: User; workspace_id: string; role: string }>(`/api/v1/invites/${encodeURIComponent(code)}/register`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  acceptInvite: (code: string) =>
    request<{ workspace_id: string; user_id: string; role: string; joined_at: string }>(`/api/v1/invites/${encodeURIComponent(code)}/accept`, {
      method: "POST",
    }),
  logout: () => request<void>("/api/v1/auth/logout", { method: "POST" }),
  me: () => request<User>("/api/v1/auth/me"),
	updateProfile: (input: Pick<User, "display_name" | "avatar_url" | "bio" | "current_focus">) =>
		request<User>("/api/v1/auth/me", { method: "PATCH", body: JSON.stringify(input) }),
	hubUsers: async () => (await request<{ users: User[] }>("/api/v1/admin/users")).users,
	updateHubUserRole: (userID: string, role: User["role"]) =>
		request<User>(`/api/v1/admin/users/${encodeURIComponent(userID)}`, { method: "PATCH", body: JSON.stringify({ role }) }),
	archiveHubUser: (userID: string) =>
		request<void>(`/api/v1/admin/users/${encodeURIComponent(userID)}`, { method: "DELETE" }),
	runnerOwnership: (runnerID: string) =>
		request<{ owned_by_current_user: boolean }>(`/api/v1/runners/${encodeURIComponent(runnerID)}/ownership`),
  workspaces: async () => (await request<{ workspaces: Workspace[] }>("/api/v1/workspaces")).workspaces,
  createWorkspace: (name: string) =>
    request<Workspace>("/api/v1/workspaces", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  updateWorkspace: (workspaceID: string, name: string) =>
    request<Workspace>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}`, {
      method: "PATCH",
      body: JSON.stringify({ name }),
    }),
  deleteWorkspace: (workspaceID: string) =>
    request<void>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}`, { method: "DELETE" }),
  workspaceMembers: async (workspaceID: string) =>
    (await request<{ members: User[] }>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/members`)).members,
  updateWorkspaceMember: (workspaceID: string, userID: string, role: "admin" | "member") =>
    request<void>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/members/${encodeURIComponent(userID)}`, { method: "PATCH", body: JSON.stringify({ role }) }),
  removeWorkspaceMember: (workspaceID: string, userID: string) =>
    request<void>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/members/${encodeURIComponent(userID)}`, { method: "DELETE" }),
  createRunnerInvite: (workspaceID: string, expiresInSeconds = 30 * 60) =>
    request<RunnerInvite>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/runner-invites`, {
      method: "POST",
      body: JSON.stringify({ expires_in_seconds: expiresInSeconds }),
    }),
  createHumanInvite: (workspaceID: string, roomID: string, expiresInSeconds = 24 * 60 * 60) =>
    request<HumanInvite>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/invites`, {
      method: "POST",
      body: JSON.stringify({ role: "member", room_id: roomID, expires_in_seconds: expiresInSeconds }),
    }),
  rooms: async (workspaceID: string) =>
    (await request<{ rooms: Room[] }>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/rooms`)).rooms,
  createRoom: (workspaceID: string, name: string) =>
    request<Room>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/rooms`, {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  setRoomPinned: (roomID: string, pinned: boolean) =>
    request<Room>(`/api/v1/rooms/${encodeURIComponent(roomID)}/preferences`, {
      method: "PATCH",
      body: JSON.stringify({ pinned }),
    }),
  messages: async (roomID: string, after = 0) =>
    (await request<{ messages: Message[] }>(`/api/v1/rooms/${encodeURIComponent(roomID)}/messages?limit=100&after=${after}`)).messages,
  uploadAttachment: (roomID: string, file: File) => {
    const body = new FormData();
    body.append("file", file, file.name);
    return request<Attachment>(`/api/v1/rooms/${encodeURIComponent(roomID)}/attachments`, { method: "POST", body });
  },
  workspaceKnowledge: (workspaceID: string) =>
    request<KnowledgeCollection>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/knowledge`),
  roomKnowledge: (roomID: string) =>
    request<KnowledgeCollection>(`/api/v1/rooms/${encodeURIComponent(roomID)}/knowledge`),
  uploadWorkspaceKnowledge: (workspaceID: string, file: File) => {
    const body = new FormData();
    body.append("file", file, file.name);
    return request<KnowledgeAsset>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/knowledge`, { method: "POST", body });
  },
  uploadRoomKnowledge: (roomID: string, file: File) => {
    const body = new FormData();
    body.append("file", file, file.name);
    return request<KnowledgeAsset>(`/api/v1/rooms/${encodeURIComponent(roomID)}/knowledge`, { method: "POST", body });
  },
  deleteWorkspaceKnowledge: (workspaceID: string, assetID: string) =>
    request<void>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/knowledge/${encodeURIComponent(assetID)}`, { method: "DELETE" }),
  deleteRoomKnowledge: (roomID: string, assetID: string) =>
    request<void>(`/api/v1/rooms/${encodeURIComponent(roomID)}/knowledge/${encodeURIComponent(assetID)}`, { method: "DELETE" }),
  sendMessage: (roomID: string, body: string, mentionAgentIDs: string[] = [], attachmentIDs: string[] = [], thread?: { reply_to_message_id: string; root_message_id: string }, planStepID?: string, executionSource?: TaskExecutionSource) =>
    request<Message>(`/api/v1/rooms/${encodeURIComponent(roomID)}/messages`, {
      method: "POST",
      body: JSON.stringify({ body, mention_agent_ids: mentionAgentIDs, attachment_ids: attachmentIDs, ...thread, plan_step_id: planStepID || undefined, execution_source: executionSource }),
    }),
  members: async (roomID: string) =>
    (await request<{ members: User[] }>(`/api/v1/rooms/${encodeURIComponent(roomID)}/members`)).members,
  addRoomMember: (roomID: string, userID: string) =>
    request<void>(`/api/v1/rooms/${encodeURIComponent(roomID)}/members`, { method: "POST", body: JSON.stringify({ user_id: userID }) }),
  updateRoomMember: (roomID: string, userID: string, role: "admin" | "member") =>
    request<void>(`/api/v1/rooms/${encodeURIComponent(roomID)}/members/${encodeURIComponent(userID)}`, { method: "PATCH", body: JSON.stringify({ role }) }),
  removeRoomMember: (roomID: string, userID: string) =>
    request<void>(`/api/v1/rooms/${encodeURIComponent(roomID)}/members/${encodeURIComponent(userID)}`, { method: "DELETE" }),
  agents: async (workspaceID: string) =>
    (await request<{ agents: Agent[] }>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/agents`)).agents,
  agentCandidates: async (workspaceID: string) =>
    (await request<{ agents: Agent[] }>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/agent-candidates`)).agents,
  importAgent: (workspaceID: string, agentID: string) =>
    request<void>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/agents/import`, {
      method: "POST",
      body: JSON.stringify({ agent_id: agentID }),
    }),
  removeAgent: (workspaceID: string, agentID: string) =>
    request<void>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/agents/${encodeURIComponent(agentID)}`, {
      method: "DELETE",
    }),
  updateAgentPolicy: (workspaceID: string, agentID: string, input: { visibility: "private" | "workspace"; invocation_scope: "owner_only" | "room_members"; shared_skills: string[] }) =>
    request<void>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/agents/${encodeURIComponent(agentID)}/policy`, { method: "PATCH", body: JSON.stringify(input) }),
  roomAgents: async (roomID: string) =>
    (await request<{ agents: Agent[] }>(`/api/v1/rooms/${encodeURIComponent(roomID)}/agents`)).agents,
  addRoomAgent: (roomID: string, agentID: string) =>
    request<void>(`/api/v1/rooms/${encodeURIComponent(roomID)}/agents`, {
      method: "POST",
      body: JSON.stringify({ agent_id: agentID }),
    }),
  removeRoomAgent: (roomID: string, agentID: string) =>
    request<void>(`/api/v1/rooms/${encodeURIComponent(roomID)}/agents/${encodeURIComponent(agentID)}`, {
      method: "DELETE",
    }),
  updateRoomAgentSettings: (roomID: string, agentID: string, input: { response_mode: Agent["response_mode"]; reply_delay_seconds: number; role_prompt: string; max_auto_turns: number }) =>
    request<void>(`/api/v1/rooms/${encodeURIComponent(roomID)}/agents/${encodeURIComponent(agentID)}/settings`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  tasks: async (roomID: string) =>
    (await request<{ tasks: Task[] }>(`/api/v1/rooms/${encodeURIComponent(roomID)}/tasks`)).tasks,
  workItems: async (roomID: string) =>
    (await request<{ work_items: WorkItem[] }>(`/api/v1/rooms/${encodeURIComponent(roomID)}/work-items`)).work_items,
  updateWorkItem: (workItemID: string, input: Partial<{ title: string; status: WorkItemStatus; priority: WorkItemPriority; owner_user_id: string; clear_owner: boolean; due_at: string; clear_due_at: boolean; plan_step_id: string; clear_plan_step: boolean }>) =>
    request<WorkItem>(`/api/v1/work-items/${encodeURIComponent(workItemID)}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  mergeWorkItem: (workItemID: string) => request<WorkItem>(`/api/v1/work-items/${encodeURIComponent(workItemID)}/merge`, { method: "POST" }),
  startWorkItemAIReview: (workItemID: string, reviewAgentID: string, verificationAgentID: string) => request<WorkItem>(`/api/v1/work-items/${encodeURIComponent(workItemID)}/ai-review`, { method: "POST", body: JSON.stringify({ review_agent_id: reviewAgentID, verification_agent_id: verificationAgentID }) }),
  rejectWorkItem: (workItemID: string, reason: string) => request<WorkItem>(`/api/v1/work-items/${encodeURIComponent(workItemID)}/reject`, { method: "POST", body: JSON.stringify({ reason }) }),
  reviewWorkItem: (workItemID: string) => request<WorkItemReview>(`/api/v1/work-items/${encodeURIComponent(workItemID)}/review`),
  submitWorkItemWorkspace: (workItemID: string, files: File[]) => uploadWorkspaceDirectory<WorkItem>(`/api/v1/work-items/${encodeURIComponent(workItemID)}/submit-workspace`, files),
  projectPlan: (workspaceID: string) => request<ProjectPlan>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/project-plan`),
  projectRepository: (workspaceID: string) => request<ProjectRepository>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/repository`),
  updateProjectPlan: (workspaceID: string, input: Partial<{ objective: string; status: ProjectPlanStatus }>) =>
    request<ProjectPlan>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/project-plan`, { method: "PATCH", body: JSON.stringify(input) }),
  createProjectStep: (workspaceID: string, input: { parent_step_id?: string; title: string; description?: string; weight: number; status?: ProjectStepStatus; sort_order?: number; responsible_user_ids?: string[]; responsible_agent_ids?: string[] }) =>
    request<ProjectStep>(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/project-steps`, { method: "POST", body: JSON.stringify(input) }),
  updateProjectStep: (stepID: string, input: Partial<{ parent_step_id: string; clear_parent: boolean; title: string; description: string; weight: number; status: ProjectStepStatus; sort_order: number; responsible_user_ids: string[]; responsible_agent_ids: string[] }>) =>
    request<ProjectStep>(`/api/v1/project-steps/${encodeURIComponent(stepID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  deleteProjectStep: (stepID: string) => request<void>(`/api/v1/project-steps/${encodeURIComponent(stepID)}`, { method: "DELETE" }),
  uploadProjectStepWorkspace: (stepID: string, files: File[]) => uploadWorkspaceDirectory<ProjectStep>(`/api/v1/project-steps/${encodeURIComponent(stepID)}/workspace`, files),
  removeProjectStepWorkspace: (stepID: string) => request<ProjectStep>(`/api/v1/project-steps/${encodeURIComponent(stepID)}/workspace`, { method: "DELETE" }),
  saveProjectStepLocalBinding: (stepID: string, input: { runner_id: string; device_id: string; directory_label: string }) => request<ProjectStep>(`/api/v1/project-steps/${encodeURIComponent(stepID)}/local-bindings/current`, { method: "PUT", body: JSON.stringify(input) }),
  removeProjectStepLocalBinding: (stepID: string, runnerID: string) => request<ProjectStep>(`/api/v1/project-steps/${encodeURIComponent(stepID)}/local-bindings/current/${encodeURIComponent(runnerID)}`, { method: "DELETE" }),
  updateRoomProjectStep: (roomID: string, planStepID?: string) => request<void>(`/api/v1/rooms/${encodeURIComponent(roomID)}/project-step`, { method: "PATCH", body: JSON.stringify(planStepID ? { plan_step_id: planStepID } : { clear_plan_step: true }) }),
  createTask: async (roomID: string, input: { target_agent_id: string; title: string; prompt: string; acceptance_criteria: string; plan_step_id?: string; execution_source?: TaskExecutionSource }) =>
    (await request<{ task: Task; message: Message }>(`/api/v1/rooms/${encodeURIComponent(roomID)}/tasks`, {
      method: "POST",
      body: JSON.stringify(input),
    })),
  cancelTaskRoot: async (roomID: string, rootMessageID: string) =>
    (await request<{ tasks: Task[]; message?: Message }>(`/api/v1/rooms/${encodeURIComponent(roomID)}/task-roots/${encodeURIComponent(rootMessageID)}/cancel`, {
      method: "POST",
    })),
};
