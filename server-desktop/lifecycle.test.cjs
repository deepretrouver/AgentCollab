const test = require("node:test");
const assert = require("node:assert/strict");
const { launchedHidden, shouldRevealInitialWindow } = require("./lifecycle.cjs");

test("configured explicit hidden launch stays hidden", () => {
  assert.equal(launchedHidden(["AgentCollab Server", "--hidden"]), true);
  assert.equal(shouldRevealInitialWindow(["AgentCollab Server", "--hidden"], true), false);
});

test("normal launch and first-run onboarding remain visible", () => {
  assert.equal(shouldRevealInitialWindow(["AgentCollab Server"], true), true);
  assert.equal(shouldRevealInitialWindow(["AgentCollab Server", "--hidden"], false), true);
});
