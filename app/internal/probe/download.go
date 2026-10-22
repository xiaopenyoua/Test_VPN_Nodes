package probe

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
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

// dlWindow 是长跑模式的分窗长度。5 秒是「够长到能看清趋势、够短到能定位断点」的折中：
// 30 分钟 = 360 个窗口，曲线不至于稀疏；而"几十秒后就不行"这种断流会落在某个具体的窗口上，
// 而不是被均值抹平。
//
// 做成包级变量（而不是常量）只有一个理由：测试。用 5 秒窗口验证"连续两个空窗判停滞"
// 至少要等 10 秒真实时间，一个用例就吃掉 CI 十几秒；测试里压到 300ms 才能真正跑得动。
var dlWindow = 5 * time.Second

// dlStallWindows 是判「中途断流」所需的连续空窗数。
// 为什么是 2 而不是 1：单个空窗很可能只是 TCP 重传 / 拥塞窗口收缩造成的短暂停顿，
// 一次就判死会把「偶发抖一下」的节点误报成断流；连续两窗（默认 10 秒）一个字节都收不到，
// 才是用户口中的"下着下着就不动了"。
const dlStallWindows = 2

// dlMaxConsecFail 是连续失败多少次才判 StatusFailed。
// 断流重连是长跑的常态（代理节点切换、TLS 会话被掐），单次失败就判死不成立；
// 但也不能无限重试——真的断网时会让探测一直挂着空转到 30 分钟结束，
// 用户白等半小时才看到"失败"。
const dlMaxConsecFail = 5

// DownloadOpts 是 P1 的入参。
//
// 用结构体而不是继续加位置参数：P1 的参数已经到 6 个（ctx/tr/并发/流量上限/时长/采样回调），
// 再加 longRun、window 就只能靠"第 7 个 bool 是什么"来读代码，调用点也容易传错顺序。
// 结构体字段带名字，调用点自解释。
type DownloadOpts struct {
	Concurrency int
	BudgetBytes int64
	Duration    time.Duration
	// LongRun 打开长时稳定性分析：分窗采样、断流定位、有效吞吐占比。
	// 为 false 时执行路径与历史版本完全一致（短测不该因为长跑改造而改变结论）。
	LongRun  bool
	OnSample func(Sample)
}

// dlWin 是一个采样窗口。
type dlWin struct {
	t0    float64 // 窗口起点，相对开始时刻的毫秒
	bytes int64   // 本窗口新增字节（不是累计值：判空窗必须看增量）
	mbps  float64
}

// RunDownload 执行 P1：多并发 Range 流式下载，数据全部丢进 io.Discard，不落盘。
func RunDownload(ctx context.Context, tr *http.Transport, o DownloadOpts) *Result {
	conc := o.Concurrency
	budgetBytes := o.BudgetBytes
	dur := o.Duration
	onSample := o.OnSample
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
		// flaps 是"断流重连"次数：单次 Range 请求被掐断后成功重连的次数。
		// 这个数字要被写进证据——用户说"下几十秒就不行了"，
		// 而"期间重连了 4 次"能直接说明是链路在反复断，不是"节点就是慢"。
		flaps atomic.Int64
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

	// ── 长跑分窗采样 ─────────────────────────────────────────────────
	// 250ms 的瞬时点适合画曲线，但不能直接用来判「这段时间到底有没有数据」：
	// 30 分钟 = 7200 个点，任何一个点是 0 都可能只是抖动/重传。
	// 按 dlWindow 聚合成窗口后，"连续两窗一个字节都没进来"才是可解释的断流证据。
	var (
		wins      []dlWin
		wMu       sync.Mutex
		winDone   = make(chan struct{})
		budgetHit atomic.Bool
		fatal     atomic.Bool // 连续失败达上限，主动收工
	)
	if o.LongRun {
		go func() {
			defer close(winDone)
			ticker := time.NewTicker(dlWindow)
			defer ticker.Stop()
			lastBytes := int64(0)
			lastAt := started
			record := func(now time.Time) {
				cur := total.Load()
				delta := cur - lastBytes
				dt := now.Sub(lastAt).Seconds()
				mbps := 0.0
				if dt > 0 {
					mbps = float64(delta) * 8 / 1e6 / dt
				}
				wMu.Lock()
				wins = append(wins, dlWin{t0: float64(lastAt.Sub(started).Milliseconds()), bytes: delta, mbps: Round(mbps, 2)})
				wMu.Unlock()
				lastBytes, lastAt = cur, now
			}
			// 收尾时补一个不满整窗的尾窗：真实跑到一半被预算/取消截断时，
			// 最后这几秒的字节不该凭空消失（否则「窗口字节之和」与「总字节」对不上，
			// 用户会怀疑数据是编的）。但只补跨度不小于半窗的尾窗——
			// 1 毫秒的尾巴算出来的速率是噪声，会把中位数带偏。
			defer func() {
				if now := time.Now(); now.Sub(lastAt).Seconds() >= dlWindow.Seconds()/2 {
					record(now)
				}
			}()
			for {
				select {
				case <-stop:
					return
				case now := <-ticker.C:
					record(now)
				}
			}
		}()
	} else {
		// 短测不启分窗：多一个 goroutine 与多一套统计只会让短测结论有机会漂移，
		// 而短测的结论口径必须与改造前逐字一致。
		close(winDone)
	}

	deadline := time.Now().Add(dur)
	for i := 0; i < conc; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// consecFail 只统计"连续"失败：中间只要有一条请求真的下到字节就清零。
			// 断流重连是长跑的常态，单次失败就判死会把正常节点误报成不可用。
			consecFail := 0
			for {
				if time.Now().After(deadline) {
					return
				}
				if total.Load() >= budgetBytes {
					budgetHit.Store(true)
					return
				}
				if fatal.Load() {
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
					// 连不上也是"断"的一种：用户视角就是这里断了、然后重连，
					// 所以同样计入 flap，也参与"连续 5 次才判死"的计数。
					flaps.Add(1)
					consecFail++
					// 连续失败达上限就收工（仅长跑）：真的断网时不该让用户
					// 干等 30 分钟才看到失败。
					if o.LongRun && consecFail >= dlMaxConsecFail {
						fatal.Store(true)
						return
					}
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
				gotBytes := int64(0)
				var rerr error
				buf := make([]byte, 32<<10)
				for {
					if time.Now().After(deadline) {
						break
					}
					if total.Load() >= budgetBytes {
						budgetHit.Store(true)
						break
					}
					var n int
					n, rerr = resp.Body.Read(buf)
					if n > 0 {
						total.Add(int64(n))
						gotBytes += int64(n)
					}
					if rerr != nil {
						break
					}
				}
				_ = resp.Body.Close()
				cancel()
				// 请求收尾的三种形态必须分开记账，否则"断流"和"正常读完"会混成一个数字：
				//  1) 出了字节 → 这条流是活的，连续失败计数清零；
				//  2) 一个字节都没出，或 read 返回非 EOF 的错误（reset / unexpected EOF /
				//     被中途掐断的短读）→ 记一次断流，立刻重连；
				//  3) 出了字节且以 EOF 收尾（含 CDN 按更小区间满足 Range）→ 正常完成，不算断流。
				// 第 3 条特意不把"短读"算成断流：服务端返回比请求区间更小的 206 是合法实现，
				// 全算断流会让每个健康节点都背上一堆假"重连"，反而把真断流淹掉。
				if gotBytes > 0 {
					consecFail = 0
				}
				if gotBytes == 0 || (rerr != nil && rerr != io.EOF && reqCtx.Err() == nil) {
					flaps.Add(1)
					consecFail++
					if rerr != nil {
						lastErr.Store(rerr.Error())
					}
					if o.LongRun && consecFail >= dlMaxConsecFail {
						fatal.Store(true)
						return
					}
				}
			}
		}(i)
	}

	wg.Wait()
	// 顺序不能反：先等 worker 全部退出（此时 total 已是终值），
	// 再关 stop 让采样器做最后一次收尾统计，否则尾窗会漏掉最后一段字节。
	close(stop)
	<-samplerDone
	<-winDone

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

	wMu.Lock()
	winCopy := append([]dlWin(nil), wins...)
	wMu.Unlock()

	// 长跑走独立的结论分支：它要的指标（断流时刻 / 有效吞吐占比）在短测口径里
	// 根本不存在，硬塞进下面这段会把短测的结论一起改掉。
	if o.LongRun {
		return concludeLongRun(res, longRunInput{
			conc:        conc,
			connN:       connN,
			wins:        winCopy,
			elapsed:     time.Since(started),
			planned:     dur,
			budgetBytes: budgetBytes,
			budgetHit:   budgetHit.Load(),
			flaps:       flaps.Load(),
			fatal:       fatal.Load(),
			cancelled:   ctx.Err() != nil,
		})
	}

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

// longRunInput 是长跑结论所需的全部实测输入（刻意不传 res 之外的活状态，
// 结论函数只做纯计算，便于测试单独喂合成数据验证口径）。
type longRunInput struct {
	conc        int
	connN       int
	wins        []dlWin
	elapsed     time.Duration
	planned     time.Duration
	budgetBytes int64
	budgetHit   bool
	flaps       int64
	fatal       bool
	cancelled   bool
}

// concludeLongRun 产出长跑的结论。
//
// 这一支存在的唯一目的：把「跑了 N 秒之后就不出数据了」这件事变成一个显式指标，
// 而不是继续被 30 分钟的均值摊平成「还行」。用户原话是
// 「好多都是只能下载一会儿、几十秒，就不行了」——短测的均值对这类节点毫无分辨力。
func concludeLongRun(res *Result, in longRunInput) *Result {
	wins := in.wins
	n := len(wins)

	// 窗口序列换成前端要的样本序列：Bytes 用累计值，与短测 250ms 采样同一语义，
	// 前端画曲线不必区分两套口径。长跑 30 分钟只有 360 个点（短测是 7200 个），
	// 报告体积也跟着降下来了。
	series := make([]Sample, 0, n)
	cum := int64(0)
	for _, w := range wins {
		cum += w.bytes
		series = append(series, Sample{T: w.t0, Mbps: w.mbps, Bytes: cum})
	}
	res.Samples = series

	all := make([]float64, 0, n)
	nonEmpty := make([]float64, 0, n)
	zero := 0
	for _, w := range wins {
		all = append(all, w.mbps)
		if w.bytes == 0 {
			zero++
			continue
		}
		nonEmpty = append(nonEmpty, w.mbps)
	}

	// ── 吞吐口径：只用非空窗 ──────────────────────────────────────────
	// 空窗代表"这段时间一个字节都没进来"。把它算进吞吐均值，等于让断流时间
	// 去做均值的除法：一条能跑 10 Mbps 的流断掉一半，均值会显示 5 Mbps，
	// 用户会以为"节点只是慢一点"，而事实是"有一半时间根本没有数据"。
	// 断流占用的时间由 SustainPct 单独表达——两个数字各说一件事，谁也不掩盖谁。
	// 代价是 Mbps 会高于"总字节/总时长"，所以必须把口径写进证据（见下）。
	if len(nonEmpty) > 0 {
		res.Mbps = Round(Mean(nonEmpty), 2)
		res.P50Mbps = Round(Percentile(nonEmpty, 50), 2)
		res.P90Mbps = Round(Percentile(nonEmpty, 90), 2)
		res.PeakMbps = Round(maxOf(nonEmpty), 2)
		if mean := Mean(nonEmpty); mean > 0 {
			res.CV = Round(StdDev(nonEmpty)/mean, 3)
		}
	}
	if n > 0 {
		res.Evidence = append(res.Evidence, fmt.Sprintf(
			"口径：吞吐指标（均值/P50/P90/峰值）只用有数据的 %d 个窗口计算，空窗 %d 个不进均值；有效时长占比另由「有效吞吐」一项表达",
			len(nonEmpty), zero))
	}

	// ── 断流定位：连续空窗 ────────────────────────────────────────────
	// 用"连续"而不是"累计比例"：长跑里偶发的单窗空档（TCP 重传、拥塞窗口收缩）
	// 是正常现象，累计比例会把它们攒成一个假断流；
	// 而连续两整窗（默认 10 秒）一个字节都收不到，就是用户说的"卡住不动了"。
	streak := 0
	for i, w := range wins {
		if w.bytes > 0 {
			streak = 0
			continue
		}
		streak++
		if streak == dlStallWindows && !res.Stalled {
			res.Stalled = true
			// 取这一串空窗的**第一个**作为断流时刻：用户问的是"什么时候开始不行的"，
			// 不是"确认它不行了是什么时候"。
			res.StallAtMs = wins[i-dlStallWindows+1].t0
		}
	}

	// ── 有效吞吐占比 ──────────────────────────────────────────────────
	// 基准取全部窗口中位数（含空窗）：中位数代表"这条链路典型的一窗是多少"，
	// 不以峰值为基准是因为峰值往往只是启动瞬间的突发。
	// 达到基准 20% 才算"这段时间在有效出数据"，低于 20% 视为没在干活。
	if n == 0 {
		res.SustainPct = 0
	} else if med := Percentile(all, 50); med <= 0 {
		// 中位数为 0 说明过半窗口根本没数据。此时"低于基准 20%"这个判据塌了
		// （任何窗口都 >= 0），naive 实现会把彻底断流的节点算成 100% 有效，
		// 正好把最该报警的情形洗白。直接给 0。
		res.SustainPct = 0
	} else {
		good := 0
		for _, v := range all {
			if v >= med*0.2 {
				good++
			}
		}
		res.SustainPct = Round(float64(good)/float64(n)*100, 0)
	}

	// ── 证据 ──────────────────────────────────────────────────────────
	res.Evidence = append(res.Evidence, fmt.Sprintf("长跑 %s，按 %s 切成 %d 个窗口（%d 个窗口无数据）",
		formatDurZh(float64(in.elapsed.Milliseconds())), formatDurZh(float64(dlWindow.Milliseconds())), n, zero))
	if in.flaps > 0 {
		res.Evidence = append(res.Evidence, fmt.Sprintf("期间断流重连 %d 次（连续失败 %d 次才判失败，本次%s）",
			in.flaps, dlMaxConsecFail, condText(in.fatal, fmt.Sprintf("已达上限，提前收工于 %s", formatDurZh(float64(in.elapsed.Milliseconds()))), "未达上限")))
	}
	if res.Stalled {
		res.Evidence = append(res.Evidence, fmt.Sprintf("首次断流发生在第 %s（该时刻起连续 %d 个窗口零字节）",
			formatDurZh(res.StallAtMs), dlStallWindows))
	}
	if res.SustainPct > 0 {
		res.Evidence = append(res.Evidence, fmt.Sprintf("有效吞吐占 %s 时长（基准：窗口中位数 %.2f Mbps 的 20%%）",
			fmt.Sprintf("%.0f%%", res.SustainPct), Percentile(all, 50)))
	}

	// ── 有没有真的跑满 ────────────────────────────────────────────────
	// 这一条不许含糊：预算/断网导致提前收工时，如果卡片还写着"30 分钟长跑"，
	// 用户会以为"30 分钟都很稳"，而其实只测了 2 分钟。
	short := in.planned - in.elapsed
	if in.budgetHit {
		msg := fmt.Sprintf("触及流量上限 %d MB，提前结束（实际跑了 %s，预定 %s）",
			in.budgetBytes>>20, formatDurZh(float64(in.elapsed.Milliseconds())), formatDurZh(float64(in.planned.Milliseconds())))
		res.Evidence = append(res.Evidence, msg)
		res.Subline = msg
	} else if !in.cancelled && short > 500*time.Millisecond {
		res.Evidence = append(res.Evidence, fmt.Sprintf("未跑满预定 %s，实际 %s 结束（%s）",
			formatDurZh(float64(in.planned.Milliseconds())), formatDurZh(float64(in.elapsed.Milliseconds())),
			condText(in.fatal, "连续失败达上限，判定链路已断", "提前收工")))
	}

	// ── 结论措辞 ──────────────────────────────────────────────────────
	base := fmt.Sprintf("%s · %.1f MB · %d 并发", res.Subline, float64(res.Bytes)/(1<<20), in.conc)
	if in.connN > 0 && in.connN != in.conc {
		base = fmt.Sprintf("%s · %.1f MB · %d 并发/%d 连接", res.Subline, float64(res.Bytes)/(1<<20), in.conc, in.connN)
		res.Evidence = append(res.Evidence, fmt.Sprintf("实际独立 TCP 连接 %d 条（声明并发 %d）", in.connN, in.conc))
	}

	switch {
	case in.cancelled:
		// 取消时不给结论：半程数据算出来的分数会把"用户自己按了取消"
		// 渲染成"这个节点不行"。这与 engine 层对取消的处理口径一致。
		res.Status = StatusCancelled
		res.Headline = "已取消"
		res.Subline = fmt.Sprintf("%s · 取消前跑了 %s", base, formatDurZh(float64(in.elapsed.Milliseconds())))
		res.Score = 0
		return res

	case res.Stalled:
		res.Status = StatusDegraded
		res.Headline = "中途断流"
		detail := fmt.Sprintf("跑到 %s 后不再出数据", formatDurZh(res.StallAtMs))
		if in.flaps > 0 {
			detail += fmt.Sprintf("（期间重连 %d 次）", in.flaps)
		}
		res.Subline = base + " · " + detail
		res.NextStep = "断流后不再恢复：换个节点重测本项；若同一节点每次都在相近时刻断，多为服务端按流量/时长掐连接或代理侧空闲超时"
		res.Score = SpeedScore(res.Mbps)
		// 断流时均值不可信：按"能不能连续拉流"封顶，并说明封顶依据。
		if res.Score > 40 {
			res.Score = 40
			res.Evidence = append(res.Evidence, "中途断流，速度分封顶 40（有数据的时段再快也不能连续使用）")
		}
		return res

	case res.SustainPct < 70:
		res.Status = StatusDegraded
		res.Headline = "中途失速"
		res.Subline = base + fmt.Sprintf(" · 有效吞吐只占 %.0f%% 时长", res.SustainPct)
		res.NextStep = "没断流但大半时间没有吞吐：多为代理限速/时段拥塞，可换节点或换时段复测本项"
		res.Evidence = append(res.Evidence, "有效吞吐不足 70% 时长，判为降级（能连上、但持续性不够）")

	case res.Bytes < 512*1024:
		// 窗口再多也没用：总量太小说明一开始就没下起来。
		res.Status = StatusDegraded
		res.Headline = fmt.Sprintf("%.1f Mbps", res.Mbps)
		res.Subline = base + " · 有效下载字节过少，结论不可靠"
		res.Score = SpeedScore(res.Mbps * res.SustainPct / 100)
		res.Evidence = append(res.Evidence, "有效下载字节过少，吞吐结论不可靠")
		return res

	default:
		res.Status = StatusOK
		res.Headline = fmt.Sprintf("%.1f Mbps", res.Mbps)
		res.Subline = base + fmt.Sprintf(" · 有效吞吐 %.0f%% 时长", res.SustainPct)
	}
	// 分数按有效时长折算：62% 的时间在出数据就只能拿 62% 的速度分。
	// 否则"中途失速 / 降级"的卡片配 85 分，看的人不知道该信哪个。
	res.Score = Round(SpeedScore(res.Mbps)*res.SustainPct/100, 0)
	return res
}

// formatDurZh 把毫秒格式化成中文时长，用于"跑到 3 分 12 秒后不再出数据"这类句子。
func formatDurZh(ms float64) string {
	if ms < 0 {
		ms = 0
	}
	total := int(ms / 1000)
	switch {
	case total < 60:
		return fmt.Sprintf("%d 秒", total)
	case total < 3600:
		return fmt.Sprintf("%d 分 %d 秒", total/60, total%60)
	default:
		return fmt.Sprintf("%d 小时 %d 分", total/3600, (total%3600)/60)
	}
}

func condText(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}
