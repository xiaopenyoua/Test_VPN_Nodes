// Package probe 定义探测项的指标模型与执行逻辑。
package probe

import (
	"math"
	"sort"
)

// Status 是探测结论的枚举。刻意区分"慢"与"不可用"。
type Status string

const (
	StatusPending      Status = "pending"
	StatusRunning      Status = "running"
	StatusOK           Status = "ok"
	StatusDegraded     Status = "degraded"
	StatusInconclusive Status = "inconclusive" // 拿到 200 但无法验证业务可用
	StatusRateLimited  Status = "rate_limited" // 限流：网络通，非代理故障
	StatusChallenge    Status = "challenge"
	StatusRegionBlock  Status = "region_blocked"
	StatusAuthRequired Status = "auth_required"
	StatusHijacked     Status = "hijacked"
	StatusFailed       Status = "failed"
	StatusSkipped      Status = "skipped"
	StatusCancelled    Status = "cancelled"
)

// Sample 是一个吞吐采样点（相对时间，毫秒）。
type Sample struct {
	T     float64 `json:"t"`
	Mbps  float64 `json:"mbps"`
	Bytes int64   `json:"bytes"`
}

// Phase 是网络分层耗时（毫秒）。
type Phase struct {
	DNS   float64 `json:"dnsMs"`
	TCP   float64 `json:"tcpMs"`
	TLS   float64 `json:"tlsMs"`
	TTFB  float64 `json:"ttfbMs"`
	Total float64 `json:"totalMs"`
}

// Result 是单个探测项的完整结果。
type Result struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Group       string   `json:"group"` // speed | stability | ai
	Status      Status   `json:"status"`
	Headline    string   `json:"headline"` // 主指标展示值，如 "42.3 Mbps"
	Subline     string   `json:"subline"`  // 副标题，如 "dl.google.com · 24 MB"
	Phases      Phase    `json:"phases"`
	HTTPStatus  int      `json:"httpStatus"`
	FinalURL    string   `json:"finalUrl"`
	RemoteAddr  string   `json:"remoteAddr"`
	TLSVersion  string   `json:"tlsVersion"`
	Bytes       int64    `json:"bytes"`
	Mbps        float64  `json:"mbps"`
	PeakMbps    float64  `json:"peakMbps"`
	P50Mbps     float64  `json:"p50Mbps"`
	P90Mbps     float64  `json:"p90Mbps"`
	CV          float64  `json:"cv"`
	TTFB        float64  `json:"ttfbMs"`
	Jitter      float64  `json:"jitterMs"`
	SuccessPct  float64  `json:"successPct"`
	ConnCount   int      `json:"connCount"` // 实际观测到的独立 TCP 连接数，用来证明"N 并发"名副其实
	Stalled     bool     `json:"stalled"`   // 下载中途出现持续性零吞吐，均值会把它摊平
	MediaPath   string   `json:"mediaPath"` // 媒体链路走到哪一级：unavailable/page_only/thumb_only/gvs_handshake
	Samples     []Sample `json:"samples"`
	Evidence    []string `json:"evidence"`
	Error       string   `json:"error"`
	Score       float64  `json:"score"`
	Attribution string   `json:"attribution"`
	NextStep    string   `json:"nextStep"`
	DurationMs  float64  `json:"durationMs"`
}

// Meta 是探测项的静态元信息，供前端渲染。
type Meta struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
	Desc  string `json:"desc"`
}

// Metas 按前端展示顺序排列。
var Metas = []Meta{
	{"P3", "ChatGPT 网页", "ai", "chatgpt.com 首页与静态资源，识别 Cloudflare 风控与地区限制"},
	{"P4", "Gemini 网页", "ai", "gemini.google.com 首页，识别地区不可用"},
	{"P5", "OpenAI API", "ai", "api.openai.com/v1/models，401 也代表网络与服务可达"},
	{"P6", "国际链路基线", "stability", "google.com / gstatic.com 多次采样，测 RTT 与抖动"},
	{"P2", "YouTube 链路", "speed", "youtube.com 首屏 + ytimg 缩略图 + googlevideo 分片（可用时）"},
	{"P1", "Google 大文件下载", "speed", "dl.google.com 流式下载，测吞吐、TTFB 与速度衰减"},
}

// MetaByID 返回元信息。
func MetaByID(id string) Meta {
	for _, m := range Metas {
		if m.ID == id {
			return m
		}
	}
	return Meta{ID: id, Name: id}
}

// Round 保留 n 位小数。
func Round(v float64, n int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	p := math.Pow(10, float64(n))
	return math.Round(v*p) / p
}

// Percentile 计算分位数（p 取 0-100），输入需已排序或内部复制排序。
func Percentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	cp := make([]float64, len(vals))
	copy(cp, vals)
	sort.Float64s(cp)
	if len(cp) == 1 {
		return cp[0]
	}
	idx := (p / 100) * float64(len(cp)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi {
		return cp[lo]
	}
	return cp[lo] + (cp[hi]-cp[lo])*(idx-float64(lo))
}

// Mean 求均值。
func Mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var s float64
	for _, v := range vals {
		s += v
	}
	return s / float64(len(vals))
}

// StdDev 求标准差。
func StdDev(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	m := Mean(vals)
	var s float64
	for _, v := range vals {
		d := v - m
		s += d * d
	}
	return math.Sqrt(s / float64(len(vals)-1))
}
