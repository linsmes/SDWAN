//go:build windows

// Windows 下自动申请管理员权限(UAC)
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// isAdmin 检查当前进程是否已提权(管理员)
func isAdmin() bool {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}

// ensureAdmin 检查当前进程是否已提权,未提权则通过 UAC 以管理员身份重启自己
func ensureAdmin() {
	if isAdmin() {
		return
	}
	log.Printf("当前非管理员权限,正在请求 UAC 提权...")
	if err := relaunchAsAdmin(); err != nil {
		fatal("请求管理员权限失败: %v(请右键\"以管理员身份运行\")", err)
	}
	fmt.Println("已请求管理员权限,请在新窗口中继续")
	os.Exit(0)
}

// relaunchAsAdmin 用 ShellExecute(verb=runas) 以管理员身份重启自己,
// 带上原命令行参数,工作目录设为 exe 目录
func relaunchAsAdmin() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	exePtr, _ := windows.UTF16PtrFromString(exe)
	paramsPtr, _ := windows.UTF16PtrFromString(quoteArgs(os.Args[1:]))
	dirPtr, _ := windows.UTF16PtrFromString(filepath.Dir(exe))
	return windows.ShellExecute(0, verb, exePtr, paramsPtr, dirPtr, windows.SW_SHOWNORMAL)
}

// quoteArgs 拼接命令行参数,含空格的参数加引号
func quoteArgs(args []string) string {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		if strings.ContainsAny(a, " \t\"") {
			b.WriteByte('"')
			b.WriteString(strings.ReplaceAll(a, `"`, `\"`))
			b.WriteByte('"')
		} else {
			b.WriteString(a)
		}
	}
	return b.String()
}
