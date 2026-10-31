const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { migrateRunnerConfig, standardRunnerConfigPath } = require("./config-paths.cjs");

test("an explicitly requested import is copied into the current major namespace", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentcollab-config-"));
  const legacy = path.join(root, "source-checkout", "runner.yaml");
  fs.mkdirSync(path.dirname(legacy), { recursive: true });
  fs.writeFileSync(legacy, "hub_url: http://127.0.0.1:8080\n", { mode: 0o644 });
  const result = migrateRunnerConfig(root, [legacy]);
  assert.equal(result.path, standardRunnerConfigPath(root, "v1"));
  assert.equal(result.migratedFrom, legacy);
  assert.equal(fs.readFileSync(result.path, "utf8"), "hub_url: http://127.0.0.1:8080\n");
  assert.equal(fs.statSync(result.path).mode & 0o777, 0o600);
});

test("existing stable config wins and is never overwritten", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentcollab-config-"));
  const target = standardRunnerConfigPath(root);
  const legacy = path.join(root, "legacy.yaml");
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.writeFileSync(target, "hub_url: http://current\n", { mode: 0o600 });
  fs.writeFileSync(legacy, "hub_url: http://legacy\n");
  const result = migrateRunnerConfig(root, [legacy]);
  assert.equal(result.migratedFrom, "");
  assert.equal(fs.readFileSync(target, "utf8"), "hub_url: http://current\n");
});

test("different major releases never share Runner configuration", () => {
  assert.notEqual(standardRunnerConfigPath("/Users/test", "v1"), standardRunnerConfigPath("/Users/test", "v2"));
  assert.match(standardRunnerConfigPath("/Users/test", "v2"), /\.agentcollab\/v2\/runner\.yaml$/);
});
