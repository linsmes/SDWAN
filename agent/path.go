// 线路优选：P2P 优先，P2P 失败或质量差时自动切换到 relay 中转。
// 为每个 peer 维护 P2P endpoint + 所有在线 relay 作为候选，
// 根据 WireGuard 握手状态与 ICMP RTT 自动选择当前最快可用线路。
package main

import (
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// relayView 服务端下发的在线中转节点
type relayView struct {
	ID      string `json:"id"`
	Address string `json:"address"`
	PubKey  string `json:"pubkey,omitempty"`
	Note    string `json:"note,omitempty"`
}

const (
	pathInitialTimeout = 30 * time.Second  // 新候选等待首次握手的宽限
	pathProbeInterval  = 60 * time.Second  // 主动探测其他候选的最小间隔
	pathSwitchCooldown = 15 * time.Second  // 两次实际切换之间的最小间隔
	pathRttStale       = 120 * time.Second // RTT 记录过期时间
)

type pathStat struct {
	rtt     float64
	rttAt   time.Time
	ok      bool
	okAt    time.Time
	fail    int
	lastTry time.Time
}

// peerPath 单个 peer 的候选线路状态
type peerPath struct {
	pubkey     string
	virtualIP  string
	candidates []string // 首选 P2P 排在最前，后面是在线 relay
	current    string
	stats      map[string]*pathStat
	lastSwitch time.Time
	lastProbe  time.Time // 上次主动探测非当前候选的时间
}

// pathSelector 管理所有 peer 的线路选择
// 注意：selector 自身加锁，但调用方应保证 Pick/RecordRTT 等操作发生在同一线程
// （agent 的心跳循环），避免与 Update 并发导致状态错乱。
type pathSelector struct {
	mu    sync.Mutex
	peers map[string]*peerPath
}

func newPathSelector() *pathSelector {
	return &pathSelector{peers: map[string]*peerPath{}}
}

// Update 根据新的 peer 列表与 relay 列表重建候选。
// 保留旧的统计信息与当前选中线路（若仍在新候选中）。
func (s *pathSelector) Update(peers []peerView, relays []relayView) {
	s.mu.Lock()
	defer s.mu.Unlock()

	relayAddrs := make([]string, 0, len(relays))
	seen := map[string]bool{}
	for _, r := range relays {
		addr := strings.TrimSpace(r.Address)
		if addr == "" || seen[addr] {
			continue
		}
		seen[addr] = true
		relayAddrs = append(relayAddrs, addr)
	}
	sort.Strings(relayAddrs)

	newMap := make(map[string]*peerPath, len(peers))
	for _, p := range peers {
		if p.PubKey == "" {
			continue
		}
		cands := make([]string, 0, 1+len(relayAddrs))
		add := func(addr string) {
			addr = strings.TrimSpace(addr)
			if addr == "" {
				return
			}
			for _, existing := range cands {
				if existing == addr {
					return
				}
			}
			cands = append(cands, addr)
		}
		add(p.Endpoint)
		for _, addr := range relayAddrs {
			add(addr)
		}

		pp := &peerPath{
			pubkey:    p.PubKey,
			virtualIP: p.VirtualIP,
			candidates: cands,
			stats:     map[string]*pathStat{},
		}
		if old := s.peers[p.PubKey]; old != nil {
			pp.stats = old.stats
			pp.lastProbe = old.lastProbe
			for _, c := range cands {
				if c == old.current {
					pp.current = old.current
					pp.lastSwitch = old.lastSwitch
					break
				}
			}
		}
		newMap[p.PubKey] = pp
	}
	s.peers = newMap
}

// Pick 根据握手状态与各候选历史 RTT，决定每个 peer 当前应使用的 endpoint。
// lastHandshake 为 0 表示从未握手；stats 中缺少某个 peer 时按无握手处理。
func (s *pathSelector) Pick(stats map[string]int64, now time.Time) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[string]string, len(s.peers))
	for key, pp := range s.peers {
		chosen := s.pickOne(pp, stats[key], now)
		if chosen != "" {
			out[key] = chosen
		}
	}
	return out
}

func (s *pathSelector) pickOne(pp *peerPath, lastHandshake int64, now time.Time) string {
	if len(pp.candidates) == 0 {
		return ""
	}

	hasNewHandshake := lastHandshake > 0 && time.Unix(lastHandshake, 0).After(pp.lastSwitch)

	// 当前线路是否仍可用
	currentOK := false
	if pp.current != "" {
		switch {
		case !hasNewHandshake && now.Sub(pp.lastSwitch) < pathInitialTimeout:
			// 刚切换，还在等首次握手
			currentOK = true
		case hasNewHandshake && now.Sub(time.Unix(lastHandshake, 0)) < handshakeStaleAfter:
			// 当前线路有近期握手
			currentOK = true
		default:
			// 检查是否有该候选的成功记录兜底
			st := pp.stats[pp.current]
			if st != nil && st.ok && now.Sub(st.okAt) < handshakeStaleAfter {
				currentOK = true
			}
		}
	}

	// 按 RTT 给所有候选打分
	type scored struct {
		addr  string
		score float64 // 越小越好
	}
	scores := make([]scored, 0, len(pp.candidates))
	for _, c := range pp.candidates {
		st := pp.stats[c]
		score := float64(1<<63 - 1)
		if st != nil && st.ok {
			if st.rtt > 0 && now.Sub(st.rttAt) < pathRttStale {
				score = st.rtt
			} else if now.Sub(st.okAt) < handshakeStaleAfter {
				// 有成功记录但 RTT 过期，给一个中等偏大的分
				score = 800
			}
		}
		scores = append(scores, scored{addr: c, score: score})
	}
	sort.SliceStable(scores, func(i, j int) bool { return scores[i].score < scores[j].score })
	best := scores[0]

	// 如果当前可用，只在发现明显更好的候选时才切换
	if currentOK && pp.current != "" {
		curScore := float64(1<<63 - 1)
		for _, sc := range scores {
			if sc.addr == pp.current {
				curScore = sc.score
				break
			}
		}
		// 最佳候选比当前好 25% 以上，且已过冷却期
		if best.addr != pp.current && best.score < 1<<62 && curScore > 0 &&
			float64(best.score) < curScore*0.75 && now.Sub(pp.lastSwitch) >= pathSwitchCooldown {
			log.Printf("[path] %s 切换到更优线路 %s (RTT %.1fms < %.1fms)",
				pp.virtualIP, best.addr, best.score, curScore)
			pp.current = best.addr
			pp.lastSwitch = now
		}
		return pp.current
	}

	// 当前不可用：直接切到最佳候选
	if best.addr != pp.current || pp.current == "" {
		if now.Sub(pp.lastSwitch) >= pathSwitchCooldown || pp.current == "" {
			if pp.current != "" {
				log.Printf("[path] %s 当前线路 %s 不可用，切换到 %s",
					pp.virtualIP, pp.current, best.addr)
			} else {
				log.Printf("[path] %s 初始选择线路 %s", pp.virtualIP, best.addr)
			}
			pp.current = best.addr
			pp.lastSwitch = now
		}
	}
	return pp.current
}

// RecordRTT 记录对某个 peer 当前活跃线路的 RTT 探测结果。
// endpoint 为空时会记录到该 peer 当前选中的线路上。
func (s *pathSelector) RecordRTT(pubkey, endpoint string, rtt float64, ok bool, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pp := s.peers[pubkey]
	if pp == nil {
		return
	}
	if endpoint == "" {
		endpoint = pp.current
	}
	if endpoint == "" {
		return
	}

	st, exists := pp.stats[endpoint]
	if !exists {
		st = &pathStat{}
		pp.stats[endpoint] = st
	}
	st.lastTry = now
	if ok {
		st.ok = true
		st.okAt = now
		st.rtt = rtt
		st.rttAt = now
		st.fail = 0
	} else {
		st.fail++
		if st.fail >= 3 {
			st.ok = false
		}
	}
}

// NeedProbe 返回是否需要主动探测某个 peer 的非当前候选。
// 当当前线路已稳定（有近期握手），且超过 pathProbeInterval 未探测其他候选时返回 true。
func (s *pathSelector) NeedProbe(pubkey string, lastHandshake int64, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	pp := s.peers[pubkey]
	if pp == nil || len(pp.candidates) <= 1 {
		return false
	}
	hasRecentHS := lastHandshake > 0 && now.Sub(time.Unix(lastHandshake, 0)) < handshakeStaleAfter
	if !hasRecentHS {
		return false
	}
	if now.Sub(pp.lastProbe) < pathProbeInterval {
		return false
	}
	pp.lastProbe = now
	return true
}

// Current 返回某个 peer 当前选中的 endpoint。
func (s *pathSelector) Current(pubkey string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pp := s.peers[pubkey]; pp != nil {
		return pp.current
	}
	return ""
}

// endpointReachable 快速判断一个 UDP endpoint 是否能解析（仅格式检查）。
func endpointReachable(addr string) bool {
	_, err := net.ResolveUDPAddr("udp", addr)
	return err == nil
}
