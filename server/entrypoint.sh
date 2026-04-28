#!/bin/sh
set -e

SANDBOX_DIR="/sandbox"

if [ ! -f "$SANDBOX_DIR/bin/sh" ]; then
  echo "==> Bootstrapping Alpine rootfs into $SANDBOX_DIR ..."
  mkdir -p "$SANDBOX_DIR/etc/apk"
  echo "https://dl-cdn.alpinelinux.org/alpine/latest-stable/main" > "$SANDBOX_DIR/etc/apk/repositories"
  echo "https://dl-cdn.alpinelinux.org/alpine/latest-stable/community" >> "$SANDBOX_DIR/etc/apk/repositories"
  apk add --root "$SANDBOX_DIR" --initdb --no-cache --allow-untrusted \
    alpine-base bash coreutils curl wget git apk-tools
  # Copy host keys and DNS config so chroot has network access
  cp -a /etc/apk/keys "$SANDBOX_DIR/etc/apk/" 2>/dev/null || true
  cp /etc/resolv.conf "$SANDBOX_DIR/etc/resolv.conf" 2>/dev/null || true
  echo "==> Sandbox ready."
else
  echo "==> Sandbox already populated, skipping bootstrap."
fi

# Mount virtual filesystems into the sandbox.
# Try bind-mount first (requires SYS_ADMIN + seccomp:unconfined on the host);
# fall back to a fresh native mount if bind fails (works in more environments);
# warn and continue if both fail so the server is not stuck in a restart loop.
try_mount() {
  target="$1"; fstype="$2"; source="$3"
  mkdir -p "$target"
  if mountpoint -q "$target"; then
    return 0
  fi
  # Attempt 1: bind-mount from host
  if mount --bind "$source" "$target" 2>/dev/null; then
    return 0
  fi
  # Attempt 2: fresh native mount (proc/devtmpfs/sysfs)
  if mount -t "$fstype" "$fstype" "$target" 2>/dev/null; then
    return 0
  fi
  echo "WARNING: could not mount $fstype at $target -- sandbox will run in degraded mode" >&2
}

try_mount "$SANDBOX_DIR/proc" proc    /proc
try_mount "$SANDBOX_DIR/dev"  devtmpfs /dev
try_mount "$SANDBOX_DIR/sys"  sysfs   /sys

# Bind-mount skills directory so the sandbox can read/write skills
mkdir -p /data/skills "$SANDBOX_DIR/data/skills"
if ! mountpoint -q "$SANDBOX_DIR/data/skills"; then
  mount --bind /data/skills "$SANDBOX_DIR/data/skills" 2>/dev/null || \
    echo "WARNING: could not bind-mount /data/skills into sandbox" >&2
fi

# Run database migrations when the migrate binary is available (production image)
if command -v migrate > /dev/null 2>&1 && [ -n "${DATABASE_URL}" ]; then
  echo "==> Running database migrations..."
  migrate -path /migrations -database "${DATABASE_URL}" up
  echo "==> Migrations complete."
fi

exec "$@"
