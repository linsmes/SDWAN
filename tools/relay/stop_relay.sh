#!/bin/bash
# 停止并禁用 Aleiyun Relay systemd 服务

set -e

SERVICE="aleiyun-relay.service"

echo "==> 停止 Aleiyun Relay"

if systemctl list-unit-files "$SERVICE" >/dev/null 2>&1; then
    systemctl stop "$SERVICE" || true
    systemctl disable "$SERVICE" || true
fi

# 同时清理旧的手动启动进程
pkill -f aleiyun_relay_Linux_X64 || true

echo "==> Relay 已停止"
