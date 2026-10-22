package probe

import "math"

// lerp 分段线性插值，xs 必须升序。
func lerp(x float64, xs, ys []float64) float64 {
	if x <= xs[0] {
		return ys[0]
	}
	for i := 1; i < len(xs); i++ {
		if x <= xs[i] {
			span := xs[i] - xs[i-1]
			if span == 0 {
				return ys[i]
			}
			r := (x - xs[i-1]) / span
			return ys[i-1] + r*(ys[i]-ys[i-1])
		}
	}
	return ys[len(ys)-1]
}

// SpeedScore 把吞吐（Mbps）折算为 0-100。断点即评分口径，公开可解释。
func SpeedScore(mbps float64) float64 {
	xs := []float64{0, 1, 2, 5, 10, 20, 50, 100}
	ys := []float64{0, 25, 40, 58, 72, 85, 95, 100}
	return Round(lerp(mbps, xs, ys), 0)
}

// StabilityScore 由成功率与抖动折算。
func StabilityScore(successPct, jitterMs float64) float64 {
	base := successPct
	penalty := 0.0
	if jitterMs > 30 {
		penalty = math.Min(30, (jitterMs-30)/8)
	}
	return Round(math.Max(0, base-penalty), 0)
}

// StatusScoreOf 导出单探测项状态分，供汇总层调用。
func StatusScoreOf(s Status) float64 { return statusScore(s) }

// statusScore 是单个探测项的状态分。
func statusScore(s Status) float64 {
	switch s {
	case StatusOK:
		return 100
	case StatusAuthRequired:
		return 92
	case StatusDegraded:
		return 55
	case StatusInconclusive:
		return 60
	case StatusRateLimited:
		return 70
	case StatusChallenge:
		return 35
	case StatusRegionBlock:
		return 10
	case StatusHijacked:
		return 5
	case StatusFailed, StatusCancelled, StatusSkipped:
		return 0
	default:
		return 0
	}
}
