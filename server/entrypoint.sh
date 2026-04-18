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

# Bind-mount virtual filesystems into sandbox
for fs in proc dev sys; do
  mkdir -p "$SANDBOX_DIR/$fs"
  mountpoint -q "$SANDBOX_DIR/$fs" || mount --bind /$fs "$SANDBOX_DIR/$fs"
done

exec "$@"
