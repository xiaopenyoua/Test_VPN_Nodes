# ProxyLens 安全设计：威胁建模与本地服务攻击面

| 项 | 值 |
|---|---|
| 文档 | 04-security.md |
| 角色 | Security Architect |
| 状态 | v1.2（已与 `05-prd.md` v1.1 对齐：① 代理地址**整体不持久化**，仅非敏感 UI 设置可落盘；② 证书校验降级入口按 P2 存在但受"不出分"约束；③ 新增 `HIJACKED` 用户侧安全指引），供 03-architecture / 06-engine-design / 09-test-plan 落地 |
| 系统形态 | Go 单二进制，`127.0.0.1` 本地 HTTP 服务 + 内嵌 UI + SSE 进度推送 |
| 上游依据 | `docs/00-task-brief.md`、`docs/05-prd.md`（FR-A6 / FR-F1 / FR-G8 / NFR-9 / L-01~L-05） |

---

## 0. 安全目标、非目标与设计原则

### 0.1 安全目标（按优先级，均为可测判据）

| ID | 目标 | 判定标准 |
|---|---|---|
| G1 | **凭据零落盘** | 全盘搜索（`~/Library`、`%APPDATA%`、临时目录、报告、日志）搜不到代理密码原文；进程退出后无残留 |
| G2 | **本地服务不可被滥用** | 无 token 的请求（含浏览器其他标签页、其他 OS 用户、`curl`）访问任何 `/api/*` 返回 401，且**不产生任何外连副作用** |
| G3 | **不做 SSRF 跳板** | 出连目标 IP 不可能是 `169.254.0.0/16`、`127.0.0.0/8`（代理自身除外）、RFC1918（自定义目标场景）等内网/元数据地址 |
| G4 | **结果不可静默污染** | 任何降级（跳过证书校验、明文 HTTP 代理、疑似劫持）都必须出现在报告里，禁止"高分但被 MITM"的静默失败 |
| G5 | **零外传** | 除探测目标站点外，进程不向任何第三方端点发送数据（无遥测、无 CDN、无错误上报） |
| G6 | **最小攻击面** | 依赖受控、端口内核分配（非固定）、无 debug/pprof 端点、**不需要管理员权限** |

### 0.2 明确的安全非目标（必须在 UI/文档中明说，避免虚假安全感）

1. **不防御本机已被完全控制**：同用户恶意进程可读本进程内存、注入、抓 loopback 流量。这是 OS 级信任假设。
2. **不是匿名工具**：不隐藏 IP，不承诺抗流量指纹，不承诺抗 GFW 主动探测。它只回答"你现有代理行不行"。
3. **不校验代理是否恶意**：用户填的代理即用户信任的代理（ADV-4 只做可用性/一致性检测，不做信誉判定）。
4. **不做多用户 / 无权限模型**：一个进程一份状态，无账号体系。
5. **不做订阅解析**（与 brief 一致），顺带消除订阅 URL 内 token 的泄漏面。

### 0.3 五条设计铁律

1. **Loopback ≠ 可信**。`127.0.0.1` 服务等于"暴露给本机所有用户 + 用户浏览器里所有网页"，必须当公网服务加固。
2. **默认拒绝**。CORS 不出头、未知 Host 拒绝、未知路径 404、私网目标默认拒、证书错误默认失败。
3. **单一收口点（choke point）**。凭据脱敏一个函数、外连 IP 校验一个 `DialContext`、鉴权一个 middleware。安全逻辑分散＝必漏。
4. **失败要露出**。安全相关错误禁止 `_ = err`；任何降级写进报告。
5. **按"凭据终将泄露"设计**。缩短生命周期：只在内存、只在一个探测会话内、只用一次。

---

## 1. 资产与信任边界

### 1.1 资产清单

| 资产 | 位置 | 敏感度 | 泄露影响 |
|---|---|---|---|
| A1 代理凭据 user/pass | 进程内存、POST body | **Critical** | 代理被白嫖/接管；同密码横向扩散 |
| A2 代理地址 host:port | 内存、报告 | Medium | 暴露内网结构、代理存在性 |
| A3 会话 token（启动随机） | 内存、URL、Cookie | High | 可代替用户操作工具（不含凭据，危害有限） |
| A4 进程内存（含响应体片段） | RAM | High | 响应体可能含用户身份相关页面内容 |
| A5 探测结果 | 内存 → 导出报告 | Low–Medium | 可推断网络环境与地理位置；用户主动导出即视为同意 |
| A6 系统代理配置 / 环境变量 | 注册表/plist、env | Medium | 企业环境系统代理 URL 常内嵌凭据 |
| A7 二进制与内嵌前端资源 | 磁盘 | Low | 被篡改即可窃取 A1 |

> **A7 推论**：二进制被替换后一切设计失效。分发的完整性与签名（§9）是保护链第一环，不是可选项。

### 1.2 信任边界图

```
┌─────────────────────────────────────────────────────────────────────┐
│ OS 用户会话（信任假设：同用户进程互不设防 → 见 §0.2-1）               │
│                                                                     │
│  ┌────────────────────┐   TB-1   ┌──────────────────────────────┐  │
│  │ 浏览器上下文 A      │◄────────►│ ProxyLens 进程（Go 单二进制） │  │
│  │ 内嵌 WebView /      │ HTTP+SSE │  ┌────────────────────────┐  │  │
│  │ 系统浏览器（我们的UI）│         │  │ HTTP Server            │  │  │
│  └────────────────────┘          │  │ 127.0.0.1:<kernel-port>│  │  │
│                                  │  │ auth/host/origin 中间件 │  │  │
│  ┌────────────────────┐   TB-1'  │  └───────────┬────────────┘  │  │
│  │ 浏览器上下文 B      │─────────►│              │               │  │
│  │ 任意恶意网页/其他    │ token    │  ┌───────────▼────────────┐  │  │
│  │ 标签页/其他 OS 用户  │ +Host 拦 │  │ Probe Engine           │  │  │
│  └────────────────────┘          │  │ 并发探测/限流/脱敏收口   │  │  │
│                                  │  └───────────┬────────────┘  │  │
│                                  │  ┌───────────▼────────────┐  │  │
│                                  │  │ Credential Store (RAM) │  │  │
│                                  │  │ A1 唯一存在地           │  │  │
│                                  │  └────────────────────────┘  │  │
│                                  └──────────┬───────────────────┘  │
└─────────────────────────────────────────────┼───────────────────────┘
              TB-2 出网                       │ TB-3 日志/报告
   ┌──────────────────┬───────────────────────┴────────┬──────────────┐
   ▼                  ▼                                ▼              ▼
┌──────────────┐ ┌────────────────────┐      ┌──────────────┐ ┌────────────┐
│ 系统出网路径  │ │ 用户指定代理        │      │stdout/stderr │ │ 导出报告    │
│ TUN / 直连   │ │ 127.0.0.1:7890     │      │(默认不落文件) │ │ JSON/MD    │
└──────┬───────┘ │ 或 LAN:1080        │      └──────────────┘ └────────────┘
       │         └─────────┬──────────┘
       └────────┬──────────┘   TB-4 探测（出网）
                ▼
   ┌────────────────────────────────────────────────┐
   │ 固定目标集：dl.google.com / chatgpt.com /       │
   │ gemini.google.com / api.openai.com / googlevideo│
   │ （无自定义目标功能，见 §4.2 D1）               │
   └────────────────────────────────────────────────┘
        ✗ 禁止：169.254.169.254、127.0.0.1(非代理)、
          RFC1918、100.64/10、组播/保留地址
```

### 1.3 信任边界表

| 边界 | From → To | 信任变化 | 必施控制 |
|---|---|---|---|
| **TB-1** | 我们的 UI（同源上下文）→ 本地服务 | 本机，但仍需鉴权 | token、Host 校验、Origin 校验、CSP、`X-Frame-Options: DENY` |
| **TB-1'** | 任意网页/其他标签页/其他 OS 用户 → 本地服务 | **不可信** | 无 token 即 401；零 CORS 头；不批准 PNA preflight；Host 白名单 |
| **TB-2** | 进程 → 用户指定代理 | 用户信任该代理（可记日志） | 凭据仅在握手用；不落盘；不跟随代理跳到非目标主机 |
| **TB-3** | 进程 → 日志/报告/屏幕 | **敏感度降级点** | 唯一脱敏函数；报告 schema 白名单；零上传 |
| **TB-4** | 进程 → 公网目标 | 不可信（响应可被劫持） | 强制 TLS 校验；响应体大小上限；绝不执行/渲染远端内容，只做字节/正则指纹匹配 |
| **TB-5** | 构建机 → 依赖 | 不可信供应链 | `go.sum` 校验、锁定版本、`-mod=readonly`、无构建期远端脚本 |
| **TB-6** | OS → 进程权限 | 最小权限 | 普通用户运行，**不请求** admin/root |

### 1.4 攻击者模型

| 编号 | 攻击者 | 能力 | 动机 | 主要威胁 |
|---|---|---|---|---|
| **ADV-1** | 恶意网页（用户浏览器同时开着我们的 UI） | 页面内 JS、跨站请求、DNS rebinding、iframe 我们的页面 | 僵尸网络批量扫本机服务（已成常规手法） | 让工具用**攻击者代理**跑探测、UI 点击劫持、rebinding 读结果 |
| **ADV-2** | 同机其他 OS 用户 / 低权限进程 | 连 loopback、端口扫描、读全局可读文件 | 共享机、多用户机 | 蹭用工具外连、读报告里的代理地址 |
| **ADV-3** | 网络中间人（运营商/防火墙/企业代理） | DNS 劫持、TLS MITM、HTTP 篡改 | 审查、合规 | 伪造"你的代理很健康"、诱导关闭证书校验 |
| **ADV-4** | 恶意代理服务器 | 用户主动连接了它 | 免费代理投毒 | 记录/篡改探测流量、返回伪造内容 |
| **ADV-5** | 本机取证者 / 崩溃转储收集者 | 读 dump、日志、临时文件 | 事后取证、崩溃上报 | 从 dump 提取 A1 |
| **ADV-6** | 供应链攻击者 | 污染依赖、篡改产物 | 广撒网 | 二进制内植入窃取 A1 的代码 |

---

## 2. STRIDE 威胁建模

> 风险 = 可能性 × 影响，四档：Critical / High / Medium / Low。每行给出具体场景、可落地到代码的缓解、以及**必须承认的残余风险**。

### 2.1 威胁表

| # | STRIDE | 组件 | 场景 | 影响 | 缓解 | 残余风险 |
|---|---|---|---|---|---|---|
| T-01 | **S** | 本地 HTTP 服务 | 恶意网页用 DNS rebinding 让 `evil.com` → `127.0.0.1:PORT`，fetch `http://evil.com:PORT/api/probe` 并传 `proxy=attacker:8080` | **High**：用户真实流量被引到攻击者代理；结果可伪造 | ① 端口内核分配 ② Host 严格白名单 ③ 全部 `/api/*` 需 token ④ 零 CORS 头（读到也算它赢不了） | rebinding 仍可"发出请求但读不到响应"；token 已挡住副作用，残余 **Low** |
| T-02 | **S** | 端口预占 | 攻击者先绑定固定端口 8787；若程序"退让到下一个端口"，用户从收藏夹访问旧端口 → 打到攻击者假 UI | **High**：钓鱼收割代理密码 | ① 绝不用固定端口，用 `127.0.0.1:0` ② 无"端口回退"逻辑，绑定失败即报错退出 ③ UI 只从本进程 `open` 的 URL 进入 | 用户把 URL 存书签跨会话访问 → 无有效 token → 401，失败即安全 |
| T-03 | **S** | 探测目标证书 | MITM 用伪造证书冒充 `api.openai.com` 返回 200，工具误判"AI 可用" | **High**：核心结论错误 | 默认 `InsecureSkipVerify=false` + 系统根池；TLS 失败即该项失败，不降级；报告记 `tls.verified=true` | 系统根证书已被植入恶意 CA → 视为本机失陷（§0.2-1） |
| T-04 | **T** | 探测数据 | 明文 HTTP 代理篡改 `generate_204` 响应、伪造状态码、注水字节，使吞吐/RTT 虚高 | **Medium**：结果失真、误判"很流畅" | ① 强制 HTTPS 目标（`generate_204` 仅在无代理直连基线时允许 HTTP）② 校验 `Content-Length`/总字节数与期望一致 ③ 记录 `proxy.scheme`，HTTP 代理在报告打 `plaintext-hop` 标记 ④ 对目标页面做多项交叉指纹（状态码 + 关键 DOM 串 + `Content-Type`） | 恶意代理可精确伪造全部指纹；表现为"一致性极高但延迟为 0"的异常，仅靠启发式告警 |
| T-05 | **T** | 导出报告 | 报告被篡改后当作"证据"传播 | **Low** | 报告内不含安全判定权重以外的隐藏字段；对外声明"报告非签名证据" | 无 |
| T-06 | **R** | 用户操作 | 无法证明"这次探测是用户发起的" | **Low** | 记录 `startedAt`、`proxy.masked`、`targets[]`、`params` 到报告；不记录用户身份 | 无（本机单用户工具，不做不可否认性） |
| T-07 | **I** | 凭据 | 代理密码进入日志/错误信息/报告/`ps` 输出 | **Critical** | §5 全套：内存-only、单一脱敏函数、`url.URL` 的 `String()` 一律走 Redact、禁 CLI 传密码、panic 不落 UI | Go `string` 不可擦除；进程被 dump 时可提取（ADV-5） |
| T-08 | **I** | 探测结果 | 探测结果被回传给第三方（"看看你的分数"分享按钮） | **Medium**：泄露网络环境 | 无遥测、无自动上报；所有分享均为"用户手动导出文件"；报告内不含本机公网 IP JS 探测 | 用户自行分享后无法控制 |
| T-09 | **I** | 本地服务 | 其他标签页/其他用户读取 `/api/state`（含当前代理地址） | **Medium** | token 鉴权 + 零 CORS + `Cross-Origin-Resource-Policy: same-origin` | 同用户恶意进程可读内存 |
| T-10 | **I** | 错误响应 | 500 响应返回 Go 堆栈、文件绝对路径（含用户名 `C:\Users\alice`） | **Low–Medium** | 统一 `writeErr()` 只吐 `{code, message}`；堆栈仅在内网日志开关下写 stderr；`http.Server.ErrorLog` 重定向到脱敏 writer | 无 |
| T-11 | **D** | 本地服务 | 恶意网页持续 POST 触发大量探测 → 本机带宽被占满、可能被目标站风控 | **Medium** | token 挡住未授权；授权方（同一 UI）也做：全局单飞行探测（`409` 若有运行中任务）、固定目标集、下载字节上限、总超时 | 用户自己狂点造成自伤 |
| T-12 | **D** | 探测目标 | 工具被用来对目标站做高频请求，导致用户 IP 被 Cloudflare 拉黑 | **Medium**：反而害了用户 | 默认目标数固定 ≤ 8、单次探测请求数上限（如 ≤ 40）、同一目标最小间隔 500ms、不提供"循环压测"模式、`Retry-After` 严格遵循 | 用户开长时测试（60s+）仍会增加请求量 |
| T-13 | **E** | 探测目标（SSRF） | 用户被诱导填入 `http://169.254.169.254/latest/meta-data/` 为自定义目标，或代理指向内网管理口 | **High**：云元数据/内网服务被探测，结果出现在报告里 | §4：默认禁私网目标、`DialContext` 单一收口做 IP 段校验、HTTP 客户端禁跟随跨主机重定向、自定义目标默认关闭 | 若用户显式开启"允许私网目标"（不建议提供），则风险自担并写入报告 |
| T-14 | **E** | UI 点击劫持 | 攻击网页 iframe 我们的 UI（`127.0.0.1:PORT`），透明覆盖诱导用户点"开始探测" | **Medium**：以用户名义发起探测（仍读不到结果） | `X-Frame-Options: DENY` + CSP `frame-ancestors 'none'` + `Cross-Origin-Opener-Policy: same-origin` | 无 |
| T-15 | **E** | 供应链 | 依赖包被投毒（typosquatting / 恶意 patch 版本），构建时执行代码 | **Critical** | §8：依赖白名单最小化、`go.sum` + `-mod=readonly` + `GOFLAGS=-mod=vendor` 可选、npm `--ignore-scripts`、锁定 lockfile、无构建期 `curl \| sh` | 上游作者恶意提交（需人工 review 依赖升级） |
| T-16 | **E** | 平台 | 用户为"让工具能改系统代理"而用管理员权限运行，导致 A1 在全权限上下文暴露 | **Medium** | **明文结论：不需要管理员权限**（§9.3），UI 与文档明确声明；安装包不请求 UAC elevation | 用户自行 `sudo` 运行无法阻止 |
| T-17 | **T/S** | 证书校验降级 | 若提供"忽略证书错误"开关，用户遇到企业自签代理时会开启并长期保留，此后所有结论都在 MITM 下产生 | **High**：用户长期处于 MITM 下而不自知 | **通过设计消除**：v1 不提供任何用户可见的校验降级入口（§6.2，对齐 PRD §6 #5 / §7.4 #6）。`InsecureSkipVerify` 只允许出现在 `tls_diagnose.go` 的证书证据采集路径，该 client 不参与任何探测项计分；CI 用 grep 断言其唯一出现位置 | 企业 TLS 检查环境下该链路会直接测不出（有意取舍，UI 需给"装企业根证书"的正确指引） |
| T-18 | **I** | 系统代理读取 | 系统代理 URL 含企业凭据，被原样打进日志 | **Medium** | A6 与 A1 同等对待：走同一脱敏函数，只输出 `scheme://host:port` | 无 |

### 2.2 风险排序（实现与测试优先级）

| 优先级 | 威胁 | 理由 |
|---|---|---|
| P0 | T-07、T-10 | 凭据与路径泄露，一次泄漏不可逆 |
| P0 | T-01、T-02、T-09、T-14 | 本地服务是全系统唯一"对外开放"面，且是 ADV-1 的现成攻击路径 |
| P0 | T-03、T-17 | 直接决定产品核心结论是否可信 |
| P1 | T-13 | SSRF，可能把用户内网暴露给报告阅读者 |
| P1 | T-15 | 供应链，影响所有用户 |
| P2 | T-04、T-11、T-12、T-18 | 影响结果质量或用户体验，不直接影响机密性 |
| P3 | T-05、T-06、T-08 | 低影响或产品边界内已声明 |

---

## 3. 本地 HTTP 服务攻击面（重点）

> 这是全系统唯一"监听端口"，也是唯一能被本机其他主体/浏览器里任意网页触达的面。默认假设：**任何能连到该端口的人都可能是攻击者**。

### 3.1 监听与端口：全部硬性要求

```go
// 唯一允许的监听方式
ln, err := net.Listen("tcp", "127.0.0.1:0") // ✅ 内核分配端口
if err != nil { fatal("cannot bind loopback") }
port := ln.Addr().(*net.TCPAddr).Port
// 使用 http.Serve(ln, handler)，不要 http.ListenAndServe(":0")（后者可能落到 0.0.0.0）
```

| 规则 | 要求 | 反例（禁止） |
|---|---|---|
| 绑定地址 | 字面量 `127.0.0.1`，只监听 IPv4 loopback | `:8080`、`0.0.0.0:8080`、`[::]:8080`、`localhost:8080`（可能解析到 `::1` 且不同平台行为不一致） |
| 端口 | `0`（内核分配），仅在 UI 上展示 | 固定 8787/3000/8080 等常见端口；"端口被占就试下一个" |
| IPv6 | **首版只监听 IPv4 loopback**。若未来要支持 `::1`，Host 白名单必须同步加 `[::1]:PORT`，且必须独立校验 | 绑定 `::`（双栈全接口，等于公网暴露） |
| 失败处理 | 绑定失败 → 打印明确错误 → 退出（exit code 非 0） | 静默回退到其他端口/地址 |
| 权限 | 普通用户；不请求 root/admin | 为了绑定 80/443 而提权 |
| 生命周期 | 进程退出即关闭 listener；不常驻、不自启（v1 不做开机启动） | 注册 systemd/launchd 常驻服务 |

**为什么端口必须内核分配**：T-02。固定端口 = 可预测目标，既便于僵尸网络批量扫，也给端口预占钓鱼留了口子。内核分配使"攻击者要先知道端口"这一前提成立，而端口只通过 `open`/stdout 交给用户自己的浏览器。

### 3.2 会话 token（所有 `/api/*` 的准入凭证）

**生成**
```go
b := make([]byte, 32)
if _, err := rand.Read(b); err != nil { fatal("no entropy") } // crypto/rand，不用 math/rand
token := base64.RawURLEncoding.EncodeToString(b) // 43 chars
```

**下发（bootstrap 握手，只走一次）**

1. 启动后由进程自己打印/打开：`http://127.0.0.1:<port>/?t=<token>`
2. 浏览器请求 `GET /?t=<token>`：服务端**常数时间比较**（`subtle.ConstantTimeCompare`）校验 token
   - 不匹配 → `403` + 纯文本 `invalid bootstrap token`
   - 匹配 → 返回 HTML，并同时做两件事：
     - `Set-Cookie: pl_sess=<token>; Path=/; HttpOnly; SameSite=Strict`（loopback 上的 http 不设 `Secure`，因为部分浏览器对 `http://127.0.0.1` 的 Secure cookie 处理不一致；靠 SameSite + Host + Origin 兜底）
     - 在 HTML 中写 `<meta name="pl-token" content="<token>">`，供 JS 发自定义头
3. token **不写磁盘、不进日志、不进报告**；进程退出即失效（下次启动重新生成）
4. token 有效期 = 进程生命周期，且 bootstrap 端点只接受 **前 60 秒内的首次成功换取**（之后 `GET /?t=` 一律 403），既防"用户把带 token 的 URL 贴到聊天软件被复用"，也避免 `?t=` 长期留在历史记录里可用
5. 前端拿到 token 后 `history.replaceState(null,'','/')`，从地址栏抹掉 `?t=`

**校验（middleware，唯一收口）**

```go
func auth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !hostAllowed(r.Host) { writeErr(w, 403, "bad host"); return }        // §3.3
        if r.Method == http.MethodOptions { w.WriteHeader(405); return }        // 不响应任何 preflight
        // 状态变更类必须来自我们自己的 JS：自定义头无法被跨站表单伪造
        if isMutating(r) && r.Header.Get("X-ProxyLens-Token") != token {
            writeErr(w, 401, "unauthorized"); return
        }
        // SSE 无法设置自定义头 → 用 cookie（SameSite=Strict）或 ?t= 兜底
        if r.URL.Path == "/api/events" && !cookieValid(r) && r.URL.Query().Get("t") != token {
            writeErr(w, 401, "unauthorized"); return
        }
        if isMutating(r) && !originAllowed(r) { writeErr(w, 403, "bad origin"); return } // §3.4
        next.ServeHTTP(w, r)
    })
}
```

> **EventSource 限制（必须注意）**：浏览器 `EventSource` **不能**设置自定义请求头。因此 SSE 端点用 `pl_sess` cookie（`SameSite=Strict` + HttpOnly）鉴权，并额外接受 `?t=` 参数作为兜底。禁止为了"方便"把 SSE 端点改成无鉴权。

**token 泄露面的现实评估**：token 会出现在 (a) 进程启动时打印的 URL、(b) 浏览器地址栏（替换前）、(c) 浏览器历史（若替换失败）。这些都要求攻击者已能读用户屏幕/历史，属于本机失陷。可接受。

### 3.3 DNS rebinding 防护：Host 头白名单

DNS rebinding 的机理：攻击者域名先解析到公网 IP 通过浏览器同源检查，再重解析到 `127.0.0.1`，从而让页面 JS 向"自己的源"发请求，请求实际打到本机服务。此时浏览器发出的 `Host` 头是**攻击者域名**，不是 `127.0.0.1`。因此校验 `Host` 即有效。

```go
var allowedHosts = map[string]bool{
    fmt.Sprintf("127.0.0.1:%d", port): true,
    fmt.Sprintf("localhost:%d", port): true, // 仅当确实用 localhost 打开时才需要
}

func hostAllowed(h string) bool {
    return allowedHosts[strings.ToLower(h)] // 精确匹配，不做前缀/后缀/Contains
}
```

| 要求 | 说明 |
|---|---|
| 精确字符串匹配 | 禁止 `strings.Contains(host, "127.0.0.1")`（`127.0.0.1.evil.com` 可绕过） |
| 必须含端口 | `Host: 127.0.0.1`（默认 80）与 `127.0.0.1:<port>` 不同，只接受后者 |
| 不合法即 403 | 返回 `403`，**不重定向、不回显 Host 值**（防反射型注入） |
| 与 token 双保险 | 即使 Host 校验被绕过，无 token 依然 401 |
| 反代/系统代理干扰 | 若用户系统代理把 loopback 请求也代理走（少见但存在），`Host` 会被改写；此时工具应报错退出而不是放宽校验，并在 UI 提示"请为 127.0.0.1 设置绕过（NO_PROXY）" |

**实现提示**：Go 的 `r.Host` 即 Host 头原值；不要用 `r.URL.Host`（服务端为服务端解析结果，语义不同）。HTTP/2 也提供 `:authority`，`r.Host` 已统一。

### 3.4 CSRF：同源校验 + token + 内容类型约束

三层，任一层失效仍有下一层：

| 层 | 控制 | 实现 |
|---|---|---|
| L1 | 会话 token（自定义头） | 所有 `POST/PUT/DELETE` 必须带 `X-ProxyLens-Token`。跨站 `<form>` 无法设置自定义头 → 攻击失败 |
| L2 | Origin 校验 | 状态变更请求若带 `Origin` 头，必须严格等于 `http://127.0.0.1:<port>`（或 `http://localhost:<port>`）；**不为 `null`**（`file://`/sandboxed iframe 会发 `Origin: null`，一律拒）。缺失 `Origin` 的非浏览器客户端（如 `curl`）在有 token 的前提下放行——这是有意取舍：token 才是真正的准入 |
| L3 | Content-Type 白名单 | 状态变更请求必须 `Content-Type: application/json; charset=utf-8`；拒绝 `application/x-www-form-urlencoded`、`multipart/form-data`、`text/plain`。跨站"简单请求"只能发这三种，全被挡 |

补充要求：
- **所有状态变更必须 POST**。`GET /api/probe?proxy=...` 这种设计是反面教材（一个 `<img src>` 就能触发）。
- 不实现 `GET` 带副作用的端点（导出报告也用 `POST` 返回文件流，或 `GET` 但仅读缓存结果）。
- `SameSite=Strict` 的 `pl_sess` cookie 是 L0 兜底（现代浏览器默认已 `Lax`）。
- **不做** `Referer` 校验作为主控制（可能被 `Referrer-Policy: no-referrer` 剔除，易误伤）；可作为可选告警。

### 3.5 CORS 策略

**结论：一个 CORS 响应头都不发。** 同源访问（我们自己的 UI）不需要任何 CORS 头；跨源访问是我们明确要拒绝的。

| 规则 | 要求 |
|---|---|
| 禁止 `Access-Control-Allow-Origin: *` | 绝对禁止 |
| 禁止 `*` 与 `Access-Control-Allow-Credentials: true` 共存 | 浏览器本身会拒，但**中间件配错**常出现此组合，必须用测试断言在响应中不存在 |
| 不反射 `Origin` | 不实现"动态回显 Origin"逻辑，杜绝配置错误导致任意源可读 |
| 不响应 `OPTIONS` | 返回 `405 Method Not Allowed` 且不带任何 `Access-Control-*` 头；preflight 失败 = 跨源读写均不可行 |
| `Access-Control-Allow-Private-Network` | **绝不返回**（响应私有网络访问/PNA preflight 时保持静默拒绝）。这是 Chrome 对"公网页面访问 localhost"的额外闸门，不能替攻击者打开 |
| `Vary` | 若未来真的引入 CORS，必须加 `Vary: Origin`；v1 不发 CORS 故无需 |

**验收断言**：任意 `/api/*` 响应头中 `Access-Control-` 前缀出现次数必须为 **0**。

### 3.6 安全响应头与 CSP（内嵌资源，无 CDN）

所有响应（HTML 与 API）都加：

```
Content-Security-Policy: default-src 'none'; script-src 'nonce-<per-req-nonce>'; style-src 'nonce-<nonce>';
  img-src 'self' data:; font-src 'self'; connect-src 'self'; base-uri 'none';
  form-action 'none'; frame-ancestors 'none'; object-src 'none'
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
Referrer-Policy: no-referrer
Cross-Origin-Opener-Policy: same-origin
Cross-Origin-Resource-Policy: same-origin
Cross-Origin-Embedder-Policy: require-corp
Permissions-Policy: geolocation=(), camera=(), microphone=(), usb=(), interest-cohort=()
Cache-Control: no-store
```

| 要点 | 说明 |
|---|---|
| **不用 `unsafe-inline`** | 前端构建时必须把内联样式/脚本改成 nonce 或外链内嵌文件。若用 Vite/React，注意构建产物的内联 `<script>` 需注入 nonce 占位符并在服务端替换 |
| `frame-ancestors 'none'` + `X-Frame-Options: DENY` | 双写，兼容老浏览器；防 T-14 点击劫持 |
| `connect-src 'self'` | 禁止前端向任何外部域发请求（防止引入 CDN/埋点时偷跑，也防 G5 被违反） |
| `Cache-Control: no-store` | 防探测结果/HTML 被浏览器或中间件缓存留存（A5/A3） |
| 不设 `HSTS` | loopback 上的 http 无意义，别做戏 |
| 无 `Server` / `X-Powered-By` | 显式清空，减少指纹（`Server` 头由 Go 默认不发送，但要确保中间层不加） |

### 3.7 同机其他用户 / 其他进程 / 其他标签页

| 场景 | 风险 | 控制 | 残余 |
|---|---|---|---|
| **浏览器其他标签页**（ADV-1） | 跨源请求、rebinding、iframe | token + Host + 零 CORS + `frame-ancestors 'none'` | 无法阻止"发请求"，但副作用被 token 挡死 |
| **同机其他 OS 用户**（ADV-2） | loopback 无用户隔离（TCP 无 `SO_PEERCRED` 等价物），任何本地用户都能连 | token 是唯一准入；token 只在内存与用户自己的浏览器里 | 端口可被扫描到存在（`/` 返回 403/401，不泄露版本信息）；多用户机上"工具存在"这一事实可见 |
| **同机同用户其他进程** | 可读 `/proc/<pid>/cmdline`、可 ptrace | ① 命令行不留密码（§5.2）② macOS 无 `ptrace` 限制但需同用户 ③ 不做任何"本进程内可信"假设 | **同用户进程可读内存 = 不可防**（§0.2-1） |
| **退出后的残留** | 端口占用、共享内存、临时文件 | 进程退出关 listener；不写临时文件（报告由用户另存）；不建 socket 文件（用 TCP 而非 unix socket，避免 `/tmp` 文件名竞态与权限问题） | 无 |
| **`unix socket` 方案为何不用** | `/tmp/x.sock` 有 symlink 竞态、权限 0666 默认、跨平台（Windows AF_UNIX 支持有限） | 选择 TCP loopback + token | — |

### 3.8 端点清单与最小权限

| 方法 | 路径 | 鉴权 | 副作用 | 备注 |
|---|---|---|---|---|
| GET | `/?t=<token>` | token（一次性，60s 窗口） | 无 | bootstrap，返回内嵌 UI |
| GET | `/assets/*` | Host 校验（**不要求 token**） | 无 | 静态资源；无 token 时也可加载，因为 HTML 本身已受保护；避免前端资源加载与鉴权耦合 |
| GET | `/api/state` | token / cookie | 无 | 只返回脱敏后的状态（代理地址已 mask） |
| POST | `/api/probe` | token + Origin + JSON | **发起外连** | 请求体 ≤ 8 KiB；同一时刻只允许 1 个运行中的探测（否则 `409`） |
| POST | `/api/probe/cancel` | token | 取消 | 幂等 |
| GET | `/api/events` | cookie / `?t=` | SSE 长连接 | 单连接上限（如 4 个），超限关最旧的 |
| POST | `/api/export` | token + JSON | 读缓存结果生成文件 | 不重新探测 |
| POST | `/api/diagnose/tls` | token + JSON | 外连 | 证书诊断（§6.4），目标同样过 §4 校验 |

**禁止存在**的端点：`/debug/pprof/*`（`net/http/pprof` 绝不 import）、`/metrics`、`/version`（版本号走 `/api/state` 且需 token）、`/healthz`（本地工具无需）、任何 `GET` 触发的写操作、任何代理转发端点（`/proxy?url=` —— 这会把工具变成开放代理，**绝对禁止**）。

---

## 4. SSRF 风险与目标校验策略

### 4.1 三处用户可控的"连接意图"

| 入口 | 用户可填 | 连接目的地 | 真实风险 |
|---|---|---|---|
| **E1 代理地址** | `scheme://[user:pass@]host:port` | **我们的进程直连该 host:port** | 中。用户被诱导填内网管理口/云元数据，等于让我们替他对内网发起 TCP 连接（结果会暴露在"代理可用/不可用"里） |
| **E2 自定义测试 URL**（若提供） | 任意 URL | 经 E1 或系统路径请求 | **高**。经典 SSRF：`http://169.254.169.254/latest/meta-data/iam/security-credentials/` |
| **E3 目标站重定向** | 不可控，但目标站可返回 `302` 到任意地址 | 跟随重定向 | 中。`dl.google.com` 若被劫持可把我们导向内网 |

### 4.2 产品决策（先定范围，再谈校验）

| 决策 | 结论 | 理由 |
|---|---|---|
| D1 首版是否提供"任意自定义 URL" | **不提供**（已与 PRD 对齐：FR-P1~P6 全是固定端点点集，PRD 未定义自定义目标）。§4.5 的约束保留为**未来扩展的前置条件**：任何人要加这个功能，必须先满足 §4.5 全部条目 | 从源头消灭 90% SSRF 场景；产品价值不依赖任意 URL |
| D2 代理地址是否允许公网 IP | **允许**（用户可能用远程代理），但私网/保留段需分级处理（见 4.3） | 禁止会破坏真实用法 |
| D3 是否允许"私网目标" | **默认拒绝**；代理地址侧允许 RFC1918（因为本机/局域网代理是主场景），但**永不**允许云元数据与 link-local | 取舍点：代理必须允许局域网，探测目标不必 |
| D4 是否跟随重定向 | 跟随，但**最多 3 跳**，且每跳重新做主机校验；不同主机即视为新目标，仍需通过策略 | 兼顾功能与安全 |

### 4.3 网段策略表（唯一的判定依据）

```go
// 必须用 net/netip（Go 1.18+），不要用字符串前缀比较
var (
    alwaysDeny = []netip.Prefix{           // 任何场景都拒绝
        netip.MustParsePrefix("169.254.0.0/16"),   // link-local / 云元数据 169.254.169.254
        netip.MustParsePrefix("fe80::/10"),        // IPv6 link-local
        netip.MustParsePrefix("0.0.0.0/8"),        // "this network"
        netip.MustParsePrefix("100.64.0.0/10"),    // CGNAT（云内网常用）
        netip.MustParsePrefix("192.0.0.0/24"),     // IETF protocol assignments
        netip.MustParsePrefix("192.0.2.0/24"),     // TEST-NET-1
        netip.MustParsePrefix("198.18.0.0/15"),    // benchmark
        netip.MustParsePrefix("198.51.100.0/24"),
        netip.MustParsePrefix("203.0.113.0/24"),
        netip.MustParsePrefix("224.0.0.0/4"),      // multicast
        netip.MustParsePrefix("240.0.0.0/4"),      // reserved + broadcast
        netip.MustParsePrefix("::/128"),           // unspecified
        netip.MustParsePrefix("ff00::/8"),         // v6 multicast
    }
    loopback  = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")}
    private   = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"),
                               netip.MustParsePrefix("172.16.0.0/12"),
                               netip.MustParsePrefix("192.168.0.0/16"),
                               netip.MustParsePrefix("fc00::/7")} // ULA
)
```

| 目标类别 | 作为**代理地址**（E1） | 作为**探测目标**（E2/E3） |
|---|---|---|
| loopback `127/8`、`::1` | ✅ 允许（主场景 TUN/Clash） | ❌ 拒绝（除 E1 自身） |
| RFC1918 `10/8`、`172.16/12`、`192.168/16`、`fc00::/7` | ✅ 允许（局域网代理/网关） | ❌ 默认拒绝 |
| link-local `169.254/16`、`fe80::/10` | ❌ **拒绝**（云元数据，无正当用途） | ❌ 拒绝 |
| CGNAT `100.64/10` | ❌ 拒绝（云厂商内网/元数据代理） | ❌ 拒绝 |
| 公网单播 | ✅ 允许 | ✅ 允许（默认唯一允许） |
| 组播/保留/benchmark/文档段 | ❌ 拒绝 | ❌ 拒绝 |
| 非 IP 的主机名（FQDN） | ✅ 允许 → 解析后按上述 IP 规则复检 | ✅ 允许（限制见 4.5） |

> **为什么把 `169.254.0.0/16` 设为"任何场景都拒"**：它没有合法的代理/探测用途，唯一出名的用途就是云元数据（AWS/GCP/Azure/阿里云/腾讯云全在 `169.254.169.254`）。一刀切比"允许但告警"更安全，且零功能损失。

### 4.4 单一收口点：`DialContext` 里做 IP 校验（默认做法）

仅校验 URL 字符串是不够的——**DNS 可以在校验后重绑定**。唯一可靠的落点是实际建连时的 IP。

```go
func guardedDialer(mode Mode /* Proxy | Target */) *net.Dialer {
    return &net.Dialer{
        Timeout: 10 * time.Second,
        Control: func(network, address string, _ syscall.RawConn) error {
            host, _, _ := net.SplitHostPort(address)
            ip, err := netip.ParseAddr(host)
            if err != nil { return fmt.Errorf("non-ip dial") }
            return checkIPAllowed(ip, mode) // ✅ 唯一判定函数，违反即 error → 建连失败
        },
    }
}
```

关键性质：
- `Control` 在 **connect(2) 之前**被调用，拿到的是**已解析的最终 IP** → 天然免疫 DNS rebinding / TOCTOU。
- 每个 IP 都过一遍（dual-stack 会尝试多个地址）。**不能**"解析到第一个合法 IP 就放行整个域名"。
- 顺序：先 `LookupNetIP` 显式校验（给出好的 UI 错误信息），再用 `Control` 兜底（真正阻断）。**两道都要**，前者为了可读的错误提示，后者为了不可绕过的强制。

### 4.5 探测目标（E2/E3）的额外约束

| 约束 | 值 | 说明 |
|---|---|---|
| 协议 | 仅 `https`（`generate_204` 基线允许 `http`） | 明文目标可被任意中间人篡改，且 `http://` 是 SSRF 最常见的绕过（`gopher://`、`file://`、`dict://` 一律不支持，`url.Parse` 后检查 `Scheme`） |
| 端口 | 仅 443（+ 80 用于 204 基线） | 阻止 `:22`、`:6379`、`:9200` 等端口探测 |
| 主机形式 | 必须是 FQDN，且**禁止** IP 字面量 | IP 字面量是 SSRF 的首要特征；合法目标全是域名 |
| 禁止的主机名 | `localhost`、`*.localhost`、`*.local`、`*.internal`、`*.home.arpa`、`*.lan`、无点单标签名（如 `router`） | 内网设备常见命名 |
| 域名后缀 | 不含下划线（非法 DNS 但内网常见）、不含 `%`（防 IDN/百分号编码绕过） | 归一化后再判定 |
| 大小 / 长度 | URL 总长 ≤ 2048；`Host` ≤ 253 | — |
| 解析结果 | **所有** A/AAAA 记录都必须通过 4.3 的公网判定；任一落入禁止段即整体拒绝 | 防"一个域名同时解析到公网和内网"的 DNS 投毒技巧 |
| 重定向 | 最多 3 跳；每跳重跑本表全部校验；不允许 `https → http` 降级 | 降级 = 放弃 TLS 保护 |
| 凭据 | 目标 URL 中**禁止** `user:pass@` | 无正当用途，且易被用于 `https://victim.com@169.254.169.254/` 类混淆 |

### 4.6 代理地址（E1）的额外约束

| 约束 | 值 |
|---|---|
| scheme | 仅 `http`、`https`、`socks5`、`socks5h`（`socks5h` = 远端 DNS，DNS 在代理侧解析，我们看不到目标 IP，故**必须**改用主机名白名单策略，见下） |
| 端口 | 1–65535，但拒绝 `0` 与 < 1024 的低端口（除 `1080`/`1081`/`3128`/`7890`/`7891`/`8080`/`8118`/`8888`/`10808` 等常见代理端口——**不强制**，仅做异常提示）。**不阻断**，避免误伤自建代理 |
| 地址解析 | 显式 `LookupNetIP` + `Control` 双校验（4.4），按 4.3 的 Proxy 列判定 |
| 凭据长度 | `user` ≤ 128、`pass` ≤ 256，含 `:`/`@` 必须 URL 编码（解析失败即拒绝，并提示编码而非"尽力猜测"） |
| 连接超时 | 建连 8s，握手 8s，整体探测可配 — 防"输入一个黑洞地址把自己挂死"（T-11） |

**`socks5h` 的 SSRF 特殊性（必须写进实现说明）**：远端 DNS 下我们只能控制"代理连谁"，无法验证解析结果。因此：
- `socks5h` 模式下，探测目标**必须**来自固定目标集或 FQDN 白名单（后缀匹配 `google.com`/`googleapis.com`/`gstatic.com`/`googlevideo.com`/`openai.com`/`oaistatic.com` 等）；自定义目标在 `socks5h` 下**不可用**。
- 这是有意的功能取舍，须在 UI 上说明（"远端 DNS 模式下无法校验目标安全性，自定义目标已禁用"）。

### 4.7 报告中的安全标注

每次导出的报告必须包含（便于事后判断结果是否可信）：

```json
"security": {
  "tlsVerification": "strict",        // v1 唯一合法值；不存在 disabled-by-user
  "tlsEvidenceCollected": true,       // 是否做过证书证据采集（只读诊断，不影响计分）
  "proxyScheme": "socks5h",
  "plaintextHop": false,
  "remoteDNS": true,
  "targetPolicy": "fixed-catalog",
  "privateTargetsBlocked": 2,      // 本次被策略拦截的次数（>0 说明有人试图探测内网）
  "warnings": ["REDIRECT_CROSSED_HOST"]
}
```

`privateTargetsBlocked > 0` 时 UI 必须提示："检测到有目标被安全策略拦截，可能填错了地址"。

---

## 5. 凭据安全（T-07 的完整对策）

### 5.1 生命周期（唯一合法的数据流）

```
用户键盘输入
  → <input type="password"> (浏览器内存)
  → POST /api/probe  body: {"proxy":"socks5://user:pass@host:port"}  ← loopback，HS256 无，明文但仅本机
  → Go: url.Parse → 取出 User → 存入 session struct（[]byte 保存密码）
  → 仅在需要认证的那一次连接中交给 net/http / golang.org/x/net/proxy
  → 探测结束 / 取消 / 进程退出 → 显式清零（overwrite）
  ✗ 不写磁盘  ✗ 不进日志  ✗ 不进报告  ✗ 不进 os.Args  ✗ 不进 env  ✗ 不进回调 URL
```

### 5.2 硬性规则

| # | 规则 | 落地方式 | 反例（禁止） |
|---|---|---|---|
| C1 | **代理地址与凭据整体不落盘**（对齐 PRD v1.1 裁决与 FR-A6 第 2 条） | ① **零**持久化的对象：代理协议、主机、端口、用户名、密码、完整 URL、探测目标 URL、系统代理原文；`settings.json` / `history.jsonl` / 日志 / `sessionStorage` / `localStorage` / cookie 一律不得含上述字段；② **允许**落盘的对象：仅非敏感 UI 设置（主题、语言、超时阈值、窗口尺寸、并发数等枚举/数值），按 §7.5 白名单字段化落盘；③ 不存在"记住代理""上次使用的代理"功能，重启后输入框为空 | 用 `localStorage` 存上次代理地址；把 `proxy` 字段塞进 `settings.json` 并只把密码置空（"用户名+host+port 可恢复"在 v1.1 已裁决**取消**） |
| C2 | **不写日志** | 全项目**只有** `redact.go` 里的 `Sanitize(string) string` 与 `MaskProxy(u *url.URL) string` 被允许处理含凭据的对象；`golangci-lint` 加自定义规则：禁止对 `*url.URL` 直接 `%v/%s/Println` | `log.Printf("using proxy %s", proxyURL)` |
| C3 | **不进报告** | 报告字段用**白名单**结构体（`type ReportProxy struct{ Scheme, Host string; Port int; HasAuth bool }`），不是 `json.Marshal(原始结构)` | 直接把 `config` 序列化导出 |
| C4 | **内存处理** | 密码用 `[]byte` 保存，会话结束时 `for i := range b { b[i] = 0 }`；不在 `struct` 里用 `string` 字段存密码（`string` 无法擦除） | `Password string` 字段长期存活 |
| C5 | **UI 打码** | 输入框 `type=password` + `autocomplete="off"` + `spellcheck=false`；回显一律 `socks5://user:***@host:port`（固定三个星，**不泄露长度**）；不做"点击显示密码"（除用户主动在输入框内切换） | 回显 `pass: pa**`（泄露长度与首尾字符） |
| C6 | **命令行不承载密码** | **不提供** `--proxy` 参数（v1 只能通过 UI 输入）。若为 CI 场景保留 `--proxy`，则解析后若含 userinfo **直接报错退出**并提示"通过 UI 或 stdin 输入" | `proxylens --proxy socks5://u:p@h:1080`（`ps aux` 全机可见，Windows 上 WMI 也能读） |
| C7 | **`ps` / `/proc` 不可见** | 进程标题固定为 `proxylens`；不用 `os.Args` 传递任何敏感值 | 把密码塞进临时 env（`os.Getenv` 可见于 `/proc/<pid>/environ`，macOS 上 `ps -E` 也可读） |
| C8 | **错误信息脱敏** | Go 标准库 `url.Parse` 的错误**会包含完整 URL**（如 `parse "socks5://u:p@h:port": invalid port`）→ 所有 error 输出的位置必须过 `Sanitize()`，且 `Sanitize` 用正则把 `://[^/@]*@` 替换为 `://***@` | `http.Error(w, err.Error(), 500)` |
| C9 | **崩溃不泄露** | ① `recover()` 中间件：panic → 记日志（脱敏后，仅 stderr）→ 客户端收 `500 {"code":"INTERNAL"}`，**不含堆栈** ② 关闭 unix core dump：`syscall.Setrlimit(RLIMIT_CORE, {0,0})` ③ Windows 不主动触发 WER 完整转储；不提供"发送崩溃报告"功能 ④ 关键点：不让密码出现在 panic 可达的局部变量名/格式串里（Go 堆栈只打函数与地址，风险主要在 error 字符串） | 把 `err.Error()`（含 URL）拼进 `panic(...)` |
| C10 | **输入框不留在 DOM** | 探测提交后立即清空 `<input>` 的 `value`，只在 state 里保留 masked 字符串 | 让密码一直躺在输入框里等用户截图 |
| C11 | **剪贴板** | "复制代理"按钮复制的是 **masked** 形式；若用户需要复制含密码的原文，必须二次确认并在 30s 后提示"已复制到剪贴板，请及时清空" | 一键复制含密码 URL |
| C12 | **系统代理（A6）同等待遇** | `scutil --proxy` / 注册表读到的系统代理若含 userinfo，展示与记录都走 `MaskProxy` | 把系统代理原文打进"环境信息"卡片 |

### 5.3 脱敏函数（唯一实现，必须照抄语义）

```go
// MaskProxy 返回可安全展示/记录/导出的代理字符串。
func MaskProxy(u *url.URL) string {
    cp := *u
    if cp.User != nil {
        name := cp.User.Username()
        if name == "" { name = "user" }
        cp.User = url.UserPassword(name, "***") // 固定三星，不泄露长度
    }
    // 额外：去掉 query/fragment（可能含 token）
    cp.RawQuery, cp.Fragment = "", ""
    return cp.Redacted() // 标准库兜底，双保险
}

// Sanitize 处理任意字符串（含 error 文本）中的 user:pass@ 片段。
var userinfoRe = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/@\s"']*@`)

func Sanitize(s string) string {
    return userinfoRe.ReplaceAllString(s, "${1}***@")
}
```

**测试断言（安全用例强制项）**：
- `MaskProxy(url.Parse("socks5://alice:s3cr3t@1.2.3.4:1080"))` == `socks5://alice:***@1.2.3.4:1080`
- `Sanitize(`parse "http://u:p@h:99999": invalid port`)` 不含 `p@`、不含 `:p`
- 全仓库 grep：`url.URL` 类型的变量不得出现在 `fmt.Print*/log.*` 的参数位置（CI 用 `go vet` 自定义 analyzer 或 semgrep 规则）

### 5.4 为什么**不**做"记住密码"（含 Keychain/DPAPI 的取舍）

brief 要求"凭据不得明文落盘"，技术上可以用 macOS Keychain（`SecItemAdd`）或 Windows DPAPI（`CryptProtectData`）实现加密存储。**本项目的决定：v1 不做**。

| 理由 | 说明 |
|---|---|
| 收益低 | 用户每次开工具出于"测一下现在行不行"的短会话场景，重输一次密码成本 < 5s |
| 成本与风险高 | 引入 Keychain/DPAPI 需要 cgo 或平台 API 封装（Keychain 走 cgo 会破坏 `CGO_ENABLED=0` 的纯静态单文件分发），且加密存储只防"离线读取"，不防"同用户进程读取" |
| 攻击面 | 有加密存储就必须设计"解锁/超时/清除"策略，属于新增功能而非安全增强 |
| 若未来要做 | 必须：Keychain（`AccessControl` 绑 `kSecAccessControlUserPresence`）/ DPAPI + `CRYPTPROTECT_LOCAL_MACHINE` 以外的作用域；默认不勾选；提供"立即忘记"；且必须在 UI 明示"已加密保存于系统钥匙串" |

---

## 6. TLS 与中间人（MITM）

### 6.1 默认策略：严格校验，无例外

```go
tr := &http.Transport{
    TLSClientConfig: &tls.Config{
        MinVersion: tls.VersionTLS12,   // 不允许 TLS 1.0/1.1
        // InsecureSkipVerify: false    // 默认值即 false，不要写 true
        RootCAs: nil,                   // nil = 使用系统根证书池（正确做法）
    },
    ForceAttemptHTTP2: true,
    MaxIdleConnsPerHost: 2,
    TLSHandshakeTimeout: 8 * time.Second,
}
```

| 要求 | 说明 |
|---|---|
| 校验主机名 | 默认启用（`ServerName` 由 URL host 自动填） |
| 校验有效期 | 默认启用 |
| 校验链 | 系统根池；**不**加载任何内置 CA bundle 覆盖系统池 |
| TLS 版本下限 | 1.2；`api.openai.com`/Google 均已 1.3，若仅 1.0/1.1 可用应视为"链路可疑"并在报告标注 |
| SNI | 使用真实主机名（不要为空；空 SNI 会被部分中间设备特殊对待） |
| 失败即失败 | 证书错误 → 该探测项 `status=failed, reason=TLS_VERIFY_FAILED`，**不得**静默降级重试 |

### 6.2 不提供任何"忽略证书校验"开关（对齐 PRD §6 #5 / §7.4 #6）

**结论：v1 不提供、且不预留用户可见的证书校验降级入口。** 这是产品级安全决策，不是实现细节。

| 维度 | 规定 |
|---|---|
| 用户可见开关 | **不存在**。设置面板不出现、高级区不出现、CLI 不提供 `--insecure`、环境变量不提供 `PROXYLENS_INSECURE` |
| 为什么不给 | ① 它把"测不出"变成"假装能测"，直接摧毁产品核心断言（G4）；② 一旦存在，用户遇到企业代理自签证书就会开它并**永久留着**（T-17），此后所有结论都不可信；③ 违反本文件铁律"不推荐关闭安全控制作为解决方案"——正确的解法是**修根因**（把企业 CA 装进系统信任库）或**接受测不出** |
| 正确解法（写进 UI 文案） | 证书异常时提示："如果你在企业网络，需要把企业根证书安装到系统信任库；本工具不会跳过证书校验来绕过这个问题" |
| 内部诊断能力 | **仍然存在**，但仅在 `TLS 诊断` 这一只读路径内：以 `InsecureSkipVerify=true` 单独建连**只为采集证书证据**（§6.3），随后用 `x509.Verify` 做一次真实验证，两者结果分开报告。**诊断结果永远不参与计分，也不改变主探测路径的严格校验** |
| 实现红线 | `InsecureSkipVerify: true` 在代码中只允许出现在 `tls_diagnose.go` 的**单个**函数内，且该 `http.Client` 不用于任何探测项。CI 用 `grep -n "InsecureSkipVerify"` 断言只出现在该文件（见 S-20） |
| 报告字段 | `security.tlsVerification` 在 v1 只有一个合法值 `"strict"`；另设 `tlsEvidenceCollected: true` 表示做过证据采集。**不存在** `disabled-by-user` 这类取值 |

> 这样处理的好处：用户拿到"为什么连不上"的完整解释（证书链是否完整、签发者是谁、是否疑似 MITM），但工具永远不替用户降低安全标准。诊断价值保留，判定纯洁性也保留。

### 6.3 如何区分"证书被劫持" vs "证书链不完整"

这是本产品一个真实的技术难点，也是用户最需要答案的问题（"我这是被公司代理拦了，还是被运营商劫持了？"）。**做法：不要只判定失败原因，要采集结构化证据再推断。**

实现方式：诊断项用 `InsecureSkipVerify=true` 单独建连，**只为拿证书**，然后手工 `x509.Certificate.Verify()` 做真实校验，把两者结果分开报告：

```go
type TLSFinding struct {
    HandshakeOK   bool
    VerifyErr     string   // 手工 Verify 的错误类别
    ChainLen      int      // len(PeerCertificates)
    LeafSubject   string
    LeafIssuer    string
    NotBefore     time.Time
    NotAfter      time.Time
    SigAlg        string
    ChainRootHitSystemPool bool
    SelfSigned    bool
    IssuerLooksLikeMITMProd bool // 命中已知 MITM 产品/企业 CA 名称特征
    LifetimeHours float64  // NotAfter-NotBefore
}
```

判定表：

| 观察特征 | 结论 | 报告文案 |
|---|---|---|
| `VerifyErr=nil`，`ChainLen≥2`，根命中系统池 | 正常 | 证书链完整可信 |
| `VerifyErr=UnknownAuthority` + `ChainLen==1` + `LeafIssuer != LeafSubject` | **证书链不完整**：服务端没下发中间证书（客户端缺中间 CA）。典型于自建站、老设备、部分 CDN 配置 | "服务端未发送完整证书链（可能需补 intermediate），替换中间人可能性低" |
| `VerifyErr=UnknownAuthority` + `ChainLen==1` + `SelfSigned` | 自签名证书 | "自签名证书，非公共 CA 签发" |
| `VerifyErr=UnknownAuthority` + `LeafIssuer` 命中 MITM 特征（`mitmproxy`、`Charles`、`Fiddler`、`Burp`、`Zscaler`、`Netskope`、`Fortinet`、`Palo Alto`、`*.corp`、`*内网 CA 名*`）或 `LifetimeHours < 24`（即时签发的短证书）或 `LeafSubject` 与 `LeafIssuer` 同名 | **高度疑似 TLS 中间人** | "疑似中间人拦截：证书由 `X` 即时签发，非目标站真实证书。若你正在使用企业网络/杀软/代理，这是预期行为；否则视为劫持" |
| `VerifyErr=HostnameError` | 链可信但域名不匹配 | "证书与域名不匹配，可能被劫持或连接到错误的服务" |
| `VerifyErr=CertificateInvalid(Expired)` | 过期 | "证书已过期" |
| `NotBefore` 在未来 | 系统时间错误或伪造证书 | "设备时间可能不正确，或证书伪造" |
| `SigAlg` 为 `SHA1*`/`MD5*` | 弱签名 | "使用已弃用签名算法，链路可疑" |

**关键设计**：把"疑似 MITM"做成**证据 + 推断**（列出 ChainLen、Issuer、有效期、是否命中系统根），让用户能自己判断。**不要**给出"你被劫持了"的强断言——在企业网络里这是正常行为，误报会造成恐慌。

### 6.4 证书透明（CT）与公钥固定：做还是不做

| 方案 | 决定 | 理由 |
|---|---|---|
| Certificate Transparency 日志校验 | **不做** | Go `crypto/tls` 不做 CT 强制校验；自行实现需信任 CT 日志清单，复杂度高、收益低于成本 |
| SPKI pinning（固定公钥） | **不做** | Google/Cloudflare 轮换频繁，pin 会导致大量误报"不可用"，与产品目标（测可用性）冲突 |
| 记录 SPKI 指纹（sha256） | **做**（报告字段 `tls.spkiSha256`） | 零成本，用户可对比两次结果；若同域名指纹突变 → UI 提示"服务端证书可能已更换或被拦截" |
| 证书指纹对比基线 | **不做** | 需要云端基线 = 违反 G5 |

---

## 7. 数据最小化与隐私

### 7.1 收集了什么（全部在内存，全部可解释）

| 数据 | 用途 | 是否入报告 | 备注 |
|---|---|---|---|
| 各探测项耗时/TTFB/吞吐/成功率 | 核心功能 | ✅ | 聚合值，非原始样本（原始样本可选择性导出） |
| HTTP 状态码 | AI 可用性判定 | ✅ | — |
| 响应体前 N KiB（仅用于指纹匹配） | 识别 Cloudflare 挑战/地区限制页 | ❌ 仅存匹配结果与命中片段（≤120 字符） | 匹配完即丢；**不落盘** |
| 页面 title / 关键 DOM 选择器命中 | 可用性判定 | ✅ 仅命中结果 | — |
| TLS 证书元信息 | 安全诊断 | ✅（issuer/subject/有效期/SPKI 指纹） | 不含证书原文 |
| 代理 host:port、scheme | 结果语境 | ✅（已 mask） | 不含 user/pass |
| 本机公网 IP | — | ❌ **不采集** | 不调用任何 `ipify`/`ifconfig.me`；地理位置推断仅来自目标站返回的地区文本，且报告中明确标注"推断，非实测 IP" |
| 用户身份 / 主机名 / 用户名 / 系统序列号 | — | ❌ 不采集 | 版本信息仅 `OS` 家族 + 架构 |
| 使用行为 / 崩溃 / 遥测 | — | ❌ **完全没有** | 无 telemetry SDK、无 crash reporting、无 analytics |

### 7.2 隐私硬性要求

| # | 要求 | 实现 |
|---|---|---|
| P1 | **零外传** | 除 §4 白名单目标外无任何出连；CI 加测试：用 `httptest` 记录所有出连 host，断言 ⊆ 允许集合 |
| P2 | **前端零外部依赖** | 无 Google Fonts、无 CDN、无 analytics script、无外部图片。`grep -r "https://" dist/` 只允许出现在注释与文档链接中；CSP `connect-src 'self'` 兜底 |
| P3 | **报告仅本地** | 导出走浏览器下载（`Content-Disposition: attachment`）或用户指定路径的 `POST /api/export`；**没有"上传/分享/云端"按钮** |
| P4 | **不承诺 IP 地理位置的强结论** | 报告措辞固定：`regionHint: "inferred-from-text"`，禁止出现 "你的 IP 在 XX 国" 这类断言。理由：地区限制页文本可能来自缓存、CDN 边缘、甚至劫持者伪造 |
| P5 | **User-Agent 不暴露用户身份** | 见 7.3 |
| P6 | **报告可追溯谁生成** | 无。报告不含机器标识（有意为之：报告可安全分享） |
| P7 | **删除权** | 无服务端数据；进程退出后内存释放即"删除"。临时下载文件：全部流式丢弃到 `io.Discard`（或最多写入 `os.DevNull`），**不创建临时文件**。本地持久化**仅限** §7.5 的三类文件，且"清空历史"必须删除文件本身；卸载后数据目录不留残余 |

### 7.3 User-Agent 策略

| 场景 | UA | 理由 |
|---|---|---|
| 探测 `chatgpt.com` / `gemini.google.com` | 一个**固定的、与真实浏览器版本一致的** UA 字符串（如 `Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36`） | 这类站点对非浏览器 UA 会直接给 403/挑战页，测不到真实"浏览器能否打开"的语义。这是**功能必需**的伪装，属产品需求 |
| 探测 `api.openai.com` | `ProxyLens/<version> (+local-diagnostic)` | API 端点无浏览器要求；明确自报身份更诚实，且 `401` 语义不受 UA 影响 |
| 下载 `dl.google.com` | 同上自报 | 不影响 CDN |

**红线**：
- UA 中**禁止**包含：用户名、主机名、邮箱、代理地址、机器 ID、UUID。
- 不伪造 `Accept-Language` 以"绕过地区限制"（那会污染"地区限制"这一核心判定）；使用系统默认或固定 `en-US,en;q=0.9` 并**记录进了哪个值**。
- 不做 UA 轮换/随机化（那是在规避风控，不是测量）。

### 7.4 响应体处理（防"被污染的数据"变成"被污染的内存"）

| 限制 | 值 | 目的 |
|---|---|---|
| 单响应读取上限 | 探测类 512 KiB；下载类走 `io.LimitReader` 到设计字节数 + 16 KiB | 防超大响应打爆内存（T-11） |
| 不解析 HTML 到 DOM | 只做**字节级正则/字符串包含**匹配指纹 | 避免引入 HTML 解析器的 XXE/原型污染类风险，也避免渲染 |
| 不跟随 `<script>`、不执行 JS | 无 headless 浏览器（v1） | 若未来引入 headless，必须单独威胁建模（这是重大新增攻击面） |
| 指纹列表 | 硬编码常量（"not available in your country"、"Just a moment..."、"Attention Required"、"Enable JavaScript and cookies to continue" 等） | 不接受远端下发的规则表（防"规则投毒"） |

### 7.5 本地持久化文件的安全基线（对齐 PRD FR-F1 / FR-G8 / FR-H3）

> 本文初稿按"零持久化"设计；PRD 引入了三类本地文件（设置、历史快照、可选诊断日志）。这里给出**唯一允许的持久化基线**。
> 总原则：**凭据零持久化是绝对的；结果类数据可持久化，但必须有内容白名单 + 文件权限 + 可彻底删除。**

**位置与权限（硬要求）**

| 平台 | 目录 | 权限 | 禁止位置 |
|---|---|---|---|
| macOS | `~/Library/Application Support/ProxyLens/` | 目录 `0700`，文件 `0600` | `/Library/Application Support`（系统级、需提权、多用户可读）、`/tmp`、CloudStorage 目录 |
| Windows | `%LOCALAPPDATA%\ProxyLens\` | 依赖用户 ACL（`LOCALAPPDATA` 天然按用户隔离） | **绝不用 `%APPDATA%`**（漫游配置文件会被域漫游/OneDrive 同步到服务器，等于把代理 host 与历史分数带出本机）、`%ProgramData%`、`%TEMP%` |

| 文件 | 内容白名单（只有这些可以出现） | 明确禁止 | 生命周期 |
|---|---|---|---|
| `settings.json` | `schemaVersion`、`mode`、代理的 `scheme/host/port/username`、档位、并发数、字节预算、超时、采样次数、主题、语言 | **密码（任何形式**：明文、空串占位、哈希、加密、base64）、完整原始 URL、`?t=` token、session cookie | 覆盖写；损坏 → 回落默认值（FR-G8），不得启动失败 |
| `history.jsonl`（每行一条，≤50 条） | 时间戳、`mode`、代理 **masked** 摘要（`scheme://host:port` + `hasAuth` 布尔）、出网方式枚举、六项状态、指标聚合值、三分项与总分、结论类型、`security` 段 | 密码、完整 URL（含 query/fragment）、**响应体与响应体片段**（只存命中规则 ID）、证书原文、IP 地址、主机名/用户名等机器标识、token | 追加 + 超限淘汰最旧；"清空历史"必须**删除文件本身**而非清空内容 |
| `proxylens.log`（P2 可砍，≤5 MiB 滚动） | 时间戳、级别、阶段名、错误类别（枚举）、脱敏后的代理摘要 | 响应体、请求/响应头原文、密码、token、绝对用户路径（`C:\Users\<name>`）、堆栈中的 error 原文（未过 `Sanitize`） | 滚动覆盖；随"清空"一并删除 |

**写入方式**：创建时直接带权限（`os.OpenFile(path, O_CREAT|O_WRONLY|O_EXCL, 0600)`，或先写 `*.tmp` 再 `Rename`），**不要**先以 `0644` 创建再 `chmod`（存在可被读的时间窗）。历史文件追加写不重写全文件（避免"读到半个文件"）。

**为什么不做应用层加密（结论，附对产品的影响评估）**

| 方案 | 安全收益 | 成本 / 问题 | 判定 |
|---|---|---|---|
| 无口令，密钥与数据同盘 | **零**（同用户进程与取证者都能拿到密钥） | 增加代码量与"我们加密了"的虚假安全感 | ❌ 安全剧场 |
| 用户口令加密（每次启动输入） | 有（能挡离线取证） | 每次启动一个口令框；忘密码 = 历史与设置全丢，而 PRD FR-G8 要求"设置损坏必须回落默认值并启动成功"，加密失败会变成"每次启动都坏"；用户会写便利贴或直接放弃 | ❌ 收益不抵成本 |
| 系统钥匙串/DPAPI 保管密钥 | 仅挡"离线读文件"，挡不住同用户进程 | 需平台 API 封装，破坏 `CGO_ENABLED=0` 纯静态单文件；与 FR-A6#2"密码不恢复"策略不一致 | ❌ v1 不做 |
| **磁盘全盘加密 + 字段最小化 + 0600 + 可删除** | 正确层次：静态数据由 OS 保护，应用不再自造密钥体系 | 仅需 README 建议开启 FileVault / BitLocker | ✅ **采纳** |

**残余敏感度评估**：去掉密码、完整 URL、IP、机器标识后，历史文件剩下的信息 ≈ 浏览器历史（"某个时间点用过代理、跑分多少、哪些站点可达"）。这个量级值得做的是**字段最小化 + 可删除 + 不漫游**，而不是应用层加密。

---

## 8. 供应链与依赖安全

### 8.1 依赖最小化原则（硬性）

| 层 | 允许 | 禁止 |
|---|---|---|
| Go 运行时依赖 | 标准库优先。允许：`golang.org/x/net`（proxy/socks5）、`golang.org/x/sys`（系统代理读取）。**其他一律需 ADR 说明理由** | Web 框架（gin/echo/chi 视情况，优先 `net/http` + `ServeMux`）、ORM、日志框架（用 `log/slog`）、配置框架、任何"全家桶" |
| 前端依赖 | 优先零依赖原生实现；若用框架，锁定版本并 vendored 构建产物 | 运行时 CDN 引用、`eval` 类库、任何需要在浏览器端联网的 SDK |
| 依赖总量红线 | Go 直接依赖 ≤ 8 个；`go.sum` 中间接模块总数 ≤ 120 | 超过需 security/arch 双方签字 |

### 8.2 完整性校验

| 控制 | 实现 |
|---|---|
| `go.sum` 校验 | 提交 `go.sum`；CI 与发布构建一律 `GOFLAGS=-mod=readonly`，任何篡改导致构建失败 |
| 版本锁定 | `go.mod` 固定精确版本（不用 `latest`）；不允许 `replace` 指向本地路径进入发布构建 |
| 可复现构建 | `CGO_ENABLED=0`、`-trimpath`、`-buildvcs=false`（避免嵌入 commit 与时间）、固定 `-ldflags "-s -w -X main.version=x.y.z"`；同 tag 两次构建产物 sha256 应一致（跨机器差异仅来自 Go 版本，需记录 toolchain 版本） |
| npm | 提交 `package-lock.json`；安装用 `npm ci --ignore-scripts`；构建产物必须完整内嵌（`go:embed`），发布时**不执行**任何前端构建 |
| 构建期零网络拉取 | 发布脚本禁止 `curl \| sh`、禁止下载 extra tools、禁止 `go get`。Go 工具链按 `go.mod` 的 `toolchain` 指令固定版本 |
| 依赖漏洞扫描 | CI 跑 `govulncheck ./...`，Critical/High 阻断合并；前端跑 `npm audit --omit=dev` |
| SBOM | 构建时生成 `go version -m <binary>` 输出作为简易 SBOM 附在 release 说明（Go 会把依赖与版本写进二进制） |

### 8.3 发布完整性（A7）

| 控制 | 要求 |
|---|---|
| 哈希公布 | Release 页给出每个产物的 `sha256`；同时提供 `SHA256SUMS` 文件 |
| 签名 | 优先：Windows Authenticode 签名 + macOS `codesign` + 公证（notarization）。若短期无证书：**必须**公布哈希并在文档说明"未签名"（见 §9） |
| 安装包 | 不做"静默下载更新"（v1 无自动更新）。若未来做，必须签名校验（Ed25519 detached signature）+ HTTPS + 版本回滚保护 |
| 依赖投毒防护 | Go module 路径全部来自知名域（`golang.org/x/*`、`github.com/<known>`）；新增依赖须 review 作者与提交历史；启用 `GONOSUMDB`/`GOSUMDB=sum.golang.org`（默认开） |

---

## 9. 平台侧安全

### 9.1 macOS：未签名分发的 Gatekeeper

| 情形 | 用户看到 | 我们的对策 |
|---|---|---|
| 直接下载未签名 `ProxyLens.app` / 二进制 | "无法打开，因为无法验证开发者" | ① 文档给出明确步骤：右键 → 打开；或 `系统设置 → 隐私与安全性 → 仍要打开`；② 同时公布 `sha256`，并在文档中说明"如果你无法核对哈希，请不要运行" |
| `xattr` 隔离属性 | `com.apple.quarantine` 导致拦截 | 文档给出 `xattr -d com.apple.quarantine ./proxylens` 作为可选操作，并**说明其风险**（等于放弃 Gatekeeper 检查） |
| 有无签名 / 公证的区别 | 有公证 → 双击即开 | 长期方案：Apple Developer ID + `notarytool` 公证。首版可无，但必须在 README 首屏声明 |
| **红线** | 绝不引导用户 `spctl --master-disable`（全局关闭 Gatekeeper） | 这是对整机安全的破坏，比本工具的安全问题严重得多 |
| 二进制权限 | 需 `chmod +x` | 分发时保留可执行位（zip 保留，tar.gz 保留） |

### 9.2 Windows：SmartScreen

| 情形 | 用户看到 | 对策 |
|---|---|---|
| 未签名 exe | SmartScreen "Windows 已保护你的电脑" → "更多信息" → "仍要运行" | 文档给步骤 + 公布 `sha256`（PowerShell `Get-FileHash`） |
| 无 Authenticode 签名 | 属性页显示"无数字签名" | 长期：EV/OV 代码签名证书；首版可无，须声明 |
| 杀软误报 | 网络探测 + 大量 HTTPS 出连易被启发式判为"可疑" | ① 不自修改、不加密载荷、不加壳 ② 不写注册表、不建自启 ③ 提供误报申诉说明 ④ 明确"本工具不修改系统代理设置" |
| **红线** | 不引导用户关闭 Defender / 加排除项作为常规步骤 | 仅在用户明确遇到误报时作为最后手段并说明风险 |

### 9.3 是否需要管理员权限：**不需要**（明确结论）

| 功能 | 是否需要提权 | 说明 |
|---|---|---|
| 监听 `127.0.0.1:0`（>1024 端口） | ❌ | 内核分配端口必 >1024 |
| 读取系统代理（macOS `scutil --proxy` / `SCDynamicStore`） | ❌ | 用户 session 可读 |
| 读取系统代理（Windows WinINET / `HKCU\...\Internet Settings`） | ❌ | `HKCU` 是当前用户 hive |
| 打开默认浏览器 | ❌ | — |
| 发起 HTTPS 出连 | ❌ | 普通用户即可 |
| 流式下载到内存/丢弃 | ❌ | 不写系统目录 |
| 修改系统代理设置 | **本工具不做** | 明确不做，因此无需提权，也消除了"改坏用户网络配置"的风险 |
| 安装到 `/Applications` / `Program Files` | 仅安装包需要，且可由用户拖拽到 `~/Applications` 规避 | 单文件分发形态完全不需要 |

**结论必须写进 UI/文档**：以普通用户身份运行；若出现任何 UAC/`sudo` 提示，说明拿到了被篡改的版本或使用了错误的分发方式 → 应当停止并核对哈希。这本身也是一条可用的完整性探测手段。

---

## 10. 可执行安全验收清单（测试可直接照跑）

> 前置：`PORT`、`TOKEN` 为本次启动时打印的端口与 token。全部条目**必须**在 macOS 与 Windows 各跑一遍，任一条不通过即阻断发布。

| # | 项目 | 操作步骤 | 期望结果 |
|---|---|---|---|
| **S-01** | 端口绑定 | macOS `lsof -nP -iTCP -sTCP:LISTEN \| grep -i proxylens`；Windows `netstat -ano \| findstr LISTENING` 比对 PID | 仅 `127.0.0.1:PORT`；**无** `0.0.0.0:PORT`、无 `*:PORT`、无 `[::]:PORT` |
| **S-02** | 端口非固定 | 连续启动/退出 3 次，记录端口 | 3 次端口各不相同；若目标端口被占，进程拿到另一个内核分配端口（不是"回退到固定端口"），且绝不因占用而放宽绑定地址 |
| **S-03** | Host 头校验 | `curl -i -H "Host: evil.com" http://127.0.0.1:$PORT/api/state` | `403`，响应体不含状态数据，无堆栈 |
| **S-04** | 无 token 拒绝（含零副作用） | `curl -i http://127.0.0.1:$PORT/api/state`；再 `curl -i -X POST -H 'Content-Type: application/json' -d '{"proxy":"socks5://1.1.1.1:1080"}' http://127.0.0.1:$PORT/api/probe` | 均 `401`；同时 `lsof -i -p <pid>` / 抓包确认**没有**新外连（未产生探测副作用） |
| **S-05** | 有 token 放行 | `curl -i -H "X-ProxyLens-Token: $TOKEN" http://127.0.0.1:$PORT/api/state` | `200`，JSON 中 `proxy` 为 masked 形式 |
| **S-06** | bootstrap token 一次性 | 用 `?t=$TOKEN` 打开 UI；60s 后同一 URL 再开一个窗口 | 第一次 `200`；第二次 `403` |
| **S-07** | 零 CORS 头 | `curl -sI -H "Origin: https://evil.com" http://127.0.0.1:$PORT/api/state \| grep -i "access-control"`；再 `curl -i -X OPTIONS -H "Origin: https://evil.com" -H "Access-Control-Request-Method: POST" http://127.0.0.1:$PORT/api/probe` | 第一条**无输出**；第二条 `405` 且无 `access-control` 头 |
| **S-08** | 安全响应头 | `curl -sI http://127.0.0.1:$PORT/ \| grep -Ei "content-security-policy\|x-frame-options\|x-content-type-options\|referrer-policy\|cross-origin-opener\|cache-control"` | 六头齐备；CSP 无 `unsafe-inline`/`unsafe-eval`/`*`；`X-Frame-Options: DENY`；`Cache-Control: no-store` |
| **S-09** | 点击劫持防护 | 本地建 `attack.html`：`<iframe src="http://127.0.0.1:$PORT/?t=$TOKEN" width=800 height=600>`，浏览器打开 | iframe 空白/被拒；控制台出现 `frame-ancestors` 或 `X-Frame-Options` 违规 |
| **S-10** | 其他标签页读写 | 任意外部站点 DevTools 执行 `fetch('http://127.0.0.1:'+PORT+'/api/state').then(r=>r.text()).then(console.log)` | Promise reject（CORS）；服务端记录该请求被 `401` |
| **S-11** | 密码不落盘（含编码变形） | 用密码 `SECRET123` 与**含特殊字符**的密码 `p@ss:w/rd%40` 各跑一次；对数据目录、日志目录、导出目录、`/tmp`（Windows `%TEMP%`）搜索：原文 + **URL 编码变形**（`p%40ss%3Aw%2Frd%2540`、`p%40ss:w/rd%40`）+ base64。macOS：`grep -rl -e 'SECRET123' -e 'p@ss' -e 'p%40ss' ~/Library/Application\ Support/ProxyLens /tmp ~/Documents 2>/dev/null` | 零命中（含所有编码变形）。注意：只搜原文是**不充分**的——密码经 `url.UserPassword()` 序列化后会变成 `%40` 形式，原文搜索会漏 |
| **S-11b** | 数据目录存在但无凭据 | 完成设置改动 + 跑 ≥2 次检测（触发 settings/history 落盘），然后 `ls -la` 该目录并 `cat` 每个文件 | 目录 `0700`、文件 `0600`（macOS 可见 `drwx------`/`-rw-------`）；Windows 位于 `%LOCALAPPDATA%\ProxyLens\`（**不在** `%APPDATA%`）；文件内容符合 §7.5 白名单，无 `password` 字段、无完整 URL、无 IP、无 token |
| **S-11c** | 清空历史是删除而非清空 | 造 3 条历史 → 点"清空历史"（二次确认）→ 检查文件；再删一条历史 | 清空后文件**不存在**（不是变成空数组）；删单条后该条内容在文件中不再出现（含 `strings` 搜索）；卸载/删除目录后无残余 |
| **S-11d** | 日志脱敏与不落 body | 开启诊断日志（若实现 FR-H3），跑一次含 403 挑战页与一次证书异常的探测；`grep -iE "SECRET123|<html|<!DOCTYPE|Bearer|t=" proxylens.log` | 零命中：无密码、无响应体 HTML、无 token；日志只含时间/阶段/错误枚举/脱敏代理；文件 ≤5 MiB 且滚动
| **S-12** | 密码不进日志 | 同上探测，完整捕获 `2>&1 \| tee log`，`grep -i "SECRET123" log`、`grep -i "user:" log` | 零命中；代理出现处一律 `socks5://user:***@host:port` |
| **S-13** | 密码不进 `ps` | 探测运行中另开终端：macOS `ps auxww \| grep -i proxylens`；Windows 管理员 PowerShell `Get-CimInstance Win32_Process \| ? {$_.Name -like "*proxylens*"} \| select CommandLine` | 命令行仅程序路径与无害参数；**无** `user:pass@`、无 token、无密码 |
| **S-14** | 密码/token 不进报告 | 导出 JSON 与 Markdown，`grep -i "SECRET123"`；`grep -i "$TOKEN"` | 零命中；代理字段为 `socks5://user:***@host:port`（三星固定，不反映真实长度） |
| **S-15** | 错误信息脱敏 | 代理填 `socks5://u:MYTOPSECRET@127.0.0.1:99999`（非法端口）提交 | UI 显示可读错误（如"端口无效"）不含 `MYTOPSECRET`；直接 `curl` 该请求返回 `400` 且 body 无密码 |
| **S-16** | SSRF：云元数据（目标侧） | `curl -X POST -H "X-ProxyLens-Token: $TOKEN" -H 'Content-Type: application/json' -d '{"target":"https://169.254.169.254/latest/meta-data/"}' http://127.0.0.1:$PORT/api/probe` | `400 TARGET_BLOCKED`；`lsof -i`/抓包确认未向 `169.254.169.254` 建连 |

| **S-17** | SSRF：内网/本机/域名绕过 | 依次提交 `http://127.0.0.1:22`、`http://10.0.0.1`、`http://[::1]`、`http://localhost`、`http://169.254.169.254.nip.io`、`https://user@evil.com@169.254.169.254/`、`gopher://127.0.0.1:6379/_INFO` | 全部 `400 TARGET_BLOCKED`，均无实际外连。`nip.io` 那条必须被 `Dialer.Control` 的解析后 IP 校验拦住（验证收口生效） |
| **S-18** | 代理地址策略 | 提交代理 `http://169.254.169.254:80` → 应拒绝（`PROXY_BLOCKED`）；再提交 `socks5://127.0.0.1:7890` → 应允许并正常探测 | loopback 代理不被误伤；link-local 代理被拒 |
| **S-19** | TLS 默认严格 | 用 `openssl s_server -accept 443 -cert selfsigned.pem` 或 `mitmproxy` 造自签目标，把探测目标指向它 | 该项 `failed`，`reason=TLS_VERIFY_FAILED`，报告 `tlsVerification=strict`；**不得**自动降级重试成功 |
| **S-20** | 不存在证书降级入口 | ① 遍历设置面板与高级/诊断区，搜索 `忽略`/`跳过`/`insecure`/`跳过校验`；② 尝试 CLI 参数 `--insecure`、`--insecure-skip-verify`、`-k`；③ 尝试环境变量 `PROXYLENS_INSECURE=1`；④ `grep -rn "InsecureSkipVerify" --include=*.go .` | ①②③ 均无此开关/参数/变量（未知参数应报错退出，而非静默忽略）；④ 仅 `tls_diagnose.go` 一处命中，且该 client 不出现在任何探测项调用链上 |
| **S-20b** | 证书证据不污染计分 | 用自签目标跑一次完整检测，检查该项计分与结论 | 该项为 `NET_FAIL`（0 分）；报告中 `tlsVerification=strict`、`tlsEvidenceCollected=true`，且证据里能读到 `chainLen`/`leafIssuer`/自签标记；**不出现**"跳过校验后可用"这类结果 |
| **S-21** | 证书证据采集 | 对 `https://api.openai.com` 跑 TLS 诊断 | 报告含 `chainLen≥2`、`leafIssuer`、`notBefore/notAfter`、`spkiSha256`、`chainRootHitSystemPool=true`、`verifyErr=""` |
| **S-22** | 零外传 | 探测全程 `tcpdump -n -v 'tcp port 443'`（或 Proxyman/Charles）记录全部 SNI | 目标 ⊆ {dl.google.com, speed.cloudflare.com, chatgpt.com, cdn.oaistatic.com, gemini.google.com, api.openai.com, *.googlevideo.com, www.google.com, www.gstatic.com} + 用户指定代理；**无** analytics/字体/CDN/上报域名 |
| **S-23** | 前端无外部资源 | `grep -rEo "https?://[^\"' )]+" <前端产物>` 逐条核对；DevTools Network 观察 | 仅注释/文档链接；运行期无任何第三方请求（CSP `connect-src 'self'` 实际生效） |
| **S-24** | 无 debug/探测端点 | `curl -i http://127.0.0.1:$PORT/debug/pprof/`、`/metrics`、`/healthz`、`/version`（带与不带 token 各一次） | 全部 `404`（不是 `403`，不暴露端点存在性） |
| **S-25** | 无开放代理端点 | `curl -i "http://127.0.0.1:$PORT/proxy?url=http://example.com"` | `404` |
| **S-26** | 无需提权 | 以普通用户启动，完整跑一遍读取系统代理 + 探测 + 导出；Windows 用 `whoami /groups` 确认完整性级别 | 全功能可用；无 UAC、无 `sudo`；未运行在 High/System 完整性级别 |
| **S-27** | 无持久化/无自启 | 完成探测+导出后退出，检查 `~/Library/LaunchAgents`、`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`、`%APPDATA%`、`/tmp` | 无自启项；无本工具创建的目录/文件；无临时文件残留 |
| **S-28** | 崩溃不泄露 | debug build 下触发一个 panic 路径，观察客户端响应与 stderr | 客户端收 `500 {"code":"INTERNAL"}` 无堆栈；stderr 无密码、无绝对用户路径（或已脱敏） |
| **S-29** | 并发与自伤限制 | 快速连点"开始探测"10 次；并行 `curl` 10 个 `/api/probe` | 同时仅 1 个探测在跑，其余 `409`；`/api/state` 的 `running` 正确；无 goroutine 泄漏 |
| **S-30** | 明文代理标注 | 填 `http://127.0.0.1:8080`（明文 HTTP 代理）跑完 | 报告 `plaintextHop=true`；UI 提示"经明文代理，结果可能被篡改" |
| **S-31** | 依赖完整性 | 改动 `go.sum` 任意一行后执行发布构建；再恢复 | 改动后构建**失败**（`-mod=readonly`）；恢复后成功；`govulncheck ./...` 无 Critical/High |
| **S-32** | 可复现构建 | 同 commit + 同 Go 版本构建两次（`-trimpath -buildvcs=false`） | 两次 `sha256` 完全一致 |
| **S-33** | 响应体不落盘/不落日志 | 抓一次 `chatgpt.com` 挑战页探测，结束后搜索数据目录、日志、导出文件中的页面正文特征串（如 `cf-browser-verification`、`<!DOCTYPE`） | 零命中正文；报告中只有 `matchedRuleID` 与 ≤120 字符的命中片段（片段中不得含 `<script`、cookie、邮箱、任何 `token=` 串） |
| **S-34** | 异常 200 不被判为可用（防"假通"） | 用 mitmproxy/local 反代伪造 `https://api.openai.com/v1/models` 返回 `200 {"ok":true}`，并让系统信任该证书 | 该项**不得**判 `PASS`：应判 `NET_FAIL`/可疑并提示"响应异常，可能被中间层拦截"；证据显示 `content-type` 非 JSON/无 OpenAI 错误信封 |
| **S-35** | 环境变量凭据脱敏 | 设 `HTTP_PROXY=http://alice:ENVSECRET@proxy.corp:8080` 后启动，进入结果页与"关于/环境"卡片 | 展示为 `http://alice:***@proxy.corp:8080`；`grep -rl ENVSECRET` 在数据目录零命中；冲突提示正常出现 |

**退出准则**：S-01~S-35（含 S-11b/11c/11d、S-20b）全绿。若存在"已知不通过"，必须写入 `10-acceptance-report.md` 的"已知风险"章节并由 team-lead 签字。

---

## 11. 残余风险登记（必须随发布一起公开）

| ID | 残余风险 | 等级 | 为什么无法消除 | 用户可做的缓解 |
|---|---|---|---|---|
| R-1 | 同用户恶意进程可读进程内存（A1、A3） | Medium | 同机无内存隔离 | 只在可信设备使用；用完关闭进程 |
| R-2 | Go `string` 无法可靠擦除，密码可能在 GC 前残留堆中 | Low | 语言运行时限制 | 缩短进程存活时间 |
| R-3 | 系统根证书库被植入恶意 CA 时所有 TLS 校验失效 | Medium | 依赖 OS 信任根 | 检查系统证书；用 TLS 诊断看 `leafIssuer` 是否合理 |
| R-4 | 浏览器 PNA/第三方 cookie 策略差异导致"网页能否触达 loopback"行为不一致 | Low | 浏览器实现差异 | token + Host + 零 CORS 多重兜底 |
| R-5 | 恶意代理可精确伪造全部指纹，使结果虚高 | Medium | 无法区分"真快"与"假快" | 看 `plaintextHop`、`spkiSha256` 突变、延迟异常低；做"开代理 vs 直连"对比 |
| R-6 | 未签名分发被篡改（A7） | High（若不核哈希） | 首版无代码签名证书 | **核对 sha256**；只从官方 release 下载 |
| R-7 | 企业 TLS 检查环境（自签/私有 CA）下相关探测项直接测不出，用户可能认为工具坏了 | Medium | 有意取舍：不提供校验降级（§6.2） | 按 UI 指引把企业根证书装入系统信任库；或改用非企业网络/手机热点对照 |
| R-11 | 本地历史快照/设置文件被同机他人或取证者读取（含代理 host、出网方式、端点、分数） | Low–Medium | 详见 §7.5：不加密（无口令的本地密钥等于安全剧场），靠 0600 权限 + 字段最小化 + 系统全盘加密兜底 | 开启 FileVault / BitLocker；用"清空历史"；不要把数据目录放进会同步的云盘 |
| R-8 | `socks5h` 远端 DNS 下无法校验目标 IP | Low | 协议特性 | 限制为固定目标集 + 域名白名单 |
| R-9 | 长时测试（60s+）增加目标站风控概率 | Low | 功能需求 | 默认单次 ≤10s；长时需显式勾选并提示 |
| R-10 | 用户把带 `?t=` 的 bootstrap URL 贴到聊天工具 | Low | 用户行为 | token 60s 一次性窗口；进程退出即失效 |

---

## 12. 与 PRD 验收项（L-01~L-05）的映射与补强

PRD 第 10 章 L 组给出 5 条隐私安全验收项。下面把它们映射到本文清单，并指出**按 PRD 现有写法测不出来的漏洞**（建议 `09-test-plan.md` 采纳补强版）。

| PRD 项 | 本文对应 | 补强点（PRD 现有写法会漏的） |
|---|---|---|
| L-01 数据目录全量搜索无明文密码 | **S-11 / S-11b** | ① 只搜**原文**不够：`url.UserPassword()` 会把 `p@ss:w/rd` 序列化成 `p%40ss%3Aw%2Frd`，必须同时搜 URL 编码变形与 base64；② 搜索范围要含**导出目录**与**日志文件**（不只数据目录）；③ 应用**特殊字符密码**测一遍（`@ : / % #` 是解析与脱敏的高危字符） |
| L-02 报告无凭据、无 URL 认证段 | **S-14** | 除密码外还要断言 **token 不出现**、**绝对用户路径不出现**（`C:\Users\<name>`、`/Users/<name>`） |
| L-03 重启后密码字段为空 | **S-11b**（文件层面）+ 人工 UI 走查 | 断言 `settings.json` 里**不存在** password 相关 key（不是"值为空"——留着 key 就等于留了将来被填回去的口子） |
| L-04 强制结束后日志/临时文件无密码残留 | **S-11 / S-11d** | 必须用 `kill -9`（macOS）/ 任务管理器结束进程（Windows）**而不是正常退出**；再检查 `~/Library/Logs/DiagnosticReports`（macOS 崩溃报告）与 `%LOCALAPPDATA%\CrashDumps`。崩溃报告里出现的任何 URL 都必须是脱敏后的 |
| L-05 界面有"不联网上传任何检测数据"声明 | 本文 §7.2 P1/P2、S-22 | 声明文案不能只写"不上传"，还要能自证：建议同屏给出"本次运行的外连目标清单"入口（与 FR-H2 端点清单合并），让用户一眼核对，比一句承诺更可信 |

---

## 附录 A：给实现阶段（engine / arch / ui）的硬性约束

> 以下 12 条为**不可协商项**。若实现中确实无法满足，必须回来修订本文档并写明理由，禁止静默降级。

| # | 约束 | 责任方 |
|---|---|---|
| H1 | 监听必须 `net.Listen("tcp", "127.0.0.1:0")`；禁止固定端口/`0.0.0.0`/`[::]`；绑定失败即非零退出 | engine |
| H2 | 所有 `/api/*` 过单一 `auth` middleware：Host 精确白名单 → token/cookie → Origin → Content-Type，缺一即 401/403 | engine |
| H3 | 零 `Access-Control-*` 响应头；`OPTIONS` 返回 405 且不带 CORS 头；绝不返回 `Access-Control-Allow-Private-Network` | engine |
| H4 | 统一安全响应头 + nonce CSP（无 `unsafe-inline`）；`X-Frame-Options: DENY`；`Cache-Control: no-store` | engine + ui |
| H5 | 状态变更必须 `POST` + JSON + `X-ProxyLens-Token`；禁止 `GET` 副作用；禁止 `/proxy`、`/debug/pprof`、`/metrics`、`/version` | engine |
| H6 | 外连 IP 校验唯一收口于 `Dialer.Control`，用 `netip.Prefix`；`169.254/16`、`fe80::/10`、`100.64/10`、组播/保留段**任何场景**拒绝 | engine |
| H7 | 探测目标仅 `https` + 公网 FQDN + `:443`（`:80` 仅 204 基线）；禁 IP 字面量、禁 `user:pass@`；重定向 ≤3 跳且每跳复检 | engine |
| H8 | 密码只存 `[]byte`；不落盘/不进日志/不进报告/不进 `os.Args`/不进 env；仅 `MaskProxy` + `Sanitize` 两个脱敏函数，**所有** error 输出前必过 `Sanitize` | engine |
| H9 | **不提供任何证书校验降级入口**（无开关/无 CLI 参数/无环境变量）；`InsecureSkipVerify` 只允许出现在 `tls_diagnose.go` 的证据采集函数内且不参与计分；报告 `tlsVerification` 只有 `"strict"` | engine + ui |
| H10 | 报告用**白名单结构体**序列化（禁止 dump 配置）；必须含 §4.7 的 `security` 段 | engine |
| H11 | 前端零外部资源（无 CDN/字体/analytics）；`go:embed` 内嵌全部静态资源；运行期无第三方请求 | ui + engine |
| H12 | 不请求管理员权限；不修改系统代理设置；不写注册表/LaunchAgents；不创建后台常驻。持久化只允许 §7.5 三类文件（`settings.json` / `history.jsonl` / 可选日志），位置与权限按 §7.5 表执行（Windows 用 `%LOCALAPPDATA%`，禁用 `%APPDATA%`） | engine |

---

## 附录 B：与其他文档的接口

| 文档 | 需要吸收的点 |
|---|---|
| `03-architecture.md` | H1/H2/H3/H5 决定 HTTP 层结构（middleware 链顺序）；`auth` 必须是第一个 middleware |
| `06-engine-design.md` | `Dialer.Control` 收口（H6）、目标校验（H7）、降级必须暴露（G4）、请求数/字节数上限（T-11/T-12）、响应体读取上限 |
| `07-ux-interaction.md` | **不存在任何证书降级入口**（不要设计"跳过校验"开关，见 §6.2）；证书异常的文案必须指向"装企业根证书"而非"关校验"；密码掩码组件的临时显示行为；错误文案不泄露内部细节；"检测到目标被拦截"提示 |
| `08-ui-spec.md` | 密码输入框规范（`type=password`、`autocomplete=off`、不回显长度）、masked 显示组件 |
| `09-test-plan.md` | 直接采纳 §10 的 S-01~S-35（含 S-11b/11c/11d、S-20b）作为安全测试集；与 PRD 第 10 章 L-01~L-05 的对应关系见 §12；每次 PR 的冒烟子集建议 S-01/03/04/07/08/11/11b/14/16 |

---

**文档结束。** 任何与本文件冲突的实现决策，以本文件为准；若要偏离，需在 `03-architecture.md` 的 ADR 中显式记录并说明补偿措施。

---

## 附录 C：变更记录

| 版本 | 日期 | 变更 | 作者 |
|---|---|---|---|
| v1.0 | 2026-10-22 | 首版：资产/边界、STRIDE 18 条、本地服务加固、SSRF 策略、凭据 12 条、TLS/MITM 判定、隐私、供应链、平台、S-01~S-32 | Security |
| v1.1 | 2026-10-22 | 与 PRD 对齐：① §6.2 由"提供诊断开关"改为**完全不提供证书校验降级入口**（T-17 由"控制"改为"设计消除"，H9 同步收紧，S-20 重写为"不存在该入口"的搜证式验收）；② 新增 §7.5 本地持久化基线（settings/history/log + `%LOCALAPPDATA%` 而非 `%APPDATA%` + 不做应用层加密的结论）；③ §4.2 D1 标注 PRD 无自定义目标功能；④ 新增 S-11b/11c/11d、S-20b、S-33/S-34/S-35；⑤ 新增 §12 与 PRD L-01~L-05 的映射及补强点 | Security |
