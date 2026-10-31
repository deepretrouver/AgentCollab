function majorVersion(version) {
  const match = String(version || "").trim().match(/^(\d+)/);
  if (!match) throw new Error(`invalid application version: ${version}`);
  return match[1];
}

function versionNamespace(version) {
  return `v${majorVersion(version)}`;
}

function versionedUserDataName(applicationName, version) {
  return `${applicationName} ${versionNamespace(version)}`;
}

module.exports = { majorVersion, versionNamespace, versionedUserDataName };
