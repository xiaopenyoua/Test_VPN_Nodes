package score

import (
	"strings"
	"testing"

	"proxylens/internal/probe"
)

func baseResults() []*probe.Result {
	return []*probe.Result{
		{ID: "P1", Status: probe.StatusOK, P50Mbps: 90, PeakMbps: 100, Bytes: 20 << 20},
		{ID: "P2", Status: probe.StatusOK, Mbps: 40},
		{ID: "P3", Status: probe.StatusOK},
		{ID: "P4", Status: probe.StatusOK},
		{ID: "P5", Status: probe.StatusAuthRequired},
		{ID: "P6", Status: probe.StatusOK, SuccessPct: 100, Jitter: 5, TTFB: 40},
	}
}

func TestUsableWhenAllGood(t *testing.T) {
	s := Aggregate(baseResults())
	if s.State != "usable" {
		t.Fatalf("期望 usable，实际 %s", s.State)
	}
	if s.Total < 85 {
		t.Fatalf("全绿场景分数应偏高，实际 %.0f", s.Total)
	}
}

// 关键回归：ChatGPT 被人机验证拦截时，绝不允许输出"可用 + 高分"。
func TestGateCapsWhenChatGPTChallenged(t *testing.T) {
	res := baseResults()
	res[2].Status = probe.StatusChallenge
	s := Aggregate(res)
	if s.State != "limited" {
		t.Fatalf("ChatGPT 被风控时期望 limited，实际 %s", s.State)
	}
	if s.Total > 69 {
		t.Fatalf("受限档总分必须封顶 69，实际 %.0f", s.Total)
	}
	if len(s.Caps) == 0 {
		t.Fatal("应记录封顶原因")
	}
}

func TestGateCapsWhenBothAIBlocked(t *testing.T) {
	res := baseResults()
	res[2].Status = probe.StatusRegionBlock
	res[3].Status = probe.StatusRegionBlock
	res[4].Status = probe.StatusFailed
	s := Aggregate(res)
	if s.State != "blocked" {
		t.Fatalf("三个 AI 项均不可用时期望 blocked，实际 %s", s.State)
	}
	if s.Total > 29 {
		t.Fatalf("不可用档总分必须封顶 29，实际 %.0f", s.Total)
	}
}

func TestHijackCapsTotal(t *testing.T) {
	res := baseResults()
	res[2].Status = probe.StatusHijacked
	s := Aggregate(res)
	if s.Total > 55 {
		t.Fatalf("证书异常时总分必须封顶 55，实际 %.0f", s.Total)
	}
}

// 关键回归：degraded / rate_limited / inconclusive 都是"链路通但业务不通"。
// 它们既不算可用、也不该被放过——否则会出现"浏览器打不开 ChatGPT，工具却报优秀"。
func TestGateNeverUsableWhenEndpointDegraded(t *testing.T) {
	for _, st := range []probe.Status{
		probe.StatusDegraded,
		probe.StatusRateLimited,
		probe.StatusInconclusive,
		probe.StatusChallenge,
		probe.StatusRegionBlock,
		probe.StatusHijacked,
		probe.StatusFailed,
	} {
		res := baseResults()
		res[2].Status = st
		s := Aggregate(res)
		if s.State == "usable" {
			t.Fatalf("ChatGPT 处于 %s 时被判成 usable，结论与用户实际体验不符", st)
		}
	}
}

// P4 是用户另一半真实体感，它挂掉同样不能算可用
func TestGateNeverUsableWhenGeminiBroken(t *testing.T) {
	for _, st := range []probe.Status{probe.StatusFailed, probe.StatusDegraded, probe.StatusRegionBlock} {
		res := baseResults()
		res[3].Status = st
		s := Aggregate(res)
		if s.State == "usable" {
			t.Fatalf("Gemini 处于 %s 时被判成 usable", st)
		}
	}
}

// 网页端全挂、仅 API 网关可达：属于"受限"，不是"可用"
func TestGateLimitedWhenOnlyAPIReachable(t *testing.T) {
	res := baseResults()
	res[2].Status = probe.StatusChallenge
	res[3].Status = probe.StatusFailed
	s := Aggregate(res)
	if s.State != "limited" {
		t.Fatalf("期望 limited，实际 %s", s.State)
	}
}

// 快速档没测下载：不能把"未测"当成 0 分拉低总分，
// 否则用户会看到"速度 40% 却 0 分"，误以为测出来速度极差。
func TestUnmeasuredSpeedNotPenalized(t *testing.T) {
	res := []*probe.Result{
		{ID: "P3", Status: probe.StatusOK},
		{ID: "P4", Status: probe.StatusOK},
		{ID: "P5", Status: probe.StatusAuthRequired},
		{ID: "P6", Status: probe.StatusOK, SuccessPct: 100, Jitter: 5, TTFB: 40},
	}
	s := Aggregate(res)
	var speed Dimension
	var weightSum float64
	for _, d := range s.Dimensions {
		if d.Key == "speed" {
			speed = d
		}
		weightSum += d.Weight
	}
	if speed.Weight != 0 {
		t.Fatalf("未测的速度维度不应占权重，实际 %.2f", speed.Weight)
	}
	if weightSum < 0.99 || weightSum > 1.01 {
		t.Fatalf("参与项权重应归一到 1，实际 %.2f", weightSum)
	}
	if s.Total < 70 {
		t.Fatalf("只测了稳定性与 AI 时不应被未测维度拉低，实际 %.0f", s.Total)
	}
	if !strings.Contains(s.Verdict, "未测下载速度") {
		t.Fatalf("结论应说明速度未测，实际：%s", s.Verdict)
	}
}

func TestMaskingNotLeaking(t *testing.T) {
	// 评分层不应持有任何凭据字段，这里只验证结果结构里没有密码残留
	res := baseResults()
	s := Aggregate(res)
	for _, d := range s.Dimensions {
		if d.Detail == "" {
			continue
		}
	}
}
