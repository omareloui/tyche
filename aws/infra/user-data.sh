#!/usr/bin/env bash
# EC2 user data of the tyche ECS host (ECS-optimized AL2023). Runs before the
# ECS agent starts, so the data volume is mounted before any task runs.
# __VOLUME_ID__ is replaced by up.sh.
set -euo pipefail

VOLUME_ID="__VOLUME_ID__"
DEVICE="/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_${VOLUME_ID/-/}"
MOUNT=/srv/tyche
LABEL=tyche-data

# The volume is attached right after launch.
for _ in $(seq 1 120); do
  [ -e "$DEVICE" ] && break
  sleep 5
done
if [ ! -e "$DEVICE" ]; then
  echo "data volume $VOLUME_ID never showed up" >&2
  exit 1
fi

# Format only a blank volume.
if ! blkid "$DEVICE" >/dev/null 2>&1; then
  mkfs.ext4 -L "$LABEL" "$DEVICE"
fi

mkdir -p "$MOUNT"
if ! grep -q "LABEL=$LABEL" /etc/fstab; then
  echo "LABEL=$LABEL $MOUNT ext4 defaults,nofail 0 2" >>/etc/fstab
fi
mountpoint -q "$MOUNT" || mount "$MOUNT"

# 1001: the nonroot user of the tyche image.
mkdir -p "$MOUNT/data" "$MOUNT/logs" "$MOUNT/caddy/data" "$MOUNT/caddy/config"
chown 1001:1001 "$MOUNT/data" "$MOUNT/logs"

cat >/etc/ecs/ecs.config <<EOF
ECS_CLUSTER=tyche
ECS_ENABLE_CONTAINER_METADATA=true
EOF
