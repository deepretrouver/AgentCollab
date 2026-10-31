import type { Agent } from "../api/client";

/** One rejected dispatch must not hide tasks already created for other agents. */
export async function dispatchVerification<A extends Pick<Agent, "id" | "display_name">, T>(
  agents: A[],
  submit: (agent: A) => Promise<T>,
) {
  const results = await Promise.allSettled(agents.map(async (agent) => submit(agent)));
  const created: T[] = [];
  const failures: { agentName: string; reason: unknown }[] = [];
  results.forEach((result, index) => {
    if (result.status === "fulfilled") created.push(result.value);
    else failures.push({ agentName: agents[index].display_name, reason: result.reason });
  });
  return { created, failures };
}
