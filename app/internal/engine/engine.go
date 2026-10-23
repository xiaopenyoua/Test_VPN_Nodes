// Package engine 负责编排一次探测运行，并把过程事件流式广播出去。
package engine

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"proxylens/internal/config"
	"proxylens/internal/netx"
	"proxylens/internal/probe"
	"proxylens/internal/score"
)

// Version 是应用版本号，随报告一起输出。
const Version = "1.0.0"

// Event 是 SSE 推送的统一信封。
type Event struct {
	Seq     int64       `json:"seq"`
	Type    string      `json:"type"`
	At      int64       `json:"at"`
	Payload interface{} `json:"payload"`
}

// EnvInfo 是随报告记录的环境快照。
type EnvInfo struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Version   string `json:"version"`
	GoVersion string `json:"goVersion"`
}

// State 是一次运行的完整状态。
type State struct {
	RunID     string          `json:"runId"`
	Running   bool            `json:"running"`
	StartedAt string          `json:"startedAt"`
	EndedAt   string          `json:"endedAt"`
	Config    config.Config   `json:"config"`
	Exit      netx.Settings   `json:"exit"`
	Env       EnvInfo         `json:"env"`
	Results   []*probe.Result `json:"results"`
	Summary   *score.Summary  `json:"summary"`
	Error     string          `json:"error"`
	ElapsedMs float64         `json:"elapsedMs"`
	// Only 表示当前（或最近一次）运行是单项复测：结果只替换对应那一项，
	// 总评保持上一次完整运行的值。
	Only bool `json:"only"`

	// Busy 表示"此刻有测试在跑"——综合测试或带宽长跑，二者共用同一把独占锁。
	// Running 只代表综合测试，带宽长跑不写它：那一档跑两小时，
	// 若把它也算成 Running，总评页的顶栏会连着两小时停在"探测中…"。
	Busy bool `json:"busy"`

	// Bandwidth 是独立带宽长跑自己的状态位，与上面的 Results / Summary 完全隔离。
	// 单独占一位的理由：两小时的长跑结果一旦写回六项的 P1，
	// "综合测试测出来的 P1" 就被覆盖掉了，而两者根本不是同一个观测。
	Bandwidth *BandwidthRun `json:"bandwidth"`
}

// BandwidthRun 是一次独立带宽长跑的状态与结果。
type BandwidthRun struct {
	RunID     string        `json:"runId"`
	Running   bool          `json:"running"`
	StartedAt string        `json:"startedAt"`
	EndedAt   string        `json:"endedAt"`
	Config    config.Config `json:"config"`
	Exit      netx.Settings `json:"exit"`
	Result    *probe.Result `json:"result"`
	Error     string        `json:"error"`
	ElapsedMs float64       `json:"elapsedMs"`

	// Samples 是跑动过程中的采样序列。
	//
	// SSE 只推"从订阅那一刻起"的增量，所以中途才打开页面的人会看到一张空图——
	// 明明已经下了 600 MB，界面上却像刚起步。把序列一起挂在快照里，
	// 新打开的页面就能把已经跑过的那段补齐。
	Samples []probe.Sample `json:"samples"`
}

// Engine 持有运行状态与订阅者。
type Engine struct {
	mu      sync.Mutex
	state   *State
	cancel  context.CancelFunc
	seq     int64
	subs    map[chan Event]struct{}
	running bool
}

// New 创建引擎。
func New() *Engine {
	return &Engine{subs: map[chan Event]struct{}{}}
}

// Env 返回当前环境快照。
func Env() EnvInfo {
	return EnvInfo{OS: runtime.GOOS, Arch: runtime.GOARCH, Version: Version, GoVersion: runtime.Version()}
}

// Subscribe 注册事件订阅，返回通道与取消函数。
func (e *Engine) Subscribe() (chan Event, func()) {
	ch := make(chan Event, 256)
	e.mu.Lock()
	e.subs[ch] = struct{}{}
	e.mu.Unlock()
	return ch, func() {
		e.mu.Lock()
		if _, ok := e.subs[ch]; ok {
			delete(e.subs, ch)
			close(ch)
		}
		e.mu.Unlock()
	}
}

func (e *Engine) broadcast(typ string, payload interface{}) {
	e.mu.Lock()
	e.seq++
	ev := Event{Seq: e.seq, Type: typ, At: time.Now().UnixMilli(), Payload: payload}
	targets := make([]chan Event, 0, len(e.subs))
	for ch := range e.subs {
		targets = append(targets, ch)
	}
	e.mu.Unlock()
	for _, ch := range targets {
		select {
		case ch <- ev:
		default: // 慢消费者直接丢弃，绝不阻塞探测
		}
	}
}

// Snapshot 返回当前状态副本。
func (e *Engine) Snapshot() *State {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state == nil {
		return &State{Env: Env(), Exit: netx.Settings{Warnings: []string{}}, Results: []*probe.Result{}, Busy: e.running}
	}
	cp := *e.state
	cp.Results = append([]*probe.Result{}, e.state.Results...)
	cp.Busy = e.running
	// 带宽长跑的结果按值拷一份出去：直接共享指针的话，
	// 前端每次轮询快照都会和正在写入的 goroutine 抢同一块内存。
	if e.state.Bandwidth != nil {
		b := *e.state.Bandwidth
		b.Samples = append([]probe.Sample(nil), e.state.Bandwidth.Samples...)
		cp.Bandwidth = &b
	}
	return &cp
}

// Running 报告是否正在测试。
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// Cancel 取消当前运行。
func (e *Engine) Cancel() bool {
	e.mu.Lock()
	c := e.cancel
	running := e.running
	e.mu.Unlock()
	if running && c != nil {
		c()
		return true
	}
	return false
}

// Start 启动一次运行。
func (e *Engine) Start(cfg config.Config) (string, error) {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	// 带宽长跑走自己的入口：它不跑六项里的任何一项，也不参与总评。
	if cfg.Bandwidth {
		return e.startBandwidth(cfg)
	}
	if cfg.Only && len(cfg.Probes) != 1 {
		return "", errors.New("单项复测必须且只能指定一个探测项")
	}
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return "", errors.New("已有测试正在进行，请先取消")
	}
	exit, err := netx.Resolve(cfg.Mode, cfg.ProxyURL)
	if err != nil {
		e.mu.Unlock()
		return "", err
	}
	ctx, cancel := context.WithCancel(context.Background())
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	st := &State{
		RunID:     runID,
		Running:   true,
		StartedAt: time.Now().Format(time.RFC3339),
		Config:    cfg,
		Exit:      exit,
		Env:       Env(),
		Results:   []*probe.Result{},
		Only:      cfg.Only,
	}
	// 单项复测：沿用上一轮的其它结果与总评，只替换被复测的那一项。
	if cfg.Only && e.state != nil {
		st.Results = append([]*probe.Result{}, e.state.Results...)
		st.Summary = e.state.Summary
	}
	e.state = st
	e.cancel = cancel
	e.running = true
	e.mu.Unlock()

	go e.execute(ctx, st)
	return runID, nil
}

func (e *Engine) execute(ctx context.Context, st *State) {
	started := time.Now()
	defer func() {
		e.mu.Lock()
		e.running = false
		st.Running = false
		st.ElapsedMs = probe.Round(float64(time.Since(started).Microseconds())/1000, 0)
		st.EndedAt = time.Now().Format(time.RFC3339)
		e.mu.Unlock()
	}()

	only := st.Config.Only
	e.broadcast("run_start", map[string]interface{}{
		"runId":  st.RunID,
		"config": st.Config,
		"exit":   st.Exit,
		"env":    st.Env,
		"only":   only,
	})

	results := make([]*probe.Result, 0, len(st.Config.Probes))
	if only {
		results = append(results, st.Results...)
	}
	var resMu sync.Mutex

	// 单项复测时替换同 ID 的旧结果，而不是追加——否则其它 5 项会被抹掉。
	replaceOrAppend := func(list []*probe.Result, r *probe.Result) []*probe.Result {
		if only {
			for i, old := range list {
				if old.ID == r.ID {
					list[i] = r
					return list
				}
			}
		}
		return append(list, r)
	}

	runOne := func(id string) {
		r := e.runProbe(ctx, st.Exit, id, st.Config)
		r.Only = only
		resMu.Lock()
		results = replaceOrAppend(results, r)
		resMu.Unlock()
		e.mu.Lock()
		st.Results = replaceOrAppend(st.Results, r)
		e.mu.Unlock()
		e.broadcast("probe_result", r)
	}

	// 第一组：AI 可达性 + 基线 + YouTube 链路，彼此干扰小，并行跑
	light := []string{}
	hasDownload := false
	for _, id := range st.Config.Probes {
		switch id {
		case config.ProbeGoogle:
			hasDownload = true
		case config.ProbeYouTube:
			light = append(light, id)
		default:
			light = append(light, id)
		}
	}

	var wg sync.WaitGroup
	for _, id := range light {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			runOne(id)
		}(id)
	}
	wg.Wait()

	// 第二组：大文件下载独占带宽，必须单独跑，否则吞吐数据失真
	if hasDownload && ctx.Err() == nil {
		runOne(config.ProbeGoogle)
	}

	// 未执行的探测项补位，让前端拿到完整卡片
	for _, id := range st.Config.Probes {
		found := false
		for _, r := range results {
			if r.ID == id {
				found = true
				break
			}
		}
		if !found {
			m := probe.MetaByID(id)
			status := probe.StatusSkipped
			headline := "未执行"
			if ctx.Err() != nil {
				status = probe.StatusCancelled
				headline = "已取消"
			}
			r := &probe.Result{ID: id, Name: m.Name, Group: m.Group, Status: status, Headline: headline, Samples: []probe.Sample{}}
			resMu.Lock()
			results = append(results, r)
			resMu.Unlock()
			e.mu.Lock()
			st.Results = append(st.Results, r)
			e.mu.Unlock()
			e.broadcast("probe_result", r)
		}
	}

	// 排序输出，保证顺序稳定
	ordered := make([]*probe.Result, 0, len(results))
	for _, m := range probe.Metas {
		for _, r := range results {
			if r.ID == m.ID {
				ordered = append(ordered, r)
				break
			}
		}
	}

	// 取消时绝不产出结论：半程数据算出来的分数会把"用户自己按了取消"
	// 渲染成"这套代理不可用"，这是最伤可信度的一类误判。
	e.mu.Lock()
	st.Results = ordered
	switch {
	case only:
		// 单项复测不重算总评：总评代表一次完整运行，
		// 把不同时间窗口的结果混算在一起无法归因。
		e.mu.Unlock()
	case ctx.Err() != nil:
		st.Summary = nil
		st.Error = ""
		e.mu.Unlock()
	case score.MeasurementInvalid(ordered):
		// 所有端点都卡在连接层，一个 HTTP 响应都没拿到。
		// 这时给 0 分 / E·不可用会误导：用户重测一次可能又是 C·可用。
		// 不评分 + 说清原因，比编一个分数诚实。
		st.Summary = nil
		st.Error = "本轮所有探测项都没能建立可用的连接，无法区分是代理失效还是本地网络中断，因此不给出评分。建议确认本地网络后重试一次。"
		e.mu.Unlock()
	default:
		summary := score.Aggregate(ordered)
		st.Summary = &summary
		e.mu.Unlock()
		e.broadcast("score", summary)
	}

	status := "done"
	if ctx.Err() != nil {
		status = "cancelled"
	}
	e.broadcast("run_end", map[string]interface{}{
		"runId":     st.RunID,
		"status":    status,
		"elapsedMs": probe.Round(float64(time.Since(started).Microseconds())/1000, 0),
		"only":      only,
	})
}

func (e *Engine) runProbe(ctx context.Context, exit netx.Settings, id string, cfg config.Config) *probe.Result {
	m := probe.MetaByID(id)
	e.broadcast("probe_start", map[string]interface{}{"id": id, "name": m.Name, "group": m.Group})

	tr, err := netx.NewTransport(exit)
	if err != nil {
		return &probe.Result{ID: id, Name: m.Name, Group: m.Group, Status: probe.StatusFailed, Headline: "无法建立连接", Error: err.Error(), Samples: []probe.Sample{}}
	}
	defer tr.CloseIdleConnections()

	pctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSec)*time.Second)
	defer cancel()

	switch id {
	case config.ProbeChatGPT, config.ProbeGemini, config.ProbeOpenAI:
		return probe.RunSite(pctx, tr, id)
	case config.ProbeBase:
		return probe.RunBaseline(pctx, tr, 6)
	case config.ProbeYouTube:
		return probe.RunYouTube(pctx, tr, 2<<20)
	case config.ProbeGoogle:
		dur := time.Duration(cfg.DurationSec) * time.Second
		// 下载需要更长的独立预算，不受单 probe 超时限制。
		// 这里的 ctx 直接挂在根 ctx 上（不是那个被 TimeoutSec 卡住的 pctx），
		// 所以「只跑 P1 + longRun」时，跑满 1800 秒不会被别的探测项的超时预算打断。
		// 额外 +20s 是留给最后一个 Range 请求收尾的余量，不是固定值——
		// 若写死 20s，长跑会被自己的外层 ctx 在 20 秒时整条掐掉。
		dctx, dcancel := context.WithTimeout(ctx, dur+20*time.Second)
		defer dcancel()
		return probe.RunDownload(dctx, tr, probe.DownloadOpts{
			Concurrency: cfg.Concurrency,
			BudgetBytes: int64(cfg.BudgetMB) << 20,
			Duration:    dur,
			// 长跑开关必须一路传到探针：漏传的话配置里勾了「长时稳定性」，
			// 探测仍按短测口径出结论，用户等于没勾。
			LongRun: cfg.LongRun,
			OnSample: func(s probe.Sample) {
				e.broadcast("sample", map[string]interface{}{"id": config.ProbeGoogle, "sample": s})
			},
		})
	default:
		return &probe.Result{ID: id, Name: m.Name, Group: m.Group, Status: probe.StatusSkipped, Headline: "未知探测项", Samples: []probe.Sample{}}
	}
}

// ── 独立带宽长跑 ────────────────────────────────────────────────────

// bwSampleCap 是快照里保留的采样点上限（两两合并前的阈值）。
// 长跑报上来的是 5 秒窗口而不是 250ms 瞬时点：4 小时也只有 2,880 个，
// 本来就在上限之内，合并不再触发——这是把口径统一到窗口后的附带好处，
// 快照体积从几 MB 降到百来 KB，而形状与最终报告逐点一致。
const bwSampleCap = 3600

// recordBandwidthSample 把采样点追加进运行状态，超过上限就两两合并。
//
// 合并而不是截断尾部：截断会丢掉"最开始那一段"，而长跑最想看的恰恰是
// 开头有没有跑满。合并保住整条曲线的形状，只降低时间分辨率。
func (e *Engine) recordBandwidthSample(br *BandwidthRun, s probe.Sample) {
	e.mu.Lock()
	defer e.mu.Unlock()
	br.Samples = append(br.Samples, s)
	if len(br.Samples) < bwSampleCap {
		return
	}
	merged := make([]probe.Sample, 0, len(br.Samples)/2+1)
	for i := 0; i < len(br.Samples); i += 2 {
		a := br.Samples[i]
		b := a
		if i+1 < len(br.Samples) {
			b = br.Samples[i+1]
		}
		merged = append(merged, probe.Sample{
			T:     (a.T + b.T) / 2,
			Mbps:  probe.Round((a.Mbps+b.Mbps)/2, 2),
			Bytes: b.Bytes,
		})
	}
	br.Samples = merged
}

// startBandwidth 启动一次带宽长跑。
//
// 它与综合测试共用同一把独占锁（e.running），但只写 Bandwidth 这一个状态位：
// 两条大流量同时跑，各自的吞吐都会被对方拉低，测出来的数字谁都不算数；
// 而写入位置分开，是为了让"综合测试的 P1"不被两小时的压测覆盖掉。
func (e *Engine) startBandwidth(cfg config.Config) (string, error) {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		// 这里不能只说"已有测试正在进行"：用户真正需要知道的是
		// 两条流量并行会把彼此的吞吐拉低，测出来的数谁都不算数。
		return "", errors.New("已有测试正在进行。带宽长跑需要独占带宽，与其它测试并行会让两边的吞吐都不准，请先取消当前测试")
	}
	exit, err := netx.Resolve(cfg.Mode, cfg.ProxyURL)
	if err != nil {
		e.mu.Unlock()
		return "", err
	}
	ctx, cancel := context.WithCancel(context.Background())
	runID := fmt.Sprintf("bw-%d", time.Now().UnixMilli())
	br := &BandwidthRun{
		RunID:     runID,
		Running:   true,
		StartedAt: time.Now().Format(time.RFC3339),
		Config:    cfg,
		Exit:      exit,
	}
	e.cancel = cancel
	e.running = true
	if e.state == nil {
		e.state = &State{Env: Env(), Exit: netx.Settings{Warnings: []string{}}, Results: []*probe.Result{}}
	}
	e.state.Bandwidth = br
	e.state.Busy = true
	st := e.state
	e.mu.Unlock()

	go e.executeBandwidth(ctx, st, br)
	return runID, nil
}

func (e *Engine) executeBandwidth(ctx context.Context, st *State, br *BandwidthRun) {
	started := time.Now()
	cfg := br.Config

	finish := func(status, errMsg string) {
		elapsed := probe.Round(float64(time.Since(started).Microseconds())/1000, 0)
		e.mu.Lock()
		e.running = false
		if st.Busy {
			st.Busy = false
		}
		br.Running = false
		br.ElapsedMs = elapsed
		br.EndedAt = time.Now().Format(time.RFC3339)
		if errMsg != "" {
			br.Error = errMsg
		}
		e.mu.Unlock()
		e.broadcast("bw_end", map[string]interface{}{
			"runId":     br.RunID,
			"status":    status,
			"elapsedMs": elapsed,
			"error":     br.Error,
		})
	}

	e.broadcast("bw_start", map[string]interface{}{
		"runId":  br.RunID,
		"config": cfg,
		"exit":   br.Exit,
	})

	tr, err := netx.NewTransport(br.Exit)
	if err != nil {
		finish("failed", "无法建立连接："+err.Error())
		return
	}
	defer tr.CloseIdleConnections()

	dur := time.Duration(cfg.DurationSec) * time.Second
	// 外层 ctx 留的余量比综合测试更宽：长跑输在多流收尾上时，
	// 被自己的 ctx 提前掐掉会表现为"最后一个窗口没有数据"，
	// 在图上看起来就像断流。
	dctx, dcancel := context.WithTimeout(ctx, dur+45*time.Second)
	defer dcancel()

	res := probe.RunDownload(dctx, tr, probe.DownloadOpts{
		Concurrency: cfg.Concurrency,
		BudgetBytes: int64(cfg.BudgetMB) << 20,
		Duration:    dur,
		LongRun:     true,
		// 长跑只把**5 秒窗口**报给前端，250ms 的瞬时值不再外发。
		//
		// 这不只是省带宽（4 小时 57,600 条 SSE → 2,880 条），更重要的是口径：
		// 曲线、当前速率、峰值、P50/P90 必须与 concludeLongRun 的结论同源，
		// 否则界面上会出现「全程平均 13.8 / 峰值 73.9」——峰值取的是某一次
		// 代理缓冲区冲刷的量化噪声，而不是链路能持续的速度。
		OnWindow: func(s probe.Sample) {
			e.broadcast("bw_window", map[string]interface{}{"runId": br.RunID, "sample": s})
			e.recordBandwidthSample(br, s)
		},
	})

	// 用户主动取消时绝不产出结论：半程数据算出来的"平均 3 Mbps"
	// 会被读成"这条链路只有 3 Mbps"，而事实上是用户自己按了停止。
	if ctx.Err() != nil {
		res.Status = probe.StatusCancelled
		res.Headline = "已取消"
		res.Score = 0
		e.mu.Lock()
		br.Result = nil
		e.mu.Unlock()
		finish("cancelled", "")
		return
	}

	e.mu.Lock()
	br.Result = res
	e.mu.Unlock()
	e.broadcast("bw_result", map[string]interface{}{"runId": br.RunID, "result": res})

	status := "done"
	if res.Status == probe.StatusFailed {
		status = "failed"
	}
	finish(status, "")
}
