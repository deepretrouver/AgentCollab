#!/bin/sh
set -eu

plist="__LAUNCH_AGENT_PATH__"
domain="gui/$(id -u)"

if [ ! -f "$plist" ]; then
  osascript -e 'display alert "AgentCollab Client 尚未安装" message "请重新运行 AgentCollab Client 安装包。" as critical' >/dev/null 2>&1 || true
  exit 1
fi

# The application is intentionally a tiny local controller. LaunchServices
# opens it for agentcollab://reconnect, and launchd remains the durable Client.
if launchctl print "$domain/com.agentcollab.client" >/dev/null 2>&1; then
  launchctl kickstart -k "$domain/com.agentcollab.client" >/dev/null 2>&1
  exit 0
fi

# Local development installations before Client 0.3 used this label. Keeping
# the fallback makes the update itself reconnect the existing local Runner.
if launchctl print "$domain/com.agentcollab.runner" >/dev/null 2>&1; then
  launchctl kickstart -k "$domain/com.agentcollab.runner" >/dev/null 2>&1
  exit 0
fi

launchctl bootstrap "$domain" "$plist" >/dev/null 2>&1 || true
launchctl kickstart -k "$domain/com.agentcollab.client" >/dev/null 2>&1
