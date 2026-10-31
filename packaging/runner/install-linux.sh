#!/bin/sh
set -eu

package_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
user_home=$(cd ~ && pwd)
install_dir="$user_home/.local/bin"
mkdir -p "$install_dir"
cp "$package_dir/agentcollab-runner" "$install_dir/agentcollab-runner"
chmod 755 "$install_dir/agentcollab-runner"
printf '%s\n' "AgentCollab Runner 已安装到 $install_dir/agentcollab-runner"
printf '%s\n' "如果 ~/.local/bin 不在 PATH，请先执行：export PATH=\"$install_dir:\$PATH\""
printf '%s\n' "现在会在浏览器打开本机配对页面；请准备 Hub 地址和一次性设备配对码。"
printf '%s\n' "Manual start only; keep this terminal open. Existing pairing is reused."
exec "$install_dir/agentcollab-runner" "$@"
