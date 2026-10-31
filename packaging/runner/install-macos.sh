#!/bin/sh
set -eu
package_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
install_dir="$HOME/.local/bin"
mkdir -p "$install_dir"
cp "$package_dir/agentcollab-runner" "$install_dir/agentcollab-runner"
chmod 755 "$install_dir/agentcollab-runner"
printf '%s\n' "Manual Runner: keep this terminal open. Ctrl+C stops it."
printf '%s\n' "Existing pairing is reused. No login startup item is installed."
exec "$install_dir/agentcollab-runner" "$@"
