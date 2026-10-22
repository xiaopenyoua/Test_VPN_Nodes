package probe

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// ── 出口地区层：口径参考 OpenClash 的流媒体嗅探 ────────────────────────────
//
// 参考实现：openclash_streaming_unlock.lua 的 openai_unlock_test / fetch_trace_region。
// 它用纯 curl（完全不开无头浏览器）判「地区是否解锁」，顺序是：
//  1. GET api.openai.com/compliance/cookie_requirements，带 Authorization: Bearer null，
//     正文命中 unsupported_country → 判「封」；
//  2. GET ios.chat.openai.com/，正文含 vpn → 判「封」；
//  3. 否则判「解锁」，再从 chat.openai.com/cdn-cgi/trace 的 loc= 读出口地区。
//
// 这套口径能回答「这个出口地区被支持吗」，回答不了「页面现在能不能打开」。本机实测：
// trace 报 loc=SG/colo=SIN/warp=on，cookie_requirements 回 {"cookie_consent_required":false}，
// 按 OpenClash 会判「解锁」——而真浏览器打开 chatgpt.com 是 403「Unable to load site …
// try turning it off」。病因不在地区，在 warp=on：出口是 Cloudflare WARP 的共享 IP，
// 被 OpenAI 的 Cloudflare 规则按 IP 段直接拒。
//
// 所以本文件实现的是**证据补充层**，不是结论层：
//   - 拿不到硬证据（unsupported_country）就绝不改 Status；
//   - loc 只作 evidence 提示，措辞带「疑似」，不单独定性；
//   - warp=on 必须写进 evidence，并把 NextStep 换成可执行的真话。
//
// 第 2 条判据（ios.chat.openai.com 正文含 vpn）刻意没有搬过来：实测该地址返回 403、
// 正文只有 77 字节且不含 "vpn"，在本环境下恒为假——搬过来只会多一次请求和一份幻觉信心。

// traceMaxBody 只需前 8KB：真实 trace 正文不到 400 字节。给足上限是为了容忍
// 中间层插页后仍能读到 k=v 行，而不是为了读全。
const traceMaxBody = 8 << 10

// openAIComplianceURL 是 OpenAI 自述地区策略的接口（OpenClash 用的同一个）。
const openAIComplianceURL = "https://api.openai.com/compliance/cookie_requirements"

// parseTrace 解析 Cloudflare /cdn-cgi/trace 的 k=v 逐行文本。
// 抽成纯函数是为了脱离网络单测：解析一旦错位（比如把 warp=off 读成 on），
// 整条结论会反过来，而这种错误在真机上是看不出来的。
func parseTrace(body []byte) (loc, colo string, warp bool) {
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		// trace 的每一行都是裸的 key=value；k 里出现尖括号或空白说明这行是 HTML
		// （例如 gemini.google.com 回的 Google 404 页），必须整行丢弃，
		// 否则页面正文里随便一句 "loc=XX" 都会被当成出口地区。
		if k == "" || strings.ContainsAny(k, "<> \t\"'") {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "loc":
			loc = strings.ToUpper(v)
		case "colo":
			colo = strings.ToUpper(v)
		case "warp":
			// trace 报的是 on/off 明文；其它值（含缺失）一律按"不是 WARP"处理，
			// 宁可漏报也不要把普通节点误标成 WARP——那会把用户引向错误的换节点动作。
			warp = strings.EqualFold(v, "on")
		}
	}
	return loc, colo, warp
}

// FetchTrace 问一次 Cloudflare：目标站看到的出口地区（loc）、接入机房（colo）、
// 是否 WARP 共享出口（warp）。ok=false 表示这次没拿到有效 trace（请求失败、
// 非 200、或正文里没有 loc），调用方据此保持沉默，不得凭空填地区。
func FetchTrace(ctx context.Context, tr *http.Transport, host string) (loc, colo string, warp bool, ok bool) {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")
	if host == "" {
		return "", "", false, false
	}

	f := Fetch(ctx, tr, FetchOpts{
		URL:     "https://" + host + "/cdn-cgi/trace",
		Header:  map[string]string{"Accept": "text/plain,*/*"},
		MaxBody: traceMaxBody,
	})
	// 注意：gemini.google.com 也走同一个函数，但它背后不是 Cloudflare，
	// /cdn-cgi/trace 回的是 Google 的 404 页 —— 这里必须靠 status 与 loc 双重把关。
	if f == nil || f.Err != nil || f.Status != http.StatusOK {
		return "", "", false, false
	}
	loc, colo, warp = parseTrace(f.Body)
	if loc == "" {
		return "", "", false, false
	}
	return loc, colo, warp, true
}

// RegionBlockedByOpenAI 只认 OpenAI 自述的硬证据：正文含 unsupported_country。
// 刻意不扩大到 "unsupported country"（空格版）之类的模糊词：cookie_requirements
// 正常返回 {"cookie_consent_required": false}，一旦放宽匹配，任何带这两个词的
// 错误页/插页都会把"能打开"误判成"地区封"，而地区封是重度降级结论。
func RegionBlockedByOpenAI(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	return strings.Contains(strings.ToLower(string(body)), "unsupported_country")
}

// fetchOpenAIRegionVerdict 问一次 OpenAI 自述：当前出口地区是否受支持。
// ok=false 表示这次没问到（传输错误 / 空正文），此时不允许据此做任何判断。
//
// 刻意不要求 200：unsupported_country 本身是错误响应（常见 403），
// 只有拿到 200 才判的写法会正好错过唯一想抓的那条证据。
func fetchOpenAIRegionVerdict(ctx context.Context, tr *http.Transport) (blocked bool, ok bool) {
	f := Fetch(ctx, tr, FetchOpts{
		URL: openAIComplianceURL,
		Header: map[string]string{
			// "Bearer null" 是 OpenClash 的原样口径：伪造一个"已登录但无 key"的请求，
			// 服务端才会走到地区判定，而不是先回一句与地区无关的 401。
			"Authorization": "Bearer null",
			"Content-Type":  "application/json",
			"Accept":        "*/*",
		},
		MaxBody: 4 << 10,
	})
	if f == nil || f.Err != nil || len(f.Body) == 0 {
		return false, false
	}
	return RegionBlockedByOpenAI(f.Body), true
}

// traceEvidence 把 trace 结果翻成一行证据。warp 一定要显式写出来：
// 它是"共享 IP 被 CF 规则拒绝"的病因，而不是地区问题；不写清楚，
// 用户只会看到"地区 SG"然后以为换个地区名就好。
func traceEvidence(loc, colo string, warp bool) string {
	line := "Cloudflare 看到出口 loc=" + loc
	if colo != "" {
		line += " · colo=" + colo
	}
	if warp {
		line += " · warp=on"
	} else {
		line += " · warp=off"
	}
	return line
}

// warpNextStep 是 warp=on 时唯一值得做的动作。
// 措辞必须是真话：重试同一个节点不会有任何变化，换非 WARP 节点才有用。
func warpNextStep() string {
	return fmt.Sprintf("出口是 Cloudflare WARP 共享 IP，OpenAI 的 Cloudflare 规则会直接拒绝这类 IP"+
		"（表现为 %d / Unable to load site）。换一个非 WARP 的节点才有用，重试同一节点不会变", http.StatusForbidden)
}
