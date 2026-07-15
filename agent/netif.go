package main

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"runtime"
)

// assignInterfaceIP 给虚拟网卡配置 IP 地址(调用系统命令,需要管理员/root)
func assignInterfaceIP(ifName, ip, cidr string) error {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	ones, _ := ipnet.Mask.Size()

	switch runtime.GOOS {
	case "windows":
		return assignInterfaceIPWindows(ifName, ip, ones)
	case "linux":
		if err := run("ip", "addr", "replace", fmt.Sprintf("%s/%d", ip, ones), "dev", ifName); err != nil {
			return err
		}
		return run("ip", "link", "set", ifName, "up")
	case "darwin":
		mask := net.IP(ipnet.Mask).String()
		return run("ifconfig", ifName, "inet", ip, ip, "netmask", mask, "up")
	default:
		return fmt.Errorf("暂不支持的平台: %s", runtime.GOOS)
	}
}

// assignInterfaceIPWindows 用 PowerShell 先禁用 DHCP、清理旧 IP,再设置静态地址,
// 避免 wintun 被 Windows 自动分配 169.254.x.x 的 APIPA 地址或保留旧地址。
func assignInterfaceIPWindows(ifName, ip string, ones int) error {
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$ifAlias = '%s'
$ipAddr = '%s'
$prefix = %d

# 1. 禁用 DHCP,阻止系统继续争夺地址
Set-NetIPInterface -InterfaceAlias $ifAlias -Dhcp Disabled -ErrorAction SilentlyContinue

# 2. 删除该接口上所有现有 IPv4 单播地址(含 APIPA/旧静态地址)
Get-NetIPAddress -InterfaceAlias $ifAlias -AddressFamily IPv4 -ErrorAction SilentlyContinue |
    Remove-NetIPAddress -Confirm:$false -ErrorAction SilentlyContinue

# 3. 添加目标静态地址
New-NetIPAddress -InterfaceAlias $ifAlias -IPAddress $ipAddr -PrefixLength $prefix

	# 4. 限制网卡 MTU,防止 Windows 上层发大包导致 wireguard-go 的 RIO 发送缓冲区溢出
	Set-NetIPInterface -InterfaceAlias $ifAlias -NlMtuBytes 1420 -ErrorAction SilentlyContinue
`, ifName, ip, ones)
	return run("powershell", "-NoProfile", "-Command", script)
}

// allowInboundUDP 放行 WireGuard 监听端口的入站 UDP
// (Windows 防火墙默认拦截入站连接,不加规则会导致握手失败)
func allowInboundUDP(port int) error {
	switch runtime.GOOS {
	case "windows":
		rule := fmt.Sprintf("Aleiyun Client UDP %d", port)
		// 先删后加,避免重复规则堆积
		_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+rule).Run()
		return run("netsh", "advfirewall", "firewall", "add", "rule",
			"name="+rule, "dir=in", "action=allow", "protocol=UDP",
			fmt.Sprintf("localport=%d", port))
	default:
		// Linux/macOS 桌面环境通常无入站限制,服务器环境请自行放行
		return nil
	}
}

// allowTunnelInbound 放行隧道网卡的全部入站流量:
// 虚拟局域网内视为可信网络(与主流 VPN 客户端行为一致),
// 否则解密后的业务流量(ICMP/TCP/UDP)会被主机防火墙拦截
func allowTunnelInbound(ifName string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	const rule = "Aleiyun Tunnel Inbound"
	script := fmt.Sprintf(
		`Remove-NetFirewallRule -DisplayName '%s' -ErrorAction SilentlyContinue;`+
			`New-NetFirewallRule -DisplayName '%s' -Direction Inbound -Action Allow -InterfaceAlias '%s' | Out-Null`,
		rule, rule, ifName)
	return run("powershell", "-NoProfile", "-Command", script)
}

// installedRoutes 记录 syncRoutes 已安装的 LAN 路由(目标网段集合),用于增量 diff 和退出清理
var installedRoutes = map[string]bool{}

// syncRoutes 路由同步:目标集合 = 所有 peer 通告的 LAN 网段 ∪ 互联规则路由(extra),
// 与已安装集合做 diff——新增的加路由、消失的删路由。跳过格式非法的网段。
// 失败只记日志,不影响隧道本身。
func syncRoutes(ifName string, peers []peerView, extra []string) {
	want := map[string]bool{}
	for _, p := range peers {
		for _, c := range p.LANSubnets {
			if _, _, err := net.ParseCIDR(c); err != nil {
				log.Printf("peer %s 的网段 %q 格式非法,跳过路由安装", p.Name, c)
				continue
			}
			want[c] = true
		}
	}
	for _, c := range extra {
		if _, _, err := net.ParseCIDR(c); err != nil {
			log.Printf("互联规则路由 %q 格式非法,跳过路由安装", c)
			continue
		}
		want[c] = true
	}
	for cidr := range want {
		if installedRoutes[cidr] {
			continue
		}
		if err := addRoute(ifName, cidr); err != nil {
			log.Printf("添加内网路由 %s 失败: %v", cidr, err)
		} else {
			installedRoutes[cidr] = true
			log.Printf("已添加内网路由: %s via %s", cidr, ifName)
		}
	}
	for cidr := range installedRoutes {
		if want[cidr] {
			continue
		}
		if err := delRoute(ifName, cidr); err != nil {
			log.Printf("删除内网路由 %s 失败: %v", cidr, err)
		} else {
			delete(installedRoutes, cidr)
			log.Printf("已删除内网路由: %s", cidr)
		}
	}
}

// cleanupRoutes 退出时清理 syncRoutes 安装的全部路由(尽力而为,失败仅日志)
func cleanupRoutes(ifName string) {
	for cidr := range installedRoutes {
		if err := delRoute(ifName, cidr); err != nil {
			log.Printf("清理内网路由 %s 失败: %v", cidr, err)
		} else {
			delete(installedRoutes, cidr)
		}
	}
}

func addRoute(ifName, cidr string) error {
	switch runtime.GOOS {
	case "windows":
		// Windows 隧道接口的 on-link 路由需要显式指定下一跳 0.0.0.0，
		// 否则 New-NetRoute 可能因缺少网关而失败。先尝试删除已存在路由避免冲突。
		_ = exec.Command("powershell", "-NoProfile", "-Command", fmt.Sprintf(
			"Remove-NetRoute -DestinationPrefix '%s' -InterfaceAlias '%s' -Confirm:$false -ErrorAction SilentlyContinue",
			cidr, ifName)).Run()
		return run("powershell", "-NoProfile", "-Command", fmt.Sprintf(
			"New-NetRoute -DestinationPrefix '%s' -InterfaceAlias '%s' -NextHop 0.0.0.0 -ErrorAction SilentlyContinue",
			cidr, ifName))
	case "linux":
		return run("ip", "route", "replace", cidr, "dev", ifName)
	default:
		return fmt.Errorf("暂不支持的平台: %s", runtime.GOOS)
	}
}

func delRoute(ifName, cidr string) error {
	switch runtime.GOOS {
	case "windows":
		return run("powershell", "-NoProfile", "-Command", fmt.Sprintf(
			"Remove-NetRoute -DestinationPrefix '%s' -InterfaceAlias '%s' -Confirm:$false -ErrorAction SilentlyContinue",
			cidr, ifName))
	case "linux":
		return run("ip", "route", "del", cidr, "dev", ifName)
	default:
		return fmt.Errorf("暂不支持的平台: %s", runtime.GOOS)
	}
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %v: %w\n%s", name, args, err, out)
	}
	return nil
}
