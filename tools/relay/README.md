# Aleiyun Relay — 独立 UDP 中继服务

为无法 P2P 打通的 Aleiyun 客户端提供数据中转。该程序**完全独立于 controller**，部署在任意带公网 IP 的服务器上即可。

当前版本支持与 Aleiyun controller 联动：

- relay 主动向 controller 上报心跳
- controller 在 Web 面板实时展示 relay 在线状态
- 客户端注册/心跳时自动获取在线 relay 列表
- 客户端线路优选器会优先 P2P，P2P 失败时自动切到 relay

## 工作原理

1. 客户端向 relay 的 UDP 端口发送注册包，上报自己的 WireGuard 公钥。
2. relay 记录 `"公钥 -> 客户端 UDP 地址"` 映射（NAT 后的地址由 relay 自动学习）。
3. 客户端把需要中转的 peer 的 WireGuard `endpoint` 改成 relay 的公网地址。
4. relay 收到 WireGuard 数据包后，读取前 32 字节 receiver 公钥，查表转发到对应客户端。

## 编译

```bash
# Windows
cd tools/relay
go build -o dist/aleiyun_relay.exe .

# Linux
cd tools/relay
GOOS=linux GOARCH=amd64 go build -o dist/aleiyun_relay_Linux_X64 .
```

## 部署流程

### 1. 在 Web 面板添加中转记录

登录 controller 的 Web 面板 → 「中转」页 → 「添加中转」，填写：

| 字段 | 示例 | 说明 |
|---|---|---|
| 地址 | `121.40.193.74:53478` | relay 的公网 UDP 地址，客户端和 controller 都用它识别 |
| 备注 | `阿里云-relay` | 方便识别 |
| 公钥 | 留空 | 当前版本按地址匹配，无需填写 |

保存后记录状态为 **离线**，下一步启动 relay 后才会变绿。

### 2. 上传 relay 文件到服务器

在任意带公网 IP 的服务器上创建独立目录，不要把 relay 文件和 controller/client 混放：

```bash
mkdir -p /opt/SDWAN/relay
cd /opt/SDWAN/relay

# 上传以下文件到该目录：
# aleiyun_relay_Linux_X64
# start_relay.sh
# stop_relay.sh
# restart_relay.sh

chmod +x aleiyun_relay_Linux_X64 *.sh
```

推荐目录结构：

```text
/opt/SDWAN/
├── aleiyun_controller_Linux_X64   # controller
├── aleiyun_client_Linux_X64       # client（如需）
└── relay/                         # relay 独立目录
    ├── aleiyun_relay_Linux_X64
    ├── start_relay.sh
    ├── stop_relay.sh
    └── restart_relay.sh
```

### 3. 启动 relay

```bash
cd /opt/SDWAN/relay
sudo ./start_relay.sh <controller地址> <UDP端口> <HTTP管理端口> <公网地址:端口>
```

完整示例：

```bash
sudo ./start_relay.sh \
  http://121.40.193.74:52888/api/relays/beat \
  53478 \
  127.0.0.1:58081 \
  121.40.193.74:53478
```

该脚本会生成 systemd service 并启动，开机自启。

手动启动（不推荐生产环境使用）：

```bash
./aleiyun_relay_Linux_X64 \
  -udp :53478 \
  -public-addr 121.40.193.74:53478 \
  -http 127.0.0.1:58081 \
  -timeout 5m \
  -log /var/log/aleiyun_relay.log \
  -controller http://121.40.193.74:52888/api/relays/beat
```

## 参数说明

| 参数 | 默认 | 说明 |
|---|---|---|
| `-udp` | `:3478` | 接收客户端 WireGuard 流量和注册/心跳包的 UDP 端口 |
| `-public-addr` | 同 `-udp` | 客户端/controller 看到的公网地址，如 `1.2.3.4:53478`。若 `-udp` 是 `:53478` 等无法从外部访问的地址，必须显式指定 |
| `-http` | `:8081` | HTTP 管理接口，`0` 表示关闭；建议只监听 `127.0.0.1` |
| `-timeout` | `5m` | 客户端注册映射超时时间，超时未心跳则清理 |
| `-log` | 空 | 日志文件路径，空则只输出到 stdout |
| `-controller` | 空 | controller 心跳地址，如 `http://<controller>:52888/api/relays/beat` |
| `-controller-id` | 空 | 面板中该 relay 的 ID；空则按 `address` 自动匹配 |

## 防火墙要求

- 放行 relay 的 UDP 端口，如 `53478/udp`
- **不需要**把 `-http` 端口暴露到公网
- controller 的 `52888/tcp` 和 `52888/udp` 保持放行

## 与 controller 联动

relay 启动时带上 `-controller`，就会每 30 秒向 controller 发送一次心跳。

- controller 在 90 秒内收到心跳，面板显示 **在线**
- 超过 90 秒未收到，面板显示 **离线**

如果面板里已手动添加了 relay 地址，启动时可以通过 `-controller-id <面板中的ID>` 精确对应；不指定 ID 时，controller 会按心跳 body 里的 `address` 自动合并。

> 心跳 404 的常见原因：`-public-addr` 与面板填写的地址不一致。请确保两者完全相同，例如面板写 `121.40.193.74:53478`，启动参数也写 `121.40.193.74:53478`。

## 客户端使用

当前版本客户端已支持自动线路优选：

1. **优先 P2P**：尝试直连/打洞
2. **P2P 失败自动切 relay**：如果握手不通或 RTT 过高，自动把 peer endpoint 改为在线 relay 地址
3. **恢复探测**：P2P 恢复后自动切回直连

手动临时指定也支持：把目标 peer 的 WireGuard `endpoint` 改成 relay 的公网地址和 UDP 端口：

```text
endpoint = 121.40.193.74:53478
```

controller 下发的 relay 列表可通过 `/api/register` 和 `/api/heartbeat` 的响应字段 `relays` 获取。

## HTTP 管理接口

建议只通过 `127.0.0.1` 或 SSH 隧道访问：

- `GET /health` — 健康检查，返回 `ok`
- `GET /stats` — 当前注册客户端数量、UDP 监听地址、公网地址
- `GET /clients` — 已注册客户端列表（公钥、endpoint、最后活跃时间、流量统计）

示例：

```bash
curl http://127.0.0.1:58081/stats
# {"clients":0,"udp":":53478","public_addr":"121.40.193.74:53478"}
```

## 常用运维命令

```bash
# 查看状态
systemctl status aleiyun-relay --no-pager

# 重启 relay
sudo ./restart_relay.sh

# 停止 relay
sudo ./stop_relay.sh

# 查看日志
sudo tail -f /var/log/aleiyun_relay.log
```

## 安全建议

1. relay 本身不做认证，建议只把 UDP 端口暴露给公网，HTTP 管理接口限制为内网或 localhost 访问。
2. 生产环境建议通过 controller 下发 relay 地址，并控制哪些客户端强制走 relay，避免手动配置。
3. relay 只转发 WireGuard 数据包，无法解密内容；安全依赖 WireGuard 本身的加密。
