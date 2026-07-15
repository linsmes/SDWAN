// pathcheck 是一个简易的通路判断网页服务：支持 ICMP ping 和 TCP 端口探测。
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const page = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>通路判断</title>
<style>
  body { font-family: "Segoe UI", "Microsoft YaHei", sans-serif; background: #0f172a; color: #e2e8f0; padding: 40px; }
  .box { max-width: 480px; margin: 0 auto; background: #1e293b; border: 1px solid #334155; border-radius: 8px; padding: 24px; }
  h1 { font-size: 20px; margin-bottom: 20px; }
  label { display: block; font-size: 13px; color: #94a3b8; margin: 12px 0 6px; }
  input, select { width: 100%; background: #0f172a; border: 1px solid #334155; color: #e2e8f0; padding: 8px 10px; border-radius: 4px; font-size: 14px; box-sizing: border-box; }
  button { width: 100%; margin-top: 18px; background: #0ea5e9; border: none; color: #fff; padding: 10px; border-radius: 4px; cursor: pointer; font-size: 15px; }
  button:hover { background: #0284c7; }
  #result { margin-top: 16px; padding: 12px; border-radius: 4px; font-size: 14px; white-space: pre-wrap; }
  .ok { background: rgba(34,197,94,0.15); color: #86efac; }
  .fail { background: rgba(239,68,68,0.15); color: #fca5a5; }
  .hint { color: #64748b; font-size: 12px; margin-top: 12px; }
</style>
</head>
<body>
<div class="box">
  <h1>通路判断</h1>
  <label>目标地址</label>
  <input id="target" placeholder="例如 121.40.193.74 或 baidu.com">
  <label>探测类型</label>
  <select id="mode">
    <option value="icmp">ICMP Ping</option>
    <option value="tcp">TCP 端口</option>
  </select>
  <label>端口（仅 TCP）</label>
  <input id="port" placeholder="80" value="80">
  <button onclick="check()">开始探测</button>
  <div id="result" style="display:none"></div>
  <div class="hint">由本机发起探测，ICMP 需要本机支持 ping 命令。</div>
</div>
<script>
async function check() {
  const target = document.getElementById("target").value.trim();
  const mode = document.getElementById("mode").value;
  const port = document.getElementById("port").value;
  const r = document.getElementById("result");
  if (!target) { r.textContent = "请输入目标地址"; r.className = "fail"; r.style.display = "block"; return; }
  r.textContent = "探测中..."; r.className = ""; r.style.display = "block";
  try {
    const resp = await fetch("/check", {
      method: "POST",
      headers: {"Content-Type": "application/json"},
      body: JSON.stringify({target, mode, port})
    });
    const data = await resp.json();
    r.textContent = data.output || data.error || JSON.stringify(data, null, 2);
    r.className = data.ok ? "ok" : "fail";
  } catch (e) {
    r.textContent = "请求失败: " + e.message;
    r.className = "fail";
  }
}
</script>
</body>
</html>
`

type checkReq struct {
	Target string `json:"target"`
	Mode   string `json:"mode"` // "icmp" | "tcp"
	Port   string `json:"port"`
}

type checkResp struct {
	OK     bool   `json:"ok"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

func ping(target string) checkResp {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// 使用 PowerShell Test-Connection，避免 cmd ping 的 GBK 编码乱码
		ps := fmt.Sprintf(`$r=Test-Connection -ComputerName '%s' -Count 4 -Delay 1 -ErrorAction SilentlyContinue; if ($r) { Write-Output ('OK: reached '+$r[0].Address); $r | ForEach-Object { Write-Output ('time='+$_.ResponseTime+'ms') } } else { Write-Output 'FAIL: unreachable' }`, target)
		cmd = exec.Command("powershell", "-NoProfile", "-Command", ps)
	} else {
		cmd = exec.Command("ping", "-c", "4", "-W", "2", target)
	}
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		if output == "" {
			return checkResp{OK: false, Error: err.Error()}
		}
	}
	var ok bool
	if runtime.GOOS == "windows" {
		ok = strings.HasPrefix(output, "OK:")
	} else {
		ok = strings.Contains(output, "0% packet loss")
	}
	return checkResp{OK: ok, Output: output}
}

func tcpProbe(target, port string) checkResp {
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return checkResp{OK: false, Error: "端口非法"}
	}
	addr := net.JoinHostPort(target, port)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return checkResp{OK: false, Error: err.Error()}
	}
	defer conn.Close()
	ms := time.Since(start).Milliseconds()
	return checkResp{OK: true, Output: fmt.Sprintf("TCP %s 连通\n延迟: %d ms", addr, ms)}
}

func handleCheck(w http.ResponseWriter, r *http.Request) {
	var req checkReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Target = strings.TrimSpace(req.Target)
	if req.Target == "" {
		writeJSON(w, checkResp{OK: false, Error: "目标地址不能为空"})
		return
	}
	var res checkResp
	switch req.Mode {
	case "tcp":
		res = tcpProbe(req.Target, req.Port)
	default:
		res = ping(req.Target)
	}
	writeJSON(w, res)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8081"
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		t := template.Must(template.New("page").Parse(page))
		_ = t.Execute(w, nil)
	})
	http.HandleFunc("/check", handleCheck)
	fmt.Printf("pathcheck 服务启动: http://0.0.0.0%s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
}
