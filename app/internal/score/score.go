// Package score 把各探测项结果折算为可解释的分项分与总分。
package score

import (
	"fmt"
	"strings"

	"proxylens/internal/probe"
)

// Dimension 是一个评分维度。
type Dimension struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Score  float64 `json:"score"`
	Weight float64 `json:"weight"`
	Detail string  `json:"detail"`
	// Measured=false 表示本轮没有测这一维（不是测出来差），前端应显示"未测"
	Measured bool `json:"measured"`
}

// Summary 是一次运行的评分汇总。
type Summary struct {
	Total      float64     `json:"total"`
	Grade      string      `json:"grade"`
	State      string      `json:"state"` // usable | limited | blocked
	StateLabel string      `json:"stateLabel"`
	Verdict    string      `json:"verdict"`
	Dimensions []Dimension `json:"dimensions"`
	Advice     []string    `json:"advice"`
	Caps       []string    `json:"caps"`
}

// Aggregate 计算总分。权重：速度 30%、稳定性 30%、AI 可用性 40%。
func Aggregate(results []*probe.Result) Summary {
	byID := map[string]*probe.Result{}
	for _, r := range results {
		byID[r.ID] = r
	}

	// —— 速度维度 ——
	speedVal := 0.0
	speedDetail := "本次未测下载速度"
	speedMeasured := false
	if r := byID["P1"]; r != nil && isMeasured(r) {
		speedMeasured = true
		if r.Status == probe.StatusOK || r.Status == probe.StatusDegraded {
			// 直接用分项分，保证「P1 卡片上的分数」和「速度维度分数」永远是同一个数。
			// 之前这里用 P50Mbps 而卡片用 Mbps，一旦两者分叉，报告就会自相矛盾
			// （卡片写 3.8 Mbps / 51 分，维度写 0 分 / 约 0 Mbps）。
			speedVal = r.Score
			speedDetail = fmt.Sprintf("约 %.1f Mbps · %s", r.Mbps, speedHint(r.Mbps))
			if r.Stalled {
				speedDetail += " · 中途停滞"
			}
		} else {
			speedDetail = "下载失败：" + firstNonEmpty(r.Headline, r.Error)
		}
	}
	if r := byID["P2"]; r != nil && r.Status == probe.StatusOK && r.Mbps > 0 {
		// YouTube 分片实测更贴近真实业务，取两者较优值
		speedMeasured = true
		yt := probe.SpeedScore(r.Mbps)
		if yt > speedVal {
			speedVal = yt
			speedDetail = fmt.Sprintf("YouTube 分片 %.1f Mbps", r.Mbps)
		}
	}

	// —— 稳定性维度 ——
	stabVal := 0.0
	stabDetail := "本次未测稳定性"
	stabMeasured := false
	if r := byID["P6"]; r != nil && isMeasured(r) {
		stabMeasured = true
		stabVal = probe.StabilityScore(r.SuccessPct, r.Jitter)
		stabDetail = fmt.Sprintf("成功率 %.0f%% · RTT %.0f ms · 抖动 %.0f ms", r.SuccessPct, r.TTFB, r.Jitter)
	}

	// —— AI 可用性维度 ——
	aiWeights := map[string]float64{"P3": 40, "P4": 35, "P5": 25}
	aiVal, aiTotalW, aiSum := 0.0, 0.0, 0.0
	var aiNotes []string
	for id, w := range aiWeights {
		r := byID[id]
		if !isMeasured(r) {
			continue
		}
		aiTotalW += w
		aiSum += probe.StatusScoreOf(r.Status) * w
		aiNotes = append(aiNotes, fmt.Sprintf("%s %s", shortName(id), statusWord(r.Status)))
	}
	if aiTotalW > 0 {
		aiVal = aiSum / aiTotalW
	}
	aiDetail := strings.Join(aiNotes, " · ")
	aiMeasured := aiTotalW > 0

	caps := []string{}
	// 封顶规则：ChatGPT 网页被拦截时，不能给"AI 可用"的高分（避免绿色误导）
	if p3 := byID["P3"]; p3 != nil {
		switch p3.Status {
		case probe.StatusChallenge, probe.StatusRegionBlock, probe.StatusHijacked, probe.StatusFailed:
			if aiVal > 60 {
				aiVal = 60
				caps = append(caps, "ChatGPT 网页不可用，AI 分封顶 60")
			}
		}
	}
	// 封顶规则：两个 AI 网页都被地区限制时，AI 分不得高于 25
	if p3, p4 := byID["P3"], byID["P4"]; p3 != nil && p4 != nil &&
		p3.Status == probe.StatusRegionBlock && p4.Status == probe.StatusRegionBlock {
		if aiVal > 25 {
			aiVal = 25
			caps = append(caps, "ChatGPT 与 Gemini 均被地区限制，AI 分封顶 25")
		}
	}
	// 证书异常是硬伤：总分封顶 55
	for _, r := range results {
		if r.Status == probe.StatusHijacked {
			caps = append(caps, "检测到证书/TLS 异常，总分封顶 55")
			break
		}
	}

	dims := []Dimension{
		{"speed", "速度", probe.Round(speedVal, 0), 0.40, speedDetail, speedMeasured},
		{"stability", "稳定性", probe.Round(stabVal, 0), 0.30, stabDetail, stabMeasured},
		{"ai", "AI 可用性", probe.Round(aiVal, 0), 0.30, aiDetail, aiMeasured},
	}
	measured := map[string]bool{"speed": speedMeasured, "stability": stabMeasured, "ai": aiMeasured}

	// 未测的维度不参与加权：快速档没测下载时，"速度 0 分"会被读成"速度极差"，
	// 实际是没测。按实际参与项重新归一，分数才对得上用户看到的事实。
	var wSum, sSum float64
	for _, d := range dims {
		if measured[d.Key] {
			wSum += d.Weight
			sSum += d.Score * d.Weight
		}
	}
	total := 0.0
	if wSum > 0 {
		total = sSum / wSum
		// 展示权重同步归一，未测维度归零，避免"占 40% 却是 0 分"的错觉
		for i := range dims {
			if measured[dims[i].Key] {
				dims[i].Weight = probe.Round(dims[i].Weight/wSum, 2)
			} else {
				dims[i].Weight = 0
			}
		}
	}
	for _, c := range caps {
		if strings.Contains(c, "封顶 55") && total > 55 {
			total = 55
		}
	}

	// gate-not-weight：先定可用性闸门，再让分数服从闸门上限。
	// 禁止出现"分数 85+ 但 ChatGPT 打不开"的绿色误导。
	gate, stateLabel := gateOf(byID)
	switch gate {
	case "blocked":
		if total > 29 {
			total = 29
			caps = append(caps, "可用性闸门=不可用，总分封顶 29")
		}
	case "limited":
		if total > 69 {
			total = 69
			caps = append(caps, "可用性闸门=受限，总分封顶 69")
		}
	}
	total = probe.Round(total, 0)

	s := Summary{Total: total, Dimensions: dims, Caps: caps}
	s.Grade = grade(total)
	s.State, s.StateLabel = gate, stateLabel
	s.Verdict = verdict(byID, total, s.State)
	s.Advice = advice(byID, total, s.State)
	return s
}

// MeasurementInvalid 判断「这一轮到底测成没测成」。
//
// 背景：链路会在分钟级别剧烈抖动（实测同一 IP 五分钟内 chatgpt.com 从 6/6 200
// 变成 5/6 403+challenge，另有一整轮全部端点连接层失败）。那种情况下如果照常算分，
// 用户会看到 0 分 / E·不可用，但下一次重测又是 C·可用——同一套代理给出两个相反的结论。
//
// 判据：只要有任何一项拿到了可判读的 HTTP 响应（哪怕是被风控、地区限制、限流），
// 就说明链路是通的，测量成立。全部卡在连接层才说明"这一轮什么都没测到"。
func MeasurementInvalid(results []*probe.Result) bool {
	sawAny := false
	for _, r := range results {
		if r == nil || !isMeasured(r) {
			continue
		}
		sawAny = true
		switch r.Status {
		case probe.StatusOK, probe.StatusAuthRequired, probe.StatusChallenge,
			probe.StatusRegionBlock, probe.StatusRateLimited, probe.StatusDegraded,
			probe.StatusHijacked, probe.StatusInconclusive:
			// 这些都意味着"服务器回话了"，测量有效
			return false
		}
	}
	// 一项都没测（全 skipped）不算"测量无效"，那是配置问题，由调用方区分
	return sawAny
}

func shortName(id string) string {
	switch id {
	case "P3":
		return "ChatGPT"
	case "P4":
		return "Gemini"
	case "P5":
		return "OpenAI API"
	default:
		return id
	}
}

func statusWord(s probe.Status) string {
	switch s {
	case probe.StatusOK:
		return "可用"
	case probe.StatusAuthRequired:
		return "可达（需 Key）"
	case probe.StatusDegraded:
		return "受限"
	case probe.StatusInconclusive:
		return "无法确认"
	case probe.StatusRateLimited:
		return "限流（网络通）"
	case probe.StatusChallenge:
		return "被风控"
	case probe.StatusRegionBlock:
		return "地区受限"
	case probe.StatusHijacked:
		return "疑似劫持"
	case probe.StatusFailed:
		return "不可达"
	case probe.StatusCancelled:
		return "已取消"
	case probe.StatusSkipped:
		return "未执行"
	default:
		return string(s)
	}
}

// grade 返回「档位字母 + 档位词」。PRD 要求 A–E 字母必须显式可见，
// 只给数字或只给形容词都算不达标。分档门槛对齐 PRD：A85/B70/C55/D30/E0。
func grade(v float64) string {
	switch {
	case v >= 85:
		return "A · 优秀"
	case v >= 70:
		return "B · 良好"
	case v >= 55:
		return "C · 可用"
	case v >= 30:
		return "D · 较差"
	default:
		return "E · 不可用"
	}
}

// speedHint 把吞吐翻成"能干什么"。用户关心的是能不能流畅看视频，
// 不是 P50/峰值这类只有工程师看的术语。
func speedHint(mbps float64) string {
	switch {
	case mbps >= 25:
		return "够 4K 串流"
	case mbps >= 8:
		return "够 1080p"
	case mbps >= 3:
		return "够 720p"
	default:
		return "仅够文字与轻量页面"
	}
}

// isMeasured 区分"测了但结果差"与"压根没测"。
// 缺少这个区分，未测项会以 0 分参与加权，把"没测速度"渲染成"速度极差"。
func isMeasured(r *probe.Result) bool {
	if r == nil {
		return false
	}
	switch r.Status {
	case probe.StatusSkipped, probe.StatusCancelled, probe.StatusPending, probe.StatusRunning:
		return false
	}
	return true
}

func gateOf(byID map[string]*probe.Result) (string, string) {
	p3, p4, p5 := byID["P3"], byID["P4"], byID["P5"]

	// 硬阻断：任一 AI 端点处于"链路通但业务不通"的状态，整体都不能算可用。
	// 尤其是 degraded / rate_limited —— 它们证明连接建立了，但服务端在降级或限流，
	// 用户此刻打开网页就是不能用。漏掉它们会直接产出"浏览器打不开却报优秀"的假绿。
	hardBlock := false
	for _, r := range []*probe.Result{p3, p4, p5} {
		if r == nil {
			continue
		}
		switch r.Status {
		case probe.StatusChallenge, probe.StatusRegionBlock, probe.StatusHijacked,
			probe.StatusFailed, probe.StatusInconclusive, probe.StatusDegraded,
			probe.StatusRateLimited:
			hardBlock = true
		}
	}

	// 网页端（P3/P4）才是用户真实体感的来源，API 网关可达只作补充证据
	webUsable := 0
	for _, r := range []*probe.Result{p3, p4} {
		if r != nil && (r.Status == probe.StatusOK || r.Status == probe.StatusAuthRequired) {
			webUsable++
		}
	}
	apiReachable := p5 != nil && (p5.Status == probe.StatusOK || p5.Status == probe.StatusAuthRequired)

	// 两个网页端都打不开、API 也不可达，才算彻底不可用
	if webUsable == 0 && !apiReachable {
		return "blocked", "AI 不可用"
	}
	// 至少一个网页端可用，且没有任何端点处于阻断态
	if webUsable >= 1 && !hardBlock {
		return "usable", "AI 可用"
	}
	return "limited", "AI 受限"
}

// siteReason 把单个 AI 端点的非可用状态翻译成一句人话。
// 返回空串表示"这个端点没给出可解释的异常"。
func siteReason(r *probe.Result) string {
	name := "ChatGPT"
	if r.ID == "P4" {
		name = "Gemini"
	} else if r.ID == "P5" {
		name = "OpenAI API"
	}
	switch r.Status {
	case probe.StatusChallenge:
		return name + " 被人机验证拦截，链路本身是通的"
	case probe.StatusRegionBlock:
		return name + " 判定当前出口地区不受支持"
	case probe.StatusHijacked:
		return "访问 " + name + " 时响应异常，疑似被中间人劫持"
	case probe.StatusFailed:
		return name + " 完全打不开"
	case probe.StatusDegraded:
		return name + " 返回异常响应（疑似风控降级），页面此刻打不开"
	case probe.StatusRateLimited:
		return name + " 正在限流，稍后可重试"
	case probe.StatusInconclusive:
		return "连上了 " + name + "，但无法确认是本体页面"
	}
	return ""
}

func verdict(byID map[string]*probe.Result, total float64, state string) string {
	p3, p4, p5 := byID["P3"], byID["P4"], byID["P5"]
	p1 := byID["P1"]
	p6 := byID["P6"]

	var b strings.Builder
	switch state {
	case "usable":
		b.WriteString("这套代理可以正常使用 AI 服务")
	case "blocked":
		b.WriteString("这套代理目前无法使用 AI 服务")
	default:
		b.WriteString("这套代理能用 AI，但存在明显短板")
	}

	// 补齐"为什么"：结论必须能被用户在自己的浏览器里复现，
	// 只说"有短板"却不说短板在哪，等于让用户自己去猜。
	for _, r := range []*probe.Result{p3, p4} {
		if r == nil {
			continue
		}
		if s := siteReason(r); s != "" {
			b.WriteString("；" + s)
			break
		}
	}

	if p1 != nil && p1.Status == probe.StatusOK {
		if p1.Mbps < 5 {
			b.WriteString(fmt.Sprintf("；带宽仅 %.1f Mbps，视频与长对话会比较吃力", p1.Mbps))
		} else if p1.CV > 0.5 {
			// 语气必须服从裁定：usable 时不能说"忽快忽慢"，否则与"总分优秀"直接打架
			if state == "usable" {
				b.WriteString(fmt.Sprintf("；速度有起伏（平均 %.1f / 峰值 %.1f Mbps），日常够用但大文件耗时不稳", p1.Mbps, p1.PeakMbps))
			} else {
				b.WriteString(fmt.Sprintf("；平均 %.1f Mbps 但波动较大，体感会忽快忽慢", p1.Mbps))
			}
		}
	}
	if p6 != nil && p6.SuccessPct < 100 && p6.SuccessPct > 0 {
		b.WriteString(fmt.Sprintf("；基线采样成功率仅 %.0f%%，链路不稳", p6.SuccessPct))
	}
	if p5 != nil && p5.Status == probe.StatusAuthRequired && state == "blocked" {
		b.WriteString("；注意 OpenAI API 是可达的，问题集中在网页端")
	}
	// 没测的维度要主动说明，否则用户会把"没测"读成"很差"
	if !isMeasured(p1) && !(byID["P2"] != nil && byID["P2"].Status == probe.StatusOK && byID["P2"].Mbps > 0) {
		b.WriteString("；本次未测下载速度，速度分不参与总分")
	}
	// 分数只由评分环承担，句尾不再重复一遍（同一信息出现三次会稀释可信度）
	b.WriteString("。")
	return b.String()
}

func advice(byID map[string]*probe.Result, total float64, state string) []string {
	out := []string{}
	p1 := byID["P1"]
	p3, p4 := byID["P3"], byID["P4"]

	if p3 != nil && p3.Status == probe.StatusChallenge {
		out = append(out, "ChatGPT 遇到人机验证：优先更换出口 IP 段（住宅/家宽优先），或换一个地区节点后重测")
	}
	if p4 != nil && p4.Status == probe.StatusRegionBlock {
		out = append(out, "Gemini 地区受限：把节点切到美国/日本/新加坡等支持地区")
	}
	if p3 != nil && p3.Status == probe.StatusHijacked {
		out = append(out, "出现证书异常：检查是否被中间人代理，确认系统时间正确，必要时更换节点")
	}
	if p1 != nil && p1.Status == probe.StatusOK && p1.Mbps < 10 {
		out = append(out, "带宽偏低：优先换带宽更大的节点，或检查本地是否被限速")
	}
	if p1 != nil && p1.CV > 0.5 {
		if state == "usable" {
			out = append(out, fmt.Sprintf("速度有起伏（CV %.2f）：大文件下载耗时不稳定，长视频高峰时段建议先测一次", p1.CV))
		} else {
			out = append(out, "速度波动大：可能是共享节点拥塞，避开高峰或换专线")
		}
	}
	if state == "usable" && len(out) == 0 {
		out = append(out, "当前状态良好，可直接用于 ChatGPT / Gemini 日常使用")
	}
	if total < 40 {
		out = append(out, "建议先用「直连对照」跑一次，确认是代理问题还是本地网络问题")
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
