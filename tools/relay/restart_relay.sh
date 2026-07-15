#!/bin/bash
# 重启 Aleiyun Relay 并查看状态

SERVICE="aleiyun-relay.service"

systemctl restart "$SERVICE"
sleep 1
systemctl status "$SERVICE" --no-pager
