package probe

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestDownloadUsesHTTP1AndRealConnections 守住一个曾把 P1 打成 failed 的坑：
// 下载探针为了「N 并发名副其实」要禁 h2，但如果只在 Transport 上设
// ForceAttemptHTTP2=false / 空 TLSNextProto，ALPN 仍会协商成 h2，
// 客户端却用 HTTP/1.x 解析 h2 帧，得到
// "malformed HTTP response ... http2_handshake_failed"，所有端点全部超时。
// 正确做法是连 TLSClientConfig.NextProtos 一起摘掉 h2。
func TestDownloadUsesHTTP1AndRealConnections(t *testing.T) {
	var (
		mu     sync.Mutex
		protos = map[string]int{}
		peers  = map[string]struct{}{}
	)

	payload := make([]byte, 64<<10)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		protos[r.Proto]++
		// 必须用 host:port 做键：本地测试服务的对端 IP 恒为 127.0.0.1，
		// 只取 host 会把 N 条连接压成 1 条，测不出并发真伪。
		peers[r.RemoteAddr] = struct{}{}
		mu.Unlock()

		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload)
	}))
	srv.EnableHTTP2 = true // 服务端支持 h2，正是会诱发 ALPN 错配的场景
	srv.StartTLS()
	defer srv.Close()

	old := dlEndpoints
	dlEndpoints = []dlEndpoint{{srv.URL + "/blob", "test"}}
	defer func() { dlEndpoints = old }()

	const conc = 4
	tr := &http.Transport{
		ForceAttemptHTTP2: true, // 上层是浏览器同构，默认允许 h2
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
	}
	res := RunDownload(context.Background(), tr, DownloadOpts{
		Concurrency: conc,
		BudgetBytes: 8 << 20,
		Duration:    2 * time.Second,
	})

	if res.Status != StatusOK {
		t.Fatalf("下载探针应成功，实际 status=%s error=%q evidence=%v", res.Status, res.Error, res.Evidence)
	}

	mu.Lock()
	defer mu.Unlock()
	if n := protos["HTTP/2.0"]; n > 0 {
		t.Errorf("下载探针必须走 HTTP/1.1（否则 N 并发是假的、且会触发 ALPN 错配），实际有 %d 个 h2 请求", n)
	}
	if protos["HTTP/1.1"] == 0 {
		t.Errorf("没有观测到任何 HTTP/1.1 请求，protos=%v", protos)
	}
	// h2 下 4 个并发会复用 1 条 TCP；要证明并发是真的，必须看到多条独立连接。
	if len(peers) < conc {
		t.Errorf("声明 %d 并发，实际只有 %d 条独立连接，并发标注不成立", conc, len(peers))
	}
	// ConnCount 只统计下载阶段的请求（开头的端点探测走 Fetch，不入 trace），
	// 因此允许比服务端看到的少一点，但必须达到声明并发数。
	if res.ConnCount < conc {
		t.Errorf("自证连接数应不小于声明并发 %d，实际上报 %d", conc, res.ConnCount)
	}
}

// TestDownloadFailsWithActionableDiagnosis 守住「失败必须自己解释清楚」这条线：
// 用户反馈下载项总是失败，但卡片只给一个红色失败、标题空着、没有任何归因，
// 无法判断是代理坏了还是网络断了。这条测试要求失败结果必须带
// 标题 / 逐个端点的失败原因 / 归因 / 下一步。
func TestDownloadFailsWithActionableDiagnosis(t *testing.T) {
	// 用一个刚被释放的本地端口，连接会被明确拒绝（比硬编码 1 端口更可移植）
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("申请空闲端口失败: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	old := dlEndpoints
	dlEndpoints = []dlEndpoint{
		{"http://" + addr + "/a", "dead-a"},
		{"http://" + addr + "/b", "dead-b"},
	}
	defer func() { dlEndpoints = old }()

	res := RunDownload(context.Background(), &http.Transport{}, DownloadOpts{
		Concurrency: 2,
		BudgetBytes: 1 << 20,
		Duration:    500 * time.Millisecond,
	})

	if res.Status != StatusFailed {
		t.Fatalf("所有端点都连不上时必须判 failed，实际 status=%s error=%q", res.Status, res.Error)
	}
	if res.Headline == "" {
		t.Error("失败时卡片标题不能为空（否则 UI 上是一片空白）")
	}
	if res.Subline == "" {
		t.Error("失败时副标题要说明试了几个端点，否则看不出是单点失败还是全都不通")
	}
	if res.Error == "" {
		t.Error("失败必须有原因说明")
	}
	if res.Attribution != AttrProxy {
		t.Errorf("连接被拒绝应归因代理侧，实际 %q", res.Attribution)
	}
	if res.NextStep == "" {
		t.Error("失败必须给出下一步动作")
	}
	if len(res.Evidence) < len(dlEndpoints) {
		t.Errorf("应逐个端点记录失败原因（%d 个），实际只有 %d 条：%v", len(dlEndpoints), len(res.Evidence), res.Evidence)
	}
}
