package main

import (
	"encoding/binary"
	"log"
	"net"
)

// stunMagicCookie STUN 协议魔数(RFC 5389),agent 侧有相同定义
const stunMagicCookie = 0x2112A442

// serveSTUN 极简 STUN Binding 服务端:
// 收到 Binding Request 后回 XOR-MAPPED-ADDRESS(请求方的公网映射地址),
// 供 agent 发现自己在 NAT 后面的公网 IP:端口。
func serveSTUN(conn *net.UDPConn) {
	buf := make([]byte, 1500)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		// 只认 Binding Request(0x0001)+ 正确魔数
		if n < 20 ||
			binary.BigEndian.Uint16(buf[0:2]) != 0x0001 ||
			binary.BigEndian.Uint32(buf[4:8]) != stunMagicCookie {
			continue
		}
		resp := stunBindingResponse(buf[8:20], addr)
		if resp == nil {
			continue
		}
		if _, err := conn.WriteToUDP(resp, addr); err == nil {
			log.Printf("[stun] %s 探测公网映射", addr)
		}
	}
}

// stunBindingResponse 构造 Binding Response,仅含 XOR-MAPPED-ADDRESS 属性(IPv4)
func stunBindingResponse(txid []byte, addr *net.UDPAddr) []byte {
	ip4 := addr.IP.To4()
	if ip4 == nil {
		return nil // 暂只支持 IPv4
	}
	msg := make([]byte, 20+12)
	binary.BigEndian.PutUint16(msg[0:2], 0x0101) // Binding Response
	binary.BigEndian.PutUint16(msg[2:4], 12)     // 属性区长度
	binary.BigEndian.PutUint32(msg[4:8], stunMagicCookie)
	copy(msg[8:20], txid)

	attr := msg[20:]
	binary.BigEndian.PutUint16(attr[0:2], 0x0020) // XOR-MAPPED-ADDRESS
	binary.BigEndian.PutUint16(attr[2:4], 8)
	attr[5] = 0x01 // family: IPv4
	binary.BigEndian.PutUint16(attr[6:8], uint16(addr.Port)^uint16(stunMagicCookie>>16))
	binary.BigEndian.PutUint32(attr[8:12], binary.BigEndian.Uint32(ip4)^stunMagicCookie)
	return msg
}
