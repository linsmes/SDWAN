// 端到端延迟测试:经隧道 ICMP ping peer 的虚拟 IP,测量 RTT。
//
// 使用原始 ICMP 套接字,需要管理员/root 权限(agent 起隧道本来就需要)。
package main

import (
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// PingRTT 向 ip 发送 count 个 ICMP echo(每个超时 timeout),
// 返回成功包的平均 RTT(毫秒);全部失败或监听失败(如权限不足)时 ok=false。
func PingRTT(ip string, count int, timeout time.Duration) (avgMs float64, ok bool) {
	c, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return 0, false
	}
	defer c.Close()

	dst, err := net.ResolveIPAddr("ip4", ip)
	if err != nil {
		return 0, false
	}

	id := os.Getpid() & 0xffff
	var total time.Duration
	succ := 0
	for seq := 1; seq <= count; seq++ {
		msg := icmp.Message{
			Type: ipv4.ICMPTypeEcho, Code: 0,
			Body: &icmp.Echo{ID: id, Seq: seq, Data: []byte("sdwan-ping")},
		}
		wb, err := msg.Marshal(nil)
		if err != nil {
			continue
		}
		start := time.Now()
		if _, err := c.WriteTo(wb, dst); err != nil {
			continue
		}
		_ = c.SetReadDeadline(start.Add(timeout))
		for {
			rb := make([]byte, 1500)
			n, _, err := c.ReadFrom(rb)
			if err != nil {
				break // 超时或读失败,该包视为丢失
			}
			reply, err := icmp.ParseMessage(ipv4.ICMPTypeEchoReply.Protocol(), rb[:n])
			if err != nil {
				continue
			}
			if echo, match := reply.Body.(*icmp.Echo); match &&
				reply.Type == ipv4.ICMPTypeEchoReply &&
				echo.ID == id && echo.Seq == seq {
				total += time.Since(start)
				succ++
				break
			}
		}
	}
	if succ == 0 {
		return 0, false
	}
	return total.Seconds() * 1000 / float64(succ), true
}
