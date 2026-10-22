package probe

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

var siteURLs = map[string]string{
	"P3": "https://chatgpt.com/",
	"P4": "https://gemini.google.com/",
	"P5": "https://api.openai.com/v1/models",
}

// severity 用于"多次采样取最差"：值越大越严重。
// 目标站点（尤其 Cloudflare 后面的）本身会时好时坏，单次采样等于抽签，
// 抽到一次 200 就宣布"优秀"，与用户随后打不开的现实直接冲突。
func severity(s Status) int {
	switch s {
	case StatusOK:
		return 0
	case StatusAuthRequired:
		return 1
	case StatusRateLimited:
		return 2
	case StatusDegraded:
		return 3
	case StatusInconclusive:
		return 4
	case StatusChallenge:
		return 5
	case StatusRegionBlock:
		return 6
	case StatusFailed:
		return 7
	case StatusHijacked:
		return 8
	default:
		return 9
	}
}

// RunSite 执行 P3/P4/P5。
// P3/P4 是网页端，用户体感来自"现在能不能打开"，因此做多次采样并取最差结论，
// 同时在证据里如实给出 n/m 次的分布，避免用一次好运掩盖不稳定。
func RunSite(ctx context.Context, tr *http.Transport, id string) *Result {
	m := MetaByID(id)
	res := &Result{ID: m.ID, Name: m.Name, Group: m.Group, Status: StatusRunning, Samples: []Sample{}}
	target := siteURLs[id]
	res.Subline = shortenURL(target)

	rounds := 1
	if id == "P3" || id == "P4" {
		rounds = 3
	}

	// ── P3 出口地区层（口径参考 OpenClash 的 cdn-cgi/trace）────────────────
	// 这一层只解释"为什么打不开"和"该换什么样的节点"，不负责下结论。
	// 红线：地区层不能推翻可用层——"出口地区受支持"绝不等于"页面能打开"。
	// 2026-10-22 本机就是活例：loc=SG、warp=on、OpenClash 口径判"解锁"，
	// 而真浏览器打开 chatgpt.com 是 403。所以 loc / warp 只进证据链。
	// P4 不做这一层：gemini.google.com 背后不是 Cloudflare，
	// /cdn-cgi/trace 回的是 Google 的 404 页（实测），没有 loc 可读，不硬造。
	var (
		regionEv      []string
		regionBlocked bool
	)
	if id == "P3" {
		// 地区层是"补充"而不是主线，给它单独的时限：即便它整体超时，
		// 也不能把预算吃光，页面 GET 才是结论来源。
		rctx, rcancel := context.WithTimeout(ctx, 8*time.Second)
		if loc, colo, warp, ok := FetchTrace(rctx, tr, "chatgpt.com"); ok {
			res.Region, res.Colo, res.Warp = loc, colo, warp
			regionEv = append(regionEv, traceEvidence(loc, colo, warp))
		}
		// 只有 OpenAI 自己说"这个地区不支持"才算硬证据。
		// 放在页面 GET 之前，是为了在拿到硬证据时省掉后两轮没有意义的重试。
		if blocked, ok := fetchOpenAIRegionVerdict(rctx, tr); ok && blocked {
			regionBlocked = true
			regionEv = append(regionEv, "OpenAI 自述：当前出口地区不受支持（compliance/cookie_requirements 返回 unsupported_country）")
			rounds = 1
		}
		rcancel()
	}
	// 200KB 足以覆盖标题与首屏指纹（Gemini 首页约 800KB，标题在 96KB 之后）
	const maxBody = 200 << 10

	started := time.Now()
	tally := map[Status]int{}
	worst := StatusOK
	var worstHint string
	var worstEv []string
	okCount := 0
	done := 0
	var first *FetchResult

	for i := 0; i < rounds; i++ {
		if ctx.Err() != nil {
			break
		}
		f := Fetch(ctx, tr, FetchOpts{URL: target, MaxBody: maxBody})
		st, ev, hint := Classify(id, f)
		// 连接层瞬态错误先重试：TUN / 隧道链路上 EOF、reset、握手超时是高频瞬发事件，
		// 单次失手就判"这个站打不开"，产出的是假阴性——用户在自己浏览器里明明能打开。
		// 只有连续失败才算数，且措辞必须中性（见 errorHint）。
		for attempt := 0; attempt < 2 && isTransient(st, f.Err) && ctx.Err() == nil; attempt++ {
			select {
			case <-ctx.Done():
			case <-time.After(time.Duration(800*(attempt+1)) * time.Millisecond):
			}
			f2 := Fetch(ctx, tr, FetchOpts{URL: target, MaxBody: maxBody})
			st2, ev2, hint2 := Classify(id, f2)
			res.Evidence = append(res.Evidence, fmt.Sprintf("第 %d 次瞬态失败后重试，结果：%s", i+1, statusShort(st2)))
			f, st, ev, hint = f2, st2, ev2, hint2
			if !isTransient(st, f.Err) {
				break
			}
		}
		if first == nil {
			first = f
		}
		tally[st]++
		done++
		if st == StatusOK || st == StatusAuthRequired {
			okCount++
		}
		if severity(st) > severity(worst) {
			worst = st
			worstHint = hint
			worstEv = ev
			res.Error = ""
			if f.Err != nil {
				res.Error = f.Err.Error()
			}
		}
		if rounds > 1 {
			res.Evidence = append(res.Evidence, fmt.Sprintf("第 %d 次：HTTP %d · %s", i+1, f.Status, statusShort(st)))
		}
		if i < rounds-1 {
			// 间隔不能太短：1.2s 内连打 3 次采到的是同一个瞬时窗口，等于单次采样。
			select {
			case <-ctx.Done():
			case <-time.After(1500 * time.Millisecond):
			}
		}
	}

	res.DurationMs = Round(float64(time.Since(started).Microseconds())/1000, 0)

	// 一次都没跑完（中途取消）不该冒充结论
	if done == 0 {
		res.Status = StatusCancelled
		res.Headline = "已取消"
		res.Subline = "本次未产生结论"
		res.Score = statusScore(StatusCancelled)
		res.Attribution = AttrLocal
		res.NextStep = NextStep(res.Attribution)
		return res
	}

	res.Phases = first.Phases
	res.TTFB = first.Phases.TTFB
	res.HTTPStatus = first.Status
	res.FinalURL = first.FinalURL
	res.TLSVersion = first.TLSVersion
	res.RemoteAddr = first.RemoteAddr
	res.Bytes = first.BodyLen

	res.Status = worst
	// 证据顺序：结论 → 每次采样的分布 → 最差那次的关键证据（首行与摘要重复，跳过）
	finalEv := []string{}
	if worstHint != "" {
		finalEv = append(finalEv, worstHint)
	}
	finalEv = append(finalEv, res.Evidence...)
	if len(worstEv) > 1 {
		finalEv = append(finalEv, worstEv[1:]...)
	}
	res.Evidence = finalEv

	// 结论按最差采样给出，但分布必须说出来——这是"符合实际"的关键
	if done > 1 && okCount > 0 && okCount < done {
		res.Headline = "时好时坏"
		res.Subline = fmt.Sprintf("%s · %d/%d 次能打开", shortenURL(target), okCount, done)
	} else {
		res.Headline = shortHeadline(worst)
		res.Subline = fmt.Sprintf("%s · HTTP %d", shortenURL(target), first.Status)
		if done > 1 {
			res.Subline = fmt.Sprintf("%s · %d/%d 次能打开", shortenURL(target), okCount, done)
		}
	}

	res.Score = statusScore(worst)
	res.Attribution = AttributionOf(worst, first.Err)
	res.NextStep = NextStep(res.Attribution)

	// ── 地区层收尾：先落证据，再决定能不能改结论 ──────────────────────────
	// 证据排在最后一行：结论与 n/m 次分布比"为什么"更重要。
	res.Evidence = append(res.Evidence, regionEv...)
	if regionBlocked {
		switch res.Status {
		case StatusOK, StatusAuthRequired:
			// 页面真的打开了，就必须如实报"能打开"——地区自述不能反过来把它推翻。
			res.Evidence = append(res.Evidence, "但页面此刻确实能正常打开，以页面结果为准；地区自述与页面表现不一致，疑似灰度或缓存差异")
		default:
			// 反过来，页面明明打不开时，把原因落到"地区不受支持"才是对用户有用的解释：
			// 这时"换地区"与"换节点"是同一个动作。
			res.Status = StatusRegionBlock
			res.Score = statusScore(StatusRegionBlock)
			res.Headline = shortHeadline(StatusRegionBlock)
			res.Attribution = AttrTarget
			res.NextStep = NextStep(AttrTarget)
			res.Evidence = append(res.Evidence, "页面确实打不开，与 OpenAI 自述的地区限制一致")
		}
	}

	// warp=on 是独立于地区的病因，且它决定 NextStep 该说什么：出口是 Cloudflare
	// 的共享 IP，OpenAI 按 IP 段拒（403 / Unable to load site），跟"地区名"无关。
	// 不说清楚，用户会以为换个地区名就能好，然后在同一个节点上反复重试。
	if res.Warp {
		if res.Status != StatusOK && res.Status != StatusAuthRequired {
			res.NextStep = warpNextStep()
		} else {
			// 页面此刻能开也不代表安全：WARP 出口是共享的，随时可能被按 IP 段拒掉。
			res.Evidence = append(res.Evidence, "warp=on 属共享出口，OpenAI 随时可能按 IP 段拒绝；一旦出现 403，优先换非 WARP 节点，而不是重试")
		}
	}
	return res
}

// statusShort 是给证据链用的极简标签（不暴露状态码以外的术语）。
func statusShort(s Status) string {
	switch s {
	case StatusOK:
		return "可用"
	case StatusAuthRequired:
		return "需登录"
	case StatusDegraded:
		return "受限"
	case StatusInconclusive:
		return "无法确认"
	case StatusRateLimited:
		return "限流"
	case StatusChallenge:
		return "被风控"
	case StatusRegionBlock:
		return "地区受限"
	case StatusHijacked:
		return "疑似劫持"
	case StatusFailed:
		return "打不开"
	case StatusCancelled:
		return "已取消"
	default:
		return string(s)
	}
}

// shortHeadline 给探测卡一个短标签，长解释放证据链。
func shortHeadline(s Status) string {
	switch s {
	case StatusOK:
		return "可用"
	case StatusAuthRequired:
		return "可达"
	case StatusDegraded:
		return "受限"
	case StatusInconclusive:
		return "无法确认"
	case StatusRateLimited:
		return "限流"
	case StatusChallenge:
		return "被风控"
	case StatusRegionBlock:
		return "地区受限"
	case StatusHijacked:
		return "疑似劫持"
	case StatusFailed:
		return "不可达"
	case StatusCancelled:
		return "已取消"
	default:
		return "未执行"
	}
}

// RunBaseline 执行 P6：多次采样 generate_204，量化 RTT、成功率与抖动。
func RunBaseline(ctx context.Context, tr *http.Transport, rounds int) *Result {
	m := MetaByID("P6")
	res := &Result{ID: m.ID, Name: m.Name, Group: m.Group, Status: StatusRunning, Samples: []Sample{}}
	if rounds <= 0 {
		rounds = 6
	}
	targets := []string{"https://www.google.com/generate_204", "https://www.gstatic.com/generate_204"}
	started := time.Now()

	var rtts []float64
	okCount := 0
	for i := 0; i < rounds; i++ {
		target := targets[i%len(targets)]
		if ctx.Err() != nil {
			break
		}
		sub, cancel := context.WithTimeout(ctx, 8*time.Second)
		f := Fetch(sub, tr, FetchOpts{URL: target, MaxBody: 1024})
		cancel()
		if f.Err == nil && f.Status == 204 {
			okCount++
			rtt := f.Phases.Total
			rtts = append(rtts, rtt)
			res.Samples = append(res.Samples, Sample{T: float64(time.Since(started).Milliseconds()), Mbps: Round(rtt, 1), Bytes: int64(f.Status)})
		} else {
			res.Samples = append(res.Samples, Sample{T: float64(time.Since(started).Milliseconds()), Mbps: 0, Bytes: 0})
			if f.Err != nil {
				res.Evidence = append(res.Evidence, fmt.Sprintf("第 %d 次失败：%s", i+1, f.Err.Error()))
			} else {
				res.Evidence = append(res.Evidence, fmt.Sprintf("第 %d 次返回 HTTP %d", i+1, f.Status))
			}
		}
		select {
		case <-ctx.Done():
		case <-time.After(500 * time.Millisecond):
		}
	}

	res.DurationMs = Round(float64(time.Since(started).Microseconds())/1000, 0)
	res.SuccessPct = Round(float64(okCount)/float64(rounds)*100, 0)
	if len(rtts) > 0 {
		res.TTFB = Round(Percentile(rtts, 50), 1)
		res.P50Mbps = 0
		var jit float64
		if len(rtts) > 1 {
			for i := 1; i < len(rtts); i++ {
				d := rtts[i] - rtts[i-1]
				if d < 0 {
					d = -d
				}
				jit += d
			}
			jit /= float64(len(rtts) - 1)
		}
		res.Jitter = Round(jit, 1)
		res.Headline = fmt.Sprintf("RTT %.0f ms", res.TTFB)
		res.Subline = fmt.Sprintf("抖动 %.0f ms · 成功 %d/%d", jit, okCount, rounds)
		res.Evidence = append(res.Evidence, fmt.Sprintf("采样 %d 次，成功 %d 次，RTT 中位数 %.0f ms，抖动 %.0f ms", rounds, okCount, res.TTFB, jit))
	} else {
		res.Headline = "全部失败"
		res.Subline = fmt.Sprintf("0/%d 次成功", rounds)
	}

	switch {
	case okCount == rounds:
		res.Status = StatusOK
	case okCount == 0:
		res.Status = StatusFailed
		res.Headline = "不可达"
	default:
		res.Status = StatusDegraded
	}
	if res.Jitter > 120 && res.Status == StatusOK {
		res.Status = StatusDegraded
		res.Evidence = append(res.Evidence, "抖动偏大，链路不稳定")
	}
	res.Score = StabilityScore(res.SuccessPct, res.Jitter)
	return res
}
