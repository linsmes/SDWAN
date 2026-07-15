//go:build windows

// 站点到站点网关模式(Windows 尽力而为):
//   - 开启 IP 转发:注册表 IPEnableRouter=1(重启后永久生效)+ 各 LAN 网卡即时开启 Forwarding
//   - SNAT:Windows NetNat 按源前缀 NAT,无法表达"隧道到 LAN"的回程转换,
//     此处不硬拼不可用的命令,降级为告警日志,请手动配置 NAT 或在 LAN 主机加回程路由
package main

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func setupGateway(ifName string, lanCIDRs []string) (cleanup func()) {
	// 1. 注册表开启 IP 转发(重启后永久生效)
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Services\Tcpip\Parameters`, registry.SET_VALUE)
	if err != nil {
		log.Printf("开启 IP 转发失败(写注册表,需管理员权限): %v", err)
	} else {
		if err := k.SetDWordValue("IPEnableRouter", 1); err != nil {
			log.Printf("写 IPEnableRouter 失败: %v", err)
		} else {
			log.Printf("已设置 IPEnableRouter=1(重启后永久生效)")
		}
		k.Close()
	}

	// 2. 各 LAN 出口网卡即时开启转发(无需重启)
	seen := map[string]bool{}
	for _, cidr := range lanCIDRs {
		for _, lanIf := range windowsLANIfaces(cidr) {
			if seen[lanIf] {
				continue
			}
			seen[lanIf] = true
			script := fmt.Sprintf(
				"Set-NetIPInterface -InterfaceAlias '%s' -Forwarding Enabled -ErrorAction SilentlyContinue", lanIf)
			if err := run("powershell", "-NoProfile", "-Command", script); err != nil {
				log.Printf("网卡 %s 开启转发失败: %v", lanIf, err)
			} else {
				log.Printf("网卡 %s 已开启转发 (Forwarding Enabled)", lanIf)
			}
		}
	}

	// 3. SNAT 无法可靠表达,降级为告警(详见 README「站点到站点」章节)
	log.Printf("Windows SNAT 未配置,请手动配置 NAT 或在 LAN 主机加回程路由(经隧道回程的流量可能不通)")

	return func() {} // 未创建需回滚的资源,路由清理由 netif 侧负责
}

// windowsLANIfaces 反查到 LAN 网段的出口网卡:Find-NetRoute -RemoteIPAddress <网段首地址>
func windowsLANIfaces(cidr string) []string {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Printf("网段 %q 格式非法,跳过出口网卡查询", cidr)
		return nil
	}
	first := ipnet.IP.Mask(ipnet.Mask)
	script := fmt.Sprintf(
		"Find-NetRoute -RemoteIPAddress '%s' -ErrorAction SilentlyContinue | Select-Object -ExpandProperty InterfaceAlias",
		first.String())
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).Output()
	if err != nil {
		log.Printf("网段 %s 反查出口网卡失败: %v", cidr, err)
		return nil
	}
	var ifaces []string
	for _, line := range strings.Split(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			ifaces = append(ifaces, name)
		}
	}
	if len(ifaces) == 0 {
		log.Printf("网段 %s 未查到出口网卡", cidr)
	}
	return ifaces
}
