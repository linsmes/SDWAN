// MAC 地址采集与机器码(设备指纹)
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"sort"
	"strings"
)

// virtualMACPrefixes 常见虚拟网卡/虚拟机 MAC 前缀,采集机器指纹时过滤,
// 避免一台物理机上的 VMware/VirtualBox/Hyper-V 虚拟网卡把指纹变乱。
var virtualMACPrefixes = []string{
	"00:50:56", // VMware
	"00:0c:29", // VMware
	"00:05:69", // VMware
	"00:1c:14", // VMware
	"08:00:27", // VirtualBox
	"00:15:5d", // Hyper-V
	"52:54:00", // QEMU/KVM
	"02:42:",   // Docker
}

// isVirtualMAC 判断 MAC 是否属于虚拟网卡/虚拟机
func isVirtualMAC(mac string) bool {
	m := strings.ToLower(mac)
	for _, p := range virtualMACPrefixes {
		if strings.HasPrefix(m, p) {
			return true
		}
	}
	return false
}

// machineID 根据 MAC 列表生成固定机器码:过滤虚拟 MAC 后排序,SHA256 取前 16 位十六进制。
// 如果过滤后为空,则回退到全部 MAC,保证至少有一个指纹。
func machineID(macs []string) string {
	filtered := make([]string, 0, len(macs))
	for _, m := range macs {
		if m != "" && !isVirtualMAC(m) {
			filtered = append(filtered, strings.ToLower(m))
		}
	}
	if len(filtered) == 0 {
		for _, m := range macs {
			if m != "" {
				filtered = append(filtered, strings.ToLower(m))
			}
		}
	}
	sort.Strings(filtered)
	h := sha256.Sum256([]byte(strings.Join(filtered, "|")))
	return hex.EncodeToString(h[:])[:16]
}

// macAddrs 采集所有启用、非环回网卡的 MAC 地址(小写冒号分隔),
// 用于设备识别与封禁。虚拟网卡/本机回环自动跳过;采集失败返回 nil。
func macAddrs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var macs []string
	seen := map[string]bool{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if len(iface.HardwareAddr) == 0 {
			continue
		}
		// 跳过本软件创建的虚拟网卡,避免把自身 TUN 当成机器指纹
		if iface.Name == "aleiyun_sdwan0" || strings.HasPrefix(iface.Name, "wintun") {
			continue
		}
		m := iface.HardwareAddr.String()
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		macs = append(macs, m)
	}
	return macs
}
