package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/curve25519"
)

// KeyPair WireGuard 密钥对,base64 编码(与 wg 工具兼容)
type KeyPair struct {
	Private string
	Public  string
}

// loadOrCreateKeyPair 从文件加载私钥,不存在则生成新的
func loadOrCreateKeyPair(path string) (*KeyPair, error) {
	if data, err := os.ReadFile(path); err == nil {
		priv := strings.TrimSpace(string(data))
		pub, err := publicFromPrivate(priv)
		if err != nil {
			return nil, fmt.Errorf("私钥文件 %s 内容无效: %w", path, err)
		}
		return &KeyPair{Private: priv, Public: pub}, nil
	}

	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return nil, err
	}
	// Curve25519 clamping
	priv[0] &= 248
	priv[31] = (priv[31] & 127) | 64

	privB64 := base64.StdEncoding.EncodeToString(priv[:])
	pub, err := publicFromPrivate(privB64)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(privB64), 0o600); err != nil {
		return nil, err
	}
	return &KeyPair{Private: privB64, Public: pub}, nil
}

func publicFromPrivate(privB64 string) (string, error) {
	priv, err := base64.StdEncoding.DecodeString(privB64)
	if err != nil {
		return "", err
	}
	if len(priv) != 32 {
		return "", fmt.Errorf("私钥长度应为 32 字节,实际 %d", len(priv))
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pub), nil
}

func b64ToHex(s string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
