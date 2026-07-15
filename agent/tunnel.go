package main

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

// Tunnel 包装 wireguard-go:TUN 网卡 + Device + UAPI 配置通道
type Tunnel struct {
	dev     *device.Device
	tunName string
}

func startTunnel(ifName string, verbose bool) (*Tunnel, error) {
	logLevel := device.LogLevelError
	if verbose {
		logLevel = device.LogLevelVerbose
	}
	logger := newLogger(logLevel)

	tunDev, err := tun.CreateTUN(ifName, device.DefaultMTU)
	if err != nil {
		return nil, fmt.Errorf("创建 TUN 设备失败: %w", err)
	}

	realName, err := tunDev.Name()
	if err != nil {
		tunDev.Close()
		return nil, err
	}

	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	// 显式拉起设备(官方 main_windows.go 的做法):
	// Wintun 不会发送 EventUp 事件,不显式 Up 会导致 peer 永不启动、
	// 设备能收 TUN 包但永远不发起握手
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("启动设备失败: %w", err)
	}

	// UAPI 监听(等价于 wg 工具配置通道),仅用于外部 wg 命令查看状态,
	// 不是隧道的必需组件——某些权限环境下创建命名管道会失败,降级为警告
	uapiListener, err := openUAPI(realName)
	if err != nil {
		log.Printf("UAPI 监听不可用(不影响隧道通信): %v", err)
	} else {
		go func() {
			for {
				c, err := uapiListener.Accept()
				if err != nil {
					return
				}
				go dev.IpcHandle(c)
			}
		}()
	}

	return &Tunnel{dev: dev, tunName: realName}, nil
}

func (t *Tunnel) Name() string { return t.tunName }

// newLogger 构造走默认 log 输出的 Logger(替代 device.NewLogger,
// 后者写死 os.Stdout,无法跟随日志重定向到文件)
func newLogger(level int) *device.Logger {
	logger := &device.Logger{Verbosef: device.DiscardLogf, Errorf: device.DiscardLogf}
	if level >= device.LogLevelVerbose {
		logger.Verbosef = func(f string, a ...any) { log.Printf("DEBUG: sdwan: "+f, a...) }
	}
	if level >= device.LogLevelError {
		logger.Errorf = func(f string, a ...any) { log.Printf("ERROR: sdwan: "+f, a...) }
	}
	return logger
}

// Configure 通过 UAPI 下发配置(与 wg set 等价)
func (t *Tunnel) Configure(uapiConfig string) error {
	return t.dev.IpcSet(uapiConfig)
}

func (t *Tunnel) Close() {
	t.dev.Close()
}

// Rebind 关闭并重开 WireGuard 的 UDP 套接字(网络切换/公网 IP 变化后恢复连接用)。
// 底层 BindUpdate 专为网络切换设计,设备需处于 Up 状态(其内部会判断)。
func (t *Tunnel) Rebind() error {
	return t.dev.BindUpdate()
}

// PeerStats 读取每个 peer 的握手状态:hex 公钥 -> 最近握手的 Unix 秒(0 = 从未握手)
func (t *Tunnel) PeerStats() (map[string]int64, error) {
	var buf bytes.Buffer
	if err := t.dev.IpcGetOperation(&buf); err != nil {
		return nil, err
	}
	stats := map[string]int64{}
	var curPeer string
	for _, line := range strings.Split(buf.String(), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "public_key":
			curPeer = v
		case "last_handshake_time_sec":
			if curPeer != "" {
				n, _ := strconv.ParseInt(v, 10, 64)
				stats[curPeer] = n
			}
		}
	}
	return stats, nil
}

// buildUAPIConfig 生成 UAPI 配置文本:本机私钥 + 监听端口 + 全量 peer
func buildUAPIConfig(privKeyB64 string, listenPort int, peers []peerView) (string, error) {
	hexPriv, err := b64ToHex(privKeyB64)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\n", hexPriv)
	fmt.Fprintf(&b, "listen_port=%d\n", listenPort)
	b.WriteString("replace_peers=true\n")
	for _, p := range peers {
		hexPub, err := b64ToHex(p.PubKey)
		if err != nil {
			return "", fmt.Errorf("peer %s 公钥无效: %w", p.Name, err)
		}
		fmt.Fprintf(&b, "public_key=%s\n", hexPub)
		if p.Endpoint != "" {
			fmt.Fprintf(&b, "endpoint=%s\n", p.Endpoint)
		}
		// 放行该 peer 的虚拟 IP 及其通告的 LAN 网段(站点到站点路由)
		fmt.Fprintf(&b, "allowed_ip=%s/32\n", p.VirtualIP)
		for _, cidr := range p.LANSubnets {
			fmt.Fprintf(&b, "allowed_ip=%s\n", cidr)
		}
		b.WriteString("persistent_keepalive_interval=25\n")
	}
	return b.String(), nil
}
