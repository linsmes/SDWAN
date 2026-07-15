// 配置文件(aleiyun_client.json)与致命错误处理
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// fileConfig 对应 exe 同目录的 aleiyun_client.json,字段与命令行参数对应
type fileConfig struct {
	Controller string `json:"controller"`
	Name       string `json:"name"`
	Username   string `json:"username"` // v0.7.0 账号体系:注册凭据,账号在面板创建
	Password   string `json:"password"`
	Key        string `json:"key"`
	ListenPort int    `json:"listen_port"`
	Log        string `json:"log"`
	Stun       string `json:"stun"`
	NoTun      bool   `json:"no_tun"`
	Debug      bool   `json:"debug"`
	LAN        string `json:"lan"`     // 站点到站点:本机 LAN 网段(逗号分隔 CIDR),如 "192.168.1.0/24,10.10.0.0/24"
	Network    string `json:"network"` // v0.7.0 起废弃:所属网络由账号绑定决定,此字段保留读取但不再作为注册依据
}

// stripJSONComments 去除 JSON 中的注释(// 行注释、/* */ 块注释),
// 状态机跳过字符串字面量,不会误伤 "http://..." 这类值。标准 JSON 输入原样通过。
func stripJSONComments(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inStr, esc := false, false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if inStr {
			out = append(out, c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			out = append(out, c)
			continue
		}
		if c == '/' && i+1 < len(data) {
			switch data[i+1] {
			case '/': // 行注释:跳到行尾(保留换行,错误行号不失真)
				for i < len(data) && data[i] != '\n' {
					i++
				}
				if i < len(data) {
					out = append(out, '\n')
				}
				continue
			case '*': // 块注释:跳到 */(保留其中的换行)
				i += 2
				for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
					if data[i] == '\n' {
						out = append(out, '\n')
					}
					i++
				}
				i++ // 跳过 '/'
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// loadConfig 读取配置文件(支持 // 和 /* */ 注释);文件不存在时返回 (nil, nil)
func loadConfig(path string) (*fileConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c fileConfig
	if err := json.Unmarshal(stripJSONComments(data), &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// detectLANCommentLines 探测本机所有 LAN 网段(IPv4、非环回、网卡启用),
// 生成注释掉的 "lan" 配置行,用户取消注释即可,无需手动查 ipconfig
func detectLANCommentLines() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	var b strings.Builder
	seen := map[string]bool{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			ones, bits := ipnet.Mask.Size()
			if bits != 32 || ones == 0 {
				continue
			}
			cidr := fmt.Sprintf("%s/%d", ip.Mask(ipnet.Mask), ones)
			if seen[cidr] {
				continue
			}
			seen[cidr] = true
			fmt.Fprintf(&b, "  // \"lan\": \"%s\",  ← 网卡 %s\n", cidr, iface.Name)
		}
	}
	return b.String()
}

// writeDefaultConfig 生成默认配置文件(带注释,controller 留空,要求用户编辑;
// 自动探测本机 LAN 网段写成注释行,需要共享哪个取消注释即可)
func writeDefaultConfig(path, hostname string) error {
	lanHints := detectLANCommentLines()
	if lanHints != "" {
		lanHints = "  // 本机探测到的网段如下,需要共享哪个就取消注释对应行(可多行累加):\n" + lanHints
	}
	tpl := `{
  // server(controller)地址,必填。支持裸 IP/裸域名,端口可省略
  // 例:121.40.193.74  example.com  https://sdwan.example.com
  "controller": "",

  // 注册账号/密码(v0.7.0 起必填,在 Web 面板"账号"页创建;
  // 所属网络由账号绑定的公司决定,无需在此选择)
  "username": "",
  "password": "",

  // 设备名称,显示在面板设备列表;留空自动使用主机名
  "name": "%s",

  // 站点到站点:共享给对端网络的本机 LAN 网段(逗号分隔 CIDR),不共享留空。
%s  "lan": "",

  // 私钥文件(首次运行自动生成;删除即换身份,面板会分配新虚拟 IP)
  "key": "aleiyun_client.key",
  // WireGuard 监听端口
  "listen_port": 51820,
  // 日志文件(屏幕和文件同时输出)
  "log": "aleiyun_client.log"
}
`
	return os.WriteFile(path, []byte(fmt.Sprintf(tpl, hostname, lanHints)), 0o644)
}

// normalizeController 补全 controller 地址:允许裸 IP/裸域名(可带端口、可带路径),
// 无 scheme 补 http://;缺端口时 https 补 :443(HTTPS 惯例,通常走反代),其余补 :52888
func normalizeController(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return addr
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil || u.Host == "" {
		return addr
	}
	if u.Port() == "" {
		if strings.EqualFold(u.Scheme, "https") {
			u.Host += ":443"
		} else {
			u.Host += ":52888"
		}
	}
	return u.String()
}

// resolvePath 相对路径统一解析为相对于 exe 目录(双击运行时工作目录不可靠)
func resolvePath(exeDir, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(exeDir, p)
}

// fatal 打印致命错误后退出;Windows 终端下暂停等回车,方便双击运行时看清报错
func fatal(format string, args ...any) {
	log.Printf(format, args...)
	pauseAndExit(1)
}

// pauseAndExit Windows 且 stdin 是终端时等待回车再退出
func pauseAndExit(code int) {
	if runtime.GOOS == "windows" {
		if st, err := os.Stdin.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
			fmt.Print("按回车键退出...")
			bufio.NewReader(os.Stdin).ReadString('\n')
		}
	}
	os.Exit(code)
}
