const test = require("node:test");
const assert = require("node:assert/strict");
const { majorVersion, versionNamespace, versionedUserDataName } = require("./major-version.cjs");

test("major releases use independent application and configuration namespaces", () => {
  assert.equal(majorVersion("1.0.0"), "1");
  assert.equal(versionNamespace("2.3.4"), "v2");
  assert.equal(versionedUserDataName("AgentCollab", "1.0.0"), "AgentCollab v1");
});

test("an invalid package version cannot silently fall back to legacy data", () => {
  assert.throws(() => majorVersion("dev"), /invalid application version/);
});
