package probe

import "strings"

// 归因层级：刻意保留"待定"，因为强制选边会输出错误归因。
const (
	AttrLocal   = "local"        // 本地网络/本机环境
	AttrProxy   = "proxy"        // 代理链路
	AttrTarget  = "target"       // 目标服务自身
	AttrUnknown = "undetermined" // 单次不可判定，需补一次直连对照
)

// AttributionOf 依据错误形态给出归因层级。
// 判定不了就返回待定，并在 NextStep 里绑定排除动作。
func AttributionOf(s Status, err error) string {
	if s == StatusOK || s == StatusAuthRequired {
		return ""
	}
	if err == nil {
		switch s {
		case StatusChallenge, StatusRegionBlock, StatusDegraded:
			return AttrTarget
		default:
			return AttrUnknown
		}
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "no such host"), strings.Contains(msg, "dns"):
		return AttrProxy
	case strings.Contains(msg, "connection refused"):
		return AttrProxy
	case strings.Contains(msg, "x509"), strings.Contains(msg, "certificate"):
		return AttrProxy
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "deadline"):
		return AttrUnknown
	default:
		return AttrUnknown
	}
}

// NextStep 给出该归因下唯一值得做的排除动作。
func NextStep(attr string) string {
	switch attr {
	case AttrLocal:
		return "先切「直连」跑一次对照，确认本地网络本身是否正常"
	case AttrProxy:
		return "换一个节点重测；若直连正常、代理异常，问题在代理侧"
	case AttrTarget:
		return "换出口地区或稍后重试；目标站点侧的策略问题，不代表你的网络坏了"
	case AttrUnknown:
		return "切「直连」再跑一次对照，才能区分是代理问题还是目标服务问题"
	default:
		return ""
	}
}
