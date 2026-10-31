import { dispatchVerification } from "./verification";

const agents = [{ id: "one", display_name: "研究员" }, { id: "two", display_name: "Reviewer" }];

test("partial dispatch retains successful task and identifies denied agent without rejecting", async () => {
  const denied = new Error("Permission denied");
  const submit = vi.fn(async (agent: typeof agents[number]) => {
    if (agent.id === "two") throw denied;
    return { taskID: "created-task" };
  });
  expect(await dispatchVerification(agents, submit)).toEqual({
    created: [{ taskID: "created-task" }], failures: [{ agentName: "Reviewer", reason: denied }],
  });
  expect(submit).toHaveBeenCalledTimes(2);
});

test("successful dispatch preserves input order even when completion order differs", async () => {
  let release!: (value: string) => void;
  const first = new Promise<string>((resolve) => { release = resolve; });
  const pending = dispatchVerification(agents, (agent) => agent.id === "one" ? first : Promise.resolve("second"));
  release("first");
  expect(await pending).toEqual({ created: ["first", "second"], failures: [] });
});

test("synchronous and non-Error failures are collected without losing other dispatches", async () => {
  const result = await dispatchVerification(agents, () => { throw "Offline"; });
  expect(result.created).toEqual([]);
  expect(result.failures).toEqual(agents.map((agent) => ({ agentName: agent.display_name, reason: "Offline" })));
});
