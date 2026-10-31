import type { Agent, User } from "../api/client";

export interface AgentOwnerGroup {
  ownerID: string;
  ownerName: string;
  owner?: User;
  agents: Agent[];
}

const agentTypeLabels: Record<string, string> = {
  "claude-code": "Claude Code",
  claude: "Claude Code",
  codex: "Codex",
  hermes: "Hermes",
  opencode: "OpenCode",
  openclaw: "OpenClaw",
  pi: "Pi Agent",
  workbuddy: "WorkBuddy",
};

function stripOwnerPrefix(value: string, prefixes: string[]) {
  const normalized = value.trim();
  for (const prefix of prefixes) {
    const candidate = prefix.trim();
    if (!candidate) continue;
    const escaped = candidate.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const match = normalized.match(new RegExp(`^${escaped}(?:\\s+|[-_:]+\\s*)`, "i"));
    if (match && normalized.slice(match[0].length).trim()) return normalized.slice(match[0].length).trim();
  }
  return normalized;
}

export function agentTypeLabel(agent: Pick<Agent, "type">) {
  const type = agent.type.trim();
  return agentTypeLabels[type.toLocaleLowerCase()] ?? type;
}

export function agentOwnerName(agent: Agent, people: User[] = []) {
  return people.find((person) => person.id === agent.owner_user_id)?.display_name || agent.owner_name || "未知成员";
}

/**
 * Old Agent records may still use the account username as their generated
 * prefix (for example `owner Codex`).  Keep the stored mention identity
 * untouched, but present it with the human's current display name.
 */
export function agentDisplayName(agent: Agent, owner?: User) {
  if (!owner) return agent.display_name;
  const suffix = stripOwnerPrefix(agent.display_name, [owner.username, agent.owner_name, owner.display_name]);
  if (suffix === agent.display_name.trim()) return agent.display_name;
  return `${owner.display_name} ${suffix}`;
}

export function agentShortName(agent: Agent, owner?: User) {
  const displayName = agentDisplayName(agent, owner);
  const suffix = stripOwnerPrefix(displayName, [owner?.display_name ?? "", owner?.username ?? "", agent.owner_name]);
  return suffix === displayName.trim() ? displayName : suffix || agentTypeLabel(agent);
}

export function groupAgentsByOwner(agentList: Agent[], people: User[] = []): AgentOwnerGroup[] {
  const owners = new Map(people.map((person) => [person.id, person]));
  const groups = new Map<string, AgentOwnerGroup>();
  for (const agent of agentList) {
    const owner = owners.get(agent.owner_user_id);
    const group = groups.get(agent.owner_user_id) ?? {
      ownerID: agent.owner_user_id,
      ownerName: owner?.display_name || agent.owner_name || "未知成员",
      owner,
      agents: [],
    };
    group.agents.push(agent);
    groups.set(agent.owner_user_id, group);
  }
  return Array.from(groups.values()).sort((left, right) => {
    const leftIndex = people.findIndex((person) => person.id === left.ownerID);
    const rightIndex = people.findIndex((person) => person.id === right.ownerID);
    if (leftIndex >= 0 && rightIndex >= 0) return leftIndex - rightIndex;
    if (leftIndex >= 0) return -1;
    if (rightIndex >= 0) return 1;
    return left.ownerName.localeCompare(right.ownerName, "zh-CN");
  });
}
