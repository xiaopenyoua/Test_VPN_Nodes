package probe

import (
	"strings"
	"testing"
)

// 这一组测试守的是地区层的两条红线：
//  1. 只有 OpenAI 自述的 unsupported_country 才算硬地区证据，其余一律不得定性强降级；
//  2. trace 解析一旦错位（把 warp=off 读成 on），整条结论会反过来，而真机上看不出来。

// 真实 trace 正文（本机 2026-10-22 实测原样，只把 ip= 换成占位符）。
// warp=on + loc=SG + colo=SIN 就是"地区看着没问题、但出口是 WARP 共享 IP"的那个现场。
const realTraceSample = `fl=22f434
h=chatgpt.com
ip=203.0.113.9
ts=1792652894.000
visit_scheme=https
uag=Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36
colo=SIN
sliver=010-tier1
http=http/2
loc=SG
tls=TLSv1.3
sni=plaintext
warp=on
gateway=off
rbi=off
kex=X25519
`

func TestParseTraceRealSample(t *testing.T) {
	loc, colo, warp := parseTrace([]byte(realTraceSample))
	if loc != "SG" {
		t.Fatalf("loc 应解析为 SG，实际 %q", loc)
	}
	if colo != "SIN" {
		t.Fatalf("colo 应解析为 SIN，实际 %q", colo)
	}
	if !warp {
		t.Fatal("warp=on 必须解析为 true：它才是被 OpenAI 按 IP 段拒绝的真正病因")
	}
}

// warp=off 绝不能被读成 on：误报会把用户引去"换非 WARP 节点"这个无效动作。
func TestParseTraceWarpOff(t *testing.T) {
	body := "loc=JP\ncolo=NRT\nwarp=off\ngateway=off\n"
	loc, colo, warp := parseTrace([]byte(body))
	if loc != "JP" || colo != "NRT" {
		t.Fatalf("解析错位：loc=%q colo=%q", loc, colo)
	}
	if warp {
		t.Fatal("warp=off 不能解析为 true")
	}
}

// 没有 warp 行（老网关/中间层裁剪）时按"不是 WARP"处理，宁可漏报也不误标。
func TestParseTraceMissingWarp(t *testing.T) {
	loc, _, warp := parseTrace([]byte("h=example.com\nloc=US\n"))
	if loc != "US" {
		t.Fatalf("loc 应解析为 US，实际 %q", loc)
	}
	if warp {
		t.Fatal("缺少 warp 行时不能判定为 WARP 出口")
	}
}

// 拿到的不是 trace（例如 gemini.google.com 回 Google 的 404 页）时，
// 必须解析不出任何东西，调用方才能保持沉默而不是硬造地区。
func TestParseTraceHTMLReturnsEmpty(t *testing.T) {
	html := `<html><head><title>Error 404 (Not Found)!!1</title></head><body>loc=XX 只是正文里的字</body></html>`
	loc, colo, warp := parseTrace([]byte(html))
	if colo != "" || warp {
		t.Fatalf("HTML 不能解析出 colo/warp，实际 colo=%q warp=%v", colo, warp)
	}
	// 单行 HTML 里没有 k=v 行，"loc=XX" 夹在正文中间不构成合法行，必须读不到。
	if loc != "" {
		t.Fatalf("HTML 正文里不该解析出 loc，实际 %q", loc)
	}
}

func TestRegionBlockedByOpenAI(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"OpenAI 硬证据", `{"error":{"message":"...","code":"unsupported_country"}}`, true},
		{"正常返回不是地区封", `{"cookie_consent_required": false}`, false},
		{"大小写不敏感", `{"error":{"code":"Unsupported_Country"}}`, true},
		{"空正文不做判断", ``, false},
		{"泛泛的 country 字样不算证据", `{"error":{"code":"rate_limit_exceeded","message":"check your country later"}}`, false},
	}
	for _, c := range cases {
		if got := RegionBlockedByOpenAI([]byte(c.body)); got != c.want {
			t.Fatalf("%s：期望 %v，实际 %v（正文 %q）", c.name, c.want, got, c.body)
		}
	}
}

// 证据行的格式是给用户看的，必须稳定：loc / colo / warp 三项都要出现。
func TestTraceEvidenceLine(t *testing.T) {
	got := traceEvidence("SG", "SIN", true)
	want := "Cloudflare 看到出口 loc=SG · colo=SIN · warp=on"
	if got != want {
		t.Fatalf("证据行格式不符：\n期望 %q\n实际 %q", want, got)
	}
	if got := traceEvidence("SG", "", false); got != "Cloudflare 看到出口 loc=SG · warp=off" {
		t.Fatalf("colo 缺失时不应留空占位，实际 %q", got)
	}
}

// warp 的 NextStep 必须是可执行的真话：说清病因（共享 IP 被 CF 拒），
// 明确"换非 WARP 节点"，并堵死"原地重试"这个无效动作。
func TestWarpNextStepIsActionable(t *testing.T) {
	s := warpNextStep()
	for _, key := range []string{"WARP", "非 WARP", "重试同一节点不会变", "403"} {
		if !strings.Contains(s, key) {
			t.Fatalf("NextStep 缺少关键信息 %q，实际 %q", key, s)
		}
	}
}
