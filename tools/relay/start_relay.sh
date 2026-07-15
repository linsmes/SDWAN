#!/bin/bash
# 快速启动 Aleiyun Relay（systemd 方式）
# 用法: sudo ./start_relay.sh [controller地址] [relay-udp端口] [http管理端口] [公网地址:端口]

set -e

CONTROLLER="${1:-http://121.40.193.74:52888/api/relays/beat}"
UDP_PORT="${2:-53478}"
HTTP_PORT="${3:-127.0.0.1:58081}"
# 默认从 controller URL 推断主机名，并拼接 UDP 端口作为公网地址
CTRL_HOST="$(echo "$CONTROLLER" | sed -n 's#.*://\([^:/]*\).*#\1#p')"
PUBLIC_ADDR="${4:-${CTRL_HOST:-}:$UDP_PORT}"

BIN_DIR="/opt/SDWAN/relay"
BIN="$BIN_DIR/aleiyun_relay_Linux_X64"
SERVICE="aleiyun-relay.service"

echo "==> 部署 Aleiyun Relay"
echo "    Controller:  $CONTROLLER"
echo "    UDP 端口:    $UDP_PORT"
echo "    HTTP 管理:   $HTTP_PORT"
echo "    公网地址:    $PUBLIC_ADDR"

mkdir -p "$BIN_DIR"

if [ ! -f "$BIN" ]; then
    echo "错误: 未找到 $BIN，请先上传 aleiyun_relay_Linux_X64" >&2
    exit 1
fi

chmod +x "$BIN"

# 生成 systemd service 文件
SERVICE_FILE="/etc/systemd/system/$SERVICE"
cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Aleiyun Relay (UDP relay for SD-WAN)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$BIN_DIR
ExecStart=$BIN -udp :$UDP_PORT -http $HTTP_PORT -public-addr $PUBLIC_ADDR -timeout 5m -log /var/log/aleiyun_relay.log -controller $CONTROLLER
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE"
systemctl restart "$SERVICE"

echo "==> Relay 已启动"
sleep 1
systemctl status "$SERVICE" --no-pager
