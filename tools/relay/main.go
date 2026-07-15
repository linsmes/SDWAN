// Aleiyun Relay — 独立 UDP 中继服务
//
// 为无法 P2P 打通的 Aleiyun 客户端提供数据中转。
// 该程序独立于 controller，部署在带公网 IP 的服务器上即可。
//
// 工作原理：
//   1. 每个客户端向 relay 的 UDP 端口发送注册包，上报自己的 WireGuard 公钥。
//   2. relay 记录 "公钥 -> 客户端 UDP 地址" 映射。
//   3. 客户端把 peer 的 endpoint 配置为 relay 的公网地址。
//   4. relay 收到 WireGuard 数据包后，读取前 32 字节 receiver 公钥，
//      查表转发到对应客户端的 UDP 地址。
//
// WireGuard 数据包格式：前 32 字节为接收方公钥，relay 据此转发。
//
// 命令行：
//   ./aleiyun_relay -udp :3478 -http :8081 -timeout 5m -log relay.log
//
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// WireGuard 数据包头部长度：类型(1) + 预留(3) + receiver pub key(32) = 36
	// 实际上只需要前 32 字节的 receiver key，这里用 32。
	wgReceiverKeyLen = 32
	// 注册包前缀
	registerPrefix = "REGISTER:"
	// 心跳包前缀
	heartbeatPrefix = "HEARTBEAT:"
	// 默认向 controller 上报心跳周期
	defaultControllerBeatInterval = 30 * time.Second
)

// Client 记录一个中继客户端
// 注意：公钥是 WireGuard 的 32 字节原始公钥的 base64 编码
// Endpoint 是 NAT 后的 "ip:port"，由 relay 从 UDP 收包地址学习得到
type Client struct {
	PublicKey   string    `json:"public_key"`
	Endpoint    string    `json:"endpoint"`
	LastSeen    time.Time `json:"last_seen"`
	BytesIn     uint64    `json:"bytes_in"`
	BytesOut    uint64    `json:"bytes_out"`
	PacketsIn   uint64    `json:"packets_in"`
	PacketsOut  uint64    `json:"packets_out"`
}

type Relay struct {
	mu          sync.RWMutex
	clients     map[string]*Client // key: base64(public_key)
	udpAddr     *net.UDPAddr
	publicAddr  string             // 客户端/controller 看到的公网地址
	conn        *net.UDPConn
	timeout     time.Duration
	logFile     *os.File
}

func main() {
	udpAddr := flag.String("udp", ":3478", "UDP 监听地址(客户端 WireGuard 流量)")
	publicAddr := flag.String("public-addr", "", "公网可达地址，客户端/controller 使用，如 1.2.3.4:3478；默认与 -udp 相同")
	httpAddr := flag.String("http", ":8081", "HTTP 管理接口地址，0 表示关闭")
	timeout := flag.Duration("timeout", 5*time.Minute, "客户端注册映射超时时间")
	logPath := flag.String("log", "", "日志文件路径，空则只输出到 stdout")
	controllerURL := flag.String("controller", "", "Controller 心跳上报地址，如 http://121.40.193.74:52888/api/relays/beat")
	controllerID := flag.String("controller-id", "", "Controller 中该 relay 的 ID(空则由 controller 按地址匹配)")
	flag.Parse()

	if *publicAddr == "" {
		*publicAddr = *udpAddr
	}

	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("打开日志文件失败: %v", err)
		}
		defer f.Close()
		log.SetOutput(f)
	}

	relay, err := NewRelay(*udpAddr, *timeout)
	if err != nil {
		log.Fatalf("初始化 relay 失败: %v", err)
	}
	relay.publicAddr = *publicAddr

	if *controllerURL != "" {
		go relay.controllerHeartbeat(*controllerURL, *controllerID)
	}

	if *httpAddr != "0" {
		go relay.serveHTTP(*httpAddr)
	}

	log.Printf("Aleiyun relay 启动: UDP %s, 公网 %s, HTTP %s, 超时 %v", *udpAddr, *publicAddr, *httpAddr, *timeout)
	relay.run()
}

// NewRelay 创建 relay 并监听 UDP
func NewRelay(addr string, timeout time.Duration) (*Relay, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}
	return &Relay{
		clients: make(map[string]*Client),
		udpAddr: udpAddr,
		conn:    conn,
		timeout: timeout,
	}, nil
}

// run 主循环：接收 UDP 包并处理
func (r *Relay) run() {
	buf := make([]byte, 65535)
	for {
		n, clientAddr, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("读取 UDP 错误: %v", err)
			continue
		}
		if n < 1 {
			continue
		}
		data := buf[:n]
		r.handlePacket(data, clientAddr)
	}
}

// handlePacket 处理单个 UDP 包
func (r *Relay) handlePacket(data []byte, from *net.UDPAddr) {
	msg := string(data)

	// 注册包：REGISTER:<base64_pubkey>[:<preferred_endpoint>]
	if strings.HasPrefix(msg, registerPrefix) {
		pubB64, endpoint := parseRelayControlMsg(msg, registerPrefix)
		if pubB64 == "" {
			return
		}
		pubRaw, err := base64.StdEncoding.DecodeString(pubB64)
		if err != nil || len(pubRaw) != 32 {
			log.Printf("[%s] 注册包公钥格式错误", from.String())
			return
		}
		if endpoint == "" {
			endpoint = from.String()
		}
		r.register(pubB64, endpoint)
		return
	}

	// 心跳包：HEARTBEAT:<base64_pubkey>[:<preferred_endpoint>]
	if strings.HasPrefix(msg, heartbeatPrefix) {
		pubB64, endpoint := parseRelayControlMsg(msg, heartbeatPrefix)
		if pubB64 == "" {
			return
		}
		if endpoint == "" {
			endpoint = from.String()
		}
		r.heartbeat(pubB64, endpoint)
		return
	}

	// WireGuard 数据包：前 32 字节是 receiver public key
	if n := len(data); n < wgReceiverKeyLen {
		return
	}
	receiverKey := data[:wgReceiverKeyLen]
	receiverB64 := base64.StdEncoding.EncodeToString(receiverKey)

	r.mu.RLock()
	client := r.clients[receiverB64]
	sender := r.clients[pubkeyFromAddrLocked(r.clients, from)]
	r.mu.RUnlock()

	if client == nil {
		// 未知目标，丢弃
		return
	}

	// 转发给目标客户端
	_, err := r.conn.WriteToUDP(data, stringToUDPAddr(client.Endpoint))
	if err != nil {
		log.Printf("转发到 %s 失败: %v", client.Endpoint, err)
		return
	}

	// 统计
	r.mu.Lock()
	client.LastSeen = time.Now()
	client.PacketsOut++
	client.BytesOut += uint64(len(data))
	if sender != nil {
		sender.LastSeen = time.Now()
		sender.PacketsIn++
		sender.BytesIn += uint64(len(data))
	}
	r.mu.Unlock()
}

// parseRelayControlMsg 解析 REGISTER:/HEARTBEAT: 控制包。
// 格式: PREFIX:<base64_pubkey>[:<preferred_endpoint>]
// preferred_endpoint 为空时，调用方应使用 UDP 源地址。
func parseRelayControlMsg(msg, prefix string) (pubB64, endpoint string) {
	body := strings.TrimPrefix(msg, prefix)
	parts := strings.SplitN(body, ":", 3)
	if len(parts) >= 1 {
		pubB64 = strings.TrimSpace(parts[0])
	}
	if len(parts) >= 3 {
		endpoint = strings.TrimSpace(parts[2])
	}
	return
}

// register 注册或更新客户端映射
func (r *Relay) register(pubB64 string, endpoint string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[pubB64] = &Client{
		PublicKey: pubB64,
		Endpoint:  endpoint,
		LastSeen:  time.Now(),
	}
	log.Printf("[注册] %s -> %s", pubB64[:16]+"...", endpoint)
}

// heartbeat 刷新客户端活跃时间
func (r *Relay) heartbeat(pubB64 string, endpoint string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.clients[pubB64]
	if c == nil {
		// 没有注册过的心跳，直接注册
		r.clients[pubB64] = &Client{
			PublicKey: pubB64,
			Endpoint:  endpoint,
			LastSeen:  time.Now(),
		}
		log.Printf("[心跳注册] %s -> %s", pubB64[:16]+"...", endpoint)
		return
	}
	c.LastSeen = time.Now()
	// 客户端上报的 preferred endpoint 优先级高于 UDP 源地址
	if endpoint != "" {
		c.Endpoint = endpoint
	}
}

// serveHTTP 提供简单的管理接口
func (r *Relay) serveHTTP(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/clients", r.handleClients)
	mux.HandleFunc("/stats", r.handleStats)
	mux.HandleFunc("/health", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// 启动清理协程
	go r.cleaner()

	log.Fatal(http.ListenAndServe(addr, mux))
}

// handleClients 返回当前已注册客户端列表
func (r *Relay) handleClients(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	list := make([]*Client, 0, len(r.clients))
	for _, c := range r.clients {
		list = append(list, c)
	}
	r.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// handleStats 返回简单统计
func (r *Relay) handleStats(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	count := len(r.clients)
	r.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"clients":       count,
		"udp":           r.udpAddr.String(),
		"public_addr":   r.publicAddr,
	})
}

// controllerHeartbeat 定期向 controller 上报 relay 在线状态。
// controller 根据最后一次心跳时间判断 relay 是否在线。
func (r *Relay) controllerHeartbeat(url, id string) {
	if id != "" {
		url = strings.TrimSuffix(url, "/") + "/" + id + "/beat"
	}
	client := &http.Client{Timeout: 15 * time.Second}
	body := map[string]string{"address": r.publicAddr}
	first := true
	for {
		if !first {
			time.Sleep(defaultControllerBeatInterval)
		}
		first = false
		b, err := json.Marshal(body)
		if err != nil {
			continue
		}
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(b)))
		if err != nil {
			log.Printf("[controller] 构造心跳请求失败: %v", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[controller] 心跳上报失败: %v", err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			log.Printf("[controller] 心跳上报返回 %s", resp.Status)
		}
	}
}

// cleaner 定期清理超时未心跳的客户端
func (r *Relay) cleaner() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		r.mu.Lock()
		now := time.Now()
		for k, c := range r.clients {
			if now.Sub(c.LastSeen) > r.timeout {
				log.Printf("[清理] 超时客户端 %s (%s)", k[:16]+"...", c.Endpoint)
				delete(r.clients, k)
			}
		}
		r.mu.Unlock()
	}
}

// pubkeyFromAddrLocked 根据 UDP 地址查找对应公钥（调用方需持有读锁）
func pubkeyFromAddrLocked(clients map[string]*Client, addr *net.UDPAddr) string {
	s := addr.String()
	for k, c := range clients {
		if c.Endpoint == s {
			return k
		}
	}
	return ""
}

// stringToUDPAddr 把 "ip:port" 转成 *net.UDPAddr
func stringToUDPAddr(s string) *net.UDPAddr {
	addr, err := net.ResolveUDPAddr("udp", s)
	if err != nil {
		return nil
	}
	return addr
}
