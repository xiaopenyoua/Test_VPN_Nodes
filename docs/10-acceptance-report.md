# 10 · 验收报告（Acceptance Report）

| 项目 | 内容 |
|---|---|
| 产品 | ProxyLens 1.0.0 · 系统代理能力探测工具 |
| 验收时间 | 2026-10-22 09:30 – 10:10 GMT+8 |
| 验收方式 | 真实链路端到端 + **浏览器人工访问对照** + 单元测试 + 四平台交叉编译 |
| 结论 | **有条件通过（Conditional Pass）** — macOS 端可交付；Windows 待真机验证 |

## 1. 一句话结论

工具在 macOS（arm64，TUN 模式）上跑通了全部 6 项真实探测，**结论与人工在浏览器里看到的情况完全一致**；本轮发现并修复 7 个缺陷（其中 3 个会导致用户误判），三档性能均达标。Windows 四平台已编译通过但未在真机运行，是唯一未闭环项。

## 2. 验收环境

| 项 | 值 |
|---|---|
| 主机 | macOS 26.3.1 / arm64 |
| 出网方式 | 系统代理 **TUN 模式**（未单独开启系统 HTTP 代理） |
| TUN 证据 | 探测回填 `remoteAddr = 198.18.0.70:443` / `198.18.0.64:443`，属 TUN 虚拟网卡保留段 |
| 代理地址 | **留空**（走系统代理，符合"TUN 下不用填"的说明） |
| 环境变量干扰 | shell 存在 `HTTP_PROXY=http://127.0.0.1:xxxxx`（端口每次不同，属终端注入） |
| 工具处理 | 系统模式**刻意忽略**该环境变量，并在界面显式 warning，避免测错对象 |
| 二进制 | `app/build/proxylens` 6.6 MB，单文件，零外部依赖 |

## 3. 交付物核对

| # | 交付物 | 文件 | 状态 |
|---|---|---|---|
| 1 | 任务简报 | `docs/00-task-brief.md` | ✅ |
| 2 | 产品调研 | `docs/01-product-research.md` | ✅ |
| 3 | 技术调研 | `docs/02-tech-research.md` | ✅ |
| 4 | 架构决策 | `docs/03-architecture.md` | ✅ |
| 5 | 安全设计与威胁建模 | `docs/04-security.md` | ✅ |
| 6 | 产品需求文档 PRD | `docs/05-prd.md` | ✅ |
| 7 | 探测引擎详细设计 | `docs/06-engine-design.md` | ✅ |
| 8 | 交互架构 | `docs/07-ux-interaction.md` | ✅ |
| 9 | UI 视觉规范 | `docs/08-ui-spec.md` | ✅ |
| 10 | 视觉走查（补充） | `docs/08a-visual-audit.md` | ✅ |
| 11 | 测试方案与用例 | `docs/09-test-plan.md` | ✅ |
| 12 | 验收报告（本文） | `docs/10-acceptance-report.md` | ✅ |
| 13 | 可运行工具 | `app/`（Go 源码 + 内嵌前端） | ✅ |
| 14 | 四平台产物 | win/amd64、win/arm64、darwin/arm64、darwin/amd64 | ✅ 编译通过 |

## 4. 核心验收：探测结论 vs 人工访问

> 验收要求原话：「在测试 chatgpt、openai、gemini 等可用的同时，需要手动打开这页面 验证测试的情况是否是真实的」

### 4.1 验证方法

在同一时间窗口内做两件事：

1. 用工具跑一次 **标准档** 探测（6 并发 / 64 MB / 15 s）
2. **同时**在真实 Chrome 浏览器里人工打开三个目标站点

浏览器与工具走同一套系统网络栈（同一 TUN 路由），构成可对照的同条件实验。

### 4.2 三项逐一对照

| 探测项 | 工具结论 | 人工在浏览器看到的事实 | 一致性 |
|---|---|---|---|
| **P3 ChatGPT 网页** | `ok` / 100 分 / 可用<br>`chatgpt.com/ · HTTP 200` | 页面完整加载，**已登录**（账号 wang lao），左侧历史会话列表正常渲染，输入框可交互 | ✅ 一致 |
| **P4 Gemini 网页** | `ok` / 100 分 / 可用<br>`gemini.google.com/ · HTTP 200` | 页面完整加载，**已登录**（小笠友），"需要我为你做些什么？"输入区就绪 | ✅ 一致 |
| **P5 OpenAI API** | `auth_required` / 92 分 / **可达 · 需 Key**<br>`api.openai.com/v1/models · HTTP 401` | 返回 `{"error":{"message":"Missing bearer authentication in header","type":"invalid_request_error"}}` | ✅ 一致（401 正是"可达但需鉴权"） |

### 4.3 这次对照证明了什么

1. **不是假阳性**：工具报"可用"的三项，人工打开确实可用；没有出现"工具说能用、人打不开"。
2. **不是假阴性**：工具没有把可用站点误判为受限。
3. **401 的语义判断正确**：`api.openai.com` 不带 Key 必然返回 401。若把 401 判成"失败/不可用"，会误导用户以为节点坏了。工具判为 `auth_required`（可达 · 需 Key）+ 92 分，是正确语义。
4. **同时验证了闸门未误触发**：本轮三站全 ok，闸门放行给出 `usable`；此前某轮 ChatGPT 被 Cloudflare 拦截时，闸门正确把总分压到 69 以下并判 `limited`（见 §6-D2）。

### 4.4 已知差异（必须说清，否则会成为信任缺口）

工具发的是**无 Cookie、无登录态**的裸请求；人工浏览器带**完整登录态**。二者对风控的敏感度不同：

- 裸请求更容易触发 Cloudflare 人机验证（`cf-mitigated: challenge`）
- 登录态浏览器通常已被站点放行

**因此工具结论偏保守**——工具判"被风控"，不代表你的浏览器一定打不开；但工具判"可用"，你的浏览器一定可用。这个方向是安全的（宁可误报受限，不可漏报可用）。

## 5. 性能验收

### 5.1 三档实测（同一台机器、同一 TUN 出口）

| 档位 | 并发 | 预算 | 下载窗口 | **实测总耗时** | 目标 | 结果 |
|---|---|---|---|---|---|---|
| 快速 quick | 5 | 20 MB | 8 s | **7.86 s** | < 10 s | ✅ |
| 标准 standard | 6 | 64 MB | 15 s | **19.9 s** | < 25 s | ✅ |
| 深度 deep | 8 | 160 MB | 30 s | **34.0 s** | < 60 s | ✅ |

### 5.2 真实下载吞吐（`dl.google.com`，不落盘，`io.Discard`）

| 轮次 | 档位 | 平均 | P50 | P90 | 峰值 | 实收字节 | CV |
|---|---|---|---|---|---|---|---|
| 1 | quick | 31.2 Mbps | — | — | — | 20 MB | — |
| 2 | standard | 36.2 Mbps | — | — | — | 64 MB | — |
| 3 | quick | 42.5 Mbps | — | — | — | 20 MB | — |
| 4 | quick | 40.9 Mbps | 46.2 | — | 91.8 | 20.1 MB | 0.59 |
| 5 | standard | 33.5 Mbps | 28.8 | — | 110.6 | 64.0 MB | >0.5 |
| 6 | **deep** | **38.1 Mbps** | **38.8** | **69.56** | **129.0** | **137.5 MB** | **0.626** |

**观察**：同一出口连续多小时吞吐在 31–43 Mbps 区间波动，峰值可达 129 Mbps。CV 稳定在 0.5–0.63，说明这条链路**带宽够用但抖动明显**——这正是用户最初想知道的答案类型（"能不能流畅用"比"节点延迟多少"更有意义）。

### 5.3 四平台交叉编译

| 平台 | 产物大小 | 状态 |
|---|---|---|
| darwin/arm64 | 6.6 MB | ✅ 本机实测运行 |
| darwin/amd64 | 7.1 MB | ⚠️ 编译通过，未在 Intel Mac 真机运行 |
| windows/amd64 | 7.2 MB | ⚠️ 编译通过，未在 Windows 真机运行 |
| windows/arm64 | 6.5 MB | ⚠️ 编译通过，未在 Windows ARM 真机运行 |

编译命令（可复现）：

```sh
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o pl-win-amd64.exe .
GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o pl-win-arm64.exe .
GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o pl-mac-amd64 .
```

## 6. 本轮发现并修复的缺陷

### P0：会导致用户误判，必须修

| # | 缺陷 | 现象 | 根因 | 修复 |
|---|---|---|---|---|
| **D1** | **P1 下载得分恒为 0** | 报告里出现「33.5 Mbps / score=0」的自相矛盾 | `internal/probe/download.go` 从未给 `Result.Score` 赋值 | 末尾补 `res.Score = SpeedScore(res.Mbps)`；失败分支补 `statusScore(StatusFailed)` |
| **D2** | 分数与裁定打架 | ChatGPT 被 Cloudflare 拦截却输出「AI 可用 88 分」绿色结论 | 旧 `stateOf()` 只看 `aiUsable>=2 && total>=70`，不看可用性事实 | 重构为 `gateOf()` 闸门模型：`p3Blocked` 时不得 usable；blocked 封顶 29、limited 封顶 69。5 个单元测试钉死 |
| **D3** | 运行中结论区不刷新 | 点了「开始探测」，大圆环仍显示「还没有测过 / 0 分」 | `main.js` 中 `if (phase !== 'running') verdict.render()`，running 且无 summary 时直接跳过渲染 | 统一为 `renderVerdict(state)` + 签名节流（只在语义变化时重绘，避免每次采样重建圆环导致闪烁） |

### P1：明显影响观感或可用性

| # | 缺陷 | 现象 | 修复 |
|---|---|---|---|
| **D4** | 启动占位永久残留 | 页面顶部一直挂着「正在加载 ProxyLens…」 | `main.js` 挂载前 `clear(root)`，清掉 `index.html` 里的 `.boot` |
| **D5** | 出网方式长句挤成一行 | TUN + 环境变量场景下拼出含 URL 的超长黄字，读不完 | 警告各占一行（`flex: 1 0 100%` + 左侧竖线），主描述与警告分层 |
| **D6** | 无结果时导出按钮可点 | 空态点「导出报告」才知道没东西 | `TopBar` 增加 `render()`，无 `summary` 时 `disabled` + 说明性 `title` |
| **D7** | 运行中圆环显示「0 分」 | 探测中显示 0 分会被误读成"测出来是 0 分" | `ScoreRing` 支持 `score=null` → 显示 `—`，运行中用 brand 蓝而非绿色 |

> **一次误判的排查记录**：完成态截图里「AI 可用性」那行说明文字疑似带黄色底纹，与另两行的灰色不一致。经页面内 `getComputedStyle` + DOM 检查确认：三行 `.dim-detail` 背景均为 `rgba(0,0,0,0)`、颜色均为 `rgb(106,114,128)`、无 `<mark>`、无扩展注入节点。该黄底实为**鼠标悬停触发的浏览器原生 `title` tooltip**（`.dim-detail` 挂 `title` 用于显示被省略号截断的完整文本），非样式缺陷。这类"看起来是 bug 其实是浏览器行为"的情况，必须查证据再下结论。

## 7. 功能验收（P1–P6）

| 项 | 名称 | 本轮结论 | 说明 |
|---|---|---|---|
| P1 | Google 大文件下载 | ✅ ok / 91 分 / 38.1 Mbps | 8 并发 Range 流式下载，实收 137.5 MB，不落盘 |
| P2 | YouTube 链路 | ⚠️ **degraded** / 55 分 / 链路可用 | 首页 TTFB 333 ms 正常，但**未取到视频分片直链**（YouTube 已下线旧 innertube 取流接口）。已如实标注，不伪造吞吐数字 |
| P3 | ChatGPT 网页 | ✅ ok / 100 分 / HTTP 200 | 人工对照一致 |
| P4 | Gemini 网页 | ✅ ok / 100 分 / HTTP 200 | 人工对照一致（曾因标题出现在 96 KB 之后误判，已把采样窗口提到 200 KB） |
| P5 | OpenAI API | ✅ auth_required / 92 分 / HTTP 401 | 人工对照一致；401 判为"可达 · 需 Key"而非失败 |
| P6 | 国际链路基线 | ✅ ok / 100 分 / RTT 93 ms / 抖动 32 ms / 6-6 | 采样间隔 500 ms |

**P2 的诚实性说明**：YouTube 一项目前只能证明"到 YouTube 的链路通"，不能证明"视频播放流畅"。这是能力边界，不是 bug。界面明确写了「未取到分片直链」，没有编造一个吞吐数字。

## 8. 安全验收（对照 `04-security.md` 基线，逐条实测）

| # | 要求 | 实测方法与结果 | 结论 |
|---|---|---|---|
| S1 | 仅绑 `127.0.0.1`，拒绝 `0.0.0.0` | `./proxylens -addr 0.0.0.0:18799` → 直接拒绝启动，提示"仅允许绑定 127.0.0.1，避免把本地探测能力暴露到局域网" | ✅ |
| S2 | 无令牌访问 API 必须拒绝 | `GET /api/state` 不带令牌 → **401** | ✅ |
| S3 | DNS rebinding 防护（Host 白名单） | 带合法令牌 + `Host: evil.example.com` → **403** | ✅ |
| S4 | 禁止 CORS 通配 | 带 `Origin: https://evil.example.com` 请求，响应头中**无任何 `Access-Control-*`** | ✅ |
| S5 | 凭据脱敏 | `maskURL()` 对 `user:pass@host` 的密码段打码；导出报告二次脱敏 `maskReportURL()` | ✅ 代码审阅 |
| S6 | 启动令牌强度 | `crypto/rand` 生成 16 字节 → 32 位十六进制 | ✅ |
| S7 | 下载数据不落盘 | `io.Discard` 直接丢弃，实测收 137.5 MB 无临时文件产生 | ✅ |
| S8 | 不做证书降级 | 无"忽略证书错误"开关；遇到证书异常判 `hijacked` 并给出排查建议 | ✅ |
| S9 | SSE 鉴权兜底 | `SameSite=Strict` Cookie + `X-ProxyLens-Token` 双通道 | ✅ |

## 9. 未闭环项与风险

| 级别 | 项 | 说明 | 建议 |
|---|---|---|---|
| **中** | Windows / Intel Mac 未真机运行 | 仅验证了交叉编译产物能生成，未在目标机跑过探测 | 交付前在一台 Windows 10/11 上跑一次 `quick` 档，确认系统代理读取（注册表 `Internet Settings` + `netsh winhttp show proxy`）正确 |
| **中** | P2 YouTube 只能证明到站可达 | 旧 innertube 取流接口已失效，取不到分片直链 | 后续可改为解析 watch 页 `playerResponse`，或用 `youtubei/v1/player` 新版本接口；当前如实标注为"受限" |
| **低** | 无代码签名 | macOS Gatekeeper 会拦未签名二进制，Windows SmartScreen 会警告 | 分发时附 `xattr -d com.apple.quarantine` 说明；正式发布建议走公证 |
| **低** | deep 档 34 s 略高于设计目标 30 s | 主要是 160 MB 预算 + 30 s 下载窗口的物理下限 | 可接受；界面有实时计时，用户不会觉得卡死 |
| **低** | 评分断点偏宽松 | 28.8 Mbps 即得 88 分（20 Mbps→85，100 Mbps→100） | 这是 engine 定的"够用即高分"口径，4K 视频约需 25 Mbps，逻辑成立；报告内已写明 100 Mbps 为满分便于用户自行换算 |

## 10. 最终结论

**可发布（macOS 端），Windows 端待真机验证。**

判据：

1. **真实性成立** —— 这是本项目最核心的验收点。三项 AI 站点（ChatGPT / Gemini / OpenAI API）的探测结论与人工浏览器访问**逐项一致**，且方向偏保守（宁可误报受限，不漏报可用）。
2. **测量对象正确** —— 测的是系统代理整体能力，不是节点延迟。留空即走系统代理，TUN 下 `remoteAddr=198.18.x.x` 可自证。
3. **评分不可误导** —— gate-not-weight 闸门保证"ChatGPT 打不开时不可能出现高分绿色结论"，5 个单元测试钉死该规则。
4. **性能达标** —— 三档 7.86 s / 19.9 s / 34.0 s，全部在目标内。
5. **安全基线通过** —— 9 项逐条实测，本地服务攻击面已收口。
6. **诚实性** —— 拿不到的数据（YouTube 分片吞吐）明确标注为未取到，不编造。

### 复现步骤

```sh
cd app
go build -trimpath -ldflags "-s -w" -o build/proxylens .
./build/proxylens -no-open -port 18770     # 终端会打印带令牌的访问地址
# 浏览器打开打印出的 http://127.0.0.1:18770/?t=<token>
# 出口模式选「系统代理」，代理地址留空，强度选「快速」，点「开始探测」
```

### 人工对照验证步骤（可复现 §4）

1. 在工具里点「开始探测」（选标准档，持续约 20 s）
2. 立刻在浏览器新标签打开 `https://chatgpt.com/`、`https://gemini.google.com/`、`https://api.openai.com/v1/models`
3. 20 s 后回到工具看结果，三项状态应分别为 `可用` / `可用` / `可达·需Key`
4. 若工具判 `可用` 而人工打不开 → **P0 缺陷**，需立即排查

---

## 附录 A · 第二轮修订：一次被推翻的「一致性」结论

上文 §"真实性成立"记录的是首次人工对照的结论。该结论**在 40 分钟后被自己的复测推翻**，
原因不是人工对照错了，而是**单次采样不足以支撑"可用"这种断言**。此处如实留档。

### A.1 发生了什么

| 时刻 | 工具结论 | 独立复测（同机器、同请求头、Proxy=nil 走 TUN） |
|---|---|---|
| 10:35 | P3 `ok` HTTP 200 → 总分 94 / A·优秀 | — |
| 10:40 | — | Go 连打 6 次：**6×403 + `cf-mitigated: challenge`** |
| 10:42 | — | curl 连打 8 次：6×403、2×超时，**0×200** |
| 10:52 | P3 `challenge` · 2/3 → limited 69 / C·可用 | Go 连打 6 次：4×200、2×403 |

结论：目标站点在 Cloudflare 后面会**分钟级抖动**，单次请求等于抽签。
工具当时没有撒谎，但它**用一次好运产出了一个高置信度的断言**，这与"结果符合实际"冲突。

### A.2 因此修了什么

| # | 缺陷 | 后果 | 修复 |
|---|---|---|---|
| A | `gateOf` 漏判 `degraded`/`rate_limited` | P3 降级仍判 usable（假绿） | 任一 AI 端点处于阻断态即 `hardBlock`；网页可用数 ≥1 且无阻断才 usable |
| B | `MaxBody` 截断后与 `Content-Length` 比长度 | 首页 >200 KB 被判「疑似劫持」 | `FetchResult.Truncated`，截断时跳过该判定 |
| C | 取消时在飞请求落 `failed`，评分照跑 | 用户按取消，工具宣判「代理不可用」 | `classifyError` → `StatusCancelled`；取消时不产出 Summary |
| D | 未测维度按 0 分参与加权 | 快速档「速度 40% 却 0 分」，读成速度极差 | `isMeasured()`，未测维度不参与加权、权重归一、前端显示「未测」 |
| E | P3/P4 单次采样 | 一次 200 就宣布「优秀」 | 改为 3 次采样取**最差**结论，卡片显示 `n/m 次能打开`，证据链给每次分布 |

### A.3 修订后的判定口径

- **可用（usable）**：至少一个 AI 网页端能打开，且**没有任何** AI 端点处于
  challenge / region_blocked / hijacked / failed / inconclusive / degraded / rate_limited。
- **受限（limited）**：能打开，但存在上述任一阻断或降级。
- **不可用（blocked）**：两个网页端都打不开，且 API 网关也不可达。
- **取消**：不出结论、不给分数，明确显示「本次探测已取消」。

### A.4 验证

- 单元测试 11 条全绿，其中 5 条是闸门/未测维度回归（`go test ./internal/score/ -v`）
- 标准档全量复测 20 s，P3/P4 各 3/3 → usable 93 / A
- 快速档（仅 P3–P6）复测 69 / C·可用，速度维度标「未测」、权重 0

### A.5 仍未闭环

- Windows 真机回归（当前仅交叉编译）
- `docs/06-engine-design.md` 的权重表与采样参数需与实现同步（权重归一化、P3/P4 三次采样）
- P2 YouTube 仍取不到分片直链，恒为 `degraded`——属于**已知未取到**，不是代理故障

