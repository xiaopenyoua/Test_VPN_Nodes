// Package config 定义一次探测运行的参数与预设。
package config

import (
	"errors"
	"net/url"
	"strings"
)

// 出口模式
const (
	ModeSystem = "system" // 走操作系统默认出网路径（TUN 下等价于按系统路由直出）
	ModeCustom = "custom" // 手动指定代理
	ModeDirect = "direct" // 强制直连，用于与代理对照
)

// 预设档位
const (
	PresetQuick    = "quick"
	PresetStandard = "standard"
	PresetDeep     = "deep"
)

// 探测项 ID
const (
	ProbeGoogle  = "P1" // Google 大文件下载
	ProbeYouTube = "P2" // YouTube 链路
	ProbeChatGPT = "P3" // chatgpt.com 网页
	ProbeGemini  = "P4" // gemini.google.com 网页
	ProbeOpenAI  = "P5" // api.openai.com
	ProbeBase    = "P6" // 国际链路基线
)

// AllProbes 是默认执行顺序。
var AllProbes = []string{ProbeChatGPT, ProbeGemini, ProbeOpenAI, ProbeBase, ProbeYouTube, ProbeGoogle}

// ── 独立带宽长跑的量纲 ─────────────────────────────────────────────
//
// 这一档刻意不属于六项综合测试。它回答的问题不是"那一刻能跑多快"，
// 而是"连续跑两小时，这条链路还能不能一直跑到该有的速度"——
// 后者才对应"挂了一晚上下载，早上起来发现只下了 200 MB"这类真实遭遇。
//
// 正因为问的是另一件事，它的量纲也不是六项那一套：
// 六项的 1800 秒 / 4096 MB 上限是给"顺手测一下"设的，套在长跑上
// 会让用户填的 2 小时静默变成 30 分钟、填的 20 GB 静默变成 4 GB。
const (
	MaxBandwidthDurationSec = 14400  // 4 小时
	MaxBandwidthBudgetMB    = 102400 // 100 GB
)

// 带宽长跑的默认值：30 分钟 / 8 GB。
// 8 GB 对应约 36 Mbps 的持续吞吐能跑满全程——既不至于开局就被预算截断，
// 也不会让第一次点开这一页的人一上手就吃掉几十 GB 流量。
const (
	bandwidthDefaultDurationSec = 1800
	bandwidthDefaultBudgetMB    = 8192
)

// Config 是一次运行的完整入参。
type Config struct {
	Mode        string   `json:"mode"`
	ProxyURL    string   `json:"proxyUrl"`
	Preset      string   `json:"preset"`
	Concurrency int      `json:"concurrency"`
	BudgetMB    int      `json:"budgetMb"`
	DurationSec int      `json:"durationSec"`
	TimeoutSec  int      `json:"timeoutSec"`
	Probes      []string `json:"probes"`

	// LongRun 打开 P1 的长时稳定性长跑（最长 30 分钟）。
	// 用户报的「好多节点只能下载一会儿、几十秒就不行了」在短测里根本看不出来：
	// 20 秒的均值会把「前 10 秒跑满、后 10 秒停摆」摊成一个还看得过去的数字。
	// 长跑才需要按窗口切分去看吞吐是不是一直在出数据。
	LongRun bool `json:"longRun"`

	// Only 表示这是一次「单项复测」：只跑 Probes 里的那一项，
	// 保留其它探测项的既有结果，也不重算总评。
	// 总评代表一次完整运行；把不同时间窗口的结果混算在一起无法归因。
	Only bool `json:"only"`

	// Bandwidth 打开「独立带宽长跑」：只跑大文件下载，但与六项里的 P1
	// 完全隔离——不写入 Results、不参与总评、结果单独占一个状态位。
	//
	// 为什么单独开一个开关而不是复用 Only+longRun：Only 是"六项之一复测"，
	// 它会把结果写回 P1 那张卡；长跑跑两小时之后，分评页上那张 P1 卡
	// 就再也不是"综合测试里测出来的 P1"了。两件事必须分开存。
	Bandwidth bool `json:"bandwidth"`
}

// Normalize 填充默认值并做范围收敛。
func (c *Config) Normalize() {
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	if c.Mode == "" {
		c.Mode = ModeSystem
	}
	c.ProxyURL = strings.TrimSpace(c.ProxyURL)
	// 带宽长跑是另一套量纲，走完自己那条分支直接返回。
	// 让它继续往下走会被六项的 1800 秒 / 4096 MB 夹死——
	// 用户填的 2 小时会静默变成 30 分钟，而界面上还显示着"2 小时"。
	if c.Bandwidth {
		c.normalizeBandwidth()
		return
	}
	switch c.Preset {
	case PresetStandard, PresetDeep:
	default:
		c.Preset = PresetQuick
	}
	def := presetDefaults(c.Preset)
	if c.Concurrency <= 0 {
		c.Concurrency = def.concurrency
	}
	if c.Concurrency > 16 {
		c.Concurrency = 16
	}
	// 长跑档的"留空即按长跑取值"必须放在通用默认值之前。
	// 否则 DurationSec 会先被档位默认值（8/15/30 秒）填掉，
	// 用户勾了「长时稳定性测试」却只跑了 30 秒，指标面板上会多出一堆 0 窗口，
	// 看起来像"节点断了"，其实是配置没生效。
	if c.LongRun {
		if c.DurationSec <= 0 {
			c.DurationSec = 1800
		}
		if c.BudgetMB <= 0 {
			// 30 分钟 × 峰值算不出的情况下，先按 ~4.5 Mbps 的持续吞吐给出 1 GB 上限：
			// 够跑满 30 分钟不提前截断，又不会把用户的流量套餐当垃圾桶。
			c.BudgetMB = 1024
		}
		if c.Concurrency <= 0 {
			// 长跑测的是"单条流的持续性"，不是峰值带宽。
			// 多流并发时只要有一条流断，总吞吐就会被其它流掩盖，断流反而更难被发现。
			c.Concurrency = 1
		}
	}
	if c.BudgetMB <= 0 {
		c.BudgetMB = def.budgetMB
	}
	if c.BudgetMB > 4096 {
		c.BudgetMB = 4096
	}
	if c.DurationSec <= 0 {
		c.DurationSec = def.durationSec
	}
	// 上限 1800 秒（30 分钟）：长跑需要足够长才能暴露"几十秒后断流"。
	// 再长没有意义——超过 30 分钟的测试用户不会等，而且流量成本不可控。
	if c.DurationSec > 1800 {
		c.DurationSec = 1800
	}
	if c.TimeoutSec <= 0 {
		c.TimeoutSec = def.timeoutSec
	}
	if c.TimeoutSec > 60 {
		c.TimeoutSec = 60
	}
	c.ProxyURL = strings.TrimSpace(c.ProxyURL)
	c.Probes = normalizeProbes(c.Probes)
}

// normalizeBandwidth 收敛独立带宽长跑的参数。
//
// 和六项那套的唯一区别是上限：时长按小时（最长 4 小时）、流量按 GB（最多 100 GB）。
// 其余口径刻意与 P1 保持一致（单流优先、Range 流式、不落盘），
// 否则"独立长跑"和"综合测试里的下载"会变成两把量不出同一件事的尺子。
func (c *Config) normalizeBandwidth() {
	if c.Concurrency <= 0 {
		// 长跑要抓的是"某一条流会不会断"。多流并发时只要还有一条在跑，
		// 总吞吐就看不出来，断流反而被别人的流量盖住。
		c.Concurrency = 1
	}
	if c.Concurrency > 16 {
		c.Concurrency = 16
	}
	if c.DurationSec <= 0 {
		c.DurationSec = bandwidthDefaultDurationSec
	}
	if c.DurationSec > MaxBandwidthDurationSec {
		c.DurationSec = MaxBandwidthDurationSec
	}
	if c.BudgetMB <= 0 {
		c.BudgetMB = bandwidthDefaultBudgetMB
	}
	if c.BudgetMB > MaxBandwidthBudgetMB {
		c.BudgetMB = MaxBandwidthBudgetMB
	}
	if c.TimeoutSec <= 0 {
		c.TimeoutSec = 20
	}
	if c.TimeoutSec > 60 {
		c.TimeoutSec = 60
	}
	// 带宽长跑不是六项之一：不跑任何综合探测项，也不允许参与总评。
	c.Probes = nil
	c.Only = false
	// 长时间本身就是这一档的全部意义，所以恒按长跑口径分析
	// （分窗、断流定位、有效吞吐占比）。短测那套均值在这里只会掩盖问题。
	c.LongRun = true
}

type defaults struct {
	concurrency int
	budgetMB    int
	durationSec int
	timeoutSec  int
}

// presetDefaults 返回各档位的默认参数。
// concurrency 恒为 1：单流下载才是「打开一个文件下载」的真实体感，
// 多流并发会把总带宽叠加成用户实际拿不到的数字（且 HTTP/2 下多条流复用同一条 TCP，
// 界面上写着「N 并发」而实际只有 1 条连接）。需要跑满带宽时由用户在设置里手动调高。
func presetDefaults(p string) defaults {
	switch p {
	case PresetStandard:
		return defaults{1, 64, 15, 20}
	case PresetDeep:
		return defaults{1, 160, 30, 25}
	default:
		return defaults{1, 20, 8, 15}
	}
}

func normalizeProbes(in []string) []string {
	if len(in) == 0 {
		return append([]string(nil), AllProbes...)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = strings.ToUpper(strings.TrimSpace(p))
		if seen[p] || !isProbe(p) {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return append([]string(nil), AllProbes...)
	}
	return out
}

func isProbe(p string) bool {
	for _, v := range AllProbes {
		if v == p {
			return true
		}
	}
	return false
}

// Validate 返回面向用户的错误信息（中文）。
func (c *Config) Validate() error {
	switch c.Mode {
	case ModeSystem, ModeDirect:
		return nil
	case ModeCustom:
		if c.ProxyURL == "" {
			return errors.New("选择了自定义代理，但代理地址为空")
		}
		u, err := url.Parse(c.ProxyURL)
		if err != nil {
			return errors.New("代理地址无法解析，请检查格式")
		}
		switch strings.ToLower(u.Scheme) {
		case "http", "https", "socks5", "socks5h":
		default:
			return errors.New("代理协议仅支持 http / https / socks5 / socks5h")
		}
		if u.Hostname() == "" {
			return errors.New("代理地址缺少主机名")
		}
		if u.Port() == "" {
			return errors.New("代理地址缺少端口，例如 127.0.0.1:7890")
		}
		return nil
	default:
		return errors.New("未知的出口模式")
	}
}
