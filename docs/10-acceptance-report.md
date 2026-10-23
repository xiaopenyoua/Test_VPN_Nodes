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

---

## 附录 B · 第三轮修订：单项复测、总评/分评分页、桌面外壳

### B.1 本轮修了什么

| # | 变更 | 位置 |
|---|---|---|
| 1 | 「只测这项」接通：后端支持 `only=true`，保留其它 5 项结果、不重算总评 | `internal/config`、`internal/engine`、`internal/probe/types.go` |
| 2 | 前端注入 `onProbeOnly`，卡片显示「单项复测 · 不参与总评」 | `web/js/components/ProbeGrid.js`、`ProbeCard.js`、`main.js` |
| 3 | 新增「总评 / 分评」两个独立功能页（Tab 切换，方向键可切换） | `web/js/components/Tabs.js`、`main.js`、`ui.css` |
| 4 | 总评页常驻提示：分评中有 N 项单项复测，总评未重算 | `main.js`、`ui.css` |
| 5 | Electron 桌面外壳：sidecar 拉起、`PROXYLENS_READY` 解析、`hiddenInset` + vibrancy、退出收尸、electron-builder 打包 | `app/desktop/` |
| 6 | Apple 视觉规范：自动主题（`prefers-color-scheme`）、标题栏安全区、动效曲线 | `docs/11-apple-visual.md`、`tokens.css`、`ui.css`、`core/theme.js` |
| 7 | 四平台产物重建 | `app/dist/` |

### B.2 实测证据

**单项复测（API 层）**

| 步骤 | 结果 |
|---|---|
| 完整快速档 | 6 项结果，总评 **69 / C·可用** |
| 对 P6 发起 `only=true` | 结果仍为 **6 项**；P6 `only=true`、`RTT 182 → 204 ms`；总评仍 **69** |

**单项复测（浏览器层）**

- 分评页 6 张卡齐全，P6 复测后出现「单项复测 · 不参与总评」，其余 5 张卡结果原样保留；
- 总评页出现「分评中有 1 项单项复测（P6），总评仍来自上一次完整探测，未重算。」；
- 总分与三维分项（77 / 90 / 60）均未变化；
- 控制台无报错。

**P3 出口身份**

- 分评页 P3 卡片实际显示 `出口 loc=JP · colo=NRT`，`region` / `colo` 已贯通到前端；
- 本轮出口 `warp=false`，WARP 徽章未出现（符合预期）。

**Electron 外壳**

- `npm start` 启动后：Electron 主进程 + 渲染进程 + sidecar（`proxylens -no-open -emit-ready`）均在；
- 通过 AppleScript 正常退出后，sidecar 进程同步退出，无残留；
- `npm run dist:mac` 产出 `ProxyLens-1.0.0-arm64.dmg`（100 MB）与 `ProxyLens-1.0.0.dmg`（104 MB）；
- 两个 app bundle 的 `Contents/Resources/bin/` 均包含 darwin-arm64 / darwin-amd64 两个 sidecar。

**四平台产物**

| 平台 | 大小 |
|---|---|
| darwin/arm64 | 6.8 MB |
| darwin/amd64 | 7.3 MB |
| windows/amd64 | 7.4 MB |
| windows/arm64 | 6.8 MB |

### B.3 仍未闭环

- **30 分钟长跑未实跑**：本轮只验证了长跑代码路径与参数贯通，未消耗约 1 GB 流量做 30 分钟真实断流样本；需要用户择时确认后执行。
- Windows 真机回归（仍仅交叉编译）。
- macOS 安装包未签名（无 Developer ID），首次打开需右键「打开」或在「隐私与安全性」中放行。
- P2 YouTube 仍取不到分片直链，恒为 `degraded`。

### B.4 长时测试（30 分钟）的物理上限

P1 单测支持 30 分钟长跑，口径固定为 `durationSec=1800 / concurrency=1`，预算由前端按
`ceil(mbps × 1800 / 8 × 1.05)` 自动取，夹在 `[1024, 4096] MB`。**上限就是 4096 MB**，
换算成速度：链路快过 18.2 Mbps 时，跑满 30 分钟所需的流量会超过单次预算上限，
探测会在预算耗尽处提前结束。

界面不隐瞒这件事——开启长跑立即按当前实测速度算一遍：

| 情况 | 提示 |
|---|---|
| 预算够 | 单流跑满 30 分钟 · 预算 X，预计消耗约 Y |
| 预算不够但补得上 | 只够跑约 N 分钟，请把流量预算提到 X |
| 需要量超过 4096 MB | 超过单次预算上限 4.0 GB，实际会在约 N 分钟处因预算耗尽提前结束 |

所以「30 分钟样本」在快速链路上实际会变成十几到二十几分钟的样本。它仍是有意义的长时信号，
但核对结果时**不能把「曲线只画了一半」当成断流证据**。

### B.5 UI 重构记录（粘土拟态 v3 落地）

按 `design/v3` 把设计稿落到真实代码：

- 样式重分为 `css/tokens.css`（纯令牌）+ `css/ui.css`（约 890 行）；`internal/*` 与 API 未改动；
- 配置拆成两层：整体设置 `config`（六项共用，不含单次时长）+ 单项覆盖 `probeConfig`
  （只存改过的字段，`{}` 即跟随整体），分评页同时只展开一个面板；
- 结论区按信息层级拆三段：**裁定**（发生了什么）→ **分数封顶**（评分口径，小字）→ **下一步**（唯一要动手的部分）；
  此前三者混在同一个项目符号列表里，同字号同权重，最重要的动作被埋掉；
- 新增 URL hash 承接视图（`#verdict` / `#probes/P1` / `#settings`），刷新停在原地，也能把某一项直接分享出去；
- 修掉暗色主题下**总分数字近乎隐形**：总分币始终是浅色黏土面，数字却用 `--warn` 令牌，
  该令牌在暗色下翻成 `#fcd34d`，压在币面 `#f8d79a` 上只有 **1.4:1**；改为固定深色数值；
- 新增 `-dev <dir>` 启动参数：前端资源改从磁盘读，改样式不必重新编译；不传时仍走 `go:embed`，与发布产物完全一致。

设计检查（1440 / 768 / 390 三档）仍报 4 项 FAIL，**全部是粘土拟态的定义性特征**
（渐变黏土面、按钮渐变+投影、厚重投影、行首着色徽章），属设计系统内取舍，不做修改；
真缺陷 `headline-lines`（整句裁定当 26px 标题、768 下换五行）已修。


### B.6 独立带宽长跑（第四项菜单）

综合测试里的 P1 回答的是「一次完整体检里下载这一项表现如何」，上限 1800 s / 4096 MB。
它装不下另一个问题：**连续跑几小时，这条链路会不会自己掉下来**。所以把长跑独立出来，
给一套自己的量纲、自己的状态位、自己的事件通道。

| 项 | P1 长跑 | 独立带宽长跑 |
|---|---|---|
| 开关 | `longRun=true` | `bandwidth=true` |
| 时长 / 流量上限 | 1800 s / 4096 MB | 14400 s（4 小时）/ 102400 MB（100 GB） |
| 默认 | 1800 s / 1024 MB | 1800 s / 8192 MB |
| 写 `Results` / 重算总评 | 是 | **否** |
| 事件 | `sample` / `run_end` | `bw_start` / `bw_sample` / `bw_result` / `bw_end` |

`Config.Normalize()` 在 `bandwidth=true` 时早返回走 `normalizeBandwidth()`，六项那套
1800 / 4096 的夹紧逻辑完全不复用——否则 4 小时与 100 GB 会被静默夹死，界面上选得上、跑起来却不是那个参数。
独占锁两边共用：并行会让两侧吞吐互相污染，因此互相启动直接拒绝并说明原因。

#### 实测证据（真实网络，非构造数据）

| 场景 | 结果 |
|---|---|
| 开发版 5 分钟跑（300 s / 4096 MB） | `ok` / 20.9 Mbps，P50 23.11、P90 29.65、峰值 33.69，**750 MB**，60 个 5 s 窗口，有效时长占比 90%，未断流 |
| 发布版 45 秒跑（45 s / 2048 MB） | `ok` / 11.6 Mbps，**64.1 MB**，9 个窗口，有效时长占比 100%，未断流 |
| SSE（发布版，10 s 采样） | `snapshot` × 1 + `bw_sample` × 40（250 ms 一点，速率对得上） |
| 隔离性 | 两次跑完 `results` 均为 0 条、`summary` 为 `null`、`busy` 回落 `false`；综合测试的 P1 文案与总分逐字未变 |

发布产物核验方式：直接跑 `dist/proxylens-1.0.0-darwin-arm64`（走内嵌资源，不传 `-dev`），
从 HTTP 取 `js/components/BandwidthPage.js`、`BandwidthChart.js`、`core/bandwidth.js`，
确认 `longDur` / `seedRun` / `groupWindows` / `niceAxis` / `maxBudgetMB` 均在。macOS 的 `strings`
不认 UTF-8，**不能用它来判断中文文案有没有打进去**，会被误判成「缺失」。

#### 本轮修掉的四个真问题

1. **中途打开页面看到空图**：已下载 628 MB 的跑动，曲线只有末端一个尖峰——SSE 只给「从这一刻起」的增量。
   `BandwidthRun.Samples` 随快照下发（超 3600 点两两合并，保留开头），前端 `seedRun()` 补齐前半段。
2. **前后端两把尺子**：前端按 250 ms 采样点算空点，得出「有效时长占比 50%」，后端按 5 s 分窗得出 100%。
   采样间隙里的 Range 请求不是断流。前端改为同一口径（5 s 分窗 + 中位数 20% 阈值 + 连续 2 空窗）。
3. **y 轴刻度像随手写的数**：峰值 33.7 → 上限抬到 40 再三等分 → 标签 `0 / 13 / 27`。
   改为先反推干净步长（1 / 2 / 2.5 / 5 × 10ⁿ）再抬上限，现在是 `0 / 10 / 20 / 30`。
4. **同一事实两个数**：跑完后大数字旁写「已跑 4分56秒」（最后一个采样点），结果区写「长跑 5 分 1 秒」
   （服务端结算）。已跑时长改以 `durationMs` 为唯一口径，全程平均也随之从 21.3 修正为 20.9，
   与后端 `mbps=20.92` 对齐。

附带修了两处：`fmt.dur` 只到分钟，4 小时长跑会读成「已跑 240分」，本页改用含小时的 `longDur()`；
窄屏段控里「100 GB」折成两行把那一格撑高，加 `white-space: nowrap` 并下调字号。

设计检查三档视口（1440 / 768 / 390）仍只报 3 项 FAIL（`gradient` / `button-fx` / `heavy-shadow`），
均为粘土拟态定义性特征，不做修改。
