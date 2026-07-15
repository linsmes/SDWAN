package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"
)

// stunMagicCookie 与 controller 侧定义一致(RFC 5389)
const stunMagicCookie = 0x2112A442

// stunProbe 从本地端口 localPort 向 STUN 服务器发 Binding 请求,
// 返回 NAT 映射后的公网地址 "ip:port"。
// 必须在 WireGuard 绑定监听端口之前调用(NAT 映射按源端口分配,
// 用同一个端口探测,后续隧道流量才能复用该映射)。
func stunProbe(server string, localPort int) (string, error) {
	raddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return "", err
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: localPort})
	if err != nil {
		// 运行期重探测时该端口已被隧道占用,退回随机端口;
		// NAT 映射按源端口分配,此时探测到的公网端口仅供参考
		log.Printf("绑定本地端口 %d 失败(%v),改用随机端口探测", localPort, err)
		conn, err = net.ListenUDP("udp", nil)
		if err != nil {
			return "", fmt.Errorf("绑定本地端口 %d 失败: %w", localPort, err)
		}
	}
	defer conn.Close()

	var txid [12]byte
	if _, err := rand.Read(txid[:]); err != nil {
		return "", err
	}
	req := make([]byte, 20)
	binary.BigEndian.PutUint16(req[0:2], 0x0001) // Binding Request
	binary.BigEndian.PutUint32(req[4:8], stunMagicCookie)
	copy(req[8:20], txid[:])

	for attempt := 0; attempt < 3; attempt++ {
		if _, err := conn.WriteToUDP(req, raddr); err != nil {
			return "", err
		}
		_ = conn.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
		buf := make([]byte, 1500)
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue // 超时重试
		}
		if pub, err := parseStunResponse(buf[:n], txid); err == nil {
			return pub, nil
		}
	}
	return "", fmt.Errorf("STUN 服务器 %s 无响应", server)
}

// parseStunResponse 解析 Binding Response,提取 XOR-MAPPED-ADDRESS(兼容 MAPPED-ADDRESS)
func parseStunResponse(msg []byte, txid [12]byte) (string, error) {
	if len(msg) < 20 ||
		binary.BigEndian.Uint16(msg[0:2]) != 0x0101 ||
		binary.BigEndian.Uint32(msg[4:8]) != stunMagicCookie ||
		string(msg[8:20]) != string(txid[:]) {
		return "", fmt.Errorf("非法 STUN 响应")
	}
	total := 20 + int(binary.BigEndian.Uint16(msg[2:4]))
	if total > len(msg) {
		total = len(msg)
	}
	for off := 20; off+4 <= total; {
		atype := binary.BigEndian.Uint16(msg[off : off+2])
		alen := int(binary.BigEndian.Uint16(msg[off+2 : off+4]))
		val := msg[off+4:]
		if len(val) < alen {
			break
		}
		switch {
		case atype == 0x0020 && alen >= 8 && val[1] == 0x01: // XOR-MAPPED-ADDRESS, IPv4
			port := binary.BigEndian.Uint16(val[2:4]) ^ uint16(stunMagicCookie>>16)
			ip := binary.BigEndian.Uint32(val[4:8]) ^ stunMagicCookie
			return fmt.Sprintf("%d.%d.%d.%d:%d",
				byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip), port), nil
		case atype == 0x0001 && alen >= 8 && val[1] == 0x01: // MAPPED-ADDRESS 兜底
			return fmt.Sprintf("%d.%d.%d.%d:%d",
				val[4], val[5], val[6], val[7],
				binary.BigEndian.Uint16(val[2:4])), nil
		}
		off += 4 + (alen+3)&^3 // 属性按 4 字节对齐
	}
	return "", fmt.Errorf("响应中无映射地址属性")
}

// isPrivateHost 判断主机名/IP 是否为内网或回环地址
// (controller 在局域网时 STUN 无意义,应跳过)
func isPrivateHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	ips, err := net.LookupHost(host)
	if err != nil {
		return false
	}
	for _, s := range ips {
		ip := net.ParseIP(s)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() {
			return true
		}
	}
	return false
}
