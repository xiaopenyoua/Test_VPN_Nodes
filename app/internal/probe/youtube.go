package probe

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ============================================================================
// P2 口径（先读这段）
//
// 用户诉求：通过这套代理能不能看 YouTube 视频。理想做法是拿 googlevideo 分片
// 直链、Range 拉 1~3 秒算真实 Mbps。真机实测（2026-10）证明匿名环境拿不到直链：
//
//  1. POST youtubei/v1/player（WEB / MWEB）→ 200，但 playabilityStatus.status=
//     LOGIN_REQUIRED，reason="Sign in to confirm you're not a bot"，
//     formats 与 adaptiveFormats 均为 0。
//  2. ANDROID / IOS client → 400 FAILED_PRECONDITION（需 PO Token / 设备认证）。
//  3. TVHTML5_SIMPLY_EMBEDDED_PLAYER → status=ERROR，"no longer supported"。
//  4. watch 页能抠到 initplayback(onesie) 直链，但 GET→403、POST 空体→411，
//     Content-Type: application/vnd.yt-ump，需完整 onesie 协议体，不适合作探测。
//
// 因此本项**不做吞吐实测、不产出分数**，只回答三件可复核的事实：
//   A. 网页与播放页能否打开（网页可达）
//   B. 播放器自己怎么说（playabilityStatus：正常 / 要求登录验证 / 地区受限 / 无法播放）
//   C. 视频 CDN（*.googlevideo.com）能否连上
//
// C 的价值：`https://<gvs host>/generate_204` 实测返回 204 且 `Server: gvs 1.0`，
// 零字节、可复核，是「视频 CDN 通不通」最干净的信号 —— 网页能开但视频 CDN 不通，
// 正是"能刷首页、看不了视频"的真实故障形态。
//
// 硬约束（团队裁定）：任何"失败"结论必须先排除瞬态。连接层一次 EOF / reset /
// TLS 握手超时只说明"这一次没连上"，不足以定性；必须重试、措辞中性、归因待定。
// ============================================================================

// ytVideoID 用 Big Buck Bunny：公开、长期稳定、无版权/地区波动。
const ytVideoID = "aqz-KE-bpKQ"

// MediaPath 取值，与 06 §1.4 的五值枚举对齐。
// 本实现没有 gvt1_throughput / googlevideo_real 两档（见文件头：不测吞吐），
// thumb_only 用于"网页与图片 CDN 通、但页面没给出分片主机"的情形。
const (
	ytPathUnavailable = "unavailable"
	ytPathPageOnly    = "page_only"
	ytPathThumb       = "thumb_only"
	ytPathGVS         = "gvs_handshake"
)

// 播放页与缩略图的固定地址。视频 ID 固定，地址可复核，不做任何随机化。
const (
	ytWatchURL = "https://www.youtube.com/watch?v=" + ytVideoID
	ytThumbURL = "https://i.ytimg.com/vi/" + ytVideoID + "/hqdefault.jpg"
)

var (
	// playabilityStatus 实测位于 watch 页 ~773KB 处，采样窗口必须 ≥1MiB，
	// 否则永远抽不到；而"抽不到"只能降级为"无法确认"，绝不能当成"可用"。
	ytPbStatusRe = regexp.MustCompile(`"playabilityStatus":\s*\{\s*"status"\s*:\s*"([A-Z_]+)"`)
	ytPbReasonRe = regexp.MustCompile(`"playabilityStatus":\s*\{[^{}]*?"reason"\s*:\s*"([^"]*)"`)
	// 页面里以 https:\/\/rr3---sn-xxx.googlevideo.com 形式出现，只取主机名。
	ytGvsHostRe = regexp.MustCompile(`[a-z0-9][a-z0-9\-\.]*\.googlevideo\.com`)
)

// ytRetryGap 是重试间隔。间隔取 900ms 是为了跨出同一个瞬时窗口：
// 实测目标站在秒级抖动，背靠背重试等于在同一次抖动里取样，等于没重试。
const ytRetryGap = 900 * time.Millisecond

// ytEdgeSlice 是「向视频边缘握手」单次尝试的预算。
// 3s 足够完成 DNS+TCP+TLS（实测正常时 200~400 ms），又能在预算内做第二次真尝试。
const ytEdgeSlice = 3 * time.Second

// ytFetch 带重试地发一次请求，返回结果与实际尝试次数。
//
// 为什么必须重试：一次 TLS handshake timeout / EOF / connection reset 只能说明
// "这一次没连上"。实测同一分钟里 P2 单次握手超时、P1 仍在持续下载、P6 抖动 286ms，
// 链路当时在剧烈抖动 —— 单次失败被写成"链路可能被部分拦截"是无依据的诊断。
func ytFetch(ctx context.Context, tr *http.Transport, o FetchOpts, attempts int) (*FetchResult, int) {
	if attempts < 1 {
		attempts = 1
	}
	var last *FetchResult
	for i := 0; i < attempts; i++ {
		last = Fetch(ctx, tr, o)
		if !ytRetryable(last) {
			return last, i + 1
		}
		if i == attempts-1 || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			return last, i + 1
		case <-time.After(ytRetryGap):
		}
	}
	return last, attempts
}

// ytRetryable 只对"可能是一次抖动"的形态重试。
// 已拿到明确业务结论（200/403/404/451…）不重试，重试只会得到同一个答案。
func ytRetryable(f *FetchResult) bool {
	if f == nil {
		return true
	}
	if f.Err != nil {
		return true
	}
	return f.Status >= 500 || f.Status == 429 || f.Status == 0
}

// ytStatusOfErr 只做保守映射：证书类错误是硬证据，其余一律 failed（不是 hijacked）。
// 特别注意 "tls handshake timeout" 不含 "tls:"，因此不会落进 hijacked 分支。
func ytStatusOfErr(err error) Status {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context canceled"), strings.Contains(msg, "operation was canceled"):
		return StatusCancelled
	case strings.Contains(msg, "x509"), strings.Contains(msg, "certificate"),
		strings.Contains(msg, "tls:"), strings.Contains(msg, "handshake failure"):
		return StatusHijacked
	default:
		return StatusFailed
	}
}

// ytErrHint 解释错误，但绝不越过证据下结论。
func ytErrHint(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "x509"), strings.Contains(msg, "certificate"):
		return "证书校验失败：证书链本身是硬证据，可判定链路被中间人替换"
	case strings.Contains(msg, "context canceled"):
		return "本次探测已被取消，不代表链路结果"
	case strings.Contains(msg, "no such host"):
		return "DNS 解析失败，域名可能被污染"
	case strings.Contains(msg, "connection refused"):
		return "连接被拒绝，代理端口可能未监听"
	default:
		return "已重试仍未连通。单次连接层失败不足以定性，需对比 P1/P6 与直连对照才能区分代理抖动与目标侧问题"
	}
}

// ytPlayability 抽播放器自述状态与原因。status 为空表示页面里没有这段声明。
func ytPlayability(body []byte) (status, reason string) {
	if m := ytPbStatusRe.FindSubmatch(body); len(m) > 1 {
		status = string(m[1])
	}
	if m := ytPbReasonRe.FindSubmatch(body); len(m) > 1 {
		reason = ytCleanText(m[1], 160)
	}
	return status, reason
}

// ytCleanText 还原 JSON 转义并截断，避免把整页内容带进证据链。
func ytCleanText(raw []byte, max int) string {
	s := strings.ReplaceAll(string(raw), `\u0026`, "&")
	s = strings.ReplaceAll(s, `\/`, "/")
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.Join(strings.Fields(s), " ")
	if max > 0 && len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// ytTitle 取 <title>，仅用于让证据链可复核。
var ytTitleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

func ytTitle(body []byte) string {
	m := ytTitleRe.FindSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return ytCleanText(m[1], 120)
}

// ytConsentMarkers 是 cookie/条款同意页的特征词。命中即说明会话被人为拦截，
// 网页与播放都过不去，不属于"链路正常"。
var ytConsentMarkers = []string{
	"before you continue to youtube",
	"consent.youtube.com",
	"we use cookies and data to",
	"同意 youtube",
}

// ytRegionMarkers 是 YouTube 自己给出的地区限制文案（而非我们推断）。
var ytRegionMarkers = []string{
	"not made this video available in your country",
	"not available in your country",
	"blocked it in your country",
	"not available in your region",
	"在您所在的国家/地区不可用",
	"该地区不可用",
}

// ytHit 返回首个命中的特征词，未命中返回空串。
func ytHit(body []byte, markers []string) string {
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

// ytPlayerMarkers 是「页面确实回了播放器初始化数据」的指纹。
// 只看状态码不够：中间层塞一个 200 的假页面同样能过，必须抓到本体特征。
var ytPlayerMarkers = []string{"ytinitialplayerresponse", "ytinitialdata"}

// ytBotMarkers 是 YouTube 自己给出的「先证明你不是机器人」文案。
var ytBotMarkers = []string{
	"sign in to confirm you're not a bot",
	"sign in to confirm you’re not a bot",
	"confirm you're not a bot",
	"unusual traffic",
}

// ytHasPlayer 判断页面是否包含播放器初始化数据。
func ytHasPlayer(body []byte) bool { return ytHit(body, ytPlayerMarkers) != "" }

// ytRemain 返回父级预算还剩多少；没有 deadline 时给一个宽口径。
func ytRemain(ctx context.Context) time.Duration {
	if dl, ok := ctx.Deadline(); ok {
		return time.Until(dl)
	}
	return 60 * time.Second
}

// ytSlice 在父级预算里切一块给某一级用。
// 不切块的后果：第一级吃满整个预算，后两级根本没机会跑，
// 结论就会变成「媒体未测」——那不是测出来的，是被我们自己的超时饿死的。
func ytSlice(ctx context.Context, want time.Duration) (context.Context, context.CancelFunc) {
	if remain := ytRemain(ctx); remain < want {
		want = remain
	}
	// 下限 1.2s：比一次 TLS 握手还短，必然全误判成失败
	if want < 1200*time.Millisecond {
		want = 1200 * time.Millisecond
	}
	return context.WithTimeout(ctx, want)
}

func ytMin(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// ytClassifyPage 把播放页的响应翻译成结论。顺序即语义优先级：
// 先看「会话是否被人为拦住」（同意墙 / 人机验证 / 地区），再看状态码，最后看播放器自述。
func ytClassifyPage(f *FetchResult, pbStatus, pbReason string) (Status, string) {
	if f == nil {
		return StatusFailed, "探测未执行"
	}
	reason := strings.ToLower(pbReason)

	// 同意墙：能连上，但会话被拦在播放器之外，网页与播放都过不去
	if strings.Contains(strings.ToLower(f.FinalURL), "consent.youtube.com") || ytHit(f.Body, ytConsentMarkers) != "" {
		return StatusDegraded, "被跳到 cookie / 条款同意页，会话被拦在播放器之外"
	}
	// 「先证明你不是机器人」：这是 YouTube 对本工具这条**无 Cookie 匿名请求**的判词，
	// 不是对用户浏览器的判词。真机对照（2026-10-22，Chromium + 真实登录态）已证实：
	// 匿名 watch 页给出 LOGIN_REQUIRED / "Sign in to confirm you're not a bot" 的同一环境里，
	// 浏览器打开同一个视频 readyState=4、duration 634s、MSE blob 正常，完整可播。
	//
	// 因此这里绝不能判 challenge —— 那会把「能看 YouTube」写成「被封控」并拉低总分，
	// 正是用户报的"测试结果和实际不一致"。只有连播放器数据都拿不到（页面本身就是一张
	// 验证插页）时，才降级并明确要求人工实开确认。
	if ytHit(f.Body, ytBotMarkers) != "" || strings.Contains(reason, "not a bot") {
		if !ytHasPlayer(f.Body) {
			return StatusDegraded, "匿名访问被要求先验证「不是机器人」，且页面里没有播放器数据（浏览器登录后通常可播，需实开确认）"
		}
		// 有播放器数据就继续走状态码与 playabilityStatus，不在此定性
	}
	if ytHit(f.Body, ytRegionMarkers) != "" ||
		strings.Contains(reason, "country") || strings.Contains(reason, "region") {
		return StatusRegionBlock, "YouTube 自己判定该视频在当前出口地区不可用"
	}

	switch f.Status {
	case 200:
		// 指纹先于状态码：200 可能是被改写过的假页面
		if !ytHasPlayer(f.Body) {
			return StatusInconclusive, "页面返回 200，但没取到播放器初始化数据（ytInitialPlayerResponse / ytInitialData），无法确认是播放页本体"
		}
		switch pbStatus {
		case "":
			return StatusInconclusive, "取到了播放器初始化数据，但没有 playabilityStatus 声明，无法确认能否播放"
		case "OK":
			return StatusOK, "播放页可正常打开"
		case "LOGIN_REQUIRED":
			// 同上：匿名请求被要求登录 ≠ 浏览器不可播（已实机对照）。
			// 判 challenge 会直接产出假阴性，这里只作为提示，不参与定性。
			return StatusOK, "播放页可正常打开（匿名接口要求登录验证，不影响浏览器播放）"
		case "UNPLAYABLE", "ERROR", "LIVE_STREAM_OFFLINE":
			msg := "播放器自述无法播放：" + pbStatus
			if pbReason != "" {
				msg += "（" + pbReason + "）"
			}
			return StatusDegraded, msg
		default:
			return StatusOK, "播放页可正常打开"
		}
	case 429:
		return StatusRateLimited, "YouTube 限流（429），网络是通的"
	case 403:
		return StatusDegraded, "播放页被拒绝（403），可能是风控或地区策略"
	case 451:
		return StatusRegionBlock, "因法律原因不可用（451）"
	}
	if f.Status >= 500 {
		return StatusDegraded, fmt.Sprintf("YouTube 服务端错误 %d", f.Status)
	}
	if f.Status >= 400 {
		return StatusDegraded, fmt.Sprintf("播放页返回 %d", f.Status)
	}
	return StatusOK, "播放页可达"
}

// ytCancelled 统一产出「被取消」的结果，避免把取消渲染成链路故障。
func ytCancelled(res *Result, started time.Time) *Result {
	res.Status = StatusCancelled
	res.Headline = "已取消"
	res.Subline = "本次未产生结论"
	res.Score = statusScore(StatusCancelled)
	res.DurationMs = Round(float64(time.Since(started).Microseconds())/1000, 0)
	res.Attribution = AttrLocal
	res.NextStep = NextStep(res.Attribution)
	return res
}

// RunYouTube 执行 P2：三级真实请求，只回答「网页能否打开 / 播放器怎么说 / 媒体 CDN 能否连上」。
//
// 不产出任何速度数字（Mbps 恒为 0）。理由见文件头：匿名环境拿不到播放分片直链，
// 而拿 redirector.gvt1.com 的下行速度冒充「YouTube 播放速度」属错误标注，
// 且与 P1 同属 Google 边缘、是纯重复采集。降级必须可见——证据链里写清楚没测吞吐。
func RunYouTube(ctx context.Context, tr *http.Transport, maxBody int64) *Result {
	m := MetaByID("P2")
	res := &Result{ID: m.ID, Name: m.Name, Group: m.Group, Status: StatusRunning, Samples: []Sample{}, MediaPath: ytPathUnavailable}
	if maxBody < 1<<20 {
		maxBody = 2 << 20
	}
	started := time.Now()

	// ── T1 播放页 ─────────────────────────────────────────────────────────
	// 同时承担两件事：证明 youtube.com 可达，以及拿到播放器自述状态。
	// 预算给到剩余时间的 55%：页面实测约 1.4 MB，慢链路上比其它两级都吃时间。
	t1Ctx, t1Cancel := ytSlice(ctx, ytRemain(ctx)*55/100)
	f1, t1Tries := ytFetch(t1Ctx, tr, FetchOpts{URL: ytWatchURL, MaxBody: maxBody}, 2)
	t1Cancel()
	if ctx.Err() != nil {
		return ytCancelled(res, started)
	}

	pageStatus := StatusFailed
	pageHint := ""
	anonVerify := false // 匿名请求是否被要求「先证明不是机器人」
	var pbStatus, pbReason string
	if f1.Err != nil {
		pageStatus = ytStatusOfErr(f1.Err)
		pageHint = ytErrHint(f1.Err)
		res.Error = f1.Err.Error()
		res.Evidence = append(res.Evidence, fmt.Sprintf("播放页连不上（试了 %d 次）：%s", t1Tries, f1.Err.Error()))
	} else {
		pbStatus, pbReason = ytPlayability(f1.Body)
		anonVerify = pbStatus == "LOGIN_REQUIRED" || ytHit(f1.Body, ytBotMarkers) != ""
		pageStatus, pageHint = ytClassifyPage(f1, pbStatus, pbReason)
		ev := fmt.Sprintf("播放页 HTTP %d · %d KB · TTFB %.0f ms", f1.Status, f1.BodyLen/1024, f1.Phases.TTFB)
		if t1Tries > 1 {
			ev += fmt.Sprintf("（第 %d 次才成功）", t1Tries)
		}
		res.Evidence = append(res.Evidence, ev)
		if title := ytTitle(f1.Body); title != "" {
			res.Evidence = append(res.Evidence, "页面标题："+title)
		}
		if pbStatus != "" {
			s := "播放器状态：" + pbStatus
			if pbReason != "" {
				s += " · " + pbReason
			}
			res.Evidence = append(res.Evidence, s)
		} else {
			res.Evidence = append(res.Evidence, fmt.Sprintf("页面里没有 playabilityStatus 声明（采样窗口 %d MB），不能据此判断播放能力", maxBody>>20))
		}
		if anonVerify {
			res.Evidence = append(res.Evidence, "匿名接口要求「先确认不是机器人」：这是对无 Cookie 探测请求的限制，不是对浏览器的判词；本项结论以页面本体与视频 CDN 是否连通为准")
		}

		res.Phases = f1.Phases
		res.TTFB = Round(f1.Phases.TTFB, 1)
		res.HTTPStatus = f1.Status
		res.FinalURL = f1.FinalURL
		res.TLSVersion = f1.TLSVersion
		res.RemoteAddr = f1.RemoteAddr
		res.Bytes = f1.BodyLen
	}

	// ── T2 缩略图 ─────────────────────────────────────────────────────────
	// 只报字节与 TTFB，禁止折算 Mbps：20 KB 的样本折算误差可达数倍，属伪精度。
	// 价值在于它是独立于 youtube.com 的图片 CDN（sffe），能区分「主页能开、图裂了」。
	thumbOK := false
	runMedia := pageStatus != StatusFailed && pageStatus != StatusHijacked && pageStatus != StatusCancelled
	if runMedia {
		t2Ctx, t2Cancel := ytSlice(ctx, ytMin(ytRemain(ctx)*20/100, 5*time.Second))
		f2, t2Tries := ytFetch(t2Ctx, tr, FetchOpts{URL: ytThumbURL, MaxBody: 128 << 10}, 2)
		t2Cancel()
		if ctx.Err() != nil {
			return ytCancelled(res, started)
		}
		ctype := ""
		if f2.Err == nil {
			ctype = strings.ToLower(f2.Header.Get("Content-Type"))
		}
		thumbOK = f2.Err == nil && f2.Status == 200 && f2.BodyLen >= 8000 && strings.Contains(ctype, "image")
		switch {
		case thumbOK:
			res.Evidence = append(res.Evidence, fmt.Sprintf("缩略图 HTTP 200 · %d B · TTFB %.0f ms（图片 CDN 可用）", f2.BodyLen, f2.Phases.TTFB))
		case f2.Err != nil:
			res.Evidence = append(res.Evidence, fmt.Sprintf("缩略图连不上（试了 %d 次）：%s", t2Tries, f2.Err.Error()))
		default:
			res.Evidence = append(res.Evidence, fmt.Sprintf("缩略图返回 %d（%d B），图片 CDN 不可用", f2.Status, f2.BodyLen))
		}
	}

	// ── T3 googlevideo 边缘握手 ───────────────────────────────────────────
	// 从页面里抠真实的分片域名，向它发起一次握手。拿到任何 HTTP 响应即证明
	// DNS+TCP+TLS 打通到真实视频边缘（实测该域名的 generate_204 是零字节、可复核）。
	// 抠不到主机名记 skipped，不重试、不据此判失败（页面改版属已知情形）。
	gvsHost := ""
	if runMedia {
		if mt := ytGvsHostRe.Find(f1.Body); len(mt) > 0 {
			gvsHost = string(mt)
		}
	}
	gvsOK := false
	switch {
	case !runMedia:
		// 网页那一级就没通，媒体级必然同样不通，测了也只是重复同一个结论
	case gvsHost == "":
		res.Evidence = append(res.Evidence, "页面里没出现 *.googlevideo.com 主机名，视频边缘握手未测（不计为失败）")
	case ytRemain(ctx) < 1500*time.Millisecond:
		res.Evidence = append(res.Evidence, "预算不足，视频边缘握手未测（"+gvsHost+"）")
	default:
		// 每一级尝试给独立预算：共用一块总预算时，第一次把时间耗光，
		// 第二次会在已过期的 ctx 上瞬间返回，写成"试了 2 次"就成了虚报。
		// 实测该主机在 TUN 下会挂满超时（fake-ip 黑洞），必须让每次尝试都真有时间。
		budget := ytMin(ytRemain(ctx), 2*ytEdgeSlice)
		var f3 *FetchResult
		tries := 0
		for i := 0; i < 2; i++ {
			if ytRemain(ctx) < 1200*time.Millisecond {
				break
			}
			aCtx, aCancel := ytSlice(ctx, ytMin(budget/2, ytEdgeSlice))
			f3 = Fetch(aCtx, tr, FetchOpts{URL: "https://" + gvsHost + "/generate_204", MaxBody: 1024})
			tries++
			aCancel()
			if f3.Err == nil {
				break
			}
			if i == 0 && ctx.Err() == nil && ytRemain(ctx) > 1500*time.Millisecond {
				select {
				case <-ctx.Done():
				case <-time.After(ytRetryGap):
				}
			}
		}
		if ctx.Err() != nil {
			return ytCancelled(res, started)
		}
		if f3 == nil {
			res.Evidence = append(res.Evidence, "预算不足，视频边缘握手未测（"+gvsHost+"）")
		} else if f3.Err != nil {
			// 措辞必须中性：重试过的连接层失败仍然只说明「这一次没连上」。
			// 实测该域名在 TUN 下会解析到 198.18.x.x 的 fake-ip、握手在本地就断，
			// 既可能是分流规则没覆盖视频 CDN，也可能是 CDN 侧问题，不可强行归因。
			res.Evidence = append(res.Evidence, fmt.Sprintf("视频边缘 %s 连不上（试了 %d 次，每次 %s 内未完成握手）：%s",
				gvsHost, tries, ytEdgeSlice, f3.Err.Error()))
		} else {
			gvsOK = true
			res.Evidence = append(res.Evidence, fmt.Sprintf("视频边缘 %s 已建立连接 · HTTP %d · TLS %s · TTFB %.0f ms", gvsHost, f3.Status, f3.TLSVersion, f3.Phases.TTFB))
		}
	}

	// ── 归并与结论 ────────────────────────────────────────────────────────
	// 关键分支（PRD FR-P2 #4）：网页通、媒体域不通，绝不能整体判「YouTube 不可用」，
	// 必须单独指出媒体那一级失败——这正是「能刷首页、看不了视频」的真实形态。
	mediaBroken := runMedia && (!thumbOK || (gvsHost != "" && !gvsOK))
	mediaPath := ytPathUnavailable
	switch {
	case pageStatus == StatusFailed || pageStatus == StatusHijacked:
		mediaPath = ytPathUnavailable
	case gvsOK:
		mediaPath = ytPathGVS
	case thumbOK:
		mediaPath = ytPathThumb
	default:
		mediaPath = ytPathPageOnly
	}

	final := pageStatus
	var headline, subline string
	switch pageStatus {
	case StatusOK:
		if mediaBroken {
			final = StatusDegraded
			headline = "页面可开"
			subline = "媒体 CDN 不通 · 视频大概率放不了"
		} else {
			headline = "可看"
			subline = fmt.Sprintf("页面 TTFB %.0f ms", res.TTFB)
			if thumbOK {
				subline += " · 缩略图可用"
			}
			if gvsOK {
				subline += " · 视频 CDN 已连上"
			} else {
				subline += " · 视频边缘未验证"
			}
			if anonVerify {
				subline += " · 匿名接口要求验证"
			}
		}
	case StatusInconclusive:
		headline = "无法确认"
		subline = "youtube.com · 页面可开但未取到播放器状态"
	case StatusChallenge:
		headline = "被要求验证"
		// 必须说清"这是匿名探测遇到的门槛"，而不是"YouTube 不可用"：
		// 用户浏览器里已登录，往往能正常播放，把验证页写成不可用就是假阴性。
		subline = "youtube.com · 匿名探测被要求登录/验证，不代表你浏览器里看不了"
	case StatusRegionBlock:
		headline = "地区受限"
		subline = "youtube.com · 该视频在当前出口地区不可用"
	case StatusDegraded:
		headline = "受限"
		subline = "youtube.com · " + pageHint
	case StatusRateLimited:
		headline = "限流"
		subline = "youtube.com · 网络通，目标站点限流"
	case StatusHijacked:
		headline = "疑似劫持"
		subline = "youtube.com · 证书异常"
	case StatusFailed:
		headline = "不可达"
		subline = "youtube.com · 连不上"
	default:
		headline = shortHeadline(pageStatus)
		subline = "youtube.com"
	}

	res.Status = final
	res.MediaPath = mediaPath
	res.Headline = headline
	res.Subline = subline
	// Mbps 恒为 0：本项没有测吞吐。汇总层只在 Mbps>0 时才采信 P2 的速度，
	// 填一个假数字进去会直接污染速度维度并与 P1 打架。
	res.Mbps = 0
	res.Score = statusScore(final)
	res.DurationMs = Round(float64(time.Since(started).Microseconds())/1000, 0)

	if mediaBroken && pageStatus == StatusOK {
		// 网页通、媒体不通：既可能是分流规则漏了视频域名，也可能是 CDN 侧限制，
		// 单轮无法区分，归因必须留待定，并给出唯一值得做的排除动作。
		res.Attribution = AttrUnknown
		res.NextStep = "网页能开但视频媒体域不通：先在浏览器里实际播放一次做对照；若也放不了，检查分流规则是否覆盖 googlevideo"
	} else if anonVerify && final == StatusOK {
		// 注意：这里不能判 challenge。播放页与视频 CDN 都已连通，
		// "要求验证"只是匿名请求的常态，判成风控就是假阴性（已实机对照证伪）。
		res.Attribution = AttributionOf(final, f1.Err)
		res.NextStep = "本项判可看：播放页与视频 CDN 都通了。若想亲自确认，用浏览器打开同一个视频播几秒——匿名接口要求验证是这类请求的常态，不代表你看不了"
	} else {
		res.Attribution = AttributionOf(final, f1.Err)
		res.NextStep = NextStep(res.Attribution)
	}

	// 降级必须可见（PRD FR-P2 #6）：说清这一项到底测了什么、没测什么
	res.Evidence = append(res.Evidence, "本项不产出速度数字：YouTube 播放分片需 JS 签名，纯 Go 取不到直链；只回答网页能否打开 / 播放器怎么说 / 媒体 CDN 能否连上")
	return res
}
