// Aleiyun 客户端 agent(SD-WAN)
//
// 流程:生成/加载密钥 -> 向 controller 注册 -> 拿到虚拟 IP 和 peer 列表
// -> 拉起 WireGuard 隧道(基于 wireguard-go) -> 周期心跳并同步 peer 变更。
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	heartbeatInterval   = 15 * time.Second
	stunProbeRounds     = 10               // 每 10 轮心跳(约 2.5 分钟)重探测一次公网 endpoint
	handshakeStaleAfter = 3 * time.Minute  // 超过此时长无握手视为隧道失联
	recoverCooldown     = 2 * time.Minute  // 隧道主动恢复动作的最小间隔,避免抖动
	backoffBase         = 5 * time.Second  // 心跳失败退避起始值
	backoffMax          = 60 * time.Second // 退避上限
)

// errNotRegistered 心跳返回 401/404:controller 丢了注册状态(如重启),需重新注册
var errNotRegistered = errors.New("需要重新注册")

// httpClient 带超时的心跳/注册客户端(网络中断时避免请求长时间挂死)
var httpClient = &http.Client{Timeout: 15 * time.Second}

type peerView struct {
	Name       string   `json:"name"`
	PubKey     string   `json:"pubkey"`
	VirtualIP  string   `json:"virtual_ip"`
	Endpoint   string   `json:"endpoint"`
	LANSubnets []string `json:"lan_subnets,omitempty"` // 该 peer 通告的 LAN 网段(站点到站点路由)
}

type registerResp struct {
	ID        string     `json:"id"`
	VirtualIP string     `json:"virtual_ip"`
	CIDR      string     `json:"cidr"`
	Network   string     `json:"network,omitempty"` // 服务端分配的所属网络名称(v0.7.0,账号绑定值)
	Peers     []peerView `json:"peers"`
	Routes    []string   `json:"routes,omitempty"` // 互联规则带来的额外路由(对端网络 CIDR / 对端设备 IP/32)
	Relays    []relayView `json:"relays,omitempty"` // 当前在线的独立 UDP 中转节点
	Revision  int64      `json:"revision,omitempty"` // 服务器配置版本号
}

type agent struct {
	controller     string
	name           string
	username       string // v0.7.0 账号体系:注册凭据
	password       string
	network        string // 所属网络名称(v0.7.0 起由账号绑定决定,注册响应回填;配置里的 network 已废弃)
	platform       string
	key            *KeyPair
	endpoint       string // 内网 endpoint
	publicEndpoint string // STUN 探测到的公网 endpoint
	listenPort     int
	ifName         string
	noTun          bool
	debug          bool

	tun     *Tunnel
	lastCfg string

	lanSubnets []string // 站点到站点:本机通告的 LAN 网段(归一化后的 CIDR)

	latencies map[string]float64 // 到各 peer 虚拟 IP 的端到端 RTT(毫秒),随心跳上报

	rev int64 // 服务器配置版本号,用于长轮询即时同步

	virtualIP string // 当前隧道网卡配置的虚拟 IP
	cidr      string // 当前隧道网卡配置的 CIDR

	keyPath string // 私钥文件路径(已解析为绝对路径)
	stunRaw string // -stun 原始值(空表示从 controller 地址推导)

	relays   []relayView     // 服务端下发的在线 relay 列表
	selector *pathSelector   // 线路优选器

	mu sync.RWMutex // 保护 name / network / rev

	gwCleanup    func()    // 网关模式清理函数(IP 转发/SNAT),退出时调用
	shutdownOnce sync.Once // 退出清理幂等保护

	stunServer   string    // 解析后的 STUN 服务器地址(空表示跳过公网探测)
	autoEndpoint bool      // endpoint 为自动探测所得(手动指定时不参与 IP 变化检测)
	round        int       // 心跳轮次计数
	retry        int       // 连续心跳失败次数(退避用)
	tunUpAt      time.Time // 隧道启动时间(握手失联判断的启动宽限)
	lastRecover  time.Time // 上次隧道主动恢复时间(冷却用)
}

func main() {
	hostname, _ := os.Hostname()

	a := &agent{}
	flag.StringVar(&a.controller, "controller", "http://127.0.0.1:52888", "controller 地址")
	flag.StringVar(&a.name, "name", hostname, "设备名称")
	flag.StringVar(&a.username, "username", "", "注册账号(在面板创建)")
	flag.StringVar(&a.password, "password", "", "注册账号密码")
	flag.StringVar(&a.network, "network", "", "已废弃(v0.7.0 起所属网络由账号绑定决定)")
	keyPath := flag.String("key", "aleiyun_client.key", "私钥文件路径(自动生成)")
	flag.StringVar(&a.ifName, "if", "aleiyun_sdwan0", "虚拟网卡名称")
	flag.IntVar(&a.listenPort, "listen", 51820, "WireGuard 监听端口")
	flag.StringVar(&a.endpoint, "endpoint", "", "手动指定对外 endpoint (IP:端口),留空自动探测")
	flag.BoolVar(&a.noTun, "no-tun", false, "只注册不起隧道(调试用,无需管理员权限)")
	flag.BoolVar(&a.debug, "debug", false, "输出 WireGuard 握手等详细日志")
	logPath := flag.String("log", "aleiyun_client.log", "日志文件路径(屏幕和文件同时输出)")
	stunServer := flag.String("stun", "", "STUN 服务器地址(默认用 controller 同地址;controller 在局域网时自动跳过)")
	lanRaw := flag.String("lan", "", "站点到站点:本机 LAN 网段(逗号分隔 CIDR),如 192.168.1.0/24,10.10.0.0/24")
	flag.Parse()

	// 配置文件:exe 同目录的 aleiyun_client.json,优先级 命令行 > 配置文件 > 默认值
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	exeDir := filepath.Dir(exe)
	cfgPath := filepath.Join(exeDir, "aleiyun_client.json")
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		fatal("读取配置文件 %s 失败: %v", cfgPath, err)
	}
	if cfg == nil {
		// 兼容提示:发现旧配置 agent.json 时提醒用户改名(不做自动迁移)
		oldCfgPath := filepath.Join(exeDir, "agent.json")
		if _, err := os.Stat(oldCfgPath); err == nil {
			log.Printf("检测到旧配置 agent.json,建议改名为 aleiyun_client.json")
		}
		if err := writeDefaultConfig(cfgPath, hostname); err != nil {
			fatal("生成默认配置文件失败: %v", err)
		}
		fmt.Printf("已生成默认配置文件 %s\n请编辑其中的 controller 字段后重新运行。\n", cfgPath)
		pauseAndExit(0)
	}
	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	if !explicit["controller"] {
		a.controller = cfg.Controller
	}
	if !explicit["name"] && cfg.Name != "" {
		a.name = cfg.Name
	}
	if !explicit["username"] {
		a.username = cfg.Username
	}
	if !explicit["password"] {
		a.password = cfg.Password
	}
	if !explicit["network"] {
		a.network = cfg.Network
	}
	if !explicit["key"] && cfg.Key != "" {
		*keyPath = cfg.Key
	}
	if !explicit["listen"] && cfg.ListenPort != 0 {
		a.listenPort = cfg.ListenPort
	}
	if !explicit["log"] && cfg.Log != "" {
		*logPath = cfg.Log
	}
	if !explicit["stun"] && cfg.Stun != "" {
		*stunServer = cfg.Stun
	}
	if !explicit["lan"] && cfg.LAN != "" {
		*lanRaw = cfg.LAN
	}
	if !explicit["no-tun"] {
		a.noTun = cfg.NoTun
	}
	if !explicit["debug"] {
		a.debug = cfg.Debug
	}
	// 补全地址:允许只填 IP/域名
	a.controller = normalizeController(a.controller)
	if a.controller == "" {
		fatal("controller 地址为空,请编辑 %s 填写 controller 后重跑(或用 -controller 指定)", cfgPath)
	}

	// 相对路径统一解析为相对于 exe 目录(双击运行时工作目录不可靠)
	*keyPath = resolvePath(exeDir, *keyPath)
	*logPath = resolvePath(exeDir, *logPath)

	// 日志双写:屏幕 + 文件
	setupLogFile(*logPath)

	log.Printf("Aleiyun client %s 启动", Version)

	// 站点到站点:解析校验 LAN 网段,非法项打日志并跳过
	a.lanSubnets = parseLANSubnets(*lanRaw)
	if len(a.lanSubnets) > 0 {
		log.Printf("LAN 网段(站点到站点): %s", strings.Join(a.lanSubnets, ", "))
	}

	// 起隧道需要管理员/root 权限,Windows 下未提权则自动 UAC 重启
	if !a.noTun {
		ensureAdmin()
	}

	a.keyPath = *keyPath
	a.stunRaw = *stunServer

	// 退出信号:收到 Ctrl+C/SIGTERM 时关闭 stop,心跳循环退出;
	// 第二次信号(清理卡顿时)强制立即退出
	stop := make(chan struct{})
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		close(stop)
		<-sig
		log.Printf("再次收到退出信号,强制退出")
		os.Exit(1)
	}()

	if err := runAgent(a, stop); err != nil {
		fatal("%v", err)
	}
}

// heartbeatEnsure 发一次心跳;失败时按指数退避重试(5s→10s→…→60s 封顶),
// 401/404 时先自动重新注册。返回 ok=false 表示重试期间收到退出信号。
func (a *agent) heartbeatEnsure(stop <-chan struct{}) (*registerResp, bool) {
	r, err := a.heartbeat()
	for err != nil {
		a.retry++
		if errors.Is(err, errNotRegistered) {
			log.Printf("controller 未识别本设备(可能已重启),用现有密钥重新注册")
			if rr, rerr := a.register(); rerr != nil {
				err = rerr // 重注册失败同样走退避
			} else {
				log.Printf("重新注册成功: 虚拟 IP = %s,当前 %d 个 peer", rr.VirtualIP, len(rr.Peers))
				a.setRegistration(rr.Network)
				if a.tun != nil {
					_ = a.setVirtualIP(rr.VirtualIP, rr.CIDR)
				}
				r = rr
				break
			}
		}
		delay := backoffDelay(a.retry)
		log.Printf("心跳失败(第 %d 次重试, %v 后重试): %v", a.retry, delay, err)
		if !sleepOrSignal(delay, stop) {
			return nil, false
		}
		r, err = a.heartbeat()
	}
	if a.retry > 0 {
		log.Printf("已恢复与 controller 的连接")
		a.retry = 0
	}
	if r != nil && r.Revision > 0 {
		a.setRev(r.Revision)
	}
	return r, true
}

// backoffDelay 计算第 n 次重试的退避时长:5s 起翻倍,60s 封顶
func backoffDelay(n int) time.Duration {
	d := backoffBase
	for i := 1; i < n && d < backoffMax; i++ {
		d *= 2
	}
	if d > backoffMax {
		d = backoffMax
	}
	return d
}

// sleepOrSignal 睡眠 d;期间收到退出信号则立即返回 false
func sleepOrSignal(d time.Duration, stop <-chan struct{}) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-stop:
		return false
	case <-t.C:
		return true
	}
}

var waitClient = &http.Client{Timeout: 30 * time.Second}

// setRev 安全更新服务器配置版本号
func (a *agent) setRev(rev int64) {
	if rev <= 0 {
		return
	}
	a.mu.Lock()
	a.rev = rev
	a.mu.Unlock()
}

// forceSync 立即向 controller 同步一次配置(不干扰主心跳的退避计数器)
func (a *agent) forceSync(stop <-chan struct{}) {
	select {
	case <-stop:
		return
	default:
	}
	r, err := a.heartbeat()
	if errors.Is(err, errNotRegistered) {
		log.Printf("强制同步:controller 未识别本设备,尝试重新注册")
		rr, rerr := a.register()
		if rerr != nil {
			log.Printf("强制同步:重新注册失败: %v", rerr)
			return
		}
		a.setRegistration(rr.Network)
		r = rr
	} else if err != nil {
		log.Printf("强制同步:心跳失败: %v", err)
		return
	}
	if r != nil {
		if r.Revision > 0 {
			a.setRev(r.Revision)
		}
		if a.tun != nil {
			if err := a.setVirtualIP(r.VirtualIP, r.CIDR); err != nil {
				log.Printf("强制同步:虚拟 IP 更新失败: %v", err)
			}
			if _, err := a.syncPeers(r.Peers, r.Routes); err != nil {
				log.Printf("强制同步:peer 同步失败: %v", err)
			}
		}
	}
}

// waitForChanges 长轮询 /api/wait,在服务器配置变更时触发即时同步
func (a *agent) waitForChanges(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
		}
		a.mu.RLock()
		controller, rev := a.controller, a.rev
		a.mu.RUnlock()
		resp, err := waitClient.Get(fmt.Sprintf("%s/api/wait?rev=%d", controller, rev))
		if err != nil {
			// 30s 超时是正常情况,继续下一轮长轮询
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			continue
		}
		var rv struct {
			Revision int64 `json:"revision"`
		}
		if err := json.Unmarshal(body, &rv); err != nil {
			continue
		}
		if rv.Revision > rev {
			a.setRev(rv.Revision)
			log.Printf("收到配置变更通知,立即同步")
			a.forceSync(stop)
		}
	}
}

// checkLocalIPChange 检测本机出口 IP 变化(向 controller 地址发起 UDP 连接取 LocalAddr,不实际发包);
// 变化时更新内网 endpoint 并让隧道重新绑定 UDP 套接字。手动指定的 endpoint 不自动改。
func (a *agent) checkLocalIPChange() {
	if !a.autoEndpoint {
		return
	}
	u, err := url.Parse(a.controller)
	if err != nil || u.Host == "" {
		return
	}
	conn, err := net.Dial("udp", u.Host)
	if err != nil {
		return
	}
	defer conn.Close()
	ip := conn.LocalAddr().(*net.UDPAddr).IP.String()

	oldHost, port, err := net.SplitHostPort(a.endpoint)
	if err != nil || ip == oldHost {
		return
	}
	a.endpoint = net.JoinHostPort(ip, port)
	log.Printf("本机出口 IP 变化: %s -> %s,内网 endpoint 更新为 %s", oldHost, ip, a.endpoint)
	if a.tun != nil {
		if err := a.tun.Rebind(); err != nil {
			log.Printf("隧道 Rebind 失败: %v", err)
		}
	}
}

// checkPublicEndpoint 重新 STUN 探测公网 endpoint,有变化时更新并打日志(随下次心跳上报)
func (a *agent) checkPublicEndpoint() {
	if a.stunServer == "" {
		return
	}
	pub, err := stunProbe(a.stunServer, a.listenPort)
	if err != nil {
		log.Printf("STUN 重探测失败: %v", err)
		return
	}
	if pub != a.publicEndpoint {
		log.Printf("公网 endpoint 变化: %s -> %s", a.publicEndpoint, pub)
		a.publicEndpoint = pub
	}
}

// recoverTunnel 隧道失联主动恢复:立即重探测公网 endpoint、重绑 UDP 套接字、强制刷新 peer 配置。
// 有冷却期(recoverCooldown),避免网络抖动时反复触发。
func (a *agent) recoverTunnel(peers []peerView, routes []string) {
	if time.Since(a.lastRecover) < recoverCooldown {
		return
	}
	a.lastRecover = time.Now()
	log.Printf("所有 peer 超过 %v 无握手,尝试恢复隧道连接", handshakeStaleAfter)
	a.checkPublicEndpoint()
	if err := a.tun.Rebind(); err != nil {
		log.Printf("隧道 Rebind 失败: %v", err)
	}
	a.lastCfg = "" // 清空缓存,强制全量下发
	if _, err := a.syncPeers(peers, routes); err != nil {
		log.Printf("peer 强制同步失败: %v", err)
	}
}

func (a *agent) post(path string, body any) (*registerResp, error) {
	payload, _ := json.Marshal(body)
	resp, err := httpClient.Post(a.controller+path, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("controller 返回 %s: %w", resp.Status, errNotRegistered)
	}
	if resp.StatusCode == http.StatusForbidden {
		// 403:多为指定的网络不存在,带上 controller 返回的具体原因
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("controller 拒绝访问: %s", strings.TrimSpace(string(msg)))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("controller 返回 %s", resp.Status)
	}
	var r registerResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// parseLANSubnets 解析逗号分隔的 CIDR 列表,非法项打日志并跳过,合法项归一化
func parseLANSubnets(raw string) []string {
	var out []string
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(s)
		if err != nil {
			log.Printf("LAN 网段 %q 格式非法,已跳过", s)
			continue
		}
		out = append(out, ipnet.String())
	}
	return out
}

func (a *agent) register() (*registerResp, error) {
	a.mu.RLock()
	username, password, network := a.username, a.password, a.network
	a.mu.RUnlock()
	return a.post("/api/register", map[string]any{
		"name":            a.getName(),
		"username":        username,
		"password":        password,
		"network":         network, // 已废弃:服务端按账号绑定决定所属网络,此字段被忽略
		"pubkey":          a.key.Public,
		"endpoint":        a.endpoint,
		"public_endpoint": a.publicEndpoint,
		"platform":        a.platform,
		"lan_subnets":     a.lanSubnets, // 始终上报(空列表表示不通告任何 LAN 网段)
		"macs":            macAddrs(),
		"machine_id":      machineID(macAddrs()),
	})
}

func (a *agent) heartbeat() (*registerResp, error) {
	a.mu.RLock()
	network := a.network
	a.mu.RUnlock()
	body := map[string]any{
		"name":            a.getName(),
		"pubkey":          a.key.Public,
		"network":         network, // 已废弃:账号绑定设备由服务端固定网络,此字段被忽略
		"endpoint":        a.endpoint,
		"public_endpoint": a.publicEndpoint,
		"lan_subnets":     a.lanSubnets,
		"macs":            macAddrs(),
		"machine_id":      machineID(macAddrs()),
	}
	if len(a.latencies) > 0 {
		body["latencies"] = a.latencies
	}
	r, err := a.post("/api/heartbeat", body)
	if err == nil && r != nil && r.Revision > 0 {
		a.setRev(r.Revision)
	}
	return r, err
}

// registerToRelays 向所有在线 relay 注册并维持心跳。
// 携带本机公网 endpoint（若已知），让 relay 记录 WireGuard 流量应转发到的目标地址。
func (a *agent) registerToRelays(relays []relayView) {
	if len(relays) == 0 || a.key == nil || a.key.Public == "" {
		return
	}
	pub := a.key.Public
	registerBody := "REGISTER:" + pub
	heartbeatBody := "HEARTBEAT:" + pub
	if a.publicEndpoint != "" {
		registerBody += ":" + a.publicEndpoint
		heartbeatBody += ":" + a.publicEndpoint
	}
	msgReg := []byte(registerBody)
	msgHb := []byte(heartbeatBody)

	for _, r := range relays {
		addr := strings.TrimSpace(r.Address)
		if addr == "" {
			continue
		}
		udpAddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			continue
		}
		// 优先尝试从 WireGuard 监听端口发送，复用 NAT 映射；失败则回退随机端口
		var conn *net.UDPConn
		if lconn, err := net.ListenUDP("udp", &net.UDPAddr{Port: a.listenPort}); err == nil {
			conn = lconn
		} else if lconn, err := net.ListenUDP("udp", nil); err == nil {
			conn = lconn
		} else {
			continue
		}
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, _ = conn.WriteToUDP(msgReg, udpAddr)
		// 稍微错开注册与心跳，降低丢包影响
		time.Sleep(5 * time.Millisecond)
		_, _ = conn.WriteToUDP(msgHb, udpAddr)
		conn.Close()
	}
}

// goOffline 正常退出前通知 controller 主动下线(3 秒超时),
// 失败只记 debug 日志不阻塞退出,离线状态由 controller 的超时窗口兜底。
func (a *agent) goOffline() {
	payload, _ := json.Marshal(map[string]string{"pubkey": a.key.Public})
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(a.controller+"/api/offline", "application/json", bytes.NewReader(payload))
	if err != nil {
		if a.debug {
			log.Printf("主动下线通知发送失败(忽略): %v", err)
		}
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK && a.debug {
		log.Printf("主动下线通知返回 %s(忽略)", resp.Status)
	}
}

// measureLatencies 逐个 ping 在线 peer 的虚拟 IP(每个约 3s 预算),
// 返回 虚拟IP -> RTT(毫秒) 表;ping 失败的 peer 不放入结果。
// stop 关闭时立即中断返回已完成部分(避免退出时被 ping 卡住)。
func measureLatencies(peers []peerView, stop <-chan struct{}) map[string]float64 {
	lat := map[string]float64{}
	for _, p := range peers {
		select {
		case <-stop:
			return lat
		default:
		}
		if p.VirtualIP == "" {
			continue
		}
		if ms, ok := PingRTT(p.VirtualIP, 3, time.Second); ok {
			lat[p.VirtualIP] = ms
		}
	}
	return lat
}

func (a *agent) startTunnel(resp *registerResp) error {
	tun, err := startTunnel(a.ifName, a.debug)
	if err != nil {
		return err
	}
	a.tun = tun
	log.Printf("虚拟网卡 %s 已创建 (MTU %d)", tun.Name(), 1420)

	// 首次创建隧道时必须显式配置网卡地址;setVirtualIP 在此时会因为
	// 注册阶段已经记录过相同 IP/CIDR 而跳过,所以直接调用 assignInterfaceIP
	if err := assignInterfaceIP(tun.Name(), resp.VirtualIP, resp.CIDR); err != nil {
		return fmt.Errorf("配置网卡 IP 失败: %w", err)
	}
	log.Printf("网卡地址已配置: %s/%s", resp.VirtualIP, resp.CIDR)

	// 放行 WireGuard 入站 UDP(Windows 防火墙默认拦截入站,会导致握手失败)
	if err := allowInboundUDP(a.listenPort); err != nil {
		log.Printf("防火墙规则添加失败(如握手不通请手动放行 UDP %d): %v", a.listenPort, err)
	}
	// 放行隧道网卡的全部入站流量(虚拟局域网内视为可信)
	if err := allowTunnelInbound(tun.Name()); err != nil {
		log.Printf("隧道网卡防火墙放行失败(如业务流量不通请检查防火墙): %v", err)
	}

	// 初始同步 peer，让线路选择器在隧道启动后立即生效
	if _, err := a.syncPeers(resp.Peers, resp.Routes); err != nil {
		return err
	}
	a.tunUpAt = time.Now()
	a.logPeerStats(resp.Peers)
	return nil
}

// logPeerStats 播报每个 peer 的隧道握手状态;
// 返回 true 表示存在应有 peer 且所有 peer 均超过 handshakeStaleAfter 无握手(隧道疑似失联)
func (a *agent) logPeerStats(peers []peerView) bool {
	if a.tun == nil || len(peers) == 0 {
		return false
	}
	stats, err := a.tun.PeerStats()
	if err != nil {
		return false
	}
	checked, stale := 0, 0
	for _, p := range peers {
		hexPub, err := b64ToHex(p.PubKey)
		if err != nil {
			continue
		}
		checked++
		hs := stats[hexPub]
		if hs == 0 {
			stale++
			log.Printf("隧道状态: %s (%s) 从未握手", p.Name, p.VirtualIP)
		} else {
			ago := time.Since(time.Unix(hs, 0)).Round(time.Second)
			log.Printf("隧道状态: %s (%s) 最近握手 %v 前", p.Name, p.VirtualIP, ago)
			if ago > handshakeStaleAfter {
				stale++
			}
		}
	}
	return checked > 0 && stale == checked
}

// syncPeers 全量刷新 WireGuard peer 配置(配置有变化才下发);
// routes 为互联规则带来的额外路由,与 peer 通告的 LAN 网段一起同步到本机路由表
// 返回实际为每个 peer 选中的 endpoint map(pubkey->endpoint)。
func (a *agent) syncPeers(peers []peerView, routes []string) (map[string]string, error) {
	// 线路优选：根据握手状态与历史 RTT 为每个 peer 挑选当前最佳 endpoint
	chosen := map[string]string{}
	if a.selector != nil {
		stats, _ := a.tun.PeerStats()
		chosen = a.selector.Pick(stats, time.Now())
	}

	// 复制 peers 避免修改原始切片，并按 chosen 替换 endpoint
	applied := make([]peerView, len(peers))
	copy(applied, peers)
	endpointChanged := false
	for i := range applied {
		orig := applied[i].Endpoint
		if c, ok := chosen[applied[i].PubKey]; ok && c != "" {
			applied[i].Endpoint = c
		} else if applied[i].Endpoint == "" {
			// selector 未决策且服务端未提供，尝试用第一个可用 relay 兜底
			for _, r := range a.relays {
				if endpointReachable(r.Address) {
					applied[i].Endpoint = r.Address
					break
				}
			}
		}
		if applied[i].Endpoint != orig {
			endpointChanged = true
		}
	}

	cfg, err := buildUAPIConfig(a.key.Private, a.listenPort, applied)
	if err != nil {
		return chosen, err
	}
	if cfg == a.lastCfg && !endpointChanged {
		// 配置未变也要同步路由(Routes 可能单独变化)
		syncRoutes(a.tun.Name(), applied, routes)
		return chosen, nil
	}
	if err := a.tun.Configure(cfg); err != nil {
		return chosen, err
	}
	a.lastCfg = cfg
	log.Printf("peer 配置已更新: %d 个节点", len(applied))
	for _, p := range applied {
		log.Printf("  - %s (%s) via %s", p.Name, p.VirtualIP, p.Endpoint)
	}
	// 站点到站点 + 互联规则:按 peer LAN 网段和 resp.Routes 同步本机路由,失败只记日志不影响隧道
	syncRoutes(a.tun.Name(), applied, routes)
	return chosen, nil
}

// detectLocalIP 获取本机出口 IP(向公网地址发起 UDP 连接,不实际发包)
func detectLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

// runAgent 启动 agent 并进入心跳循环,直到 stop 关闭;启动失败返回 error
func runAgent(a *agent, stop <-chan struct{}) error {
	if err := a.start(); err != nil {
		return err
	}
	// 长轮询:服务器拓扑/规则变更时触发即时同步,与周期心跳互补
	go a.waitForChanges(stop)
	a.heartbeatLoop(stop)
	return nil
}

// start 启动序列:平台标识 -> 密钥 -> endpoint 探测 -> 注册 -> 隧道。
// 失败时返回 error 并清理已创建的隧道。
func (a *agent) start() (err error) {
	defer func() {
		if err != nil && a.tun != nil {
			a.tun.Close()
			a.tun = nil
		}
	}()

	a.platform = runtime.GOOS + "/" + runtime.GOARCH

	// 1. 密钥
	a.key, err = loadOrCreateKeyPair(a.keyPath)
	if err != nil {
		return fmt.Errorf("密钥加载失败: %w", err)
	}
	log.Printf("公钥: %s", a.key.Public)

	// 2. 确定对外 endpoint(内网 + STUN 公网映射)
	if a.endpoint == "" {
		a.endpoint = detectLocalIP() + fmt.Sprintf(":%d", a.listenPort)
		a.autoEndpoint = true
		log.Printf("内网 endpoint: %s", a.endpoint)
	}
	stun := a.stunRaw
	if stun == "" {
		if u, err := url.Parse(a.controller); err == nil && u.Host != "" {
			stun = u.Host
		}
	}
	if stun == "" {
		log.Printf("无法从 controller 地址推导 STUN 服务器,跳过公网探测")
	} else if a.stunRaw == "" && isPrivateHost(stun) {
		log.Printf("controller 在局域网内,跳过 STUN 公网探测")
	} else {
		a.stunServer = stun
		if pub, err := stunProbe(stun, a.listenPort); err != nil {
			log.Printf("STUN 探测失败(跨公网将不可用): %v", err)
		} else {
			a.publicEndpoint = pub
			log.Printf("公网 endpoint (STUN): %s", pub)
		}
	}

	// 3. 注册
	resp, err := a.register()
	if err != nil {
		return fmt.Errorf("注册失败: %w", err)
	}
	a.setRegistration(resp.Network)
	a.setVirtualIP(resp.VirtualIP, resp.CIDR)
	if resp.Revision > 0 {
		a.setRev(resp.Revision)
	}
	log.Printf("注册成功: 虚拟 IP = %s,当前 %d 个 peer", resp.VirtualIP, len(resp.Peers))

	// 保存 relay 列表并初始化线路选择器
	a.relays = resp.Relays
	a.selector = newPathSelector()

	// 4. 起隧道
	if !a.noTun {
		if err := ensureWintun(); err != nil {
			return err
		}
		if err := a.startTunnel(resp); err != nil {
			return fmt.Errorf("隧道启动失败(需要管理员/root 权限): %w", err)
		}
		// 站点到站点:配了 lan 的节点作为网关,开启 IP 转发 + SNAT
		if len(a.lanSubnets) > 0 {
			a.gwCleanup = setupGateway(a.tun.Name(), a.lanSubnets)
		}
	} else {
		log.Printf("已跳过隧道建立 (-no-tun)")
	}
	return nil
}

// heartbeatLoop 周期心跳 + peer 同步,直到 stop 关闭;返回前执行退出清理
func (a *agent) heartbeatLoop(stop <-chan struct{}) {
	defer a.shutdown()
	tick := time.NewTicker(heartbeatInterval)
	defer tick.Stop()

	log.Printf("agent 运行中,每 15s 心跳并同步 peer 列表,Ctrl+C 退出")
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			a.round++
			a.checkLocalIPChange()
			if a.round%stunProbeRounds == 0 {
				a.checkPublicEndpoint()
			}
			r, ok := a.heartbeatEnsure(stop)
			if !ok {
				return
			}
			// 服务端可能在心跳响应里改了网络/IP(账号换绑),同步本地状态
			a.setRegistration(r.Network)
			if a.tun != nil {
				// 同步最新 relay 列表并维持 NAT 映射
				a.relays = r.Relays
				if a.selector != nil {
					a.selector.Update(r.Peers, a.relays)
				}
				a.registerToRelays(a.relays)

				// 虚拟 IP/CIDR 变化时同步更新本地网卡地址
				if err := a.setVirtualIP(r.VirtualIP, r.CIDR); err != nil {
					log.Printf("虚拟 IP 更新失败: %v", err)
				}
				chosen, err := a.syncPeers(r.Peers, r.Routes)
				if err != nil {
					log.Printf("peer 同步失败: %v", err)
				}
				// 启动宽限期内不判断失联(peer 可能尚未完成首次握手)
				if a.logPeerStats(r.Peers) && time.Since(a.tunUpAt) >= handshakeStaleAfter {
					a.recoverTunnel(r.Peers, r.Routes)
				}
				// 端到端延迟测试:串行 ping 每个 peer,失败的 peer 不放入结果,
				// 不阻塞本轮已完成的心跳上报,结果随下次心跳发出;收到退出信号立即中断
				lat := measureLatencies(r.Peers, stop)
				a.latencies = lat
				// 把 RTT 记录到线路选择器,用于下次优选
				if a.selector != nil {
					now := time.Now()
					for _, p := range r.Peers {
						endpoint := chosen[p.PubKey]
						if endpoint == "" {
							continue
						}
						if ms, ok := lat[p.VirtualIP]; ok {
							a.selector.RecordRTT(p.PubKey, endpoint, ms, true, now)
						} else {
							a.selector.RecordRTT(p.PubKey, endpoint, 0, false, now)
						}
					}
				}
			}
		}
	}
}

// shutdown 退出清理(幂等):主动下线 -> 网关清理(SNAT) -> 路由清理 -> 关闭隧道。
// 总时长超过 8s(如 wintun 关闭阻塞)放弃剩余清理直接返回,进程随即退出。
func (a *agent) shutdown() {
	a.shutdownOnce.Do(func() {
		done := make(chan struct{})
		go func() {
			defer close(done)
			a.goOffline()
			if a.gwCleanup != nil {
				a.gwCleanup()
			}
			if a.tun != nil {
				cleanupRoutes(a.tun.Name())
			}
			log.Printf("退出")
			if a.tun != nil {
				a.tun.Close()
			}
		}()
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			log.Printf("退出清理超时(8s),放弃剩余清理直接退出")
		}
	})
}

// setRegistration 记录注册结果:服务端分配的所属网络名称
func (a *agent) setRegistration(network string) {
	a.mu.Lock()
	if network != "" {
		a.network = network
	}
	a.mu.Unlock()
}

// setVirtualIP 更新本地记录的虚拟 IP/CIDR;若与当前不同且隧道已建立,则重新配置网卡地址
func (a *agent) setVirtualIP(ip, cidr string) error {
	a.mu.Lock()
	if ip == "" {
		a.mu.Unlock()
		return nil
	}
	changed := a.virtualIP != ip || a.cidr != cidr
	if changed {
		a.virtualIP = ip
		a.cidr = cidr
	}
	tun := a.tun
	a.mu.Unlock()

	if !changed || tun == nil {
		return nil
	}
	log.Printf("虚拟 IP 变化,更新网卡地址: %s/%s", ip, cidr)
	if err := assignInterfaceIP(tun.Name(), ip, cidr); err != nil {
		return err
	}
	log.Printf("网卡地址已更新: %s", ip)
	return nil
}

// getName 设备名称读取(心跳 goroutine 读,统一走 mu)
func (a *agent) getName() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.name
}
