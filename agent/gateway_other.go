//go:build !windows

// 站点到站点网关模式(Linux 完整实现):开启 IP 转发 + 对隧道到 LAN 的流量做 SNAT。
// 配了 lan 的节点才启用,退出时通过返回的 cleanup 回滚(尽力而为,失败仅日志)。
package main

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"runtime"
	"strings"
)

// setupGateway 开启 IP 转发,并为每个 LAN 网段添加 POSTROUTING MASQUERADE 规则
// (从隧道 ifName 转发到 LAN 出口网卡的流量做 SNAT,省去 LAN 主机的回程路由)。
func setupGateway(ifName string, lanCIDRs []string) (cleanup func()) {
	if runtime.GOOS != "linux" {
		log.Printf("网关模式在 %s 上未实现,请手动开启 IP 转发并配置 SNAT", runtime.GOOS)
		return func() {}
	}

	if err := run("sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		log.Printf("开启 IP 转发失败(需 root 权限): %v", err)
	} else {
		log.Printf("已开启 IP 转发 (net.ipv4.ip_forward=1)")
	}

	// 记录本次新增的规则 (出口网卡),cleanup 时只删自己加的
	var added []string
	for _, cidr := range lanCIDRs {
		lanIf, err := egressIface(cidr)
		if err != nil {
			log.Printf("网段 %s 反查出口网卡失败,跳过 SNAT: %v", cidr, err)
			continue
		}
		dup := false
		for _, a := range added {
			if a == lanIf {
				dup = true
			}
		}
		if dup {
			continue // 同一出口网卡只建一条规则
		}
		rule := []string{"-t", "nat", "POSTROUTING", "-i", ifName, "-o", lanIf, "-j", "MASQUERADE"}
		// 先 -C 检查,避免重复添加
		if err := run("iptables", append([]string{"-C"}, rule...)...); err == nil {
			log.Printf("SNAT 规则已存在: 隧道 -> %s (%s)", lanIf, cidr)
			continue
		}
		if err := run("iptables", append([]string{"-A"}, rule...)...); err != nil {
			log.Printf("添加 SNAT 规则失败(隧道 -> %s): %v", lanIf, err)
			continue
		}
		log.Printf("已添加 SNAT 规则: 隧道 -> %s (%s)", lanIf, cidr)
		added = append(added, lanIf)
	}

	return func() {
		for _, lanIf := range added {
			rule := []string{"-t", "nat", "-D", "POSTROUTING", "-i", ifName, "-o", lanIf, "-j", "MASQUERADE"}
			if err := run("iptables", rule...); err != nil {
				log.Printf("删除 SNAT 规则失败(隧道 -> %s): %v", lanIf, err)
			} else {
				log.Printf("已删除 SNAT 规则: 隧道 -> %s", lanIf)
			}
		}
	}
}

// egressIface 反查到 LAN 网段的出口网卡:ip route get <网段首地址> 解析 dev 字段
func egressIface(cidr string) (string, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}
	first := ipnet.IP.Mask(ipnet.Mask) // 网段首地址(网络号)
	out, err := exec.Command("ip", "route", "get", first.String()).Output()
	if err != nil {
		return "", fmt.Errorf("ip route get %s: %w", first, err)
	}
	// 输出形如 "192.168.1.0 dev eth0 src 192.168.1.2 ..."
	fields := strings.Fields(string(out))
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			return fields[i+1], nil
		}
	}
	return "", fmt.Errorf("无法从 %q 解析出口网卡", strings.TrimSpace(string(out)))
}
