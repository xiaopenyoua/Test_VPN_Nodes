package engine

import (
	"fmt"
	"strings"

	"proxylens/internal/probe"
)

// MarkdownReport 生成可直接贴进 issue / 聊天窗口的纯文本报告。
func MarkdownReport(st *State) string {
	var b strings.Builder
	b.WriteString("# ProxyLens 探测报告\n\n")
	b.WriteString(fmt.Sprintf("- 运行时间：%s\n", st.StartedAt))
	b.WriteString(fmt.Sprintf("- 耗时：%.0f ms\n", st.ElapsedMs))
	b.WriteString(fmt.Sprintf("- 出口方式：%s\n", st.Exit.Detail))
	if st.Exit.Effective != "" {
		b.WriteString(fmt.Sprintf("- 实际代理：`%s`\n", maskReportURL(st.Exit.Effective)))
	} else {
		b.WriteString("- 实际代理：直连（无代理）\n")
	}
	b.WriteString(fmt.Sprintf("- 环境：%s / %s · ProxyLens %s\n\n", st.Env.OS, st.Env.Arch, st.Env.Version))

	if st.Summary != nil {
		b.WriteString(fmt.Sprintf("## 结论：%s（%.0f 分 · %s）\n\n", st.Summary.StateLabel, st.Summary.Total, st.Summary.Grade))
		b.WriteString(st.Summary.Verdict + "\n\n")
		b.WriteString("| 维度 | 得分 | 权重 | 依据 |\n|---|---|---|---|\n")
		for _, d := range st.Summary.Dimensions {
			b.WriteString(fmt.Sprintf("| %s | %.0f | %.0f%% | %s |\n", d.Label, d.Score, d.Weight*100, d.Detail))
		}
		if len(st.Summary.Caps) > 0 {
			b.WriteString("\n> 封顶规则：" + strings.Join(st.Summary.Caps, "；") + "\n")
		}
		if len(st.Summary.Advice) > 0 {
			b.WriteString("\n**建议**\n\n")
			for _, a := range st.Summary.Advice {
				b.WriteString("- " + a + "\n")
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("## 明细\n\n")
	b.WriteString("| 探测项 | 状态 | 主指标 | DNS | TCP | TLS | TTFB |\n|---|---|---|---|---|---|---|\n")
	for _, r := range st.Results {
		b.WriteString(fmt.Sprintf("| %s %s | %s | %s | %.0f | %.0f | %.0f | %.0f |\n",
			r.ID, r.Name, statusLabel(r.Status), r.Headline,
			r.Phases.DNS, r.Phases.TCP, r.Phases.TLS, r.Phases.TTFB))
	}

	b.WriteString("\n### 证据链\n\n")
	for _, r := range st.Results {
		if len(r.Evidence) == 0 && r.Error == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("**%s %s**\n", r.ID, r.Name))
		for _, e := range r.Evidence {
			b.WriteString("- " + e + "\n")
		}
		if r.Error != "" {
			b.WriteString("- 错误：" + r.Error + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n")
	// 权重不能写死：一个手写的"30%/30%/40%"会在权重调整或某维度未测后
	// 与实际算法脱节，让报告自己说谎。这里直接从 summary 反推口径。
	if st.Summary != nil && len(st.Summary.Dimensions) > 0 {
		parts := make([]string, 0, len(st.Summary.Dimensions))
		for _, d := range st.Summary.Dimensions {
			if !d.Measured {
				parts = append(parts, d.Label+" 未测（不参与加权）")
				continue
			}
			parts = append(parts, fmt.Sprintf("%s %.0f%%", d.Label, d.Weight*100))
		}
		b.WriteString("评分口径：" + strings.Join(parts, " + ") + "；分项归一化为分段线性，断点公开见项目文档 06-engine-design.md。\n")
	} else {
		b.WriteString("评分口径：速度 40% + 稳定性 30% + AI 可用性 30%；某维度未测则不参与加权；分项归一化为分段线性，断点公开见项目文档 06-engine-design.md。\n")
	}
	b.WriteString("本报告仅在本机生成，不含任何凭据。\n")
	return b.String()
}

func maskReportURL(raw string) string {
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		if at := strings.Index(rest, "@"); at >= 0 {
			cred := rest[:at]
			if colon := strings.Index(cred, ":"); colon >= 0 {
				return raw[:i+3] + cred[:colon] + ":***" + rest[at:]
			}
		}
	}
	return raw
}

func statusLabel(s probe.Status) string {
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
