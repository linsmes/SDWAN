package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestFilterLANConflict 两设备上报同网段:先注册者保留,后者被剔除并记 conflict;
// 格式非法的网段同样被剔除并原样记录。
func TestFilterLANConflict(t *testing.T) {
	s := &Store{Devices: map[string]*Device{}}
	a := &Device{PubKey: "aaa", Name: "A"}
	b := &Device{PubKey: "bbb", Name: "B"}
	s.Devices["aaa"] = a
	s.Devices["bbb"] = b

	s.filterLANLocked(a, []string{"192.168.1.0/24", "not-a-cidr"})
	if !reflect.DeepEqual(a.LANSubnets, []string{"192.168.1.0/24"}) {
		t.Errorf("A LANSubnets = %v, 期望 [192.168.1.0/24]", a.LANSubnets)
	}
	if !reflect.DeepEqual(a.LANConflicts, []string{"not-a-cidr"}) {
		t.Errorf("A LANConflicts = %v, 期望 [not-a-cidr]", a.LANConflicts)
	}

	s.filterLANLocked(b, []string{"192.168.1.0/24", "10.10.0.0/24"})
	if !reflect.DeepEqual(b.LANSubnets, []string{"10.10.0.0/24"}) {
		t.Errorf("B LANSubnets = %v, 期望 [10.10.0.0/24](192.168.1.0/24 应被 A 占用剔除)", b.LANSubnets)
	}
	if !reflect.DeepEqual(b.LANConflicts, []string{"192.168.1.0/24"}) {
		t.Errorf("B LANConflicts = %v, 期望 [192.168.1.0/24]", b.LANConflicts)
	}

	// A 再次上报(心跳):自己已通告的网段不算冲突
	s.filterLANLocked(a, []string{"192.168.1.0/24"})
	if !reflect.DeepEqual(a.LANSubnets, []string{"192.168.1.0/24"}) || len(a.LANConflicts) != 0 {
		t.Errorf("A 重复上报后 LANSubnets = %v, LANConflicts = %v, 期望保持不变且无冲突", a.LANSubnets, a.LANConflicts)
	}
}

// newTestStore 构造含两个网络(n1: 10.10.10.0/24,n2: 20.20.20.0/24)的测试 Store
func newTestStore() *Store {
	return &Store{
		Devices: map[string]*Device{},
		Networks: map[string]*Network{
			"n1": {ID: "n1", Name: "A公司", CIDR: "10.10.10.0/24"},
			"n2": {ID: "n2", Name: "B公司", CIDR: "20.20.20.0/24"},
		},
		Links:    map[string]*Link{},
		Accounts: map[string]*Account{},
	}
}

// addTestAccount 往测试 Store 加一个账号(明文密码直接换算成 salt+hash)
func addTestAccount(s *Store, id, username, password, netID string) *Account {
	a := &Account{ID: id, Username: username, Salt: "salt-" + id, NetworkID: netID, CreatedAt: time.Now()}
	a.PasswordHash = hashPassword(a.Salt, password)
	s.Accounts[id] = a
	return a
}

// addTestDevice 往测试 Store 加一台在线设备
func addTestDevice(s *Store, id, pubkey, name, netID, ip string) *Device {
	d := &Device{ID: id, PubKey: pubkey, Name: name, Network: netID, VirtualIP: ip, LastSeen: time.Now()}
	s.Devices[pubkey] = d
	return d
}

func peerIPs(peers []peerView) []string {
	out := []string{}
	for _, p := range peers {
		out = append(out, p.VirtualIP)
	}
	return out
}

// TestNetworkIsolation 无任何互联规则时,跨网络设备互不可见
func TestNetworkIsolation(t *testing.T) {
	s := newTestStore()
	a := addTestDevice(s, "da", "pub-a", "a", "n1", "10.10.10.2")
	b := addTestDevice(s, "db", "pub-b", "b", "n2", "20.20.20.2")

	if got := s.peersForLocked(a); len(got) != 0 {
		t.Errorf("无规则时 a 的 peers = %v,期望为空", peerIPs(got))
	}
	if got := s.peersForLocked(b); len(got) != 0 {
		t.Errorf("无规则时 b 的 peers = %v,期望为空", peerIPs(got))
	}
}

// TestDeviceLink device link 只打通指定的两台设备,且路由为对端 /32;
// 同网络其他设备不受影响
func TestDeviceLink(t *testing.T) {
	s := newTestStore()
	a := addTestDevice(s, "da", "pub-a", "a", "n1", "10.10.10.2")
	a2 := addTestDevice(s, "da2", "pub-a2", "a2", "n1", "10.10.10.3")
	b := addTestDevice(s, "db", "pub-b", "b", "n2", "20.20.20.2")
	s.Links["l1"] = &Link{ID: "l1", Type: "device", A: "da", B: "db"}

	// 同网络的 a2 仍然可见(同网络默认互通),device link 额外带来 b
	if got := peerIPs(s.peersForLocked(a)); !reflect.DeepEqual(got, []string{"10.10.10.3", "20.20.20.2"}) {
		t.Errorf("a 的 peers = %v,期望 [10.10.10.3 20.20.20.2]", got)
	}
	if got := peerIPs(s.peersForLocked(b)); !reflect.DeepEqual(got, []string{"10.10.10.2"}) {
		t.Errorf("b 的 peers = %v,期望 [10.10.10.2]", got)
	}
	if got := s.peersForLocked(a2); len(got) != 1 || got[0].VirtualIP != "10.10.10.2" {
		t.Errorf("a2 的 peers = %v,期望只看到同网络的 a", peerIPs(got))
	}
	if got := s.routesForLocked(a); !reflect.DeepEqual(got, []string{"20.20.20.2/32"}) {
		t.Errorf("a 的 routes = %v,期望 [20.20.20.2/32]", got)
	}
	if got := s.routesForLocked(b); !reflect.DeepEqual(got, []string{"10.10.10.2/32"}) {
		t.Errorf("b 的 routes = %v,期望 [10.10.10.2/32]", got)
	}
}

// TestNetworkLink network link 打通双方整网,路由为对端网络 CIDR
func TestNetworkLink(t *testing.T) {
	s := newTestStore()
	a := addTestDevice(s, "da", "pub-a", "a", "n1", "10.10.10.2")
	addTestDevice(s, "db", "pub-b", "b", "n2", "20.20.20.2")
	addTestDevice(s, "db2", "pub-b2", "b2", "n2", "20.20.20.3")
	s.Links["l1"] = &Link{ID: "l1", Type: "network", A: "n1", B: "n2"}

	if got := peerIPs(s.peersForLocked(a)); !reflect.DeepEqual(got, []string{"20.20.20.2", "20.20.20.3"}) {
		t.Errorf("a 的 peers = %v,期望 [20.20.20.2 20.20.20.3]", got)
	}
	if got := s.routesForLocked(a); !reflect.DeepEqual(got, []string{"20.20.20.0/24"}) {
		t.Errorf("a 的 routes = %v,期望 [20.20.20.0/24]", got)
	}
}

// TestNextIPPerNetwork 各网络地址池相互独立,第一个设备都拿到 .2
func TestNextIPPerNetwork(t *testing.T) {
	s := newTestStore()
	ip1, err := s.nextIPInNetworkLocked("n1", "10.10.10.0/24")
	if err != nil || ip1 != "10.10.10.2" {
		t.Errorf("n1 首个 IP = %q, err=%v,期望 10.10.10.2", ip1, err)
	}
	ip2, err := s.nextIPInNetworkLocked("n2", "20.20.20.0/24")
	if err != nil || ip2 != "20.20.20.2" {
		t.Errorf("n2 首个 IP = %q, err=%v,期望 20.20.20.2", ip2, err)
	}
	// n1 占用 .2 后,下一个应为 .3;不影响 n2
	addTestDevice(s, "da", "pub-a", "a", "n1", ip1)
	ip3, _ := s.nextIPInNetworkLocked("n1", "10.10.10.0/24")
	if ip3 != "10.10.10.3" {
		t.Errorf("n1 第二个 IP = %q,期望 10.10.10.3", ip3)
	}
}

// TestOldDataMigration 旧版 data.json(无 networks/links 字段)加载后:
// 自动创建默认网络,存量设备归入默认网络
func TestOldDataMigration(t *testing.T) {
	old := `{"devices":{"pub-x":{"id":"dx","name":"x","pubkey":"pub-x","virtual_ip":"10.66.0.2"}}}`
	path := t.TempDir() + "/data.json"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStore(path)
	def := s.Networks[defaultNetworkID]
	if def == nil || def.CIDR != defaultNetworkCIDR {
		t.Fatalf("默认网络未正确创建: %+v", def)
	}
	d := s.Devices["pub-x"]
	if d == nil || d.Network != defaultNetworkID {
		t.Fatalf("存量设备未归入默认网络: %+v", d)
	}
}

// TestHeartbeatRename 运行中改名:注册后用心跳带新 name,/api/devices 里名称应更新;
// 空 name 的心跳不改名(老版本 agent 行为兼容)
func TestHeartbeatRename(t *testing.T) {
	s := newTestStore()
	addTestAccount(s, "acc1", "user1", "pw1", "n1")
	s.path = t.TempDir() + "/data.json" // saveLocked 需要可写路径
	mux := http.NewServeMux()
	mux.HandleFunc("/api/register", s.handleRegister)
	mux.HandleFunc("/api/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("/api/devices", s.handleDevices)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	post := func(path string, body map[string]any) int {
		payload, _ := json.Marshal(body)
		resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	deviceName := func() string {
		resp, err := http.Get(srv.URL + "/api/devices")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var devices []Device
		if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
			t.Fatal(err)
		}
		for _, d := range devices {
			if d.PubKey == "pk-rename" {
				return d.Name
			}
		}
		return ""
	}

	if code := post("/api/register", map[string]any{"name": "旧名字", "pubkey": "pk-rename", "username": "user1", "password": "pw1"}); code != http.StatusOK {
		t.Fatalf("注册返回 %d,期望 200", code)
	}
	if got := deviceName(); got != "旧名字" {
		t.Fatalf("注册后名称 = %q,期望 旧名字", got)
	}

	// 心跳带新名字 => 面板名称更新
	if code := post("/api/heartbeat", map[string]any{"name": "新名字", "pubkey": "pk-rename", "network": "A公司"}); code != http.StatusOK {
		t.Fatalf("心跳返回 %d,期望 200", code)
	}
	if got := deviceName(); got != "新名字" {
		t.Fatalf("心跳改名后名称 = %q,期望 新名字", got)
	}

	// 空 name 心跳(老版本 agent)不改名
	if code := post("/api/heartbeat", map[string]any{"pubkey": "pk-rename", "network": "A公司"}); code != http.StatusOK {
		t.Fatalf("空 name 心跳返回 %d,期望 200", code)
	}
	if got := deviceName(); got != "新名字" {
		t.Fatalf("空 name 心跳后名称 = %q,期望保持 新名字", got)
	}
}

// TestAccountRegistration 账号体系:公开注册关闭,凭账号密码注册,所属网络由账号绑定决定
func TestAccountRegistration(t *testing.T) {
	s := newTestStore()
	addTestAccount(s, "acc-b", "alice", "secret123", "n2") // 绑定 B公司 (20.20.20.0/24)
	s.path = t.TempDir() + "/data.json"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/register", s.handleRegister)
	mux.HandleFunc("/api/accounts", s.handleAccounts)
	mux.HandleFunc("/api/accounts/", s.handleAccounts)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	post := func(path string, body map[string]any) (int, []byte) {
		payload, _ := json.Marshal(body)
		resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, data
	}

	// 无账密 => 403
	if code, _ := post("/api/register", map[string]any{"name": "x", "pubkey": "pk1"}); code != http.StatusForbidden {
		t.Fatalf("无账密注册返回 %d,期望 403", code)
	}
	// 错密码 => 403
	if code, _ := post("/api/register", map[string]any{"name": "x", "pubkey": "pk1", "username": "alice", "password": "wrong"}); code != http.StatusForbidden {
		t.Fatalf("错密码注册返回 %d,期望 403", code)
	}
	// 正确账密 => 成功;自报其他网络被忽略,IP 落在账号绑定网络(n2: 20.20.20.0/24)
	code, data := post("/api/register", map[string]any{
		"name": "x", "pubkey": "pk1", "username": "alice", "password": "secret123", "network": "A公司"})
	if code != http.StatusOK {
		t.Fatalf("正确账密注册返回 %d (%s),期望 200", code, data)
	}
	var rr registerResp
	if err := json.Unmarshal(data, &rr); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rr.VirtualIP, "20.20.20.") {
		t.Fatalf("注册所得 IP = %q,期望落在账号绑定网络 20.20.20.0/24(自报 A公司 应被忽略)", rr.VirtualIP)
	}
	if rr.Network != "B公司" {
		t.Fatalf("响应网络 = %q,期望 B公司", rr.Network)
	}
	if d := s.Devices["pk1"]; d.Username != "alice" || d.Network != "n2" {
		t.Fatalf("设备记录 username=%q network=%q,期望 alice/n2", d.Username, d.Network)
	}

	// GET /api/accounts 输出绝不含 password_hash/salt
	resp, err := http.Get(srv.URL + "/api/accounts")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), "password_hash") || strings.Contains(string(body), "salt") {
		t.Fatalf("/api/accounts 输出泄露了 hash/salt: %s", body)
	}

	// 删除有关联设备的账号 => 409
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/accounts/acc-b", nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusConflict {
		t.Fatalf("删除有设备的账号返回 %d,期望 409", delResp.StatusCode)
	}
}

// TestAccountDeviceLimit 账号设备数上限:默认 1 台,达上限拒绝新设备注册,
// 同密钥重注册不受限;调大上限后可继续注册;上限可通过 API 调整
func TestAccountDeviceLimit(t *testing.T) {
	s := newTestStore()
	addTestAccount(s, "acc-lim", "bob", "pw", "n1") // 默认上限 1
	s.path = t.TempDir() + "/data.json"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/register", s.handleRegister)
	mux.HandleFunc("/api/accounts/", s.handleAccounts)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	post := func(path string, body map[string]any) (int, []byte) {
		payload, _ := json.Marshal(body)
		resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, data
	}
	reg := func(pk string) (int, []byte) {
		return post("/api/register", map[string]any{"name": pk, "pubkey": pk, "username": "bob", "password": "pw"})
	}

	// 第 1 台:成功
	if code, data := reg("pk-a"); code != http.StatusOK {
		t.Fatalf("首台设备注册返回 %d (%s),期望 200", code, data)
	}
	// 同密钥重注册:不受上限限制
	if code, data := reg("pk-a"); code != http.StatusOK {
		t.Fatalf("同密钥重注册返回 %d (%s),期望 200", code, data)
	}
	// 第 2 台(不同密钥):默认上限 1 => 403
	code, data := reg("pk-b")
	if code != http.StatusForbidden {
		t.Fatalf("超限注册返回 %d (%s),期望 403", code, data)
	}
	if !strings.Contains(string(data), "上限") {
		t.Fatalf("超限提示应包含「上限」: %s", data)
	}
	// 调大上限为 2
	if code, data := post("/api/accounts/acc-lim/limit", map[string]any{"max_devices": 2}); code != http.StatusOK {
		t.Fatalf("调整上限返回 %d (%s),期望 200", code, data)
	}
	// 非法上限 => 400
	if code, _ := post("/api/accounts/acc-lim/limit", map[string]any{"max_devices": 0}); code != http.StatusBadRequest {
		t.Fatalf("非法上限返回 %d,期望 400", code)
	}
	// 第 2 台:成功;第 3 台:再次超限
	if code, data := reg("pk-b"); code != http.StatusOK {
		t.Fatalf("调大上限后注册返回 %d (%s),期望 200", code, data)
	}
	if code, _ := reg("pk-c"); code != http.StatusForbidden {
		t.Fatalf("再次超限注册返回 %d,期望 403", code)
	}
}

// TestDeviceKickAndDisable 面板可强制下线、禁用/启用设备;
// 禁用后该 pubkey 与相同 MAC 的新注册/心跳均 403
func TestDeviceKickAndDisable(t *testing.T) {
	s := newTestStore()
	a := addTestAccount(s, "acc1", "user1", "pw1", "n1")
	a.MaxDevices = 5
	d := addTestDevice(s, "da", "pub-a", "deviceA", "n1", "10.10.10.2")
	d.MACs = []string{"aa:bb:cc:11:22:33"}
	d.Username = "user1"
	s.path = t.TempDir() + "/data.json"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/register", s.handleRegister)
	mux.HandleFunc("/api/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("/api/devices/", s.handleDevices)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	post := func(path string, body map[string]any) (int, []byte) {
		payload, _ := json.Marshal(body)
		resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, data
	}

	// 强制下线 => 204,设备 Offline=true
	if code, _ := post("/api/devices/da/kick", nil); code != http.StatusNoContent {
		t.Fatalf("kick 返回 %d,期望 204", code)
	}
	if !d.Offline {
		t.Fatal("kick 后设备 Offline 标记应为 true")
	}

	// 禁用 => 204,Disabled=true
	if code, _ := post("/api/devices/da/disable", nil); code != http.StatusNoContent {
		t.Fatalf("disable 返回 %d,期望 204", code)
	}
	if !d.Disabled || !d.Offline {
		t.Fatal("disable 后 Disabled 与 Offline 均应为 true")
	}

	// 同一 pubkey 心跳 => 403
	if code, data := post("/api/heartbeat", map[string]any{"pubkey": "pub-a", "macs": []string{"aa:bb:cc:11:22:33"}}); code != http.StatusForbidden {
		t.Fatalf("禁用后心跳返回 %d (%s),期望 403", code, data)
	}

	// 同一 pubkey 注册 => 403
	if code, data := post("/api/register", map[string]any{"name": "x", "pubkey": "pub-a", "username": "user1", "password": "pw1"}); code != http.StatusForbidden {
		t.Fatalf("禁用后注册返回 %d (%s),期望 403", code, data)
	}

	// 换密钥但 MAC 相同 => 注册 403(封机器)
	if code, data := post("/api/register", map[string]any{"name": "x", "pubkey": "pub-b", "username": "user1", "password": "pw1", "macs": []string{"aa:bb:cc:11:22:33"}}); code != http.StatusForbidden {
		t.Fatalf("换密钥同 MAC 注册返回 %d (%s),期望 403", code, data)
	}

	// 启用原设备 => 禁用解除
	if code, _ := post("/api/devices/da/enable", nil); code != http.StatusNoContent {
		t.Fatalf("enable 返回 %d,期望 204", code)
	}
	if d.Disabled {
		t.Fatal("enable 后 Disabled 应为 false")
	}

	// 启用后可注册新密钥(同 MAC 不再冲突)
	if code, data := post("/api/register", map[string]any{"name": "x", "pubkey": "pub-b", "username": "user1", "password": "pw1", "macs": []string{"aa:bb:cc:11:22:33"}}); code != http.StatusOK {
		t.Fatalf("启用后同 MAC 新密钥注册返回 %d (%s),期望 200", code, data)
	}
}

// TestDeviceChangeNetworkUpdatesAccount 设备页修改公司(网络)时,账号绑定网络同步跟随,
// 且该账号下所有设备一起迁移到新网络,避免心跳时又被迁回
func TestDeviceChangeNetworkUpdatesAccount(t *testing.T) {
	s := newTestStore()
	addTestAccount(s, "acc1", "user1", "pw1", "n1")
	d1 := addTestDevice(s, "d1", "pub-a", "A1", "n1", "10.10.10.2")
	d1.Username = "user1"
	d2 := addTestDevice(s, "d2", "pub-b", "A2", "n1", "10.10.10.3")
	d2.Username = "user1"
	s.path = t.TempDir() + "/data.json"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/devices/", s.handleDevices)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/devices/d1", strings.NewReader(`{"network":"B公司"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("换网络返回 %d,期望 200", resp.StatusCode)
	}

	acc := s.Accounts["acc1"]
	if acc.NetworkID != "n2" {
		t.Fatalf("账号绑定网络未跟随变化: %q,期望 n2", acc.NetworkID)
	}
	if d1.Network != "n2" || !strings.HasPrefix(d1.VirtualIP, "20.20.20.") {
		t.Fatalf("d1 未迁移到 n2: network=%s ip=%s", d1.Network, d1.VirtualIP)
	}
	if d2.Network != "n2" || !strings.HasPrefix(d2.VirtualIP, "20.20.20.") {
		t.Fatalf("d2 未迁移到 n2: network=%s ip=%s", d2.Network, d2.VirtualIP)
	}
}
