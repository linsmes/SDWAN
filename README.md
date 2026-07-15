# Aleiyun — 自建虚拟局域网(WireGuard + Web 总控面板)

基于 wireguard-go 的虚拟局域网方案:设备装 client(agent)即自动组网,中心 server(controller)提供 Web 总控面板。

交付物命名:`aleiyun_<角色>_<平台>_<架构>`,如 `aleiyun_client_Windows_X64.exe`、`aleiyun_server_Linux_X64`。

## 架构

```
                 ┌─────────────────────┐
                 │    server           │  设备注册 / 虚拟IP分配 / peer拓扑下发
                 │  (Web 面板 + API)   │  数据存 data.json
                 └─────────┬───────────┘
              注册/心跳    │    注册/心跳
         ┌─────────────────┴──────────────────┐
   ┌─────┴─────┐                      ┌───────┴───┐
   │  client A │ ← WireGuard P2P →    │  client B │
   │ 10.66.0.2 │   (直连/打洞)        │ 10.66.0.3 │
   └───────────┘                      └───────────┘
```

- `wireguard-go/` — WireGuard 用户态实现源码(隧道引擎,**不改协议**)
- `controller/` — 总控面板(server),零依赖,Web 页面内嵌在二进制里
- `agent/` — 客户端(client),包装 wireguard-go,负责注册、隧道、peer 同步

## 编译

需要 Go 1.26+。

```bash
# 产物统一输出到 dist/,源码目录保持整洁
# server(controller)
go build -o dist/aleiyun_server_Windows_X64.exe ./controller/
GOOS=linux GOARCH=amd64 go build -o dist/linux/aleiyun_server_Linux_X64 ./controller/

# client(agent)
go build -o dist/AleiyunClient/aleiyun_client_Windows_X64.exe ./agent/
GOOS=linux GOARCH=amd64 go build -o dist/linux/aleiyun_client_Linux_X64 ./agent/

# relay(独立 UDP 中继)
cd tools/relay
go build -o dist/aleiyun_relay.exe .
GOOS=linux GOARCH=amd64 go build -o dist/aleiyun_relay_Linux_X64 .
```

## 运行

```bash
# 1. 启动总控面板(默认 :52888)
./controller/aleiyun_server_Windows_X64.exe
# 浏览器打开 http://localhost:52888

# Linux 生产部署示例
./aleiyun_server_Linux_X64 -addr :52888 \
  -data /var/lib/aleiyun/data.json \
  -admin-db /var/lib/aleiyun/admin.json \
  -admin-user linsme -admin-pass linsme123
# 首次启动会在 -admin-db 中创建默认管理员,之后修改密码请直接改该 JSON 文件(需哈希存储)

# 2. 各主机上启动 client
#    编辑 aleiyun_client.json(填 controller)后双击 aleiyun_client_Windows_X64.exe 即可,自动申请管理员权限(UAC)
#    也可用命令行参数覆盖配置:
./agent/aleiyun_client_Windows_X64.exe -controller http://<controller的IP>:52888 -name 我的电脑

# 调试模式:只注册不起隧道,无需管理员权限
./agent/aleiyun_client_Windows_X64.exe -controller http://127.0.0.1:52888 -name 测试机 -no-tun
```

client 启动时读取 exe 同目录的 `aleiyun_client.json`(首次运行自动生成,需填写 `controller` 后重跑),
优先级:**命令行参数 > aleiyun_client.json > 默认值**,字段对应见 `agent/aleiyun_client.json.example`。
配置文件支持 `//` 行注释和 `/* */` 块注释(字符串内的 `http://` 不受影响),默认生成的文件自带各字段说明,
并自动探测本机所有 LAN 网段写成注释行——要共享哪个网段,取消对应行注释填到 `lan` 即可,无需手动查 ipconfig。
若同目录存在旧配置 `agent.json`,启动时会打一行提示日志,建议改名为 `aleiyun_client.json`。

Windows 下 `wintun.dll` 已内嵌进 aleiyun_client_Windows_X64.exe,首次运行自动释放到同目录,
**单文件分发即可**,无需手动安装驱动。

新设备自动注册、自动分配 10.66.0.x 虚拟 IP,面板实时显示在线状态。
client 每 15s 向 server 发一次心跳,server 以 45s 在线窗口(3 倍心跳间隔)判定设备在线。
client 正常退出(Ctrl+C / SIGTERM)时会主动通知 server 立即下线,来不及通知的由超时窗口兜底。
**设备管理**:设备行有"下线"(仅标记离线,下次心跳会恢复)、"禁用"(封禁该设备及相同 MAC 地址的机器,换密钥也无效)、"移除"按钮;面板自动显示设备上报的 MAC 地址作为机器指纹。

设备间通过 WireGuard P2P 直连,`ping 10.66.0.x` 即通。

## 常用参数(client)

| 参数 | 默认 | 说明 |
|---|---|---|
| `-controller` | `http://127.0.0.1:52888` | server(controller)地址。支持裸 IP/裸域名/带端口/带路径等写法,自动补全:无 scheme 补 `http://`;缺端口时 `https` 补 `:443`、其余补 `:52888`(如 `example.com` → `http://example.com:52888`,`https://example.com` → `https://example.com:443`) |
| `-name` | 主机名 | 设备名称 |
| `-username` | 空 | 注册账号(v0.7.0,在面板"账号"页创建) |
| `-password` | 空 | 注册账号密码 |
| `-key` | `aleiyun_client.key` | 私钥文件(自动生成,删除即换身份) |
| `-if` | `aleiyun_sdwan0` | 虚拟网卡名 |
| `-listen` | `51820` | WireGuard 监听端口 |
| `-log` | `aleiyun_client.log` | 日志文件(屏幕和文件同时输出) |
| `-endpoint` | 自动探测 | 手动指定内网 endpoint(IP:端口) |
| `-stun` | 同 controller | STUN 服务器;server 在局域网时自动跳过探测 |
| `-no-tun` | false | 只注册不起隧道 |
| `-debug` | false | 输出 WireGuard 握手细节 |
| `-lan` | 空 | 站点到站点:本机 LAN 网段(逗号分隔 CIDR),如 `192.168.1.0/24,10.10.0.0/24` |
| `-network` | 空(默认网络) | **v0.7.0 起废弃**:所属网络由账号绑定决定,此参数被服务端忽略 |

## 多网络集群管理(多租户)

v0.5.0 起支持**多虚拟网络**:每个网络集群有独立网段和地址池,不同网络默认互相隔离,
适合把不同公司/部门的设备分开管理。全部操作在 Web 面板完成,分三步:

1. **面板建网络集群**:在"网络集群管理"区填写名称 + CIDR(如 `A公司` / `10.10.10.0/24`)新建。
   CIDR 不能非法、不能与已有网络重叠,名称不能重复;非空网络不允许删除。
2. **客户端配 network**:client 加 `-network A公司` 或配置 `"network": "A公司"`,
   注册后自动分到该网络的虚拟 IP(如 10.10.10.x);留空加入"默认网络"(10.66.0.0/24,兼容旧版)。
   > v0.7.0 起此方式废弃:设备归属改由账号绑定决定,见下节「账号体系」。
3. **面板建互联规则**:在"互联规则"区添加规则打通隔离的网络——
   `network` 类型整网互通(双方自动加对端 CIDR 路由),`device` 类型只通指定的两台设备
   (自动加对端虚拟 IP/32 路由)。**未建立规则的网络互相隔离**,同网络设备默认互通无需建规则。

旧数据自动迁移:升级后首次启动自动创建"默认网络",所有存量设备归入其中,行为与旧版一致。

## 账号体系(v0.7.0)

v0.7.0 起**关闭公开注册**:客户端必须凭账号密码注册,账号只能在 Web 面板创建,
**所属网络(公司)由账号绑定值决定**,客户端自报的 `network` 被忽略。

1. **面板建账号**:在"账号"页填写用户名、密码(留空则随机生成 10 位)、选择所属公司(网络)、
   设备数上限(默认 1,即一个账号只允许绑定一台设备,之后可点"调整上限"修改)。
   创建成功(或"重置密码")后明文密码**只显示一次**,请立即复制给设备使用方。
   绑定设备数达上限时新设备注册被拒(403,提示删除闲置设备或调大上限);同一设备重注册不受限。
2. **客户端填账密**:配置 `"username"` / `"password"`(或命令行 `-username` / `-password`)。
   账密错误或未创建账号时注册被拒(403)。
3. 密码存储:客户端注册密码服务端存 `sha256(salt+password)`,不明文保存;`/api/accounts` 绝不返回 hash/salt。
   管理员登录密码则按 `sha256(salt+md5(password))` 存储,前端传输前会先对密码做 MD5。
4. 防爆破:`/api/register` 对失败注册按源 IP 做速率限制,5 分钟内超过 10 次失败则返回 429,请 5 分钟后再试。
5. 心跳只按公钥识别设备,不重复验证账密;删除账号前需先移除其关联设备。

旧设备(升级前已注册,无账号)心跳不受影响;重新注册则需要账号。

## 管理后台登录

Web 面板使用独立的**管理员账号体系**(存于 `-admin-db` 指定的 JSON 文件,默认 `/var/lib/aleiyun/admin.json`),
与客户端注册账号分开。

- 首次启动若 admin 数据库为空,自动用 `-admin-user` / `-admin-pass` 创建默认管理员。
- 登录时前端先把密码做 **MD5 hex**,再 POST 到 `/api/admin/login`,服务端存储为 `sha256(salt + md5(password))`,
  全程密码不以明文传输或明文保存。
- 登录成功后服务端写 HttpOnly Session Cookie,24h 内免密;调用 `/api/admin/logout` 可登出。
- 面板右上角提供「修改密码」,修改成功后当前 Session 失效,需重新登录。
- Windows 客户端 API(`/api/register`、`/api/heartbeat` 等)不受管理员登录影响,保持公开。

## 站点到站点(多网段互联)

在 Mesh 互联的基础上,让两个**局域网**通过各自的 client 节点互通(站点到站点)。
配了 `lan` 的节点成为该站点的**网关**:它把 LAN 网段通告给 controller,
controller 下发给所有 peer,各 client 自动加路由、扩展 WireGuard allowed IPs,
并把流量经隧道转发进对端 LAN。

```
   LAN A 192.168.1.0/24                    LAN B 192.168.2.0/24
  ┌──────────────┐                       ┌──────────────┐
  │ 主机A1       │                       │ 主机B1       │
  │ 192.168.1.11 │                       │ 192.168.2.11 │
  └──────┬───────┘                       └───────┬──────┘
         │                                       │
  ┌──────┴───────┐   WireGuard 隧道    ┌────────┴─────┐
  │ client A(网关)│ ←→ 10.66.0.0/24 ←→ │ client B(网关)│
  │ 192.168.1.2  │                     │ 192.168.2.2  │
  └──────────────┘                     └──────────────┘
```

**网关配置**(client A 的 `aleiyun_client.json`):

```json
{
  "controller": "http://<controller的IP>:52888",
  "name": "站点A-网关",
  "lan": "192.168.1.0/24"
}
```

多个网段用逗号分隔:`"lan": "192.168.1.0/24,10.10.0.0/24"`。非法网段会被跳过(日志提示);
**两个站点通告同一网段时,controller 做冲突检测——先注册者保留,后者被剔除并在面板标红**(不拒绝注册)。

**LAN 主机的回程路由**:对端 LAN 的主机(如主机B1)需要知道回 192.168.1.0/24 的路由,二选一:

- 在 LAN 主机上加静态路由(网关填本站 client 节点的 LAN IP):
  - Windows:`route add 192.168.1.0 mask 255.255.255.0 192.168.2.2`
  - Linux:`ip route add 192.168.1.0/24 via 192.168.2.2`
- 或依赖网关 SNAT(仅 Linux 网关):网关自动开 IP 转发,并对"隧道 → LAN"的流量做
  `iptables -t nat -A POSTROUTING -j MASQUERADE`,LAN 主机**无需任何配置**,
  但看到的是网关的 IP 而不是真实来源 IP。

**Windows 网关限制**:Windows 版 client 会开启 IP 转发(注册表 `IPEnableRouter=1` + 各 LAN 网卡
`Forwarding Enabled`),但 **SNAT 未配置**(Windows NetNat 按源前缀 NAT,无法表达隧道回程转换),
日志会告警;请在 LAN 主机加回程路由,或自行配置 NAT。生产环境建议用 Linux 主机做网关。

## 跨公网部署

server 需放在**有公网 IP** 的机器上(如云服务器),放行 **TCP+UDP 52888**:

- TCP:Web 面板 + 注册/心跳 API
- UDP:内置 STUN 服务,client 据此探测 NAT 后的公网映射地址

client 无需任何额外配置:注册时同时上报内网和公网 endpoint,
server 自动判断——两端在同一 NAT 后面走内网直连,跨 NAT 走公网地址,
配合 WireGuard 的 persistent keepalive(25s)完成 NAT 打洞。

> 对称 NAT(部分企业网/移动网络)打洞会失败,需要中继节点兜底。

## 中继节点(对称 NAT 兜底)

对于无法 P2P 打通的场景(如对称 NAT、移动网络、企业防火墙),提供独立的 UDP 中继服务 `tools/relay`。
它部署在**任意带公网 IP 的服务器**上即可,与 controller 解耦,只负责按 WireGuard receiver 公钥转发 UDP 包。

### 部署 relay

```bash
cd tools/relay
GOOS=linux GOARCH=amd64 go build -o dist/aleiyun_relay_Linux_X64 .

# 方式一:只启动 relay(手动在面板添加中转地址)
./dist/aleiyun_relay_Linux_X64 -udp :53478 -public-addr 121.40.193.74:53478 \
  -http 127.0.0.1:58081 -timeout 5m -log /var/log/aleiyun_relay.log

# 方式二:启动时自动向 controller 上报心跳(推荐)
./dist/aleiyun_relay_Linux_X64 -udp :53478 -public-addr 121.40.193.74:53478 \
  -http 127.0.0.1:58081 -timeout 5m -log /var/log/aleiyun_relay.log \
  -controller http://121.40.193.74:52888/api/relays/beat

# 方式三:用 systemd 脚本部署(推荐生产环境)
# 把 dist/aleiyun_relay_Linux_X64 和 tools/relay/*.sh 上传到 /opt/SDWAN/relay/ 后执行:
sudo ./start_relay.sh \
  http://121.40.193.74:52888/api/relays/beat \
  53478 \
  127.0.0.1:58081 \
  121.40.193.74:53478
```

参数说明:

| 参数 | 默认 | 说明 |
|---|---|---|
| `-udp` | `:3478` | 接收客户端 WireGuard 流量和注册/心跳包的 UDP 端口 |
| `-public-addr` | 同 `-udp` | 客户端/controller 看到的公网地址,如 `1.2.3.4:53478`。若 `-udp` 是 `:53478` 等无法从外部访问的地址,必须显式指定 |
| `-http` | `:8081` | HTTP 管理接口,`0` 表示关闭;建议只监听 `127.0.0.1` |
| `-timeout` | `5m` | 客户端映射超时时间,超时未心跳则清理 |
| `-log` | 空 | 日志文件路径,空则输出到 stdout |
| `-controller` | 空 | Controller 心跳地址,如 `http://<controller>:52888/api/relays/beat` |
| `-controller-id` | 空 | 面板中该 relay 的 ID,空则按 address 自动匹配 |

防火墙只需放行 `-udp` 端口(示例中为 `53478/udp`)。HTTP 管理接口建议不暴露公网。

### 面板管理 relay

启动并带上 `-controller` 后,relay 每 30 秒向 controller 发一次心跳,面板「中转」页会显示其在线状态:

- 在线:最近一次心跳在 90 秒内。
- 离线:超过 90 秒未收到心跳。

你也能在面板里手动「添加中转」,然后启动 relay 时通过 `-controller-id` 指定对应 ID;不指定时 controller 会按上报的 `address` 自动合并。

### 客户端使用 relay

client 注册/心跳成功后,服务端会自动把当前在线的 relay 列表下发到 `relays` 字段。client 的线路优选器会:

1. **优先 P2P**:尝试直连/打洞
2. **P2P 失败自动切 relay**:如果握手不通或 RTT 过高,自动把 peer endpoint 改为在线 relay 地址
3. **恢复探测**:P2P 恢复后自动切回直连

手动临时指定也支持,把目标 peer 的 WireGuard `endpoint` 改成 relay 的公网地址和 UDP 端口:

```text
endpoint = 121.40.193.74:53478
```

### relay 本身安全

- relay 只转发 UDP 数据包,**不解密** WireGuard 内容,安全依赖 WireGuard 协议本身。
- 建议 `-http` 只监听 `127.0.0.1`,必要时用 SSH 隧道查看 `/clients`、`/stats`。
- 控制哪些设备必须走 relay,目前通过 controller 下发 relay 列表实现;client 是否启用 relay 由本地配置决定。

详细用法见 [`tools/relay/README.md`](tools/relay/README.md)。

## 路线图

- [x] 设备注册 / 虚拟 IP 分配 / Web 面板
- [x] WireGuard 隧道建立与 peer 自动同步
- [x] STUN 探测公网 endpoint(内置服务端,与面板同端口)
- [x] NAT 打洞(双 endpoint 上报 + 同 NAT 内网优选)
- [x] Windows 防火墙自动放行(隧道端口 + 隧道网卡入站)
- [x] 端到端延迟测试(client 经隧道 ping 各 peer,随心跳上报,面板展示 RTT)
- [x] 网络波动/公网 IP 变化自动恢复(心跳失败指数退避重连、401 自动重注册、本机/公网 endpoint 周期重探测、失联时 BindUpdate 重绑套接字)
- [x] 站点到站点:多网段路由互联(LAN 网段通告 + 冲突检测 + 自动路由 + 网关 IP 转发/SNAT)
- [x] 多虚拟网络(多租户):网络隔离 + 互联规则(整网/点对点)+ 按网络独立分配 IP
- [x] 拓扑图:可视化展示网络集群、设备、账号及互联关系
- [x] 独立 UDP 中继服务(`tools/relay`):对称 NAT / 无法 P2P 时的流量兜底
- [x] 自动线路优选:P2P 优先,打洞失败/质量差时自动切换到在线 relay
- [ ] 设备审批模式(面板手动放行)
- [ ] 二层模式(广播/局域网游戏发现)
