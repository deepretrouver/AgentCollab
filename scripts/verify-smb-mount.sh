#!/bin/sh
set -eu

ROOT=${1:-/srv/agentcollab}
MOUNT_POINT=${AGENTCOLLAB_SMB_MOUNT:-/mnt/team-share}

if ! test -d "$ROOT"; then
  printf 'storage root does not exist: %s\n' "$ROOT" >&2
  exit 1
fi

if ! test -w "$ROOT"; then
  printf 'storage root is not writable: %s\n' "$ROOT" >&2
  exit 1
fi

if ! mount | grep -F " on $MOUNT_POINT (smbfs" >/dev/null; then
  printf 'expected SMB mount not found at: %s\n' "$MOUNT_POINT" >&2
  exit 1
fi

PROBE="$ROOT/.agentcollab-shell-probe-$$"
umask 077
: >"$PROBE"
rm -f "$PROBE"

printf 'storage_root=%s status=ok\n' "$ROOT"
df -h "$ROOT"
