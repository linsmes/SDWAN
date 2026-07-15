//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed wintun.dll
var wintunDLL []byte

// ensureWintun 把内嵌的 wintun.dll 释放到 agent.exe 同目录(已存在则跳过)
func ensureWintun() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dst := filepath.Join(filepath.Dir(exe), "wintun.dll")
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	if err := os.WriteFile(dst, wintunDLL, 0o644); err != nil {
		return fmt.Errorf("释放 wintun.dll 失败: %w", err)
	}
	return nil
}
