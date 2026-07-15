// 日志文件:日志同时写屏幕和文件
package main

import (
	"io"
	"log"
	"os"
)

// setupLogFile 日志双写:屏幕 + 文件;文件打开失败时降级为只写屏幕
func setupLogFile(logPath string) {
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(os.Stdout, f))
	} else {
		log.Printf("日志文件 %s 打开失败,只输出到屏幕: %v", logPath, err)
	}
}
