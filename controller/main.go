// Aleiyun 总控面板 (controller,SD-WAN server)
//
// 职责:设备注册、虚拟 IP 分配、peer 拓扑下发、在线状态跟踪、Web 面板。
// 数据持久化到 data.json,零依赖。可用 -data 指定其他路径。
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed all:static
var staticFS embed.FS

const (
	defaultNetworkID   = "default"      // 默认网络固定 ID
	defaultNetworkName = "默认网络"         // 默认网络名称
	defaultNetworkCIDR = "10.66.0.0/24" // 默认网络网段(兼容旧版扁平网络)
	onlineWindow       = 45 * time.Second
)

// Network 虚拟网络(租户):设备按网络隔离,同网络设备自动互联
type Network struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	CIDR string `json:"cidr"`
}

// Link 互联规则:打通两个网络(整网互通)或两台设备(点对点)
// Type 为 "network" 时 A/B 是网络 ID,为 "device" 时是设备 ID;A/B 无序(双向)
type Link struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	A    string `json:"a"`
	B    string `json:"b"`
}

// Account 客户端账号(v0.7.0):绑定公司(网络),客户端凭账密注册,
// 所属网络由账号决定;账号只能从面板创建(关闭公开注册)
type Account struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"` // sha256(salt+password) hex
	Salt         string    `json:"salt"`
	NetworkID    string    `json:"network_id"`
	MaxDevices   int       `json:"max_devices"` // 允许绑定的设备数上限,<=0 取默认值 defaultMaxDevices
	CreatedAt    time.Time `json:"created_at"`
}

// defaultMaxDevices 账号默认设备数上限(一个账号默认只允许绑定一台设备)
const defaultMaxDevices = 1

// deviceLimit 账号的有效设备数上限(兼容旧数据:未设置时取默认值)
func (a *Account) deviceLimit() int {
	if a.MaxDevices > 0 {
		return a.MaxDevices
	}
	return defaultMaxDevices
}

// RelayNode 独立 UDP 中转节点，部署在带公网 IP 的服务器上
type RelayNode struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`    // 客户端可访问的公网地址，如 relay.example.com:3478
	PubKey    string    `json:"pubkey"`     // relay 的 WireGuard 公钥（可选，用于后续认证）
	Note      string    `json:"note"`       // 备注
	LastSeen  time.Time `json:"last_seen"`  // 最后一次心跳时间
	Online    bool      `json:"online"`     // 是否在线（由心跳状态推导）
	CreatedAt time.Time `json:"created_at"`
}

// relayOnlineWindow 判定 relay 在线的时间窗口
const relayOnlineWindow = 90 * time.Second

type Device struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	PubKey         string    `json:"pubkey"`
	VirtualIP      string    `json:"virtual_ip"`
	Endpoint       string    `json:"endpoint"`        // 内网 IP:端口
	PublicEndpoint string    `json:"public_endpoint"` // STUN 探测到的公网映射 IP:端口
	Platform       string    `json:"platform"`
	Network        string    `json:"network"`  // 所属网络 ID(空视为默认网络)
	Username       string    `json:"username"` // 注册所用账号(v0.7.0,空为旧版公开注册设备)
	CreatedAt      time.Time `json:"created_at"`
	LastSeen       time.Time `json:"last_seen"`
	Offline        bool      `json:"offline,omitempty"` // agent 主动下线标记,心跳恢复时清除

	// 机器指纹:设备上报的 MAC 地址列表,用于封禁机器(换密钥也能识别)
	MACs []string `json:"macs,omitempty"`
	// MachineID 是后台根据 MAC 生成的短机器码,便于面板展示
	MachineID string `json:"machine_code,omitempty"`
	// Disabled=true 表示该设备被后台禁用,注册/心跳直接拒绝(无法换 key 绕过)
	Disabled bool `json:"disabled,omitempty"`

	// 站点到站点:设备通告的 LAN 网段;冲突/非法的被剔除后记入 LANConflicts
	LANSubnets   []string `json:"lan_subnets,omitempty"`
	LANConflicts []string `json:"lan_conflicts,omitempty"`

	// 端到端延迟:该设备到各 peer 虚拟 IP 的 RTT(毫秒),由 agent 心跳上报
	Latencies        map[string]float64 `json:"latencies,omitempty"`
	LatencyUpdatedAt time.Time          `json:"latency_updated_at,omitempty"`
}

// registerFailures 客户端注册失败速率限制(防爆破)
type registerFailures struct {
	mu    sync.Mutex
	slots map[string][]time.Time // key: IP
}

// allowed 检查该 IP 是否允许继续尝试注册
func (rf *registerFailures) allowed(ip string) bool {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-5 * time.Minute)
	var valid []time.Time
	for _, t := range rf.slots[ip] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	rf.slots[ip] = valid
	return len(valid) < 10
}

// record 记录一次失败注册
func (rf *registerFailures) record(ip string) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.slots[ip] = append(rf.slots[ip], time.Now())
}

type Store struct {
	mu                sync.Mutex
	cond              *sync.Cond
	rev               int64                // 配置版本号,任何拓扑变更后递增
	Devices           map[string]*Device   `json:"devices"`  // key: pubkey
	Networks          map[string]*Network  `json:"networks"` // key: 网络 ID
	Links             map[string]*Link     `json:"links"`    // key: 规则 ID
	Accounts          map[string]*Account  `json:"accounts"` // key: 账号 ID(v0.7.0)
	Relays            map[string]*RelayNode `json:"relays"`  // key: relay ID
	path              string
	registerFailLimit *registerFailures    `json:"-"`
}

func NewStore(path string) *Store {
	s := &Store{
		Devices:           map[string]*Device{},
		Networks:          map[string]*Network{},
		Links:             map[string]*Link{},
		Accounts:          map[string]*Account{},
		Relays:            map[string]*RelayNode{},
		path:              path,
		registerFailLimit: &registerFailures{slots: map[string][]time.Time{}},
	}
	s.cond = sync.NewCond(&s.mu)
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, s)
	}
	// 兼容旧数据:旧版 data.json 没有 networks/links/accounts 字段
	if s.Networks == nil {
		s.Networks = map[string]*Network{}
	}
	if s.Links == nil {
		s.Links = map[string]*Link{}
	}
	if s.Accounts == nil {
		s.Accounts = map[string]*Account{}
	}
	if s.Relays == nil {
		s.Relays = map[string]*RelayNode{}
	}
	// 没有任何网络时自动创建默认网络
	if len(s.Networks) == 0 {
		s.Networks[defaultNetworkID] = &Network{ID: defaultNetworkID, Name: defaultNetworkName, CIDR: defaultNetworkCIDR}
	}
	// 存量设备未归属网络,全部归入默认网络
	for _, d := range s.Devices {
		if d.Network == "" {
			d.Network = defaultNetworkID
		}
	}
	return s
}

func (s *Store) saveLocked() {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.path, data, 0o600)
	// 拓扑配置变更后递增版本号,通知所有长轮询客户端立即重新同步
	s.rev++
	if s.cond != nil {
		s.cond.Broadcast()
	}
}

// nextIPInNetworkLocked 在指定网络的 CIDR 内分配第一个空闲虚拟 IP
// (只统计同网络设备的占用,不同网络地址池相互独立)
func (s *Store) nextIPInNetworkLocked(netID, cidr string) (string, error) {
	used := map[string]bool{}
	for _, d := range s.Devices {
		if d.Network == netID {
			used[d.VirtualIP] = true
		}
	}
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("网络 CIDR 非法: %s", cidr)
	}
	ip := ipnet.IP.Mask(ipnet.Mask).To4()
	if ip == nil {
		return "", fmt.Errorf("暂只支持 IPv4 网络: %s", cidr)
	}
	// 广播地址 = 网络地址 | 掩码取反
	broadcast := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		broadcast[i] = ip[i] | ^ipnet.Mask[i]
	}
	// 从 .2 开始分配(.0 网络地址、.1 惯例留作网关),到广播地址前一位结束
	incIP(ip)
	incIP(ip)
	for ; !ip.Equal(broadcast); incIP(ip) {
		cand := ip.String()
		if !used[cand] {
			return cand, nil
		}
	}
	return "", fmt.Errorf("地址池已耗尽")
}

// incIP 原地递增 IPv4 地址
func incIP(ip net.IP) {
	for i := 3; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

// resolveNetworkLocked 按名称解析网络(空名称 => 默认网络),不存在返回 nil
func (s *Store) resolveNetworkLocked(name string) *Network {
	name = strings.TrimSpace(name)
	if name == "" {
		return s.Networks[defaultNetworkID]
	}
	for _, n := range s.Networks {
		if n.Name == name {
			return n
		}
	}
	return nil
}

// refreshRelayStatusLocked 根据 LastSeen 刷新所有 relay 的 Online 标记
func (s *Store) refreshRelayStatusLocked() {
	now := time.Now()
	for _, r := range s.Relays {
		r.Online = !r.LastSeen.IsZero() && now.Sub(r.LastSeen) <= relayOnlineWindow
	}
}

// onlineRelaysLocked 返回当前在线的 relay 视图列表
func (s *Store) onlineRelaysLocked() []relayView {
	s.refreshRelayStatusLocked()
	list := []relayView{}
	for _, r := range s.Relays {
		if !r.Online {
			continue
		}
		list = append(list, relayView{ID: r.ID, Address: r.Address, PubKey: r.PubKey, Note: r.Note})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Address < list[j].Address })
	return list
}

// deviceByIDLocked 按设备 ID 查找设备
func (s *Store) deviceByIDLocked(id string) *Device {
	for _, d := range s.Devices {
		if d.ID == id {
			return d
		}
	}
	return nil
}

// switchNetworkLocked 把设备换到指定网络,并按新网络 CIDR 重分配虚拟 IP(释放旧 IP)
func (s *Store) switchNetworkLocked(d *Device, nw *Network) error {
	if d.Network == nw.ID {
		return nil
	}
	ip, err := s.nextIPInNetworkLocked(nw.ID, nw.CIDR)
	if err != nil {
		return err
	}
	log.Printf("[network] 设备 %s 换网络 %s -> %s,虚拟 IP %s -> %s",
		d.Name, d.Network, nw.ID, d.VirtualIP, ip)
	d.Network = nw.ID
	d.VirtualIP = ip
	return nil
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// hashPassword 账号密码哈希:sha256(salt+password) hex
func hashPassword(salt, password string) string {
	sum := sha256.Sum256([]byte(salt + password))
	return hex.EncodeToString(sum[:])
}

// passwordAlphabet 随机密码字符集(大小写字母+数字)
const passwordAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// randomPassword 生成 10 位随机明文密码(创建/重置账号时用)
func randomPassword() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = passwordAlphabet[int(b[i])%len(passwordAlphabet)]
	}
	return string(b)
}

// authenticateLocked 校验账号密码,通过返回账号(其绑定的网络决定设备所属),否则 nil
func (s *Store) authenticateLocked(username, password string) *Account {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil
	}
	for _, a := range s.Accounts {
		if a.Username == username && a.PasswordHash == hashPassword(a.Salt, password) {
			return a
		}
	}
	return nil
}

// accountByIDLocked 按 ID 查找账号
func (s *Store) accountByIDLocked(id string) *Account {
	return s.Accounts[id]
}

// ---------- HTTP API ----------

type registerReq struct {
	Name           string `json:"name"`
	PubKey         string `json:"pubkey"`
	Endpoint       string `json:"endpoint"`        // 内网 endpoint
	PublicEndpoint string `json:"public_endpoint"` // STUN 探测到的公网 endpoint
	Platform       string `json:"platform"`
	Network        string `json:"network"` // v0.7.0 起废弃:所属网络由账号绑定决定,此字段仅旧版兼容
	Username       string `json:"username"`
	Password       string `json:"password"`

	// 心跳可选附带:到各 peer 虚拟 IP 的端到端 RTT(毫秒)
	Latencies map[string]float64 `json:"latencies,omitempty"`

	// 站点到站点:设备通告的 LAN 网段(CIDR 列表);nil 表示未上报(老版本 agent)
	LANSubnets []string `json:"lan_subnets,omitempty"`

	// 机器指纹:设备上报的 MAC 地址列表(后台据此封禁机器)
	MACs []string `json:"macs,omitempty"`
	// MachineID 是 agent 根据物理 MAC 计算的短机器码
	MachineID string `json:"machine_id,omitempty"`
}

type peerView struct {
	Name       string   `json:"name"`
	PubKey     string   `json:"pubkey"`
	VirtualIP  string   `json:"virtual_ip"`
	Endpoint   string   `json:"endpoint"`
	LANSubnets []string `json:"lan_subnets,omitempty"` // 该 peer 通告的 LAN 网段(站点到站点路由)
}

type registerResp struct {
	ID        string       `json:"id"`
	VirtualIP string       `json:"virtual_ip"`
	CIDR      string       `json:"cidr"`
	Network   string       `json:"network,omitempty"` // 所属网络名称(v0.7.0,账号绑定值)
	Peers     []peerView   `json:"peers"`
	Routes    []string     `json:"routes,omitempty"` // 互联规则带来的额外路由(对端网络 CIDR / 对端设备 IP/32)
	Relays    []relayView  `json:"relays,omitempty"` // 当前在线的独立 UDP 中转节点列表
	Revision  int64        `json:"revision,omitempty"` // 服务器配置版本号,用于客户端长轮询即时同步
}

// relayView 客户端/面板可见的 relay 信息(不含内部字段)
type relayView struct {
	ID      string `json:"id"`
	Address string `json:"address"`
	PubKey  string `json:"pubkey,omitempty"`
	Note    string `json:"note,omitempty"`
}

func (s *Store) peersForLocked(self *Device) []peerView {
	// 收集与本设备互联的对象:device link 的对端设备、network link 的对端网络
	linkedDevIDs := map[string]bool{}
	linkedNetIDs := map[string]bool{}
	for _, l := range s.Links {
		switch l.Type {
		case "device":
			if l.A == self.ID {
				linkedDevIDs[l.B] = true
			} else if l.B == self.ID {
				linkedDevIDs[l.A] = true
			}
		case "network":
			if l.A == self.Network {
				linkedNetIDs[l.B] = true
			} else if l.B == self.Network {
				linkedNetIDs[l.A] = true
			}
		}
	}
	peers := []peerView{}
	for _, d := range s.Devices {
		if d.PubKey == self.PubKey {
			continue
		}
		// 只把"在线"的设备作为 peer 下发(主动下线或心跳超时都算离线)
		if d.Offline || time.Since(d.LastSeen) > onlineWindow {
			continue
		}
		// 互联判定:① 同网络 ② device link 指定的设备 ③ network link 对方网络的设备
		if d.Network != self.Network && !linkedDevIDs[d.ID] && !linkedNetIDs[d.Network] {
			continue
		}
		peers = append(peers, peerView{
			Name:       d.Name,
			PubKey:     d.PubKey,
			VirtualIP:  d.VirtualIP,
			Endpoint:   chooseEndpoint(self, d),
			LANSubnets: d.LANSubnets,
		})
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].VirtualIP < peers[j].VirtualIP })
	return peers
}

// routesForLocked 计算互联规则给本设备带来的额外路由:
// network link => 对端网络 CIDR;device link => 对端设备虚拟 IP/32(去重)
func (s *Store) routesForLocked(self *Device) []string {
	seen := map[string]bool{}
	routes := []string{}
	for _, l := range s.Links {
		switch l.Type {
		case "network":
			var otherID string
			if l.A == self.Network {
				otherID = l.B
			} else if l.B == self.Network {
				otherID = l.A
			} else {
				continue
			}
			if n := s.Networks[otherID]; n != nil && !seen[n.CIDR] {
				seen[n.CIDR] = true
				routes = append(routes, n.CIDR)
			}
		case "device":
			var otherID string
			if l.A == self.ID {
				otherID = l.B
			} else if l.B == self.ID {
				otherID = l.A
			} else {
				continue
			}
			if d := s.deviceByIDLocked(otherID); d != nil && d.VirtualIP != "" {
				r := d.VirtualIP + "/32"
				if !seen[r] {
					seen[r] = true
					routes = append(routes, r)
				}
			}
		}
	}
	sort.Strings(routes)
	return routes
}

// chooseEndpoint 为 peer 选择合适的 endpoint:
//   - 两端在同一 NAT 后面(公网 IP 相同)→ 走内网 endpoint,直连最快
//   - 跨 NAT → 走 STUN 探测到的公网映射地址,配合打洞
//   - 没有公网信息(controller 在局域网)→ 退回内网 endpoint
func chooseEndpoint(self, peer *Device) string {
	selfPub := hostOf(self.PublicEndpoint)
	peerPub := hostOf(peer.PublicEndpoint)
	if selfPub != "" && peerPub != "" {
		if selfPub == peerPub && peer.Endpoint != "" {
			return peer.Endpoint
		}
		return peer.PublicEndpoint
	}
	return peer.Endpoint
}

func hostOf(endpoint string) string {
	h, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return ""
	}
	return h
}

// filterLANLocked 校验并过滤设备上报的 LAN 网段(站点到站点):
// 格式非法的网段、或已被其他设备通告的网段,从本设备剔除并记入 LANConflicts
// (不拒绝注册,只日志告警);无冲突时 LANConflicts 清空。
func (s *Store) filterLANLocked(d *Device, reported []string) {
	// 其他设备当前已通告的网段(归一化后比较)-> 占用设备名
	taken := map[string]string{}
	for _, other := range s.Devices {
		if other.PubKey == d.PubKey {
			continue
		}
		for _, c := range other.LANSubnets {
			taken[c] = other.Name
		}
	}
	var ok, conflicts []string
	for _, raw := range reported {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(raw)
		if err != nil {
			conflicts = append(conflicts, raw) // 原样记录,便于排查
			log.Printf("[lan] 设备 %s 上报的网段 %q 格式非法,已剔除", d.Name, raw)
			continue
		}
		cidr := ipnet.String()
		if owner, dup := taken[cidr]; dup {
			if owner == d.Name {
				continue // 同一设备重复上报,只保留一份
			}
			conflicts = append(conflicts, raw)
			log.Printf("[lan] 设备 %s 上报的网段 %s 与设备 %s 冲突,已剔除", d.Name, cidr, owner)
			continue
		}
		ok = append(ok, cidr)
		taken[cidr] = d.Name
	}
	d.LANSubnets = ok
	d.LANConflicts = conflicts
}

// macSet 把 MAC 列表转成集合(小写归一化)
func macSet(macs []string) map[string]bool {
	m := map[string]bool{}
	for _, mac := range macs {
		if mac = strings.ToLower(strings.TrimSpace(mac)); mac != "" {
			m[mac] = true
		}
	}
	return m
}

// isDisabledByMACLocked 检查是否有其他被禁用的设备与给定 MAC 列表匹配;
// 返回匹配到的设备(用于错误提示),无匹配返回 nil。
func (s *Store) isDisabledByMACLocked(pubKey string, macs []string) *Device {
	if len(macs) == 0 {
		return nil
	}
	set := macSet(macs)
	for _, d := range s.Devices {
		if !d.Disabled || d.PubKey == pubKey {
			continue
		}
		for _, m := range d.MACs {
			if set[m] {
				return d
			}
		}
	}
	return nil
}

func (s *Store) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ip := clientIP(r)
	if s.registerFailLimit != nil && !s.registerFailLimit.allowed(ip) {
		http.Error(w, "注册尝试过于频繁,请 5 分钟后再试", http.StatusTooManyRequests)
		return
	}
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PubKey == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 账号校验(v0.7.0 起关闭公开注册):账密必须匹配后台已创建的账号,
	// 所属网络由账号绑定值决定(忽略客户端自报的 network)
	acc := s.authenticateLocked(req.Username, req.Password)
	if acc == nil {
		if s.registerFailLimit != nil {
			s.registerFailLimit.record(ip)
		}
		http.Error(w, "账号或密码错误,或账号未在后台创建,请联系管理员", http.StatusForbidden)
		return
	}
	nw := s.Networks[acc.NetworkID]
	if nw == nil {
		http.Error(w, "账号绑定的网络不存在,请联系管理员", http.StatusForbidden)
		return
	}

	// 设备数上限:统计该账号已绑定的其他设备(同一密钥重注册不算),
	// 达上限拒绝新设备注册——删除闲置设备或调大上限后再试
	other := 0
	for _, dev := range s.Devices {
		if dev.Username == acc.Username && dev.PubKey != req.PubKey {
			other++
		}
	}
	if limit := acc.deviceLimit(); other >= limit {
		http.Error(w, fmt.Sprintf("账号 %s 的设备数已达上限 %d,请在面板删除闲置设备或调大上限", acc.Username, limit), http.StatusForbidden)
		return
	}

	d, exists := s.Devices[req.PubKey]
	if exists {
		if d.Disabled {
			http.Error(w, "该设备已被禁用", http.StatusForbidden)
			return
		}
	} else if matched := s.isDisabledByMACLocked(req.PubKey, req.MACs); matched != nil {
		// 同机器换密钥也无法绕过禁用
		http.Error(w, fmt.Sprintf("该机器(MAC)已被禁用(关联设备 %s),请先在面板启用原设备", matched.Name), http.StatusForbidden)
		return
	}

	if !exists {
		ip, err := s.nextIPInNetworkLocked(nw.ID, nw.CIDR)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		d = &Device{
			ID:        newID(),
			PubKey:    req.PubKey,
			VirtualIP: ip,
			Network:   nw.ID,
			CreatedAt: time.Now(),
		}
		s.Devices[req.PubKey] = d
		log.Printf("[register] 新设备 %s (%s) -> %s (网络 %s)", req.Name, shortKey(req.PubKey), ip, nw.Name)
	} else if err := s.switchNetworkLocked(d, nw); err != nil {
		// 设备报的网络与当前所属不同:换网络并重分配 IP
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if req.Name != "" {
		d.Name = req.Name
	}
	if req.Endpoint != "" {
		d.Endpoint = req.Endpoint
	}
	if req.PublicEndpoint != "" {
		d.PublicEndpoint = req.PublicEndpoint
	}
	if req.Platform != "" {
		d.Platform = req.Platform
	}
	if len(req.MACs) > 0 {
		d.MACs = req.MACs
	}
	if req.MachineID != "" {
		d.MachineID = req.MachineID
	}
	d.LastSeen = time.Now()
	d.Offline = false // 重新注册/心跳恢复,清除主动下线标记
	d.Username = acc.Username
	// 站点到站点:上报了 LAN 网段则做冲突检测后更新(nil 为老版本 agent,不动)
	if req.LANSubnets != nil {
		s.filterLANLocked(d, req.LANSubnets)
	}
	s.saveLocked()

	writeJSON(w, registerResp{
		ID:        d.ID,
		VirtualIP: d.VirtualIP,
		CIDR:      nw.CIDR,
		Network:   nw.Name,
		Peers:     s.peersForLocked(d),
		Routes:    s.routesForLocked(d),
		Relays:    s.onlineRelaysLocked(),
		Revision:  s.rev,
	})
}

func (s *Store) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PubKey == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.Devices[req.PubKey]
	if !ok {
		http.Error(w, "not registered", http.StatusUnauthorized)
		return
	}
	if d.Disabled {
		http.Error(w, "该设备已被禁用", http.StatusForbidden)
		return
	}
	if matched := s.isDisabledByMACLocked(req.PubKey, req.MACs); matched != nil {
		http.Error(w, fmt.Sprintf("该机器(MAC)已被禁用(关联设备 %s),请先在面板启用原设备", matched.Name), http.StatusForbidden)
		return
	}
	// 心跳附带设备名称:允许运行中的客户端改名(GUI 改配置后随心跳同步)
	nameChanged := false
	if req.Name != "" && req.Name != d.Name {
		d.Name = req.Name
		nameChanged = true
	}
	// 账号绑定的设备:网络固定为账号绑定值(忽略客户端自报);
	// 旧设备(无账号)按自报网络解析:名称不存在 => 403,与当前所属不同 => 换网络并重分配 IP
	var nw *Network
	if d.Username != "" {
		for _, a := range s.Accounts {
			if a.Username == d.Username {
				nw = s.Networks[a.NetworkID]
				break
			}
		}
		if nw == nil {
			http.Error(w, "账号绑定的网络不存在,请联系管理员", http.StatusForbidden)
			return
		}
	} else {
		nw = s.resolveNetworkLocked(req.Network)
		if nw == nil {
			http.Error(w, fmt.Sprintf("网络 %s 不存在,请在面板创建后重试", req.Network), http.StatusForbidden)
			return
		}
	}
	netChanged := d.Network != nw.ID
	if err := s.switchNetworkLocked(d, nw); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if req.Endpoint != "" {
		d.Endpoint = req.Endpoint
	}
	if req.PublicEndpoint != "" {
		d.PublicEndpoint = req.PublicEndpoint
	}
	d.LastSeen = time.Now()
	d.Offline = false // 心跳恢复,清除主动下线标记
	if len(req.MACs) > 0 {
		d.MACs = req.MACs
	}
	if req.MachineID != "" {
		d.MachineID = req.MachineID
	}
	// 站点到站点:心跳同样上报 LAN 网段,冲突检测后更新(nil 为老版本 agent,不动)
	if req.LANSubnets != nil {
		s.filterLANLocked(d, req.LANSubnets)
	}
	// 心跳附带延迟数据则更新;全部 ping 失败时 agent 不上报,旧数据自然过期
	if len(req.Latencies) > 0 {
		d.Latencies = req.Latencies
		d.LatencyUpdatedAt = time.Now()
	}
	if netChanged || nameChanged {
		s.saveLocked() // 换网络导致虚拟 IP 变化 / 改名,需要持久化
	}
	// 心跳顺带返回最新 peer 列表,agent 一个请求搞定两件事
	writeJSON(w, registerResp{
		ID:        d.ID,
		VirtualIP: d.VirtualIP,
		CIDR:      nw.CIDR,
		Network:   nw.Name,
		Peers:     s.peersForLocked(d),
		Routes:    s.routesForLocked(d),
		Relays:    s.onlineRelaysLocked(),
		Revision:  s.rev,
	})
}

// handleOffline 处理 agent 正常退出时的主动下线通知:
// 置 Offline 标记(保留 LastSeen 供面板显示最后心跳时间),设备立即判定为离线。
func (s *Store) handleOffline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		PubKey string `json:"pubkey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PubKey == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.Devices[req.PubKey]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	d.Offline = true
	s.saveLocked()
	log.Printf("[offline] 设备 %s (%s) 主动下线", d.Name, d.VirtualIP)
	w.WriteHeader(http.StatusOK)
}

func (s *Store) handleDevices(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.HasPrefix(r.URL.Path, "/api/devices/") {
		rest := strings.TrimPrefix(r.URL.Path, "/api/devices/")
		id, action, _ := strings.Cut(rest, "/")
		d := s.deviceByIDLocked(id)
		if d == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		switch {
		case r.Method == http.MethodDelete && action == "":
			delete(s.Devices, d.PubKey)
			// 清理涉及该设备的点对点互联规则，避免残留
			for lid, l := range s.Links {
				if l.Type == "device" && (l.A == id || l.B == id) {
					delete(s.Links, lid)
					log.Printf("[delete] 设备 %s 删除，清理点对点规则 %s", d.Name, lid)
				}
			}
			s.saveLocked()
			log.Printf("[delete] 设备 %s (%s)", d.Name, d.VirtualIP)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && action == "":
			s.handleUpdateDeviceLocked(w, r, id)
		case r.Method == http.MethodPost && action == "kick":
			// 强制下线:只置 offline 标记(下次心跳会恢复),不拒绝后续连接
			d.Offline = true
			s.saveLocked()
			log.Printf("[kick] 强制下线设备 %s (%s)", d.Name, d.VirtualIP)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && action == "disable":
			// 禁用设备:立刻踢下线并拒绝该 pubkey/该 MAC 的后续注册和心跳
			d.Disabled = true
			d.Offline = true
			s.saveLocked()
			log.Printf("[disable] 禁用设备 %s (%s)", d.Name, d.VirtualIP)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && action == "enable":
			d.Disabled = false
			s.saveLocked()
			log.Printf("[enable] 启用设备 %s (%s)", d.Name, d.VirtualIP)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	type deviceView struct {
		*Device
		Online      bool   `json:"online"`
		NetworkName string `json:"network_name"` // 所属网络名称(面板展示用)
	}
	list := []deviceView{}
	for _, d := range s.Devices {
		netName := ""
		if n := s.Networks[d.Network]; n != nil {
			netName = n.Name
		}
		list = append(list, deviceView{Device: d, Online: !d.Offline && time.Since(d.LastSeen) <= onlineWindow, NetworkName: netName})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].VirtualIP < list[j].VirtualIP })
	writeJSON(w, list)
}

// handleWait 长轮询:客户端上报当前已知的 revision,服务器在配置版本号变化时立即返回,
// 否则阻塞到客户端断开(约 25s 超时)。实现设备/网络/规则变更后 near-realtime 同步。
func (s *Store) handleWait(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rev, _ := strconv.ParseInt(r.URL.Query().Get("rev"), 10, 64)
	s.mu.Lock()
	if s.rev > rev {
		defer s.mu.Unlock()
		writeJSON(w, map[string]int64{"revision": s.rev})
		return
	}
	// 客户端断开时唤醒,避免 goroutine 长时间阻塞
	done := make(chan struct{})
	go func() {
		<-r.Context().Done()
		s.mu.Lock()
		s.cond.Broadcast()
		s.mu.Unlock()
		close(done)
	}()
	for s.rev <= rev {
		s.cond.Wait()
		select {
		case <-done:
			s.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		default:
		}
	}
	defer s.mu.Unlock()
	writeJSON(w, map[string]int64{"revision": s.rev})
}

// handleUpdateDeviceLocked 修改设备:换网络(按名称,换网络会重分配虚拟 IP)、手动指定虚拟 IP
// body 字段均可选;手动 IP 必须落在所属网络 CIDR 内且未被同网络设备占用
func (s *Store) handleUpdateDeviceLocked(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Network   *string `json:"network"`
		VirtualIP *string `json:"virtual_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	d := s.deviceByIDLocked(id)
	if d == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if req.Network != nil {
		nw := s.resolveNetworkLocked(*req.Network)
		if nw == nil {
			http.Error(w, fmt.Sprintf("网络 %s 不存在", *req.Network), http.StatusBadRequest)
			return
		}
		// 账号绑定设备:修改设备所属网络时,账号绑定的网络同步跟随变化,
		// 且该账号下所有设备一起迁移到新网络(避免心跳时又被迁回)
		if d.Username != "" {
			var acc *Account
			for _, a := range s.Accounts {
				if a.Username == d.Username {
					acc = a
					break
				}
			}
			if acc != nil {
				acc.NetworkID = nw.ID
			}
			for _, dev := range s.Devices {
				if dev.Username == d.Username && dev.Network != nw.ID {
					if err := s.switchNetworkLocked(dev, nw); err != nil {
						log.Printf("[update] 账号 %s 的设备 %s 同步换网络失败: %v", d.Username, dev.Name, err)
					}
				}
			}
		} else {
			if err := s.switchNetworkLocked(d, nw); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
		}
	}
	if req.VirtualIP != nil && strings.TrimSpace(*req.VirtualIP) != "" {
		ip := net.ParseIP(strings.TrimSpace(*req.VirtualIP))
		if ip == nil || ip.To4() == nil {
			http.Error(w, fmt.Sprintf("虚拟 IP %s 格式非法", *req.VirtualIP), http.StatusBadRequest)
			return
		}
		nw := s.Networks[d.Network]
		_, ipnet, err := net.ParseCIDR(nw.CIDR)
		if err != nil || !ipnet.Contains(ip) {
			http.Error(w, fmt.Sprintf("虚拟 IP %s 不在所属网络 %s 的网段 %s 内", ip, nw.Name, nw.CIDR), http.StatusBadRequest)
			return
		}
		for _, other := range s.Devices {
			if other.PubKey != d.PubKey && other.Network == d.Network && other.VirtualIP == ip.String() {
				http.Error(w, fmt.Sprintf("虚拟 IP %s 已被设备 %s 占用", ip, other.Name), http.StatusConflict)
				return
			}
		}
		d.VirtualIP = ip.String()
	}
	s.saveLocked()
	log.Printf("[update] 设备 %s 更新:网络 %s,虚拟 IP %s", d.Name, d.Network, d.VirtualIP)
	w.WriteHeader(http.StatusOK)
}

// shortKey 截断公钥用于日志展示(短于 12 字符时原样返回,避免切片越界)
func shortKey(k string) string {
	if len(k) <= 12 {
		return k
	}
	return k[:12] + "..."
}

// ---------- 账号管理 API ----------

func (s *Store) handleAccounts(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.HasPrefix(r.URL.Path, "/api/accounts/") {
		rest := strings.TrimPrefix(r.URL.Path, "/api/accounts/")
		id, action, _ := strings.Cut(rest, "/")
		acc := s.accountByIDLocked(id)
		if acc == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		switch {
		case r.Method == http.MethodDelete && action == "":
			// 删除账号前,先强制下线该账号下所有在线设备;
			// 后续心跳因账号不存在会被拒绝,实现实时踢人。
			kicked := 0
			for _, d := range s.Devices {
				if d.Username == acc.Username {
					if !d.Offline {
						d.Offline = true
						kicked++
						log.Printf("[account] 账号 %s 删除,强制下线设备 %s (%s)", acc.Username, d.Name, d.VirtualIP)
					}
				}
			}
			delete(s.Accounts, acc.ID)
			s.saveLocked()
			log.Printf("[account] 删除账号 %s (关联 %d 个设备,本次下线 %d 个)", acc.Username, len(s.Devices), kicked)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && action == "password":
			// 重置密码:支持手动指定( body.password ),留空则随机生成 10 位;明文只在本次响应返回一次
			var req struct {
				Password string `json:"password"`
			}
			json.NewDecoder(r.Body).Decode(&req) // 空 body 也允许
			pw := strings.TrimSpace(req.Password)
			if pw == "" {
				pw = randomPassword()
			}
			acc.Salt = newID()
			acc.PasswordHash = hashPassword(acc.Salt, pw)
			s.saveLocked()
			log.Printf("[account] 重置账号 %s 密码", acc.Username)
			writeJSON(w, map[string]string{"password": pw})
		case r.Method == http.MethodPost && action == "limit":
			// 调整设备数上限(>=1)
			var req struct {
				MaxDevices int `json:"max_devices"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MaxDevices < 1 {
				http.Error(w, "max_devices 必须 >= 1", http.StatusBadRequest)
				return
			}
			acc.MaxDevices = req.MaxDevices
			s.saveLocked()
			log.Printf("[account] 账号 %s 设备数上限调整为 %d", acc.Username, req.MaxDevices)
			writeJSON(w, map[string]int{"max_devices": acc.MaxDevices})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		// 列表绝不含 password_hash/salt
		type accountView struct {
			ID          string    `json:"id"`
			Username    string    `json:"username"`
			NetworkName string    `json:"network_name"`
			DeviceCount int       `json:"device_count"`
			MaxDevices  int       `json:"max_devices"`
			CreatedAt   time.Time `json:"created_at"`
		}
		list := []accountView{}
		for _, a := range s.Accounts {
			cnt := 0
			for _, d := range s.Devices {
				if d.Username == a.Username {
					cnt++
				}
			}
			netName := ""
			if n := s.Networks[a.NetworkID]; n != nil {
				netName = n.Name
			}
			list = append(list, accountView{ID: a.ID, Username: a.Username, NetworkName: netName, DeviceCount: cnt, MaxDevices: a.deviceLimit(), CreatedAt: a.CreatedAt})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Username < list[j].Username })
		writeJSON(w, list)
	case http.MethodPost:
		var req struct {
			Username   string `json:"username"`
			Password   string `json:"password"`
			NetworkID  string `json:"network_id"`
			MaxDevices int    `json:"max_devices"` // 留空/0 取默认值
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" {
			http.Error(w, "用户名不能为空", http.StatusBadRequest)
			return
		}
		for _, a := range s.Accounts {
			if a.Username == req.Username {
				http.Error(w, fmt.Sprintf("用户名 %s 已存在", req.Username), http.StatusConflict)
				return
			}
		}
		if s.Networks[req.NetworkID] == nil {
			http.Error(w, "指定的网络不存在", http.StatusBadRequest)
			return
		}
		pw := req.Password
		if pw == "" {
			pw = randomPassword() // 未填密码则随机生成,明文只在本次响应返回一次
		}
		acc := &Account{
			ID:         newID(),
			Username:   req.Username,
			Salt:       newID(),
			NetworkID:  req.NetworkID,
			MaxDevices: req.MaxDevices,
			CreatedAt:  time.Now(),
		}
		acc.PasswordHash = hashPassword(acc.Salt, pw)
		s.Accounts[acc.ID] = acc
		s.saveLocked()
		log.Printf("[account] 新建账号 %s (网络 %s, 设备上限 %d)", acc.Username, req.NetworkID, acc.deviceLimit())
		writeJSON(w, map[string]any{
			"id": acc.ID, "username": acc.Username, "network_id": acc.NetworkID,
			"max_devices": acc.deviceLimit(), "password": pw, "created_at": acc.CreatedAt,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------- 网络管理 API ----------

func (s *Store) handleNetworks(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.HasPrefix(r.URL.Path, "/api/networks/") {
		id := strings.TrimPrefix(r.URL.Path, "/api/networks/")
		n, ok := s.Networks[id]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodDelete:
			if id == defaultNetworkID {
				http.Error(w, "默认网络不允许删除", http.StatusForbidden)
				return
			}
			for _, d := range s.Devices {
				if d.Network == id {
					http.Error(w, fmt.Sprintf("网络 %s 下还有设备,请先移除或转移设备", n.Name), http.StatusConflict)
					return
				}
			}
			delete(s.Networks, id)
			// 清理涉及该网络的互联规则
			for lid, l := range s.Links {
				if l.Type == "network" && (l.A == id || l.B == id) {
					delete(s.Links, lid)
				}
			}
			s.saveLocked()
			log.Printf("[network] 删除网络 %s (%s)", n.Name, n.CIDR)
			w.WriteHeader(http.StatusNoContent)
		case http.MethodPut:
			// 编辑网络:允许修改名称和 CIDR(默认网络只能改名)
			var req struct {
				Name string `json:"name"`
				CIDR string `json:"cidr"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			req.Name = strings.TrimSpace(req.Name)
			if req.Name == "" {
				http.Error(w, "网络名称不能为空", http.StatusBadRequest)
				return
			}
			// 名称冲突检查(排除自身)
			for _, other := range s.Networks {
				if other.ID != id && other.Name == req.Name {
					http.Error(w, fmt.Sprintf("网络名称 %s 已存在", req.Name), http.StatusConflict)
					return
				}
			}
			newCIDR := strings.TrimSpace(req.CIDR)
			if newCIDR != "" {
				if id == defaultNetworkID {
					http.Error(w, "默认网络不允许修改 CIDR", http.StatusForbidden)
					return
				}
				_, ipnet, err := net.ParseCIDR(newCIDR)
				if err != nil {
					http.Error(w, fmt.Sprintf("CIDR %s 格式非法", newCIDR), http.StatusBadRequest)
					return
				}
				// CIDR 重叠检查(排除自身)
				for _, other := range s.Networks {
					if other.ID == id {
						continue
					}
					_, existing, err := net.ParseCIDR(other.CIDR)
					if err == nil && (existing.Contains(ipnet.IP) || ipnet.Contains(existing.IP)) {
						http.Error(w, fmt.Sprintf("CIDR %s 与已有网络 %s (%s) 重叠", ipnet, other.Name, other.CIDR), http.StatusConflict)
						return
					}
				}
				n.CIDR = ipnet.String()
			}
			n.Name = req.Name
			s.saveLocked()
			log.Printf("[network] 编辑网络 %s (%s)", n.Name, n.CIDR)
			writeJSON(w, n)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		type networkView struct {
			*Network
			DeviceCount int `json:"device_count"`
		}
		list := []networkView{}
		for _, n := range s.Networks {
			cnt := 0
			for _, d := range s.Devices {
				if d.Network == n.ID {
					cnt++
				}
			}
			list = append(list, networkView{Network: n, DeviceCount: cnt})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
		writeJSON(w, list)
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
			CIDR string `json:"cidr"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			http.Error(w, "网络名称不能为空", http.StatusBadRequest)
			return
		}
		_, ipnet, err := net.ParseCIDR(strings.TrimSpace(req.CIDR))
		if err != nil {
			http.Error(w, fmt.Sprintf("CIDR %s 格式非法", req.CIDR), http.StatusBadRequest)
			return
		}
		for _, n := range s.Networks {
			if n.Name == req.Name {
				http.Error(w, fmt.Sprintf("网络名称 %s 已存在", req.Name), http.StatusConflict)
				return
			}
			_, existing, err := net.ParseCIDR(n.CIDR)
			if err == nil && (existing.Contains(ipnet.IP) || ipnet.Contains(existing.IP)) {
				http.Error(w, fmt.Sprintf("CIDR %s 与已有网络 %s (%s) 重叠", ipnet, n.Name, n.CIDR), http.StatusConflict)
				return
			}
		}
		n := &Network{ID: newID(), Name: req.Name, CIDR: ipnet.String()}
		s.Networks[n.ID] = n
		s.saveLocked()
		log.Printf("[network] 新建网络 %s (%s)", n.Name, n.CIDR)
		writeJSON(w, n)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------- 互联规则 API ----------

func (s *Store) handleLinks(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.HasPrefix(r.URL.Path, "/api/links/") {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/links/")
		if l, ok := s.Links[id]; ok {
			delete(s.Links, id)
			s.saveLocked()
			log.Printf("[link] 删除规则 %s (%s: %s <-> %s)", id, l.Type, l.A, l.B)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		list := []*Link{}
		for _, l := range s.Links {
			list = append(list, l)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
		writeJSON(w, list)
	case http.MethodPost:
		var req struct {
			Type string `json:"type"`
			A    string `json:"a"`
			B    string `json:"b"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Type != "network" && req.Type != "device" {
			http.Error(w, "type 只能是 network 或 device", http.StatusBadRequest)
			return
		}
		if req.A == "" || req.B == "" || req.A == req.B {
			http.Error(w, "两端不能为空且不能相同", http.StatusBadRequest)
			return
		}
		// 校验两端存在
		if req.Type == "network" {
			if s.Networks[req.A] == nil || s.Networks[req.B] == nil {
				http.Error(w, "指定的网络不存在", http.StatusBadRequest)
				return
			}
		} else {
			if s.deviceByIDLocked(req.A) == nil || s.deviceByIDLocked(req.B) == nil {
				http.Error(w, "指定的设备不存在", http.StatusBadRequest)
				return
			}
		}
		// 与已有规则重复(A/B 无序)
		for _, l := range s.Links {
			if l.Type == req.Type && (l.A == req.A && l.B == req.B || l.A == req.B && l.B == req.A) {
				http.Error(w, "该互联规则已存在", http.StatusConflict)
				return
			}
		}
		l := &Link{ID: newID(), Type: req.Type, A: req.A, B: req.B}
		s.Links[l.ID] = l
		s.saveLocked()
		log.Printf("[link] 新建规则 %s (%s: %s <-> %s)", l.ID, l.Type, l.A, l.B)
		writeJSON(w, l)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type topologyNode struct {
	ID           string `json:"id"`
	Type         string `json:"type"` // network | device | account
	Name         string `json:"name"`
	CIDR         string `json:"cidr,omitempty"`
	VirtualIP    string `json:"virtual_ip,omitempty"`
	Username     string `json:"username,omitempty"`
	Network      string `json:"network,omitempty"`
	Online       bool   `json:"online,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
	DeviceCount  int    `json:"device_count,omitempty"`
	AccountColor string `json:"account_color,omitempty"`
}

type topologyEdge struct {
	ID     string `json:"id"`
	Type   string `json:"type"` // belongs | network-link | device-link | owns
	Source string `json:"source"`
	Target string `json:"target"`
	Name   string `json:"name,omitempty"`
}

// ---------- Relay 中转节点管理 API ----------

// handleRelays 管理面板的 relay CRUD;支持 relay 服务通过 /api/relays/beat 上报心跳。
func (s *Store) handleRelays(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.HasPrefix(r.URL.Path, "/api/relays/") {
		rest := strings.TrimPrefix(r.URL.Path, "/api/relays/")
		id, action, _ := strings.Cut(rest, "/")

		// 心跳接口:供独立 relay 服务调用,无需管理员认证(通过共享 secret 或内网信任)
		// 支持 /api/relays/beat 以及 /api/relays/{id}/beat
		if r.Method == http.MethodPost && (action == "beat" || rest == "beat") {
			if rest == "beat" {
				id = ""
			}
			s.handleRelayBeatLocked(w, r, id)
			return
		}

		rn := s.Relays[id]
		if rn == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		switch {
		case r.Method == http.MethodDelete && action == "":
			delete(s.Relays, id)
			s.saveLocked()
			log.Printf("[relay] 删除中转节点 %s (%s)", rn.ID, rn.Address)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && action == "":
			var req struct {
				Address string `json:"address"`
				PubKey  string `json:"pubkey"`
				Note    string `json:"note"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			req.Address = strings.TrimSpace(req.Address)
			if req.Address == "" {
				http.Error(w, "中转地址不能为空", http.StatusBadRequest)
				return
			}
			// 地址冲突(排除自身)
			for _, other := range s.Relays {
				if other.ID != id && other.Address == req.Address {
					http.Error(w, "中转地址已存在", http.StatusConflict)
					return
				}
			}
			rn.Address = req.Address
			rn.PubKey = strings.TrimSpace(req.PubKey)
			rn.Note = strings.TrimSpace(req.Note)
			s.saveLocked()
			log.Printf("[relay] 更新中转节点 %s -> %s", rn.ID, rn.Address)
			writeJSON(w, rn)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.refreshRelayStatusLocked()
		list := []*RelayNode{}
		for _, rn := range s.Relays {
			list = append(list, rn)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
		writeJSON(w, list)
	case http.MethodPost:
		var req struct {
			Address string `json:"address"`
			PubKey  string `json:"pubkey"`
			Note    string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		req.Address = strings.TrimSpace(req.Address)
		if req.Address == "" {
			http.Error(w, "中转地址不能为空", http.StatusBadRequest)
			return
		}
		for _, other := range s.Relays {
			if other.Address == req.Address {
				http.Error(w, "中转地址已存在", http.StatusConflict)
				return
			}
		}
		now := time.Now()
		rn := &RelayNode{
			ID:        newID(),
			Address:   req.Address,
			PubKey:    strings.TrimSpace(req.PubKey),
			Note:      strings.TrimSpace(req.Note),
			CreatedAt: now,
			LastSeen:  now,
			Online:    true,
		}
		s.Relays[rn.ID] = rn
		s.saveLocked()
		log.Printf("[relay] 新建中转节点 %s (%s)", rn.ID, rn.Address)
		writeJSON(w, rn)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRelayBeatLocked 处理 relay 心跳:report address/pubkey 并刷新 last_seen。
// 若 ID 不存在但 address 已存在,则按地址合并(方便 relay 重启后 ID 变化场景)。
func (s *Store) handleRelayBeatLocked(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Address string `json:"address"`
		PubKey  string `json:"pubkey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// 兼容纯 GET 心跳(无 body)
		req.Address = ""
	}
	req.Address = strings.TrimSpace(req.Address)

	var rn *RelayNode
	if id != "" {
		rn = s.Relays[id]
	}
	if rn == nil && req.Address != "" {
		for _, other := range s.Relays {
			if other.Address == req.Address {
				rn = other
				break
			}
		}
	}
	if rn == nil {
		http.Error(w, "relay not registered", http.StatusNotFound)
		return
	}

	if req.Address != "" {
		rn.Address = req.Address
	}
	if req.PubKey != "" {
		rn.PubKey = req.PubKey
	}
	rn.LastSeen = time.Now()
	rn.Online = true
	s.saveLocked()
	writeJSON(w, map[string]any{"id": rn.ID, "online": true})
}

// handleTopology 返回网络拓扑图数据:网络、设备、账号为节点,归属/互联/拥有关系为边
func (s *Store) handleTopology(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// 为每个账号分配一个颜色,用于设备染色
	accountColors := []string{"#0ea5e9", "#22c55e", "#f59e0b", "#ec4899", "#8b5cf6", "#14b8a6", "#f97316", "#06b6d4"}
	accColor := map[string]string{}
	idx := 0
	for _, a := range s.Accounts {
		accColor[a.Username] = accountColors[idx%len(accountColors)]
		idx++
	}

	nodes := []topologyNode{}
	edges := []topologyEdge{}

	// 网络节点
	for _, n := range s.Networks {
		cnt := 0
		for _, d := range s.Devices {
			if d.Network == n.ID {
				cnt++
			}
		}
		nodes = append(nodes, topologyNode{
			ID:          "net_" + n.ID,
			Type:        "network",
			Name:        n.Name,
			CIDR:        n.CIDR,
			DeviceCount: cnt,
		})
	}

	// 设备节点 + 归属边
	for _, d := range s.Devices {
		online := !d.Offline && time.Since(d.LastSeen) <= onlineWindow
		nodes = append(nodes, topologyNode{
			ID:           "dev_" + d.ID,
			Type:         "device",
			Name:         d.Name,
			VirtualIP:    d.VirtualIP,
			Username:     d.Username,
			Network:      d.Network,
			Online:       online,
			Disabled:     d.Disabled,
			AccountColor: accColor[d.Username],
		})
		edges = append(edges, topologyEdge{
			ID:     "belongs_" + d.ID,
			Type:   "belongs",
			Source: "dev_" + d.ID,
			Target: "net_" + d.Network,
		})
		// 账号拥有边
		if d.Username != "" {
			edges = append(edges, topologyEdge{
				ID:     "owns_" + d.ID,
				Type:   "owns",
				Source: "acc_" + d.Username,
				Target: "dev_" + d.ID,
			})
		}
	}

	// 账号节点
	for _, a := range s.Accounts {
		nodes = append(nodes, topologyNode{
			ID:           "acc_" + a.Username,
			Type:         "account",
			Name:         a.Username,
			AccountColor: accColor[a.Username],
		})
	}

	// 互联规则边
	for _, l := range s.Links {
		switch l.Type {
		case "network":
			edges = append(edges, topologyEdge{
				ID:     "link_" + l.ID,
				Type:   "network-link",
				Source: "net_" + l.A,
				Target: "net_" + l.B,
				Name:   "网对网",
			})
		case "device":
			edges = append(edges, topologyEdge{
				ID:     "link_" + l.ID,
				Type:   "device-link",
				Source: "dev_" + l.A,
				Target: "dev_" + l.B,
				Name:   "点对点",
			})
		}
	}

	writeJSON(w, map[string]any{"nodes": nodes, "edges": edges})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	addr := flag.String("addr", ":52888", "监听地址(HTTP 走 TCP,STUN 走 UDP,同端口)")
	dataPath := flag.String("data", "data.json", "数据持久化文件路径(默认当前目录 data.json)")
	adminDBPath := flag.String("admin-db", "/var/lib/aleiyun/admin.json", "管理员账号存储文件路径(JSON 格式)")
	adminUser := flag.String("admin-user", "admin", "默认管理员用户名")
	adminPass := flag.String("admin-pass", "", "默认管理员密码(空则随机生成并打印到日志)")
	secureCookie := flag.Bool("secure-cookie", false, "Cookie 是否标记 Secure(需要 HTTPS)")
	flag.Parse()

	store := NewStore(*dataPath)

	if err := initAdminAuth(*adminDBPath, *adminUser, *adminPass, *secureCookie); err != nil {
		log.Fatalf("初始化管理员认证失败: %v", err)
	}

	// STUN 服务:UDP 与 HTTP 同端口,供 agent 探测公网映射地址
	if udpAddr, err := net.ResolveUDPAddr("udp", *addr); err == nil {
		if uc, err := net.ListenUDP("udp", udpAddr); err == nil {
			go serveSTUN(uc)
			log.Printf("STUN 服务启动: UDP %s", *addr)
		} else {
			log.Printf("STUN 服务启动失败(不影响面板): %v", err)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/register", store.handleRegister)
	mux.HandleFunc("/api/heartbeat", store.handleHeartbeat)
	mux.HandleFunc("/api/offline", store.handleOffline)
	mux.HandleFunc("/api/wait", store.handleWait)
	mux.HandleFunc("/api/devices", store.handleDevices)
	mux.HandleFunc("/api/devices/", store.handleDevices)
	mux.HandleFunc("/api/networks", store.handleNetworks)
	mux.HandleFunc("/api/networks/", store.handleNetworks)
	mux.HandleFunc("/api/links", store.handleLinks)
	mux.HandleFunc("/api/links/", store.handleLinks)
	mux.HandleFunc("/api/accounts", store.handleAccounts)
	mux.HandleFunc("/api/accounts/", store.handleAccounts)
	mux.HandleFunc("/api/relays", store.handleRelays)
	mux.HandleFunc("/api/relays/", store.handleRelays)
	mux.HandleFunc("/api/topology", store.handleTopology)
	mux.HandleFunc("/api/admin/login", handleAdminLogin)
	mux.HandleFunc("/api/admin/logout", handleAdminLogout)
	mux.HandleFunc("/api/admin/password", handleAdminPasswordChange)
	mux.HandleFunc("/api/admin/session", handleAdminSession)

	panel, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(panel)))

	log.Printf("Aleiyun server %s 启动: http://localhost%s  (默认网段 %s)", Version, *addr, defaultNetworkCIDR)
	log.Printf("源码地址: GitHub https://github.com/linsmes | Gitee https://gitee.com/linsmes")
	log.Fatal(http.ListenAndServe(*addr, authMiddleware(mux)))
}
