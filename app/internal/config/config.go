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
}

// Normalize 填充默认值并做范围收敛。
func (c *Config) Normalize() {
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	if c.Mode == "" {
		c.Mode = ModeSystem
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
	if c.BudgetMB <= 0 {
		c.BudgetMB = def.budgetMB
	}
	if c.BudgetMB > 512 {
		c.BudgetMB = 512
	}
	if c.DurationSec <= 0 {
		c.DurationSec = def.durationSec
	}
	if c.DurationSec > 120 {
		c.DurationSec = 120
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

type defaults struct {
	concurrency int
	budgetMB    int
	durationSec int
	timeoutSec  int
}

func presetDefaults(p string) defaults {
	switch p {
	case PresetStandard:
		return defaults{6, 64, 15, 20}
	case PresetDeep:
		return defaults{8, 160, 30, 25}
	default:
		return defaults{5, 20, 8, 15}
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
