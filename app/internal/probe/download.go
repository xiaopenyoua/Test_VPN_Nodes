package probe

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type dlEndpoint struct {
	URL  string
	Name string
}

// dlEndpoints 按优先级排列：Google 官方 CDN → 备用路径 → Google 另一条边缘（gvt1）→ Cloudflare 对照。
//
// 为什么要多放两条：单条主 URL 失败无法区分「dl.google.com 被针对性拦截」
// 与「整条链路当时不通」。多端点交叉失败/成功，才能把责任层说清楚；
// Cloudflare 只作对照，绝不单独作为唯一来源（其在本机 TUN 下实测就通不了）。
var dlEndpoints = []dlEndpoint{
	{"https://dl.google.com/chrome/mac/universal/stable/GGRO/googlechrome.dmg", "dl.google.com"},
	{"https://dl.google.com/chrome/mac/stable/GGRO/googlechrome.dmg", "dl.google.com（备用路径）"},
	{"https://redirector.gvt1.com/edgedl/go/go1.22.0.darwin-arm64.pkg", "redirector.gvt1.com"},
	{"https://speed.cloudflare.com/__down", "speed.cloudflare.com"},
}

// dlProbe 发一次 64KB Range 探测，瞬态失败重试一次。
//
// 单次探测失败就否定整个端点，会把链路瞬断误判成「所有下载端点不可达」：
// 用户在 UI 上看到红色失败，过一分钟重测又好了，等于工具自证不准。
// 这与 P3/P4 的瞬态重试同一口径（见 isTransient）。
func dlProbe(ctx context.Context, tr *http.Transport, u string) (*FetchResult, int) {
	const attempts = 2
	var f *FetchResult
	for i := 0; i < attempts; i++ {
		f = Fetch(ctx, tr, FetchOpts{URL: u, Header: map[string]string{"Range": "bytes=0-65535"}, MaxBody: 1 << 16})
		if f.Err == nil || ctx.Err() != nil || !isTransient(classifyError(f.Err), f.Err) {
			return f, i + 1
		}
		if i < attempts-1 {
			select {
			case <-ctx.Done():
				return f, i + 1
			case <-time.After(time.Duration(800*(i+1)) * time.Millisecond):
			}
		}
	}
	return f, attempts
}

// RunDownload 执行 P1：多并发 Range 流式下载，数据全部丢进 io.Discard，不落盘。
func RunDownload(ctx context.Context, tr *http.Transport, conc int, budgetBytes int64, dur time.Duration, onSample func(Sample)) *Result {
	m := MetaByID("P1")
	res := &Result{ID: m.ID, Name: m.Name, Group: m.Group, Status: StatusRunning, Samples: []Sample{}}
	started := time.Now()

	// 下载场景必须让"N 并发"是真的：上层 Transport 为浏览器同构开了 HTTP/2，
	// 多个 Range 请求会在同一条连接上多路复用——实际只有一条 TCP，
	// 与界面标注的并发数不符，测出的吞吐也系统性偏低。
	// 这里显式禁用 h2，让每个并发流各占一条连接，口径才和"并发下载"一致。
	dlTr := tr.Clone()
	dlTr.DisableCompression = true
	// 必须在 TLS 层把 h2 从 ALPN 里摘掉。只设 ForceAttemptHTTP2=false 或空 TLSNextProto
	// 是不够的：clone 过来的 TLSClientConfig 里 ALPN 仍声明 h2，服务端照 h2 回 SETTINGS 帧，
	// 客户端却用 HTTP/1.x 解析 → "malformed HTTP response ... http2_handshake_failed"，
	// 表现为所有端点都超时、P1 直接判 failed。
	if dlTr.TLSClientConfig != nil {
		dlTr.TLSClientConfig = dlTr.TLSClientConfig.Clone()
	} else {
		dlTr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	dlTr.TLSClientConfig.NextProtos = []string{"http/1.1"}
	dlTr.ForceAttemptHTTP2 = false
	dlTr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	dlTr.MaxIdleConnsPerHost = conc
	dlTr.MaxConnsPerHost = 0
	dlTr.DisableKeepAlives = false
	client := &http.Client{Transport: dlTr}

	// 选定端点：先做一次 64KB 探测，确认 Range 与可达性（瞬态失败会重试一次）
	chosen := ""
	var firstFetch *FetchResult
	var firstErr error
	certSeen := false
	tried := 0
	for _, ep := range dlEndpoints {
		if ctx.Err() != nil {
			break
		}
		u := ep.URL
		if ep.Name == "speed.cloudflare.com" {
			u = fmt.Sprintf("%s?bytes=%d", ep.URL, 4<<20)
		}
		f, tries := dlProbe(ctx, dlTr, u)
		tried++
		if f.Err == nil && f.Status < 400 && f.BodyLen > 0 {
			chosen = u
			firstFetch = f
			res.Subline = ep.Name
			ev := fmt.Sprintf("%s 可用（HTTP %d，TTFB %.0f ms，%d B）", ep.Name, f.Status, f.Phases.TTFB, f.BodyLen)
			if tries > 1 {
				ev += fmt.Sprintf("，第 %d 次才成功", tries)
			}
			res.Evidence = append(res.Evidence, ev)
			break
		}
		if f.Err != nil {
			msg := f.Err.Error()
			if firstErr == nil {
				firstErr = f.Err
			}
			if strings.Contains(msg, "x509") || strings.Contains(msg, "certificate") {
				certSeen = true
			}
		}
		ev := fmt.Sprintf("%s 不可用：%s", ep.Name, errText(f))
		if tries > 1 {
			ev += fmt.Sprintf("（试了 %d 次）", tries)
		}
		res.Evidence = append(res.Evidence, ev)
	}
	if chosen == "" {
		res.Status = StatusFailed
		res.Score = statusScore(res.Status)
		res.Headline = "下载失败"
		// 副标题必须说清"试了几个端点"：否则卡片上只有一行红字，
		// 用户无法判断是"只打了一个地址失败"还是"全网都拿不到数据"。
		res.Subline = fmt.Sprintf("%d 个下载端点全部失败", tried)
		res.DurationMs = Round(float64(time.Since(started).Microseconds())/1000, 0)
		res.Error = "所有下载端点都没能取到数据（已逐个重试）"
		// 失败也要给归因与下一步：只报红不给方向，用户只能自己猜是代理坏了还是网络断了。
		// 归因优先级：证书异常 > 代理端口拒绝连接 > 待定。
		// 证书与"proxyconnect ... connection refused" 都是硬证据；
		// 其余（超时 / EOF）单轮无法区分链路瞬断与端点被拦，必须留待定。
		res.Attribution = AttributionOf(StatusFailed, firstErr)
		if certSeen {
			res.Attribution = AttrProxy
			res.Error = "所有下载端点都因证书异常失败"
		} else if res.Attribution == AttrProxy {
			res.Error = "代理端口拒绝连接：代理软件没启动，或端口填错了"
		}
		switch res.Attribution {
		case AttrProxy:
			res.NextStep = "确认代理软件在运行、端口填写正确（或改用系统代理模式），再单独重测本项"
		default:
			res.NextStep = "先单独重测一次本项；若仍全部失败，切「直连」跑同一项做对照，才能区分代理问题与本地网络问题"
		}
		return res
	}
	res.Phases = firstFetch.Phases
	res.TTFB = firstFetch.Phases.TTFB
	res.HTTPStatus = firstFetch.Status
	res.FinalURL = firstFetch.FinalURL
	res.TLSVersion = firstFetch.TLSVersion
	res.RemoteAddr = firstFetch.RemoteAddr

	var (
		total   atomic.Int64
		wg      sync.WaitGroup
		stop    = make(chan struct{})
		chunk   = int64(4 << 20) // 每个 Range 请求 4MB
		deadCtx = ctx
		// 请求级计数：端点探测成功 ≠ 真的能下载。
		// 没有这两个计数时，"连上了但一个字节都下不来"会被渲染成
		// 「0.0 Mbps · 可用」，比直接报失败更误导。
		okReqs  atomic.Int64
		errReqs atomic.Int64
		lastErr atomic.Value
	)

	// 并发真伪自证：httptrace 的 GotConn 会给出每次请求实际用的连接，
	// 按本地端口去重即可得到"到底开了几条 TCP"。
	// 这样 subline 上的"N 并发"是实测出来的，不是我们自己声明的。
	var conns sync.Map

	// 采样器：每 250ms 输出一个瞬时速率点
	samples := make([]float64, 0, 256)
	series := make([]Sample, 0, 256)
	var sMu sync.Mutex
	samplerDone := make(chan struct{})
	go func() {
		defer close(samplerDone)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		lastBytes := int64(0)
		lastAt := time.Now()
		for {
			select {
			case <-stop:
				return
			case now := <-ticker.C:
				cur := total.Load()
				delta := cur - lastBytes
				dt := now.Sub(lastAt).Seconds()
				lastBytes, lastAt = cur, now
				mbps := 0.0
				if dt > 0 {
					mbps = float64(delta) * 8 / 1e6 / dt
				}
				samp := Sample{T: float64(now.Sub(started).Milliseconds()), Mbps: Round(mbps, 2), Bytes: cur}
				sMu.Lock()
				samples = append(samples, Round(mbps, 2))
				// 同时留一份带时间轴的序列给前端画速率曲线。
				// 之前只往 []float64 里塞，res.Samples 永远是空的，曲线一直是张白图。
				series = append(series, samp)
				sMu.Unlock()
				if onSample != nil {
					onSample(samp)
				}
			}
		}
	}()

	deadline := time.Now().Add(dur)
	for i := 0; i < conc; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for {
				if time.Now().After(deadline) || total.Load() >= budgetBytes {
					return
				}
				select {
				case <-deadCtx.Done():
					return
				default:
				}
				reqCtx, cancel := context.WithTimeout(deadCtx, 20*time.Second)
				sep := "?"
				if containsQ(chosen) {
					sep = "&"
				}
				u := fmt.Sprintf("%s%scb=%d_%d_%d", chosen, sep, idx, time.Now().UnixNano(), rand.Intn(1e6))
				req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
				if err != nil {
					cancel()
					return
				}
				req.Header.Set("User-Agent", BrowserUA)
				req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", chunk-1))
				req.Header.Set("Accept-Encoding", "identity")
				req = req.WithContext(httptrace.WithClientTrace(reqCtx, &httptrace.ClientTrace{
					GotFirstResponseByte: func() {},
					GotConn: func(info httptrace.GotConnInfo) {
						if la := info.Conn.LocalAddr(); la != nil {
							conns.Store(la.String(), struct{}{})
						}
					},
				}))

				resp, err := client.Do(req)
				if err != nil {
					cancel()
					if ctx.Err() != nil {
						return
					}
					errReqs.Add(1)
					lastErr.Store(err.Error())
					// 失败后必须让出：链路断了的时候紧循环重试会把这台机器的
					// 一个核跑满整个时长（用户看到的是风扇狂转 + 依旧失败）。
					select {
					case <-deadCtx.Done():
						return
					case <-time.After(200 * time.Millisecond):
					}
					continue
				}
				okReqs.Add(1)
				buf := make([]byte, 32<<10)
				for {
					if time.Now().After(deadline) || total.Load() >= budgetBytes {
						break
					}
					n, rerr := resp.Body.Read(buf)
					if n > 0 {
						total.Add(int64(n))
					}
					if rerr != nil {
						break
					}
				}
				resp.Body.Close()
				cancel()
			}
		}(i)
	}

	wg.Wait()
	close(stop)
	<-samplerDone

	connN := 0
	conns.Range(func(_, _ any) bool { connN++; return true })
	res.ConnCount = connN

	elapsed := time.Since(started).Seconds()
	res.Bytes = total.Load()
	res.DurationMs = Round(elapsed*1000, 0)
	res.Mbps = Round(float64(res.Bytes)*8/1e6/elapsed, 2)

	// 端点探测通过、却一个字节都没下到 → 这是真失败，不是"0 Mbps 的可用"。
	// 以前这种情形会落到下面的 degraded/ok 分支，卡片显示「0.0 Mbps」，
	// 用户根本分不清是链路极慢还是一点都下不来。
	if res.Bytes < 64*1024 && okReqs.Load() == 0 && errReqs.Load() > 0 {
		sMu.Lock()
		res.Samples = series
		sMu.Unlock()
		msg := fmt.Sprintf("端点探测通过，但 %d 个下载请求全部失败，一个字节都没取到", errReqs.Load())
		if v, ok := lastErr.Load().(string); ok && v != "" {
			msg += "：" + v
		}
		res.Status = StatusFailed
		res.Headline = "下载失败"
		res.Error = msg
		res.Score = statusScore(StatusFailed)
		res.Attribution = AttrUnknown
		res.NextStep = "端点可达但拿不到数据：先在浏览器里下载一次同名文件对照；若浏览器能下、本工具不能，多为代理对 Range 请求或大文件的处理异常"
		res.Evidence = append(res.Evidence, msg)
		return res
	}

	sMu.Lock()
	vals := make([]float64, len(samples))
	copy(vals, samples)
	res.Samples = series
	sMu.Unlock()

	// 丢弃启动前 1 个采样点（含 TCP 慢启动空窗），避免把 0 值算进分布
	if len(vals) > 3 {
		vals = vals[1:]
	}
	if len(vals) > 0 {
		res.P50Mbps = Round(Percentile(vals, 50), 2)
		res.P90Mbps = Round(Percentile(vals, 90), 2)
		res.PeakMbps = Round(maxOf(vals), 2)
		mean := Mean(vals)
		if mean > 0 {
			res.CV = Round(StdDev(vals)/mean, 3)
		}
		// 停滞检测：均值会把「前段跑满、后段彻底停摆」摊成一个看起来还行的数字。
		// 但用户实际体验是"下载到一半卡死"，必须单独识别出来，否则卡片会骗人。
		zero := 0
		for _, v := range vals {
			if v < 1 {
				zero++
			}
		}
		stallRatio := float64(zero) / float64(len(vals))
		if stallRatio > 0.3 {
			res.Stalled = true
			res.Evidence = append(res.Evidence, fmt.Sprintf("约 %.0f%% 的采样窗吞吐接近 0，下载中途停滞", stallRatio*100))
		}
		if len(vals) >= 6 {
			k := len(vals) / 3
			head := Mean(vals[:k])
			tail := Mean(vals[len(vals)-k:])
			if head > 0 {
				drop := (head - tail) / head * 100
				res.Evidence = append(res.Evidence, fmt.Sprintf("速度衰减：前段 %.1f Mbps → 后段 %.1f Mbps（%.0f%%）", head, tail, drop))
				if drop > 40 {
					res.Evidence = append(res.Evidence, "后段明显掉速，疑似被限速或链路拥塞")
				}
			}
		}
	}

	res.Status = StatusOK
	if res.Bytes < 512*1024 {
		res.Status = StatusDegraded
		res.Evidence = append(res.Evidence, "有效下载字节过少，吞吐结论不可靠")
	}
	// 中途停滞直接判降级：9.3 MB / 20 s 的均值 3.8 Mbps 会让人以为"720p 够用"，
	// 实际是前 5 秒跑满、后 15 秒零吞吐——用户点开视频只会一直转圈。
	if res.Stalled {
		res.Status = StatusDegraded
	}
	if res.CV > 0.5 && res.Bytes >= 1<<20 {
		res.Evidence = append(res.Evidence, fmt.Sprintf("速度波动较大（CV %.2f）", res.CV))
	}
	res.Headline = fmt.Sprintf("%.1f Mbps", res.Mbps)
	if res.Stalled {
		res.Headline = fmt.Sprintf("%.1f Mbps · 中途停滞", res.Mbps)
	}
	// subline 里的并发数用实测连接数，和声明的并发数一并说出，避免"标 6 并发实际 1 条"
	if connN > 0 && connN != conc {
		res.Subline = fmt.Sprintf("%s · %.1f MB · %d 并发/%d 连接", res.Subline, float64(res.Bytes)/(1<<20), conc, connN)
		res.Evidence = append(res.Evidence, fmt.Sprintf("实际独立 TCP 连接 %d 条（声明并发 %d）", connN, conc))
	} else if connN > 0 {
		res.Subline = fmt.Sprintf("%s · %.1f MB · %d 并发/%d 连接", res.Subline, float64(res.Bytes)/(1<<20), conc, connN)
	} else {
		res.Subline = fmt.Sprintf("%s · %.1f MB · %d 并发", res.Subline, float64(res.Bytes)/(1<<20), conc)
	}
	// 单项分必须由实测吞吐给出：漏赋值会让报告里出现"33.5 Mbps / 0 分"的自相矛盾
	res.Score = SpeedScore(res.Mbps)
	// 停滞时均值不可信：按"能连续拉流"的能力封顶，并在证据里说明封顶依据。
	if res.Stalled && res.Score > 40 {
		res.Score = 40
		res.Evidence = append(res.Evidence, "中途停滞，速度分封顶 40（均值不能代表可持续吞吐）")
	}
	return res
}

func containsQ(u string) bool {
	for _, c := range u {
		if c == '?' {
			return true
		}
	}
	return false
}

func maxOf(v []float64) float64 {
	m := 0.0
	for _, x := range v {
		if x > m {
			m = x
		}
	}
	return m
}

func errText(f *FetchResult) string {
	if f == nil {
		return "无响应"
	}
	if f.Err != nil {
		return f.Err.Error()
	}
	return fmt.Sprintf("HTTP %d", f.Status)
}
