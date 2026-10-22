package probe

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"regexp"
	"strings"
	"time"
)

// BrowserUA 使用桌面浏览器 UA，避免站点对未知 UA 返回简化页或直接 403。
const BrowserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// FetchOpts 描述一次带指标采集的请求。
type FetchOpts struct {
	URL     string
	Method  string
	Header  map[string]string
	MaxBody int64
	Body    []byte
}

// FetchResult 同时承载指标与响应内容。
type FetchResult struct {
	Phases     Phase
	Status     int
	FinalURL   string
	TLSVersion string
	RemoteAddr string
	Header     http.Header
	Body       []byte
	BodyLen    int64
	Declared   int64
	// Truncated 表示读取被 MaxBody 上限截断，而不是链路出问题。
	// 若不区分，首页大于采样窗口时会被误判成"响应被劫持"。
	Truncated bool
	Redirects []string
	Err       error
}

// Fetch 执行一次请求，采集 DNS / TCP / TLS / TTFB 分阶段耗时。
func Fetch(ctx context.Context, tr *http.Transport, o FetchOpts) *FetchResult {
	res := &FetchResult{}
	method := o.Method
	if method == "" {
		method = http.MethodGet
	}
	maxBody := o.MaxBody
	if maxBody <= 0 {
		maxBody = 64 << 10
	}

	var (
		start               = time.Now()
		dnsStart, dnsDone   time.Time
		connStart, connDone time.Time
		tlsStart, tlsDone   time.Time
		firstByte           time.Time
		gotConn             time.Time
	)

	ms := func(t time.Time) float64 {
		if t.IsZero() {
			return 0
		}
		return float64(t.Sub(start).Microseconds()) / 1000
	}

	trace := &httptrace.ClientTrace{
		DNSStart:          func(httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:           func(httptrace.DNSDoneInfo) { dnsDone = time.Now() },
		ConnectStart:      func(string, string) { connStart = time.Now() },
		ConnectDone:       func(string, string, error) { connDone = time.Now() },
		TLSHandshakeStart: func() { tlsStart = time.Now() },
		TLSHandshakeDone: func(cs tls.ConnectionState, err error) {
			tlsDone = time.Now()
			res.TLSVersion = tlsVersionName(cs.Version)
		},
		GotConn: func(info httptrace.GotConnInfo) {
			gotConn = time.Now()
			res.RemoteAddr = info.Conn.RemoteAddr().String()
		},
		GotFirstResponseByte: func() { firstByte = time.Now() },
	}

	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), method, o.URL, bodyReader(o.Body))
	if err != nil {
		res.Err = err
		return res
	}
	req.Header.Set("User-Agent", BrowserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")
	for k, v := range o.Header {
		req.Header.Set(k, v)
	}

	client := &http.Client{
		Transport: tr,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) > 3 {
				return errors.New("重定向次数过多")
			}
			res.Redirects = append(res.Redirects, r.URL.String())
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		res.Err = err
		res.Phases = Phase{DNS: Round(ms(dnsDone)-ms(dnsStart), 1), TCP: Round(ms(connDone)-ms(connStart), 1),
			TLS: Round(ms(tlsDone)-ms(tlsStart), 1), Total: Round(ms(time.Now()), 1)}
		return res
	}
	defer resp.Body.Close()

	res.Status = resp.StatusCode
	res.FinalURL = resp.Request.URL.String()
	res.Header = resp.Header
	res.Declared = resp.ContentLength

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	res.Body = body
	res.BodyLen = int64(len(body))
	// 读满上限即视为"可能被截断"，此时内容长度不完整，不能据此判断链路异常
	res.Truncated = int64(len(body)) >= maxBody
	if err != nil && len(body) == 0 {
		res.Err = err
	}

	dnsMs := 0.0
	if !dnsStart.IsZero() && !dnsDone.IsZero() {
		dnsMs = ms(dnsDone) - ms(dnsStart)
	}
	tcpMs := 0.0
	if !connStart.IsZero() && !connDone.IsZero() {
		tcpMs = ms(connDone) - ms(connStart)
	}
	tlsMs := 0.0
	if !tlsStart.IsZero() && !tlsDone.IsZero() {
		tlsMs = ms(tlsDone) - ms(tlsStart)
	}
	ttfb := 0.0
	if !firstByte.IsZero() {
		ttfb = ms(firstByte)
	} else if !gotConn.IsZero() {
		ttfb = ms(time.Now())
	}
	res.Phases = Phase{
		DNS:   Round(dnsMs, 1),
		TCP:   Round(tcpMs, 1),
		TLS:   Round(tlsMs, 1),
		TTFB:  Round(ttfb, 1),
		Total: Round(ms(time.Now()), 1),
	}
	return res
}

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS10:
		return "TLS 1.0"
	case 0:
		return ""
	default:
		return fmt.Sprintf("TLS 0x%04x", v)
	}
}

var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

func bodyReader(b []byte) io.Reader {
	if len(b) == 0 {
		return nil
	}
	return bytes.NewReader(b)
}

func titleOf(body []byte) string {
	m := titleRe.FindSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	t := strings.Join(strings.Fields(string(m[1])), " ")
	if len(t) > 120 {
		t = t[:120]
	}
	return t
}

var challengeMarkers = []string{
	"just a moment",
	"enable javascript and cookies to continue",
	"checking your browser before accessing",
	"cf-chl",
	"challenge-platform",
	"verify you are human",
	"ddos protection by",
	"unusual traffic from your computer",
}

// cfDenyMarkers 是「Cloudflare 直接拒绝这个出口 IP」的特征，与上面的 JS 验证墙
// 是截然不同的两件事：
//   - 验证墙（challenge）：浏览器执行 JS 就能过，用户打开是正常的 —— 判成"被封"就是假阴性
//   - 直接拒绝（本组）：浏览器同样打不开，页面明说"试试关掉 VPN"。判成降级才是事实
//
// 2026-10-22 真机对照取证：本机出口（Cloudflare 段 IPv6）访问 chatgpt.com，
// 真浏览器与纯 Go 探测双双得到 403 + "Unable to load site / If you are using a VPN,
// try turning it off" + Ray ID，且响应头 cf-mitigated 为空 —— 结论一致，都是硬拒绝。
var cfDenyMarkers = []string{
	"unable to load site",
	"if you are using a vpn, try turning it off",
	"sorry, you have been blocked",
}

var regionMarkers = []string{
	"not available in your country",
	"isn't available in your country",
	"is not available in your country",
	"not currently supported in your country",
	"unsupported_country",
	"unsupported country",
	"not available in your region",
	"isn't available in your region",
	"service is not available in your location",
	"该地区不可用",
}

var hijackMarkers = []string{
	"您访问的网站",
	"该网站已被屏蔽",
	"blocked by administrator",
	"connection has been reset",
	"网络连接被重置",
}

// hasToken 判断页面是否包含站点特征词（标题可能落在采样窗口之外）。
func hasToken(body []byte, token string) bool {
	if len(body) == 0 {
		return false
	}
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, token) || strings.Contains(strings.ToLower(titleOf(body)), token)
}

func containsAny(body []byte, markers []string) string {
	if len(body) == 0 {
		return ""
	}
	lower := strings.ToLower(string(body))
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return m
		}
	}
	return ""
}

// Classify 把响应翻译成可解释的结论。
func Classify(id string, f *FetchResult) (Status, []string, string) {
	ev := []string{}
	if f == nil {
		return StatusFailed, []string{"无响应"}, "探测未执行"
	}
	if len(f.Redirects) > 0 {
		ev = append(ev, fmt.Sprintf("重定向 %d 次，最终落地 %s", len(f.Redirects), shortenURL(f.FinalURL)))
	}

	if f.Err != nil {
		ev = append(ev, "错误: "+f.Err.Error())
		return classifyError(f.Err), ev, errorHint(f.Err)
	}

	ev = append(ev, fmt.Sprintf("HTTP %d · %s", f.Status, shortenURL(f.FinalURL)))
	if title := titleOf(f.Body); title != "" {
		ev = append(ev, "页面标题: "+title)
	}
	// 只有在没有主动截断的前提下，实收字节远小于声明长度才说明链路被中断。
	// 采样窗口写满时 f.Truncated 为真，此时比较毫无意义——否则大首页一律被误报劫持。
	if !f.Truncated && f.Declared > 0 && f.BodyLen > 0 && f.BodyLen < f.Declared && f.BodyLen < f.Declared/2 && f.Declared > 4096 {
		ev = append(ev, fmt.Sprintf("响应体被截断：声明 %d 字节，实收 %d 字节", f.Declared, f.BodyLen))
		return StatusHijacked, ev, "响应被中断，疑似链路劫持或强制断流"
	}
	if m := containsAny(f.Body, hijackMarkers); m != "" {
		ev = append(ev, "命中劫持特征: "+m)
		return StatusHijacked, ev, "返回内容被中间设备替换"
	}
	// Cloudflare 风控优先看响应头，body 正则只作补充（实测 chatgpt.com 返回 403 + cf-mitigated: challenge）
	if cf := strings.ToLower(f.Header.Get("Cf-Mitigated")); cf == "challenge" {
		ev = append(ev, "响应头 cf-mitigated: challenge")
		return StatusChallenge, ev, "链路是通的，站点发了一张 JS 验证页；浏览器会自动过，纯探测过不去——请用浏览器实开一次再判"
	}
	// Cloudflare 直接拒绝出口 IP：浏览器同样打不开，属硬拒绝。
	// 必须排在通用风控特征**之前**：这类拒绝页的标题就是 "Attention Required"，
	// 而 "attention required" 曾被当成验证墙特征，于是硬拒绝被误判成"人机验证"——
	// 用户看到"需要验证"会以为刷新一下就好，实际换节点才有用。
	if m := containsAny(f.Body, cfDenyMarkers); m != "" {
		ev = append(ev, "命中 Cloudflare 拒绝特征: "+m)
		return StatusDegraded, ev, "Cloudflare 判定当前出口 IP 是代理/VPN 并直接拒绝（浏览器同样打不开，不是地区限制、也不是你的本地网络坏了）：换一个没被标记的出口节点，原地重试同一个节点不会变"
	}
	if m := containsAny(f.Body, challengeMarkers); m != "" {
		ev = append(ev, "命中人机验证特征: "+m)
		return StatusChallenge, ev, "链路是通的，站点发了一张 JS 验证页；浏览器会自动过，纯探测过不去——请用浏览器实开一次再判"
	}
	if m := containsAny(f.Body, regionMarkers); m != "" {
		ev = append(ev, "命中地区限制特征: "+m)
		return StatusRegionBlock, ev, "出口 IP 所在地区不被该服务支持"
	}

	switch f.Status {
	case 204:
		return StatusOK, ev, "网络通"
	case 200:
		switch id {
		case "P3":
			if hasToken(f.Body, "chatgpt") {
				ev = append(ev, "ChatGPT 首页正常返回")
				return StatusOK, ev, "ChatGPT 网页可正常打开"
			}
			ev = append(ev, "200 但未命中 ChatGPT 特征，无法确认业务可用")
			return StatusInconclusive, ev, "能连上，但无法确认是 ChatGPT 本体（可能需要 JS 渲染或被中间层改写）"
		case "P4":
			if hasToken(f.Body, "gemini") {
				ev = append(ev, "Gemini 首页正常返回")
				return StatusOK, ev, "Gemini 网页可正常打开"
			}
			ev = append(ev, "200 但未命中 Gemini 特征，无法确认业务可用")
			return StatusInconclusive, ev, "能连上，但无法确认是 Gemini 本体（可能需要 JS 渲染或被中间层改写）"
		case "P2":
			return StatusOK, ev, "YouTube 首页可达"
		default:
			return StatusOK, ev, "可达"
		}
	case 401:
		if id == "P5" {
			ev = append(ev, "401 说明已抵达 OpenAI 服务端（仅缺 API Key）")
			return StatusAuthRequired, ev, "网络与服务可达，仅缺 API Key"
		}
		ev = append(ev, "需要登录态")
		return StatusAuthRequired, ev, "需要登录，网络可达"
	case 403:
		if strings.Contains(strings.ToLower(string(f.Body)), "unsupported_country") {
			return StatusRegionBlock, ev, "OpenAI 判定当前出口地区不受支持"
		}
		return StatusDegraded, ev, "被目标站点拒绝（403），可能是风控或地区策略"
	case 429:
		ev = append(ev, "限流（429）：网络是通的，不是代理故障")
		return StatusRateLimited, ev, "目标站点限流，网络可用"
	case 451:
		return StatusRegionBlock, ev, "因法律原因不可用（451）"
	case 502, 503, 504:
		return StatusDegraded, ev, "目标站点网关异常，与本地链路关系不大"
	default:
		if f.Status >= 500 {
			return StatusDegraded, ev, fmt.Sprintf("服务端错误 %d", f.Status)
		}
		if f.Status >= 400 {
			return StatusDegraded, ev, fmt.Sprintf("返回 %d", f.Status)
		}
		return StatusOK, ev, "可达"
	}
}

func classifyError(err error) Status {
	msg := strings.ToLower(err.Error())
	switch {
	// 用户主动取消不是链路故障，必须与"打不开"区分开
	case strings.Contains(msg, "context canceled"), strings.Contains(msg, "operation was canceled"):
		return StatusCancelled
	case strings.Contains(msg, "x509"), strings.Contains(msg, "certificate"),
		strings.Contains(msg, "tls:"), strings.Contains(msg, "handshake failure"):
		return StatusHijacked
	case strings.Contains(msg, "context deadline exceeded"), strings.Contains(msg, "timeout"),
		strings.Contains(msg, "i/o timeout"):
		return StatusFailed
	default:
		return StatusFailed
	}
}

// isTransient 判断一次失败是否属于"这次恰好没连上"的瞬态，
// 而不是目标站点的稳定策略（风控 / 地区限制 / 证书异常）。
//
// 隧道链路上 EOF、连接重置、握手超时是高频瞬发事件。把它们当定性证据，
// 就会产出假阴性：工具说"ChatGPT 不可达 / 典型的阻断特征"，而用户自己
// 刷新浏览器就打开了。这类失败必须先重试，连续失败才允许下结论。
func isTransient(st Status, err error) bool {
	if err == nil {
		return false
	}
	// 只有"失败/无法确认"才谈得上瞬态；已拿到策略性结论的不重试
	if st != StatusFailed && st != StatusInconclusive {
		return false
	}
	msg := strings.ToLower(err.Error())
	// 证书类错误是确定性的，重试结果不会变
	if strings.Contains(msg, "x509") || strings.Contains(msg, "certificate") {
		return false
	}
	for _, m := range []string{
		"eof", "connection reset", "broken pipe", "i/o timeout",
		"timeout", "use of closed network connection", "temporary",
	} {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

func errorHint(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context canceled"), strings.Contains(msg, "operation was canceled"):
		return "本次探测已被取消，不代表链路结果"
	case strings.Contains(msg, "x509"), strings.Contains(msg, "certificate"):
		return "TLS 证书校验失败，链路可能被中间人劫持或系统时间异常"
	case strings.Contains(msg, "context deadline exceeded"), strings.Contains(msg, "timeout"):
		return "请求超时，本次没有等到响应（可能是链路瞬断、拥塞或被人为丢包）"
	case strings.Contains(msg, "connection reset"), strings.Contains(msg, "eof"):
		// 不能直接说"典型的阻断特征"：重试过后仍然失败才谈得上阻断，
		// 而且链路自身瞬断也会产生完全一样的 EOF。
		return "连接被对端中断（EOF/重置），本次未取到响应"
	case strings.Contains(msg, "no such host"), strings.Contains(msg, "dns"):
		return "DNS 解析失败，域名可能被污染"
	case strings.Contains(msg, "connection refused"):
		return "连接被拒绝，代理端口可能未监听"
	default:
		return "请求失败"
	}
}

func shortenURL(raw string) string {
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	if len(raw) > 60 {
		return raw[:60] + "…"
	}
	return raw
}
