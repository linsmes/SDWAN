//go:build !windows

// 非 Windows 平台:无 UAC,仅提示权限
package main

import (
	"log"
	"os"
)

// isAdmin 检查当前进程是否为 root
func isAdmin() bool {
	return os.Geteuid() == 0
}

// ensureAdmin 非 root 时给出警告(创建虚拟网卡需要 root)
func ensureAdmin() {
	if !isAdmin() {
		log.Printf("警告: 当前非 root 用户,创建虚拟网卡可能失败,建议用 sudo 运行")
	}
}
