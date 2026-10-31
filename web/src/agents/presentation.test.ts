import type { Agent, User } from "../api/client";
import { agentDisplayName, agentShortName, groupAgentsByOwner } from "./presentation";

const owner: User = { id: "user-1", username: "owner", display_name: "郭昱科", role: "admin" };
const agent: Agent = {
  id: "agent-1", display_name: "owner Codex", type: "codex", owner_user_id: owner.id, owner_name: "Owner",
  device_id: "device-1", device_name: "MacBook", os: "darwin", arch: "arm64", channel: "runner/darwin", permission_mode: "admin", status: "online",
  imported: true, available: true, version: "1.0", executable: "/usr/local/bin/codex", native_history: "",
  response_mode: "mention", reply_delay_seconds: 0, role_prompt: "", max_auto_turns: 4,
};

test("旧 Agent 记录按真人显示名呈现，但保留 Agent 自身类型名称", () => {
  expect(agentDisplayName(agent, owner)).toBe("郭昱科 Codex");
  expect(agentShortName(agent, owner)).toBe("Codex");
});

test("按真人形成协作席位并保留一人多个 Agent", () => {
  const groups = groupAgentsByOwner([agent, { ...agent, id: "agent-2", display_name: "owner WorkBuddy", type: "workbuddy" }], [owner]);
  expect(groups).toHaveLength(1);
  expect(groups[0]).toMatchObject({ ownerID: owner.id, ownerName: "郭昱科" });
  expect(groups[0].agents).toHaveLength(2);
});
