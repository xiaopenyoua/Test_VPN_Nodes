package probe

import (
	"net/http"
	"testing"
)

// 这一组测试守的是同一条红线：**探测结论必须和用户浏览器里看到的一致**。
//
// 背景（2026-10-22 真机对照取证）：
//   - chatgpt.com 在本机出口被 Cloudflare 直接拒绝：403 + "Unable to load site /
//     If you are using a VPN, try turning it off"，响应头 cf-mitigated 为空。
//     真浏览器打开得到同一个页面 —— 所以这必须判「受限/打不开」，不能判成验证墙。
//   - youtube.com 同一环境里真浏览器播放正常（video.readyState=4，duration 634s），
//     而匿名的 youtubei/v1/player 却回 LOGIN_REQUIRED —— 说明"匿名接口要求验证"
//     绝不能当成"YouTube 被封控"。

func newBody(status int, body string, hdr map[string]string) *FetchResult {
	h := http.Header{}
	for k, v := range hdr {
		h.Set(k, v)
	}
	return &FetchResult{
		Status:   status,
		FinalURL: "https://example.com/",
		Header:   h,
		Body:     []byte(body),
		BodyLen:  int64(len(body)),
	}
}

// Cloudflare 直接拒绝出口 IP：浏览器同样打不开，必须判 degraded，不能判 challenge。
func TestClassifyCloudflareHardDenyIsNotChallenge(t *testing.T) {
	f := newBody(403,
		`<html><head><title>Attention Required! | Cloudflare</title></head><body>`+
			`Unable to load site<br>Please try again later. If you are using a VPN, try turning it off.`+
			`<br>[Ray ID: a4e66a60285e643b]</body></html>`,
		map[string]string{"Server": "cloudflare", "Cf-Ray": "a4e66a60285e643b-HKG"})

	st, ev, hint := Classify("P3", f)
	if st != StatusDegraded {
		t.Fatalf("Cloudflare 硬拒绝应判 degraded，实际 %s（证据：%v）", st, ev)
	}
	if hint == "" {
		t.Fatal("必须给出可读解释，不能只留一个状态码")
	}
}

// cf-mitigated: challenge 是平台自己声明的「这是验证页，不是拒绝」，保持 challenge。
func TestClassifyChallengeHeaderStaysChallenge(t *testing.T) {
	f := newBody(403, `<html><body>Just a moment...</body></html>`,
		map[string]string{"Server": "cloudflare", "Cf-Mitigated": "challenge"})

	st, _, _ := Classify("P3", f)
	if st != StatusChallenge {
		t.Fatalf("cf-mitigated: challenge 应判 challenge，实际 %s", st)
	}
}

// 匿名 watch 页带「先证明你不是机器人」，但拿到了播放器数据 —— 浏览器能播（已实机对照），
// 绝不能判 challenge。这是本次修复的核心假阴性。
func TestYouTubeAnonymousVerifyWithPlayerDataIsNotChallenge(t *testing.T) {
	body := `<html><head><title>Big Buck Bunny - YouTube</title></head><body>` +
		`<script>var ytInitialPlayerResponse = {"playabilityStatus":{"status":"LOGIN_REQUIRED",` +
		`"reason":"Sign in to confirm you&#39;re not a bot"}};var ytInitialData = {};</script>` +
		`</body></html>`

	f := newBody(200, body, map[string]string{"Content-Type": "text/html"})
	st, hint := ytClassifyPage(f, "LOGIN_REQUIRED", "Sign in to confirm you're not a bot")
	if st == StatusChallenge {
		t.Fatalf("匿名要求验证 + 有播放器数据时不应判 challenge（会产出假阴性），实际 %s：%s", st, hint)
	}
	if st != StatusOK {
		t.Fatalf("应判 ok（页面本体可开），实际 %s：%s", st, hint)
	}
}

// 反过来：连播放器数据都没有、页面本身就是一张验证插页时，必须降级并要求人工实开。
func TestYouTubeAnonymousVerifyWithoutPlayerDataDegrades(t *testing.T) {
	f := newBody(200, `<html><body>Sign in to confirm you're not a bot</body></html>`,
		map[string]string{"Content-Type": "text/html"})
	st, hint := ytClassifyPage(f, "", "")
	if st != StatusDegraded {
		t.Fatalf("没有播放器数据的验证插页应判 degraded，实际 %s：%s", st, hint)
	}
}
