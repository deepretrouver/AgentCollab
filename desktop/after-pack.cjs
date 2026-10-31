const { execFileSync } = require("node:child_process");
const path = require("node:path");
const { packageNotices } = require("../scripts/package-notices.cjs");

module.exports = async function adhocSignClient(context) {
  const resources = context.electronPlatformName === "darwin" ? path.join(context.appOutDir, "AgentCollab.app", "Contents", "Resources") : path.join(context.appOutDir, "resources");
  packageNotices(context, resources);
  if (context.electronPlatformName !== "darwin") return;
  const application = path.join(context.appOutDir, "AgentCollab.app");
  execFileSync("/usr/bin/codesign", [
    "--force", "--deep", "--sign", "-", "--identifier", "com.agentcollab.desktop.v1", application,
  ], { stdio: "inherit" });
};
