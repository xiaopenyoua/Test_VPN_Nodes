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
		return &State{Env: Env(), Exit: netx.Settings{Warnings: []string{}}, Results: []*probe.Result{}}
	}
	cp := *e.state
	cp.Results = append([]*probe.Result{}, e.state.Results...)
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
