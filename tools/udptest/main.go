// UDP 连通性测试工具:判断两台机器之间 UDP 是否可达(排除 ICMP 干扰)
//
// 服务端: udptest -listen :51821
// 客户端: udptest -to 192.168.1.46:51821
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"time"
)

func main() {
	listen := flag.String("listen", "", "监听地址(服务端模式),如 :51821")
	to := flag.String("to", "", "目标地址(客户端模式),如 192.168.1.46:51821")
	flag.Parse()

	if *listen == "" && *to == "" {
		flag.Usage()
		return
	}

	if *listen != "" {
		addr, err := net.ResolveUDPAddr("udp", *listen)
		if err != nil {
			log.Fatal(err)
		}
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("UDP echo 服务端监听 %s,等待数据...", conn.LocalAddr())
		buf := make([]byte, 1500)
		for {
			n, peer, err := conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}
			log.Printf("收到来自 %s 的 %d 字节: %q", peer, n, buf[:n])
			_, _ = conn.WriteToUDP(append([]byte("echo:"), buf[:n]...), peer)
		}
	}

	if *to != "" {
		conn, err := net.Dial("udp", *to)
		if err != nil {
			log.Fatal(err)
		}
		defer conn.Close()
		log.Printf("向 %s 发送 4 个探测包...", *to)
		ok := 0
		for i := 1; i <= 4; i++ {
			msg := fmt.Sprintf("probe %d", i)
			start := time.Now()
			if _, err := conn.Write([]byte(msg)); err != nil {
				log.Printf("probe %d: 发送失败: %v", i, err)
				continue
			}
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			buf := make([]byte, 1500)
			n, err := conn.Read(buf)
			if err != nil {
				log.Printf("probe %d: 超时无回应", i)
			} else {
				ok++
				log.Printf("probe %d: 收到回应 %q (RTT %v)", i, buf[:n], time.Since(start).Round(time.Millisecond))
			}
			time.Sleep(time.Second)
		}
		log.Printf("完成: %d/4 收到回应", ok)
		if ok == 0 {
			log.Printf("结论: UDP 不通——检查对端 udptest 是否在运行、防火墙弹窗是否允许、路由器是否开启 AP 隔离")
		} else {
			log.Printf("结论: UDP 双向可达")
		}
	}
}
