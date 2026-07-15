package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripJSONComments(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"无注释", `{"a": 1}`, `{"a": 1}`},
		{"行注释", "{\n// 注释\n\"a\": 1\n}", "{\n\n\"a\": 1\n}"},
		{"行尾注释", `{"a": 1} // tail`, `{"a": 1} `},
		{"字符串内的双斜杠不误伤", `{"url": "http://x:52888//p"}`, `{"url": "http://x:52888//p"}`},
		{"块注释", "{/* x */\"a\": 1}", `{"a": 1}`},
		{"块注释保留换行", "{/* a\nb */\"x\":1}", "{\n\"x\":1}"},
		{"转义引号", `{"s": "a\"//b"} // c`, `{"s": "a\"//b"} `},
	}
	for _, c := range cases {
		if got := string(stripJSONComments([]byte(c.in))); got != c.want {
			t.Errorf("%s: stripJSONComments(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestLoadConfigWithComments(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "aleiyun_client.json")
	os.WriteFile(p, []byte(`{
  // controller 地址
  "controller": "121.40.193.74", // 裸 IP
  /* 账号 */
  "username": "u",
  "password": "p",
  "lan": "192.168.1.0/24,10.10.0.0/24"
}`), 0o644)
	c, err := loadConfig(p)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if c.Controller != "121.40.193.74" || c.Username != "u" || c.Password != "p" || c.LAN != "192.168.1.0/24,10.10.0.0/24" {
		t.Errorf("解析结果错误: %+v", c)
	}
}

func TestExampleConfigParses(t *testing.T) {
	c, err := loadConfig("aleiyun_client.json.example")
	if err != nil {
		t.Fatalf("example 配置解析失败: %v", err)
	}
	if c.Controller == "" {
		t.Error("example 缺少 controller")
	}
}

func TestDetectLANCommentLines(t *testing.T) {
	lines := detectLANCommentLines()
	// 不依赖本机网卡情况:有输出时必须是合法的注释行且含 CIDR
	for _, line := range strings.Split(strings.TrimRight(lines, "\n"), "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "  // \"lan\": \"") {
			t.Errorf("探测行格式错误: %q", line)
		}
		if !strings.Contains(line, "/") {
			t.Errorf("探测行缺少 CIDR: %q", line)
		}
	}
}

func TestWriteDefaultConfig(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "aleiyun_client.json")
	if err := writeDefaultConfig(p, "myhost"); err != nil {
		t.Fatalf("writeDefaultConfig: %v", err)
	}
	c, err := loadConfig(p) // 带注释的默认配置必须能解析回来
	if err != nil {
		t.Fatalf("默认配置解析失败: %v", err)
	}
	if c.Name != "myhost" || c.Key != "aleiyun_client.key" || c.ListenPort != 51820 || c.Log != "aleiyun_client.log" {
		t.Errorf("默认配置字段错误: %+v", c)
	}
}

func TestNormalizeController(t *testing.T) {
	cases := []struct{ in, want string }{
		// 裸 IP / 裸域名:补 http:// + :52888
		{"1.2.3.4", "http://1.2.3.4:52888"},
		{"example.com", "http://example.com:52888"},
		// 自带端口:保留
		{"1.2.3.4:9000", "http://1.2.3.4:9000"},
		{"example.com:9000", "http://example.com:9000"},
		{"http://121.40.193.74:52888", "http://121.40.193.74:52888"},
		// http:// 无端口:补 :52888
		{"http://example.com", "http://example.com:52888"},
		// https:// 无端口:补 :443;带端口保留
		{"https://example.com", "https://example.com:443"},
		{"https://example.com:8443", "https://example.com:8443"},
		// 空串 / 纯空白
		{"", ""},
		{"   ", ""},
		// 带路径:保留路径并补端口
		{"domain.com/sd", "http://domain.com:52888/sd"},
		{"https://domain.com/sd", "https://domain.com:443/sd"},
	}
	for _, c := range cases {
		if got := normalizeController(c.in); got != c.want {
			t.Errorf("normalizeController(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
