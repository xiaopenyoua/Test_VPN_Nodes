# 09 · 测试方案与用例（Test Plan & Cases）

| 项目 | 内容 |
|---|---|
| 被测对象 | ProxyLens 1.0.0（Go 1.26.3，`app/`，单二进制 6.6 MB，darwin/arm64 实测） |
| 文档定位 | 交付清单第 9 项；把 `05-prd.md` 的验收硬指标、`06-engine-design.md` 的设计口径、`04-security.md` 的安全基线，落成**可执行、可判定、可回归**的用例集 |
| 上游依据 | `00-task-brief.md`（需求基线）、`04-security.md`（S-33/34/35、H9/H12）、`05-prd.md`（§3.1 状态词表 / §3.2 五分档 / 第 7 章伪造判定）、`06-engine-design.md`（§13 单测要点、§15.3 自检清单） |
| 下游依据 | `10-acceptance-report.md`（本轮实测数字，本文直接引用，不重测、不改数） |
| 测试方式 | 代码审阅 + 单元/表驱动 + 本地假服务器（`httptest`）+ 端到端 HTTP/SSE + 真实链路人工对照 + 四平台编译 |
| 用例总数 | **211 条**（P0 = **117**，P1 = **82**，P2 = **12**），详见 §10 |
| 结论摘要 | 现有自动化测试仅 **5 条**（`internal/score`，覆盖率 80.6%），其余 6 个包 **0 测试**。本轮新增 **16 个缺陷**（P0 六个、P1 八个、前置门禁两个），其中 3 个会导致用户误判（§9-N1/N11/N12），必须先修 |

---

## 0. 怎么读这份文档

### 0.1 三层「真相」与冲突处理规则

本项目的特殊性在于：**设计文档、PRD 词表、已实现代码三者不完全一致**（见 `06-engine-design.md` §16 修订记录）。测试必须明确「以谁为准」，否则用例之间会自相矛盾。

| 层 | 文件 | 在测试中的角色 |
|---|---|---|
| **验收准绳** | `05-prd.md` | 状态词表（十态）、五分档边界、业务硬指标。**用户可见的结论**以此判定 |
| **设计口径** | `06-engine-design.md` | 采集点、算法、参数、事件协议。用于发现「实现偏离设计」 |
| **回归基线** | `app/**` 已跑通代码 | 现有行为快照。改代码前先让本文用例绿，再改设计 |

**冲突处理三条规则（本文所有用例遵守）**：

1. **用户可见的结论**（状态、档位、裁决、封顶原因）→ 以 PRD 为准。代码与 PRD 不一致时记为**缺陷**，不修改期望值来迁就代码。
2. **数值口径**（断点、权重、百分位算法）→ 以 `06` §14 参数总表为唯一权威。代码与 06 不一致时，用例先断言**当前代码值**并标注 `[DIVERGE]`，同时给出 06 期望值，避免「改了算法没改测试」。
3. **未实现的字段**（`blame`/`subReason`/`expectedProbeCount`/`probe.stage`/`configMode`/`egressKind`）→ 用例**保留**并标 `[PENDING-IMPL]`，作为「实现清单」而非「测试失败」，防止能力缺口被静默遗忘。

### 0.2 用例编号与优先级

```
TC-<域>-<两位序号>
域：ST 状态判定 | CL Classify 分支 | GATE 闸门 | SC 评分纯函数 | DL 下载吞吐
    P6 稳定性 | STAB 稳定性算法 | NET 出网与代理 | ENG 编排调度
    SSE 事件流 | API 本地服务 | SEC 安全 | PLAT 跨平台 | PERF 性能 | MAN 人工对照
优先级：P0 阻断发布 | P1 影响可信度/观感 | P2 打磨
```

**P0 的判定标准**（三条任一即 P0）：会输出**与事实相反**的结论（假绿/假红）；会泄露凭据或响应体；会让进程崩溃或卡死。

### 0.3 本轮已实测事实（直接引用，不再重测）

| # | 事实 | 来源 |
|---|---|---|
| F1 | 四平台交叉编译全部通过：windows/amd64 7.2 MB、windows/arm64 6.5 MB、darwin/arm64 6.6 MB、darwin/amd64 7.1 MB | `10` §5.3 |
| F2 | 三档实测：quick **7.86 s**、standard **19.9 s**、deep **34.0 s** | `10` §5.1 |
| F3 | P1 吞吐多轮：31.2 / 36.2 / 42.5 / 40.9 Mbps；deep 38.1 Mbps、P50 38.8、P90 69.56、峰值 129.0、CV 0.626 | `10` §5.2 |
| F4 | 人工对照：ChatGPT **完全可用**（已登录、历史会话正常）、Gemini **完全可用**（已登录）、OpenAI API 返回 `{"error":{"message":"Missing bearer authentication in header","type":"invalid_request_error"}}` | `10` §4.2 |
| F5 | 环境：macOS TUN 模式，`remoteAddr=198.18.0.70:443`；shell 有 `HTTP_PROXY`，系统模式**刻意忽略**并 warning | `10` §2 |
| F6 | `go test ./...` 现状：仅 `internal/score` 有测试（5 条全绿），覆盖率 **80.6%**；其余 6 包 `[no test files]` | 本轮实跑 |
| F7 | 本地服务实测：`/api/health` 无令牌 **200**；`/api/state` 无/错令牌 **401**；`Host: evil.example.com` **403**；`Origin: https://127.0.0.1:18791.evil.com` **200（应 403）**；`GET /api/run` **405**；坏 JSON **400**；`OPTIONS /api/state` **200（应 405）** | 本轮实跑 |
| F8 | SSE 实际事件名：`snapshot / run_start / probe_start / sample / probe_result / score / run_end / error`（下划线风格，非 06 §8 的点分风格） | 代码审阅 `web/js/core/api.js` |

> **纪律**：本文所有期望值都可手算复核；凡是引用实测数字，均标注来源，不臆造新数字。凡标注「静态确认」的结论来自代码审阅（未跑），执行时需复现。

---

## 1. 测试分层与策略

### 1.1 五层结构

| 层 | 名称 | 被测内容 | 技术手段 | 用例数 | 进 CI |
|---|---|---|---|---|---|
| **L1** | 纯函数层 | `SpeedScore`/`StabilityScore`/`statusScore`/`gateOf`/`grade`/`Percentile`/`Mean`/`StdDev`/`Round`/`parseScutil`/`maskURL`/`AttributionOf` | 表驱动，无网络、无时钟 | 45（`SC` 13 + `GATE` 22 + `AT` 10） | ✅ |
| **L2** | 判定组件层 | `Classify(id, *FetchResult)`、`RunYouTube` 分支、判定优先级 | 构造 `FetchResult` 结构体（**不发起请求**）+ `httptest.NewServer` | 42（`ST` 31 + `CL` 6 + `P2` 5） | ✅ |
| **L3** | 编排与出网层 | `engine.execute` 波次/取消/补位、`netx.Resolve`/`NewTransport`/`socks5Dial`/`Dialer.Control` | 注入假 `RoundTripper`、假 SOCKS5/HTTP 代理 | 40（`ENG` 13 + `NET` 27） | ✅ |
| **L4** | 端到端 | `POST /api/run` → SSE 事件序列 → `GET /api/report`；本地服务攻击面 | 真起二进制 + `curl`/`EventSource` | 47（`SSE` 17 + `API` 15 + `SEC` 15） | ⚠️ 部分 |
| **L5** | 真实链路与人 | 结论真实性、跨平台、性能 | 人工浏览器对照 + 真机 | 37（`MAN` 16 + `PLAT` 11 + `PERF` 10） | ❌ 发布前手工 |

**分层原则**：

- **L1/L2 是唯一可以「断言精确数值」的层**。网络层只断言区间与关系（`p90 ≥ p50`、`bytes ≤ 预算×105%`），否则会在 CI 里随机红。
- **L2 优先于 L3**：判定逻辑（十态）是本产品价值核心，必须能在无网络环境下 100% 复现，**不得依赖真实站点**（真实站点会变、会限流、会让人以为代码坏了）。
- **L4 只在本地假栈上跑**，禁止在 CI 里打真实外网（`06` §13.5 明确要求）。

### 1.2 现状盘点（必须先说清缺口）

`go test ./...` 实测输出（F6）：

```
?   	proxylens	                  [no test files]
?   	proxylens/internal/config	  [no test files]
?   	proxylens/internal/engine	  [no test files]
?   	proxylens/internal/netx	    [no test files]
?   	proxylens/internal/probe	    [no test files]
ok  	proxylens/internal/score	  80.6% coverage
?   	proxylens/internal/server	  [no test files]
```

| 包 | 关键函数 | 测试状态 | 风险 |
|---|---|---|---|
| `internal/probe` | `Classify`（十态判定核心）、`Fetch`（httptrace 采集）、`RunDownload`、`RunBaseline`、`SpeedScore` | ❌ 无 | **最高**：核心判定完全无回归 |
| `internal/score` | `Aggregate`、`gateOf`、`verdict`、`advice` | ✅ 5 条 | 闸门主路径已钉死，边界与文案未覆盖 |
| `internal/engine` | `execute` 波次、`Cancel`、补位 | ❌ 无 | 高：取消/并发是用户高频操作 |
| `internal/netx` | `Resolve`、`NewTransport`、`socks5Dial`、`parseScutil` | ❌ 无 | 高：代理模式错判=测错对象 |
| `internal/server` | `guard`、`hostAllowed`、SSE | ❌ 无 | 高：本地服务攻击面（已发现 2 处绕过） |
| `internal/config` | `Normalize`、`Validate` | ❌ 无 | 中：参数收敛靠读代码，无断言 |

**结论**：自动化覆盖面目前只保住了一条线（闸门），判定链与网络层全靠人工验收。这也是本轮验收能发现 3 个 P0（`10` §6）却无法防止回归的根本原因。

### 1.3 必须 mock 的边界（沿用 `06` §13.1，补实测约束）

| 边界 | 做法 | 本轮代码现状 | 结论 |
|---|---|---|---|
| **时钟** | `fakeClock` 手动推进；**禁止** `time.Sleep` 等真实时间 | 代码里大量直接 `time.Now()`/`time.Since()`（`download.go`、`site.go`、`engine.go`），**无 `Clock` 接口** | `[PENDING-IMPL]` 时间相关用例只能断言区间；`TC-PERF-*` 走 L5 |
| **随机** | 固定 seed | `download.go:129` 用 `rand.Intn(1e6)` 做 cache-buster，未固定 seed | 只断言「不同请求 URL 不同」，不断言具体值 |
| **网络对端** | `httptest` 起假站点，**不 mock trace** | `Fetch` 接受 `*http.Transport`，**可直接注入** → L2/L3 可行 ✅ | 无需改造即可测 |

> 好消息：`probe.Fetch(ctx, tr, opts)` 的签名已把 Transport 作为参数传入，L2/L3 用例**不需要改产品代码**就能构造假栈。这是本文所有 L2 用例成立的前提。

### 1.4 CI 门槛建议

```sh
go vet ./...
go test ./... -race -count=1
go test ./... -coverprofile=cover.out
# 安全硬约束：InsecureSkipVerify 只允许 tls_diagnose.go 一处（security H9 / S-20）
test "$(grep -rn 'InsecureSkipVerify' --include=*.go . | wc -l)" -le 1
# 禁止把响应体写进日志（S-33）
! grep -rn 'body: %s\|body=%s' --include=*.go . | grep -v _test.go
```

| 门槛 | 目标 | 现状 |
|---|---|---|
| L1+L2 行覆盖率 | `internal/probe` ≥ 70%，`internal/score` ≥ 85% | probe 0%，score 80.6% |
| `-race` | 全绿（尤其 `engine.execute` 的 `results` 并发 append） | 未跑过 |
| 单测总耗时 | < 3 s（L1+L2），不含 L5 | — |
| 真实外网请求 | CI 中 **0 次** | — |

---

## 2. 表驱动用例表：状态判定全覆盖

### 2.1 两套状态名必须先对齐（否则用例无法判定）

产品侧（PRD 十态）与代码侧（`probe.Status` 11 个终态）**不是一一对应**。这是最容易踩的陷阱：测试断言必须用**代码值**，验收报告必须用 **PRD 值**。

| # | 代码 `Status` | 常量 | PRD wire value | 中文展示（`statusWord`） | 是否可用 |
|---|---|---|---|---|---|
| 1 | `ok` | `StatusOK` | `PASS` | 可用 | ✅ |
| 2 | `auth_required` | `StatusAuthRequired` | `PASS_WITH_NOTE` | 可达（需 Key） | ✅ |
| 3 | `degraded` | `StatusDegraded` | `SLOW` / `PASS_WITH_NOTE` | 受限 | ⚠️ |
| 4 | `inconclusive` | `StatusInconclusive` | `NET_FAIL` + `diagnosis=INCONCLUSIVE` | 无法确认 | ❌ |
| 5 | `rate_limited` | `StatusRateLimited` | `RATE_LIMITED` | 限流（网络通） | ⚠️ |
| 6 | `challenge` | `StatusChallenge` | `CHALLENGED` | 被风控 | ❌ |
| 7 | `region_blocked` | `StatusRegionBlock` | `BLOCKED_BY_POLICY` | 地区受限 | ❌ |
| 8 | `hijacked` | `StatusHijacked` | `HIJACKED`（PRD v1.1 第十态） | 疑似劫持 | ❌ |
| 9 | `failed` | `StatusFailed` | `NET_FAIL` / `TIMEOUT` | 不可达 | ❌ |
| 10 | `skipped` | `StatusSkipped` | `SKIPPED`（**不得按 0 分计入总分**） | 未执行 | — |
| 11 | `cancelled` | `StatusCancelled` | `SKIPPED` 子类（`skipReason=cancelled`） | 已取消 | — |
| — | `pending`/`running` | — | 无（过程态） | — | — |

**三处真实偏差（不是笔误，必须写成用例）**：

| 偏差 | 代码现状 | PRD / 06 要求 | 用例 | 级别 |
|---|---|---|---|---|
| `TIMEOUT` 无独立态 | 超时 → `failed`（`classifyError`） | PRD 要求 `SLOW`/`TIMEOUT`/`NET_FAIL` 三者可区分 | `TC-ST-19` | **P0** |
| `INCONCLUSIVE` 有独立态 | `inconclusive` 独立 | 06 §5.10 要求 `NET_FAIL` + `diagnosis=INCONCLUSIVE`（宁红勿绿） | `TC-ST-20` | P1 |
| `SKIPPED` 计 0 分 | `statusScore(skipped)=0`，`gateOf` 不剔除 | PRD：`SKIPPED` **不得按 0 分参与总分** | `TC-GATE-11` | **P0** |

### 2.2 主表：`Classify(id, *FetchResult)` 的 30 条判定用例

> 构造方式两种都要有：① **直接构造 `FetchResult`**（L2 最快，测逻辑分支）；② `httptest.NewServer` 返回真实响应（测 header 大小写、chunked、真实 `Content-Length`）。
> `S` = 期望 `Status`；`A` = 期望 `Attribution`；`E` = `Evidence` 必须包含的片段。

| 用例 | id | 输入（status / header / body） | S | 分值 | A | E 必须含 | 级 |
|---|---|---|---|---|---|---|---|
| `TC-ST-01` | P3 | 200 / 正常 / `<title>ChatGPT</title>…chatgpt…` | `ok` | 100 | `""` | `HTTP 200` | P0 |
| `TC-ST-02` | P4 | 200 / 正常 / `<title>Gemini</title>…gemini…` | `ok` | 100 | `""` | 同上 | P0 |
| `TC-ST-03` | P5 | 401 / `{"error":{"message":"Missing bearer authentication in header"}}` | `auth_required` | 92 | `""` | `401 说明已抵达 OpenAI 服务端` | **P0**（F4 真实场景） |
| `TC-ST-04` | P5 | 200 / json / `{"error":{"code":"unsupported_country"}}` | `region_blocked` | 10 | `""` | `命中地区限制特征` | **P0** |
| `TC-ST-05` | P5 | **200** / `text/html` / `<html>portal</html>` | `ok`（**缺陷**） | 100 | `""` | — | **P0**（S-34：200 不得判 PASS） |
| `TC-ST-06` | P3 | 403 / `cf-mitigated: challenge` / body 为**正常页** | `challenge` | 35 | `target` | `响应头 cf-mitigated` | **P0**（header 优先于 body） |
| `TC-ST-07` | P3 | 403 / 无头 / `<title>Just a moment...</title>` | `challenge` | 35 | `target` | `命中人机验证特征` | P0 |
| `TC-ST-08` | P3 | 503 / 无头 / `Enable JavaScript and cookies to continue` | `challenge` | 35 | `target` | 同上 | P0 |
| `TC-ST-09` | P4 | 200 / 无头 / `…Gemini isn't available in your country` | `region_blocked` | 10 | `""` | `命中地区限制特征` | **P0**（护栏：地区先于 200） |
| `TC-ST-10` | P4 | 451 / 正常 body | `region_blocked` | 10 | `""` | — | P1 |
| `TC-ST-11` | P3 | 429 / `Too Many Requests` | `rate_limited` | 70 | `""` | `限流（429）：网络是通的` | **P0**（429 必须与「慢」分离） |
| `TC-ST-12` | P5 | 429 / `{"error":{"code":"insufficient_quota"}}` | `rate_limited`（**待裁定**） | 70 | `""` | — | P1（§11-Q2） |
| `TC-ST-13` | P3 | 403 / body 含 `unsupported_country` | `region_blocked` | 10 | `""` | — | P1 |
| `TC-ST-14` | P3 | 403 / 普通 body | `degraded` | 55 | `target` | `被目标站点拒绝（403）` | P1 |
| `TC-ST-15` | P3 | 502 | `degraded` | 55 | `target` | `网关异常` | P2 |
| `TC-ST-16` | P3 | 200 / body 里**无** `chatgpt` | `inconclusive` | 60 | `undetermined` | `未命中 ChatGPT 特征` | **P0**（宁红勿绿） |
| `TC-ST-17` | P4 | 200 / body 里**无** `gemini` | `inconclusive` | 60 | `undetermined` | `未命中 Gemini 特征` | P0 |
| `TC-ST-18` | P6 | 204 | `ok` | — | `""` | `网络通` | P0 |
| `TC-ST-19` | P3 | `Err = context.DeadlineExceeded` | `failed`（**应 `TIMEOUT`**） | 0 | `undetermined` | `请求超时` | **P0**（§2.1 偏差表） |
| `TC-ST-20` | P3 | 200 / body 空 | `inconclusive` | 60 | `undetermined` | — | P1 |
| `TC-ST-21` | P3 | `Err = &x509.UnknownAuthorityError{}` | `hijacked` | 5 | `proxy` | `TLS 证书校验失败` | **P0**（H9 绝不降级） |
| `TC-ST-22` | P3 | `Err` 含 `tls: handshake failure` | `hijacked` | 5 | `proxy` | 同上 | P0 |
| `TC-ST-23` | P3 | `Err` 含 `connection reset` | `failed` | 0 | `undetermined` | `连接被重置` | P1 |
| `TC-ST-24` | P3 | `Err` 含 `no such host` | `failed` | 0 | `proxy` | `DNS 解析失败` | P1 |
| `TC-ST-25` | P3 | `f == nil` | `failed` | 0 | `undetermined` | `无响应` | P2 |
| `TC-ST-26` | P3 | 200 / `<title>您访问的网站已被屏蔽</title>` | `hijacked` | 5 | `undetermined` | `命中劫持特征` | P0 |
| `TC-ST-27` | P3 | 200 / `Declared=818196` / `BodyLen=204800`（**采样上限截断**） | `hijacked`（**误判**） | 5 | `undetermined` | `响应体被截断` | **P0**（§9-N1） |
| `TC-ST-28` | P3 | 200 / `Declared=100000` / `BodyLen=1000`（**真截断**） | `hijacked` | 5 | `undetermined` | `响应体被截断` | P0（必须抓） |
| `TC-ST-29` | P2 | 200 / 正常首页 | `ok` | — | `""` | `YouTube 首页可达` | P2 |
| `TC-ST-30` | P3 | 200 / 经 **3** 次重定向 | `ok` + `len(Redirects)=3` | — | — | `重定向 3 次` | P1 |
| `TC-ST-31` | P3 | 200 / 经 **4** 次重定向 | `failed`（`重定向次数过多`），**不得**继续跟随 | — | — | `重定向次数过多` | **P0**（`MaxRedirects=3` 必须封口，防重定向炸弹） |

**`TC-ST-27` / `TC-ST-28` 必须成对**：它们卡在同一个 `if` 的两侧。当前实现用 `BodyLen < Declared/2` 判截断，而 `Fetch` 用 `io.LimitReader(resp.Body, maxBody)` **主动**截断到采样上限——采样上限与截断判定互相污染。修法（供开发参考，本文不改代码）：

```go
// Fetch 中区分「读满采样上限」与「对端提前断流」
hitCap := err == nil && res.Declared > int64(len(body)) && int64(len(body)) == maxBody
res.Truncated = err == nil && res.Declared > 0 && int64(len(body)) < res.Declared && !hitCap
```

### 2.3 判定优先级用例（分支顺序本身就是业务规则）

`Classify` 里 `if` 的**先后次序**就是产品规则。必须用用例锁死，否则以后「整理代码」挪一行就全废：

| 用例 | 冲突场景 | 正确结论 | 顺序错会得到 | 级 |
|---|---|---|---|---|
| `TC-CL-01` | `cf-mitigated: challenge` + body 完全正常（含 `chatgpt`） | `challenge` | `ok`（**假绿**） | **P0** |
| `TC-CL-02` | 劫持特征 + `cf-mitigated` 同时存在 | `hijacked`（安全优先） | `challenge` | P0 |
| `TC-CL-03` | 地区特征 + 200 + `gemini` 字样 | `region_blocked` | `ok`（**假绿**） | **P0** |
| `TC-CL-04` | 截断 + 地区特征同时命中 | `hijacked` | `region_blocked` | P1 |
| `TC-CL-05` | 401 + 挑战页 body | `challenge` | `auth_required`（**假绿**） | **P0** |
| `TC-CL-06` | 429 + 挑战页 body | `challenge` | `rate_limited`（**假绿**） | P0 |

> 这 6 条是「挪错一个 `if` 就全部失效」的高危区。`TC-CL-01/03/05` 建议同时做成 golden 快照（`testdata/verdict/*.json`），防止规则漂移（`06` §13.3）。

### 2.4 归因用例 `AttributionOf` / `NextStep`（10 条）

| 用例 | 输入 | 期望归因 | `NextStep` 关键短语 | 级 |
|---|---|---|---|---|
| `TC-AT-01` | `ok` | `""` | `""` | P0 |
| `TC-AT-02` | `auth_required` | `""` | `""` | P0 |
| `TC-AT-03` | `challenge` + `err=nil` | `target` | `不代表你的网络坏了` | **P0**（不得让用户背锅） |
| `TC-AT-04` | `region_blocked` | `target` | 同上 | P0 |
| `TC-AT-05` | `degraded` | `target` | 同上 | P1 |
| `TC-AT-06` | `inconclusive` | `undetermined` | `切「直连」再跑一次对照` | P0 |
| `TC-AT-07` | `hijacked` + `err=nil` | `undetermined` | 同上 | P1 |
| `TC-AT-08` | `failed` + `x509` 错误 | `proxy` | `换一个节点重测` | **P0**（证书=代理侧，指向正确） |
| `TC-AT-09` | `failed` + `timeout` | `undetermined` | `切「直连」…` | P1（超时无法单次归因，**不得**强行判 proxy） |
| `TC-AT-10` | 任意非空归因 | — | 非空且**不含英文错误码** | P1（前端直接展示，禁术语） |

### 2.5 `statusScore` 分值表（L1 精确断言）

| 用例 | Status | 期望分值 | 备注 |
|---|---|---|---|
| `TC-SC-01` | `ok` | 100 | 全表见 `probe/score.go` |
| `TC-SC-02` | `auth_required` | 92 | 401「可达」略低于可用 |
| `TC-SC-03` | `degraded` | 55 | 与 `inconclusive`(60) 的排序见 §11-Q3 |
| `TC-SC-04` | `inconclusive` | 60 | 高于 degraded 是反直觉点，需产品确认 |
| `TC-SC-05` | `rate_limited` | 70 | pm 要求 429 独立，分值可自定 |
| `TC-SC-06` | `challenge` | 35 | |
| `TC-SC-07` | `region_blocked` | 10 | |
| `TC-SC-08` | `hijacked` | 5 | 最低（安全事件） |
| `TC-SC-09` | `failed` | 0 | |
| `TC-SC-10` | `cancelled` | 0 | 但**不得**进分母（`TC-GATE-11`） |
| `TC-SC-11` | `skipped` | 0 | 同上 |
| `TC-SC-12` | `pending`/未知值 | 0 | 默认分支必须存在，防新增枚举漏配 |
| `TC-SC-13` | **权重口径** | `total = speed×0.40 + stab×0.30 + ai×0.30` | 与报告脚注「速度30%+稳定性30%+AI40%」**直接矛盾**（§9-N7），必须二选一并同步文案 | P1 |

---

## 3. 闸门回归用例（最关键，优先级最高）

### 3.1 这一节要钉死的那个 bug

> `01-product-research.md` / `ux-researcher` 给出的标准复现用例：
> **地区拦截 + 链路优秀（0 丢包、p50 很高、RTT 优秀）→ 错误实现输出「综合分 92，优秀」，正确实现必须输出「受限：可用性层被地区策略拒绝（链路本身良好）」。**
> 业务可用性不是权重项（weight），是门槛项（gate）。

这个 bug 的现实版本在本轮真实发生过（`10` §6-D2）：ChatGPT 被 Cloudflare 拦截，旧 `stateOf()` 只看 `aiUsable>=2 && total>=70`，于是输出「AI 可用 88 分」绿色结论。修复方式是 `gateOf()` 闸门 + 双重封顶（BLOCKED→29 / RESTRICTED→69）。

**因此本节不仅是「测试」，它是产品红线**：任何人重构评分都必须让本节全绿才能合入。

### 3.2 已有 5 条测试的真实断言（逐条摘录，不可弱化）

> 编号映射：`TestUsableWhenAllGood`=`TC-GATE-01`、`TestGateCapsWhenChatGPTChallenged`=`TC-GATE-02`、`TestGateCapsWhenBothAIBlocked`=`TC-GATE-03`、`TestHijackCapsTotal`=`TC-GATE-04`、`TestMaskingNotLeaking`=`TC-GATE-05`（内容为空，等同未测）。

| # | 测试函数 | 构造 | 断言（原文语义） | 覆盖的规则 |
|---|---|---|---|---|
| 1 | `TestUsableWhenAllGood` | P1 ok/90Mbps、P2 ok/40、P3 ok、P4 ok、P5 auth、P6 ok/100%/5ms | `s.State == "usable"` 且 `s.Total >= 85` | 全绿放行，且**不得**因闸门误压分 |
| 2 | `TestGateCapsWhenChatGPTChallenged` | 同上，把 P3 改成 `challenge` | `State == "limited"`、`Total <= 69`、`len(Caps) > 0` | **CAP-2 主回归**：被风控不得 usable、必须留痕 |
| 3 | `TestGateCapsWhenBothAIBlocked` | P3/P4 `region_blocked`、P5 `failed` | `State == "blocked"`、`Total <= 29` | **CAP-1**：三项全灭必须最低档 |
| 4 | `TestHijackCapsTotal` | P3 `hijacked` | `Total <= 55` | 证书异常的硬封顶 |
| 5 | `TestMaskingNotLeaking` | 全绿 | 仅遍历 `Dimensions`（**空断言，恒真**） | ⚠️ **形同虚设**，见 `TC-GATE-16` |

**手算核对**（可直接复核，防「断言写松了也能过」）：

```
案例 2（P3=challenge）：
  speed  = max(SpeedScore(90), SpeedScore(40)) = max(99, 92) = 99
  stab   = StabilityScore(100, 5) = 100
  ai     = (35×40 + 100×35 + 92×25) / 100 = 72 → CAP 同款封顶 60
  total  = 99×0.40 + 100×0.30 + 60×0.30 = 87.6 → gate=limited → min(87.6, 69) = 69
案例 3（P3/P4 region、P5 failed）：
  ai     = (10×40 + 10×35 + 0×25) / 100 = 7.5
  total  = 39.6 + 30 + 2.25 = 71.85 → gate=blocked → min(71.85, 29) = 29
案例 4（P3=hijacked）：
  ai     = (5×40 + 100×35 + 92×25) / 100 = 60（未超 60，cap 不触发）
  total  = 39.6 + 30 + 18 = 87.6 → 证书封顶 55 → gate=limited → 69 不生效 → 55
```

> 三条手算与断言一致（≤69 / ≤29 / ≤55）。**注意案例 2 的 87.6 与案例 4 的 87.6 相同但结论不同**（69 vs 55），这正是「多个 cap 叠加顺序」必须被测试锁住的原因。

### 3.3 闸门判定矩阵（3 项 AI 状态的完整组合）

`gateOf` 的输入只有 P3/P4/P5 的状态，因此可以穷举。下表是**期望行为**（`✅` 现有实现一致，`⚠️` 现有实现如此但需产品裁定，`❌` 现有实现错误）：

| P3 ChatGPT | P4 Gemini | P5 OpenAI API | 期望 State | 现有实现 | 判定 |
|---|---|---|---|---|---|
| ok | ok | ok | `usable` | usable | ✅ |
| ok | ok | auth | `usable` | usable | ✅（F4 真实场景） |
| ok | ok | failed | `usable` | usable | ⚠️ 两个网页可用、API 不可达，判「可用」可接受 |
| ok | region | auth | `limited` | limited | ✅ |
| ok | region | ok | `limited` | limited | ✅ |
| **degraded(403)** | ok | ok | `limited` | **usable** | ❌ **P0**：`p3Blocked` 不含 `degraded`（§9-N11） |
| **rate_limited** | ok | auth | `limited` | **usable** | ❌ **P0**：429 不阻断闸门（§9-N12） |
| **inconclusive** | ok | ok | `limited` | limited | ✅（`p3Blocked` 含它，宁红勿绿） |
| challenge | ok | ok | `limited` | limited | ✅ |
| hijacked | ok | ok | `limited` | limited | ✅ |
| failed | auth | auth | `limited` | limited | ✅（API 可达但网页灭） |
| region | region | failed | `blocked` | blocked | ✅ |
| failed | failed | failed | `blocked` | blocked | ✅ |
| skipped | ok | ok | `usable` | usable | ⚠️ 未执行≠不可用（见 `TC-GATE-11`） |
| cancelled | cancelled | cancelled | `blocked` | **blocked** | ❌ 取消后被判「AI 不可用」，应给「已取消，不评级」 |

**两个新增 P0 的说明**：

- **`TC-GATE-06`（degraded 不阻断）**：`p3Blocked` 的 switch 只列了 `Challenge / RegionBlock / Hijacked / Failed / Inconclusive`，**漏了 `Degraded`**。而 `Classify` 对「403 无地区特征」正是返回 `degraded` —— 即真实浏览器能打开、但裸请求被 403 的站点，会走这条路径拿到「可用」。这与「宁可误报受限，不可漏报可用」的方向相反。
- **`TC-GATE-07`（rate_limited 不阻断）**：`aiUsable` 只数 `ok`/`auth_required`，但 `p3Blocked` 不含 `rate_limited`。P3=429、P4/P5 正常时 `aiUsable=2` 且不阻断 → `usable`。而 PRD 明确「`RATE_LIMITED` 计 0 分」，两者语义打架。

### 3.4 待补用例 `TC-GATE-06` ~ `TC-GATE-18`

| 用例 | 目的 | 构造 / 动作 | 断言 | 级 |
|---|---|---|---|---|
| `TC-GATE-06` | degraded 必须阻断 | 全绿基线 + `P3.Status = degraded` | `State == "limited"` 且 `Total <= 69` | **P0** |
| `TC-GATE-07` | rate_limited 必须阻断 | 全绿基线 + `P3.Status = rate_limited` | `State == "limited"` | **P0** |
| `TC-GATE-08` | inconclusive 必须阻断 | `P3.Status = inconclusive` | `State == "limited"` | P0 |
| `TC-GATE-09` | failed 必须阻断 | `P3.Status = failed` | `State == "limited"` | P0 |
| `TC-GATE-10` | 单项失败不拖垮全部 | 仅 `P4.Status = region_blocked` | `State == "limited"`（非 blocked）、`Total <= 69` | P0 |
| `TC-GATE-11` | **SKIPPED 不进分母** | `P3.Status = skipped`，其余 ok | `State == "usable"`，且 AI 维度分 = 其余两项加权，**不得**被 0 拉低 | **P0** |
| `TC-GATE-12` | 取消不评级 | 三项均 `cancelled` | `State != "blocked"`（应引入「不评级」） | **P0** |
| `TC-GATE-13` | **禁止假绿（负向断言）** | 遍历 12 组坏态组合 | 任何组合都**不得**出现 `State=="usable" && Total>=85` | **P0** |
| `TC-GATE-14` | 封顶必须留痕 | 触发 CAP-1/CAP-2 | `len(Caps) >= 1`，且 `Caps` 文案**含具体规则**（供 pm 要求 `caps_applied` 展示） | **P0** |
| `TC-GATE-15` | 封顶不得重复叠加失控 | P3=hijacked + P4/P5 region | `Total == 29`（blocked 优先于 55） | P1 |
| `TC-GATE-16` | 替换空断言 | `TestMaskingNotLeaking` 当前恒真 | 改为：`Summary` 序列化后**不含** `password`/`Authorization`/`cookie` 字样 | **P0** |
| `TC-GATE-17` | 档位边界 | `Total` = 29/30/39/40/54/55/69/70/84/85 | 见 §3.5 表；**30–39 区间现实现落「不可用」，PRD 应为 D 档** | P1（§9-N9） |
| `TC-GATE-18` | 裁决文案与裁定一致 | `usable` 且 `CV > 0.5` | 文案用「速度有起伏…日常够用」，**禁**「忽快忽慢」 | P1（已在代码中分流，需锁） |

### 3.5 档位边界表（代码 vs PRD，必须择一）

| `Total` | 代码 `grade` | PRD 五分档 | 一致？ |
|---|---|---|---|
| 85–100 | 优秀 | A | ✅ |
| 70–84 | 良好 | B | ✅ |
| 55–69 | 一般 | C | ✅ |
| **40–54** | 较差 | D | ✅ |
| **30–39** | **不可用** | **D** | ❌ 偏差 |
| 0–29 | 不可用 | E | ✅ |

`[DIVERGE]` `TC-GATE-17` 断言当前值（30–39 → 不可用），并在 §9-N9 登记：边界应为 `>=30 → D`、`>=55 → C`…，且**输出 `A`–`E` 而非中文**（PM 要求 UI 显示五分档字母时需要一个稳定映射）。

### 3.6 「结论不得与分数打架」的正向/负向双向断言

| 用例 | 断言形式 | 说明 |
|---|---|---|
| `TC-GATE-19` | `State=="usable"` → `Verdict` 里**不得**出现「不可用 / 打不开 / 被拦截 / 受限」 | 首屏一句话必须与裁定同向 |
| `TC-GATE-20` | `State=="blocked"` → `Verdict` 必须出现「无法使用」类表述 | 不得含糊成「存在短板」 |
| `TC-GATE-21` | `State=="limited"` → `Verdict` 必须点出**具体短板**（风控/地区/证书之一） | 空泛文案视为失败（ux-arch 要求 `why` 短语） |
| `TC-GATE-22` | `len(Advice) >= 1` 当 `State != "usable"` | 用户必须拿到下一步动作 |

---

## 4. 真实链路人工对照用例（不可自动化的最后一道关）

### 4.1 方法（可复现，`10` §4.1 已执行过一次）

1. 同一时间窗口内：工具跑 **standard 档**（约 20 s），**同时**在真实浏览器新标签打开三个目标站点。
2. 浏览器与工具走**同一套系统网络栈**（同一 TUN 路由），构成同条件实验。
3. 逐项比对「人工看到的事实」与「工具结论」。
4. 记录必须包含：是否已登录、页面是否完整渲染、是否出现人机验证/地区提示。

**为什么必须是同窗口**：Cloudflare 风控与地区策略随时间变化（几分钟内即可翻转），跨窗口对照得出的「不一致」不可信。

### 4.2 判定表：人工看到什么 → 工具应为什么状态

这是本文**唯一的验收真值表**。不一致即缺陷，按最右列定级。

| # | 人工在浏览器看到的事实 | 工具期望 `Status` | 期望 `State` | 不一致定级 |
|---|---|---|---|---|
| `TC-MAN-01` | 页面完整加载、已登录、可交互 | `ok` | `usable` | 工具判非 ok → **P0（假红）** |
| `TC-MAN-02` | 页面加载但需登录/需 Key（跳登录页） | `auth_required` | `usable` | 判 `failed` → **P0** |
| `TC-MAN-03` | 出现「Verify you are human / Just a moment」 | `challenge` | `limited` | 判 `ok` → **P0（假绿）** |
| `TC-MAN-04` | 页面显示「not available in your country/region」 | `region_blocked` | `limited`/`blocked` | 判 `ok` → **P0（假绿）** |
| `TC-MAN-05` | 浏览器提示证书错误/`NET::ERR_CERT_AUTHORITY_INVALID` | `hijacked` | `limited` | 判 `ok` → **P0（安全漏报）** |
| `TC-MAN-06` | 页面一直转圈、最终超时 | `failed`/`TIMEOUT` | `blocked` | 判 `ok` → **P0** |
| `TC-MAN-07` | 页面能开但内容被替换成运营商/网关提示页 | `hijacked` | `limited` | 判 `ok` → **P0（假绿）** |
| `TC-MAN-08` | 403 且无任何可识别特征 | `degraded` | `limited` | 判 `usable` → **P0**（§9-N11） |
| `TC-MAN-09` | 能打开但极慢（>5 s 首屏） | `ok`/`degraded`（速度维度低分） | `usable`/`limited` | 判 `failed` → P1 |
| `TC-MAN-10` | 浏览器能用、工具报「被风控」 | `challenge` | `limited` | **允许**（方向性偏保守，见 §4.4） |

### 4.3 本轮实测记录（`10` §4.2，直接引用）

| 探测项 | 工具结论 | 人工事实 | 一致性 |
|---|---|---|---|
| P3 ChatGPT 网页 | `ok` / 100 / HTTP 200 | 完整加载，**已登录**（账号 wang lao），历史会话列表正常 | ✅ |
| P4 Gemini 网页 | `ok` / 100 / HTTP 200 | 完整加载，**已登录**（小笠友），输入区就绪 | ✅ |
| P5 OpenAI API | `auth_required` / 92 / HTTP 401 | 返回 `{"error":{"message":"Missing bearer authentication in header","type":"invalid_request_error"}}` | ✅ |

**这次对照证明了三件事**（必须写进发布说明，否则用户不知道「可用」是怎么判的）：不是假阳性（工具说可用→人真的能用）、不是假阴性、401 语义正确（判「可达·需 Key」而非「坏」）。

### 4.4 方向性检验（保守性只允许单向）

| 用例 | 断言 | 理由 |
|---|---|---|
| `TC-MAN-11` | 工具判 `ok` → 人工**必须**能打开（漏报可用=**P0**） | 绿色结论是承诺 |
| `TC-MAN-12` | 工具判 `challenge` → 人工**可以**正常打开（允许误报受限） | 裸请求无 Cookie，对风控更敏感（`10` §4.4） |
| `TC-MAN-13` | 不一致时，工具结论**必须偏保守** | 违反方向性即 P0 |

> **必须保留的已知差异**：工具发裸请求（无 Cookie/登录态），浏览器带完整登录态。因此工具结论**偏保守**是设计而非缺陷。任何让工具「更乐观」的改动（例如自动带 Cookie、放宽 `challenge` 判定）都必须重新做本节对照。

### 4.5 复跑一致性（PRD 硬指标：两次复跑分项差 ≤ ±10）

| 用例 | 动作 | 断言 | 级 |
|---|---|---|---|
| `TC-MAN-14` | 同配置连续跑两次（间隔 ≥2 min，避开风控） | 每个维度分差 ≤ **±10**，`State` 相同 | **P0**（PRD 可验收项） |
| `TC-MAN-15` | 两次之间切换 quick/standard | `State` 不因档位翻转（除速度分） | P1 |
| `TC-MAN-16` | 同一出口连测 3 次 | 第 3 次若出现 `rate_limited`，**不得**导致 `State` 变化 | P1（`ux-arch` 的 provisional 问题，§11-Q1） |

---

## 5. 异常与边界矩阵

### 5.1 代理配置类（`config.Validate` / `netx.Resolve`）

| 用例 | 输入 | 期望 | 现状核实 | 级 |
|---|---|---|---|---|
| `TC-NET-01` | `mode=custom`, `proxyUrl=""` | 报错「代理地址为空」 | 一致 | P0 |
| `TC-NET-02` | `proxyUrl="127.0.0.1:7890"`（无 scheme） | 报错「无法解析」 | `url.Parse` 报 `first path segment cannot contain colon` → 一致 | P1 |
| `TC-NET-03` | `proxyUrl="ftp://host:21"` | 报错「仅支持 http/https/socks5/socks5h」 | 一致 | P1 |
| `TC-NET-04` | `proxyUrl="http://host"`（无端口） | 报错「缺少端口」 | 一致 | P1 |
| `TC-NET-05` | `proxyUrl="http://:8080"`（无主机） | 报错「缺少主机名」 | 一致 | P1 |
| `TC-NET-06` | `proxyUrl="socks5://[::1]:1080"` | **接受**（IPv6 字面量必须支持，FR-A2） | `url.Parse` 通过 | **P0** |
| `TC-NET-07` | `mode=direct` | `Effective==""`、`Source=="direct"`，**绝不**回落环境变量 | `t.Proxy=nil` 显式直连 ✅ | **P0** |
| `TC-NET-08` | `mode=system` + 环境有 `HTTP_PROXY` | 忽略环境变量 + `Warnings` 含「不会使用它」 | 一致（F5 实测） | **P0** |
| `TC-NET-09` | `mode=system`，`scutil` 返回 `HTTPSEnable=1` | 选 HTTPS 代理（优先于 HTTP/SOCKS） | 一致 | P1 |
| `TC-NET-10` | `scutil` 返回 `ProxyAutoConfigEnable=1` | `Effective==""` + detail 说明 PAC 不解析 | 一致 | P1 |
| `TC-NET-11` | `parseScutil` 喂固定字符串（纯函数） | 键值全部正确解析（含冒号在值内的情况） | 需新增测试 | P1 |
| `TC-NET-12` | 密码含 `@`/`:`（`http://u:p%40ss@h:1`） | `Mask` 后密码段为 `***`，原文不出现 | `maskURL` 只打码 password 段 ✅，但见 `TC-SEC-05` | **P0** |
| `TC-NET-13` | SOCKS5 认证失败（`0x02` 后拒绝） | 错误文案含「用户名或密码错误」 | 一致 | P1 |
| `TC-NET-14` | SOCKS5 返回 `0x05`（连接被拒绝） | 文案含「连接被拒绝」，且**归因 proxy** | 一致 | P1 |
| `TC-NET-15` | `socks5` vs `socks5h` | 前者本地解析发 `ATYP=1`，后者透传域名 `ATYP=3` | 需抓包/假代理断言 | **P0**（`01` 调研要求区分归因） |

### 5.2 链路故障类

| 用例 | 故障注入 | 期望行为 | 级 |
|---|---|---|---|
| `TC-NET-16` | 代理端口未监听（`127.0.0.1:9`） | 每个 probe `failed`，`State=blocked`，文案指向「代理端口未监听」 | P0 |
| `TC-NET-17` | 假 HTTP 代理返回 **407** | 文案指向代理鉴权失败（**不得**判目标站 401） | P0 |
| `TC-NET-18` | 假代理建立隧道后立即 RST | `failed` + `connection reset` 证据 | P1 |
| `TC-NET-19` | 自签证书 / 中间人证书 | `hijacked`，**绝不**重试或降级（H9） | **P0** |
| `TC-NET-20` | 重定向循环（A→B→A） | ≤3 跳后报「重定向次数过多」，`failed`，**不得**无限重试 | P0 |
| `TC-NET-21` | `https → http` 降级重定向 | **拒绝**（security §2.3） | **P0**（当前实现无此校验，§9-N13） |
| `TC-NET-22` | 响应体 100 MB（声明 + 实发） | 只读 `MaxBody`（64 KiB/200 KiB），进程 RSS 不涨，**不落盘** | **P0**（S-33） |
| `TC-NET-23` | 下载中途断流（4 MiB 只发 1 MiB） | 计数按实收，`Bytes` 不失真，不判成功 | P1 |
| `TC-NET-24` | 服务端 `Content-Length` 撒谎（声明 1 MB 实发 10 KB） | 判 `hijacked`（真截断路径） | P0（`TC-ST-28` 同源） |
| `TC-NET-25` | 享元/慢速攻击（每 9 s 发 1 字节） | 由 `TimeoutSec` 收口，**不得**挂死 | P1 |
| `TC-NET-26` | DNS 解析失败 | `failed` + 「DNS 可能被污染」+ 归因 `proxy` | P1 |
| `TC-NET-27` | 目标返回 gzip 压缩体 | 下载吞吐统计**关闭压缩**（`DisableCompression=true`），字节口径为原始字节 | P1 |

### 5.3 调度、取消与并发

| 用例 | 动作 | 期望 | 级 |
|---|---|---|---|
| `TC-ENG-01` | 运行中再次 `POST /api/run` | **HTTP 409** + 明确文案 | **P0**（现为 400，§9-N3） |
| `TC-ENG-02` | 运行中 `POST /api/cancel` | 返回 `cancelled:true`；**≤500 ms** 内界面可见停止（PRD/ux-arch 硬指标） | **P0** |
| `TC-ENG-03` | 取消时已完成项 | 保持原状态，**不被**改成 `cancelled` | P0 |
| `TC-ENG-04` | 取消时未开始项 | `Status=cancelled` + `skipReason=cancelled` | P0（`skipReason` 未实现，§9-N10） |
| `TC-ENG-05` | 取消正在跑的 P6 | 期望 `cancelled`；现实现因 `okCount==0` 会返回 **`failed`** | **P0**（§9-N4） |
| `TC-ENG-06` | 取消正在跑的 P1 | 返回**部分结果**（`Bytes>0` 即保留），`Status` 按实收判定，不得报 0 分成功 | P1 |
| `TC-ENG-07` | P1 与 P2 同时调度 | P1 必须**独占**带宽（波次调度），不得与 P2 并发 | **P0**（现有 `execute` 已分两批 ✅，需锁） |
| `TC-ENG-08` | 只选 P6 跑一次 | 结果数组只含 P6，`expectedProbeCount==1` | P1（字段未实现，§9-N10） |
| `TC-ENG-09` | 重复 probe id / 非法 id | `Normalize` 去重 + 丢弃非法，不 panic | P1 |
| `TC-ENG-10` | `BudgetMB=9999` / `Concurrency=999` | 收敛到 512 / 16 | P1 |
| `TC-ENG-11` | 单个 probe panic（注入） | 进程**不得崩溃**，该 probe 记 `failed` | **P0**（现无 `recover`，§9-N14） |
| `TC-ENG-12` | 慢消费者（不读 SSE） | 广播**不得阻塞**探测（现有 `select/default` 丢弃 ✅），事件可丢但探测照跑 | P0 |
| `TC-ENG-13` | `-race` 全量跑 | 无竞态（`st.Results` 并发 append / `Snapshot` 浅拷贝共享指针） | **P0** |

### 5.4 SSE 与事件流

> **必须先承认的事实（F8）**：代码实际事件名是下划线风格 `snapshot / run_start / probe_start / sample / probe_result / score / run_end / error`，而 `06` §8 定义的是点分风格 `run.started / probe.stage / probe.settled / egress.detected…`。**两者都不是错的**，但**只能有一个对外契约**。
> 本文的处理：`TC-SSE-01`~`08` 断言**当前实现**（保住现状不回归），`TC-SSE-09`~`14` 断言 **06 契约**并标 `[PENDING-IMPL]`，作为「要补的能力」清单，不计入本轮失败。

| 用例 | 断言（现状） | 级 |
|---|---|---|
| `TC-SSE-01` | 连接后**首帧**是 `snapshot`，且含完整 `State`（刷新不丢状态） | P0 |
| `TC-SSE-02` | `Content-Type: text/event-stream`、`Cache-Control: no-cache`、`X-Accel-Buffering: no` | P1 |
| `TC-SSE-03` | 每条事件形如 `event: <name>\ndata: <json>\n\n`，`data` 可 `JSON.parse` | P0 |
| `TC-SSE-04` | 15 s 无事件时收到 `: ping`（注释帧，不触发 `onmessage`） | P1 |
| `TC-SSE-05` | `run_start` 的 payload 含 `runId/config/exit/env`；**`exit.warnings` 必为数组非 `null`** | P0 |
| `TC-SSE-06` | `probe_result` 逐个到达（不是等全部结束才发），且顺序按 `probe.Metas` 稳定 | **P0**（流式体验） |
| `TC-SSE-07` | 运行中断线 → 重连后**首帧快照**能补全（无需重放历史） | **P0** |
| `TC-SSE-08` | 断线重连后 `runId` 一致，不产生第二条并行运行 | P0 |
| `TC-SSE-09` | `[PENDING-IMPL]` `expectedProbeCount` 必须显式给出，前端用它判 `partial` | **P0**（ux-arch 硬要求） |
| `TC-SSE-10` | `[PENDING-IMPL]` `probe.stage` 六阶段 `dns/tcp/tls/http/ttfb/body`，对**全部 6 项**输出 | P0 |
| `TC-SSE-11` | `[PENDING-IMPL]` `probe.skipped` 带 `skipReason`（`cancelled/upstream_failed/budget_exhausted/profile_omitted`） | P0 |
| `TC-SSE-12` | `[PENDING-IMPL]` `egress.detected` 同时给 `configMode`（`SYSTEM/CUSTOM/DIRECT`）与 `egressKind`（`system_http/system_socks/tun/env/direct/unknown`） | P1 |
| `TC-SSE-13` | `[PENDING-IMPL]` `Score.caps_applied` 出现在事件与报告中（pm P0） | **P0** |
| `TC-SSE-14` | `[PENDING-IMPL]` 取消后 `run.finished` 带 `status=cancelled` + `partial=true` | P1 |

**负面断言（防止越权与串号）**：

| 用例 | 断言 | 级 |
|---|---|---|
| `TC-SSE-15` | 事件 payload 中**不得**出现请求头 `Cookie`/`Authorization`/代理密码 | **P0** |
| `TC-SSE-16` | 同一浏览器开两个标签页 → 两路 SSE 都能收到完整事件（广播而非单播） | P1 |
| `TC-SSE-17` | 关闭标签页后服务端订阅被清理（`subs` 不泄漏，长跑 10 次页面刷新内存平稳） | P1 |

### 5.5 本地服务攻击面（`server.guard` 等）

> 实测结果见 F7。**已确认 2 处绕过**，其中 `TC-API-03` 是**本机 CSRF**，属于 P0。

| 用例 | 请求 | 期望 | 实测 | 级 |
|---|---|---|---|---|
| `TC-API-01` | `GET /api/state` 无令牌 | 401 | **401** ✅ | P0 |
| `TC-API-02` | `GET /api/state` + `Host: evil.example.com` + 合法令牌 | 403（DNS rebinding 防护） | **403** ✅ | P0 |
| `TC-API-03` | `Origin: https://127.0.0.1:PORT.evil.com` + 合法令牌 | 403 | **200** ❌（`strings.Contains` 判同源） | **P0** |
| `TC-API-04` | `OPTIONS /api/state` | **405**（security：`OPTIONS` 一律 405，不发 CORS 预检） | **200** ❌ | P1 |
| `TC-API-05` | `GET /api/run` | 405 | **405** ✅ | P1 |
| `TC-API-06` | `POST /api/run` 坏 JSON | 400 + 中文可读文案 | **400** ✅ | P1 |
| `TC-API-07` | `POST /api/run` body > 64 KiB | 拒绝（`MaxBytesReader`），不 OOM | 一致 | P1 |
| `TC-API-08` | 响应头中**无** `Access-Control-Allow-*` | 无 | 一致 ✅ | P0 |
| `TC-API-09` | `GET /api/health` 无令牌 | 200（仅版本号，可接受） | **200** ✅ | P2 |
| `TC-API-10` | `GET /../../etc/passwd`、`/..%2f` | 404，绝不越出 `embed.FS` | 需实测 | **P0** |
| `TC-API-11` | `GET /api/unknown` | 404（不得落到静态分支返回 HTML） | 代码已拦 `/api/` 前缀 ✅ | P2 |
| `TC-API-12` | `GET /?t=` 首次访问 | 302 到 `?t=<token>` + `Set-Cookie: pl_sess`（`HttpOnly`+`SameSite=Strict`） | 一致 ✅ | P1 |
| `TC-API-13` | 令牌比较时序 | 应使用 `subtle.ConstantTimeCompare` | 现用 `!=`（本机场景风险低） | P2 |
| `TC-API-14` | `-addr 0.0.0.0:18799` | 拒绝启动 + 明确提示 | 一致 ✅（S1） | **P0** |
| `TC-API-15` | 二进制内 `InsecureSkipVerify` 命中数 | ≤1（且仅在 `tls_diagnose.go`） | 当前 0 处 → 通过 | **P0**（S-20） |

### 5.6 安全与合规专项（对照 `04-security.md` 验收项）

| 用例 | 要求 | 断言方法 | 级 |
|---|---|---|---|
| `TC-SEC-01` | **S-33** 响应体绝不落盘/落日志 | `grep -rn 'body' --include=*.go` 无 `log.Printf("%s", body)`；运行期 `fs_usage`/`lsof` 无临时文件；panic 栈不带 body | **P0** |
| `TC-SEC-02` | **S-34** `/v1/models` 200 不得判 PASS | 假栈返回 200+HTML → 期望 `inconclusive`/`failed`，现为 `ok`（`TC-ST-05`） | **P0** |
| `TC-SEC-03` | **S-35** 环境变量凭据同源脱敏 | `HTTP_PROXY=http://u:p@h:1` → warning 中密码为 `***`（`maskURL` ✅） | **P0** |
| `TC-SEC-04` | **H9** 无任何证书降级入口 | 传 `-k`/`--insecure`/`PROXYLENS_INSECURE=1` → **未知参数报错退出**（S-20 会这样测） | **P0** |
| `TC-SEC-05` | 报告不含明文密码 | `GET /api/report?format=json` 全文搜索密码 → **0 命中**；现 `config.proxyUrl` 与 `exit.effective` 为明文（§9-N2） | **P0** |
| `TC-SEC-06` | Markdown 报告二次脱敏 | `maskReportURL` 生效（现行 ✅），但 `user` 段未脱敏可接受 | P1 |
| `TC-SEC-07` | **H12** 持久化路径 | 若实现落盘：Windows 用 `%LOCALAPPDATA%`（**非** `%APPDATA%`）、目录 0700、文件 0600（`O_CREAT|O_EXCL` 或 tmp+rename） | **P0**（当前未实现落盘 → 用例保留为门禁） |
| `TC-SEC-08` | 白名单序列化 | `settings.json` 不含任何含 `password` 的键；`history.jsonl` 不含完整 URL/响应体/IP/token | **P0** |
| `TC-SEC-09` | 「清空历史」= 删文件 | 删除后目录下无残留、无影子副本 | P1 |
| `TC-SEC-10` | 出站收口在 `Dialer.Control` | 尝试访问 `169.254.169.254`、`127.0.0.1:80`（目标侧）、`10.0.0.1` → **被拒**（目标侧仅公网单播） | **P0**（当前未实现，§9-N15） |
| `TC-SEC-11` | `nip.io` 绕过 | `https://169.254.169.254.nip.io` → 解析后 IP 命中私网 → **拒绝**（不能只做 host 白名单） | **P0** |
| `TC-SEC-12` | 代理侧网段表 | 代理地址允许 loopback + RFC1918；目标侧**不**允许 | P1 |
| `TC-SEC-13` | 请求总量预算 | 单 Run 请求数 ≤ 40、下载 ≤ 64 MiB（standard 档；可加 `request_manifest` 求和验证） | P1 |
| `TC-SEC-14` | 礼貌性 | P6 同目标间隔 ≥500 ms（现 `time.After(500ms)` ✅）、不携带任何凭据 | P1 |
| `TC-SEC-15` | 监听器只 IPv4 loopback | `net.Listen("tcp","127.0.0.1:0")`；**不要**因为支持 IPv6 代理地址就改监听地址（FR-A2 与本条无关） | **P0** |

### 5.7 P2 / YouTube 专项（能力边界必须如实标注）

| 用例 | 构造 | 断言 | 级 |
|---|---|---|---|
| `TC-P2-01` | 首页 500 | 期望 `degraded` + `Score == statusScore(degraded)==55`；现 `Score==0`（§9-N6） | P1 |
| `TC-P2-02` | 首页失败（`Err != nil`） | 期望 `Score` 同步赋值，`Evidence` 含 `errorHint` | P1 |
| `TC-P2-03` | 取不到分片直链 | `Evidence` **必须**出现「未实测视频分片吞吐」字样；`Headline` 不得为 `x.x Mbps` | **P0**（诚实性红线，`10` §7） |
| `TC-P2-04` | 若实现 T4（gvt1） | 文案**禁止**出现「YouTube 播放速度」；只能写「Google 边缘下行」 | **P0**（arch ADR 硬约束） |
| `TC-P2-05` | 缩略图 404 | 不判 failed；仅降级为「静态资源异常」 | P2 |


---

## 6. 跨平台验证矩阵

### 6.1 编译矩阵（F1，已全部通过）

```sh
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o pl-win-amd64.exe .
GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o pl-win-arm64.exe .
GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o pl-mac-amd64 .
GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o pl-mac-arm64 .
```

### 6.2 真机验证矩阵

| 平台 | 产物 | 编译 | 真机运行 | 必测项 | 状态 |
|---|---|---|---|---|---|
| darwin/arm64 | 6.6 MB | ✅ | ✅ 本轮实测（TUN，6 项全跑通） | 全量 | **已验证** |
| darwin/amd64 | 7.1 MB | ✅ | ❌ | `scutil --proxy` 解析、TUN 路由、quick 档 <10 s | **待真机验证** |
| windows/amd64 | 7.2 MB | ✅ | ❌ | 注册表 `Internet Settings`、`netsh winhttp`、SmartScreen、防火墙弹窗 | **待真机验证** |
| windows/arm64 | 6.5 MB | ✅ | ❌ | 同上 | **待真机验证** |

> **不许把「编译通过」写成「支持」**。`10` §9 已把它列为「中」级未闭环项，本文沿用该定级。

### 6.3 平台相关用例

| 用例 | 平台 | 断言 | 级 |
|---|---|---|---|
| `TC-PLAT-01` | darwin | `scutil --proxy` 输出被正确解析为 `http://host:port`；`HTTPSEnable` 优先 | P0 |
| `TC-PLAT-02` | darwin | 无系统代理时 `Effective==""` 且 detail 含「TUN / 透明代理场景下属正常」 | P0 |
| `TC-PLAT-03` | windows | 注册表 `ProxyEnable=0x1` + `ProxyServer=host:port` → 解析成功 | P0 |
| `TC-PLAT-04` | windows | `ProxyServer="http=a:1;https=b:2"` → 取 **https** 分支 | P1 |
| `TC-PLAT-05` | windows | 注册表为空 → 回落 `netsh winhttp show proxy`，`Direct access` 时返回空 | P1 |
| `TC-PLAT-06` | windows | **`ProxyOverride` 绕过列表未被遵守**（设置了 `*.local` 仍走代理）→ 记为 P2 缺陷 | P2 |
| `TC-PLAT-07` | windows | 中文系统 `netsh` 输出为「直接访问」时也能识别 | P1 |
| `TC-PLAT-08` | linux | `system_other.go` 返回空 → 直连；不 panic | P2 |
| `TC-PLAT-09` | 全平台 | 路径分隔符/临时目录/字体不参与探测（无文件写入） | P2 |
| `TC-PLAT-10` | 全平台 | 二进制体积 < 10 MB、启动到可交互 < 300 ms | P2 |
| `TC-PLAT-11` | win + mac | 终端输出中文不乱码（Windows 控制台代码页） | P1 |

---

## 7. 性能基线

### 7.1 三档目标 vs 实测（F2）

| 档位 | 并发 | 预算 | 下载窗口 | 目标 | 实测 | 结论 |
|---|---|---|---|---|---|---|
| quick | 5 | 20 MB | 8 s | < 10 s | **7.86 s** | ✅ 余量 21% |
| standard | 6 | 64 MB | 15 s | < 25 s | **19.9 s** | ✅ 余量 20% |
| deep | 8 | 160 MB | 30 s | < 60 s | **34.0 s** | ✅ |

> `10` §9 提到「deep 34 s 略高于设计目标 30 s」——注意这里有两个不同口径：**设计文档的「下载窗口 30 s」**与**验收的「总耗 <60 s」**。34 s 是总耗时，未超标。

### 7.2 性能用例

| 用例 | 断言 | 实测依据 | 级 |
|---|---|---|---|
| `TC-PERF-01` | quick 总耗时 < 10 s | 7.86 s | **P0** |
| `TC-PERF-02` | standard 总耗时 < 25 s | 19.9 s | **P0** |
| `TC-PERF-03` | deep 总耗时 < 60 s | 34.0 s | P1 |
| `TC-PERF-04` | 实收字节 ≤ 预算 ×105%（quick 20 MB → ≤21 MB） | 20.1 MB ✅ | **P0**（PRD 硬指标） |
| `TC-PERF-05` | P1 七项指标缺一不可：`bytes/mbps/peak/p50/p90/cv/衰减` | 报告含 CV 0.626、P50 38.8 | **P0** |
| `TC-PERF-06` | <0.5 Mbps 持续 ≥5 s → 提前收口判 `SLOW` | **未实现**（`P1SlowAbort`），§9-N16 | P0 |
| `TC-PERF-07` | 取消 → 界面停止 ≤500 ms | 未实测 | **P0** |
| `TC-PERF-08` | 内存峰值 < 100 MB（采样点不累积无上限） | `samples` 按 250 ms 累积，8 s≈32 点、30 s≈120 点，安全 | P1 |
| `TC-PERF-09` | 空闲内存 < 30 MB、CPU 空闲 ≈0 | 未测 | P2 |
| `TC-PERF-10` | 连续跑 10 次不泄漏（goroutine 数回落） | `defer tr.CloseIdleConnections()` + `samplerDone` 已收口 | P1 |

---

## 8. 验收判定标准

### 8.1 缺陷分级定义

| 级别 | 定义 | 例子 | 处理要求 |
|---|---|---|---|
| **P0** | 输出与事实相反的结论（假绿/假红）、泄露凭据或响应体、进程崩溃/卡死、安全红线被绕过 | 工具说 ChatGPT 可用但实际被拦；JSON 报告带明文密码；`Origin` 伪造可写 API | **阻断发布**，必须修复 + 补用例 |
| **P1** | 影响可信度或主要体验，但有绕法或影响面有限 | 档位边界差 10 分；P2 分数恒 0；事件缺 `expectedProbeCount` | 发布前修，或书面接受并在 UI 标注 |
| **P2** | 打磨项、极端场景 | `OPTIONS` 返回 200；`ProxyOverride` 未遵守 | 进 backlog |

### 8.2 可发布条件（三条全满足）

1. **P0 = 0**（含本文 §9 新登记的 6 个 P0 全部关闭，且每个都有对应 `TC-*` 用例转绿）。
2. **§3 闸门用例全绿**（22 条），且 `TC-GATE-13` 的负向断言可作为独立 `go test -run Gate` 一键执行。
3. **§4 人工对照一致性 100%**（`TC-MAN-01`~`10` 中任何一条「不一致」都构成 P0），且方向性检验（`TC-MAN-11`~`13`）通过。

### 8.3 有条件通过的条件

允许「有条件通过」的前提（`10` 当前就属于这种）：P0=0、P1 ≤ 3 且**全部在 UI 有明示**、未闭环项明确写进发布说明（如 Windows 未真机验证）。**不允许**用「有条件通过」放过假绿类问题。

### 8.4 本轮判定（沿用 `10` 结论，但补充本节视角）

| 维度 | 判定 | 依据 |
|---|---|---|
| 结论真实性（macOS） | ✅ 通过 | §4.3 三项逐条一致 |
| 评分不可误导 | ⚠️ **需复验** | 闸门主路径已锁，但 `TC-GATE-06/07/11/12` 未覆盖（degraded/rate_limited/skipped/cancelled 均会漏） |
| 安全基线 | ⚠️ **需复验** | `TC-API-03`（Origin 绕过）、`TC-SEC-05`（JSON 明文密码）为新增发现 |
| 性能 | ✅ 通过 | §7.1 |
| 跨平台 | ⚠️ 有条件 | 3 个平台未真机验证 |

---

## 9. 本轮测试新发现的缺陷（按优先级）

> 与 `10` §6 的 D1–D7（已修复）不重复。以下为**本轮读码 + 实测**新发现，编号 `N*` 供开发认领。

### 9.1 P0（6 个，阻断发布）

| # | 缺陷 | 位置 | 现象 / 后果 | 对应用例 |
|---|---|---|---|---|
| **N1** | **截断判定与采样上限冲突** | `probe/http.go` `Classify` | `BodyLen < Declared/2` 把「读满 `MaxBody`」误判为「响应被截断」→ 判 `hijacked`。Gemini 首页实测 **818,196 B**，采样上限 200 KiB → 若响应带 `Content-Length`，P4 会直接变「疑似劫持」并触发总分封顶 55。本轮未复现是因为该响应为 chunked（`Declared=-1`），**一旦 CDN 改为带长度即触发** | `TC-ST-27/28`、`TC-CL-04` |
| **N2** | **JSON 报告导出明文代理密码** | `engine.State.Config.ProxyURL` / `State.Exit.Effective` + `server.handleReport` | `maskReportURL` 只在 Markdown 分支生效；`?format=json` 直接 `Encode(st)`，`config.proxyUrl` 与 `exit.effective` 为原始 `http://user:pass@host`。（静态确认，未跑） | `TC-SEC-05` |
| **N3** | **并发运行返回 400 而非 409** | `server.handleRun` | `Start()` 的错误被一律写成 400，前端无法区分「参数错」与「已有运行中」；arch ADR 与 `06` §7.1 明确要求 **409** | `TC-ENG-01` |
| **N4** | **取消 P6 被记成 `failed`** | `probe/site.go` `RunBaseline` | ctx 取消 → 循环 break → `okCount==0` → `StatusFailed`。用户主动取消却得到「国际链路不可达」，且会把总分压到 blocked | `TC-ENG-05` |
| **N11** | **`degraded` 不阻断闸门** | `score.gateOf` | `p3Blocked` 的 switch 漏 `StatusDegraded`；「403 无地区特征」正是 `degraded` → ChatGPT 裸请求被 403、浏览器其实能开，却仍判 `usable`（假绿） | `TC-GATE-06` |
| **N12** | **`rate_limited` 不阻断闸门** | `score.gateOf` | 同上漏 `StatusRateLimited`；PRD 明确 429 计 0 分，闸门却放行成 `usable` | `TC-GATE-07` |

### 9.2 P1（8 个）

| # | 缺陷 | 位置 | 后果 | 用例 |
|---|---|---|---|---|
| **N5** | **双 Transport 硬约束未实现** | `probe/download.go` | 仅 `tr.Clone()+DisableCompression`，缺 `DisableKeepAlives=true`、`ForceAttemptHTTP2=false`、`MaxConnsPerHost=1`。arch D6 要求吞吐测量**禁止复用连接**，否则 5 条流被 HTTP/2 多路复用合并成 1 条 TCP，测出的不是「并发吞吐」 | `TC-PERF-05` |
| **N6** | **P2 失败/降级分支 `Score` 恒 0，且仍在调用已裁定删除的 `youtubei/v1/player`** | `probe/youtube.go` | ① 首页失败/非 200 两个早返回未赋 `Score`；② arch M8/M11 已定论该接口失效，v1.1 要求四级方案（页面→ytimg→gvs→gvt1），代码仍走旧路径且无 T4 | `TC-P2-01` |
| **N7** | **报告权重脚注与实现相反** | `engine/report.go` | 脚注写「速度 30% + 稳定性 30% + AI 40%」，实际 `total = 0.40/0.30/0.30`。用户按脚注复算会对不上 | `TC-SC-13` |
| **N8** | **SSE 事件契约两套** | `web/js/core/api.js` vs `06` §8 | 实现为 `run_start/probe_result` 下划线风格且缺 `probe.stage`/`expectedProbeCount`；文档为点分风格。前端与文档必须收敛到一版 | `TC-SSE-09~14` |
| **N9** | **档位边界 40 vs 30** | `score.grade` | 30–39 落「不可用」，PRD 应为 D 档；且未输出 A–E 字母 | `TC-GATE-17` |
| **N10** | **关键字段缺失** | 全局 | `expectedProbeCount`、`skipReason`、`probe.stage`、`configMode`/`egressKind`、`blame`、`subReason`、`caps_applied` 均未实现（ux-arch/pm 的硬要求） | `TC-SSE-09~13` |
| **N13** | **`https→http` 降级重定向未拦截** | `probe/http.go` `CheckRedirect` | 只查跳数，未校验 scheme 降级（security §2.3 要求拒绝） | `TC-NET-21` |
| **N16** | **慢速早收未实现** | `probe/download.go` | 无 `<0.5 Mbps 持续 5 s → 判 SLOW 提前收口`，慢线路会把 8 s 窗口耗尽才出结论 | `TC-PERF-06` |

### 9.3 P0（架构/安全缺口，需在 v1.1 实现前先立门禁）

| # | 缺口 | 依据 | 影响 |
|---|---|---|---|
| **N14** | 无 `recover()` 保护 | `engine.execute` 里 probe goroutine panic 会拖垮整个进程 | 一次异常响应就能让工具退出 |
| **N15** | 出站 IP 收口未实现 | security §1/§2：必须在 `net.Dialer.Control` 用 `netip` 校验（**不能**只做 host 白名单，`nip.io` 可绕过） | SSRF 面：本地工具被诱导访问内网/元数据地址 |

> N14/N15 属于「当前代码尚不涉及该攻击面，但 v1.1 一旦接自定义目标就必须先有」的**前置门禁**。本文要求：在实现 `target` 可配置前，`TC-SEC-10/11` 必须先绿。

---

## 10. 用例总表与统计

### 10.1 按域统计

| 域 | 覆盖内容 | 条数 | 优先级分布 |
|---|---|---|---|
| `ST` | `Classify` 主判定表 | 31 | P0 × 18 / P1 × 11 / P2 × 2 |
| `NET` | 代理配置、链路故障、SOCKS5、出站收口 | 27 | P0 × 11 / P1 × 14 / P2 × 2 |
| `GATE` | 闸门与封顶（含既有 5 条） | 22 | P0 × 20 / P1 × 2 |
| `SSE` | 事件流、重连、字段契约 | 17 | P0 × 11 / P1 × 6 |
| `MAN` | 人工对照与方向性 | 16 | P0 × 13 / P1 × 2 / P2 × 1 |
| `API` | 本地服务攻击面 | 15 | P0 × 6 / P1 × 8 / P2 × 1 |
| `SEC` | 安全与合规专项 | 15 | P0 × 9 / P1 × 6 |
| `SC` | 纯函数分值/权重 | 13 | P1 × 13（分值断言，非阻断） |
| `ENG` | 调度、取消、并发、竞态 | 13 | P0 × 11 / P1 × 2 |
| `PLAT` | 跨平台解析与真机 | 11 | P0 × 3 / P1 × 5 / P2 × 3 |
| `AT` | 归因与下一步动作 | 10 | P0 × 6 / P1 × 4 |
| `PERF` | 三档耗时与预算 | 10 | P0 × 5 / P1 × 3 / P2 × 2 |
| `CL` | 判定优先级冲突 | 6 | P0 × 5 / P1 × 1 |
| `P2` | YouTube 能力边界与诚实性 | 5 | P0 × 2 / P1 × 2 / P2 × 1 |
| **合计** | | **211** | **P0 117 / P1 82 / P2 12** |

### 10.2 关于「117 条 P0」的说明（避免被数字吓到）

P0 条数多，是因为**每个状态组合都要独立断言**，但它们收敛到**少量代码修复点**：

| P0 断言簇 | 断言数 | 收敛到 | 修一处即全绿 |
|---|---|---|---|
| 十态主判定 + 优先级 | 27 | `Classify` 分支表、`Blame`/`subReason` | ✅ |
| 闸门与封顶 | 20 | `gateOf` 补 `degraded`/`rate_limited`/`skipped` | ✅ |
| 报告与脱敏 | 15 | `State` 序列化前统一 `Sanitize()` | ✅ |
| 调度与取消 | 11 | `recover()` + `RunBaseline` 取消分支 | ✅ |
| 安全门禁 | 15 | `Dialer.Control` + 无 `-k` 参数 + `OPTIONS` 405 | ✅ |
| 人工对照 / 方向性 | 13 | 判定逻辑修好后自然通过 | ✅ |
| 跨平台 / 性能 | 8 | 真机验证 + 慢速早收 | ⚠️ |
| 诚实性 | 2 | P2 文案不标称播放速度 | ✅ |

**执行顺序建议**（先做收益最高的三件）：

1. `go test ./internal/score -run Gate` 先把 `TC-GATE-06/07/11/12` 写出来 → 立刻会红 2 条（N11/N12），修 `gateOf`。
2. 写 `TC-ST-27` → 会红（N1），修 `Fetch.Truncated`。
3. 写 `TC-SEC-05` → 会红（N2），在 `handleReport` 前加 `Sanitize(state)`。

这三步大约 1 天，能消掉 3 个 P0 与约 40 条 P0 断言。

### 10.3 用例执行方式速查

```sh
# L1/L2（秒级，无网络）
go test ./internal/probe -run 'TestClassify|TestStatusPriority' -v
go test ./internal/score -run 'TestGate|TestGrade|TestWeights' -v

# L3（假代理，无外网）
go test ./internal/netx -run 'TestResolve|TestSocks5|TestDialControl' -v
go test ./internal/engine -run 'TestCancel|TestWave|TestRecover' -race -v

# L4（真起二进制，本地假栈）
go test ./internal/server -run 'TestGuard|TestSSE' -v
./build/proxylens -no-open -port 18791 && ./scripts/e2e.sh

# L5（手工，发布前）
./scripts/manual-compare.md   # 见 §4.1 步骤
```

---

## 11. 待确认与偏差登记（交 team-lead / 产品裁定）

| # | 问题 | 本文暂定处理 | 需谁定 |
|---|---|---|---|
| **Q1** | `RATE_LIMITED` 是否让整轮 `State` 变 `limited`？用户 1 分钟内连测两次，第二次撞 429 会以为网络坏了（ux-arch 提的甲/乙方案） | 用例 `TC-GATE-07` 暂按 **must be limited** 写；若选「乙（记 0 但打 `provisional`，不参与历史对比）」，则改为：`rate_limited` **不阻断 gate**，但 `State` 附带 `provisional=true`，且首屏显示「本次含限流项，仅供参考」 | pm + ux-arch |
| **Q2** | `429 insufficient_quota`（额度耗尽）与 `429 Too Many Requests`（真限流）语义不同，是否都判 `rate_limited`？ | `TC-ST-12` 暂都判 `rate_limited`，标注待裁定 | pm |
| **Q3** | `statusScore(inconclusive)=60` **高于** `degraded=55`，即「无法确认」比「确认受限」得分高，与「宁红勿绿」相悖 | `TC-SC-03/04` 断言当前值并标注 | pm |
| **Q4** | `SKIPPED` 不计 0 分如何落地：剔除分母重归一化，还是按 `null` 传播？ | `TC-GATE-11` 要求「不进分母 + 权重重归一化」 | pm |
| **Q5** | 取消后三项全 `cancelled` 时 `State` 应为什么？现为 `blocked`（等于告诉用户「代理坏了」） | `TC-GATE-12` 要求新增「不评级/已取消」态 | pm + ux-arch |
| **Q6** | 权重以谁为准：实现 40/30/30 还是报告脚注 30/30/40？ | `TC-SC-13` 断言实现值 40/30/30（速度 40% 更符合「测代理」定位），并修脚注 | pm |
| **Q7** | 事件契约以下划线名（现实现）还是点分名（06 §8）为准？ | `TC-SSE-01~08` 保现状，`09~14` 挂 PENDING | arch |
| **Q8** | `verdict` 十态 vs 九态：`HIJACKED` 是否独立成态（PRD v1.1 已升为第十态，ux-arch 仍九态） | 本文按 **PRD 十态** 判定，代码 11 终态内部保留 | pm + ux-arch |
| **Q9** | Windows `ProxyOverride`（绕过列表）当前被忽略 | 记为 P2 缺陷（`TC-PLAT-06`），是否本轮修 | pm |
| **Q10** | `P1StopAbort`（<0.5 Mbps 持续 5 s 早收）是否本轮实现 | 记为 P0 断言（`TC-PERF-06`），若延期需在 UI 说明「慢线路会跑满窗口」 | pm |

### 11.1 与 06 设计文档的参数偏差登记（`[DIVERGE]`）

| 项 | 06 §14 规定 | 代码实现 | 影响 | 处理 |
|---|---|---|---|---|
| 速度断点 | `{0,2,5,10,20,50}→{0,40,60,80,95,100}` | `{0,1,2,5,10,20,50,100}→{0,25,40,58,72,85,95,100}` | 28.8 Mbps 得 88 分（06 口径约 97） | 本文按代码断言（与 `10` 报告的 91 分一致）；若要改，需同步 `10` 与本文 |
| 百分位算法 | 最近秩法 | 线性插值（`Percentile`） | p90 偏低（4 元素时 3.7 vs 4） | `TC-SC-14` 待补；建议按 06 改，否则 P90 永远不达标称分位 |
| 标准差 | 总体（n） | 样本（n-1） | CV 偏大约 15%（n=4 时 1.29 vs 1.12） | 同上，两处口径必须统一 |
| 丢弃慢启动 | 前 1000 ms | 仅丢第 1 个采样点（250 ms） | `peak/p50` 可能被慢启动前段拉低 | `TC-PERF-11` 待补 |
| 内部 tick | 100 ms | 250 ms（对外 4 Hz 合流） | 采样粒度粗 2.5× | 与 4 Hz 推送合并后可接受，需文档同步 |
| P2 方案 | 四级（页面→ytimg→gvs→gvt1） | 三级旧方案（含已裁定删除的 `youtubei`） | 见 §9-N6 | arch 已裁定，待实现 |

### 11.2 本文未覆盖的部分（明确说明，避免误以为「全测了」）

| 未覆盖 | 原因 | 建议 |
|---|---|---|
| 前端 UI 视觉与交互细节 | 属 `08-ui-spec.md` / `08a-visual-audit.md` 范畴 | 由 ui-designer 的走查清单覆盖 |
| Chromium/Safari `EventSource` 兼容差异 | 本地单机、`HttpOnly` Cookie 已兜底 | 只需在 3 个浏览器各跑一次 `TC-SSE-07` |
| 极端网络（卫星、2G、MTU 异常） | 无测试条件 | 用 `tc netem` 在 Linux 上模拟，或直接依赖慢速早收兜底 |
| 恶意代理主动投毒（返回伪造的「可用」页面） | 属对抗性场景 | `TC-NET-19/24` 已覆盖常见形态；深度对抗需专门红队 |
| 长时间稳定性（>1 h 连续跑） | 产品定位是「测一次」 | `TC-PERF-10` 用 10 次循环近似 |

---

## 12. 附：给开发的最小落地清单（按顺序打勾）

- [ ] `internal/probe/classify_test.go`：`TC-ST-01~30` + `TC-CL-01~06`（表驱动 + `testdata/verdict/*.json` golden）
- [ ] `internal/score/gate_test.go`：`TC-GATE-06~22`（把 §3.3 矩阵整表驱动化）
- [ ] `internal/probe/score_test.go`：`TC-SC-01~13`
- [ ] `internal/netx/resolve_test.go`：`TC-NET-01~15`（含 `parseScutil` 纯函数）
- [ ] `internal/netx/dial_test.go`：`TC-SEC-10/11`（`Dialer.Control` 网段表，`testing/quick` 随机 IP 1000 组）
- [ ] `internal/engine/engine_test.go`：`TC-ENG-01~13`（`-race`）
- [ ] `internal/server/server_test.go`：`TC-API-01~15`（`httptest` 直接打 `Handler()`）
- [ ] `internal/engine/report_test.go`：`TC-SEC-05`（断言 JSON 无密码）
- [ ] CI 脚本：`grep InsecureSkipVerify`、`grep body:.%s`、`go vet`、`-race`

---

**文档版本**：v1.0（2026-10-22）｜**对应代码**：`app/` 1.0.0（Go 1.26.3）｜**上游**：05/06/04｜**下游**：10-acceptance-report.md

