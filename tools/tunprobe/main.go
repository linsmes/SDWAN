// TUN 层探针:创建一个虚拟网卡,打印所有进入它的 IP 包。
// 用于验证 "应用 → 路由 → Wintun → 用户态程序" 这条链路是否正常,
// 把 WireGuard 协议层完全排除在外。
//
// 用法: tunprobe  (管理员权限)
// 然后另开终端: ping 10.99.0.2
// 如果探针打印出 ICMP 包,说明 TUN 链路正常。
package main

import (
	"fmt"
	"log"
	"os/exec"

	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

func main() {
	t, err := tun.CreateTUN("tunprobe", device.DefaultMTU)
	if err != nil {
		log.Fatalf("创建 TUN 失败: %v", err)
	}
	defer t.Close()
	name, _ := t.Name()

	out, err := exec.Command("netsh", "interface", "ip", "set", "address",
		"name="+name, "static", "10.99.0.1", "255.255.255.0").CombinedOutput()
	if err != nil {
		log.Fatalf("配置 IP 失败: %v\n%s", err, out)
	}

	log.Printf("探针网卡 %s 就绪 (10.99.0.1/24)", name)
	log.Printf("请另开终端执行: ping 10.99.0.2  (Ctrl+C 退出)")

	bufs := [][]byte{make([]byte, 2048)}
	sizes := make([]int, 1)
	for {
		if _, err := t.Read(bufs, sizes, 0); err != nil {
			log.Printf("读取结束: %v", err)
			return
		}
		pkt := bufs[0][:sizes[0]]
		if len(pkt) < 20 || pkt[0]>>4 != 4 {
			continue
		}
		proto := pkt[9]
		src := fmt.Sprintf("%d.%d.%d.%d", pkt[12], pkt[13], pkt[14], pkt[15])
		dst := fmt.Sprintf("%d.%d.%d.%d", pkt[16], pkt[17], pkt[18], pkt[19])
		protoName := map[byte]string{1: "ICMP", 6: "TCP", 17: "UDP"}[proto]
		if protoName == "" {
			protoName = fmt.Sprintf("proto=%d", proto)
		}
		log.Printf("收到 IP 包: %s -> %s %s %d 字节", src, dst, protoName, len(pkt))
	}
}
