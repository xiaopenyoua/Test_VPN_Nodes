# 06 · ProxyLens 探测引擎详细设计（Engine Design Spec）

| 项 | 值 |
|---|---|
| 文档版本 | v1.0 |
| 状态 | 定稿，可直接据此编写 Go 代码 |
| 对应需求 | `docs/00-task-brief.md` §2.1 / §2.2 / §2.3 / §2.5 |
| 上游依赖 | 已读 `00-task-brief.md`、`01-product-research.md`、`05-prd.md`。`02-tech-research.md` / `03-architecture.md` 写作时尚未生成，本文对模块边界/路由前缀给出建议值并标 `[ADR-PENDING]`，**算法与数值不受影响** |
| 实现语言 | Go 1.26.x（单二进制，`go:embed` 内嵌前端） |
| 数据出口 | `127.0.0.1` 本地 HTTP + SSE |
| 核心约束 | 下载数据**全程 `io.Discard`，不落盘、不驻留内存** |

> 所有超时/字节数/并发数/系数均为**具体数值常量**，集中在 §14 参数总表。

---

## 0. 设计目标、非目标与关键裁决

### 0.1 目标

1. **回答业务问题**：「我现在这套代理，能不能顺畅用 AI？卡不卡、稳不稳、会不会被墙/被风控？」
2. 用**真实业务行为**（下载 Google 大文件、访问 ChatGPT/Gemini 网页、打 OpenAI API、走 YouTube 链路）而非 ICMP/TCP ping 得出结论。
3. **区分「慢」与「不可用」**：慢 → Speed/Stability 低；不可用/被风控 → AI Access 直接判死并给总分设上限。
4. **可解释**：每个分数可追溯到原始样本（`Score.inputs` + `formula_id`）。
5. **可流式**：过程中每 100~500 ms 有事件推给前端，UI 不等待全量完成。
6. **可复现/可审计**：同一 `RunConfig` 的请求集合在报告中完整可见。

### 0.2 非目标

- 不做节点/订阅管理、Clash 规则解析、Netflix/Disney 解锁判定。
- 不做服务端测速集群（引擎只跑本机）。
- 不做「裸带宽」极限压测：字节与并发受 §11 合规约束，**测的是「代理可用性 + 体感吞吐」**。

### 0.3 关键设计裁决

| # | 裁决 | 依据 |
|---|---|---|
| D1 | **P2（YouTube）绝不阻塞 Run**：任何一级失败只降级、不失败（v1.1 已升级为四级链，见 D8） | 实测 `youtubei/v1/player` 的 `WEB` client 无 PO token 返回 `UNPLAYABLE`、`adaptiveFormats=0`（E4a）；`ANDROID` 400；`TVHTML5` `ERROR` |
| D2 | **`speed.cloudflare.com` 只做可选对照**，不得作为 P1 唯一来源 | 实测本机 TUN 下 TLS 握手被中断 `SSL_ERROR_SYSCALL`，CONNECT 隧道已建立（E2） |
| D3 | **CHALLENGE 用响应头优先判定**，body 指纹只作兜底 | 实测 `chatgpt.com` → `403` + `cf-mitigated: challenge` + `server: cloudflare`（E3）；头字段确定性远高于 body |
| D4 | **AI 判定顺序：重定向终态 → 状态码 → 响应头 → body(≤64 KiB) → 标题** | 避免为判词下整页（Gemini 首页实测 818,196 B，E5） |
| D5 | **吞吐必须剔除 TCP 慢启动段**（丢弃前 1000 ms） | 否则 `p50` 系统性偏低，且慢启动时长随 RTT 变化，跨代理不可比 |
| D6 | **探活 probe 共用 `Transport`；吞吐 probe 用独立 `Transport`（禁复用）** | 复用连接会让 `Connect/TLS=0`（§2.4 P-2）；吞吐需干净连接，探活需复用以降开销 |
| D7 | **不引入第三方代理库**：`http.Transport.Proxy` + `golang.org/x/net/proxy` 足够 | 降低供应链风险，便于交叉编译单文件 |
| D8 | **删除 P2 的 `youtubei/v1/player`（原 T3）**，改为「页面解析 + gvs 握手 + gvt1 吞吐」四级 | 架构实测 M8/M11：`watch` 页 `playabilityStatus=OK` 但 `adaptiveFormats` 26 项**全部无 `url`**，`signatureCipher` 含 `s=`/`n=` 待 JS 解密；直连 `serverAbrStreamingUrl` 与去 `sig` 重放均 **403**。保留未文档化 POST 只带来「YouTube 不可用」误判风险，收益为零 |
| D9 | **V1 裁决由业务态门槛决定，分数降为附录** | ux-researcher / ux-arch 硬约束：业务可用性是 **gate 而非 weight**。否则「地区拦截 + 链路满分」会被算成 92 分优秀（假绿红线） |

### 0.4 与 PRD / UX / 安全 / 架构的对齐与冲突裁决

本节是**唯一**的跨文档对齐口径；若与其它文档冲突，以本节为准并回传对应 owner 更新。

**(a) `verdict` 采用 PRD 九态，`diagnosis` 承载细分**

PRD §3.1 锁定九态，不得新增改名 → 引擎所有 probe 的对外结论只能是：

```
PASS | PASS_WITH_NOTE | SLOW | BLOCKED_BY_POLICY | CHALLENGED | RATE_LIMITED | TIMEOUT | NET_FAIL | SKIPPED
```

但 UX/安全还需要 TLS 失败、不稳定、疑似篡改、无法判定这四类语义。**裁决：双层模型**——
- `verdict`：九态之一（UI 徽章、三态收敛、结论模板只认它）；
- `diagnosis`：子码（`TLS_ERROR` / `UNSTABLE` / `TAMPERED` / `INCONCLUSIVE` / `QUOTA_EXHAUSTED` / …），只进详情层，不进首屏。

映射表（`diagnosis → verdict`，引擎内唯一实现点）：

| diagnosis | verdict | 说明 |
|---|---|---|
| `TLS_ERROR` | `NET_FAIL` | 证书/握手失败；`security.tlsVerification=false` 时无法区分劫持，`warnings` 注明 |
| `UNSTABLE` | `PASS_WITH_NOTE` | 能用但抖动大（`cv>0.25` 或 `jitter_p90>120ms`） |
| `TAMPERED` | `NET_FAIL` | 疑似劫持；**同时**置 `security.suspected_tampering=true`，由 UI 独立红警，不靠 verdict 表达严重度 |
| `INCONCLUSIVE` | `NET_FAIL` | **宁红勿绿**：无法确认可用时不得输出 PASS（假绿是一票否决红线）。UI 由 `why` 说明「无法确认」，避免误导为「网络断了」 |
| `QUOTA_EXHAUSTED` | `RATE_LIMITED` | OpenAI `429 insufficient_quota`：网络通、账号受限 |

> **待 pm 确认（已回传 team-lead）**：强烈建议 PRD 增补第 10 态 `INCONCLUSIVE`（或 `UNKNOWN`）。当前映射 `INCONCLUSIVE→NET_FAIL` 会牺牲语义精度，是为「假绿红线」付出的代价。

**(b) 五档制（覆盖本文早期 A+/B+ 版本）**

`A 85–100 优 | B 70–84 良 | C 55–69 可用 | D 30–54 较差 | E 0–29 不可用`（PRD §3.2 锁定）。

**(c) 门槛优先：V1 裁决 ≠ 总分**

```
Step 1  gate = f(各业务态)         → 裁决 AVAILABLE | RESTRICTED | BLOCKED | DEGRADED | UNKNOWN
Step 2  quality = 0.40·Speed + 0.30·Stability + 0.30·AI   → 仅作附录与 A/B 排序
Step 3  若 gate ∈ {BLOCKED, RESTRICTED} → quality 封顶（BLOCKED→29, RESTRICTED→69），
        首屏只展示裁决 + 一句话原因，quality 折叠为「链路质量（附录）」
```

**(d) 其余参数对齐（本文已改，勿再回退）**

| 项 | 对齐值 | 来源 |
|---|---|---|
| 采样推送频率 | **4 Hz**（内部 tick 仍 100 ms 用于速率精度，推送聚合到 250 ms） | ux-arch §3.2 / 附录 A |
| 事件命名 | 点分式 `run.started` / `probe.started` / `probe.stage` / `probe.sample` / `probe.settled` / `probe.skipped` / `egress.detected` / `run.finished` | ux-arch 附录 A（**覆盖**本文早期 `run_start` 等下划线命名） |
| P1 默认预算 | **20 MiB**（5 流 × 4 MiB），硬上限 24 MiB（105% 容忍 → 25.2 MiB） | pm §4.B 流量矩阵 |
| 全 Run 总字节 | **≤ 100 MB** | pm §4.B |
| 重定向上限 | **3 跳**（原 8 → 改） | security §2.3 |
| P6 同目标最小间隔 | **500 ms**（原 300 → 改） | security 限流条款 |
| 全局并发 Run | **1**（已有 → HTTP 409） | security / pm |
| 允许的目标 | 仅 `https`、端口 443、FQDN；`http:80` 仅 `generate_204`；`socks5h` 下禁止自定义目标 | security §2.3 |
| 响应体上限 | 探测类 `io.LimitReader` **≤ 512 KiB**（本文 P3/P4 用 64 KiB） | security |
| 限速早收 | `< 0.5 Mbps 持续 ≥ 5 s` → 立即收口判 `SLOW`，不死等超时 | pm 可验收指标 |

**(e) 已发现的其他文档间冲突（已回传 team-lead，不阻塞编码）**

1. ux-arch §3.2 列了 10 态（含 `TLS_ERROR` / `UNSTABLE`），与 PRD 九态冲突 → 本文按 (a) 双层模型消解，ux-arch 无需改 `verdictCopy`。
2. ux-arch 事件名（点分）与本文早期下划线命名冲突 → 本文已改，以 ux-arch 为准。
3. pm 要求「P1 七项指标缺一不可」→ 本文 §3 已含「衰减形态」枚举新增（见 §3.5）。

---

### 0.5 延迟口径与 P2 采集口径（实测补充）

**(a) 「延迟」主指标 = `tls_ms`，不是 `connect_ms`**

arch 实测：走 HTTP 代理时 `httptrace.DNSStart` **不触发**；`connect_ms` 在代理模式下是到 `127.0.0.1:PORT` 的 loopback（0.26–0.34 ms），TUN/fake-IP 下是 0.68–0.80 ms —— **两者都不是远端 RTT**。唯一端到端可归因的是 `tls_ms`（实测 94–169 ms，三种出网路径一致）与 `ttfb_ms`。

→ 引擎约束：① 代理模式下 `dns_ms` 必须输出 `null`（填 0 = 编造「DNS 0 ms」）；② `connect_ms` 的 UI 标签固定为「**本地代理建连**」，禁止表述「远端建连」；③ Stability 的「延迟」子项用 `tls_ms`（次选 `ttfb_ms`），**不使用** `connect_ms`。

**(b) P2 四级（无未文档化接口）**

| 级 | 请求 | 实测 | 产出 |
|---|---|---|---|
| T1 页面 | `GET https://www.youtube.com/watch?v=<固定ID>`（或 `/`） | 200 / ≈1.40 MB / TTFB 300 ms / `server: ESF` | 起播门控 TTFB、`playabilityStatus.status`、页面 TLS |
| T2 缩略图 | `GET https://i.ytimg.com/vi/<id>/hqdefault.jpg` | 200 / 21,011 B / TTFB 171 ms / `server: sffe` | 只报 TTFB 与 bytes，**禁止折算 Mbps** |
| T3 gvs 边缘握手 | 从 T1 页面正则抠 `serverAbrStreamingUrl` 的 host → `GET https://<host>/` | **404 + `server: gvs 1.0`** | 到真实 googlevideo 边缘的 DNS+TCP+TLS（唯一「真 googlevideo」证据） |
| T4 Google 边缘吞吐 | `GET https://redirector.gvt1.com/edgedl/go/go1.22.0.darwin-arm64.pkg`（跟随 302 → `rrN---sn-*.gvt1.com`）+ Range | 302 后 206 | Google 边缘下行吞吐（与 googlevideo 同族节点） |

- `media_path` 枚举：`page_only` / `gvs_handshake` / `gvt1_throughput` / `googlevideo_real`（预留）/ `unavailable`。
- **文案硬约束（进 ADR）**：T4 的数字**绝不能**标成「YouTube 播放速度」，必须标「Google 边缘下行（同链路代理指标）」+ 脚注「未取到真实播放分片：YouTube 需 JS 签名，纯 Go 不可行」。

---

## 1. 探测项（Probe）清单与端点表

### 1.1 Probe 枚举

| ID | 名称 | 类型 | 主要产出 | 参与 AI 分 |
|---|---|---|---|---|
| `P1` | Google 大文件下载 | 吞吐 | `throughput{p50,p90,mean}`, `ttfb_ms`, `connect_ms`, `tls_ms`, `decay_ratio`, `cv` | 否 |
| `P2` | YouTube 播放链路 | 吞吐(降级)+可用性 | `startup_ms`, `media_mbps`, `media_path`, `ai_status` | 是（权重最低） |
| `P3` | ChatGPT 网页 | 可用性 | `ai_status`, `cf_mitigated`, `final_url` | 是 |
| `P4` | Gemini 网页 | 可用性 | `ai_status`, 地区限制识别 | 是 |
| `P5` | OpenAI API | 可用性+网络 | `ai_status`, `api_error_code`, `tls{version,cipher}` | 是 |
| `P6` | 国际链路基线 | 稳定性 | `rtt[]`, `success_rate`, `jitter_ms`, `fail_streak` | 否（参与 Stability） |

### 1.2 统一请求头

除特别说明，所有 probe 使用：

```http
GET <url> HTTP/1.1
Host: <derived>
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8
Accept-Language: en-US,en;q=0.9
Accept-Encoding: identity
Cache-Control: no-cache
Pragma: no-cache
ProxyLens-Run: <run_id>
```

- `Accept-Encoding: identity` 是**硬要求**：压缩会让「下载字节数」与「线路字节数」脱钩，吞吐失真，`Content-Length` 也不可信。禁用后 `bytes_read` 即线路负载字节。
- `ProxyLens-Run` 便于目标侧排障与用户自查，**不含任何用户标识**（§11）。
- **绝不**发送 `Cookie`、`Authorization`、首跳 `Referer`、`Sec-Fetch-*`。P5 尤其不带 `Authorization`。

### 1.3 P1 — Google 大文件下载（主吞吐）

| 项 | 值 |
|---|---|
| 主 URL | `https://dl.google.com/chrome/mac/universal/stable/GGRO/googlechrome.dmg` |
| 备用 URL | `https://dl.google.com/chrome/mac/stable/GGRO/googlechrome.dmg` |
| 对照 URL（可选，**失败不扣分**，仅附注） | `https://speed.cloudflare.com/__down?bytes=10485760` |
| 方法 | `GET` |
| Range | `Range: bytes=<start>-<start+per_stream_budget-1>`；`start` 由 §12 种子算法决定，按 65536 B 对齐 |
| 预期响应 | `206 Partial Content` + `accept-ranges: bytes` + `content-range: bytes s-e/278613265` |
| 实测 E1 | `HTTP/2 206`，`content-range: bytes 0-0/278613265`，全长 278,613,265 B（≈265.7 MiB）→ 任意字节预算都不越界 |
| 失败判据 | 非 `2xx/206`；或 `Content-Type` 为 `text/html`（被劫持到门户/验证页）；或读到 0 字节 |

**为何选 `dl.google.com`**：① 稳定、支持 Range、无风控；② 与 P2/P4 同属 Google 边缘骨干，对「Google 系可达性」有代表性；③ 公开安装包，礼貌风险最低（§11）。

### 1.4 P2 — YouTube 播放链路（四级，无未文档化接口）

> **v1.1 变更**：删除原 T3 `POST youtubei/v1/player`（见 D8 / §0.5(b)）。arch 实测 M8/M11：`watch?v=` 页 `playabilityStatus=OK`、`adaptiveFormats` 26 项但**全部无 `url`/`signatureCipher`**，唯一带 `signatureCipher` 的 `formats[0]` 含待解密 `s=` 与 `n=` 变换；直打 `serverAbrStreamingUrl` → **403**，去 `sig` 重放 → **403**。纯 Go 不做 JS 签名处理拿不到任何媒体字节，故不再尝试。

按顺序尝试，**任何一级失败都不阻塞 Run**，逐级累加证据。

| 级 | 请求 | 判据 | 产出 |
|---|---|---|---|
| **T1 页面** | `GET https://www.youtube.com/watch?v=jNQXAC9IVRw`（失败退回 `/`） | `200` 且 body 含 `ytInitialPlayerResponse` 或 `ytInitialData` | `startup_ms`(=T1 TTFB)、`playability`、`page_tls_ms`、页面字节数 |
| **T2 缩略图** | `GET https://i.ytimg.com/vi/jNQXAC9IVRw/hqdefault.jpg`（备 `mqdefault.jpg`） | `200` + `image/jpeg` + `bytes ≥ 8000` | `thumb_bytes`、`thumb_ttfb_ms`（**禁止折算 Mbps**） |
| **T3 gvs 边缘握手** | 从 T1 页面正则提取 `serverAbrStreamingUrl` 的 host → `GET https://<host>/` | 任意响应（`404 + server: gvs 1.0` 即成功） | `gvs_dns_ms`、`gvs_tcp_ms`、`gvs_tls_ms`（到真实 googlevideo 边缘的证据） |
| **T4 Google 边缘吞吐** | `GET https://redirector.gvt1.com/edgedl/go/go1.22.0.darwin-arm64.pkg`（跟随 302 → `rrN---sn-*.gvt1.com`）+ `Range: bytes=<start>-<start+4MiB-1>` | `206` | `media_mbps`、`edge_host` |
| 降级标记 | 未达 T4 | — | `media_path` 依次为 `page_only` / `gvs_handshake` / `unavailable` |

`serverAbrStreamingUrl` 提取正则（对 T1 原文，不区分大小写、允许转义斜杠）：

```
(?i)"serverAbrStreamingUrl"\s*:\s*"(https:\\?/\\?/[^"]+)"   →  unescape(\/)  →  url.Parse().Host
```

若不匹配（页面改版），T3 记为 `skipped`（`skipReason="parse_miss"`），**不重试**，继续 T4。

`media_path` 枚举：`page_only` / `gvs_handshake` / `gvt1_throughput` / `googlevideo_real`（预留）/ `unavailable`。

**输出语义与文案硬约束**

- `startup_ms` = T1 的 TTFB（起播门控指标：拿到播放器页面才有起播可能）。
- T4 的 `media_mbps` **绝不能**标成「YouTube 播放速度」，报告与 UI 固定文案为：**「Google 边缘下行（同链路代理指标）」** + 脚注「未取到真实播放分片：YouTube 需 JS 签名，纯 Go 不可行」。
- T2 只报 bytes/TTFB：15~21 KB 的样本折算 Mbps 误差可达数倍，属伪精度。

**实测证据**

- **E4a/b/c**（本文，作废）：`youtubei/v1/player` 的 WEB client 返回 `UNPLAYABLE`、`adaptiveFormats=0`；ANDROID 400；TVHTML5 `ERROR`。→ 支撑「删掉该接口」的结论。
- **M8/M11**（arch，更强）：页面级 `adaptiveFormats` 26 项全无 `url`；`serverAbrStreamingUrl` 与去 `sig` 重放均 403。
- **E4d** `hqdefault.jpg` → `200`、15,921 B、TTFB 171 ms、`server: sffe`（arch 另测 21,011 B，因边缘/尺寸略有差异，判据用 `≥8000` 而非精确值）。
- **E4e** `maxresdefault.jpg` → `404`（1,097 B 占位）→ **不要用 maxresdefault 作判据**。

### 1.5 P3 — ChatGPT 网页

| 项 | 值 |
|---|---|
| URL | `https://chatgpt.com/` |
| 方法 | `GET`（不用 Range，需读 body 指纹） |
| body 上限 | **64 KiB**（`io.LimitReader(resp.Body, 65536)`，读满或 EOF 即 `Close`） |
| 伴随静态资源（区分「页面被 CF 拦」与「站不可达」） | `GET https://cdn.oaistatic.com/assets/favicon-eex17e9e.ico`（实测 200，E6） |
| 健康预期 | `200/307`，`server: cloudflare`，body 含 `__NEXT_DATA__` 或 `chatgpt`，**不含** `Just a moment` |
| 风控预期 | `403` + `cf-mitigated: challenge`（E3）；body 含 `Just a moment...` / `Enable JavaScript and cookies to continue` / `challenges.cloudflare.com` / `cf-chl` |
| 超时 | 总 15 s；`ResponseHeaderTimeout` 8 s |

### 1.6 P4 — Gemini 网页

| 项 | 值 |
|---|---|
| URL | `https://gemini.google.com/` |
| 方法 | `GET`，body 上限 **64 KiB**（首页实测 818,196 B，E5，必须截断） |
| 健康预期 | `200`，`server: ESF`（E5），`<title>Gemini</title>` 或 body 含 `gemini`/`bard` |
| 地区限制预期 | `200` 但 body 含 `not available in your country` / `isn't available in your country` / `currently not available`；或 `451` |
| 登录前置预期 | 重定向链含 `accounts.google.com`（`signin`/`ServiceLogin`）→ `AUTH_REQUIRED` |
| 超时 | 20 s（页面大且过代理） |

### 1.7 P5 — OpenAI API 端点

| 项 | 值 |
|---|---|
| URL | `https://api.openai.com/v1/models` |
| 方法 | `GET`，**不带 `Authorization`**（§11） |
| body 上限 | 8 KiB |
| 网络通、服务可达 | `401` + JSON `{"error":{"message":"Missing bearer authentication in header",...}}`（E7）→ `AVAILABLE`，`api_error_code=NO_CREDENTIAL_PATH_OK` |
| 地区封锁 | `403` + JSON 含 `unsupported_country` / `Country, region, or territory not supported` → `REGION_BLOCKED` |
| 额度/频率 | `429` + `insufficient_quota` / `rate_limit_exceeded` → `DEGRADED`（网络通，账号侧问题，**不归因代理**） |
| 超时 | 10 s |

### 1.8 P6 — 国际链路基线

| 项 | 值 |
|---|---|
| URL 轮转 | 奇数样本 `https://www.google.com/generate_204`；偶数样本 `https://www.gstatic.com/generate_204` |
| 预期 | `204`，`Content-Length: 0`（实测两端点均 204，E8） |
| 样本数 | `N = 10`（Quick 模式 `N = 5`） |
| 样本间隔 | `500 ms`（security 限流；`N=10` 总跨度 ≈ 4.5 s + 请求耗时） |
| 单次超时 | `3000 ms`（连接 + 首字节 + 读完） |
| 两端点统计 | **分别统计成功率**（ux-researcher：一端通一端不通是 GFW 典型特征，禁止平均成「半通」） |
| 采集 | `ttfb_ms`、`total_ms`、`dns_ms`、`connect_ms`、`tls_ms`、`status`、`error_code`、`conn_reused` |
| 用途 | Stability 主力数据源；`rtt[]` → jitter / 成功率 / 连续失败 |

实测抖动样本 E9（TUN 直连，gstatic，5 次 TTFB / 秒）：`0.1719, 0.1749, 0.1693, 0.1767, 0.3020`
→ 相邻差绝对均值 `= (0.0030 + 0.0056 + 0.0074 + 0.1253) / 4 = 35.3 ms`。这正说明**一次抖动（302 ms 尖峰）会主导 jitter**，因此 jitter 计算必须用中位数鲁棒化（见 §4.3）。

### 1.9 三种出网模式（RunConfig.network.mode）

| 模式 | `mode` | Transport 构造 | DNS 归属 | 说明 |
|---|---|---|---|---|
| 系统代理（留空） | `system` | `Proxy: http.ProxyFromEnvironment`（并在启动时用 OS API 补全，见下） | 本地或代理 | **默认**。必须识别并展示「实际出网方式」 |
| 自定义代理 | `custom` | `Proxy: func(*http.Request)(*url.URL, error){ return cfg.ProxyURL, nil }` | `socks5h` 为**代理端** | 支持 `http/https/socks5/socks5h` + `user:pass@` |
| 直连对照 | `direct` | `Proxy: nil` | 本地 | 显式绕过系统代理/环境变量，用于 A/B 差值 |

- `system` 模式的解析优先级：**OS 系统代理设置 > 环境变量 `HTTPS_PROXY/HTTP_PROXY/ALL_PROXY` > 无代理（TUN 语义）**。
  - macOS：`scutil --proxy`（`HTTPEnable/HTTPSEnable/SOCKSEnable` 及对应 host/port）→ 失败则 `os.Getenv`。
  - Windows：WinINET `InternetQueryOption(INTERNET_OPTION_PROXY)` / 注册表 `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`（`ProxyEnable`、`ProxyServer`）→ 失败则 `os.Getenv`。
  - 两者都为空 → 记录 `effective_route: "tun-or-direct"`，`proxy: null`，并在 UI 明示（当前环境正是这种情况：`scutil` 无独立代理但 `HTTPS_PROXY=http://127.0.0.1:61713` 存在且有 TUN）。
- `Custom` 模式的 scheme 决定底层：`socks5/socks5h` 走 `x/net/proxy` 的 `SOCKS5` dialer（`socks5h` = 传 domain 给代理解析，`socks5` = 本地解析后传 IP）；`http/https` 走 `Transport.Proxy`。
- **认证**：`user:pass@` 只放内存 `url.User`；`http` 代理用 `Proxy-Authorization: Basic`，`socks5` 用 dialer 的 `Auth` 字段。**绝不写入日志/报告**（报告中只保留 `proxy_scheme`、`proxy_host_hash` = `sha256(host:port)[:8]`）。

---

## 2. 时序图与 httptrace 采集点

### 2.1 采集机制总纲

- 统一用 `httptrace.WithClientTrace(ctx, trace)`，**每个请求新建一个 trace 实例**（trace 不可跨请求复用，否则时间点串味）。
- **时间基准**：一律用 `time.Now()` 的单调时钟部分（`t.Sub(t0)`）计算间隔；**禁止**用 `UnixMilli()` 相减（会被 NTP 跳变污染）。
- 每个时间点写入 `Sample.Timings`（毫秒 float64，保留 1 位小数）与 `Sample.Flags`（布尔，标记「不可信/复用/重试」）。

### 2.2 通用请求时序（P3/P4/P5/P6，单次）

```
t0  request-start          发出前一刻：t0 = now()
    │  ── 若 DNS 由本地解析（direct / http-proxy / socks5）──
t1  DNSStart               开始解析（可能触发多次：Happy Eyeballs / 多 A 记录）
t2  DNSDone                dns_ms = t2-t1；记录 addrs_count, coalesced, err
    │  ── 若走 http/https 代理：此处 DNS 解析的是**代理主机名**，非目标域 ──
t3  ConnectStart           TCP 三次握手开始（per-address 可能多次）
t4  ConnectDone            connect_ms = t4-t3；err 非空则直接失败
    │  ── 若目标是 https ──
t5  TLSHandshakeStart
t6  TLSHandshakeDone       tls_ms = t6-t5；取 state.Version / CipherSuite / DidResume / NegotiatedProtocol
    │  ── 若走 http 代理访问 https：CONNECT 隧道建立包含在 t3~t4 内，TLS 在隧道内 ──
t7  GotConn                记录 Reused / WasIdle / IdleTime；**在此刻判定 conn_reused**
t8  WroteRequest           请求头+body 写完
t9  GotFirstResponseByte   ttfb_ms = t9-t0（含 DNS+Connect+TLS，端到端首字节）
    │  ── 读 body（LimitReader / Range）──
t10 first-body-read        首个 body 字节到达（HTTP/2 下可能早于 t9 回调，需取 min）
t11 body-done              total_ms = t11-t0；bytes_read 汇总
t12 conn-close / reuse     归还连接池
```

### 2.3 吞吐请求时序（P1/P2-T3，多流）

```
main:
  ctx, cancel = WithTimeout(runCtx, P1_TIMEOUT)
  for i in 0..n-1:  spawn stream(i)          # n 个独立连接，各自 trace
  ticker(100ms) → 聚合所有流的 bytes_since_last_tick → 发 sample 事件（§3.2）
  wait all streams OR totalBytes >= budget OR ctx.Done()
  cancel()                                    # 提前终止其余流（全局取消传播）

stream(i):
  t0 → trace → GET Range(start_i, budget_i)
  GotFirstResponseByte → 记录 stream.ttfb_ms（取所有流的最小值作为 probe.ttfb_min，中位数作为 probe.ttfb_p50）
  loop: buf := pool.Get(32KiB); nr, err := body.Read(buf)
        io.Discard? → 直接丢弃 buf，只累加 counter（**不入内存池外的任何结构**）
        atomic.AddInt64(&bytesTotal, nr); atomic.AddInt64(&streamBytes[i], nr)
        if streamBytes[i] >= budget_i || err != nil { break }
  pool.Put(buf)
```

### 2.4 坑位清单（必须按此处理，否则数据不可信）

| # | 坑 | 表现 | 处理 |
|---|---|---|---|
| **P-1** | **DNS 缓存命中** | `DNSStart/DNSDone` 不触发，`dns_ms=0` 或字段缺失 | 若 `dns_ms==0 && !conn_reused` → 置 `flags.dns_cached=true`，`dns_ms` 标 `null`，**不参与均值**；聚合时用「有效样本数」做分母 |
| **P-2** | **连接复用** | `ConnectStart/ConnectDone/TLSHandshake*` 完全不触发，`connect_ms/tls_ms=0` | `GotConn.Reused==true` → `flags.conn_reused=true`，`connect_ms/tls_ms` 置 `null`；**P6 的 RTT 统计允许复用**（复用才是真实浏览体验），但 `connect_ms` 汇总只用 `!reused` 样本；P1/P2 强制 `DisableKeepAlives=true` + 每次新建 `Transport` 快照，保证不复用 |
| **P-3** | **TLS 会话复用** | `TLSHandshakeDone` 触发但 `tls_ms` 极低（PSK 1-RTT） | `state.DidResume==true` → `flags.tls_resumed=true`；`tls_ms` 仍记录但聚合时单列，不计入「TLS 握手成本」均值 |
| **P-4** | **代理模式下 DNS 归属错判** | `http`/`https` 代理时，本地 DNS 解析的是代理域名，目标域名由代理解析 | `network.mode==custom && scheme in {http,https,socks5h}` → `dns_source="proxy"`，`dns_ms` 置 `null`；`socks5`（本地解析）→ `dns_source="local"`；`direct` → `local` |
| **P-5** | **多次 ConnectStart** | 双栈/多 IP 重试导致同一请求多次回调 | 用 `[]struct{start,done}` 数组记录全部，`connect_ms` 取**最后一个成功项**，并记 `attempts=len(arr)` |
| **P-6** | **HTTP/2 多路复用** | 同连接并发流，`GotConn` 时刻不代表真实建连 | 吞吐 probe 显式 `ForceAttemptHTTP2=false`，用 HTTP/1.1 单流单连接，保证 Connect 语义干净（并在报告中标注 `http_version`） |
| **P-7** | **首字节 ≠ 首 body 字节** | HTTP/2 下 `GotFirstResponseByte` 与首个 `Read` 返回值顺序可能颠倒 | `first_body_ms = min(t10-t0, ttfb_ms)`，且若两者差 > 50 ms 记 `flags.ttfb_skew=true` |
| **P-8** | **重定向链丢失** | `CheckRedirect` 默认跟随，原响应被丢弃 | 自定义 `CheckRedirect`：**`len(via) >= 3` 即 `return http.ErrUseLastResponse`**（security §2.3 上限 3 跳）；每跳必须**复检目标策略**（https、443、FQDN、禁 `https→http` 降级、逐跳脱敏），记录 `{url, status, location, at_ms}` 到 `redirect_chain`。**AI 判定必须用 `resp.Request.URL` 作为 `final_url`** |
| **P-9** | **`ResponseHeaderTimeout` 与 `ctx` 双重超时** | 两者都设时，错误信息不一致 | 统一：`ctx` 为总预算（唯一权威），`Transport` 各分项超时只作兜底（`DialContext 5s`、`TLSHandshake 5s`、`ResponseHeaderTimeout 8s`）；错误归类以 `errors.Is(err, context.DeadlineExceeded)` 优先 |
| **P-10** | **`GotFirstResponseByte` 在 100-continue 场景提前触发** | 极少见（仅 POST T3） | T3 的 POST 不设 `Expect: 100-continue`，规避 |

### 2.5 采集结构（Go）

```go
type Timings struct {
    DNSms      *float64 `json:"dns_ms"`       // null = 缓存命中/代理解析/复用
    Connectms  *float64 `json:"connect_ms"`   // null = 连接复用
    TLSms      *float64 `json:"tls_ms"`       // null = 复用或非 https
    TTFBms     float64  `json:"ttfb_ms"`      // 必填，t9-t0
    Totalms    float64  `json:"total_ms"`
    FirstBodyms *float64 `json:"first_body_ms"`
}

type TraceFlags struct {
    DNSCached  bool `json:"dns_cached"`
    ConnReused bool `json:"conn_reused"`
    TLSResumed bool `json:"tls_resumed"`
    TTFBSkew   bool `json:"ttfb_skew"`
    ConnAttempts int `json:"conn_attempts"`
}

// 注册：只需在发起请求前
tr := &httptrace.ClientTrace{
    DNSStart:             func(httptrace.DNSStartInfo) { mark(&dnsStart) },
    DNSDone:              func(i httptrace.DNSDoneInfo) { mark(&dnsDone); dnsErr = i.Err; addrs = len(i.Addrs); coalesced = i.Coalesced },
    ConnectStart:         func(network, addr string) { connAttempts++; mark(&connStart) },
    ConnectDone:          func(network, addr string, err error) { mark(&connDone); connErr = err },
    TLSHandshakeStart:    func() { mark(&tlsStart) },
    TLSHandshakeDone:     func(st tls.ConnectionState, err error) { mark(&tlsDone); tlsState = st },
    GotConn:              func(i httptrace.GotConnInfo) { mark(&gotConn); reused = i.Reused; wasIdle = i.WasIdle; idleTime = i.IdleTime },
    WroteRequest:         func(i httptrace.WroteRequestInfo) { mark(&wroteReq); writeErr = i.Err },
    GotFirstResponseByte: func() { mark(&firstByte) },
}
ctx = httptrace.WithClientTrace(ctx, tr)
```

---

## 3. 吞吐算法（P1 主算法，P2-T4 复用）

### 3.1 预算与并发（具体数值）

| 常量 | Quick | Standard（默认） | Deep | 说明 |
|---|---|---|---|---|
| `P1_STREAMS (n)` | 2 | **5** | 8 | 单 Run 并发 TCP 流数 |
| `P1_PER_STREAM_BYTES` | 2 MiB | **4 MiB** | 8 MiB | 单流字节预算 |
| `P1_TOTAL_BUDGET` | 4 MiB | **20 MiB** | 64 MiB | `n × per_stream`（PRD §4.B 锁定 20 MiB） |
| `P1_HARD_CAP` | 5 MiB | **24 MiB** | 72 MiB | 实测超跑容忍 105%（→ 25.2 MiB 内），超出立即 `cancel()` |
| `P1_MAX_DURATION` | 12 s | **30 s** | 60 s | 到时长即 `cancel()`，按已下载字节折算 |
| `P1_WARMUP_DISCARD` | 800 ms | **1000 ms** | 1500 ms | 丢弃慢启动段（D5） |
| `P1_SAMPLE_INTERVAL (Δ)` | 100 ms | **100 ms** | 100 ms | **内部**采样节拍（速率精度需要） |
| `P1_PUSH_INTERVAL` | 250 ms | **250 ms** | 250 ms | **对外 SSE 推送**（4 Hz，ux-arch 硬约束） |
| `P1_WINDOW (W)` | 800 ms | **1000 ms** | 1000 ms | 滑动窗口长度 = `W/Δ = 10` 个 tick |
| `P1_EMA_ALPHA (α)` | 0.30 | **0.30** | 0.30 | 瞬时速率平滑系数 |
| `P1_MIN_SAMPLES` | 6 | **12** | 20 | 有效窗口样本数下限，不足则 `throughput.reliable=false` |
| `P1_SLOW_ABORT` | — | **< 0.5 Mbps 持续 5 s → 收口判 SLOW** | — | pm 可验收指标，不许死等 30 s 超时 |
| `P2_T4_BYTES` | — | **4 MiB** | 8 MiB | gvt1 边缘 Range 预算（PRD：P2 3–8 MiB 合计） |
| `P2_T4_DURATION` | — | 15 s | 25 s | — |

> 48 MiB / 30 s 的量级：在 100 Mbps 线路上约 4 s 跑完预算（此时报「预算耗尽」，`limited_by="bytes"`）；在 10 Mbps 上跑满 30 s 报 37.5 MB（`limited_by="duration"`）。两个上限同时存在，避免慢线路把 UI 拖住。

### 3.2 采样（滑动窗口 + EMA）

每个 tick（100 ms）读取所有流的字节增量，计算窗口字节：

```
Δbytes_k     = Σ_i (streamBytes_i(now) − streamBytes_i(prev))    // 第 k 个 tick 的增量
Wbytes_k     = Σ_{j=k-9..k} Δbytes_j                              // 最近 10 个 tick（1 s）
rate_raw_k   = Wbytes_k / W_seconds          // bytes/s，W_seconds = 1.0
rate_k       = α · rate_raw_k + (1−α) · rate_{k−1}     // α=0.3，EMA
```

- `rate_k` 单位 bytes/s → 展示换算 `Mbps = rate × 8 / 1e6`。
- **预热剔除**：从 `t0+P1_WARMUP_DISCARD` 之后的窗口才进入统计集合 `S = [rate_k]`。
- **尾部剔除**：最后 1 个窗口（`W` 内）可能因 `cancel()` 断流而偏低 → 丢弃最后 `W/Δ` 个样本。
- **无效流剔除**：若某流在预热期后 `bytes==0` 或 `err!=nil`，该流从聚合中移除，并在 `P1.streams_failed` 计数；**若失败流数 ≥ n/2**，`P1` 判 `UNSTABLE` 不进入吞吐评分（改用 P2/降级值），见 §5。

### 3.3 分位数与稳定性量

设统计集合 `S`（单位 Mbps，`|S| = m`），升序排列 `s_1..s_m`：

```
p50 = s_{ceil(0.50·m)}                       // 最近秩法，不做插值（样本量小，插值会造假精度）
p90 = s_{ceil(0.90·m)}
mean = (Σ s_i)/m
std  = sqrt( Σ(s_i − mean)² / m )            // **总体标准差**（m 为全量，不是抽样）
CV   = std / mean                            // mean<0.1 Mbps 时 CV 置 null
```

**衰减斜率（decay_ratio）**：把剔除预热/尾部后的集合 `S` 按时间顺序切成 10 份，取前 3 份均值 `A_head`（前 30%）与后 3 份均值 `A_tail`（后 30%）：

```
decay_ratio = A_tail / A_head                // 1.0 = 完全不衰减；0.5 = 掉一半
decay_slope_mbps_per_s = (A_tail − A_head) / duration_seconds_of_S
```

**峰值**：`peak = max(s_i)`（仅展示，不参与评分，因其受 TCP 突发影响）。

### 3.4 伪代码

```go
func measureThroughput(ctx context.Context, cfg P1Config, t *Transport) (Throughput, error) {
    var total, stream atomic counters
    streams := make([]StreamStat, cfg.Streams)
    g, gctx := errgroup.WithContext(ctx)
    for i := 0; i < cfg.Streams; i++ {
        i := i
        g.Go(func() error { return oneStream(gctx, cfg, t, i, &streams[i], &total) })
    }
    for {
        select {
        case <-time.After(cfg.SampleInterval):   // 100ms
            pushWindow(total.Swap(0), cfg)       // 进入环形缓冲 → rate_raw → EMA → 发 sample 事件
            if total.Load() >= cfg.TotalBudget || ctx.Err() != nil { cancel(); break loop }
        case <-done:
            break loop
        }
    }
    return aggregate(windows, cfg), nil
}
```

`aggregate` 只做 §3.2/§3.3 的纯函数运算，**不碰网络**，因此可在单测里用固定窗口数组直接验证（§13）。

### 3.5 衰减形态枚举（PRD Q3 要求：必须是枚举且非空）

`decay_shape` 由 `decay_ratio` 与 `cv` 组合判定，**只取一个值**：

| 枚举 | 判据 | 用户语义 |
|---|---|---|
| `STEADY` | `decay_ratio ≥ 0.90` 且 `cv ≤ 0.15` | 全程平稳 |
| `MILD_DECAY` | `0.70 ≤ decay_ratio < 0.90` | 轻微掉速 |
| `SHARP_DECAY` | `0.40 ≤ decay_ratio < 0.70` | 明显掉速（用户原话「一路往下掉」） |
| `CLIFF` | `decay_ratio < 0.40` 或 `S` 中存在连续 ≥3 个窗口速率 < 首个有效窗口的 10% | 断崖/中断（「下到 30% 断了」） |
| `RAMP_UP` | `decay_ratio > 1.10` 且 `cv ≤ 0.30` | 逐步升速（TCP 慢启动未充分剔除或链路预热） |

- `decay_shape` 参与 UI 文案与 `attribution`（`CLIFF` → 追加 `THROTTLED` 归因），**不进入 quality 分数**（分数已由 `penalty_decay` 反映，避免双重惩罚）。
- `cv > 0.25` 时在 `diagnosis` 追加 `UNSTABLE`（映射 `PASS_WITH_NOTE`，§0.4(a)）。


---

## 4. 稳定性算法（P6 主算法）

### 4.1 成功率 / 超时率 / 失败分布

一次 Run 内 P6 执行 `N` 次（Standard `N=10`，Quick `N=5`）。判定：

| 结果 | 定义 |
|---|---|
| `success` | `status ∈ {204}` 且 `err == nil` 且 `total_ms < 3000` |
| `soft_fail` | 收到响应但状态非预期（如 `200` 但非空体、`403`、`5xx`）→ 网络通但被拦/异常 |
| `timeout` | `errors.Is(err, context.DeadlineExceeded)` 或 `err` 为 `net.Error` 且 `Timeout()==true` |
| `hard_fail` | DNS 失败、TCP 拒绝、TLS 失败、连接被重置、代理 CONNECT 失败 |

```
success_rate = count(success) / N
timeout_rate = count(timeout) / N
fail_rate    = count(soft_fail + hard_fail) / N        // success_rate + timeout_rate + fail_rate = 1
fail_streak  = max 连续 non-success 的长度            // 0..N，表征「断流」严重度
fail_codes   = map[ErrorCode]int                       // 分布，见 §7.4 错误分类
```

> **分母固定为 `N`**（不用「有效样本数」）：超时样本本身是负向信号，剔除会美化结果。

### 4.2 RTT 与 requests-per-second

- `rtt_ms` = 每次的 `ttfb_ms`（对 `generate_204` 而言 TTFB ≈ 一个完整 RTT + 服务端处理，≈ 3×RTT，但作为**相对指标**足够；报告字段名为 `ttfb_ms`，`rtt_label="ttfb"` 以免误导）。
- `rps` = `success_count / (last_success_at − first_success_at)`，仅当 `success_count ≥ 2` 时计算，否则 `null`。

### 4.3 Jitter（鲁棒版）

```
Step 1  取成功样本的 ttfb 序列 r_1..r_k（时间序）
Step 2  diffs = [ |r_{i+1} − r_i| ]，长度 k−1
Step 3  jitter_ms = median(diffs)          // **中位数**，非均值：单点尖峰不应主导
Step 4  jitter_p90_ms = p90(diffs)         // 长尾暴露
Step 5  spike_ratio = count(diffs > 3 × median(diffs) + 1) / (k−1)
```

以实测 E9 为例：`diffs = [3.0, 5.6, 7.4, 125.3] ms` → `median = (5.6+7.4)/2 = 6.5 ms`（均值是 35.3 ms，被单尖峰拉高 5.4 倍）。`p90 ≈ 125 ms`，`spike_ratio = 1/4 = 0.25`。→ 结论：**均值会误判线路，必须用中位数 + p90 组合**。

### 4.4 可用性判定（Stability 侧）

| 条件 | `stability_state` |
|---|---|
| `success_rate == 1.0` 且 `jitter_ms ≤ 20` | `EXCELLENT` |
| `success_rate ≥ 0.9` 且 `jitter_ms ≤ 60` | `GOOD` |
| `success_rate ≥ 0.7` | `FAIR` |
| `success_rate ≥ 0.4` | `POOR` |
| `success_rate < 0.4` 或 `fail_streak ≥ 5` | `DOWN` |

`stability_state` 用于 UI 文案与总分惩罚（§6.5），**不直接给分**（给分在 §6.3）。

### 4.5 长时测试（可选，Deep 模式）

- 时长 `60 s`，`P6` 以 `500 ms` 间隔连续采样，`N = 120`。
- 额外产出 `time_series`：每 `2 s` 一个 `{t, success_rate_2s, p50_rtt_2s}`，用于画「抖动热力条」。
- 强制要求：`avg_rps ≤ 2/s`，符合 §11 礼貌约束。

---

## 5. AI 可用性判定规则表（产品核心）

### 5.1 输入与输出

**输入**（`AIProbeInput`，全部由 probe 采集，不含任何用户凭据）：

| 字段 | 类型 | 来源 | 上限 |
|---|---|---|---|
| `probe_id` | `"P2"|"P3"|"P4"|"P5"` | — | — |
| `status` | int | 首跳 HTTP 状态码 | — |
| `final_status` | int | 重定向终态状态码 | — |
| `final_url` | string | `resp.Request.URL.String()` | 512 B |
| `redirect_chain` | `[]{url,status,at_ms}` | `CheckRedirect` 记录 | ≤ 8 跳 |
| `headers` | 白名单 map | `server`、`cf-mitigated`、`cf-ray`、`cf-cache-status`、`content-type`、`content-length`、`x-served-by`、`server-timing`、`location` | — |
| `content_length` | int64 (-1=未知) | header | — |
| `body_head` | string | 前 **64 KiB**，已 `strings.ToLower` 归一化的副本保留原文 | 65536 B |
| `title` | string | `<title>(.*?)</title>` 首个匹配，解码 HTML 实体 | 256 B |
| `tls` | `{version, cipher, alpn, did_resume}` | `tls.ConnectionState` | — |
| `error_code` | string | 网络层错误（§7.4），成功为 `""` | — |

**输出**（`AIVerdict`）：

```json
{
  "status": "CHALLENGE",
  "confidence": 0.95,
  "reason_code": "CF_MITIGATED_CHALLENGE_HEADER",
  "evidence": { "http_status": 403, "header": "cf-mitigated: challenge",
                "server": "cloudflare", "title": "Just a moment...",
                "final_url": "https://chatgpt.com/", "bytes_read": 8708 },
  "rule_id": "AI-R11"
}
```

### 5.2 枚举定义（7 类，互斥）

| 枚举 | 语义 | 用户可行动作 |
|---|---|---|
| `AVAILABLE` | 正常拿到业务页面/接口响应 | 无 |
| `DEGRADED` | 通，但慢/被限速/账号侧受限/内容被弱化（非网络封锁） | 换时间或换号；网络没问题 |
| `CHALLENGE` | 被 Cloudflare/Akamai 人机验证拦截（代理 IP 信誉问题） | **换节点/换 IP 段**；这是「代理特有」的失败 |
| `REGION_BLOCKED` | 地区不支持（IP 落地国家不被服务接受） | **换落地国家** |
| `AUTH_REQUIRED` | 被重定向到登录页，或需要凭据（本工具不带凭据） | 正常现象，不代表网络坏 |
| `HIJACKED` | 响应被中间人篡改/劫持（证书伪造、内容被替换、注入脚本） | **立即停用该代理**（安全事件） |
| `UNREACHABLE` | 网络层不通：DNS/TCP/TLS 失败、超时、连接重置 | 检查代理/线路 |

### 5.3 判定流程（严格按优先级，命中即返回）

```
R0  error_code != ""  →  UNREACHABLE                      (conf 0.99)
R1  TLS 校验失败/证书异常（见 5.4 H-3） →  HIJACKED        (conf 0.95)
R2  重定向链含登录域（5.4 H-4）  →  AUTH_REQUIRED          (conf 0.92)
R3  REGION_BLOCKED 特化特征命中（5.5 各 probe 表）        (conf 0.90~0.98)
R4  CHALLENGE 特化特征命中（header 优先，body 兜底）       (conf 0.80~0.98)
R5  HIJACKED 内容特征（注入/篡改/门户跳转）                (conf 0.85)
R6  status == 451 → REGION_BLOCKED                        (conf 0.95)
R7  status ∈ {407} → UNREACHABLE（代理认证失败）           (conf 0.99)
R8  status ∈ {521,522,523,525,526} → UNREACHABLE（CF 源站不可达） (conf 0.9)
R9  status ∈ {500..599} → DEGRADED（服务端故障，非代理问题） (conf 0.8)
R10 status == 429 → DEGRADED                              (conf 0.85)
R11 status ∈ {401,403} 且 命中「凭据语义」特征 → AVAILABLE / AUTH_REQUIRED（按 probe 策略，见 5.5）
R12 status ∈ {200,204,206,301,302,307,308} 且 业务指纹命中 → AVAILABLE (conf = 5.6 计算)
R13 status ∈ {200} 但业务指纹缺失（拿到的是空白页/占位页）→ DEGRADED (conf 0.6)
R14 其它 → DEGRADED (conf 0.5)
```

**「业务指纹」**= 该服务必然存在的正特征（如 ChatGPT 页面含 `__NEXT_DATA__`、Gemini 含 `AF_initDataCallback` 或 `<title>Gemini`）。

### 5.4 通用特征库（跨 probe 复用）

**H-1 反爬/验证类 header**

| 特征 | 正则/匹配 | 语义 |
|---|---|---|
| `cf-mitigated: challenge` | 精确值匹配（`strings.EqualFold`） | **最强 CHALLENGE 信号**（E3 实测） |
| `cf-mitigated: managed` | 同上 | CHALLENGE（托管模式） |
| `cf-cache-status: HIT` + `cf-ray` | 存在性 | 仅作证据，不作判定 |
| `server: cloudflare` + `status 403` + `cf-mitigated` 缺失 | 组合 | CHALLENGE（弱信号，conf 0.80） |
| `x-akamai-transformed` / `server: AkamaiGHost` + `403` | 组合 | CHALLENGE（弱） |

**H-2 反爬 body 指纹（大小写不敏感，正则）**

| rule_id | 正则 | 语义 |
|---|---|---|
| `B-CF-1` | `(?i)just a moment\.{0,3}` | Cloudflare 5s 盾 |
| `B-CF-2` | `(?i)enable javascript and cookies to continue` | CF interstitial |
| `B-CF-3` | `(?i)challenges\.cloudflare\.com/(turnstile|cdn-cgi)` | CF Turnstile |
| `B-CF-4` | `(?i)cf-chl-(browser|opt|img)` | CF challenge 脚本 |
| `B-CF-5` | `(?i)attention required!?\s*\|\s*cloudflare` | CF block 页标题 |
| `B-CF-6` | `(?i)window\._cf_chl_opt` | CF challenge 数据对象 |
| `B-AK-1` | `(?i)reference\s*#\d+\.[0-9a-f]+` + `(?i)access denied` | Akamai deny |

**H-3 劫持/中间人特征**

| rule_id | 特征 | 语义 |
|---|---|---|
| `H-3a` | `error_code ∈ {TLS_CERT_INVALID, TLS_CERT_UNKNOWN_AUTHORITY, TLS_CERT_NAME_MISMATCH, TLS_VERIFY_FAILED}` | 证书不可信 → HIJACKED |
| `H-3b` | `tls.version == "1.0"|"1.1"`（现代站点不应低于 1.2） | 疑似中间人降级 → HIJACKED（conf 0.8） |
| `H-3c` | body 含 `(?i)(window\.(top|location)\s*=\s*['"]https?://(?!.*(google|openai|youtube)))` 且首跳非该域 | 外部跳转注入 → HIJACKED |
| `H-3d` | body 含 `(?i)(<script[^>]+src=["']http://)`（https 页面加载 http 脚本） | 注入 → HIJACKED |
| `H-3e` | 目标为 `api.openai.com` 但 `content-type` 非 `application/json` 且 body 为 HTML | 被劫持/被门户拦截 → HIJACKED |
| `H-3f` | 目标站预期 `server: cloudflare|ESF|Google` 但实际为 `nginx|Apache|openresty` 且 body 含中文运营商/校园网文案 | 运营商劫持 → HIJACKED |

**H-4 登录跳转特征**

| rule_id | 特征 | 语义 |
|---|---|---|
| `H-4a` | `final_url` host ∈ {`accounts.google.com`, `login.microsoftonline.com`, `auth0.com`, `auth.openai.com`} | AUTH_REQUIRED |
| `H-4b` | 重定向链任一 `url` 含 `signin`/`ServiceLogin`/`/auth/` 且终态 `200` | AUTH_REQUIRED |
| `H-4c` | `status ∈ {302,303}` 且 `location` 含 `login` | AUTH_REQUIRED |

### 5.5 分 Probe 规则表（每行一条可执行规则）

#### P3 · ChatGPT（`chatgpt.com`）

| 优先级 | 条件 | 输出 | conf |
|---|---|---|---|
| 1 | header `cf-mitigated ∈ {challenge, managed}` | `CHALLENGE` | 0.95 |
| 2 | body 命中 `B-CF-1..B-CF-6` 任一 | `CHALLENGE` | 0.92 |
| 3 | `final_url` host == `chatgpt.com` 且 status `200` 且 body 含 `__NEXT_DATA__`\|`data-build`\|`oai-` | `AVAILABLE` | 0.95 |
| 4 | body 含 `(?i)(unsupported_country|not available in your country|country, region, or territory not supported)` | `REGION_BLOCKED` | 0.93 |
| 5 | `final_url` host 含 `auth.openai.com`\|`login` | `AUTH_REQUIRED` | 0.90 |
| 6 | status `403` 且 无 challenge 特征、body 含 `(?i)(access denied|forbidden)` | `DEGRADED` | 0.70 |
| 7 | status `429` | `DEGRADED` | 0.85 |
| 8 | status `200` 且 body 长度 `< 2000` 且无业务指纹 | `DEGRADED` | 0.60 |
| 9 | status `5xx` | `DEGRADED` | 0.75 |

> 实测：当前环境 P3 会命中优先级 1 → `CHALLENGE`（`cf-mitigated: challenge`，body 8,708 B）。**这是真实产品要报告的典型结论**：「代理能用，但 ChatGPT 的 IP 信誉被 CF 挑战」。

#### P4 · Gemini（`gemini.google.com`）

| 优先级 | 条件 | 输出 | conf |
|---|---|---|---|
| 1 | body 含 `(?i)(not available in your country|isn'?t available in your country|currently not available in your country|不支持您所在的国家|所在国家/地区目前无法使用)` | `REGION_BLOCKED` | 0.95 |
| 2 | status `451` | `REGION_BLOCKED` | 0.95 |
| 3 | `final_url` host 含 `accounts.google.com` 或链上有 `ServiceLogin` | `AUTH_REQUIRED` | 0.90 |
| 4 | status `200` 且（`<title>Gemini</title>` 或 body 含 `AF_initDataCallback` 或 `gemini`）且 `content-type: text/html` | `AVAILABLE` | 0.93 |
| 5 | `server: ESF` 且 status `200`（E5 实测）作为 4 的加分证据（不改结论，只 +0.02 conf） | — | — |
| 6 | body 含 `(?i)(unusual traffic|sorry, we couldn'?t verify|automated queries)` | `CHALLENGE` | 0.85 |
| 7 | status `200` 且 body `< 5000` 且无业务指纹 | `DEGRADED` | 0.60 |
| 8 | status `5xx` | `DEGRADED` | 0.75 |

#### P5 · OpenAI API（`api.openai.com/v1/models`）

| 优先级 | 条件 | 输出 | 理由 |
|---|---|---|---|
| 1 | status `401` 且 body 含 `(?i)(missing bearer|incorrect api key|invalid_api_key|no api key)` | `AVAILABLE`（`api_error_code=NO_CREDENTIAL_PATH_OK`） | **无 key 的 401 = 网络通、服务可达**，这是设计前提（brief §2.1 P5） |
| 2 | status `403` 且 body 含 `(?i)(unsupported_country|country, region, or territory not supported)` | `REGION_BLOCKED` | 落地国不被接受 |
| 3 | status `429` 且 body 含 `(?i)(insufficient_quota|exceeded your current quota)` | `DEGRADED` | 账号无额度，非网络 |
| 4 | status `429` 且 body 含 `(?i)(rate limit|requests per min|tokens per min)` | `DEGRADED` | 限频，非网络 |
| 5 | status `403` 且 header `cf-mitigated` 存在 | `CHALLENGE` | CF 拦 API 流量（少见但可能） |
| 6 | status `200` 且 `content-type: application/json` | `AVAILABLE`（说明竟然带了凭据或端点开放） | — |
| 7 | `content-type` 非 JSON 且 body 是 HTML（命中 `H-3e`） | `HIJACKED` | 被劫持 |
| 8 | status `5xx` | `DEGRADED` | 服务端故障 |

> 实测 E7：`401` + `{"error":{"message":"Missing bearer authentication in header"}}` → 命中优先级 1 → `AVAILABLE`，conf 0.97。

#### P2 · YouTube（页面 + 媒体）

| 优先级 | 条件 | 输出 | conf |
|---|---|---|---|
| 1 | T1 `200` + `ytInitialPlayerResponse`，且 T2 `200` + `image/jpeg` + `bytes ≥ 8000` | `PASS` | 0.92 |
| 2 | T1 `200`，T2 失败 | `PASS_WITH_NOTE`（媒体 CDN 不可达） | 0.70 |
| 3 | T1 body 含 `(?i)(sign in to confirm you'?re not a bot|unusual traffic)` | `CHALLENGE` | 0.88 |
| 4 | T1 body 含 `(?i)(not available in your country|video unavailable in your country)` | `REGION_BLOCKED` | 0.92 |
| 5 | T1 `200` + `ytInitialPlayerResponse` 且 T4 `206` | `PASS`（`media_path=gvt1_throughput`） | 0.95 |
| 6 | T1 `302` → `consent.youtube.com` | `DEGRADED`（同意墙，不影响播放能力） | 0.65 |
| 7 | T1 `5xx` \| `error_code!=""` | `UNREACHABLE` \| 按 R0 | — |

### 5.6 置信度算法

基准置信度来自规则表（`conf_rule`），再做**证据调节**：

```
conf = clamp( conf_rule
              + 0.03·[有明确 header 信号]
              + 0.02·[body 命中 ≥2 条独立指纹]
              − 0.10·[body 被截断且关键指纹位于未读区(见下)]
              − 0.05·[status 与指纹语义矛盾，如 200 但含 block 文案]
            , 0.30, 0.99 )
```

- **截断惩罚判定**：若 `bytes_read == 65536`（即触及上限）**且**正特征未命中 → 说明可能只是把正文截掉了，`−0.10`；同时置 `evidence.body_truncated=true`。
- **指纹矛盾**：如 `status==200` 且 body 含 `Just a moment` → 按 CHALLENGE 但 `−0.05`（可能是错误页被 200 包装）。
- 所有 `conf < 0.70` 的结论在 UI 上必须显示「不确定」并附 `reason_code`，且**不参与 AI 分的封顶惩罚**（§6.5 只对 `conf ≥ 0.75` 的 REGION_BLOCKED/HIJACKED 生效），避免误封杀。

### 5.7 证据字段（报告里必须可回放）

```go
type AIVerdict struct {
    Status     AIStatusEnum `json:"status"`
    Confidence float64      `json:"confidence"`      // 0..1，2 位小数
    ReasonCode string       `json:"reason_code"`     // 如 CF_MITIGATED_CHALLENGE_HEADER
    RuleID     string       `json:"rule_id"`         // 如 AI-R11 / P3-1
    Evidence   struct {
        HTTPStatus    int               `json:"http_status"`
        FinalURL      string            `json:"final_url"`
        RedirectChain []RedirectHop     `json:"redirect_chain"`
        Headers       map[string]string `json:"headers"`        // 白名单
        Title         string            `json:"title"`
        BytesRead     int               `json:"bytes_read"`
        BodyTruncated bool              `json:"body_truncated"`
        MatchedRules  []string          `json:"matched_rules"`  // ["B-CF-1","B-CF-6"]
        Excerpt       string            `json:"excerpt"`        // 命中位置前后 80 字符，脱敏
    } `json:"evidence"`
}
```

- `Excerpt` **必须脱敏**：正则替换 `[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+`、`\b\d{1,3}(\.\d{1,3}){3}\b`（除目标站点 IP）、连续 32+ hex 串 → `«redacted»`。理由：body 里可能夹带用户可识别信息（虽然我们不登录，但 CDN 可能返回地域化内容）。

### 5.8 `attribution`：阶段级归因码（ux-researcher 要求，可指导用户「该动哪里」）

`verdict` 回答「什么状态」，`attribution` 回答「问题出在哪一层、用户该动什么」。二者正交，必须同时输出。

| `attribution` | 触发条件（引擎判定） | 用户动作 |
|---|---|---|
| `DNS_POLLUTED` | DNS 返回私有/保留 IP（§11.4 网段表命中）、或 `dns_source=local` 且解析到明显错误 IP、或 NXDOMAIN 但对照域名正常 | 换 DNS / 改用 `socks5h` 远端解析 |
| `TCP_RESET` | `TCP_RESET` error_code（常见于 TLS 前后被 RST） | 换节点/换协议 |
| `TLS_MITM_SUSPECT` | `TLS_CERT_*` 错误、或 `tls.version<1.2`、或证书链签发者非预期 | **立即停用该代理** |
| `CHALLENGE` | `verdict=CHALLENGED` | 换落地 IP（同节点换 IP 段） |
| `REGION_BLOCKED` | `verdict=BLOCKED_BY_POLICY` 且命中地区特征 | 换落地国家 |
| `RATE_LIMITED` | `verdict=RATE_LIMITED`（含 `Retry-After` 存在） | 降低频率/等待；非线路问题 |
| `QUOTA_EXHAUSTED` | `429 insufficient_quota` / `billing` 类 body | 充值；与代理无关 |
| `FORBIDDEN` | `403` 且无 challenge/无地区特征 | 换节点可能无效（先看 `why`） |
| `AUTH_REQUIRED` | 跳到登录域 | 正常现象，无需处理 |
| `BODY_TAMPERED` | 命中 H-3 系列或 `suspected_tampering` | **立即停用** |
| `TRUNCATED` | `Content-Length` 存在但 `bytes_read < 0.9×CL` 且非 Range 请求 | 换节点（中间设备截断） |
| `THROTTLED` | 吞吐 < 0.5 Mbps 持续 ≥5 s，或 `decay_ratio < 0.4`（限速特征） | 换节点/避开高峰 |
| `TIMEOUT` | `verdict=TIMEOUT` | 换节点 |
| `NONE` | 一切正常（仅 `PASS`） | — |

`attribution` 允许多值（数组，按严重度排序），首个为「主因」。**严禁**在无证据时给 `FORBIDDEN`/`THROTTLED` 之类推断性结论（置信度规则同 §5.6）。

### 5.9 `diagnosis → verdict` 映射表（双层模型的唯一实现点）

PRD 九态是唯一 UI 词表（§0.4(a)）。本文 §5.2 的 7 值 `AIStatusEnum` 与新增的 `INCONCLUSIVE` 等语义，**统一经此表折算**：

| 内部 `ai_status` / `diagnosis` | 对外 `verdict` | 备注 |
|---|---|---|
| `AVAILABLE` | `PASS` | — |
| `AVAILABLE` + P2 `media_path != googlevideo_real` | `PASS_WITH_NOTE` | 页面通、真实播放分片未验证 |
| `AUTH_REQUIRED`（P3/P4） | `PASS_WITH_NOTE` | 网络通、需登录，属正常 |
| `AUTH_REQUIRED`（P5 的 401） | `PASS` | PRD 明确：401 = 服务可达（pm §P5 硬指标） |
| `UNSTABLE` | `PASS_WITH_NOTE` | 能用但抖 |
| `DEGRADED`（慢） | `SLOW` | — |
| `DEGRADED`（内容被弱化/占位页） | `PASS_WITH_NOTE` | 有响应但业务指纹缺失 |
| `CHALLENGE` | `CHALLENGED` | — |
| `REGION_BLOCKED` | `BLOCKED_BY_POLICY` | — |
| `RATE_LIMITED` / `QUOTA_EXHAUSTED` | `RATE_LIMITED` | — |
| `HIJACKED` | `NET_FAIL` + `security.suspected_tampering=true` | 严重度由 security 段红警表达，不靠 verdict |
| `TLS_ERROR` | `NET_FAIL` + `security.tlsVerification=false` | 与劫持同一出口，靠 `attribution` 区分 |
| `UNREACHABLE` | `NET_FAIL` | — |
| `TIMEOUT` | `TIMEOUT` | 与 `NET_FAIL` 必须可区分（pm 硬指标） |
| 超时/失败被预算裁掉未测 | `SKIPPED` | 带 `skipReason`，**不参与分母** |
| `INCONCLUSIVE`（无法确认） | `NET_FAIL` | **宁红勿绿**：假绿是一票否决红线；`why` 必须写「无法确认可用性」 |

> 该表决定了「假绿不可能发生」：任何 `conf < 0.70`、`body_truncated` 且无正指纹、或三信号不一致的情形，都落到 `NET_FAIL` / `PASS_WITH_NOTE`，**没有一条路径能在证据不足时输出 `PASS`**。


### 5.10 三信号判定矩阵（假绿一票否决红线）

ux-researcher 硬约束：**任一单一信号都不得判可用**，必须三信号组合。三信号定义：

| 信号 | 含义 | 单信号被欺骗的方式 |
|---|---|---|
| `S1 状态码` | `final_status` 语义 | CF 挑战页可返回 **200** |
| `S2 连接完整性` | TCP/TLS 是否被中途 RST、body 是否被截断（`Content-Length` vs `bytes_read`） | 握手成功后 RST（"假连接"） |
| `S3 内容指纹` | 业务正特征（`__NEXT_DATA__` / `AF_initDataCallback` / JSON error 结构 / 标题） | 注入页、截断页、门户页 |

判定矩阵（`P`=通过，`F`=不通过，`?`=不可得）：

| S1 | S2 | S3 | verdict | diagnosis | 说明 |
|---|---|---|---|---|---|
| P | P | P | `PASS` | — | 唯一可判可用的组合 |
| P | P | ?（被截断未命中） | `NET_FAIL` | `INCONCLUSIVE` | **宁红勿绿**：无法确认不得给 PASS |
| P | P | F（拿到 200 但无业务指纹） | `PASS_WITH_NOTE` | `CONTENT_UNVERIFIED` | 有响应但内容弱，UI 标「需注意」 |
| P | F（截断/篡改） | — | `NET_FAIL` | `TAMPERED` / `TRUNCATED` | + `security.suspected_tampering` |
| P | P | F（命中挑战/地区特征） | `CHALLENGED` / `BLOCKED_BY_POLICY` | — | 按 §5.5 规则表 |
| F（401/403 凭据语义） | P | P（JSON error 结构） | `PASS_WITH_NOTE` | `AUTH_REQUIRED` | P5 的 401、P3/P4 的登录页 |
| F（429 配额/限频） | P | P | `RATE_LIMITED` | `QUOTA_EXHAUSTED` / `RATE_LIMITED` | 网络通，账号/频率侧 |
| F（4xx 政策） | P | F（地区文案） | `BLOCKED_BY_POLICY` | `REGION_BLOCKED` | — |
| F（5xx） | P | — | `SLOW`？→ 否，用 `NET_FAIL` | `UPSTREAM_5XX` | 服务端故障，非代理问题 |
| — | F（TCP/TLS 层） | — | `NET_FAIL` / `TIMEOUT` | `TLS_ERROR` / `TCP_RESET` | 按 §7.4 错误码 |

**实现规则**：`Classify()` 返回值必须是 `{verdict, diagnosis, ai_status, confidence, why, evidence}`；`ai_status` 保留本文 §5.2 的 7 值（详情层），`verdict` 走 §0.4(a) 映射。

### 5.11 `why` 短语（首屏文案，禁用状态码术语）

**(a) `why`（≤ 28 汉字，首屏可直接渲染，禁用状态码术语）**

| verdict | why 模板（例） |
|---|---|
| `PASS` | 「页面正常打开，内容校验通过」 |
| `PASS_WITH_NOTE` | 「可访问，但需登录才能确认完整功能」 |
| `CHALLENGED` | 「返回 403，命中 Cloudflare 人机验证特征」 |
| `BLOCKED_BY_POLICY` | 「提示该地区不可用，落地 IP 不被接受」 |
| `RATE_LIMITED` | 「请求被限频或额度不足，网络本身通畅」 |
| `SLOW` | 「可达但下行只有 0.4 Mbps，明显偏慢」 |
| `TIMEOUT` | 「8 秒内未收到响应」 |
| `NET_FAIL` | 「连接被重置（疑似被阻断）」 |
| `SKIPPED` | 「本次未测」+ `skipReason` |

`why` 由 `whyTemplate(verdict, diagnosis, metrics)` 生成：**模板在代码里，占位符只允许数值与已脱敏字符串**（防注入/防泄露）。

**(b) `attribution` 阶段级归因枚举**：完整 14 值表见 §5.8（`DNS_POLLUTED` / `TCP_RESET` / `TLS_MITM_SUSPECT` / `CHALLENGE` / `REGION_BLOCKED` / `RATE_LIMITED` / `QUOTA_EXHAUSTED` / `FORBIDDEN` / `AUTH_REQUIRED` / `BODY_TAMPERED` / `TRUNCATED` / `THROTTLED` / `TIMEOUT` / `NONE`），可多值、按严重度排序、首项为主因。

两条关键区分（ux-researcher 硬要求）：

- `RATE_LIMITED`（429）与 `FORBIDDEN`（403）**必须分开**：前者换节点可能有效，后者换了也白换。
- `THROTTLED`（能通但被限速）与 `SLOW`（链路本身窄）用「P1 × P6 交叉」区分：`p50 < 2 Mbps` 且 `tls_ms < 150 ms` → `THROTTLED`（延迟正常但吞吐被压）。

---

## 6. 评分模型（分段线性，全透明）

### 6.1 两层结构：gate（裁决）→ quality（附录）

```
Layer 1  gate      业务态门槛（§6.1.1），产出 verdict + gate_level   ← 主结论，UI 首屏
Layer 2  quality   0.40·Speed + 0.30·Stability + 0.30·AI            ← 附录，折叠展示
```

**禁止**用 quality 反向覆盖 gate（「地区拦截 + 链路满分 = 92 优秀」是被禁的输出）。

#### 6.1.1 gate_level 判定（门槛，与权重无关）

| gate_level | 触发条件（按优先级） | quality 封顶 | 首屏结论形态 |
|---|---|---|---|
| `BLOCKED` | 任一核心项（P3/P4/P5）`verdict ∈ {BLOCKED_BY_POLICY, NET_FAIL}` 且 `diagnosis=REGION_BLOCKED/TAMPERED`，或 ≥2 个核心项 `NET_FAIL`/`TIMEOUT` | **29**（E 档顶） | 「不可用：<原因>」 |
| `RESTRICTED` | 任一核心项 `verdict ∈ {CHALLENGED, RATE_LIMITED}` 且 `conf ≥ 0.75` | **69**（C 档顶） | 「受限：<原因>（链路本身<评价>）」 |
| `DEGRADED` | 核心项全 `PASS/PASS_WITH_NOTE`，但 `Speed` 或 `Stability` 子项 < 55 | 不封顶 | 「可用但偏<慢/不稳>」 |
| `AVAILABLE` | 核心项全 `PASS`，且 `Speed ≥ 55`、`Stability ≥ 55` | 不封顶 | 「可用」 |
| `UNKNOWN` | 核心项 `diagnosis=INCONCLUSIVE` 占比 ≥ 1/3 | 不封顶 | 「无法确认，建议复测」 |

- `SKIPPED` 的 probe **不计入 gate 判定，也不计入 AI 分分母**（按已测项归一化）。
- `gate_level` 一旦为 `BLOCKED/RESTRICTED`，`quality_score` 必须打上 `capped=true` 并记录 `cap_reason`。

#### 6.1.2 quality 分项权重（附录计算）

| 分项 | 权重 | 数据来源 |
|---|---|---|
| `Speed` | **0.40** | P1（主）+ P2 `startup_ms`（辅助，仅当 P1 无效时） |
| `Stability` | **0.30** | P6（延迟主指标用 `tls_ms`，见 §0.5(a)） |
| `AI Access` | **0.30** | P3 / P4 / P5 / P2（`SKIPPED` 项按重归一化） |
| `quality_score` | 1.00 | `0.40·Speed + 0.30·Stability + 0.30·AI`，再受 §6.1.1 封顶 |

> 权重理由（`formula_id = "PL-SCORE-1.1"`）：本产品回答「能不能顺畅用 AI」，故 Speed 与 AI 并列最高；Stability 决定「卡不卡」的体验方差，略低。注意：**权重只作用于附录分，不作用于裁决**。

### 6.2 Speed 分项

**吞吐归一化 `thrScore`（输入 `p50_mbps`，分段线性，端点外 clamp）**

| 断点 (Mbps) | 0 | 2 | 5 | 10 | 20 | 50 |
|---|---|---|---|---|---|---|
| 分数 | 0 | 40 | 60 | 80 | 95 | 100 |

```
thrScore(p) = lerp 到相邻断点；p ≤ 0 → 0；p ≥ 50 → 100
例：p = 42.3 → 95 + (42.3−20)/(50−20) × (100−95) = 98.72
```

**TTFB 归一化 `ttfbScore`（输入 P1 的 `ttfb_ms`）**

| 断点 (ms) | ≤100 | 300 | 600 | 1000 | 2000 | ≥3000 |
|---|---|---|---|---|---|---|
| 分数 | 100 | 90 | 70 | 50 | 20 | 0 |

**基础分**

```
speed_base = 0.75 × thrScore + 0.25 × ttfbScore
```

**惩罚（只减不乘，理由：乘法在低分时惩罚过强，用户无法理解）**

```
penalty_decay = clamp((0.85 − decay_ratio) × 100, 0, 10)   // decay_ratio 越小扣越多，上限 10
penalty_cv    = clamp((cv − 0.15) × 100,        0, 15)      // 抖动越大扣越多，上限 15
Speed = clamp(speed_base − penalty_decay − penalty_cv, 0, 100)
```

**P1 无效时的降级**：若 `P1.streams_failed ≥ n/2` 或 `samples_valid < P1_MIN_SAMPLES`：
- `Speed` 改用 P2：`thrScore(P2.media_mbps ?? 0) × 0.85`（打 15% 折扣，因 P2 样本小），`ttfbScore` 用 `P2.startup_ms`，再走同样的惩罚项；
- `score.speed.source = "P2_FALLBACK"` 必须在 UI 显示。

### 6.3 Stability 分项

| 子项 | 权重 | 归一化 |
|---|---|---|
| `successScore` | 0.50 | `success_rate × 100`（线性） |
| `jitterScore` | 0.22 | 断点 (ms)：5→100, 15→90, 30→75, 60→50, 120→25, 200→0 |
| `timeoutScore` | 0.16 | `(1 − timeout_rate) × 100` |
| `streakScore` | 0.12 | 断点（连续失败次数）：0→100, 1→85, 2→65, 3→40, ≥5→0 |

```
Stability = 0.50·successScore + 0.22·jitterScore + 0.16·timeoutScore + 0.12·streakScore
```

### 6.4 AI Access 分项

**`verdict` → 分值映射（v1.1：按 PRD 九态）**

| verdict | 分值 | 备注 |
|---|---|---|
| `PASS` | 100 | 唯一满分 |
| `PASS_WITH_NOTE` | 90 | 含 `AUTH_REQUIRED` / `INCONCLUSIVE` / `UNSTABLE` 等「通但需注意」 |
| `SLOW` | 60 | — |
| `RATE_LIMITED` | 45 | 网络通、账号/频率侧受限 |
| `CHALLENGED` | **25** | 代理 IP 信誉问题，用户应换节点 |
| `BLOCKED_BY_POLICY` | **0** | 地区/政策拒绝 |
| `TIMEOUT` | **0** | — |
| `NET_FAIL` | **0** | 含 `TLS_ERROR` / `TAMPERED`（后者另有 §6.5 CAP-4 更重惩罚） |
| `SKIPPED` | 不计入 | 从分母剔除，权重重归一化 |

> P5 的 401 属 `PASS_WITH_NOTE`（90 分），不是 100——因为它只证明「网络通、服务可达」，不证明「你的账号能用」。

**聚合**

```
w = { P3: 0.30, P4: 0.30, P5: 0.25, P2: 0.15 }   // 仅对非 SKIPPED 项求和并重归一化
ai_base = Σ w_i × points_i
confFactor = 0.85 + 0.15 × avg(conf_i)          // 置信度只做 ±15% 修正，不改变 verdict
AI = ai_base × confFactor
```

### 6.5 严重度惩罚（只作用于 quality，不改变 verdict）

| # | 触发条件 | 效果 |
|---|---|---|
| CAP-1 | gate = `BLOCKED` | `quality_score ≤ 29` |
| CAP-2 | gate = `RESTRICTED` | `quality_score ≤ 69` |
| CAP-3 | 任一核心项 `PASS_WITH_NOTE` 且 `diagnosis=INCONCLUSIVE` | `warnings += ["inconclusive:<probe>"]`，quality 不打折（宁红勿绿已由 verdict 承担） |
| CAP-4 | `security.suspected_tampering=true` | `quality_score ≤ 29`，且 UI 独立红警「疑似劫持，建议立即停用该代理」（不依赖 verdict 表达严重度） |
| CAP-5 | `Stability.state == "DOWN"` | `quality_score ≤ 54` |
| CAP-6 | `Speed.source == "P2_FALLBACK"` | `quality_score ≤ 75` |
| CAP-7 | `verdict=SKIPPED` 的项 ≥ 3 | `quality_score ≤ 75` + 标「样本不足」 |

所有惩罚写入 `score.caps_applied: ["CAP-2"]` 与 `score.cap_reason`，UI 必须可展开查看——用户要能知道「为什么链路 85 分却只给 C」。

### 6.6 档位（PRD 五分档，锁定）

| 档 | 区间 | 语义 | 一句话结论模板 |
|---|---|---|---|
| A | 85–100 | 优 | 代理质量优秀，AI 服务全通且快 |
| B | 70–84 | 良 | 良好，个别站点被风控或速度有波动 |
| C | 55–69 | 可用 | 可用，但存在明显短板（见分项） |
| D | 30–54 | 较差 | 体验差，建议更换代理 |
| E | 0–29 | 不可用 | 不可用，AI 服务基本无法访问 |

档位只描述 `quality_score`；**最终结论以 `verdict` 为准**（可能 A 档分数 + `RESTRICTED` 裁决 同时出现，此时封顶已生效，不会真出现 A 档 + RESTRICTED）。

### 6.7 完整算例（v1.1，gate 修正后）

**场景**：Standard，`network.mode="system"`（TUN），`run_id=0192f3c1-…`。

**原始数据**

| 来源 | 字段 | 值 |
|---|---|---|
| P1 | `throughput.p50_mbps` | 42.3 |
| P1 | `ttfb_ms` | 380 |
| P1 | `cv` | 0.21 |
| P1 | `decay_ratio` | 0.78 |
| P1 | `streams_failed` | 0 / 5 |
| P1 | `tls_ms` | 132（延迟主指标） |
| P6 | `ttfb_ms[]`(10) | 182,179,186,181,177,190,183,180,188,179 |
| P6 | `success_rate`/`timeout_rate`/`fail_streak` | 1.0 / 0.0 / 0 |
| P3 | 403 + `cf-mitigated: challenge` | `verdict=CHALLENGED`，`conf 0.95` |
| P4 | 200 + `<title>Gemini</title>` + `server: ESF` | `PASS`，`conf 0.93` |
| P5 | 401 + `Missing bearer authentication` | `PASS_WITH_NOTE`（`AUTH_REQUIRED`），`conf 0.97` |
| P2 | T1 200 + `ytInitialPlayerResponse`；T2 200(21,011 B)；T4 206 | `PASS`，`media_path=gvt1_throughput`，`conf 0.90` |

**Step 0 — gate（先算这个）**

```
P3 = CHALLENGED (conf 0.95 ≥ 0.75)  →  命中 RESTRICTED
gate_level = RESTRICTED   →   quality 封顶 69（CAP-2）
verdict（V1 主结论）= 「受限：ChatGPT 被 Cloudflare 人机验证拦截（链路本身良好）」
```

**Step 1 — Speed（附录）**

```
thrScore  = 95 + (42.3−20)/(50−20)×5          = 98.7167
ttfbScore = 90 + (380−300)/(600−300)×(70−90)  = 84.6667
speed_base = 0.75×98.7167 + 0.25×84.6667      = 95.2042
penalty_decay = (0.85−0.78)×100 = 7.00 ；penalty_cv = (0.21−0.15)×100 = 6.00
Speed = 95.2042 − 13.00 = 82.2042 → 82.2
```

**Step 2 — Stability（附录）**

```
diffs = [3,7,5,4,13,7,3,8,9] → median jitter = 7 ms
jitterScore = 100 + (7−5)/(15−5)×(90−100) = 98.0
Stability = 0.50×100 + 0.22×98.0 + 0.16×100 + 0.12×100 = 99.56 → 99.6
（SKIPPED 项为 0，无重归一化）
```

**Step 3 — AI Access（附录）**

```
points: P3=25(CHALLENGED) P4=100(PASS) P5=90(PASS_WITH_NOTE/AUTH_REQUIRED) P2=100(PASS)
ai_base = 0.30×25 + 0.30×100 + 0.25×90 + 0.15×100 = 7.50+30.00+22.50+15.00 = 75.00
avgConf = (0.95+0.93+0.97+0.90)/4 = 0.9375
confFactor = 0.85 + 0.15×0.9375 = 0.990625
AI = 75.00 × 0.990625 = 74.297 → 74.3
```

**Step 4 — quality 与封顶**

```
quality_raw = 0.40×82.2042 + 0.30×99.56 + 0.30×74.297
            = 32.8817 + 29.8680 + 22.2891 = 85.0388 → 85.0
gate = RESTRICTED → quality = min(85.0, 69) = 69.0  （caps_applied=["CAP-2"]）
档位 = C（55–69）
```

**最终输出（用户看到的）**

```
V1 裁决：受限  RESTRICTED
原因：ChatGPT 被 Cloudflare 人机验证拦截（返回 403，命中 cf-mitigated=challenge）
链路质量（附录）：69.0 / C        ← 原始 quality_raw 85.0 记录在报告 inputs 里
  速度 82.2（42.3 Mbps / TTFB 380ms / 衰减 0.78）
  稳定 99.6（10/10 通，抖动中位数 7ms）
  AI 可达 74.3（Gemini ✓ / OpenAI API ✓ / YouTube 边缘 ✓ / ChatGPT ✗ 挑战）
建议：更换落地 IP 或节点后复测 ChatGPT
```

**对照组（同环境直连）**：P1 0.8 Mbps → Speed ≈ 12；P6 success 0.4 → Stability 31；P3/P4/P5 全 `NET_FAIL` → `gate_level=BLOCKED` → quality 封顶 29 → E 档。裁决「不可用」，差值 40 分即「代理带来的收益」。

> 注意算例体现的设计意图：即使原始质量分 85.0（A 档），**只要 ChatGPT 被判 CHALLENGED，主结论必须是「受限」而不是「优秀」**。这就是 gate-not-weight 的落地差异。
## 7. 并发调度、超时预算与重试

### 7.1 调度波次（为什么不能全并行）

**Run 级准入（security / pm 硬约束）**：全局同时只允许 **1 个 Run**，已有运行中 → `POST /api/runs` 返回 **409** + `retry_after_ms`；单 Run 目标域名数 **≤ 8**；连续 Run 冷却 **≥ 10 s**。

```
t=0 ┌─ Wave 1（并发上限 4）────────────────────────────┐
    │ P6(3s)  P5(1s)  P3(2s)  P4(3s)                  │  ← 先出「AI 能不能用」结论（产品最高价值）
    └──────────────────────────────────────────────────┘
t≈0~20s ┌─ Wave 2（独占 1）───────────────────────────┐
        │ P1（6 流 × 8 MiB，≤30 s）                  │  ← 独占，避免其它 probe 抢带宽污染 p50
        └────────────────────────────────────────────┘
t≈+30s  ┌─ Wave 3（并发上限 2）──────────────────────┐
        │ P2（T1 页面 → T2 缩略图 → T3 分片）        │  ← 放最后，因其可能抢带宽
        └────────────────────────────────────────────┘
```

- Wave 1 与 Wave 2 之间**不等待**：Wave 2 在 `t = min(Wave1 完成, 8 s)` 启动，Wave 1 的残余 probe 若不占用 > 1 MB 带宽则允许并存（P3/P4/P5 body 上限 64 KiB，可忽略）。
- **全局信号量**：`connSem = 16`（所有活跃 TCP 连接总数），`dnsSem = 8`。P1 的 6 流必须一次性拿满 6 个 token，拿不满则降为实际可用流数并记 `streams_effective`。
- **worker pool**：每个 probe 内部用 `errgroup.Group` + 固定 goroutine 数；**不使用无界 goroutine**。P1 = 6 个 stream goroutine + 1 个采样 ticker；P6 = 串行（必须串行，否则 RTT 互相干扰）。

### 7.2 超时预算（硬数值）

| 层级 | 常量 | Quick | Standard | Deep |
|---|---|---|---|---|
| Run 总预算 | `RUN_TIMEOUT` | 25 s | **75 s** | 200 s |
| P1 | `P1_TIMEOUT` | 12 s | **30 s** | 60 s |
| P2 | `P2_TIMEOUT` | 10 s | **25 s** | 40 s |
| P3 | `P3_TIMEOUT` | 10 s | **15 s** | 15 s |
| P4 | `P4_TIMEOUT` | 12 s | **20 s** | 20 s |
| P5 | `P5_TIMEOUT` | 8 s | **10 s** | 10 s |
| P6 | `P6_TIMEOUT` | 18 s | **35 s** | 70 s |
| 单请求 DNS | `DIAL_DNS_TIMEOUT` | 4 s | 4 s | 4 s |
| 单请求 TCP 建连 | `DIAL_TIMEOUT` | 5 s | 5 s | 5 s |
| TLS 握手 | `TLS_HANDSHAKE_TIMEOUT` | 5 s | 5 s | 5 s |
| 等响应头 | `RESPONSE_HEADER_TIMEOUT` | 8 s | 8 s | 8 s |
| 空闲连接复用 | `IDLE_CONN_TIMEOUT` | 30 s | 30 s | 30 s |

**预算自检**：`sum(各 probe 最长路径) = 35+30+25+15+20+10 ≈ 75 s`（Wave1 取最大 35，Wave2 30，Wave3 25）→ 与 `RUN_TIMEOUT=75` 对齐，**不允许再增加 probe 而不改总预算**。

### 7.3 取消传播

```
main.runCtx (WithTimeout RUN_TIMEOUT)
  ├─ probeCtx = WithTimeout(runCtx, Px_TIMEOUT)
  │    └─ reqCtx = httptrace.WithClientTrace(probeCtx, trace)   // 请求级 = probe 级 + trace
  └─ cancelAll() ← 用户点「中断」/ 收到 SIGINT(Windows: Ctrl+C handler) / 进程退出
```

- 取消是**单向广播**：`cancelAll()` → `probeCtx` 全死 → 所有 `body.Read` 立即返回 `context.Canceled`。
- **客户端断连（SSE 关闭）不取消 Run**：报告仍需落盘/可导出（brief §2.5 要求可中断、可导出）。只有显式 `POST /api/runs/{id}/cancel` 才取消。
- 取消后仍要产出 `probe_result`（`status="CANCELED"`）+ `run_end(reason="canceled")`，并**照常计算分数**（用已有样本），标记 `report.partial=true`。

### 7.4 错误分类（`error_code`）

| code | 触发 | 可重试 | 语义 |
|---|---|---|---|
| `DNS_NXDOMAIN` | `*net.DNSError{IsNotFound:true}` | 否 | 域名不存在（可能被 DNS 污染） |
| `DNS_TIMEOUT` | DNSError.IsTimeout | 是(1) | — |
| `DNS_SERVFAIL` | DNSError.IsTemporary | 是(1) | — |
| `PROXY_CONNECT_FAIL` | `proxyconnect tcp: ...` / CONNECT 返回非 200 | 是(1) | 代理本身坏了 |
| `PROXY_AUTH_FAIL` | HTTP 407 | 否 | 代理认证失败 |
| `TCP_REFUSED` | `ECONNREFUSED` | 否 | — |
| `TCP_RESET` | `ECONNRESET` / `EPIPE` | 是(1) | 常见于被 RST（GFW 特征） |
| `TCP_TIMEOUT` | `Dial` 超时 | 是(1) | — |
| `TLS_HANDSHAKE_FAIL` | TLS alert/EOF | 是(1) | — |
| `TLS_TIMEOUT` | 握手超时 | 是(1) | — |
| `TLS_CERT_INVALID` / `TLS_CERT_UNKNOWN_AUTHORITY` / `TLS_CERT_NAME_MISMATCH` | `x509.*Error` | 否 | → AI 判 `HIJACKED`（H-3a） |
| `HTTP_TIMEOUT` | `ResponseHeaderTimeout` / ctx 截止 | 否 | 慢或黑洞 |
| `BODY_READ_ERROR` | `io.ErrUnexpectedEOF` 等 | 是(1) | 传输被掐断 |
| `TOO_MANY_REDIRECTS` | `CheckRedirect` 触发 8 跳 | 否 | 疑似钓鱼/劫持 |
| `CONTEXT_CANCELED` | 用户中断 | 否 | — |
| `UNSUPPORTED_SCHEME` | 代理 URL scheme 非 4 种 | 否 | 配置错误 |

**分类实现**：单函数 `classifyErr(err error) ErrorCode`，用 `errors.As` 链（`*net.DNSError` → `*net.OpError` → `*os.SyscallError` → `x509.UnknownAuthorityError` → `context.DeadlineExceeded`），最后用 `strings.Contains` 兜底匹配 `proxyconnect`。**这是单测重点**（§13）。

### 7.5 重试策略

| Probe | 重试条件 | 次数 | 退避 |
|---|---|---|---|
| P1 | **只在整个 probe 0 字节且 error 可重试**时重试整体 | 1 | `500 ms + rand(0..250)ms` |
| P1 单流 | **不重试**（缺流用剩余流聚合，记 `streams_failed`） | 0 | — |
| P2 | T1 可重试错误 → 重试 T1；T2 失败只降级不重试 | 1 | 同上 |
| P3 / P4 | 仅 `TCP_RESET`/`TLS_HANDSHAKE_FAIL`/`HTTP2_PROTOCOL_ERROR` 重试；**收到任何 HTTP 响应即不重试**（403/429 是结论，不是错误） | 1 | 同上 |
| P5 | 同上 | 1 | 同上 |
| P6 | **单样本不重试**（每样本即数据点）；但若前 3 个样本同一 `hard_fail` code → 提前终止（fail-fast），失败码填满剩余样本 | 0 | — |

- 退避的随机数来自 `runSeed` 派生的 `math/rand/v2` 实例，**不污染全局**，保证可复现（§12）。
- **绝不重试 4xx**：403/429/451 是判定输入，重试会污染 `fail_codes` 分布并增加对目标的压力。

---

## 8. 事件流协议（SSE）

### 8.1 传输约定

| 项 | 值 |
|---|---|
| 路由（arch 定稿） | `GET /api/runs/{id}/events`（SSE）<br>`GET /api/runs/{id}`（**断线恢复快照**）<br>`POST /api/runs`（创建，已有运行中→409）<br>`POST /api/runs/{id}/cancel`（幂等）<br>`POST /api/runs/{id}/export`（导出，不重探测）<br>`POST /api/diagnose/tls`（证书诊断） |
| 鉴权 | `pl_sess` cookie（`SameSite=Strict`、`HttpOnly`）+ `?t=<token>` 兜底（EventSource 无法设置自定义头）；`OPTIONS` 一律 405 且不带任何 `Access-Control-*` |
| 推送频率 | **4 Hz（250 ms 聚合）**——内部 100 ms tick 供算法使用，`probe.sample` 每 250 ms 合并一次发出 |
| 响应头 | `Content-Type: text/event-stream; charset=utf-8`、`Cache-Control: no-store`、`X-Accel-Buffering: no`、`Connection: keep-alive` |
| 帧格式 | `event: <type>\nid: <seq>\ndata: <json>\n\n`（`seq` 单调递增，从 1 开始，用于断线补发） |
| 心跳 | 每 **15 s** 发 `event: ping\ndata: {"t":<unix_ms>}`（防代理层掐断空闲连接） |
| 重连 | 浏览器 `EventSource` 自带重连；服务端支持 `Last-Event-ID` 头 → 从 `seq+1` 补发（内存保留最近 2000 条） |
| 编码 | UTF-8，单帧 `data` ≤ 8 KiB（超限则拆分/裁剪，绝不阻塞） |
| 缓冲 | 每连接 `chan []byte` 容量 256；**满了丢最旧的 `sample` 帧**（`probe_result`/`score`/`run_end` 永不丢，用阻塞写 + 200 ms 超时） |

### 8.2 事件类型总表（ux-arch 点分命名，覆盖早期下划线命名）

| `event` | 时机 | 频率 | 关键字段 |
|---|---|---|---|
| `run.started` | Run 开始 | 1 次 | `runId`, `epoch`, `configDigest`, `profile`, `expectedProbeCount`, `snapshotPath` |
| `egress.detected` | 出网方式解析完成（`run.started` 后 50 ms 内） | 1 次 | `primary`, `sources[]`, `conflict`（见 8.3，**必须多来源**） |
| `probe.started` | 单 probe 开始 | ≤6 次 | `probeId`, `timeoutMs`, `plannedRequests`, `wave` |
| `probe.stage` | 阶段切换 | 每 probe ≤6 次 | `probeId`, `stage`, `atMs` |
| `probe.sample` | 采样（**250 ms 聚合**） | ≤4 Hz | `probeId`, `tMs`, `mbps`, `mbpsRaw`, `bytesTotal`, `streamsActive`, `warm` |
| `probe.settled` | 单 probe 结束 | ≤6 次 | `probeId`, `verdict`, `diagnosis`, `aiStatus`, `why`, `confidence`, `metrics`, `evidence` |
| `probe.skipped` | 该档位不测 / 预算耗尽 / 上游失败 | 0~N 次 | `probeId`, `skipReason ∈ {cancelled, upstream_failed, budget_exhausted, profile_omitted}` |
| `probe.error` | 单 probe 内部错误但已重试/不终止 Run | 0~6 次 | `probeId`, `errorCode`, `message`（已 `Sanitize`）, `retryable`, `willRetry` |
| `score.updated` | 分项/裁决就绪 | 1~3 次 | `gateLevel`, `verdict`, `quality`, `partial`, `capsApplied` |
| `run.finished` | Run 结束 | 1 次 | `verdict`, `qualityScore`, `grade`, `reportId`, `durationMs`, `partial`, `headline` |
| `ping` | 心跳 | 每 15 s | `t` |

**`stage` 枚举（ux-arch 附录 A）**：`dns | tcp | tls | http | ttfb | body`。前端不推断阶段，只渲染引擎推的 `atMs`；可达类卡片直接把它当进度条（「卡在 TLS」本身就是诊断结论）。

**顺序契约**：`run.started → egress.detected → (probe.started → probe.stage* → probe.sample* → probe.settled|probe.skipped)* → score.updated+ → run.finished`。`probe.*` 事件必须**按 probeId 分流**（前端定向 patch，禁止全局重渲染）。

### 8.2.1 `expectedProbeCount`（档位 → 项数）

| 档位 | 参与项 | `expectedProbeCount` | 固定跳过 |
|---|---|---|---|
| `quick` | P3, P4, P5, P6 | **4** | P1、P2（`skipReason="profile_omitted"`） |
| `standard` | P1~P6 | **6** | 无 |
| `deep` | P1~P6 | **6** | 无（P6 采样 120 次） |

前端用 `settledCount + skippedCount == expectedProbeCount` 判 `done`（**不得硬编码 6**）。

### 8.3 新增/变更事件的 JSON 示例

**egress.detected**（多来源 + 冲突标记；ux-arch 必需）

```json
{ "type": "egress.detected", "runId": "0192f3c1…", "epoch": 1,
  "primary": "env",
  "sources": [
    { "source": "os",  "proxy": null, "enabled": false, "detail": "scutil: HTTPEnable=0 SOCKSEnable=0" },
    { "source": "env", "proxy": "http://127.0.0.1:61713", "enabled": true, "detail": "HTTPS_PROXY" },
    { "source": "tun", "proxy": null, "enabled": true, "detail": "default route via utun5 (fake-ip 198.18.0.0/15)" }
  ],
  "conflict": true,
  "conflictReason": "env proxy 与 TUN 同时生效，实际出网路径不确定；已按 env 优先",
  "effectiveTlsMs": 132
}
```

**probe.stage**

```json
{ "type": "probe.stage", "runId": "0192f3c1…", "probeId": "P4",
  "stage": "tls", "atMs": 214, "note": "handshake ok, alpn=h2, version=TLS1.3" }
```

**probe.sample**（P1，250 ms 聚合）

```json
{ "type": "probe.sample", "runId": "0192f3c1…", "probeId": "P1",
  "tMs": 3200, "mbps": 41.87, "mbpsRaw": 43.10,
  "bytesTotal": 16777216, "bytesBudget": 20971520,
  "streamsActive": 5, "warm": true }
```

**probe.settled**（含 `verdict` + `why` + `evidence`，前端不推断）

```json
{ "type": "probe.settled", "runId": "0192f3c1…", "probeId": "P3",
  "verdict": "CHALLENGED", "diagnosis": "CHALLENGE", "aiStatus": "CHALLENGE",
  "why": "返回 403，命中 Cloudflare 人机验证特征",
  "confidence": 0.95,
  "matchedRule": "cf_challenge_v2",
  "requestedUrl": "https://chatgpt.com/",
  "finalUrl": "https://chatgpt.com/",
  "httpStatus": 403,
  "durationMs": 1841,
  "stages": { "dns_ms": null, "tcp_ms": 0.31, "tls_ms": 121.4,
              "ttfb_ms": 176.3, "body_ms": 4.2 },
  "metrics": { "bytesRead": 8708, "connReused": false, "dnsSource": "proxy" },
  "evidence": { "headers": { "cf-mitigated": "challenge", "server": "cloudflare" },
                "title": "Just a moment...", "bytesRead": 8708,
                "bodyTruncated": false, "matchedRules": ["H-1"],
                "excerpt": "«redacted»" },
  "attribution": ["CHALLENGE"] }
```

**probe.skipped**

```json
{ "type": "probe.skipped", "runId": "0192f3c1…", "probeId": "P2",
  "skipReason": "profile_omitted", "detail": "quick 档不含大流量项" }
```

**score.updated**

```json
{ "type": "score.updated", "runId": "0192f3c1…", "partial": false,
  "gateLevel": "RESTRICTED", "verdict": "RESTRICTED",
  "quality": { "value": 69.0, "raw": 85.0, "grade": "C",
               "speed": 82.2, "stability": 99.6, "ai": 74.3,
               "capsApplied": ["CAP-2"], "capReason": "P3 CHALLENGED" },
  "formulaId": "PL-SCORE-1.1" }
```

**run.finished**

```json
{ "type": "run.finished", "runId": "0192f3c1…", "epoch": 1,
  "reason": "completed", "partial": false,
  "verdict": "RESTRICTED", "qualityScore": 69.0, "grade": "C",
  "durationMs": 41302, "reportId": "rpt_0192f3c1",
  "headline": "受限：ChatGPT 被 Cloudflare 人机验证拦截（链路本身良好）" }
```

**断线恢复快照 `GET /api/runs/{id}`**（ux-arch 必需，防状态漂移）

```json
{ "runId": "0192f3c1…", "epoch": 1, "status": "running",
  "expectedProbeCount": 6, "settledCount": 3, "skippedCount": 0,
  "seq": 412,
  "probes": { "P1": { "state": "running", "stage": "body", "lastSample": { "tMs": 3200, "mbps": 41.87 } },
              "P3": { "state": "settled", "verdict": "CHALLENGED", "why": "…" } },
  "gateLevel": null }
```

前端重连后：先拉快照对齐，再用 `Last-Event-ID: <seq>` 续订增量。

### 8.4 前端契约要点（给 ux-arch / ui-designer）

1. `probe.sample` 是唯一高频事件（**4 Hz 上限**，250 ms 聚合）；前端三层节流（采样 4 Hz → 图表 rAF ≤10 Hz → 数字 2 Hz）。
2. `score.updated.partial=true` 时未出分项为 `null`，UI 必须显示**骨架**，禁止渲染 0。
3. 首屏只显示 `verdict` + `why`；`httpStatus` / 规则 ID / 响应头等术语只进详情层。
4. `confidence < 0.70` 必须加「不确定」角标，且**不参与封顶**。
5. 取消后引擎 **≤500 ms** 停止推送（PRD US-05.1 硬验收），取消信号走独立控制通道，不排队。
6. 事件带 `runId` + `epoch`，前端按 `epoch` 丢弃过期事件（防取消后幽灵更新导致数字闪烁）。
7. `probe.skipped` 不渲染任何结论色，只标「本次未测」+ `skipReason`。
8. `raw` 中的代理地址已由引擎 `MaskProxy()` 脱敏，前端再过一遍（纵深防御）。

## 9. 数据模型与 JSON Schema

### 9.1 版本策略

- 所有持久化结构顶层必有 `schema_version: "1.1"`（string，`major.minor`；1.0→1.1 为加字段：`verdict`/`diagnosis`/`why`/`stages`/`security`）。
- **minor**：只加字段（读方必须容忍未知字段）；**major**：破坏性变更（旧报告仍可读，`report.upgrade(from, to)` 负责迁移）。
- 时间字段：ISO-8601 UTC 带毫秒 + `Z`（`2026-10-22T09:14:02.318Z`）；时长字段：`*_ms` 为 float64，**禁止**用时间戳相减得到的整数秒。
- ID：`run_id` / `report_id` 用 UUIDv7（时间有序，便于排序与分页）。

### 9.2 `RunConfig`（JSON Schema 2020-12，节选可执行部分）

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://proxylens.local/schema/run-config-1.0.json",
  "type": "object",
  "required": ["schema_version", "profile", "network"],
  "additionalProperties": false,
  "properties": {
    "schema_version": { "enum": ["1.0", "1.1"] },
    "profile": { "enum": ["quick", "standard", "deep"] },
    "network": {
      "type": "object",
      "required": ["mode"],
      "additionalProperties": false,
      "properties": {
        "mode": { "enum": ["system", "custom", "direct"] },
        "proxy_url": {
          "type": ["string", "null"],
          "pattern": "^(https?|socks5h?)://([^:@/]+(:[^@/]*)?@)?[^/?#]+$",
          "maxLength": 512
        }
      },
      "if": { "properties": { "mode": { "const": "custom" } } },
      "then": { "required": ["proxy_url"], "properties": { "proxy_url": { "type": "string" } } }
    },
    "probes": {
      "type": "array",
      "items": { "enum": ["P1", "P2", "P3", "P4", "P5", "P6"] },
      "uniqueItems": true,
      "minItems": 1,
      "default": ["P1", "P2", "P3", "P4", "P5", "P6"]
    },
    "budget": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "p1_streams":      { "type": "integer", "minimum": 1, "maximum": 8, "default": 6 },
        "p1_per_stream_mb":{ "type": "integer", "minimum": 1, "maximum": 16, "default": 8 },
        "p6_samples":      { "type": "integer", "minimum": 3, "maximum": 120, "default": 10 },
        "max_total_mb":    { "type": "integer", "minimum": 1, "maximum": 160, "default": 64 }
      }
    },
    "advanced": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "deterministic":    { "type": "boolean", "default": false },
        "insecure_skip_verify": { "type": "boolean", "default": false, "description": "仅调试，报告中标记 insecure=true" },
        "cache_buster":     { "type": "boolean", "default": true }
      }
    }
  }
}
```

> **安全约束**：`proxy_url` 含密码时**不进入报告**、不进入日志、不进入 `config_digest` 的明文（只算 `sha256(scheme://host:port)`）。

### 9.3 `Sample` / `ProbeResult`（Go 结构，即权威定义）

```go
type Sample struct {
    ProbeID     string   `json:"probe_id"`
    Index       int      `json:"index"`
    AtMs        int      `json:"at_ms"`          // 相对 probe 开始
    OK          bool     `json:"ok"`
    Timings     Timings  `json:"timings"`        // §2.5
    BytesRead   int64    `json:"bytes_read"`
    MBps        *float64 `json:"mbps,omitempty"` // 仅吞吐类样本
    MBpsRaw     *float64 `json:"mbps_raw,omitempty"`
    Status      int      `json:"status"`
    ErrorCode   string   `json:"error_code"`
    ConnReused  bool     `json:"conn_reused"`
    DNSsource   string   `json:"dns_source"`     // local|proxy|none
}

type ProbeResult struct {
    ProbeID       string        `json:"probe_id"`
    Status        ProbeStatus   `json:"status"`  // OK|PARTIAL|FAILED|CANCELED|SKIPPED
    Verdict       string        `json:"verdict"`      // PRD 九态，§0.4(a)
    Diagnosis     string        `json:"diagnosis"`    // TLS_ERROR|TAMPERED|INCONCLUSIVE|QUOTA_EXHAUSTED|…
    Why           string        `json:"why"`          // ≤28 汉字，首屏可直接渲染
    Confidence    float64       `json:"confidence"`   // 0..1
    MatchedRule   string        `json:"matched_rule"` // 规则 ID，证据区展示
    Attribution   []string      `json:"attribution"`  // §5.9(b) 12 值枚举，可多值
    SkipReason    string        `json:"skip_reason,omitempty"`
    StartedAt     time.Time     `json:"started_at"`
    DurationMs    float64       `json:"duration_ms"`
    Stages        Stages        `json:"stages"`   // dns/tcp/tls/http/ttfb/body（§8.3）
    HTTPStatus    int           `json:"http_status"`
    FinalURL      string        `json:"final_url"`
    RedirectChain []RedirectHop `json:"redirect_chain"`
    Metrics       Metrics       `json:"metrics"`
    Throughput    *Throughput   `json:"throughput,omitempty"`
    Stability     *Stability    `json:"stability,omitempty"`
    AIVerdict     *AIVerdict    `json:"ai_verdict,omitempty"` // 详情层完整证据
    MediaPath     string        `json:"media_path,omitempty"` // §1.4 五值
    Notes         []string      `json:"notes"`
    Samples       []Sample      `json:"samples"`
}

type Stages struct {
    DNSms   *float64 `json:"dns_ms"`   // 代理模式下恒为 null（§0.5(a)）
    TCPms   *float64 `json:"tcp_ms"`   // 标签：本地代理建连，非远端 RTT
    TLSms   *float64 `json:"tls_ms"`   // **延迟主指标**
    HTTPms  *float64 `json:"http_ms"`  // 响应头到达
    TTFBms  *float64 `json:"ttfb_ms"`
    Bodyms  *float64 `json:"body_ms"`
}

type Throughput struct {
    P50Mbps     float64  `json:"p50_mbps"`
    P90Mbps     float64  `json:"p90_mbps"`
    MeanMbps    float64  `json:"mean_mbps"`
    PeakMbps    float64  `json:"peak_mbps"`
    CV          *float64 `json:"cv"`
    DecayRatio  *float64 `json:"decay_ratio"`
    DecaySlope  *float64 `json:"decay_slope_mbps_per_s"`
    SamplesValid int     `json:"samples_valid"`
    StreamsEffective int `json:"streams_effective"`
    StreamsFailed    int `json:"streams_failed"`
    LimitedBy   string   `json:"limited_by"`  // bytes|duration|error
    Reliable    bool     `json:"reliable"`
}

type Stability struct {
    N           int            `json:"n"`
    SuccessRate float64        `json:"success_rate"`
    TimeoutRate float64        `json:"timeout_rate"`
    FailRate    float64        `json:"fail_rate"`
    JitterMs    *float64       `json:"jitter_ms"`       // 中位数
    JitterP90Ms *float64       `json:"jitter_p90_ms"`
    SpikeRatio  *float64       `json:"spike_ratio"`
    FailStreak  int            `json:"fail_streak"`
    FailCodes   map[string]int `json:"fail_codes"`
    RTTp50Ms    *float64       `json:"rtt_p50_ms"`
    State       string         `json:"state"`           // EXCELLENT|GOOD|FAIR|POOR|DOWN
}
```

### 9.4 `Score` 与 `Report`

```go
type Score struct {
    FormulaID  string      `json:"formula_id"`            // "PL-SCORE-1.0"
    Speed      *SubScore   `json:"speed"`
    Stability  *SubScore   `json:"stability"`
    AI         *AISubScore `json:"ai"`
    Total      *float64    `json:"total"`
    Grade      string      `json:"grade"`
    CapsApplied []string   `json:"caps_applied"`
    Inputs     map[string]float64 `json:"inputs"`         // 可复核：p50, cv, decay, ttfb, success_rate...
    Partial    bool        `json:"partial"`
}
type SubScore struct {
    Value float64            `json:"value"`
    Subs  map[string]float64 `json:"subs"`
    Source string            `json:"source,omitempty"`    // P1|P2_FALLBACK
    State  string            `json:"state,omitempty"`
}
type AISubScore struct {
    Value         float64            `json:"value"`
    Subs          map[string]float64 `json:"subs"`        // 各 probe points
    AvgConfidence float64            `json:"avg_confidence"`
    Cap           *float64           `json:"cap,omitempty"`
}
```

`Report` 顶层：

```json
{
  "schema_version": "1.0",
  "report_id": "rpt_0192f3c1",
  "run_id": "0192f3c1-7a4e-7c3b-9f21-2b6c5d8e4a10",
  "created_at": "2026-10-22T09:14:43.620Z",
  "partial": false,
  "config": { "...": "RunConfig，proxy_url 已脱敏" },
  "config_digest": "sha256:9f2b1c…",
  "environment": {
    "os": "darwin", "arch": "arm64", "go_version": "go1.26.3",
    "engine_version": "1.0.0", "app_version": "0.1.0",
    "tz": "Asia/Shanghai", "locale": "zh-Hans",
    "effective_route": { "proxy": null, "proxy_source": "env", "tun_suspected": true },
    "resolver": "system", "os_proxy_enabled": false,
    "fingerprint": "darwin/arm64|go1.26.3|PL1.0|tz=Asia/Shanghai|route=env:TUN"
  },
  "probes": [ { "probe_id": "P1", "...": "ProbeResult" } ],
  "score": { "...": "Score" },
  "security": {
    "tlsVerification": true,
    "proxyScheme": "http",
    "plaintextHop": false,
    "remoteDNS": false,
    "targetPolicy": "https+443+FQDN",
    "privateTargetsBlocked": 0,
    "suspectedTampering": false,
    "warnings": []
  },
  "headline": "受限：ChatGPT 被 Cloudflare 人机验证拦截（链路本身良好）",
  "verdict": "RESTRICTED",
  "gateLevel": "RESTRICTED",
  "warnings": ["P3 CHALLENGED: cf-mitigated=challenge", "egress conflict: env+TUN"],
  "privacy": { "credentialsIncluded": false, "bodyExcerptsRedacted": true }
}
```

**`security` 段字段契约**（`04-security.md` §4.7 要求，由引擎填充）：`tlsVerification`（证书是否校验通过；`insecure_skip_verify=true` 时为 false 且 HIJACKED 判定停用）、`proxyScheme`、`plaintextHop`（`http` 代理 + `http` 目标 = true）、`remoteDNS`（`socks5h` = true）、`targetPolicy`、`privateTargetsBlocked`（被 Dialer 拦下的次数）、`suspectedTampering`、`warnings`。

**`environment.fingerprint` 算法**（git-like，用于 A/B 可比性判断）：

```
fingerprint = join("|",
  os, arch, go_version, "PL"+engine_major_minor,
  "tz="+IANATZ, "route="+route_tag, "resolver="+resolver_kind,
  "insecure="+strconv.FormatBool(insecureSkipVerify))
route_tag = "system:os" | "system:env" | "system:tun" | "custom:"+scheme | "direct"
```

两个 Run 的 `fingerprint` 相同 → 才允许在 UI 上做严格差值对比；不同则提示「环境不一致，差值仅供参考」。

### 9.5 报告体积与脱敏规则

| 规则 | 值 |
|---|---|
| 报告内 `samples` 保留 | P1 全量（≤ 300 条）；P6 全量（≤ 120）；其它 probe 只保留聚合（`samples: []`） |
| 单报告目标体积 | ≤ 512 KiB（超出则裁剪 P1 sample 到 1 s 粒度） |
| `proxy_url` | 只留 `scheme://host:port`，`user:pass` 替换为 `«redacted»` |
| `body_head` / `Excerpt` | 仅命中位置前后 80 字符，且过 §5.7 脱敏正则 |
| `Cookie` / `Authorization` | **从不采集**，故不存在于任何结构 |

---

## 10. 模块划分与核心接口（Go 侧，供直接建包）

`[ADR-PENDING]`：包路径命名由架构文档最终裁定；此处给出**职责边界与函数签名**，签名是必须遵守的契约。

```
internal/
  engine/           # 编排：Run 生命周期、波次调度、取消、事件发布
    run.go          Run(ctx, cfg RunConfig, sink EventSink) (Report, error)
    schedule.go     planWaves(cfg) []Wave
  probe/            # 各 probe 实现（互相独立，不 import engine）
    probe.go        type Probe interface {
                      ID() ProbeID
                      Timeout(cfg Profile) time.Duration
                      Run(ctx context.Context, env *Env, sink EventSink) (*ProbeResult, error)
                    }
    p1_download.go  p2_youtube.go  p3_chatgpt.go  p4_gemini.go  p5_openai.go  p6_baseline.go
  netx/             # 传输层
    transport.go    NewProbeTransport(cfg) *http.Transport   // 复用、HTTP/2 on
                    NewBulkTransport(cfg)  *http.Transport    // DisableKeepAlives, HTTP/1.1
    trace.go        NewTrace() (*httptrace.ClientTrace, *Timings, *TraceFlags)
    proxy.go        ResolveSystemProxy() (Mode, *url.URL, error)  // macOS scutil / Windows WinINET
    errors.go       classifyErr(err error) ErrorCode
  metrics/          # 纯函数，无 IO，最易测
    window.go       type Ring struct{...}; Push(delta int64); Sum() int64; Rate() float64
    throughput.go   Aggregate(windows []Window, cfg P1Config) Throughput
    percentiles.go  P50([]float64) float64; P90(...); Median(...); StdDev(...)
    stability.go    AggregateStability(samples []Sample) Stability
    score.go        Score(run ResultSet) Score
  aiverdict/        # 判定引擎（规则表驱动）
    rules.go        var Rules = []Rule{...}      // §5.5 的可执行形式
    classify.go     Classify(in AIProbeInput) AIVerdict
    redact.go       Redact(s string) string
  report/           # 序列化、脱敏、导出
    report.go       Build(...) Report; ExportJSON(); ExportMarkdown()
  api/              # 本地 HTTP + SSE
    server.go       NewServer(engine) ; routes
```

**关键契约**：

```go
// 事件出口：实现可以是 SSE hub、也可以是测试里的内存切片
type EventSink interface {
    Emit(evt Event) // 非阻塞；内部缓冲满时按 §8.1 策略丢弃 sample
}

// 环境快照：所有 probe 共用，保证参数一致（§12）
type Env struct {
    RunID        string
    Cfg          RunConfig
    ProbeClient  *http.Client   // 复用连接
    BulkClient   *http.Client   // 每 probe 新建
    Rand         *rand.Rand     // 种子派生（§12）
    Clock        Clock          // 接口，便于测试注入
    Now          func() time.Time
}
```

> `Clock` 接口只为测试存在（`Now()` / `Since()`），生产实现直接包 `time.Now`（保留单调时钟）。

---

## 11. 礼貌与合规（对目标站点的义务）

### 11.1 请求量硬上限（单次 Run，**不可通过配置突破**）

| 约束 | 值 | 说明 |
|---|---|---|
| 总下载字节 | **≤ 64 MiB**（Deep ≤ 160 MiB） | 约等于「打开 1~2 个网页+看 20 秒视频」的量级 |
| 总请求数 | **≤ 40**（6 probe：P1 6 + P2 3 + P3 2 + P4 1 + P5 1 + P6 10 + 重试 ≤ 5 + 重定向 ≤ 8） | 远低于任何人的正常浏览 |
| 单域名请求数 | **≤ 12** | 含重定向 |
| 单域名并发连接 | **≤ 6**（P1 的 6 流指向 `dl.google.com`） | 与浏览器 6 连接上限一致 |
| 全局并发连接 | **≤ 16** | `connSem` |
| 对同一 URL 的最小间隔 | **300 ms**（P6）/ **1 s**（其它） | — |
| 单 Run 时长 | ≤ 200 s（Deep） | — |
| 连续 Run 冷却 | **≥ 10 s**（UI 侧强制，引擎返回 `retry_after_ms`） | 防用户狂点 |

### 11.2 User-Agent 策略

- 使用**固定的桌面 Chrome UA**（§1.2），且**不伪装成该 UA 的浏览器**做任何只有浏览器才会做的事（不执行 JS、不带 `Sec-Fetch-*`、不伪造 `Accept-CH`）。
- 诚实性权衡：固定浏览器 UA 是为了拿到**与真实用户一致的响应**（很多站点对 `Go-http-client` 直接 403，会让「AI 可用性」结论失真）。这是必要的，但通过 `ProxyLens-Run` 头**明示工具身份**，便于站点运营方识别与联系我们。
- **不使用随机 UA 池**（那是爬虫行为，且破坏可复现性）。

### 11.3 频率与「不做」清单

- **不**做 GET 之外的写操作（v1.1 起**全部为 GET**：P2 已删除 `youtubei/v1/player` 的 POST，§1.4）。
- **不**并发轰炸同一域名；**不**做「重试直到成功」（§7.5 明确 4xx 不重试）。
- **不**携带任何凭据：无 `Cookie`、无 `Authorization`、无 `Referer`（首跳）。
- **不**下载完整大文件（一律 `Range`）。
- **不**绕过 `robots.txt` 的语义边界：我们只访问首页/公开 API 端点/公开安装包，不爬取内容。
- 若收到 `429` / `Retry-After`：**立即停止对该域名的后续请求**（P6 除外，其间隔已 ≥300 ms），并在报告中记录 `rate_limited_by_target=true`。

### 11.4 出站目标收口（`04-security.md` 六条不可协商项）

**(a) 唯一收口点是 `net.Dialer.Control`**

```go
d := &net.Dialer{
    Timeout:   DialTimeout,
    DNSConfig: nil,
    Control: func(network, address string, _ syscall.RawConn) error {
        // address 已是「已解析的最终 IP:port」，这是在 Go 中唯一免疫 DNS rebinding 的位置
        host, _, err := net.SplitHostPort(address)
        if err != nil { return err }
        ip, err := netip.ParseAddr(host)
        if err != nil { return err }
        if blockedPrefix(ip) { atomic.AddInt64(&privateTargetsBlocked, 1); return ErrPrivateTarget }
        return nil
    },
}
```

- **两道校验**：先 `LookupNetIP` 显式查一次（只为产出可读错误信息），再由 `Control` 做硬阻断。
- **禁止**只做 `url.Host` 字符串白名单（`169.254.169.254.nip.io` 可绕过）。

**(b) 网段表（照抄，不得自行发挥）**

| 场景 | 拒绝的网段 |
|---|---|
| 任何场景 | `169.254.0.0/16`、`fe80::/10`、`100.64.0.0/10`、`0.0.0.0/8`、`192.0.2.0/24`、`198.18.0.0/15`、`224.0.0.0/4`、`240.0.0.0/4`、`::/128`、`ff00::/8` |
| 代理地址侧 | **允许** loopback + RFC1918（`10/8`、`172.16/12`、`192.168/16`、`fc00::/7`）——用户的代理常在本机/内网 |
| 探测目标侧 | **只允许公网单播**（上述全部 + RFC1918 + loopback 一律拒绝） |

> ⚠️ 注意 `198.18.0.0/15` 是 TUN fake-IP 段：**目标侧拒绝**它是对的（探测目标是真实公网站点），但**代理侧**若用户填了 fake-IP 网关则应允许——故网段表按「场景」分两张，不要合并。

**(c) 探测目标约束**

仅 `https`（`:80` 只允许 `generate_204` 基线）、端口 `443`、必须是 FQDN（禁 IP 字面量、禁 `localhost`/`*.local`/`*.internal`/无点单标签名、禁 URL 内 `user:pass@`）、重定向 ≤ **3** 跳且逐跳复检、禁 `https→http` 降级、**`socks5h` 模式下禁止自定义目标**（远端 DNS，本地看不到目标 IP，无法校验）。

**(d) 脱敏是类型问题，不是注意力问题**

```go
func Sanitize(s string) string   // 正则 ([a-z][a-z0-9+.-]*://)[^/@\s"']*@  →  ${1}***@
func MaskProxy(u *url.URL) string // 密码固定三个星，不泄露长度
```

- `url.Parse` 的错误文本会带完整含密 URL（`parse "socks5://u:p@h:99999": invalid port`）→ **所有 error 在返回/记录前必须过 `Sanitize()`**。
- 导出报告用**白名单结构体**序列化，**禁止** `json.Marshal(config)`（会连带序列化未预期字段）。
- 代理密码存 `[]byte`，会话结束显式清零（`for i := range b { b[i] = 0 }`）。

**(e) 响应体与流式下载**

探测类 body 一律 `io.LimitReader`，**≤ 512 KiB**（本文 P3/P4 用 64 KiB、P5 用 8 KiB，更严）；下载类直接 `io.Discard`，**不建临时文件**；严格遵循 `Retry-After`。

---

## 12. 可复现性（Reproducibility）

### 12.1 目标与矛盾

「完全可复现」与「避免 CDN 缓存导致结果虚高」是**天然矛盾**：同一 `Range` 命中边缘缓存 → 测到的是缓存速度而非线路速度。本设计的取舍：

| 项 | 处理 | 理由 |
|---|---|---|
| **字节偏移** | **固定**：由 `runSeed` 派生，写到报告的 `request_manifest` | 保证「同一配置 = 同一 Range」，A/B 可比 |
| **cache-buster** | 默认**开启**：`P1` 不加（`dl.google.com` 的 Range 请求本身分片就散），`P2-T2` 与对照端点加 `?plcb=<8hex>` | 只在「易被整文件缓存」的端点上打破缓存 |
| **`plcb` 的取值** | `hex(sha256(runSeed || url_path)[:4])`，**记录在报告中** | 可审计、可复算；同一配置得到同一 `plcb` |
| **`runSeed`** | `sha256(canonical_json(config_without_credentials) || engine_version)` 取前 8 字节 | 同配置同版本 → 同 seed → 请求集合完全一致 |
| **`deterministic` 开关** | 默认 `false`；若 UI 显式要求「严格对比」，置 `true` → `plcb` 固定为 `"plcb=00000000"` | 用户已知会撞缓存，换取严格可比 |

### 12.2 `runSeed` → 各请求参数的派生

```
seed = sha256(canonical_json(cfg) + "|" + engine_version)[0:8]     // 8 bytes

P1 stream i 的 Range 起点：
  h_i = sha256(seed || "P1" || itoa(i))[0:8]  →  u64
  max_start = content_length − per_stream_bytes − 1                 // 278613265 − 8388608 − 1
  start_i = (h_i mod (max_start / 65536)) × 65536                    // 64 KiB 对齐
  若 content_length 未知（首跑）→ 先发 `Range: bytes=0-0` 探测 content-range 总长（1 个额外请求，计入 §11 预算）

P6 第 i 次样本的目标 URL：i % 2 == 0 ? google.com : gstatic.com     // 确定，不随机
退避随机：backoff_n = 500ms + (sha256(seed||"retry"||itoa(n))[0] mod 250)ms
```

### 12.3 时钟与参数一致性

- **时长一律用单调时钟**（`time.Since`），**墙钟只用于时间戳字段**；报告中同时给 `started_at`（墙钟）与 `duration_ms`（单调）。
- 若检测到墙钟跳变（一次 Run 内 `|Δ墙钟 − Δ单调| > 2000 ms`）→ `warnings += ["clock_jump_detected"]`，并在 `environment` 里记录，分数不因此调整（因为用的是单调时钟）。
- **同一 Run 内所有 probe 共享同一份参数快照** `env.Cfg`（值传递 copy），运行中修改配置不影响进行中的 Run。
- 每 Run 只探测一次 `ResolveSystemProxy()`（在 `run_start` 前），结果写进 `effective_route`；**不在 Run 中重解析**（否则前后 probe 走的路由可能不同）。

### 12.4 可复现性自检（写进报告）

```json
"request_manifest": [
  {"probe_id":"P1","url":"https://dl.google.com/chrome/.../googlechrome.dmg",
   "range":"bytes=125829120-134217727","plcb":null,"est_bytes":8388608},
  {"probe_id":"P6","url":"https://www.google.com/generate_204","range":null,"plcb":null},
  {"probe_id":"P6","url":"https://www.gstatic.com/generate_204","range":null,"plcb":null},
  {"probe_id":"P2","url":"https://i.ytimg.com/vi/jNQXAC9IVRw/hqdefault.jpg?plcb=a19f3c2e", "...": "..."}
]
```

用户可以拿这份 manifest 直接用 `curl` 复现，这是「可解释」的最后一道保险。

---

## 13. 单元测试要点

### 13.1 必须 mock 的三类边界

| 边界 | 接口 | 测试手法 |
|---|---|---|
| **时钟** | `Clock{ Now() time.Time; Since(t) time.Duration }` | `fakeClock` 手动推进；断言 `Timings` 精确到 0.1 ms。**严禁**在测试里 `time.Sleep` 等真实时间 |
| **随机** | `Rand interface{ IntN(n int) int; Float64() float64 }` | 固定 seed 的 `math/rand/v2`；断言派生出的 Range 起点、`plcb`、退避值完全等于期望常量 |
| **httptrace** | 不 mock trace 本身；**mock 网络对端** | 用 `httptest.NewServer` + `httptrace` 真实触发；对「连接复用导致 Connect=0」这类场景，用 `httptest.NewUnstartedServer` + 两次请求复用同一 conn 来断言 `flags.conn_reused==true && timings.connect_ms==nil` |

### 13.2 「假代理」构造

```go
// 1) 假 HTTP CONNECT 代理：可断言 Proxy-Authorization、可注入故障
func newFakeHTTPProxy(t *testing.T, opts ...ProxyOpt) *httptest.Server {
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method == http.MethodConnect {
            if opts.requireAuth && r.Header.Get("Proxy-Authorization") != expectedBasic {
                w.WriteHeader(http.StatusProxyAuthRequired)  // 407 → PROXY_AUTH_FAIL
                return
            }
            // hijack + 隧道透传，或按 opts 直接 write RST / 延迟 6s（触发 DIAL_TIMEOUT）
        }
    }))
}

// 2) 假 SOCKS5：用 golang.org/x/net/proxy 的测试 server，或最小实现（仅 CONNECT + 域名透传）
//    断言：socks5h 应把 domain 传给代理（检查 ATYP=0x03），socks5 应传 IP（ATYP=0x01）
```

### 13.3 「假 429 / 假挑战页」构造（判定逻辑测试的核心）

```go
cases := []struct{ name string; hdr map[string]string; status int; body string; want AIStatusEnum; wantRule string }{
  {"cf-challenge-header", map[string]string{"cf-mitigated":"challenge","server":"cloudflare"},
     403, "<html><title>Just a moment...</title>", CHALLENGE, "AI-R4"},
  {"cf-body-only", nil, 503, "<title>Just a moment...</title>Enable JavaScript and cookies to continue", CHALLENGE, "AI-R4"},
  {"gemini-region", nil, 200, "<title>Gemini</title>Gemini isn't available in your country", REGION_BLOCKED, "P4-1"},
  {"openai-401-ok", nil, 401, `{"error":{"message":"Missing bearer authentication in header"}}`, AVAILABLE, "P5-1"},
  {"openai-403-country", nil, 403, `{"error":{"code":"unsupported_country","message":"Country, region, or territory not supported"}}`, REGION_BLOCKED, "P5-2"},
  {"openai-quota", nil, 429, `{"error":{"code":"insufficient_quota"}}`, DEGRADED, "P5-3"},
  {"fake-429-plain", nil, 429, `Too Many Requests`, DEGRADED, "AI-R10"},
  {"hijack-html-on-api", map[string]string{"content-type":"text/html"}, 200, "<html>portal</html>", HIJACKED, "H-3e"},
  {"login-redirect", nil, 200, "<title>Sign in</title>", AUTH_REQUIRED, "H-4a"},
  {"empty-200", nil, 200, "<html></html>", DEGRADED, "P3-8"},
}
```

- 用 `httptest.NewServer` 逐条起 handler，跑真实 `http.Client` + 真实 trace，再断言 `AIVerdict`。
- 表驱动 + `golden` 快照（`testdata/verdict/*.json`），避免以后调规则时无意改结论。
- **必须打的回归点**：`cf-mitigated: challenge` 存在时，即使 body 是完美正常页面也要判 `CHALLENGE`（header 优先级高于 body）。

### 13.4 纯函数测试清单（无网络、必须 100% 覆盖分支）

| 函数 | 用例 |
|---|---|
| `Aggregate(windows)` | ① 匀速窗口 → CV≈0、decay≈1；② 前高后低 → decay≈0.5；③ 样本数不足 → `Reliable=false`；④ 含 0 字节流 → `StreamsFailed` 计入且不参与 p50 |
| `P50/P90/Median` | 偶数/奇数/单元素/含 null；`p90` 用最近秩法（与 §3.3 一致，不许用 numpy 式插值） |
| `AggregateStability` | 全成功、半超时、连续失败 5 次（`fail_streak`）、含尖峰（验证中位数 jitter 不被拉高） |
| `Score` | §6.7 算例**逐位断言**：`quality_raw = 85.0388`（误差 < 0.01）、`gate=RESTRICTED`、`quality=69.0`、`grade="C"`、`caps_applied=["CAP-2"]`；每个 CAP 触发/不触发边界；`conf<0.75` 的 CHALLENGED **不**触发 RESTRICTED；`SKIPPED` 项从分母剔除后权重重归一化 |
| `classifyErr` | 构造每种 `error`（`&net.DNSError{IsNotFound:true}`、`&x509.UnknownAuthorityError{}`、`context.DeadlineExceeded`、含 `proxyconnect` 的字符串错误）断言 code |
| `Redact` | 邮箱、IPv4、长 hex、URL query 中的 token 全被替换 |
| `deriveRange` | 固定 seed → 固定 offset；`offset+budget ≤ contentLength` 恒成立（property-based: 随机 1000 组 seed） |

### 13.5 集成测试（可选，标记 `-tags=integration`）

- `TestAgainstLocalFakeStack`：本地起假 dl.google.com / 假 chatgpt（CF challenge）/ 假 204，跑完整 Run，断言：总时长 < 75 s、事件序列符合 §8.2 顺序、报告可 JSON 往返（`marshal → unmarshal → deep equal`）。
- **不**在 CI 里打真实外网（不稳定且失礼）。

---

## 14. 参数总表（唯一权威，代码里必须以此为准）

```go
package engine

const (
    // ── 版本 ─────────────────────────────────────────────
    EngineVersion  = "1.1.0"
    SchemaVersion  = "1.1"
    ScoreFormulaID = "PL-SCORE-1.1"

    // ── Run 级（security / pm 硬约束）──────────────────────
    RunTimeoutQuick    = 25 * time.Second
    RunTimeoutStandard = 75 * time.Second
    RunTimeoutDeep     = 200 * time.Second
    MaxConcurrentRuns  = 1          // 已有运行中 → HTTP 409
    RunCooldown        = 10 * time.Second
    MaxTargetHosts     = 8
    ConcurrencySem     = 16         // 全局 TCP 连接
    DNSSem             = 8
    MaxRequestsPerRun  = 40
    MaxRequestsPerHost = 12
    MaxRedirects       = 3          // security §2.3（原 8 作废）
    CancelStopBudget   = 500 * time.Millisecond  // 取消后停止推送（PRD US-05.1）

    // ── P1 吞吐（PRD §4.B：20 MiB）─────────────────────────
    P1StreamsStd        = 5
    P1StreamsQuick      = 2
    P1StreamsDeep       = 8
    P1PerStreamBytesStd = 4 << 20    // 4 MiB
    P1PerStreamBytesQk  = 2 << 20
    P1PerStreamBytesDp  = 8 << 20
    P1TotalBudgetStd    = 20 << 20   // 5 × 4 MiB
    P1HardCapStd        = 24 << 20   // 105% 容忍上限
    P1WarmupStd         = 1000 * time.Millisecond
    P1SampleInterval    = 100 * time.Millisecond   // 内部 tick（算法用）
    P1PushInterval      = 250 * time.Millisecond   // 对外 4 Hz（ux-arch）
    P1Window            = 1000 * time.Millisecond  // = 10 ticks
    P1EMAAlpha          = 0.30
    P1MinSamplesStd     = 12
    P1TimeoutStd        = 30 * time.Second
    P1RangeAlignBytes   = 65536
    P1SlowAbortMbps     = 0.5        // < 0.5 Mbps 持续…
    P1SlowAbortFor      = 5 * time.Second         // …5 s 立即收口判 SLOW

    // ── P2（四级，无未文档化接口）──────────────────────────
    P2TimeoutStd    = 25 * time.Second
    P2T4BytesStd    = 4 << 20        // gvt1 边缘 Range 预算
    P2T4Timeout     = 15 * time.Second
    P2MinThumbBytes = 8000
    P2VideoID       = "jNQXAC9IVRw"  // 固定 ID（可复现）

    // ── P3/P4/P5 ──────────────────────────────────────────
    P3Timeout = 15 * time.Second
    P4Timeout = 20 * time.Second
    P5Timeout = 10 * time.Second
    P3BodyCap = 64 << 10
    P4BodyCap = 64 << 10
    P5BodyCap = 8 << 10
    BodyCapMax = 512 << 10           // security 上限（探测类一律 ≤ 512 KiB）

    // ── P6 ────────────────────────────────────────────────
    P6SamplesStd   = 10
    P6SamplesQuick = 5
    P6SamplesDeep  = 120
    P6Interval     = 500 * time.Millisecond  // security：同目标 ≥500 ms（原 300 作废）
    P6TimeoutEach  = 3 * time.Second
    P6FailFastN    = 3

    // ── 传输 ──────────────────────────────────────────────
    DialTimeout           = 5 * time.Second
    DNSTimeout            = 4 * time.Second
    TLSHandshakeTimeout   = 5 * time.Second
    ResponseHeaderTimeout = 8 * time.Second
    IdleConnTimeout       = 30 * time.Second
    ProbeIdleConnTimeout  = 90 * time.Second  // 探活 Transport（arch）
    ProbeMaxIdlePerHost   = 4                 // 探活 Transport（arch）
    ReadBufBytes          = 32 << 10

    // ── 重试 ──────────────────────────────────────────────
    RetryMax       = 1
    RetryBaseDelay = 500 * time.Millisecond
    RetryJitterMax = 250 * time.Millisecond

    // ── 评分（gate 优先，quality 附录）──────────────────────
    WeightSpeed       = 0.40
    WeightStability   = 0.30
    WeightAI          = 0.30
    WeightThrInSpeed  = 0.75
    WeightTTFBInSpeed = 0.25
    CapBlocked        = 29.0   // gate=BLOCKED
    CapRestricted     = 69.0   // gate=RESTRICTED
    CapStabilityDown  = 54.0
    CapFallback       = 75.0

    // ── SSE ───────────────────────────────────────────────
    SSEHeartbeat    = 15 * time.Second
    SSEBufPerConn   = 256
    SSEReplayBuffer = 2000
)

```

**归一化断点（必须与 §6 一致，代码里用表驱动）**

```go
var ThruPoints = []Point{{0, 0}, {2, 40}, {5, 60}, {10, 80}, {20, 95}, {50, 100}}   // Mbps
var TTFBPoints = []Point{{100, 100}, {300, 90}, {600, 70}, {1000, 50}, {2000, 20}, {3000, 0}} // ms
var JitterPoints = []Point{{5, 100}, {15, 90}, {30, 75}, {60, 50}, {120, 25}, {200, 0}}      // ms
var StreakPoints = []Point{{0, 100}, {1, 85}, {2, 65}, {3, 40}, {5, 0}}                       // 次数
```

---

## 15. 风险、开放问题与验收自检

### 15.1 已知风险（按影响排序）

| 风险 | 影响 | 缓解 |
|---|---|---|
| **P2 拿不到真实播放分片（已定论）**：YouTube 需 JS 签名（arch M8/M11） | 「YouTube 播放速度」不可测 | 改四级方案（§1.4）：T3 只证明到 gvs 边缘可达，T4 用 gvt1 作「Google 边缘下行」代理指标；**文案禁止标称播放速度**（已写进 ADR 约束） |
| **`dl.google.com` 被用户所在网络针对性限速** | P1 失真 | 提供 `speed.cloudflare.com` 对照（D2）；若两者差异 > 3× 则 `warnings += ["endpoint_discrepancy"]` |
| **CF challenge 与「代理」强相关，但也会对真实浏览器出现** | 可能高估代理问题 | 置信度机制 + 文案「建议换节点验证」（§5.5 P3 说明） |
| **P1 与 P2 抢带宽** | p50 偏低 | 波次调度（§7.1）保证 P1 独占 |
| **报告体积**（P1 全量 sample） | 导出慢 | §9.5 裁剪规则 |
| **Windows `WinINET` API 需 cgo 或 syscall** | 交叉编译复杂度 | 用 `golang.org/x/sys/windows/registry` 读注册表为主，`WinINET` 为可选增强；不引入 cgo |
| **`insecure_skip_verify` 开关** | 用户可能用它绕过劫持检测 | 开启时报告中 `insecure=true`，且 **HIJACKED 判定关闭并显式提示「证书校验已禁用，无法判断劫持」** |

### 15.2 开放问题（需 team-lead / arch 裁决）

1. **报告是否落盘**：brief 只禁止「下载数据落盘」。建议报告默认存 `~/.proxylens/reports/*.json`（≤512 KiB/份，保留 50 份），需在 `03-architecture.md` / `04-security.md` 中确认。
2. **「直连对照」是否自动跑**：本设计支持但默认不自动跑（会把 Run 时长翻倍）。建议 UI 提供「对比模式」按钮，串行跑两次 Run，用 `environment.fingerprint` 校验可比性。
3. ~~`P2-T3` 的 `youtubei` 公共 key 是否保留~~ → **已关闭（arch 裁定）**：该接口返回空 formats，保留只带来误判风险，v1.1 已完全删除。
4. **P4 是否加 `aistudio.google.com` 作为旁证**：可提高地区判定准确度，但增加请求量。建议 v1.1 再议。

### 15.3 引擎侧验收自检清单（可直接转成 `09-test-plan.md` 的验收项）

- [ ] 三种模式（system / custom / direct）都能跑通，`effective_route` 描述准确（当前环境应命中 `env` + `tun_suspected=true`）。
- [ ] `socks5h` 与 `socks5` 的 DNS 归属字段分别为 `proxy` / `local`（网络抓包可证）。
- [ ] P1 在 100 Mbps 线路报 `limited_by="bytes"`，在 10 Mbps 报 `limited_by="duration"`，两者 `p50` 差 < 15%。
- [ ] 任一 probe 超时不影响其它 probe 出结果；被取消的 Run 仍产出 `partial=true` 的报告。
- [ ] 对 `chatgpt.com` 的真实请求（当前环境）判为 `verdict=CHALLENGED`、`diagnosis=CHALLENGE`、`why` 可读、`matchedRule=cf_challenge_v2`。
- [ ] §6.7 算例可复算：`quality_raw=85.0` → gate=RESTRICTED → `quality=69.0`、`grade=C`；**不得出现「85 分 + 优秀」**（gate 回归测试）。
- [ ] 对 `api.openai.com/v1/models` 的真实 401 判为 `AVAILABLE`。
- [ ] 单 Run 总请求数 ≤ 40，总下载 ≤ 64 MiB（可用 `request_manifest` 求和验证）。
- [ ] 同配置连续两次 Run 的 `request_manifest` 完全一致（`deterministic=true` 时）。
- [ ] SSE 断线重连后无事件丢失、无重复（按 `seq` 校验）。
- [ ] 报告中不含任何凭据、Cookie、Authorization、完整 body。

















---

## 16. 修订记录（v1.0 → v1.1）

| # | 变更 | 来源 | 影响范围 |
|---|---|---|---|
| 1 | `verdict` 改用 PRD 九态；原 7 值 `AIStatusEnum` 降为内部 `ai_status`；`TLS_ERROR`/`UNSTABLE`/`TAMPERED`/`INCONCLUSIVE` 由 `diagnosis` 承载 | pm §3.1、ux-arch §3.2 | §0.4(a)、§5.8、§5.9、§6.4、§9.3 |
| 2 | 引入 **gate-not-weight**：V1 裁决由业务态门槛决定，`Total` 更名 `quality_score`（附录），BLOCKED→29 / RESTRICTED→69 封顶 | ux-researcher、ux-arch | §0.4(b)(c)、§6.1、§6.5、§6.7 |
| 3 | 档位改 PRD 五分档 A/B/C/D/E（原七档作废） | pm §3.2 | §6.6、§6.7 |
| 4 | **删除 P2 的 `youtubei/v1/player`**，改四级（页面 → ytimg → gvs 握手 → gvt1 吞吐）；`media_path` 五值；T4 禁止标称「YouTube 播放速度」 | arch M8/M11 | §1.4、§0.5(b) |
| 5 | 延迟主指标改 `tls_ms`；代理模式 `dns_ms` 必须 `null`；`connect_ms` 标签改「本地代理建连」 | arch 实测 | §0.4.8→§0.5(a)、§2.4、§6.3、§9.3 `Stages` |
| 6 | P1 预算 48 MiB → **20 MiB**（5 流 × 4 MiB），硬上限 24 MiB；新增 0.5 Mbps/5 s 早收判 SLOW | pm §4.B | §3.1、§14 |
| 7 | 推送 10 Hz → **4 Hz**（内部 tick 仍 100 ms）；事件改点分命名；新增 `probe.stage`/`probe.skipped`/`egress.detected`/快照接口 | ux-arch 附录 A | §8 全章 |
| 8 | 重定向 8 → **3 跳**；P6 间隔 300 → **500 ms**；全局并发 Run=1（409）；目标数 ≤8 | security | §2.4 P-8、§7.1、§14 |
| 9 | 新增 §11.4 出站收口（`Dialer.Control` + netip 网段表 + `Sanitize`/`MaskProxy` + 白名单序列化） | security §1/2/3/4 | §11.4、§9.4 `security` 段 |
| 10 | 新增 §5.8 三信号判定矩阵（假绿红线）、§5.9 归因枚举 12 值 | ux-researcher | §5.8、§5.9 |

**未采纳但需说明的两处**：

1. ux-arch 的 `TLS_ERROR`/`UNSTABLE` **未**作为 verdict 新值（与 pm「不得新增」冲突），改由 `diagnosis` 承载——ux-arch 无需改 `verdictCopy`，但详情层要能显示 `diagnosis`。
2. ux-researcher 建议的 `INCONCLUSIVE` 独立态，当前映射为 `NET_FAIL + diagnosis=INCONCLUSIVE`（宁红勿绿）。**建议 pm 在 PRD 增补第 10 态**，届时只需改 §0.4(a) 一张表。

---

## 17. 实现口径修订（v1.2 · 与代码对齐）

> 本节记录**代码实际实现**与上文设计稿的差异。两者冲突时以本节为准；上文保留为设计意图留档。
> 对应代码：`internal/config`、`internal/engine`、`internal/probe`、`internal/score`。

### 17.1 评分权重与未测维度

| 维度 | 权重 | 说明 |
|---|---|---|
| Speed | 40% | P1 主；P1 未测时该维度不参与加权 |
| Stability | 30% | P6 成功率 / RTT / 抖动 |
| AI Access | 30% | P3 / P4 / P5 |

- 未测维度由 `isMeasured()` 判定，**不按 0 分计入**；权重在参与项上重新归一。
- 快速档不测 P1 时，前端显示「速度 · 未测」+ 虚线空槽 + 分数「—」，结论句补「本次未测下载速度，速度分不参与总分」。

### 17.2 P1 下载

| 项 | 实现值 |
|---|---|
| 默认并发 | **1**（单流）；用户可在设置中调高，上限 16 |
| 协议 | 强制 HTTP/1.1：`ForceAttemptHTTP2=false` + `TLSClientConfig.NextProtos=["http/1.1"]` |
| 端点 | 4 条：`dl.google.com` 主/备、`redirector.gvt1.com`、`speed.cloudflare.com`（仅对照） |
| 端点探测重试 | 最多 2 次，间隔 800 ms，仅瞬态错误 |
| 判失败 | 连续失败 5 次（`dlMaxConsecFail`） |
| 新增字段 | `ConnCount`（`httptrace.GotConn` 按 `LocalAddr` 去重）、`Stalled` |

> 只关 Transport 层的 h2 不够：服务端经 ALPN 仍会选 h2 并回 SETTINGS 帧，客户端用 HTTP/1.x 解析会报 `malformed HTTP response`。必须在 TLS 层摘掉 h2。

### 17.3 P2 YouTube

- 三级真实请求：T1 `watch` 页 → T2 `i.ytimg` 缩略图 → T3 从页面抠 `*.googlevideo.com` 主机做边缘握手。
- **不测吞吐**，`Mbps` 恒为 0；`mediaPath` 取值：`unavailable` / `page_only` / `thumb_only` / `gvs_handshake`。
- 匿名探测被要求验证时判 `challenge`，文案明确「不代表你浏览器里看不了」。

### 17.4 P3 / P4 采样

- 3 次采样，间隔 **1.2 s**，`severity()` 取**最差**结论。
- 卡片副标题显示 `n/m 次能打开`；混合时 headline 为「时好时坏」。
- 证据链输出每次的 HTTP 状态分布 + 最差那次的关键响应头。

### 17.5 长跑（P1 长时稳定性）

| 项 | 实现值 |
|---|---|
| 触发 | `longRun=true` |
| 默认时长 / 流量 / 并发 | 1800 s / 1024 MB / 1 |
| 分窗 | 5 s（`dlWindow`） |
| 断流判定 | 连续 **2** 个空窗（`dlStallWindows`） |
| `StallAtMs` | 这串空窗的**第一个**窗口起点（回答「什么时候开始不行的」） |
| `SustainPct` | 以全部窗口中位数的 20% 为有效阈值；中位数为 0 时直接给 0 |
| 吞吐口径 | 均值 / P50 / P90 / 峰值只用非空窗；空窗由 `SustainPct` 单独表达 |

### 17.6 单项复测（「只测这项」）

- 入参：`Config.Only=true`，且 `Probes` 恰好 1 项；否则返回「单项复测必须且只能指定一个探测项」。
- 行为：保留其它探测项的既有结果与上一轮 `Summary`，只替换目标项；**不重算总评**。
- 标记：`Result.Only=true`，前端卡片显示「单项复测 · 不参与总评」。
- 事件：`run_start` / `run_end` 均带 `only` 字段；`State.Only` 标记最近一次运行是否为单项复测。
- 理由：总评代表一次完整运行；把不同时间窗口的结果混算在一起无法归因。

### 17.7 出口身份

- P3 通过 Cloudflare `/cdn-cgi/trace` 写入 `Region` / `Colo` / `Warp`，独立 8 s 预算，前置到页面 GET 之前。
- 前端在卡片副标题下方显示 `出口 loc=JP · colo=NRT`；`Warp=true` 时用警告色徽章「WARP 出口」。

### 17.8 取消与无结论

- 取消时**不产出 Summary**（保持附录 A.3 口径）。
- 所有端点都卡在连接层时同样不评分，返回可读原因，避免把「本地网络中断」渲染成「代理不可用」。

### 17.9 独立带宽长跑（`bandwidth=true`）

与 §17.5 的 P1 长跑是**两件事**：P1 回答「一次完整体检里下载这一项表现如何」，独立长跑回答「这条链路连续跑几小时会不会自己掉下来」。所以量纲、状态位、事件通道全部独立。

| 项 | P1 长跑 | 独立带宽长跑 |
|---|---|---|
| 开关 | `longRun=true` | `bandwidth=true` |
| 时长上限 | 1800 s | **14400 s（4 小时）** |
| 流量上限 | 4096 MB | **102400 MB（100 GB）** |
| 默认值 | 1800 s / 1024 MB | 1800 s / 8192 MB |
| 探测项 | 六项之一 | `Probes=nil`，只跑下载 |
| 写入 `Results` / `Summary` | 是 | **否** |
| 事件通道 | `run_start` / `sample` / `run_end` | `bw_start` / `bw_window` / `bw_result` / `bw_end` |
| 状态位 | `State.Running` / `Results` | `State.Bandwidth` |

- **量纲收敛**：`Config.Normalize()` 在 `Bandwidth=true` 时直接走 `normalizeBandwidth()` 并**早返回**，六项那套 1800 / 4096 的夹紧逻辑完全不复用——否则 4 小时与 100 GB 会被静默夹死。
- **独占锁共用**：`e.running` 同时拦截两边。并行会让两侧吞吐互相污染，因此带宽长跑进行中启动综合测试、或反之，都直接拒绝并给出可解释文案。
- **隔离验证口径**：一次带宽长跑前后，`total` 分数、P1 卡片文案、`Results` 条数必须逐字不变。这是「独立」二字的可验收定义。
- **取消即无结论**：`ctx.Err() != nil` 时 `Status=cancelled` 且 `Result=nil`，绝不把半程数据当结论。
- **中途打开页面**：`BandwidthRun.Samples` 随快照下发。存的是 5 秒窗口样本，4 小时也只有 2,880 个，`bwSampleCap=3600` 因此永不触发合并——快照体积与形状同时改善。前端据此补齐历史曲线，否则已经下了几百 MB 的跑动看起来像刚点了开始。
- **单一数据源（v1.3 修正）**：长跑只对外发 `bw_window`，`OnSample`（250 ms 瞬时值）不再外发。曲线、当前速率、峰值、P50/P90 全部来自 `dlWin` 这一套聚合，与 `concludeLongRun` 逐字同源。
  - 反例（改造前实测）：同一段 88.3 MB / 72 s 的下载，250 ms 窗口的最大值是 **78.12 Mbps**，5 s 窗口只有 **18.48 Mbps**，而真实平均（总字节 ÷ 总时长）是 **9.81 Mbps**。界面上于是出现「全程平均 13.8 / 峰值 73.9」——峰值取的是代理缓冲区一次性冲刷的量化噪声，不是链路能持续的速度。
  - 均值在各类窗口下都稳定（字节守恒），**只有极值会随窗口变小而虚高**。所以峰值必须绑定一个足够长的窗口。
- **窗口样本的时刻取终点（`dlWin.t1`）**：样本里的 `Bytes` 是「到这一刻为止累计下了多少」，时刻必须取同一瞬间的终点。取起点会让 `总字节 ÷ 已跑时长` 凭空少算一个窗口——实测头 21 s 报「全程平均 39.9 Mbps」而同一屏「峰值」只有 36.4，平均值比峰值还高。`StallAtMs` 仍用窗口起点 `t0`，因为断流要回答的是「什么时候开始不行的」。
- **前后端不再各切一次窗**：前端直接消费窗口样本，阈值（中位数的 20%）、断流判定（连续 2 空窗）、中位数为 0 时给 0 这三条与 `concludeLongRun` 对齐。之前前端拿 250 ms 点自己合成 5 s 窗，两边差一点点舍入就会出现「有效时长占比 50%」和「100%」互相打脸。
- **结论以后端为准**：`bw_result` 到达后，峰值 / 有效时长 / 断流 / 总字节一律取结算值；结果序列比流里收到的更长时整段替换（收尾那个不满整窗的尾窗产生于采样 goroutine 退出时，可能赶不上 SSE），保证图上最高点与结果里的「峰值」是同一个点。
- **前端档位**（`web/js/core/bandwidth.js`）：时长 3 / 5 / 15 / 30 分、1 / 2 / 4 小时；并发 1 / 2 / 4 / 6 / 8 / 10 条。3 分与 5 分是「先快速看一眼」的档，能在同一分钟里横向对比几个节点；并发上限 10 由 `normalizeBandwidth` 的硬上限 16 兜底，无需改后端。后端 `minDurationSec=60`，3 分档不触边界。
