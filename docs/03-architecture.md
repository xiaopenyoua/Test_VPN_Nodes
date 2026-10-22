# 03 · ProxyLens 系统架构与架构决策记录（ADR）

| 项 | 值 |
|---|---|
| 文档版本 | v1.0（定稿） |
| 作者 | Software Architect |
| 状态 | **选型已定稿**，可据此编码 |
| 上游 | `00-task-brief.md`、`02-tech-research.md`（实测依据 `M1`–`M12`） |
| 同层依赖 | `04-security.md`（端点/鉴权/SSRF）、`06-engine-design.md`（算法）、`07-ux-interaction.md`、`08-ui-spec.md` |
| 约束 | 无数据库、无前端构建链、无云服务、无第三方运行时 |

---

## 1. 架构总览

### 1.1 一句话架构

**一个 Go 单进程，内含三个逻辑层：探测引擎（出连 + 归因）、本地 HTTP/SSE 服务（同源 API）、内嵌前端（静态资源，跑在用户浏览器里）。进程即应用，无外部依赖。**

### 1.2 分层图（ASCII）

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                       用户浏览器（Chrome / Edge / Safari）                     │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │  Presentation  （原生 ES Module，零构建，go:embed 内嵌）                 │  │
│  │  ┌──────────┬──────────┬──────────┬──────────┬──────────┬───────────┐  │  │
│  │  │ AppShell │ RunPanel │ ProbeCard│ StatChart│ ScoreRing│ ReportView│  │  │
│  │  └──────────┴──────────┴──────────┴──────────┴──────────┴───────────┘  │  │
│  │  ┌────────────────────┬────────────────────┬─────────────────────────┐  │  │
│  │  │ core/store.js      │ core/api.js (fetch)│ core/events.js (SSE)    │  │  │
│  │  └────────────────────┴────────────────────┴─────────────────────────┘  │  │
│  └────────────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────┬─────────────────────────────────────────────┘
                                 │ HTTP/1.1 同源 127.0.0.1:<随机端口>
                                 │  ├ GET  /?t=<token>        bootstrap
                                 │  ├ GET  /assets/*          静态资源
                                 │  ├ GET  /api/state         脱敏状态
                                 │  ├ POST /api/probe         启动一次 Run
                                 │  ├ POST /api/probe/cancel  取消
                                 │  ├ GET  /api/events        SSE 事件流
                                 │  └ POST /api/export        导出报告
┌────────────────────────────────┴─────────────────────────────────────────────┐
│  Transport  internal/server   （唯一监听点，127.0.0.1 only）                  │
│  ┌───────────┬──────────┬───────────┬──────────┬──────────┬────────────────┐ │
│  │ bootstrap │ auth     │ router    │ sse hub  │ export   │ security hdrs  │ │
│  │ (token)   │ (middle) │           │ (fan-out)│ (json/md)│ (CSP/Host/CSRF)│ │
│  └───────────┴──────────┴───────────┴──────────┴──────────┴────────────────┘ │
└────────────────────────────────┬─────────────────────────────────────────────┘
                                 │ 进程内函数调用（无序列化屏障）
┌────────────────────────────────┴─────────────────────────────────────────────┐
│  Application  internal/runner   （编排，不含业务规则）                        │
│  ┌───────────────┬──────────────────┬─────────────────┬───────────────────┐  │
│  │ RunController │ Scheduler        │ EventBus        │ ReportBuilder     │  │
│  │ (单 Run 互斥) │ (worker pool)    │ (throttle 200ms)│ (score + export)  │  │
│  └───────────────┴──────────────────┴─────────────────┴───────────────────┘  │
└────────────────────────────────┬─────────────────────────────────────────────┘
                                 │
┌────────────────────────────────┴─────────────────────────────────────────────┐
│  Domain  internal/probe + internal/score                                     │
│  ┌──────────────────────────────────────────────────────────────────────┐    │
│  │  Result / Phase / Sample / Status / Score（纯数据 + 纯函数）          │    │
│  │  Percentile · Mean · StdDev · CV · Jitter · DecayRatio · Verdict      │    │
│  │  ★ 不 import netx / server / runner（单一方向）                       │    │
│  └──────────────────────────────────────────────────────────────────────┘    │
└────────────────────────────────┬─────────────────────────────────────────────┘
                                 │ Ports（接口）
┌────────────────────────────────┴─────────────────────────────────────────────┐
│  Infrastructure  internal/netx + probe 采集器                                │
│  ┌────────────────┬────────────────┬─────────────────┬───────────────────┐   │
│  │ RouteResolver  │ TransportFactory│ Collector×6     │ Classifier        │   │
│  │ (系统代理探测)  │ (http.Transport)│ (P1..P6)        │ (状态码/指纹)      │   │
│  └────────────────┴────────────────┴─────────────────┴───────────────────┘   │
└────────────────────────────────┬─────────────────────────────────────────────┘
                                 │
        ┌────────────────────────┼────────────────────────┐
        ▼                        ▼                        ▼
   ┌─────────────┐        ┌──────────────┐         ┌──────────────┐
   │ OS 系统代理  │        │ HTTP 代理     │         │ TUN / 直连    │
   │ scutil/注册表│        │ 127.0.0.1:xx │         │ fake-IP       │
   └─────────────┘        └──────────────┘         └──────────────┘
        │
        ▼  真实互联网：dl.google.com · chatgpt.com · gemini.google.com ·
           api.openai.com · *.googlevideo.com · *.gvt1.com · i.ytimg.com
```

### 1.3 依赖方向（唯一允许的方向）

```
server ──► runner ──► probe(domain)  ◄── score(domain)
   │          │            ▲
   │          └──► netx ───┘   （netx 实现 probe 声明的 Port 接口）
   ▼
  web (embed)

规则 1：箭头只能向下、向领域。
规则 2：probe / score 绝不 import netx、server、runner。
规则 3：netx 可 import probe（实现其接口），反之禁止。
规则 4：server 不得直接调用 netx —— 必须经 runner，防止「controller 直连仓库」的反模式。
```

**如何强制（不靠自觉）**：

```bash
# scripts/lint-deps.sh —— CI 与提交前跑
set -e
for pkg in ./internal/probe ./internal/score; do
  if go list -deps "$pkg" | grep -qE 'internal/(netx|server|runner)'; then
    echo "DEPENDENCY VIOLATION: $pkg depends on outer layer"; exit 1
  fi
done
```

**关于「probe 包内既有纯数据又有采集器」的取舍**：这是基于 `app/` 现状（`internal/probe/http.go` 已含采集逻辑）的务实选择，而非理想分层。**折中理由**：探测器的领域规则（如何归因、如何判定状态）与其采集动作高度耦合，强行拆两个包会产生大量只有 3 个字段的 DTO 与转换函数——那是架构表演。真正的保护落在**箭头单向**这一条上，足以防止循环依赖与责任倒挂。

---

## 2. 目录结构

```
Test_VPN_Nodes/
├── app/
│   ├── go.mod                          module proxylens  (go 1.26)
│   ├── cmd/proxylens/main.go           ★ 唯一 main：flag → 启服务 → 开浏览器
│   │
│   ├── internal/
│   │   ├── config/
│   │   │   ├── config.go               RunConfig + Normalize + preset 默认值
│   │   │   └── config_test.go
│   │   ├── netx/                       ← 出网路径（Infrastructure）
│   │   │   ├── proxy.go                Route 类型 + Resolve(mode, raw)
│   │   │   ├── system_darwin.go        scutil --proxy 解析
│   │   │   ├── system_windows.go       注册表 Internet Settings 解析
│   │   │   ├── system_other.go         其他平台 stub
│   │   │   ├── socks5.go               ★ 自写 SOCKS5/SOCKS5h（DOMAINNAME + RFC1929）
│   │   │   ├── transport.go            TransportFactory
│   │   │   ├── classify.go             fake-IP / RouteKind 判定
│   │   │   ├── guard.go                SSRF 网段校验（DialContext 收口）
│   │   │   └── *_test.go
│   │   ├── probe/                      ← 领域 + 采集
│   │   │   ├── types.go                Status/Sample/Phase/Result/Meta + 数学
│   │   │   ├── timer.go                httptrace 计时器
│   │   │   ├── classify.go             状态码/响应头/body 指纹 → Status
│   │   │   ├── collect_p1.go           Google 大文件吞吐
│   │   │   ├── collect_p2.go           YouTube 三级降级链
│   │   │   ├── collect_p3.go           ChatGPT 网页
│   │   │   ├── collect_p4.go           Gemini 网页
│   │   │   ├── collect_p5.go           OpenAI API
│   │   │   ├── collect_p6.go           国际链路基线（N 次采样）
│   │   │   ├── throughput.go           速率采样器（滑动窗口/p50/CV/decay）
│   │   │   └── *_test.go
│   │   ├── score/
│   │   │   ├── model.go                权重表 + formula_id + 归一化
│   │   │   ├── verdict.go              总分 → 结论文案
│   │   │   └── model_test.go
│   │   ├── runner/                     ← 应用层编排
│   │   │   ├── controller.go           单 Run 互斥（409）、start/cancel、ctx 树
│   │   │   ├── scheduler.go            worker pool + 每 probe 独立 ctx/超时
│   │   │   ├── eventbus.go             发布 + 200ms 合并节流
│   │   │   ├── event.go                Event 类型
│   │   │   └── report.go               Run 汇总 → Report
│   │   ├── server/                     ← 传输层
│   │   │   ├── server.go               net.Listen 127.0.0.1:0 + http.Serve
│   │   │   ├── bootstrap.go            token 生成 + GET /?t= + 60s 窗口
│   │   │   ├── middleware.go           hostAllowed/auth/origin/headers
│   │   │   ├── routes_state.go         GET  /api/state
│   │   │   ├── routes_probe.go         POST /api/probe, /api/probe/cancel
│   │   │   ├── routes_events.go        GET  /api/events (SSE hub)
│   │   │   ├── routes_export.go        POST /api/export (json/markdown)
│   │   │   ├── sse.go                  SSE writer + 单连接上限
│   │   │   └── assets.go               go:embed web/ + ETag
│   │   └── version/version.go          -X 注入
│   │
│   └── web/                            ← ★ 前端，每组件一文件，零构建
│       ├── index.html                  单页外壳（<meta name="pl-token">）
│       ├── styles/
│       │   ├── tokens.css              CSS 变量（色板/字号/间距/圆角/阴影）
│       │   ├── base.css                reset + 排版基线
│       │   └── themes.css              [data-theme="dark|light"]
│       ├── app.js                      入口：装配壳 + 订阅事件 + 首屏渲染
│       ├── core/
│       │   ├── store.js                极简 state + 订阅（无框架）
│       │   ├── api.js                  fetch 封装（带 X-ProxyLens-Token）
│       │   ├── events.js               EventSource 封装（重连 + 退避）
│       │   ├── format.js               数字/字节/毫秒/Mbps 格式化
│       │   └── dom.js                  h() 轻量元素构造器
│       └── components/                 ★ 一文件一组件
│           ├── app-shell.js            顶栏 + 布局骨架 + 主题切换
│           ├── run-panel.js            模式/预设/代理输入/开始按钮
│           ├── probe-card.js           单探测项卡片（状态/主指标/分阶段）
│           ├── probe-grid.js           探测项列表容器
│           ├── phase-bar.js            分阶段耗时条（DNS/TCP/TLS/TTFB）
│           ├── speed-chart.js          实时速率曲线（canvas，200ms 点）
│           ├── score-ring.js           总分环（SVG）
│           ├── score-breakdown.js      分项分条
│           ├── route-badge.js          出网路径徽章（os_proxy/tun/direct）
│           ├── evidence-list.js        证据链展示（cf-mitigated 等）
│           ├── report-view.js          报告视图
│           ├── export-bar.js           导出 JSON/Markdown
│           ├── notice.js               提示/警告/脚注（含 CHALLENGE 文案）
│           └── footer-note.js          诚实性脚注（测量口径声明）
│
├── docs/                               见 00-task-brief.md §5
├── scripts/{build.sh,lint-deps.sh,dev.sh}
└── README.md                           Gatekeeper / SmartScreen 说明
```

### 2.1 目录结构的硬规定

| # | 规定 | 理由 |
|---|---|---|
| 1 | `web/components/` 下一文件一组件，**单文件 ≤ 250 行** | 用户明确要求组件化；250 行是可读性拐点 |
| 2 | 组件只导出 `render(props) → Node`（+ 可选 `update(node, props)`），**不得导出全局状态** | 状态只在 `core/store.js`，避免隐式耦合 |
| 3 | `probe` 用 `probe.Timer` 抽象 httptrace，**不直接暴露 trace 回调给 domain** | 便于单测注入 fake timer |
| 4 | `server` 只持有 `*runner.Controller`，**不 import `netx`** | 防止 controller 直连基础设施 |

### 2.2 前端为何零构建、不引框架

| 方案 | 体积(gz) | 需构建 | 评价 |
|---|---|---|---|
| React + Vite | 45 KB | **是** | 引入 node_modules(~200 MB)、lockfile、构建链、sourcemap 管理。为 14 个叶子组件付此代价不合理 |
| Vue + Vite | 34 KB | **是** | 同上 |
| Preact + htm | 4 KB | 否 | 候选。但 tag 函数调试体验不如原生 DOM |
| **原生 ES Module + `h()`** | **0 KB** | **否** | **选定** |

**关键论据**：本 UI 复杂度不在组件树（14 个叶子组件、无深层嵌套），而在**实时数据流**（SSE 200ms 事件 → 局部更新图表）。框架的核心收益（虚拟 DOM diff）在「每秒 5 次、每次只改几个文本节点」的场景下反而是开销。原生 + 定向 `textContent` 更新的性能与可预测性更好。

**代价（明确接受）**：① 没有 JSX，UI 结构用 `h()` 函数调用，略啰嗦；② 无响应式，需手写 `store.subscribe`；③ 无类型检查（可后续加 `// @ts-check` + JSDoc，不引 TypeScript 编译器）。

---

## 3. 并发模型

### 3.1 三层并发结构

```
                     ┌──────────────────────────────────┐
                     │  RunController（单 Run 互斥）      │
                     │  mutex + currentRun *run          │
                     └───────────────┬──────────────────┘
                                     │ 创建 runCtx（可取消）
                     ┌───────────────▼──────────────────┐
                     │  Scheduler（worker pool）          │
                     │  sem = make(chan struct{}, N)     │
                     └──┬────┬────┬────┬────┬────┬───────┘
                        │    │    │    │    │    │  每个 probe 一个 goroutine
                        ▼    ▼    ▼    ▼    ▼    ▼
                       P1   P2   P3   P4   P5   P6
                        │    │    │    │    │    │  每个 probe 内部再可有子 goroutine
                        │    │    │    │    │    │ （如 P1 多流、P6 串行采样）
                        ▼    ▼    ▼    ▼    ▼    ▼
                    ┌──────────────────────────────┐
                    │  EventBus（200ms 合并 + 广播） │
                    └──────────────┬───────────────┘
                                   ▼
                     ┌─────────────────────────┐
                     │  SSE hub（fan-out 到 N 个浏览器连接）│
                     └─────────────────────────┘
```

### 3.2 具体参数

| 参数 | 值 | 理由 |
|---|---|---|
| `Scheduler.maxConcurrent` | **默认 3**（quick）/ 4（standard/deep），上限 6 | 探活类 probe 可以并行（都是小请求）；吞吐类（P1）必须独占，见 §3.4 |
| 每 probe 超时 | `config.TimeoutSec` 为**总预算**，probe 内部再细分（如 P6 单次 3000 ms） | 避免一个慢 probe 拖死整个 Run |
| `runCtx` | `context.WithCancel(context.Background())` + `WithTimeout(总预算)` | 双层：用户取消 + 硬兜底 |
| 每 probe ctx | `context.WithTimeout(runCtx, probeBudget)` | **必须从 runCtx 派生**，否则用户取消无法传播到 probe 内部 |
| 全局取消 | `controller.Cancel()` → `runCancel()`；**同时关闭所有 `*http.Transport`** 的 idle conn | 仅 cancel ctx 不够：一个正在 `io.Copy` 的连接不会立刻感知 ctx，需在 `Copy` 循环里显式检查 `ctx.Err()` |
| 事件节流 | **200 ms** 合并窗口 | 见 §3.5 |
| SSE 单连接上限 | 4（超限关最旧的） | `04-security.md` §3.8 |

### 3.3 `Probe` 契约（engine 已提出，本文确认）

```go
// 契约：实现方必须尊重 ctx；必须通过 sink 报告进度；不得 panic。
type Probe interface {
    Meta() Meta
    Run(ctx context.Context, env *Env, sink EventSink) (*Result, error)
}

// Env 是本次 Run 的共享只读环境（由 netx 构造，probe 只读）
type Env struct {
    RunID       string
    Route       netx.Route          // 出网路径（含 Settings 与 Warnings）
    Shared      *http.Transport     // 探活类共享
    Fresh       func() *http.Transport // 吞吐类每次新建
    StartedAt   time.Time
}

// EventSink 必须非阻塞。实现方内部有缓冲 channel，满则丢弃
// （丢弃的是中间采样点，不是终态事件；终态事件用 Deliver 阻塞发送）。
type EventSink interface {
    Emit(e Event)        // 非阻塞，可丢
    Deliver(e Event)     // 阻塞直到入队（仅用于 probe.done / run.done）
}
```

**`Emit` 非阻塞的必要性（关键）**：若 `Emit` 阻塞，一个慢的 SSE 客户端会反压到探测引擎，导致**测量本身被 UI 拖慢** —— 那测量结果就失真了。`ProxyLens` 测的是网络，UI 卡顿绝不能影响数字。

**实现骨架**：

```go
func (b *EventBus) Emit(e Event) {
    select {
    case b.ch <- e:       // 快路径
    default:              // 缓冲满 → 丢弃中间采样，记录 dropped++
        atomic.AddInt64(&b.dropped, 1)
    }
}
```

**丢弃策略**：`sample`（速率点）可丢 → 前端用后续采样点仍然能画出曲线（只是稀疏）；`probe.start` / `probe.done` / `run.done` 必须 `Deliver`（阻塞）→ 保证 UI 不会漏掉状态转移。

### 3.4 执行编排：两波次（避免带宽自污染）

**问题**：P1（吞吐）与 P2（YouTube 分片）、P6（基线采样）若并行，会互相抢带宽，导致吞吐偏低、jitter 偏高。这不是「并发优化」，是**测量污染**。

**解法：分成两个波次（wave）**

```
Wave 1（探活波，可并行，并发 3）
  P3 ChatGPT ─┐
  P4 Gemini  ─┼─ 都是小请求（≤64 KiB body），彼此影响可忽略
  P5 OpenAI  ─┘

Wave 2（吞吐 + 稳定性波，串行关键路径）
  P6 基线（10 次 × 300ms 间隔 ≈ 3s）
        ↓
  P2 YouTube（页面 + 缩略图 + 边缘握手 + gvt1 吞吐）
        ↓
  P1 Google 大文件（独占，无其他 probe 运行）
```

**为什么这样分**：

| 波次 | 内容 | 并行度 | 理由 |
|---|---|---|---|
| Wave 1 | P3/P4/P5 | 3（全并行） | 小请求、总字节 < 100 KB，互相干扰 < 1% |
| Wave 2 | P6 → P2 → P1 | **1（严格串行）** | 都要测时间/速率，并行即失真 |

**总时长估算**（standard）：

```
Wave1: max(P3 15s, P4 20s, P5 10s) ≈ 20s（实际通常 1–3s，超时才是上界）
Wave2: P6 3s + P2 5s + P1 15s      ≈ 23s
合计 ≈ 5–8s（正常），最坏 ≈ 43s（全超时）
```

符合 `00-task-brief.md` §2.5「单次轻量检测 < 10s」的期望（quick 预设）。

### 3.5 事件流节流（200 ms 合并）

**问题**：P1 在 200ms 采样窗口下，10 秒内产生 ~50 个 `sample` 事件；若每个都触发一次 DOM 更新 + canvas 重绘，浏览器主线程会被打满，反而卡顿。

**解法：服务端合并 + 客户端 rAF 批处理（双保险）**

```go
// 服务端：EventBus 的 flusher goroutine
ticker := time.NewTicker(200 * time.Millisecond)
pending := map[string]*ProbeDelta{}   // key = probeID
for {
    select {
    case e := <-b.ch:
        accum(pending, e)             // 同一 probe 的多个 delta 合并
    case <-ticker.C:
        flush(pending)                 // 一次性推给 SSE
        pending = map[string]*ProbeDelta{}{}
    }
}
```

```js
// 客户端：rAF 批处理（合并窗口内的多次 flush 只渲染一次）
let dirty = false
function markDirty() { if (!dirty) { dirty = true; requestAnimationFrame(render) } }
function render() { dirty = false; /* 按 store 差异更新 */ }
```

**200 ms 的取值理由**：

| 值 | 评价 |
|---|---|
| 50 ms | 事件太密，且人眼分辨不出 20 fps 以上的数字变化 → 浪费 |
| **200 ms** | **选定**：5 Hz 更新，数字跳动可读，图表平滑，CPU 占用 < 1% |
| 500 ms | 用户会觉得「卡住了」（网络工具的反馈感很重要） |

**特殊事件不走节流**：`probe.start` / `probe.done` / `run.done` / `run.error` 必须**立即推送**（用户对「探测项变绿了」的感知延迟极敏感）。

### 3.6 取消传播的三个必做点

`context` 取消不会自动中断阻塞的 `io.Copy`。必须三处配合：

```go
// ① 请求级：NewRequestWithContext —— 覆盖连接建立阶段
req, _ := http.NewRequestWithContext(ctx, ...)

// ② 读取级：包一层 ctx-aware Reader —— 覆盖数据传输阶段
type ctxReader struct { r io.Reader; ctx context.Context }
func (c ctxReader) Read(p []byte) (int, error) {
    select {
    case <-c.ctx.Done(): return 0, c.ctx.Err()
    default: return c.r.Read(p)
    }
}

// ③ 连接级：取消时主动关闭 —— 覆盖「卡在 Read 里不返回」的极少数情况
go func() {
    <-ctx.Done()
    client.CloseIdleConnections()
    tr.CloseIdleConnections()
}()
```

**验证要求**：`TestCancel` 必须断言「取消后 500 ms 内 `Run` 返回 `ErrCancelled`，且 `Result.Status == cancelled`」。

---

## 4. 数据模型

> 与 `app/internal/config/config.go`、`app/internal/probe/types.go` 现状保持一致；本文补充 `RunConfig` 的 `network` 子结构与 `Score`/`Report`。

### 4.1 类型关系

```
RunConfig ──1:1──► Route（出网路径，由 netx 解析）
    │
    └──1:N──► ProbeResult ──1:N──► Sample（速率点）
                    │
                    ├──1:1──► Phase（分阶段耗时）
                    ├──1:N──► Evidence（证据链字符串）
                    └──1:1──► ProbeScore（分项分）
    │
    └──1:1──► Report ──1:1──► Score（总分 + 分项）
```

### 4.2 `RunConfig`

```json
{
  "runId": "01JQ8ZK3M4N5P6Q7R8S9T0",
  "mode": "system",
  "proxyUrl": "",
  "preset": "standard",
  "concurrency": 3,
  "budgetMb": 24,
  "durationSec": 30,
  "timeoutSec": 20,
  "probes": ["P3", "P4", "P5", "P6", "P2", "P1"]
}
```

| 字段 | 类型 | 取值 | 说明 |
|---|---|---|---|
| `runId` | string | ULID 风格 | 时间有序，便于排序与日志关联 |
| `mode` | enum | `system` \| `custom` \| `direct` | 手册 §2.3 的三种模式 |
| `proxyUrl` | string | 空 或 `scheme://[user:pass@]host:port` | **仅在 `custom` 时非空**；响应中必须 mask |
| `preset` | enum | `quick` \| `standard` \| `deep` | 决定下面三个的默认值 |
| `concurrency` | int | 1–16 | 上限 16（防止用户把本机打爆） |
| `budgetMb` | int | 1–512 | 单流字节预算 |
| `durationSec` | int | 1–120 | 时长预算（双闸门，见 02 §4.5） |
| `timeoutSec` | int | 1–60 | 每 probe 总预算 |

**预设默认值**：

| preset | concurrency | budgetMb | durationSec | timeoutSec | 预计耗时 |
|---|---|---|---|---|---|
| `quick` | 3 | 8 | 10 | 10 | 5–10 s |
| `standard` | 3 | 24 | 30 | 20 | 10–25 s |
| `deep` | 4 | 64 | 60 | 30 | 30–90 s |

### 4.3 `Route`（出网路径，由 `netx` 产出）

```json
{
  "kind": "tun",
  "source": "os",
  "proxyMasked": null,
  "effective": "TUN 全局接管（fake-IP 198.18.0.0/15）",
  "warnings": [
    "检测到环境变量代理 http://127.0.0.1:64080，系统模式下不使用它",
    "未检测到系统 HTTP/HTTPS/SOCKS 代理，流量由 TUN 接管"
  ]
}
```

| 字段 | 取值 | 说明 |
|---|---|---|
| `kind` | `os_proxy` \| `tun` \| `direct` \| `pac_unsupported` | **必须如实**分类（02 §2.6） |
| `source` | `os` \| `manual` \| `direct` | 从哪来的 |
| `proxyMasked` | string \| null | `http://user:***@host:port` |
| `effective` | string | 给人看的一句话 |
| `warnings` | string[] | 必须原样展示给用户 |

### 4.4 `ProbeResult`

```json
{
  "id": "P1",
  "name": "Google 大文件下载",
  "group": "speed",
  "status": "ok",
  "headline": "4.13 Mbps",
  "subline": "dl.google.com · 24 MiB · 单流",
  "phases": { "dnsMs": null, "tcpMs": 0.31, "tlsMs": 102.4, "ttfbMs": 218.6, "totalMs": 51200 },
  "httpStatus": 206,
  "finalUrl": "https://dl.google.com/chrome/mac/universal/stable/GGRO/googlechrome.dmg?pl=01JQ8Z...",
  "remoteAddr": "198.18.0.69:443",
  "tlsVersion": "TLS 1.3",
  "bytes": 25165824,
  "mbps": 4.19,
  "peakMbps": 5.82,
  "p50Mbps": 4.31,
  "p90Mbps": 5.42,
  "cv": 0.184,
  "decayRatio": 0.91,
  "ttfbMs": 218.6,
  "jitterMs": 0,
  "successPct": 100,
  "samples": [
    { "t": 1000, "mbps": 2.10, "bytes": 262144 },
    { "t": 1200, "mbps": 3.88, "bytes": 485376 },
    { "t": 1400, "mbps": 4.42, "bytes": 552960 }
  ],
  "evidence": [
    "HTTP 206 Partial Content",
    "content-range: bytes 0-25165823/278613265",
    "本地代理建连 0.31ms（非远端 RTT）",
    "TLS 1.3 握手 102.4ms"
  ],
  "error": "",
  "score": 78.4,
  "durationMs": 51200,
  "connReused": false,
  "partial": false
}
```

**字段语义要点（都是实测推出来的，见 02 §5）**：

| 字段 | 规则 |
|---|---|
| `phases.dnsMs` | **走 HTTP 代理时必须为 `null`**，不得为 0；TUN fake-IP 时也建议 `null` 并在 `evidence` 说明 |
| `phases.tcpMs` | **UI 标签必须是「本地代理建连」**，不是「TCP 建连/RTT」 |
| `phases.tlsMs` | **唯一端到端 RTT 代理量**，作为「延迟」主指标 |
| `remoteAddr` | 若 IP ∈ `198.18.0.0/15` → UI 必须标注「fake-IP（非真实出口）」 |
| `connReused` | `true` 时前端对 DNS/TCP/TLS 三段显示 `—`，不显示 `0.0 ms` |
| `samples[].t` | 相对 Run 起点的毫秒数；**前 1000 ms 的慢启动样本必须剔除**（不在 `samples` 中出现） |
| `partial` | 因 `durationSec` 截断时为 `true`，报告需标注 |

### 4.5 `Status` 枚举（与 `probe/types.go` 一致）

| Status | 中文 | 颜色语义 | 是否参与 AI 分 |
|---|---|---|---|
| `ok` | 正常 | 绿 | ✓ |
| `degraded` | 降级（慢/限流/5xx） | 黄 | ✓（打折） |
| `challenge` | 人机验证（CF 质询） | 黄 | ✓（打折，**不是失败**） |
| `region_blocked` | 地区不可用 | 红 | ✗（直接判死） |
| `auth_required` | 需登录 | 蓝 | ✓（网络通） |
| `hijacked` | 被劫持/篡改 | 红 | ✗ |
| `failed` | 失败（超时/TLS 重置等） | 红 | ✗ |
| `skipped` | 跳过（未选中） | 灰 | — |
| `cancelled` | 已取消 | 灰 | — |

**`challenge` 必须是黄色而不是红色** —— 这是产品诚实性的核心表达（02 §7.3）。

### 4.6 `Score` 与 `Report`

```json
{
  "runId": "01JQ8ZK3M4N5P6Q7R8S9T0",
  "startedAt": "2026-10-22T08:40:12Z",
  "finishedAt": "2026-10-22T08:40:31Z",
  "durationMs": 19342,
  "route": { "kind": "tun", "source": "os", "effective": "TUN 全局接管", "warnings": [] },
  "config": { "mode": "system", "preset": "standard", "budgetMb": 24 },
  "probes": [ { "id": "P1", "...": "见 4.4" } ],
  "score": {
    "total": 72.5,
    "verdict": "good",
    "verdictText": "代理可用：AI 站点可达，下行约 4 Mbps，适合网页与标清视频；4K 会缓冲。",
    "breakdown": [
      { "key": "aiAccess",   "label": "AI 可用性", "value": 85.0, "weight": 0.40, "formulaId": "ai.v1" },
      { "key": "speed",      "label": "下行速度",   "value": 62.0, "weight": 0.30, "formulaId": "speed.v1" },
      { "key": "stability",  "label": "稳定性",     "value": 74.0, "weight": 0.20, "formulaId": "stab.v1" },
      { "key": "latency",    "label": "延迟",       "value": 68.0, "weight": 0.10, "formulaId": "lat.v1" }
    ]
  },
  "caveats": [
    "未做直连对照，绝对速率仅供参考",
    "YouTube 未取到真实播放分片，边缘下行 3.2 Mbps 为同链路代理指标",
    "检测到 Cloudflare 人机验证，请在浏览器确认 ChatGPT 是否可正常使用"
  ],
  "schemaVersion": 1
}
```

**评分权重的设计原则（详细公式在 `06-engine-design.md`）**：

| 维度 | 权重 | 理由 |
|---|---|---|
| AI 可用性 | **0.40** | 这是产品的**差异化价值**。用户问的是「能不能用 AI」，这是主问题 |
| 下行速度 | 0.30 | 「卡不卡」的主要因子 |
| 稳定性 | 0.20 | 「稳不稳」；抖动比均速更能预测体感 |
| 延迟 | 0.10 | 网页首屏相关，但被 TTFB 部分覆盖，权重最低 |

**强制规则（反黑箱）**：

1. `formulaId` 必须存在且指向 `score/model.go` 里的具名函数。
2. `breakdown` 四项权重之和必须 == 1.0（单测断言）。
3. **一票否决**：任一 AI probe 为 `region_blocked` / `hijacked` / `failed` → **总分上限 40**（不管速度多快）。理由：代理不能上 AI，速度再快对用户无价值。
4. `caveats` 非空时必须展示在报告顶部，不得折叠。

---

## 5. 架构决策记录（ADR）

> 每条 ADR 强制包含：**背景 / 选项 / 决策 / 理由 / 代价**。代价项不得为空——写不出代价说明还没想清楚。

---

### ADR-01：语言与运行时 —— Go 1.26 单二进制

| | |
|---|---|
| **状态** | Accepted |
| **相关** | `02-tech-research.md` §1、`M6`、`M7` |

**背景**

需要同时支持 Windows 10/11（x64/arm64）与 macOS 12+（Intel/Apple Silicon），不允许依赖用户预装运行时，且必须能在 socket 层拿到分阶段耗时。开发机只装了 Go / Node / Python，**Rust 未装**。

**选项**

| 方案 | 分发体积 | 冷启动 | 交叉编译 | 分阶段耗时 | 判定 |
|---|---|---|---|---|---|
| **A. Go 1.26** | **3.9–4.3 MB**（实测） | **< 10 ms** | **一台机出四包**（实测） | `httptrace` 完整 | ✅ |
| B. Rust + Tauri | 8–20 MB | 300–800 ms | 需各平台链接器 | 完整但实现量 3× | ❌ 无 cargo，且仍依赖 WebView2 |
| C. Node + Electron | 90–150 MB | 1–3 s | 需各平台 runner | 部分（双网络栈混乱） | ❌ 与「空闲 CPU 近 0」冲突 |
| D. Python + WebView | 25–60 MB | 0.8–3 s | 需各平台打包 | 需手写 | ❌ 违反「不依赖预装运行时」 |

**决策**

**Go 1.26.3 + `CGO_ENABLED=0` + `-trimpath -ldflags="-s -w"`，单二进制分发；不使用 UPX。**

**理由**

1. **实测 `M6`**：一台 macOS arm64 主机，无任何外部工具链，四个目标全部编译成功，体积 3.94/4.23/4.27/3.94 MB。「一次构建、四平台单文件」是已跑通的命令，不是承诺。
2. **实测 `M7`**：进程冷启动 < 10 ms。这是 Electron（1–3 s）与 Python onefile（解压到 tmp）无法接近的量级。
3. `net/http/httptrace` 提供了**唯一无需手写 socket 层**的分阶段计时能力（`M9` 已验证可用）。
4. `CGO_ENABLED=0` 带来纯静态链接 → 无 GLIBC/musl 版本问题 → 「任何 macOS 12+ / 任何 Win10+ 直接跑」。
5. 本机已装 Go，**零工具链前置成本**。

**代价（明确接受）**

1. **UI 观感不如原生应用**：无原生窗口、无菜单栏集成。缓解：默认浏览器打开 + 全屏单页布局（ADR-02）。
2. **`-s -w` 去掉符号表** → panic 无行号。缓解：结构化日志 + `-X main.version` 注入版本。
3. **无法使用需要 cgo 的能力**：macOS WKWebView、CFNetwork API、Windows WinINET API。这直接导致 ADR-02 与 ADR-05 的取舍。
4. **不用 UPX** → 放弃 60% 体积压缩。理由：UPX 会让启动慢 30–80 ms（抵消冷启动优势）、杀软误报率极高、可能破坏 macOS 签名。**用户支持成本远高于 4 MB 下载成本。**
5. Go 的 GC 对「零分配热路径」有要求；`io.Copy` 到 `io.Discard` 已是最优路径，但速率采样器需注意避免每 200 ms 分配切片。

---

### ADR-02：UI 技术栈与外壳形态 —— 默认浏览器 + 零构建原生 ES Module

| | |
|---|---|
| **状态** | Accepted |
| **相关** | `02-tech-research.md` §1.4、§2.2 |

**背景**

需要「界面美观、性能好、实时流式刷新、静态资源本地内嵌、无外部 CDN 依赖」，且分发不能依赖用户预装运行时。

**选项**

| 方案 | 依赖 | 构建链 | 观感 | 破坏 `CGO_ENABLED=0` | 判定 |
|---|---|---|---|---|---|
| A1. 默认浏览器 + React/Vite | 浏览器 | **需** | 好 | 否 | 可选 |
| **A2. 默认浏览器 + 原生 ESM** | 浏览器 | **不需要** | 好 | 否 | ✅ |
| B. 内嵌 WebView2（Windows） | **WebView2 Runtime** | 否 | 最好 | Windows 否 | 部分可用 |
| C. 内嵌 WKWebView（macOS） | 无 | 否 | 最好 | **是** | ❌ |
| D. Electron | 内置 Chromium | 需 | 最好 | 否 | ❌ 90 MB+ |

**决策**

**A2：默认浏览器 + 原生 ES Module（零构建）+ `go:embed` 内嵌。macOS 与 Windows 使用同一形态，不做平台分化。**

**理由**

1. **内嵌 WKWebView 必须开 cgo** —— 这会直接摧毁 ADR-01 建立的「一台机器出四包」优势（cgo 交叉编译需各平台 SDK + 交叉工具链，成本回到 Tauri 水平）。**这是排除 C 的决定性理由。**
2. **内嵌 WebView2 引入「用户可能没装」的依赖** —— Win11 预装、Win10 21H2+ 多数预装，但「多数」不满足「不允许依赖用户预装运行时」。兜底需带 Evergreen Bootstrapper（+1.8 MB 且需联网）。
3. 默认浏览器**零依赖**：用户必有浏览器。
4. **零构建链**：`go:embed web/` 直接内嵌源文件，`go build` 即出包。无 node_modules、无打包器、无 lockfile 漂移、无「改了前端忘了重新 build」。
5. 本 UI 复杂度不在组件树（14 个叶子组件），在实时数据流。框架的虚拟 DOM diff 在「每秒 5 次、每次改几个文本节点」场景下是**净开销**；原生定向 `textContent` 更新更可预测。
6. 同源（`127.0.0.1:<port>` 同时提供 HTML 与 API）→ **零 CORS 配置**，且天然满足 `04-security.md` 的 SameSite/Origin 校验模型。

**代价（明确接受）**

1. **有地址栏/标签页**，观感不如原生窗口。缓解：`08-ui-spec.md` 按全屏单页设计，`max-width` 约束、无滚动条主视图。用户可能同时开多个标签页 → 由 `04-security.md` 的 SSE 单连接上限（4）保护。
2. **无 JSX / 无响应式 / 无类型检查** —— UI 代码用 `h()` 函数调用，略啰嗦；状态订阅需手写。缓解：`core/dom.js` 提供 60 行的 `h()`；可后续加 `// @ts-check` + JSDoc（不引 tsc 编译器）。
3. **浏览器兼容性责任在我方**：需支持 Chrome/Edge/Safari 当前版本。缓解：只用 Baseline 特性（ES2022、CSS 嵌套、`:has()` 避开），不引 polyfill。
4. **iframe / 弹窗拦截风险**：`open` 打开新标签页在某些浏览器设置下可能被拦。缓解：**同时把 URL 打印到 stdout**，并在 UI 上提供「手动复制链接」。
5. **用户可误触浏览器刷新导致 SSE 断连** → 必须实现自动重连 + `Last-Event-ID` 补发（ADR-08）。

**未来选项（零成本预留）**：Windows 侧可加 `--webview` 参数用 `webview2` Go 绑定（Windows cgo 可用 mingw 交叉编译，风险局部化）。**macOS 侧永不做。**

---

## 5. 架构决策记录（ADR）

---

### ADR-01：语言与运行时 —— Go 1.26 单二进制

**状态**：Accepted

**背景**

需求要求 Windows 10/11（x64/arm64）+ macOS 12+（Intel/Apple Silicon）四目标、单文件分发、**不允许依赖用户预装运行时**、空闲 CPU 近 0、能拿到 socket 级分阶段耗时。开发机实测环境：Go 1.26.3 已装、Node 22.22.2 已装、Python 3.13.12 已装、**Rust/cargo 未装**。

**选项**

| 选项 | 实测/推定体积 | 需预装运行时 | 能否分层计时 | 本机可立即开工 |
|---|---|---|---|---|
| **Go** | **3.9–4.3 MB**（实测 `M6`） | 否（纯静态） | 是（`httptrace`） | **是** |
| Rust + Tauri | 8–20 MB | WebView2（Win） | 是 | 否（无 cargo） |
| Node + Electron | 90–150 MB | 否（自带，但巨大） | 部分（网络栈分裂） | 是 |
| Python + WebView | 25–60 MB | 是 / 或 PyInstaller | 是（但需手写） | 是 |

**决策**

**Go 1.26，`CGO_ENABLED=0`，`-trimpath -ldflags="-s -w"`，单二进制，`go:embed` 内嵌前端。**

**理由**

1. **唯一的「四目标 × 单文件 × 零工具链」组合**。实测一条命令从 macOS arm64 产出 darwin/arm64（3.96 MB）、darwin/amd64（4.23 MB）、windows/amd64（4.27 MB）、windows/arm64（3.94 MB）。这不依赖任何交叉编译 SDK。
2. **`CGO_ENABLED=0` 是本决策的技术基石**。它同时带来：纯静态链接（无 GLIBC/musl 兼容问题）、零外部工具链、可复现构建。它也**反过来否决了内嵌 WKWebView**（见 ADR-02）。
3. **`net/http/httptrace` 提供唯一可信的分层计时**。实测三个路径下都能精确拿到 TLS 握手与 TTFB（`M9`），这是「区分慢与不可用」的基础。
4. **冷启动 < 10 ms**（`M7`），空闲态仅一个 goroutine 阻塞在 `Accept`，CPU ≈ 0，内存 8–15 MB。
5. **本机已装**，团队零启动成本。

**代价（明确接受）**

- **无 GUI 原生能力**：不能创建原生窗口、托盘、菜单栏。这是 ADR-02 的根源。
- **前端必须零构建**：Go 无打包器生态，前端不能用 TSX/Vite（ADR-02 展开）。
- **`panic` 栈无行号**（`-s -w` 代价）：靠 `-X main.version` + 结构化日志弥补。
- **生态无成熟 SOCKS5h 库**：需自写（ADR-05）。
- **二进制 4 MB 对「一个小工具」仍显大**：不用 UPX（杀软误报 + 破坏签名，见 `02-tech-research.md` §8.2）。

---

### ADR-02：UI 技术栈 —— 默认浏览器 + 原生 ES Module，零构建

**状态**：Accepted

**背景**

用户要求「界面美观」「性能好」「静态资源本地内嵌、无外部 CDN 依赖」，需求文档要求前端**组件化、每组件一文件、禁止单文件大段 UI**。可选的技术栈空间：React/Vue（需构建）、Preact+htm（免构建）、原生 ES Module（免构建）、内嵌 WebView。

**选项**

| 选项 | 体积(gz) | 构建链 | 组件化 | 依赖运行时时 |
|---|---|---|---|---|
| React + Vite | 45 KB | **必须** | 好 | WebView |
| Vue + Vite | 34 KB | **必须** | 好 | WebView |
| Preact + htm | 4 KB | 否 | 中（tag 模板调试差） | WebView |
| **原生 ES Module + `h()`** | **0 KB** | **否** | 好（约束到位即可） | **默认浏览器** |
| 内嵌 WKWebView/WebView2 | — | 否 | 同上 | **是**（且破坏 `CGO_ENABLED=0`） |

**决策**

**`go:embed` 内嵌原生 ES Module 静态资源 + 用户默认浏览器打开 `http://127.0.0.1:<port>/?t=<token>`。组件化靠「一文件一组件 + ≤250 行」的目录纪律，不靠框架。**

**理由**

1. **这是唯一与 `CGO_ENABLED=0` 兼容的 UI 外壳**。内嵌 WKWebView 在 macOS 必须 cgo → 交叉编译优势归零（详 `02-tech-research.md` §1.4）。这是决定性理由，优先级高于「原生窗口观感」。
2. **不引入任何「用户可能没装」的运行时**。WebView2 在 Win11 预装、Win10 21H2+ 多数预装 —— 「多数」不满足需求。系统浏览器则 100% 存在。
3. **框架的核心收益在本场景为负**。本 UI 复杂度不在组件树（14 个叶子组件、无深层嵌套），而在**实时数据流**：5 Hz 事件、每次只改几个文本节点。虚拟 DOM diff 在「每秒 5 次、每次 3 个节点」的场景是纯开销；原生 `textContent` 更新更快、更可预测。
4. **零构建消除了一整类工程风险**：node_modules（~200 MB）、lockfile 漂移、构建产物与源码不一致、CI 需要 Node。`go build` 即完整构建。
5. 组件化是**目录纪律问题**，不是框架问题。`web/assets/components/` 一文件一组件 + 行数上限 + 单测强制扫描，效果等价且无框架债。

**代价（明确接受）**

- **无原生窗口**：有地址栏、可能被误关标签页。缓解：`beforeunload` 提示进行中的运行；UI 按全屏单页设计（`08-ui-spec.md`）。
- **无 JSX**：UI 结构用 `h()` 函数调用，比模板啰嗦。14 个组件的规模下可接受。
- **无响应式**：需手写 `store.subscribe`。约 60 行。
- **无类型检查**：靠 `// @ts-check` + JSDoc（不引 TS 编译器），以及运行时的 `assert`。
- **必须自己实现**：路由（不需要）、图表（SVG，约 120 行）、环形进度（SVG `stroke-dasharray`，约 40 行）。
- **端口/token 机制带来额外复杂度**：见 `04-security.md`，但那是安全必需，不是浏览器方案的代价。

---

### ADR-03：分发形态 —— 裸单文件 + 脚本安装，不做安装包

**状态**：Accepted

**背景**

需求：「单文件分发优先；安装包形态可后续迭代」。无 Apple/Windows 代码签名预算。

**选项**

| 选项 | 用户操作成本 | 签名需求 | 自动更新 | 评价 |
|---|---|---|---|---|
| **裸二进制 + `install.sh`/`install.ps1`** | 中（需 `xattr` 或「仍要运行」） | 无 | 无 | **选定** |
| `.app` / `.dmg` | 低 | **必须**（否则"damaged"） | 无 | 打包结构反而加剧 Gatekeeper 问题 |
| MSI / NSIS 安装包 | 低 | 建议 | 需实现 | 工程量 > 产品本身 |
| Homebrew tap / Scoop | 低（对技术用户） | 无 | brew 自带 | **v1.1 增补**，零风险 |
| 带自动更新（如 `minio/selfupdate`） | 无感 | 需签名 | 是 | 引入网络依赖与信任链，v1 不做 |

**决策**

**分发裸可执行文件（4 个平台各一个）+ 两个安装脚本 + README 显著位置的三步手动指引。不做 `.app`/`.dmg`/MSI。v1.1 考虑 Homebrew tap 与 Scoop manifest。**

**理由**

1. **`.app` 结构是 macOS "damaged" 报错的主因**。裸 Mach-O 从终端运行不会走 App 完整性校验路径，只受 `com.apple.quarantine` 影响，用一个 `xattr -d` 即可解决。**不做 `.app` 反而是更省事的方案**。
2. 无签名预算下，任何「安装包」都会把用户挡在 Gatekeeper/SmartScreen 前，且比裸文件更难绕过。
3. `go build` 四目标 → 四个文件 → 直接挂 GitHub Release。零额外工具链（`M6` 已验证）。
4. 用户群体是「会配置代理的人」，命令行接受度高。

**代价（明确接受）**

- **首次运行有摩擦**：必须文档化三条路径（右键打开 / 系统设置「仍要打开」/ `xattr -d`），并说明 macOS 15+ 已移除右键绕过。这属于**必须承担的文档债**。
- **无自动更新**：用户需手动下载新版。对「一次性诊断工具」可接受。
- **无版本共存管理**：用户需自己放进 `PATH`。`install.sh` 提供 `~/.local/bin` 安装。
- **无图标/版本资源**：Windows 上需 `.syso`（`go-winres`，本机未装），v1 不做。

---

### ADR-03：分发形态 —— 裸单文件 + 脚本安装，不做安装包

| | |
|---|---|
| **状态** | Accepted |
| **相关** | `02-tech-research.md` §8 |

**背景**

`00-task-brief.md` §2.4：「单文件分发优先；安装包形态可后续迭代」。无 Apple Developer ID（$99/年）与 Windows 代码签名证书（$200–400/年）。用户未提预算。

**选项**

| 方案 | 体积 | 用户操作 | 签名要求 | 判定 |
|---|---|---|---|---|
| A. `.dmg` + `.app` bundle | 4 MB | 拖拽 | **必须签名公证**，否则"damaged" | ❌ |
| B. `.msi` / NSIS 安装包 | 4 MB + 安装器 | 双击下一步 | SmartScreen 强提示 | 后续 |
| **C. 裸二进制 + `install.sh`** | **4 MB** | `sh install.sh` 或双击运行 | **无** | ✅ |
| D. `go install` | — | 需装 Go | 无 | ❌ 违反「不依赖运行时」 |

**决策**

**C：直接分发裸二进制（`proxylens-darwin-arm64` 等四个文件），配套 `install.sh` 处理 macOS quarantine；不打包 `.app`、不做 `.dmg`、不做 `.msi`。**

**理由**

1. **裸 Mach-O 不打包 `.app` 是规避 Gatekeeper "damaged" 的最省事手段**。macOS 的 "app is damaged" 主要由「`.app` bundle 从 zip/dmg 解出后 `com.apple.quarantine` + 签名缺失」触发；裸二进制从终端运行只走轻量校验路径。
2. **"单文件"本身是用户的明确诉求**，安装包是反向操作（解压出一堆文件）。
3. **零签名成本**，符合 v1 定位（开源工具）。
4. `install.sh` 可同时做 `chmod +x`、`xattr -d com.apple.quarantine`、移动到 `~/.local/bin`，一条命令解决 macOS 全部摩擦。

**代价（明确接受）**

1. **Windows SmartScreen 会拦**（「Windows 保护了你的电脑」）。缓解：README 给「更多信息 → 仍要运行」图文说明。
2. **macOS 15+ 移除了「右键打开」绕过路径** → 必须走「系统设置 → 隐私与安全性 → 仍要打开」或 `xattr`。缓解：README 三种方法并列 + `install.sh` 自动化。
3. **无自动更新**。缓解：v1 不做；版本号打印在 `/api/state` 与 UI 角落，用户可自查。
4. **用户可能不认识"裸二进制"**（下载后双击无反应）。缓解：README 首屏即给「macOS: `sh install.sh` / Windows: 双击 `.exe`」两条路径。
5. Windows 默认带黑窗（控制台）。**接受**：用户能看到启动 URL 与日志反而有用（见 02 §8.4）。若要隐藏需 `-H windowsgui`，但会丢 stdout → 无法打印 URL。

---

### ADR-04：吞吐测量口径 —— 单流为主、双闸门、剔除慢启动、p50 优先

| | |
|---|---|
| **状态** | Accepted |
| **相关** | `02-tech-research.md` §4、`M3`、`M4`、`M5` |

**背景**

「代理有多快」没有唯一定义。同一链路在不同口径下可差 2 倍以上（实测 `M3` vs `M4`：4 MB 组均值 4.10 MB/s，8 MB 组 2.90 MB/s）。必须先定口径，否则数字不可比、不可复现。

**选项**

| 维度 | 选项 | 判定 |
|---|---|---|
| 流数 | 单流 / 多流 / 两者都测 | 单流默认，多流仅 deep |
| 终止 | 固定字节 / 固定时长 / 双闸门 | **双闸门** |
| 慢启动 | 计入 / 剔除前 1s | **剔除** |
| 主指标 | mean / max / p50 | **p50（+ p90 并列）** |
| 缓存 | 随机 query 强制 MISS / 保持 HIT | **保持 HIT**（quick/standard） |

**决策**

```
默认单流
预算：quick 8 MiB / standard 24 MiB / deep 64 MiB
时长闸门：quick 10s / standard 30s / deep 60s
终止条件：(bytes >= budget) OR (elapsed >= duration) OR ctx.Done()
          elapsed >= 3s 且 bytes < 64 KiB → STALLED 提前失败
剔除前 1000 ms（TCP 慢启动）
采样窗口 200 ms，minSamples = 5
主指标 = p50，并列展示 p90 / CV / decayRatio
quick/standard 不加随机 query；deep 加并标注「冷缓存」
deep 额外测 4 并发聚合值，与单流并列展示
```

**理由（全部有实测支撑）**

1. **`M3`：单次采样 CV = 11.3%，`M4` 显示不同字节数给出不同结果** → 单次采样是噪声，必须 N≥5 取 p50。
2. **`M5`：4 并发聚合 ≈ 单流，每流被均分** → 本链路单流已饱和，多流边际收益 ≈ 0。默认单流（快、省内存、指标干净）。
3. **用户的真实体感是单流**：浏览器下载单文件默认 1 连接。测多流聚合然后宣称「X Mbps」会让单流受限的代理显得比实际好。
4. **剔除慢启动**：TCP 初始 `cwnd` ~10 MSS，跨国 RTT 150 ms 时约需 1 s 到稳态。不剔除会让 p50 系统性偏低，**且偏低程度随 RTT 变化 → 跨代理不可比**。
5. **p50 而非 mean**：偶发单点会拉到 0.3 或飙到 8 Mbps，mean 被污染；p50 反映「典型体感」。
6. **`io.Copy(io.Discard, resp.Body)`**：不落盘、不驻留内存。这是「不写临时文件也能测速」的唯一正确做法。
7. **保持 HIT 而非强制 MISS**：用户日常下载 Chrome 安装包大概率命中边缘缓存。**测真相比测极限更重要**。
8. **双闸门**：固定字节在极慢代理上会跑很久；固定时长在极快代理上测不到字节。且 `STALLED`（3s < 64 KiB）让「基本不可用」能被 3 秒判定，不必让用户等满 30 s。

**代价（明确接受）**

1. **`deep` 才能测出高 BDP 链路的真实上限**。quick/standard 的单流值会低估「多线程下载器」场景（如 IDM / aria2）。缓解：报告脚注说明「本值基于单流」。
2. **不加随机 query 意味着可能测到 CDN 已缓存的热段**，比冷启动乐观。缓解：`deep` 模式并列给出冷缓存值。
3. **字节数与时长口径必须随数字一起展示**，否则用户无法跨 Run 比较。缓解：`subline` 强制包含「24 MiB · 单流」。
4. **`STALLED` 阈值（3s/64 KiB ≈ 0.17 Mbps）是拍的**——极端慢但可用的代理（如 128 kbps）会被提前判失败。缓解：阈值可通过 `--stall-bytes` 覆盖；且 0.17 Mbps 在 2026 年确实属于「不可用」。
5. **不做 UDP / QUIC 测量**。HTTP/3 场景（YouTube 大量走 QUIC）未被覆盖，会低估支持 QUIC 的代理。**这是本口径最大的已知盲区**，必须在报告脚注声明。

---

### ADR-05：代理实现 —— `Transport.Proxy` 走 HTTP(S)，自写 SOCKS5h，单一 `DialContext` 收口

| | |
|---|---|
| **状态** | Accepted |
| **相关** | `02-tech-research.md` §3、`04-security.md` §4.4、`app/internal/netx/socks5.go` |

**背景**

必须支持 `http://`、`https://`、`socks5://`、`socks5h://`、`user:pass@`。其中 `socks5h`（远程 DNS）是最容易被做错的一环：**若实现为本地解析，在 TUN fake-IP 环境下会拿到 `198.18.x.x` 然后必然失败**。

**选项**

| 方案 | HTTP(S) | socks5h | 依赖 | 判定 |
|---|---|---|---|---|
| A. `Transport.Proxy` + `x/net/proxy` | ✅ | ❌ `x/net/proxy` 内部强制 `LookupHost` | +1 | 不可用 |
| **B. `Transport.Proxy` + 自写 SOCKS5h** | ✅ | ✅ `DOMAINNAME` ATYP | **0** | ✅ |
| C. 引入 `txthinking/socks5` 等三方库 | ✅ | 多数仍本地解析 | +1~3 | ❌ |
| D. `golang.org/x/net/internal/socks` | — | — | — | ❌ internal 不可 import |

**决策**

**B：HTTP/HTTPS 代理用 `Transport.Proxy`（让 Go 处理 CONNECT 与 `Proxy-Authorization`）；SOCKS5/SOCKS5h 用自写 `socks5.go`（~195 行，已实现）经 `Transport.DialContext` 接入；`Proxy` 置 `nil`。所有出连经**单一 `DialContext`**做 SSRF 网段校验。**

**理由**

1. **`x/net/proxy` 的 `SOCKS5` dialer 契约是 `Dial(network, addr)`，内部对非 IP 的 host 执行 `net.LookupHost`** —— 这是 `socks5` 语义，不是 `socks5h`。**无法通过配置切换到 `DOMAINNAME` ATYP**。
2. **`DOMAINNAME` 透传是 `socks5h` 的全部意义**：让代理解析域名。在 TUN fake-IP 环境下，本地解析必然拿到 `198.18.x.x`（实测 `M2`），后面的连接注定失败。
3. SOCKS5 协议简单（RFC 1928 握手 ~60 行 + RFC 1929 认证 ~40 行），自写可控且**零依赖**。`app/internal/netx/socks5.go` 已按此实现。
4. **HTTP 代理不该手写**：`CONNECT` 隧道、`Proxy-Authorization`、连接池复用、h2 升级，Go stdlib 都做好了。手写等于重造轮子且必然有 bug。
5. **`DialContext` 单一收口**：`04-security.md` §4.4 要求所有出连过同一 IP 校验点（拒绝内网/保留网段）。`Transport.DialContext` 是唯一能覆盖「HTTP 代理、SOCKS、直连」三种路径的钩子。
6. **`Transport.Proxy` 与自定义 `DialContext` 语义不冲突**：HTTP 代理时 `DialContext` 连的是**代理服务器**（校验对象是代理地址）；SOCKS 时 `DialContext` 收到的是**目标地址**（校验对象是目标）。两种情形都要校验，正是收口的意义。

**代价（明确接受）**

1. **自写 SOCKS5 需自己承担协议正确性**。缓解：单测覆盖 `DOMAINNAME`/`IPv4`/`IPv6` 三种 ATYP、认证成功/失败、连接拒绝、超时；对比 `curl --socks5-hostname` 的输出做交叉验证。
2. **不支持 SOCKS4 / SOCKS4a / HTTP/2 代理 / UDP ASSOCIATE**。v1 范围外。
3. **`https://` 代理会做两级 TLS**（先与代理 TLS，隧道内再与目标 TLS），耗时叠加，且证书校验对象是代理 → 自签代理会失败。**不提供 `InsecureSkipVerify` 开关**（安全），只给友好错误文案（02 §3.4）。
4. **`DialContext` 内的 SSRF 校验会引入一次 DNS 解析**（直连模式）→ 与真实连接的解析结果可能不一致（DNS rebinding 窗口）。缓解：校验后用**校验过的 IP 直接拨号**，不重新解析（`04-security.md` §4.4 已定）。
5. **`04-security.md` 的网段黑名单会误伤**某些企业内网代理（`10.0.0.0/8` 上的代理）。缓解：代理地址（E1）与探测目标（E2/E3）走不同策略，前者允许私网（用户自己填的代理），后者拒绝。

---

### ADR-04：吞吐测量口径 —— 单流为主 + 固定字节 + 剔除慢启动 + p50

**状态**：Accepted

**背景**

需求要求「吞吐 Mbps（p50/p90）、TTFB、抖动、丢速曲线」。实测发现三件事：① 单次采样 CV 达 11–25%（`M3`/`M4`）；② 不同字节数给出不同结果（4 MB 组 4.10 MB/s vs 8 MB 组 2.90 MB/s）；③ 本机链路单流已饱和，4 并发聚合 ≈ 单流（`M5`）。

**选项**

| 选项 | 优点 | 缺点 |
|---|---|---|
| A. 单流 × 多次取 p50 | 贴近用户真实下载体感；指标干净；内存低 | 高 BDP 链路低估带宽 |
| B. 多流聚合取最大 | 能测出链路真实上限 | 高估体感速度；内存高；单流受限时失真 |
| C. 单流测一次报结果 | 最快 | **输出的是噪声**（CV 11–25%） |
| D. **单流 N 次 + 深模式补多流** | 兼顾体感与上限 | 实现略复杂 |

**决策**

```
quick/standard：单流，固定字节（8 / 24 MiB），连续 Range 请求，剔除前 1000 ms，
                以 200 ms 为窗口采样，报 p50（主）/ p90 / CV / decay_ratio。
deep：          先单流，再 4 并发，两个数并列展示，不做加权合并。
永不使用：      单次采样报结果；只报多流聚合值。
```

**理由**

1. **测的是「体感」，不是「标称带宽」**。用户下载文件、看视频，浏览器对单文件默认单连接。单流值才是他感受到的速度。`M5` 证明本机单流已饱和，多流无增益 —— 若报多流聚合，用户会对不上自己的体感。
2. **固定字节 + 标注字节数是可比性的前提**。`M3` vs `M4` 表明 4 MB 与 8 MB 差 40%。报告必须写「24 MB 样本」而不是裸 Mbps。
3. **剔除慢启动是跨代理可比的前提**。不剔除，RTT 越大的代理 p50 越低（因为慢启动占总时长比例更大），**这会系统性惩罚远距离代理**，与「代理质量」混淆。
4. **p50 而非 mean**：`M5` 中出现过 0.3 Mbps 与 8 Mbps 的尖峰，mean 被污染；p50 反映典型体感。p90 单独展示以满足「峰值」需求。
5. **多流仅在 deep 出现且不合并**：合并（加权或取大）会让用户无法判断「是代理限制单连还是链路饱和」。

**代价（明确接受）**

- **高 BDP 链路（跨国长肥管道）单流可能低估 2–5×**。缓解：`deep` 模式提供多流对照；报告在不一致时加 `caveat`「单流 X / 多流 Y，可能存在单连接限速」。
- **固定字节在极慢代理上会跑很久**。缓解：双闸门（字节 OR 时长）+ `STALLED` 早退（3 s 内 < 64 KiB 即判不可用）。
- **慢启动剔除会丢掉 1 s 的可用数据**。缓解：8 MiB 预算下仍剩 ~10–15 个采样点。
- **不测上行**。需求未要求（AI 场景下行主导）。若未来加，需独立预算与 UI 位。
- **`dl.google.com` 单点依赖**。缓解：备用 URL + 可选 `speed.cloudflare.com` 对照（但后者本机不可用，见 ADR-06）。

---

### ADR-05：代理实现 —— `Transport.Proxy` + 自写 SOCKS5，零第三方代理库

**状态**：Accepted

**背景**

需求要求支持 `http://` / `https://` / `socks5://` / `socks5h://` + `user:pass@`。关键约束：`socks5` 与 `socks5h` 的差别是 **DNS 归属**；本机处于 TUN fake-IP 环境，本地 DNS 会返回 `198.18.x.x`（`M2`），因此**必须能真正实现 `socks5h`（域名透传给代理解析）**，否则在该环境下测出的所有 `socks5` 结果都是假的。

**选项**

| 选项 | socks5h 支持 | 依赖 | cgo | 评价 |
|---|---|---|---|---|
| A. `x/net/proxy.SOCKS5` | **不支持**（内部先 `LookupHost`） | `x/net` | 否 | 语义不符，TUN 下失真 |
| B. `x/net/internal/socks` | 支持 `DOMAINNAME` | — | 否 | **不可 import**（internal 包） |
| C. 三方库（`txthinking/socks5` 等） | 视库而定 | 1+ | 否 | 多数默认本地解析；增供应链面 |
| D. **自写 ~180 行** | **完全支持** | **0** | 否 | **选定** |

**决策**

```
http:// / https://  →  http.Transport{Proxy: func(...) (*url.URL, error)}
socks5:// / socks5h://  →  http.Transport{Proxy: nil, DialContext: socks5Dialer}
                          自写 SOCKS5 握手（RFC 1928），socks5h 走 DOMAINNAME ATYP
                          认证走 RFC 1929 用户名/密码子协商
不引入任何第三方代理库。
```

**理由**

1. **`socks5h` 语义无法用 `x/net/proxy` 正确表达**，而它在本环境是必需的。选项 B 因 Go 的 `internal` 规则不可用，所以只能自写。
2. **SOCKS5 协议极小**：握手 3 步（方法协商 → 认证 → 请求）。实测实现 195 行（`app/internal/netx/socks5.go` 已完成），含两种 ATYP 与认证。复杂度远低于「集成一个库并调试它的语义」。
3. **零依赖带来可审计性与可交叉编译性**。`x/net` 只为 SOCKS 引入不值（`x/net/proxy` 的 SOCKS5 恰好是我们不要的语义）。
4. **`Transport.Proxy` 与 `DialContext` 分工清晰**：HTTP 代理是应用层隧道（让 Go 发 `CONNECT`、自己处理 `Proxy-Authorization`），SOCKS 是传输层隧道（整个 TCP 连接穿隧道，TLS 在隧道内正常进行）。混用会出错。

**代价（明确接受）**

- **自写协议有正确性风险**：需覆盖 IPv4 / IPv6 / DOMAINNAME 三种 ATYP、认证失败（0x01）、连接被拒（0x05）、超时。**必须配协议级单测**（mock server 逐个回复码）。
- **不支持 SOCKS4 / SOCKS4a**：需求未要求。SOCKS4 已淘汰，`socks4a` 的远程 DNS 语义已被 `socks5h` 覆盖。
- **不支持 UDP ASSOCIATE**：需求未要求（无 UDP 探测）。
- **不支持 SOCKS5 的 GSSAPI 认证**：企业场景极端边缘，不支持并在错误文案中说明。
- **`https://` 代理依赖代理服务器正确配置 TLS 证书**；自签代理会 `x509.UnknownAuthorityError`，且**不提供 `InsecureSkipVerify` 开关**（安全要求）。需专门错误文案。

---

### ADR-06：AI 可用性判定边界 —— 响应头优先、403 三分法、CHALLENGE 不判死

**状态**：Accepted

**背景**

需求要判定 AI 站点「可用性」并区分「慢」与「不可用」。实测（`M9`/`M10`）：

- `chatgpt.com` 经本机代理返回 **`403` + `cf-mitigated: challenge` + `server: cloudflare`**
- `api.openai.com/v1/models` 返回 **`401` + `{"error":{"type":"invalid_request_error","message":"Missing bearer authentication in header"}}`**
- `speed.cloudflare.com` 在 CONNECT 成功后 **TLS 被 RST**（`SSL_ERROR_SYSCALL`）
- `gemini.google.com` 返回 200 且 body 818 KB

**问题本质**：Go 客户端**必然拿不到浏览器能拿到的东西**（无 JS、无 cookie、指纹不符）。如果简单地「403 = 不可用」，会给出**大量误报**，用户会去换节点，而实际上打开浏览器完全能用。**误报比不报更糟。**

**选项**

| 选项 | 优点 | 缺点 |
|---|---|---|
| A. 状态码二值（2xx=可用，否则不可用） | 简单 | **误报极多**；401 判错；CF 挑战判死 |
| B. 用无头浏览器（Chromium）跑真实页面 | 最准 | 依赖 Chromium（90 MB+）；**给出比用户现实更乐观的结论** |
| C. body 关键词匹配为主 | 无需特殊头逻辑 | 需下完整页面（Gemini 818 KB）；文案会本地化/改版 |
| D. **响应头指纹优先 + body 兜底 + 三分法 + 降级语义** | 字节量小、确定性高、诚实 | 规则表需维护 |

**决策**

```
判定顺序（严格按此序，命中即停）：
  1. 重定向终态（跟随 301/302/307/308，最多 5 跳）
  2. 响应头指纹（cf-mitigated / server / server-timing）
  3. 状态码语义（200/204/401/403/429/451/5xx）
  4. body 指纹（≤64 KiB，io.LimitReader）
  5. 标题指纹（<title> 提取）

403 三分法（必须）：
  cf-mitigated: challenge|block        → CHALLENGE（不判死）
  body 含 unsupported_country 等        → REGION_BLOCKED
  server: cloudflare 但无 cf-mitigated   → CHALLENGE (probable)，需 body 二次确认
  其他                                  → DENIED

401 对 P5 = AVAILABLE（NO_CREDENTIAL_PATH_OK），不是失败。
429        = DEGRADED，归因"账号侧"，不归因代理。
CHALLENGE  = 不参与"一票否决"，但参与扣分；UI 用黄色警示，不用红色"不可用"。
```

**理由**

1. **`cf-mitigated` 是 Cloudflare 的 API 契约字段**，比 body 文案可靠一个数量级（文案随语言/版本变，header 不变）。实测已拿到（`M9`）。
2. **`401` 是 P5 的最优结果**。它证明 DNS/TCP/TLS/HTTP 全通、OpenAI 边缘正常响应、仅缺 key。若判为「不可用」，工具就完全失去意义。实测 body 确认（`M9`）。
3. **选 B（无头浏览器）会造成系统性乐观偏差**。用户日常用的是自己的浏览器，用「特别能过风控的客户端」测出的结论比现实好 —— 与「如实反映代理能力」的产品定位冲突。**这是产品诚实性问题，不是技术取舍。**
4. **body 上限 64 KiB 是必需的性能约束**。Gemini 首页 818 KB、YouTube 1.4 MB；全下完会让「可用性检测」变成「带宽测试」，且与 P1 抢带宽。
5. **CHALLENGE 不判死**：`chatgpt.com` 403 是当前环境的**真实状态**（不是工具 bug），但用户浏览器打开通常可用。必须给橙色「需浏览器验证」+ 明确文案，而非红色「不可用」。

**代价（明确接受）**

- **规则表需长期维护**。缓解：规则表数据化（`internal/aiverdict/rules_*.go` 纯 `[]Rule`），配表驱动测试；每条规则的 `ID` 透出到 `Evidence`，便于用户与开发者定位。
- **仍会漏判**：Cloudflare 若更换 header 名、或中间设备剥离 header，会退化到 body 兜底（可靠性下降）。
- **无法判定「浏览器实际能不能用」**。这是本工具的能力边界，**必须**在 UI 与 README 明示：「本工具判定的是『服务器是否愿意服务非浏览器客户端』，不等于『你的浏览器能否使用』」。
- **`speed.cloudflare.com` 不可作为主吞吐源**（本机 TLS 被 RST，`M10`）→ 只能做可选对照，失败不扣分。
- **地区限制判定依赖文案**，Google/OpenAI 改措辞时会漏判 → 需保留多条同义 pattern。

---

### ADR-06：AI 可用性判定边界 —— 响应头优先、`challenge` 不等于不可用、悲观归因禁止

| | |
|---|---|
| **状态** | Accepted |
| **相关** | `02-tech-research.md` §7、`M9`、`M10`、`06-engine-design.md` D3/D4 |

**背景**

这是产品差异化的核心，也是**最容易误报**的地方。实测 `M9`：`chatgpt.com` 经本机代理返回 `403` + `cf-mitigated: challenge`。若直接显示「ChatGPT 不可用」，用户会以为代理坏了——**而实际上用浏览器打开很可能完全正常**。误报比不报更糟。

**选项**

| 维度 | 选项 | 判定 |
|---|---|---|
| 判定依据 | body 指纹 / 响应头 / 两者 | **响应头优先，body 兜底** |
| 403+challenge | 判「不可用」/ 判「需浏览器验证（黄）」 | **黄，非红** |
| 429 / 5xx | 归因代理 / 归因账号或上游 | **归因账号/上游** |
| 是否模拟浏览器指纹以过 CF | 是 / 否 | **否** |

**决策**

```
判定顺序：重定向终态 → 状态码 → 响应头 → body(≤64 KiB) → <title>
challenge 证据优先级：
  1. cf-mitigated: challenge        （确定，实测 M9）
  2. cf-mitigated: block            （确定）
  3. server: cloudflare + 403 + cf-ray
  4. server-timing: chlray;desc=...
  5. body: challenges.cloudflare.com / Just a moment / cf-chl- / _cf_chl_opt
401 → AVAILABLE（NO_CREDENTIAL_PATH_OK，实测 M9 body 为
      {"error":{"type":"invalid_request_error","message":"Missing bearer authentication in header"}}）
429 → DEGRADED，归因「账号/额度」，不归因代理
5xx → DEGRADED，归因「上游」
challenge → CHALLENGE（黄色），文案：「代理已连通服务器，但服务器要求浏览器验证」
不做：伪造 UA 指纹、携带 Cookie、执行 JS 以绕过 CF
```

**理由**

1. **`cf-mitigated` 是 Cloudflare 的专用控制头**，取值 `challenge` 明确表示「这是质询页不是真页面」。这是 **API 契约级证据**，比匹配 `Just a moment...` 文案（会本地化、会改版）可靠一个数量级。
2. **body 指纹需要下载页面**：Gemini 首页实测 818 KB、YouTube 1.4 MB。为判词下整页会把「可用性检测」变成「带宽测试」，污染吞吐测量。
3. **`CHALLENGE` 的真实语义是「服务器的机器人防护在拦截非浏览器客户端」**，不等于「你的浏览器用不了」。Go 客户端必然停在质询（无 JS、无 cookie、无真实指纹），而浏览器有。
4. **不做浏览器指纹伪装**：ProxyLens 的定位是「如实反映用户日常使用代理的能力」，而用户日常用的是自己的浏览器。用「特别能过风控的客户端」去测会给出**比现实更乐观**的结论。**这是产品诚实性原则，不是技术限制。**
5. **429/5xx 绝不归因代理**：否则用户会白白换节点。实测 `M9` 的 `api.openai.com` 401 是「最好」的结果之一——网络 100% 通、服务 100% 可达、仅缺 key。

**代价（明确接受）**

1. **`CHALLENGE` 会频繁出现**（本机实测就是），用户可能觉得「这工具怎么老是警告」。缓解：黄色 + 明确文案 + 一句「请在浏览器打开确认，多数情况可以正常使用」。
2. **不伪装指纹意味着无法区分「IP 信誉差被 CF 拦」与「无 JS 被 CF 拦」**——两者都判 `CHALLENGE`。缓解：在 `evidence` 中给出 `cf-ray` / `cf-mitigated` 原值，让高级用户自行判断。
3. **body 扫描上限 64 KiB 可能漏掉后置的指纹**。缓解：先扫 header，再扫前 64 KiB；若 64 KiB 内无证据且状态码正常，判 `ok` 而非 `challenge`（宁可漏报也不误报）。
4. **不做地区推断的数据源**（如 GEOIP 数据库）→ 无法回答「我这个代理落地在哪个国家」。`00-task-brief.md` §3 已列为 out of scope，但用户会问。缓解：报告标注「未做落地地区判定」。
5. **`region_blocked` 的一票否决（总分上限 40）可能在误判时过重**。缓解：只在证据明确（451 / 明确 body 文案）时触发；`CHALLENGE` 不触发一票否决。

---

### ADR-07：YouTube 链路 —— 三级降级 + `gvt1` 代理指标，绝不因取不到分片而失败

| | |
|---|---|
| **状态** | Accepted |
| **相关** | `02-tech-research.md` §6、`M8`、`M11`、`M12`、`06-engine-design.md` D1 |

**背景**

原始设想（`00-task-brief.md` §P2）是「取 `googlevideo` 播放清单 → range 下载分片」。实测证明**在纯 Go 下不可行**：

- `M8`：`ytInitialPlayerResponse.streamingData.adaptiveFormats` 全部 26 个条目 **`url` 与 `signatureCipher` 均为空**，只有元数据。
- `M8`：`formats[0]` 有 `signatureCipher`，但被 `s=` 签名 + `n=` 变换保护。
- `M11`：不做签名变换直接下载 → **`403`，0 字节**（ABR 端点与 cipher URL 两条路径都是 403）。

**选项**

| 方案 | 可行性 | 成本 | 判定 |
|---|---|---|---|
| A. 内嵌 goja 执行 `base.js` | 技术上可行 | +5 MB 依赖；`base.js` 每 1–3 月变更 | ❌ |
| B. 移植 `n-sig` 实现 | Go 生态无可靠等价物 | 长期逆向维护 | ❌ |
| C. 调 `yt-dlp` 子进程 | 可行 | 违反「不依赖预装运行时」 | ❌ |
| **D. 三级降级 + 边缘握手 + `gvt1` 吞吐** | **完全可行** | 无额外成本 | ✅ |
| E. 完全放弃 YouTube 项 | 可行 | 丢掉一个 P0 探测项 | ❌ |

**决策**

**D：四级降级链，任何一级失败都不阻塞 Run。**

| 级 | 请求 | 实测结果（`M11`/`M12`） | 产出 |
|---|---|---|---|
| **T1 页面** | `GET https://www.youtube.com/` | `200`，955 KB，TTFB 300 ms，`server: ESF` | 页面 TTFB/TLS、`playabilityStatus.status` |
| **T2 缩略图** | `GET https://i.ytimg.com/vi/<id>/hqdefault.jpg` | `200/206`，21,011 B，TTFB 171 ms，`server: sffe` | 缩略图 TTFB（**不折算 Mbps**） |
| **T3 媒体边缘握手** | `GET https://<host from serverAbrStreamingUrl>/`（仅握手） | `404`，**`server: gvs 1.0`**，`appconnect=87.27 ms` | **到真实 `googlevideo` 边缘的 TLS 耗时** |
| **T4 边缘下行** | `GET https://redirector.gvt1.com/edgedl/go/...`（跟随 302） | `302` → `rr2---sn-*.gvt1.com`，range `206` | Google 边缘缓存下行吞吐 |

**`media_path` 枚举**：`page_only` / `thumb_only` / `googlevideo_handshake` / `gvt1_proxy_metric` / `googlevideo_real`（保留，未来） / `unavailable`

**理由**

1. **T3 的关键洞察**：即使拿不到媒体字节，`googlevideo` 边缘对裸 `GET /` 返回 `404` —— 而这个 404 已经证明 DNS 解析到真实边缘、TCP 通、TLS 握手完成（87 ms）、且 **`server: gvs 1.0` 是 "Google Video Server" 的确定性指纹**。**握手是可测的，数据不是。**
2. **T4 的关键洞察**：`redirector.gvt1.com` 302 落到 `rr2---sn-xxxx.gvt1.com` —— 与 `googlevideo` 的 `rrN---sn-` 命名规则完全一致，是**同族 Google Global Cache 边缘节点**。在此做大文件 range 下载，测的是「到 Google 边缘缓存的下行吞吐」，与 YouTube 播放**共享同一条边缘回程**。
3. **`gvt1` 无 `n-sig` 限速**，因此 T4 的数字不受 YouTube 的主动限速策略污染。
4. **绝不让「取不到分片」导致 Run 失败**：那会把「技术限制」变成「用户代理故障」的假象。
5. T2 **不折算 Mbps**：15–21 KB 的测量被 TLS 慢启动与 RTT 主导，算出的「速率」是假的，会给出荒谬的低值。

**代价（明确接受）**

1. **T4 不是 YouTube 播放速率**。缓解：报告必须写 `gvt1 代理指标` 并附脚注「未取到真实播放分片；以上为同链路代理指标，非播放实测速率」。**绝不允许标成「YouTube 播放速度」。**
2. **无法测「真实起播到首帧耗时」**（需播放器）。缓解：T1 页面 TTFB 作为「起播前门控环节」的代理量，并在 `evidence` 说明。
3. **依赖 `redirector.gvt1.com` 的公开路径**（`edgedl/go/go1.22.0-*.pkg`）。缓解：备用 `edgedl/chrome/...`；再加备用则退回 T3 的握手指标（降级不影响 Run）。
4. **`i.ytimg.com` 的 videoId 需固定**（用 `jNQXAC9IVRw`「Me at the zoo」，最老且最不可能被删的视频）。若被删，降级为 T1 单级。
5. **未覆盖 QUIC/HTTP3**（YouTube 大量走 QUIC）→ 可能低估支持 QUIC 的代理。缓解：报告脚注声明。

---

### ADR-08：前后端通道 —— 同源 HTTP + SSE（不用 WebSocket，不用 JSON-RPC）

| | |
|---|---|
| **状态** | Accepted |
| **相关** | `04-security.md` §3.8、`06-engine-design.md` §8.1 |

**背景**

需要「探测过程中流式刷新指标（不阻塞 UI）」。同时 `04-security.md` 要求：SSE 端点必须鉴权、单连接上限 4、不支持 CORS preflight。engine 已按 `GET /api/runs/{id}/events` 设计，`04-security.md` 按 `GET /api/events` 设计 —— **两者需统一**。

**选项**

| 方案 | 单向流 | 鉴权便利性 | 实现量 | 断线重连 | 判定 |
|---|---|---|---|---|---|
| A. WebSocket | 双向 | 需握手期校验，`04-security.md` 未设计 | 高（需帧处理/心跳/背压） | 需自写 | ❌ |
| **B. SSE** | 单向（够用） | cookie / `?t=`（`EventSource` 不能设自定义头） | **低（`text/event-stream` + `fmt.Fprintf`）** | **浏览器原生重连 + `Last-Event-ID`** | ✅ |
| C. 长轮询 | 单向 | 同 SSE | 中 | 需自写 | ❌ |
| D. 轮询 `/api/state` | — | 简单 | 低 | — | ❌ 200ms 轮询成本高于 SSE |

**决策**

**B：SSE。统一路由为 `GET /api/events`（采纳 `04-security.md` 的命名，engine 的 `/api/runs/{id}/events` 作为未来多 Run 扩展点保留注释）。**

**统一后的完整端点表**（与 `04-security.md` §3.8 一致）：

| 方法 | 路径 | 鉴权 | 副作用 |
|---|---|---|---|
| GET | `/?t=<token>` | token（一次性，60s 窗口） | 无 |
| GET | `/assets/*` | Host 校验（不要求 token） | 无 |
| GET | `/api/state` | token / cookie | 无 |
| POST | `/api/probe` | token + Origin + JSON | 发起外连 |
| POST | `/api/probe/cancel` | token | 取消（幂等） |
| GET | `/api/events` | cookie / `?t=` | SSE 长连接 |
| POST | `/api/export` | token + JSON | 生成报告文件 |
| POST | `/api/diagnose/tls` | token + JSON | 外连（证书诊断） |

**`/api/probe` 请求体** = `RunConfig`（§4.2）；响应 `202 Accepted` + `{"runId":"..."}`。
**并发约束**：同一时刻只允许 1 个 Run，否则 `409 Conflict`（`04-security.md` 要求）。

**SSE 事件格式**：

```
id: 42
event: probe.sample
data: {"probeId":"P1","t":1400,"mbps":4.42,"bytes":552960}

id: 43
event: probe.done
data: {"probeId":"P1","status":"ok","p50Mbps":4.31,"p90Mbps":5.42,"cv":0.184}
```

**事件类型清单**（前端必须全部处理）：

| event | 频率 | 可丢 | 说明 |
|---|---|---|---|
| `run.start` | 1 | 否 | 含 `runId`、`route`、`probes[]` |
| `probe.start` | 6 | 否 | 探测项开始 |
| `probe.sample` | ~50 | **是** | 吞吐/延迟采样点 |
| `probe.progress` | ~30 | 是 | 非速率类进度（如「第 3/10 次采样」） |
| `probe.done` | 6 | 否 | 终态 + 汇总指标 |
| `run.done` | 1 | 否 | 含完整 `Score` |
| `run.error` | 0–1 | 否 | 致命错误 |
| `run.cancelled` | 0–1 | 否 | 用户取消 |

**理由**

1. **数据流是单向的**（引擎 → UI）。双向 WebSocket 是过度设计，且需自己实现心跳/重连/背压，代码量与 bug 面都大。
2. **`EventSource` 原生支持断线重连**（3 s 后自动重试）与 `Last-Event-ID` 补发 —— 用户误触刷新/切标签页后自动恢复，零代码。
3. **`text/event-stream` 用 `fmt.Fprintf` 就能实现**，无需帧解析、无需第三方库。
4. **不响应 OPTIONS / preflight**（`04-security.md` §3.2）：同源请求根本不产生 preflight，这本身就是「不用 CORS」的收益。
5. **200 ms 节流在服务端做**（EventBus），SSE 每连接每秒只推 5 条 —— 即使 4 个连接也只有 20 msg/s。

**代价（明确接受）**

1. **`EventSource` 不能设置自定义请求头** → 无法用 `X-ProxyLens-Token`。必须用 `pl_sess` cookie（`SameSite=Strict` + `HttpOnly`）+ `?t=` 兜底。这是 `04-security.md` 已明确的妥协。
2. **SSE 是单向的** → 取消操作必须另开 `POST /api/probe/cancel`（多一次往返，可接受）。
3. **HTTP/1.1 下浏览器对同源并发连接数限制为 6** → SSE 占 1 个，剩余 5 个够用。但若用户开 4 个标签页（4 个 SSE），剩余连接只剩 2 个，可能影响 `/api/probe` 与 `/api/export`。缓解：SSE 单连接上限 4（`04-security.md` 已定），并在 UI 提示「已在其他标签页打开」。
4. **代理/中间设备可能缓冲 SSE** → 必须设 `X-Accel-Buffering: no`、`Cache-Control: no-cache`、`Connection: keep-alive`，并**立即 flush 一次**（发送 `:ok\n\n` 注释行）让浏览器进入流模式。
5. **`Last-Event-ID` 补发需要事件缓冲**（内存保留最近 N 条）。会引入内存占用与「补发多少」的策略问题。缓解：只保留最近 200 条事件；超出则发 `run.snapshot`（完整状态快照）而非逐条补发。

---

### ADR-09：测量诚实性 —— 「不可得」必须为 `null`，禁止编造 0 值

| | |
|---|---|
| **状态** | Accepted |
| **相关** | `02-tech-research.md` §2.6、§5、`M2`、`M9` |

**背景**

`M9` 揭示了三个会导致**输出假数据**的结构性问题：

1. 走 HTTP 代理时 `httptrace` 的 `DNSStart` **根本不触发** → `dns_ms` 若填 0，用户读成「DNS 快到 0ms」。
2. `tcp_ms` 测的是到**本地代理端口**或 **TUN fake-IP** 的连接（0.22–0.80 ms），不是远端 RTT。
3. TUN 下 `remote_ip` 是 `198.18.x.x`（fake-IP），若显示为「出口 IP」就是**彻头彻尾的假话**。

**这三个问题不是「精度不够」，是「数据性质不同」。** 用 0 填充等于编造。

**选项**

| 方案 | 结果 | 判定 |
|---|---|---|
| A. 填 0 | 简洁 | ❌ 编造数据 |
| B. 省略字段 | 前端需处理 `undefined` | 可用但弱类型 |
| **C. 显式 `null` + `evidence` 说明** | **类型明确 + 可解释** | ✅ |
| D. 回退到 TCP 层自测 RTT（如 ICMP/tcping） | 引入 H2 问题：TUN 下 ICMP 也被接管 | ❌ |

**决策**

```
规则 H1：dns_ms 在「走 HTTP 代理」或「TUN fake-IP」时必须为 null。
规则 H2：tcp_ms 字段的 UI 标签固定为「本地代理建连」，禁止标「TCP 建连」或「RTT」。
规则 H3：remote_ip ∈ 198.18.0.0/15 → 显示「fake-IP（TUN 接管）」，禁止显示为出口 IP。
规则 H4：connReused == true → DNS/TCP/TLS 三段前端显示 "—"，不显示 "0.0 ms"。
规则 H5：唯一可作「延迟」主指标的端到端量是 tls_ms；无 tls_ms 时退到 ttfb_ms。
规则 H6：任何 probe 的 evidence 数组必须至少含 1 条说明「本次测量口径」。
规则 H7：报告必须有 caveats 区，且不可折叠。
规则 H8：ProxyLens 不做 GEOIP → 禁止推测或展示「落地国家/地区」。
```

**理由**

1. **产品的核心价值是「可信」**。一个给出 0.3 ms RTT 的工具，用户一眼就知道在骗人，整份报告都会失去信任。
2. **`198.18.0.0/15` 是 RFC 2544 保留网段**（实测 `M2` 命中 `.69/.70/.93`），是 Clash/Mihomo/Surge 的 `fake-ip` 池默认范围。检测它成本极低（一个 CIDR 判断），收益是「不说假话」。
3. **`tls_ms` 是唯一端到端可归因的量**（实测 `M9`：三条路径均为 94–169 ms，与环境无关）。因为 TLS 握手必须与目标服务器完成 1–2 个 RTT，`CONNECT` 隧道只是透明转发。
4. **`null` 在 JSON 与前端都有明确语义**，而 `0` 会被误读为「极快」。

**代价（明确接受）**

1. **UI 会更「空」**：延迟卡片可能只有一项有值。用户可能觉得「信息太少了」。缓解：明确的 tooltip（「真实远端建连由代理完成，本工具无法分离」）+ `evidence` 说明。
2. **`null` 增加了前端的判空负担**（每个数值字段都要处理 `null`）。缓解：`core/format.js` 统一提供 `fmtMs(v)` → `v == null ? '—' : ...`。
3. **`fake-IP` 检测需要拿到解析结果**，而走 HTTP 代理时拿不到（DNS 阶段不触发）→ 此时无法判断是否 TUN。缓解：用 `route.kind` 兜底（`netx` 已能区分 `tun`/`os_proxy`/`direct`）。
4. **不做 GEOIP 会让用户失望**（「我想知道这个节点在哪」）。缓解：`00-task-brief.md` §3 已列 out of scope，在 `notice.js` 提示「本工具不判定落地地区」。
5. **`H6` 的「每条 evidence 至少 1 条口径说明」增加了实现量** —— 每个 collector 都要写。缓解：`probe.Env` 提供 `DescribeRoute()` 统一生成。

<!-- APPEND-HERE -->
