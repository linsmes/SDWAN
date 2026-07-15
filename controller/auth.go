package main

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	secureCookie bool
	sessions     = map[string]Session{}
	sessionsMu   sync.RWMutex
)

// Session 服务端会话
type Session struct {
	Username string
	Expiry   time.Time
}

const (
	sessionCookieName = "aleiyun_session"
	sessionTTL        = 24 * time.Hour
)

// adminUser 管理员用户
type adminUser struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	CreatedAt    time.Time `json:"created_at"`
}

// adminDB 管理员账号的本地 JSON 存储
type adminDB struct {
	Users []adminUser `json:"users"`
	mu    sync.RWMutex
	path  string
}

var adminDBInstance *adminDB

// initAdminAuth 初始化管理员认证：打开本地存储、创建默认账号
func initAdminAuth(storePath, defaultUser, defaultPass string, secure bool) error {
	secureCookie = secure

	dir := filepath.Dir(storePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建 admin 存储目录失败: %w", err)
	}

	db := &adminDB{path: storePath}
	if err := db.load(); err != nil {
		return fmt.Errorf("加载 admin 数据失败: %w", err)
	}

	if len(db.Users) == 0 {
		pass := defaultPass
		if pass == "" {
			pass = randomAdminPassword(12)
			log.Printf("未指定管理员密码,已随机生成: %s", pass)
		}
		// 前端传输的是 MD5(password) 的 hex，因此默认账号也用 MD5(password) 作为 hashAdminPassword 输入
		db.Users = append(db.Users, adminUser{
			Username:     defaultUser,
			PasswordHash: hashAdminPassword(md5Hex(pass)),
			CreatedAt:    time.Now().UTC(),
		})
		if err := db.save(); err != nil {
			return fmt.Errorf("保存默认管理员失败: %w", err)
		}
		log.Printf("已创建默认管理员账号: %s / %s", defaultUser, pass)
	}

	adminDBInstance = db
	go sessionCleaner()
	return nil
}

func lastSeg(path string) string {
	parts := strings.Split(strings.TrimRight(path, "/"), "/")
	return parts[len(parts)-1]
}

// randomAdminPassword 生成随机可打印密码
func randomAdminPassword(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("admin%d", time.Now().UnixNano()%1000000)
	}
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}

// md5Hex 返回字符串的 MD5 hex
func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// hashAdminPassword SHA-256(salt+password)，返回 "salt:hash" hex
func hashAdminPassword(password string) string {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		// 理论上不可能失败，失败时使用时间戳作为 salt
		salt = []byte(fmt.Sprintf("%d", time.Now().UnixNano()))[:16]
	}
	h := sha256.Sum256(append(salt, []byte(password)...))
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(h[:])
}

// checkAdminPassword 常量时间比较密码
func checkAdminPassword(hash, password string) bool {
	parts := strings.Split(hash, ":")
	if len(parts) != 2 {
		return false
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	expected, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	h := sha256.Sum256(append(salt, []byte(password)...))
	return subtle.ConstantTimeCompare(h[:], expected) == 1
}

func (db *adminDB) load() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	data, err := os.ReadFile(db.path)
	if err != nil {
		if os.IsNotExist(err) {
			db.Users = nil
			return nil
		}
		return err
	}
	return json.Unmarshal(data, db)
}

func (db *adminDB) save() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(db.path, data, 0o600)
}

func (db *adminDB) findUser(username string) *adminUser {
	db.mu.RLock()
	defer db.mu.RUnlock()
	for i := range db.Users {
		if db.Users[i].Username == username {
			return &db.Users[i]
		}
	}
	return nil
}

// updatePassword 更新指定用户的密码，并持久化到文件
// newPassword 应为前端传来的 MD5(password) hex，不再二次 MD5
func (db *adminDB) updatePassword(username, newPassword string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	for i := range db.Users {
		if db.Users[i].Username == username {
			db.Users[i].PasswordHash = hashAdminPassword(newPassword)
			data, err := json.MarshalIndent(db, "", "  ")
			if err != nil {
				return err
			}
			return os.WriteFile(db.path, data, 0o600)
		}
	}
	return fmt.Errorf("用户不存在")
}

// sessionCleaner 定时清理过期会话
func sessionCleaner() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		sessionsMu.Lock()
		for tok, s := range sessions {
			if now.After(s.Expiry) {
				delete(sessions, tok)
			}
		}
		sessionsMu.Unlock()
	}
}

// loginFailures 简单登录失败速率限制
type loginFailures struct {
	mu    sync.Mutex
	slots map[string][]time.Time
}

var failLimit = &loginFailures{slots: map[string][]time.Time{}}

func (l *loginFailures) allowed(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-5 * time.Minute)
	var valid []time.Time
	for _, t := range l.slots[ip] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	l.slots[ip] = valid
	return len(valid) < 5
}

func (l *loginFailures) record(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.slots[ip] = append(l.slots[ip], time.Now())
}

func clientIP(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

// handleAdminLogin 管理员登录
func handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ip := clientIP(r)
	if !failLimit.allowed(ip) {
		http.Error(w, "登录尝试过于频繁,请 5 分钟后再试", http.StatusTooManyRequests)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求格式错误", http.StatusBadRequest)
		return
	}

	user := adminDBInstance.findUser(req.Username)
	if user == nil || !checkAdminPassword(user.PasswordHash, req.Password) {
		failLimit.record(ip)
		http.Error(w, "用户名或密码错误", http.StatusUnauthorized)
		return
	}

	token := randomToken()
	sessionsMu.Lock()
	sessions[token] = Session{Username: req.Username, Expiry: time.Now().Add(sessionTTL)}
	sessionsMu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secureCookie,
		MaxAge:   int(sessionTTL.Seconds()),
	})

	writeJSON(w, map[string]string{"username": req.Username})
}

// handleAdminLogout 登出
func handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c, err := r.Cookie(sessionCookieName); err == nil {
		sessionsMu.Lock()
		delete(sessions, c.Value)
		sessionsMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secureCookie,
	})
	w.WriteHeader(http.StatusOK)
}

// handleAdminPasswordChange 修改当前登录管理员密码
func handleAdminPasswordChange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := currentUser(r)
	if user == "" {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求格式错误", http.StatusBadRequest)
		return
	}

	if len(req.NewPassword) < 6 {
		http.Error(w, "新密码长度不能少于 6 位", http.StatusBadRequest)
		return
	}

	u := adminDBInstance.findUser(user)
	if u == nil || !checkAdminPassword(u.PasswordHash, req.OldPassword) {
		http.Error(w, "旧密码错误", http.StatusUnauthorized)
		return
	}

	if err := adminDBInstance.updatePassword(user, req.NewPassword); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 修改成功后清除当前 session，要求重新登录
	if c, err := r.Cookie(sessionCookieName); err == nil {
		sessionsMu.Lock()
		delete(sessions, c.Value)
		sessionsMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secureCookie,
	})

	w.WriteHeader(http.StatusOK)
}

// handleAdminSession 返回当前登录状态
func handleAdminSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := currentUser(r)
	if user == "" {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]string{"username": user})
}

// currentUser 从请求 Cookie 解析当前登录用户名
func currentUser(r *http.Request) string {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	sessionsMu.RLock()
	s, ok := sessions[c.Value]
	sessionsMu.RUnlock()
	if !ok || time.Now().After(s.Expiry) {
		return ""
	}
	return s.Username
}

// randomToken 生成随机会话 token
func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// staticFileExtensions 不需要登录即可访问的静态资源扩展名
var staticFileExtensions = map[string]bool{
	".js": true, ".css": true, ".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".svg": true, ".woff": true, ".woff2": true, ".ttf": true,
	".eot": true, ".ico": true,
}

func isStaticFile(path string) bool {
	for ext := range staticFileExtensions {
		if strings.HasSuffix(strings.ToLower(path), ext) {
			return true
		}
	}
	return false
}

// publicClientAPIs 不需要管理员 Session 的客户端 API
var publicClientAPIs = map[string]bool{
	"/api/register": true,
	"/api/heartbeat": true,
	"/api/offline": true,
	"/api/wait": true,
}

// isRelayBeatAPI 判断是否为 relay 心跳上报接口(/api/relays/beat 或 /api/relays/{id}/beat)
func isRelayBeatAPI(path string) bool {
	if path == "/api/relays/beat" {
		return true
	}
	if strings.HasPrefix(path, "/api/relays/") && strings.HasSuffix(path, "/beat") {
		return true
	}
	return false
}

// authMiddleware 认证中间件：保护管理后台页面和管理后台 API
// Windows 客户端使用的 /api/register、/api/heartbeat 等 API 保持公开
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 放行静态资源，避免未登录时 JS/CSS 被拦截成登录页
		if isStaticFile(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// 放行登录相关 API
		if r.URL.Path == "/api/admin/login" || r.URL.Path == "/api/admin/session" {
			next.ServeHTTP(w, r)
			return
		}

		// 放行 Windows 客户端公开 API（注册、心跳、下线、长轮询）
		if publicClientAPIs[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		// 放行 relay 心跳上报接口(/api/relays/beat 或 /api/relays/{id}/beat)
		if isRelayBeatAPI(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// 已登录放行
		if currentUser(r) != "" {
			next.ServeHTTP(w, r)
			return
		}

		// 未登录：API 返回 401，页面请求返回登录页
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.Error(w, "未登录", http.StatusUnauthorized)
			return
		}

		// 页面请求：返回 index.html，由前端渲染登录页
		r.URL.Path = "/"
		next.ServeHTTP(w, r)
	})
}
