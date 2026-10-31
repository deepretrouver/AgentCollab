const fs = require("node:fs");
const path = require("node:path");

function standardRunnerConfigPath(homeDirectory, versionNamespace = "v1") {
  return path.join(homeDirectory, ".agentcollab", versionNamespace, "runner.yaml");
}

// Copies one legacy configuration into the stable per-user location exactly
// once. Existing stable configuration always wins, and the source is never
// deleted, so upgrades remain reversible.
function migrateRunnerConfig(homeDirectory, candidates = [], versionNamespace = "v1") {
  const target = standardRunnerConfigPath(homeDirectory, versionNamespace);
  if (fs.existsSync(target)) return { path: target, migratedFrom: "" };
  const source = candidates
    .map((candidate) => String(candidate || "").trim())
    .find((candidate) => candidate && path.resolve(candidate) !== path.resolve(target) && fs.existsSync(candidate));
  if (!source) return { path: target, migratedFrom: "" };

  fs.mkdirSync(path.dirname(target), { recursive: true, mode: 0o700 });
  const temporary = `${target}.tmp`;
  fs.copyFileSync(source, temporary);
  fs.chmodSync(temporary, 0o600);
  fs.renameSync(temporary, target);
  return { path: target, migratedFrom: source };
}

module.exports = { migrateRunnerConfig, standardRunnerConfigPath };
