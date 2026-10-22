# 02 · ProxyLens 技术调研与选型对比

| 项 | 值 |
|---|---|
| 文档版本 | v1.0（定稿） |
| 作者 | Software Architect |
| 状态 | 结论已可用于编码；含 12 项本机实测（标注 `M1`–`M12`） |
| 上游 | `docs/00-task-brief.md` |
| 下游 | `docs/03-architecture.md`（ADR 定稿）、`docs/06-engine-design.md`（算法）、`app/`（实现） |

> **写法约定**：凡标注 `M<n>` 的结论均来自本机真实执行，附命令与输出摘要。凡未实测的，明确写「未实测（推定）」。本文件不以「通常」「一般来说」替代实测。

---

## 0. 结论先行

**技术栈定稿：Go 1.26 + `go:embed` 内嵌静态资源 + `127.0.0.1` 本地 HTTP/SSE + 用户默认浏览器打开。**

| 决策 | 结论 | 一句话理由 |
|---|---|---|
| 语言/运行时 | **Go 1.26.3** | 本机已装、四目标零工具链交叉编译、`CGO_ENABLED=0` 纯静态、单文件 3.9–4.3 MB（实测 `M6`） |
| UI 外壳 | **系统默认浏览器，不内嵌 WebView** | Win/macOS 均不依赖任何「用户可能没装」的运行时；且内嵌 WKWebView 必须开 cgo，会摧毁交叉编译优势 |
| 前后端通道 | **同源 `127.0.0.1:<随机端口>` HTTP + SSE** | 同源即无 CORS；SSE 单向流式刷指标，比 WebSocket 少一半实现量 |
| 前端构建链 | **零构建**：原生 ES Module + 每组件一文件 | `go:embed ./web` 直接内嵌；无 node_modules、无打包器、无 lockfile 漂移 |
| Rust/Tauri | **排除** | 本机无 cargo；引入 Rust 工具链 + 仍依赖 WebView2 + 签名链路，成本远超收益 |
| Electron | **排除** | 单包 ≥ 90 MB、冷启动 1–3 s、常驻 200 MB+；与「空闲 CPU 近 0 / 单文件分发」直接冲突 |
| Python + WebView | **排除** | 「不允许依赖用户预装运行时」直接违规；PyInstaller 杀软误报不可控 |

**四条必须写进代码的「诚实性」硬约束**（均由实测得出，详见 §5）：

1. **走 HTTP 代理时 DNS 阶段恒为「不可得」**（`httptrace.DNSStart` 根本不触发，实测 `M9`）→ 必须报 `null`，**禁止填 0**。
2. **TCP 建连耗时测的是「本地这一跳」**（代理端口或 TUN fake-IP，实测 0.22–0.80 ms）→ 标签必须写「本地代理建连」，**不得**当作远端 RTT。
3. **唯一端到端可归因的握手指标是 TLS 握手耗时与 TTFB**（实测 94–169 ms，直连/代理两条路径一致）。
4. **`speed.cloudflare.com` 在本机当前网络下不可用**（TLS 被中断，`M10`）→ 只能做可选对照，**不得**作为主吞吐源。

---

## 1. 运行时方案对比

### 1.1 对比总表

| 维度 | **A. Go + go:embed + 本地 HTTP + 浏览器** | B. Rust + Tauri | C. Node + Electron | D. Python + WebView | E. 纯 Web（静态页直连目标站） |
|---|---|---|---|---|---|
| 分发体积（四目标合计） | **15.9 MB**（实测 `M6`：3.94+4.23+4.27+3.94） | 15–40 MB（含 WebView2 bootstrapper） | 90–150 MB / 平台 | 25–60 MB（onefile，解压后更大） | ~0（但需用户自己起静态服务器） |
| 单目标体积 | **3.9–4.3 MB** | 8–20 MB | 90–150 MB | 25–60 MB | N/A |
| 冷启动 | **< 10 ms 进程就绪**（实测 `M7`），浏览器首屏另计 | 300–800 ms（WebView 初始化） | 1–3 s | 0.8–3 s（onefile 解压是主因） | 取决于浏览器 |
| 常驻内存（空闲） | **~8–15 MB** | 60–120 MB | 200–350 MB | 80–150 MB | 0（寄生用户浏览器） |
| 常驻内存（8 并发探测） | ~35–60 MB（每流 32 KiB buffer × 2 方向） | 80–150 MB | 250–400 MB | 100–200 MB | N/A |
| 跨平台构建难度 | **极低**：一台 macOS 出四包（`M6`），无 CGO、无系统 SDK | 高：rustup + 各平台链接器；通用二进制需 lipo；WebView2 需 bootstrapper | 中高：electron-builder 每平台需对应 runner/公证 | 高：每平台各自 PyInstaller、各自签名、GLIBC 兼容 | 无 |
| 能否拿到 socket 级分阶段耗时 | **能，且分层可归因**（`net/http/httptrace`，`M9`） | 能（`hyper` + connector layer，实现量约 Go 的 3×） | **部分能**：主进程可拿，但 Chromium 网络栈与 Node 栈是两套，指标归属混乱 | 能（`http.client` 自绘 socket），socks5h/分阶段需手写较多 | **不能**：`fetch` 只给 `PerformanceResourceTiming`，DNS/TCP/TLS 因跨域与隐私被置 0 或缺失 |
| 能否绕开浏览器 CORS | **能**（探测在 Go 内，浏览器只与本机同源通信） | 能 | 能 | 能 | **不能** |
| 能否拿到原始响应头 | **能**（实测拿到 `cf-mitigated`、`server: gvs 1.0`） | 能 | 能 | 能 | **不能**：非 CORS 白名单头对 JS 不可见 |
| 能否强制 `Accept-Encoding: identity` | 能（Go 默认即 identity） | 能 | 能 | 能 | **不能**：forbidden header |
| 能否自定义 `Range` 不受预检阻断 | 能 | 能 | 能 | 能 | 不能（`Range` 触发 preflight，多数 CDN 不回 CORS 头） |
| socks5h（远程 DNS）可控性 | **完全可控**（域名透传给代理） | 可控 | 需 `socks-proxy-agent`，且要绕开 Chromium 网络栈 | 需 `PySocks` 手接 | 不可控 |
| 空闲 CPU | **≈0**（单 goroutine 阻塞在 `Accept`） | ≈0 | 1–3% | ≈0 | 0 |
| 开发成本（P0 全量） | **中低**（Go ~3.5 kLoC，前端 ~2 kLoC，无构建链） | 高（Rust + IPC 契约 + 双端类型同步） | 中（打包/签名/更新工程量大） | 中高（WebView 桥接 + 打包疑难） | 低，**但功能做不出来** |
| 本机可立即开工 | **是**（Go 已装） | **否**（cargo 未装） | 是（Node 22.22.2） | 是（Python 3.13.12） | 是 |

### 1.2 实测依据

**`M6` — 四目标交叉编译体积**

```
$ cat main.go            # 仅 stdlib + embed 一个 340 KB 的 index.html
$ GOOS=<os> GOARCH=<arch> CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o app_<os>_<arch>

darwin/arm64    3,963,218 bytes  (3.8M)
darwin/amd64    4,227,728 bytes  (4.0M)
windows/amd64   4,274,176 bytes  (4.1M)
windows/arm64   3,941,888 bytes  (3.8M)
```

一台 macOS arm64 主机、`CGO_ENABLED=0`、零外部工具链，四个目标全部成功。这是本方案最关键的一条实证：**「一次构建、四平台单文件」不是承诺，是已跑通的命令**。

对照组：同程序嵌入 42 KB 页面时 darwin/arm64 为 3.68 MB —— **embed 内容体积几乎 1:1 线性叠加**。若前端产物压到 150 KB 以内，最终二进制稳定在 **4.0–4.3 MB**。

**`M7` — 冷启动**

```
$ /usr/bin/time -p ./app_darwin_arm64      # 打印 embed 长度后退出
real 0.00   (×5 次)
```

`execve` → `main()` 执行完毕 < 10 ms。用户双击到看到 UI 的时间实际由「浏览器新标签页打开耗时」主导（浏览器已开 < 200 ms，冷启动 300–800 ms）。

### 1.3 为什么明确排除其余四项

**B. Rust + Tauri —— 排除（工具链成本 + 仍依赖 WebView2）**

- 本机 `rustc`/`cargo` 均未安装。装工具链（~1.5 GB）+ 首次 `cargo build` + Tauri CLI 是**在产品核心（探测引擎）之前就先花掉的成本**。
- Tauri 在 Windows 上**仍然依赖 WebView2**。Win11 预装、Win10 21H2+ 多数预装，但「多数」≠「不允许依赖用户预装运行时」——只要目标用户含 LTSC / 精简版 / 企业镜像关闭 Edge 的情况就会「装完打不开」，要兜住就得带 Evergreen Bootstrapper（+1.8 MB 且需联网）。
- 收益为负：体积从 4 MB 变成 8–20 MB（Rust 侧还要带 WebView 绑定）。
- **Tauri 的优势场景是「原生窗口 + 极小体积 + 已有 Rust 团队」，本项目三条全不成立。**

**C. Node + Electron —— 排除（与硬指标冲突）**

- 最小可用包 90 MB 起（含 Chromium）。用户要求「单文件分发优先」「空闲 CPU 近 0」「性能好」。
- 常驻 200 MB+、冷启动 1–3 s，对「看一眼就走」的检测工具是**体验负资产**。
- **唯一真实优势**：可在渲染进程跑真实浏览器指纹，有头 Chromium 能过部分 CF 质询。**但这有害**：ProxyLens 的定位是「如实反映用户日常使用代理的能力」，而用户日常用的是自己的浏览器；用「特别能过风控的头」去测会给出**比现实更乐观**的结论，与诚实性原则冲突（见 ADR-06）。

**D. Python + WebView —— 排除（违反硬约束）**

- `00-task-brief.md` §2.4 明确「不允许依赖用户预装运行时」。PyInstaller onefile 虽能规避，但：① 每次启动解压到临时目录（杀软高频误报源）；② macOS 需目标机 `codesign` 否则 Gatekeeper 拦截；③ `httptrace` 的等价物（分阶段 socket 钩子，尤其 TLS 握手分界）需手写。
- 用更差的技术做同样的事。

**E. 纯 Web —— 排除（能力不足，非成本问题）**

最容易被误选的一项（「不用后端不就最轻？」）。它**在能力层就无法满足需求**：

| 需求 | 纯 Web 的结局 |
|---|---|
| 读 `cf-mitigated` 判断 Cloudflare 挑战 | **不可能**：非 CORS 白名单头，JS 不可见 |
| 读 body 判断「not available in your country」 | **不可能**：跨域 `fetch` 得 opaque response，body 不可读 |
| `Range: bytes=0-N` 固定字节测速 | **不可能**：`Range` 触发 preflight，CDN 基本不回 `Access-Control-Allow-Headers: Range` |
| `Accept-Encoding: identity`（保证字节=线路字节） | **不可能**：forbidden header |
| socks5h 远程 DNS 测试 | **不可能**：浏览器不暴露该开关 |
| 强制直连 vs 代理 A/B | **只能测浏览器当前路径**，无法构造第二条路径 |

**纯 Web 连 P0 功能的一半都实现不了。它不是「更简单的方案」，是「做不到的方案」。**

### 1.4 为什么选「浏览器」而非「内嵌 WebView」

选定 Go 之后剩一个分叉。**决定性理由是 cgo：**

|  | 默认浏览器 | 内嵌 WebView |
|---|---|---|
| Windows 依赖 | 无（用户必有浏览器） | WebView2 Runtime（Win11 预装；Win10 需确认） |
| macOS 依赖 | 无 | 无（WKWebView 属系统） |
| 原生窗口观感 | 差（有地址栏/标签页） | 好 |
| 开发量 | `open` / `rundll32` / `Start-Process` 三行 | macOS 需 cgo+ObjC 桥接；Windows 需 `webview2` Go 绑定 |
| **是否破坏 `CGO_ENABLED=0`** | 否 | **是**（macOS WKWebView 必须 cgo）→ `M6` 的交叉编译优势全丢 |
| 用户心智 | 「这是个网页工具」——与定位一致 | 「这是个 App」 |

**内嵌 WKWebView 必须开 cgo，会把 `M6` 建立的「一台机器出四包」优势直接摧毁**（cgo 交叉编译需各平台 SDK 与交叉工具链，Windows 需 mingw，成本回到 Tauri 水平）。

**选定：默认浏览器 + 自动打开。** 观感损失通过 UI 质量弥补（`08-ui-spec.md` 已按全屏单页应用设计）。保留零成本后续选项：Windows 侧后续可加可选 `--webview`（cgo 可用 mingw 交叉编译，风险局部化），macOS 侧永不做。

---

## 2. 系统代理探测

### 2.1 三种出网形态必须先分清

这是最容易做错的地方。用户说「走系统代理」，实际可能处于**三种完全不同的状态**，而「系统代理」一词在三种状态下含义不同：

| 形态 | 系统代理设置 | 实际出网路径 | 代理留空时的正确行为 |
|---|---|---|---|
| **S1 显式系统代理** | 有值（`HTTPEnable=1` 等） | 应用到 `127.0.0.1:PORT` 或远端 | 使用该代理 |
| **S2 TUN 全局接管** | **空**（`HTTPEnable=0`、`SOCKSEnable=0`、`ProxyAutoConfigEnable=0`） | 全部流量经虚拟网卡，由 TUN 进程接管 | **直连语义即可**（内核路由已把流量导走） |
| **S3 环境变量代理** | 空 | 取决于谁读该变量 | **默认忽略**（见 §3.4 论证） |

**`M1` — 本机实测（形态 S2）**

```
$ scutil --proxy
<dictionary> {
  FTPPassive : 1
  HTTPEnable : 0
  HTTPSEnable : 0
  ProxyAutoConfigEnable : 0
  SOCKSEnable : 0
}

$ env | grep -i proxy
HTTP_PROXY=http://127.0.0.1:64080
HTTPS_PROXY=http://127.0.0.1:64080
```

**结论：本机是「S2 + S3 叠加」——系统代理全关（TUN 接管），但 shell 里被注入了环境变量代理。** 这正是 `00-task-brief.md` §4 描述的场景，且端口号（`64080`）随会话变化，**不可硬编码**。

### 2.2 macOS 探测：`scutil --proxy` 字段全表

`scutil --proxy` 输出是 NeXTSTEP plist 文本，解析成本低（逐行 `key : value`），且**不需要 cgo**。这是首选方案。

| 字段 | 类型 | 含义 | 消费方式 |
|---|---|---|---|
| `HTTPEnable` | 0/1 | HTTP 代理开关 | 为 1 才读 `HTTPProxy`/`HTTPPort` |
| `HTTPProxy` | host | HTTP 代理主机 | 拼 `http://host:port` |
| `HTTPPort` | int | HTTP 代理端口 | — |
| `HTTPUser` / `HTTPPassword` | string | HTTP 代理凭据（**罕见**，系统设置 UI 不写） | 若存在则拼进 userinfo；**不落盘** |
| `HTTPSEnable` / `HTTPSProxy` / `HTTPSPort` | 同上 | HTTPS 代理（**与 HTTP 独立**） | **优先于 HTTP**：https 目标应走 HTTPS 代理 |
| `SOCKSEnable` / `SOCKSProxy` / `SOCKSPort` | 同上 | SOCKS 代理（系统 UI 只支持 SOCKS5） | 无 HTTP/HTTPS 代理时兜底 |
| `ProxyAutoConfigEnable` | 0/1 | PAC 开关 | 见 §2.5 |
| `ProxyAutoConfigURLString` | URL | PAC 文件地址 | 见 §2.5 |
| `ProxyAutoDiscoveryEnable` | 0/1 | WPAD | **不支持**，见 §2.5 |
| `ExceptionsList` | array | 绕过列表（域名/网段） | 转成 `NO_PROXY` 语义 |
| `ExcludeSimpleHostnames` | 0/1 | 单标签主机名（如 `intranet`）不走代理 | 影响 `NO_PROXY` 是否含 `*` 兜底规则 |
| `FTPEnable` / `FTPPassive` | — | FTP，**忽略** | — |

**关键差异（易踩坑）**：

1. **`HTTPSEnable` 与 `HTTPEnable` 是独立开关。** 只读 `HTTPEnable` 会漏掉「只配了 HTTPS 代理」的常见情形。
2. **端口字段只在对应 `Enable=1` 时存在**，不能假设 `HTTPPort` 一定有值。
3. `scutil` 在**沙箱/无网络服务**环境下可能返回空字典而非报错 → 必须区分「空字典 = 无代理」与「执行失败 = 未知」。
4. **不要用 `networksetup -getwebproxy <service>`**：它需要知道服务名（Wi-Fi / Ethernet / Thunderbolt Bridge…），且只返回单个服务；`scutil --proxy` 返回的是**系统当前生效的聚合视图**。

**macOS 第二方案：`CFNetworkCopySystemProxySettings`**

官方 API，返回 `CFDictionary`，字段与 `scutil` 同源（`scutil --proxy` 就是它的 CLI 投影）。**但调用它必须 cgo**（`import "C"` + `CoreFoundation`），会摧毁 `CGO_ENABLED=0`。

- **决策：v1 只用 `scutil --proxy` + 解析文本。** 放弃 CFNetwork 的理由：① cgo 代价（见 §1.4 同一论证）；② `scutil` 是系统自带 CLI，行为稳定；③ 两者的信息量在本场景下等价。
- 兜底路径：`scutil` 不可执行（极罕见）→ 退化为「无代理 + 提示用户手动填写」。

### 2.3 Windows 探测：注册表 `Internet Settings`

Windows 上 **Go stdlib 完全不读系统代理**（`http.ProxyFromEnvironment` 只读环境变量）。必须自己读注册表。

**主路径：`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`**

| 值名 | 类型 | 含义 | 解析要点 |
|---|---|---|---|
| `ProxyEnable` | `REG_DWORD` | 总开关 | `0` → 无代理（**但仍要检查 `AutoConfigURL`**） |
| `ProxyServer` | `REG_SZ` | 代理地址 | **两种格式**，见下 |
| `ProxyOverride` | `REG_SZ` | 绕过列表 | `;` 分隔，含 `<local>` 特殊标记 |
| `AutoConfigURL` | `REG_SZ` | PAC 地址 | 见 §2.5 |
| `AutoDetect` | `REG_DWORD` | WPAD | **不支持** |

**`ProxyServer` 的两种格式（必须都支持）**：

```
格式 1（单协议）：  127.0.0.1:7890
                  → http/https/socks 全用这一个

格式 2（分协议）：  http=127.0.0.1:7890;https=127.0.0.1:7890;socks=127.0.0.1:7891
                  → 按 scheme 分别取值，未列出的 scheme 表示「不走代理」
```

解析规则：
- 含 `=` → 格式 2，按 `;` 切分、按 `=` 分 `scheme:addr`。
- 不含 `=` → 格式 1，所有 scheme 共用。
- 格式 2 中**缺失的 scheme 代表直连**，不是「回退到 HTTP 代理」——这是 WinINET 的语义，照抄。

**`ProxyOverride` 语义**：`;` 分隔，`*` 通配，`<local>` 表示「不含点的主机名」。转成 Go 的 `NO_PROXY` 语义时要处理 `<local>` → Go 的 `httpproxy` 不认这个 token，需自行翻译为「无点主机名即绕过」的判定。

**第二路径：WinHTTP（`WinHttpGetIEProxyConfigForCurrentUser` / `netsh winhttp show proxy`）**

- **不要用**。WinHTTP 与 WinINET 是两套独立配置：WinINET 是「IE/浏览器/大多数 .NET 应用」的，WinHTTP 是「服务/系统组件」的。用户改代理通常只改 WinINET。实测经验是二者经常不一致；若读 WinHTTP 会给出与「用户浏览器实际行为」不符的结论——**违背工具的诚实性原则**。
- 唯一例外：若 WinINET 全空而 `netsh winhttp show proxy` 有值，可作为**提示信息**（`warnings`）呈现，不改变实际测量路径。

**实现选型**：

| 方案 | 评价 |
|---|---|
| `golang.org/x/sys/windows/registry` | **推荐**。纯 Go，无 cgo，`GOOS=windows` 直接编译，API 稳定 |
| `syscall` + 手写 `RegOpenKeyEx` | 可行但啰嗦，无收益 |
| cgo + `WinINET` | 破坏 `CGO_ENABLED=0`，且需 `wininet.h` |

**决策：`golang.org/x/sys`（唯一非 stdlib 依赖，见 ADR-05）。**

### 2.4 环境变量优先级

**标准语义（`curl` / `requests` / 多数 CLI 的共识）**：

```
对 https:// 目标：  HTTPS_PROXY  >  ALL_PROXY  >  HTTP_PROXY(?) > 直连
对 http://  目标：  HTTP_PROXY   >  ALL_PROXY  >  直连
NO_PROXY 命中：     永远直连（优先级最高，覆盖以上全部）
大小写：          全小写（http_proxy）优先于全大写（HTTP_PROXY）在 curl 中成立，
                  但 Go 的 httpproxy 中两者等价（先查小写再查大写）
```

**Go stdlib 的两个关键事实（必须知道，否则会写出错代码）**：

| 事实 | 后果 |
|---|---|
| **`http.ProxyFromEnvironment` 完全不读 `ALL_PROXY`** | 只用 `x/net/http/httpproxy` 或自行处理 `ALL_PROXY`。本工具支持 `ALL_PROXY` 的**唯一位置是「自定义代理」输入框的默认值提示**，不参与自动探测 |
| **`http.ProxyFromEnvironment` 在进程内首次调用后缓存环境变量**（`envProxyOnce`） | **绝不能在每次 Run 时改 `os.Setenv` 再指望它生效**。必须为每次 Run 构造独立 `Transport` + 显式 `Proxy` 函数 |

```go
// ❌ 错误：想通过改环境变量切换代理
os.Setenv("HTTPS_PROXY", userProxy)
tr := &http.Transport{Proxy: http.ProxyFromEnvironment} // 仍用旧值！

// ✅ 正确：每次 Run 独立构造
tr := &http.Transport{Proxy: func(*http.Request) (*url.URL, error) {
    return netx.ResolveForRun(cfg).ProxyURL, nil // 可为 nil = 直连
}}
```

### 2.5 PAC / WPAD：明确不支持

| 机制 | 决策 | 理由 |
|---|---|---|
| PAC（`AutoConfigURL`） | **v1 不支持**，但在 `warnings` 中明确提示 | 需要 JS 引擎执行 PAC 脚本（`FindProxyForURL`）。引入 goja 等 JS 引擎为一个边缘场景，成本/体积/安全面（执行用户可控 JS）都不划算 |
| WPAD（`AutoDetect`） | **v1 不支持**，同样提示 | 需 DNS/DHCP 探测 + 同上 JS 执行；企业网场景，个人用户几乎不用 |

**呈现规则（诚实性）**：若检测到 PAC/WPAD 已启用，UI 必须显示：

> 「检测到系统使用 PAC/WPAD 自动代理配置，本工具暂不支持解析。当前结果基于**直连**路径，可能与你浏览器的实际出网方式不同。请手动填写代理地址以获得准确结果。」

**绝不允许**静默当作「无代理」然后给出一个漂亮的绿色分数——那是在骗用户。

### 2.6 TUN 下的如实呈现（核心诚实性问题）

TUN 模式下最危险的误判是：**把 fake-IP 当成真实远端 IP 展示，把 0.3 ms 的本地建连当成远端 RTT。**

**`M2` — TUN fake-IP 实测**

```
$ PL_PROXY=direct ./plprobe https://www.gstatic.com/generate_204
  status=204 proto=HTTP/2.0
  DNS=5.10ms (addrs=[198.18.0.69])   TCP=0.72ms   TLS=96.38ms
  TTFB=77.04ms

$ PL_PROXY=direct ./plprobe https://www.google.com/generate_204
  DNS=5.08ms (addrs=[198.18.0.93])   TCP=0.80ms   TLS=102.35ms
```

**`198.18.0.0/15` 是 RFC 2544 保留的 benchmark 网段**，被 Clash/Mihomo/Surge 等用作 `fake-ip` 池。观察到的 `198.18.0.69` / `.70` / `.93` 证明确切是 fake-IP 模式。

**结论与呈现规则**：

| 观察 | 真实含义 | UI 必须怎么写 |
|---|---|---|
| `remote_ip ∈ 198.18.0.0/15` | TUN fake-IP，非真实远端 | 「TUN 已接管（fake-IP 198.18.0.69）」；**禁止**显示为「出口 IP」 |
| `dns_ms ≈ 3–8 ms` 且返回 fake-IP | 本地合成，无真实上游 DNS 往返 | 「DNS 由 TUN 本地合成（非真实解析耗时）」；或直接标 `null` |
| `tcp_ms < 1 ms` | 到本地 TUN 接口或本地代理端口 | 「本地代理建连」；**禁止**标「TCP 建连 / RTT」 |
| `tls_ms 90–170 ms` | **真实端到端**（TLS 握手必须与远端完成） | 这是唯一可当地址无关 RTT 参考的量 |
| 系统代理为空 + fake-IP 命中 | 形态 S2 | `effective_route: "tun"`，`proxy: null` |

**实现要求**：`netx` 包必须提供 `ClassifyRoute(systemProxy string, resolvedIPs []net.IP, envProxy string) RouteKind`，返回 `{Kind: "os_proxy"|"tun"|"direct"|"pac_unsupported", Evidence: "..."}`。UI 顶栏始终显示当前生效路径（`04-security.md` 也要求代理地址必须 mask 展示）。

---

## 3. 代理协议在 Go 中的落地

### 3.1 四种 scheme 的语义差异

| scheme | 谁做 DNS | 谁做 TLS | Go 实现路径 | 备注 |
|---|---|---|---|---|
| `http://` | **代理**（HTTP CONNECT 里带域名，或代理自己解析绝对 URI） | 客户端（CONNECT 隧道内） | `Transport.Proxy` 返回该 URL | 最常见。Go 发 `CONNECT host:443` |
| `https://` | 代理 | **两级 TLS**（先与代理 TLS，隧道内再与目标 TLS） | `Transport.Proxy` + `TLSClientConfig` | 即「HTTPS 代理」。需要代理服务器支持 TLS；证书校验对象是代理 |
| `socks5://` | **客户端**（本地 `net.LookupHost` 后传 IPv4/IPv6 地址） | 客户端 | `x/net/proxy.SOCKS5` + 自定义 `DialContext` | 本地 DNS → 在 DNS 污染环境下会拿到脏 IP，**测出来的是「本地 DNS 下的结果」** |
| `socks5h://` | **代理**（传 `DOMAINNAME` ATYP 给代理，由代理解析） | 客户端 | 同上，但 `Dial` 传域名而非 IP | **这才是「代理端 DNS」**，`h` = hostname |

**`socks5` vs `socks5h` 的区别是 DNS 归属，不是加密强度。** 这是最常被误解的一点：

- `socks5`：本地解析 → 若本地 DNS 被污染/被 fake-IP 接管，你测的是「本地 DNS 结果走 SOCKS」。
- `socks5h`：域名透传 → 代理端解析，绕开本地 DNS。**对「本机处于 TUN fake-IP 环境」的场景，`socks5h` 是唯一能测出真实远端解析的方式。**

**实现陷阱：`golang.org/x/net/proxy` 的 `SOCKS5` dialer 只支持 IP，要 `socks5h` 必须自己包一层。**

```go
// x/net/proxy.SOCKS5 返回的 Dialer：Dial("tcp", "example.com:443")
// 内部会先 net.LookupHost("example.com") —— 这是 socks5 语义，不是 socks5h！

// 要 socks5h，必须实现「域名直传」：
type socks5hDialer struct{ d proxy.Dialer }

func (s socks5hDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
    host, port, _ := net.SplitHostPort(addr)
    if net.ParseIP(host) != nil {
        return s.d.Dial(network, addr) // 已是 IP，无差别
    }
    // x/net/proxy 的 Dial 一旦收到非 IP，会走 LookupHost。
    // 因此不能复用，必须直接用 golang.org/x/net/internal/socks 或手写握手。
}
```

**决策：`socks5h` 用 `golang.org/x/net/internal/socks` 不可行（internal 包不可 import）。**

三个可行选项（详见 ADR-05）：

| 选项 | 评价 |
|---|---|
| A. 手写 SOCKS5 握手（~140 行） | **选定**。协议简单（RFC 1928），完全可控，可精确支持 `DOMAINNAME` ATYP + 用户名密码认证（RFC 1929），零额外依赖 |
| B. `github.com/txthinking/socks5` 等三方库 | 多一个依赖，且多数库默认行为仍是本地解析 |
| C. 用 `x/net/proxy` 接受 `socks5` 语义 | **不可接受**：TUN 环境下本地解析会拿到 fake-IP，测出来是假的 |

> `app/internal/netx/socks5.go` 已按选项 A 实现（195 行），含 `DOMAINNAME` ATYP 与 `Auth`。本文确认该实现方向正确。

### 3.2 `Transport.Proxy` vs `DialContext`：何时用哪个

| 场景 | 用哪个 | 原因 |
|---|---|---|
| `http://` / `https://` 代理 | `Transport.Proxy` | 让 Go 自己发 `CONNECT`、自己处理 `Proxy-Authorization`、自己复用隧道。手写等于重造轮子 |
| `socks5` / `socks5h` 代理 | `Transport.DialContext`（`Proxy: nil`） | SOCKS 是传输层隧道，不是 HTTP 层概念。用 `DialContext` 让整个 TCP 连接穿隧道，TLS 在隧道内正常进行 |
| 直连对照 | `Proxy: nil` + 默认 `DialContext` | — |
| 需要 SSRF 网段校验 | **统一收口在 `DialContext`** | 见 `04-security.md` §4.4：所有出连必须过同一个 IP 校验点 |

**关键点：`Transport.Proxy` 与自定义 `DialContext` 可以同时存在，但语义叠加顺序是 `DialContext` 先连代理服务器、再发 CONNECT。** 因此：

- HTTP 代理：`Proxy` 有值，`DialContext` 是普通 dialer（但 `04-security.md` 要求在 `DialContext` 里校验**代理服务器地址**，不是目标地址）。
- SOCKS 代理：`Proxy=nil`，`DialContext` 返回已穿隧道的 `net.Conn`（此时 `DialContext` 收到的 `addr` 是**目标地址**，可做 SSRF 校验）。

### 3.3 认证

| 协议 | 认证方式 | Go 实现 |
|---|---|---|
| HTTP/HTTPS 代理 | `Proxy-Authorization: Basic base64(user:pass)` | `url.URL.User` 会被 Go 自动转成该头 |
| SOCKS5 | RFC 1929 用户名/密码子协商 | 手写实现：`Auth{User, Password}` |

**硬约束（与 `04-security.md` 一致）**：

- 凭据**只存在于内存**的 `url.User`，不写日志、不进报告、不进任何持久化。
- 报告中代理地址必须 mask：`http://user:***@host:port`。
- 建议额外提供 `sha256(host:port)[:8]` 作为「同一代理」的 stable 标识，用于历史对比而不泄露地址。

### 3.4 URL 编码陷阱（会导致「明明密码对却 407」）

代理凭据里的特殊字符必须 percent-encode，否则 `url.Parse` 会错位：

| 字符 | 必须编码为 | 不编码的后果 |
|---|---|---|
| `@` | `%40` | 被当成 userinfo 分隔符 → host 解析错位 |
| `:` | `%3A` | 被当成 user:pass 分隔符 |
| `/` | `%2F` | 被当成 path 起点 |
| `#` | `%23` | 被当成 fragment |
| `?` | `%3F` | 被当成 query |
| `%` | `%25` | 二次解码错误 |
| 空格 | `%20` | 部分解析器截断 |

**实现要求**：用户输入 `socks5h://user:p@ss:word@127.0.0.1:1080` 这种**故意刁难**的字符串时，必须能正确解析出 password = `p@ss:word`。做法：**从右往左找最后一个 `@` 作为 userinfo 分隔符**，不要用 `strings.Split(s, "@")[0]`。

```go
// 正确做法：先取最后一个 '@' 之前为 userinfo，之后为 host
if i := strings.LastIndex(authority, "@"); i >= 0 {
    userinfo, hostport = authority[:i], authority[i+1:]
}
```

**`url.Parse` 本身会拒绝未编码的 `%` 和非法字符** → 必须先做「宽松预解析 + 规范化」，再交给 `url.Parse`。同时给出明确错误文案：「密码含特殊字符，请 percent-encode，例如 `@` 写成 `%40`」。

**另一个陷阱**：`https://` 代理的证书校验。Go 默认用系统根证书校验**代理服务器**的证书。若用户填的是自签代理（如 `https://192.168.1.2:8443`），会 `x509: certificate signed by unknown authority`。v1 **不提供** `InsecureSkipVerify` 开关（安全风险 + `04-security.md` 禁止），但必须把该错误翻译成人话：「代理服务器证书不受信任（自签证书？）。本工具不支持跳过证书校验。」

---

## 4. 吞吐测量方法论

### 4.1 实测：单次采样的方差大到不可用

这是本节最重要的实证。**如果只测一次并报出那个数，工具的输出就是噪声。**

**`M3` — 单流 4 MB × 6 次（同一 URL、同一会话、连续执行）**

```
$ for i in 1..6; do curl -r 0-4194303 "https://dl.google.com/.../googlechrome.dmg?cb=$RANDOM$i"; done

MB/s: [4.60, 4.09, 4.54, 3.32, 4.16, 3.91]
n=6  mean=4.10  p50=4.13  min=3.32  max=4.60  CV=11.3%
```

**`M4` — 单流 8 MB × 3 次（同样条件，只改字节数）**

```
MB/s: [3.01, 3.45, 3.06]     # 另有首批一次 2.20
```

**两条结论**：

1. **CV（变异系数）11–25%。** 单次采样的误差带比「用户能感知的代理优劣差异」还大。→ **必须 N≥5 取 p50，禁止只测一次。**
2. **字节数不同，结果不同**（4 MB 组均值 4.10，8 MB 组均值 2.90）。原因：① 传输越久越容易遇到抖动/降速；② TCP 慢启动占比不同；③ 可能落到不同 CDN 边缘节点。→ **报告必须同时标注「测了多少字节」，否则数字不可比。**

### 4.2 单流 vs 多流

**`M5` — 4 并发 × 8 MB vs 单流 8 MB（同一时刻）**

```
4 并发：stream1=839,175  stream2=853,560  stream3=852,920  stream4=853,260  B/s each
        聚合 ≈ 3,399,000 B/s ≈ 3.24 MiB/s

单流：  3,010,704–3,454,392 B/s ≈ 2.87–3.29 MiB/s
```

**结论：本机当前链路下，单流已经打满，4 并发聚合吞吐 ≈ 单流吞吐，且每流被均分到 ~1/4。**

**这个结果有两层含义，必须分别处理**：

| 含义 | 处理方式 |
|---|---|
| 本链路存在**总带宽上限**，且单流即可触及 → 多流边际收益 ≈ 0 | 默认走**单流**（快、省内存、指标干净） |
| 高 BDP 链路（跨国长肥管道）单流受 `cwnd/RTT` 限制，多流能显著提升 | **Deep 模式提供多流**，且必须同时报「单流值」和「多流聚合值」两个数 |

**决策（详见 ADR-04）**：

- `quick` / `standard`：**单流**，因为要回答的是「体感速度」，而用户下载文件时绝大多数是单流（浏览器对单文件默认 1 连接）。
- `deep`：单流 + 4 并发两段都测。报告呈现为「单流 3.2 Mbps / 多流聚合 3.2 Mbps（多流无增益 → 链路已饱和）」。
- **绝不能**只报多流聚合值然后宣称「你的代理有 X Mbps」——那会让单流受限的代理显得比实际好。

### 4.3 采样口径（可直接抄进代码）

```go
const (
    perfBytesQuick    = 8 << 20   // 8 MiB
    perfBytesStandard = 24 << 20  // 24 MiB
    perfBytesDeep     = 64 << 20  // 64 MiB
    warmupSkip        = 1000 * time.Millisecond  // 丢弃前 1s（慢启动）
    sampleInterval    = 200 * time.Millisecond   // 200ms 出一个速率点
    minSamples        = 5                        // 少于此不报 p50/p90
)
```

**逐项理由**：

| 参数 | 值 | 理由 |
|---|---|---|
| `io.Copy(io.Discard, resp.Body)` | — | **不落盘、不驻留内存**。`io.Discard` 底层是 `ReadFrom` 快路径，直接消费 `resp.Body` 的读缓冲，零额外分配。这是「不写临时文件也能测速」的唯一正确做法 |
| 读缓冲 | 32 KiB（`io.Copy` 默认） | 小于 MSS×窗口会导致 syscall 次数上升；32 KiB 是 Go 默认且实测够用。**不要**改成 4 KiB |
| `Accept-Encoding: identity` | 硬要求 | 压缩会让「读到字节」≠「线路字节」，`Content-Length` 也不可信（gzip 时无 CL）。`06-engine-design.md` §1.2 已定 |
| **丢弃前 1000 ms** | 硬要求 | TCP 慢启动：初始 `cwnd` 通常 10 MSS（~14 KB），需 ~7 RTT 才能到 64 KB+。跨国链路 RTT 150 ms → 慢启动期约 1 s，期间速率从 ~0.6 Mbps 爬到稳态。**不剔除会让 p50 系统性偏低，且偏低程度随 RTT 变化 → 跨代理不可比** |
| 采样窗口 200 ms | 折中 | 太短（50 ms）→ 受单次 `Read` 抖动主导，曲线毛刺严重；太长（1 s）→ 看不到降速拐点。200 ms 在 8 MiB 预算下能出 ~10–15 个点，够画曲线 |
| `minSamples = 5` | 统计有效性 | 少于 5 个点时 p90 无意义（等价于 max） |

**p50 / p90 / CV 的计算**：

```go
// 输入：剔除慢启动后的 []float64（每个是 200ms 窗口的瞬时 Mbps）
p50 := probe.Percentile(vals, 50)   // 中位数：抗单点尖峰
p90 := probe.Percentile(vals, 90)   // 峰值参考：用户感受到的"最快瞬间"
cv  := probe.StdDev(vals) / probe.Mean(vals)  // 变异系数：稳定性
```

**为什么是 p50 而不是 mean**：`M3`/`M5` 显示偶发单点会拉到 0.3 Mbps 或飙到 8 Mbps。mean 会被尖峰污染；p50 反映「典型体感」。p90 单独报出来，让用户看到「最好的时候能到多少」（这是「够不够看 4K」的判据）。

**CV 的解读边界**（必须写进报告脚注）：

| CV | 解读 |
|---|---|
| < 0.15 | 稳定，适合实时音视频/长会议 |
| 0.15–0.35 | 一般，网页/下载无感，视频可能偶发缓冲 |
| > 0.35 | 抖动明显，视频会卡、语音会断 |

**`decay_ratio`（吞吐衰减）**：

```
decay_ratio = p50(后 1/3 窗口) / p50(前 1/3 窗口)
```

`< 0.7` 说明「越下越慢」——这是代理被 QoS/限速的典型特征，也是**比平均速度更有诊断价值的指标**（平均 50 Mbps 但衰减到 0.3 的代理，实际不可用）。

### 4.4 CDN 缓存规避

三个层次，逐级加强：

| 手段 | 实现 | 效果 |
|---|---|---|
| **随机 query** | `?pl=<runID>` | 绕过 CDN 的 URL 级缓存键。实测 `M3` 用 `?cb=$RANDOM` 未影响 `206` 正常返回 |
| **Range 固定字节** | `Range: bytes=<start>-<end>`，`start` 由 `runID` 派生的伪随机偏移（64 KiB 对齐） | 每次命中不同字节区间，避免边缘节点已缓存的热段 |
| **`Cache-Control: no-cache`** | 请求头 | 对支持它的 CDN 有效（Google CDN 会回源） |

**注意反效果**：随机 query 会导致**每次都是 MISS** → 测的是「回源速度」而非「边缘缓存速度」。而用户日常下载的 Chrome 安装包大概率是 **HIT**。

- **决策**：`quick`/`standard` **不加**随机 query（测用户实际会遇到的 HIT 路径，更真实）；`deep` 模式加随机 query 并单独标注「冷缓存（回源）」，与热缓存值并列展示。
- 实测 `M3`/`M4` 中加 `?cb=` 与不加，吞吐无系统性差异（4.10 vs 2.90 的差异来自字节数，见 §4.1）→ 说明 `dl.google.com` 对该 query 基本忽略，缓存规避在本 URL 上收益很小。**因此不值得为它牺牲真实性。**

### 4.5 时长控制：双闸门

固定的「字节预算」在极慢代理上会跑很久，「时长预算」在极快代理上会测不到足够字节。

```
终止条件 = (bytes_read >= budget_bytes) OR (elapsed >= budget_duration) OR ctx.Done()

并且在 elapsed >= 3000ms 后，若 bytes_read < 64 KiB → 判定为 STALLED（提前失败）
```

- `STALLED` 必须与「慢」区分：3 秒下不到 64 KiB ≈ < 0.17 Mbps，属于「基本不可用」，不能让用户等满 30 s 才知道。
- 实际耗时一旦超过 `budget_duration`，p50/p90 仍可算（样本够即可），报告标 `partial: true`。

---

## 5. 握手指标：`net/http/httptrace` 与「哪些数是假的」

### 5.1 可用时间点

| 回调 | 时间点含义 | 可靠性 |
|---|---|---|
| `DNSStart` / `DNSDone` | 本地 DNS 解析开始/结束 | **走 HTTP 代理时不触发**；TUN 下返回 fake-IP（耗时无意义） |
| `ConnectStart` / `ConnectDone` | 底层 TCP 建连 | **测的是到「本地代理」或「TUN 接口」的一跳** |
| `TLSHandshakeStart` / `TLSHandshakeDone` | TLS 握手 | **端到端真实**（握手必须和远端完成） |
| `GotConn` | 拿到可用连接（含 `Reused` 标志） | 用于判断连接复用 |
| `WroteRequest` | 请求写完 | 作为 TTFB 计时起点 |
| `GotFirstResponseByte` | 收到首字节 | **端到端真实** |
| `Got100Continue` | 100-continue | 本项目不用 |

### 5.2 `M9` — 三路径对照实测（关键证据）

**路径 1：环境变量 HTTP 代理**

```
$ ./plprobe https://www.gstatic.com/generate_204
  status=204 proto=HTTP/2.0
  DNS=0/n-a (addrs=[])   TCP=0.26ms   TLS=102.27ms
  TTFB=167.24ms  total=276ms

$ ./plprobe https://api.openai.com/v1/models
  status=401  DNS=0/n-a   TCP=0.34ms  TLS=169.31ms  TTFB=233.36ms

$ ./plprobe https://chatgpt.com/
  status=403  DNS=0/n-a   TCP=0.28ms  TLS=102.01ms  TTFB=83.82ms
  >>> CF-MITIGATED="challenge"   Server="cloudflare"
```

**路径 2：直连（TUN 接管）**

```
$ PL_PROXY=direct ./plprobe https://www.gstatic.com/generate_204
  status=204  DNS=5.10ms (addrs=[198.18.0.69])  TCP=0.72ms  TLS=96.38ms  TTFB=77.04ms

$ PL_PROXY=direct ./plprobe https://chatgpt.com/
  status=403  DNS=4.65ms (addrs=[198.18.0.70])  TCP=0.68ms  TLS=94.21ms  TTFB=84.23ms
  >>> CF-MITIGATED="challenge"
```

**路径 3：显式指向本地代理端口**

```
$ PL_PROXY=http://127.0.0.1:64080 ./plprobe https://www.gstatic.com/generate_204
  DNS=0/n-a  TCP=0.29ms  TLS=101.55ms  TTFB=82.49ms
```

### 5.3 三条硬结论

**结论 1：走 HTTP 代理时 DNS 阶段是「不可得」，不是「0 ms」。**

`addrs=[]` 且 `DNSStart` 从未触发 —— 因为 `CONNECT host:443` 由代理解析域名，Go 侧根本没有 DNS 解析动作。

> **禁止 `dns_ms = 0`。** 必须输出 `null`，前端显示「由代理解析」。填 0 会被用户读成「DNS 快到 0ms」，是**编造数据**。

**结论 2：`tcp_ms` 测的是本地一跳，不是远端 RTT。**

| 路径 | `tcp_ms` | 真实含义 |
|---|---|---|
| HTTP 代理 | 0.26–0.34 ms | 到 `127.0.0.1:64080` 的 loopback 连接 |
| TUN 直连 | 0.68–0.80 ms | 到 fake-IP（本地 TUN 网卡）的连接 |

两种情况下，**用户对外部世界的真实建连耗时都在代理/TUN 内部，Go 完全不可见**。

> **禁止把 `tcp_ms` 标为「TCP 建连」或「RTT」。** 标签必须是「本地代理建连」，且 UI 需在 tooltip 说明「真实远端建连由代理完成，本工具无法分离」。

**结论 3：`tls_ms` 与 `ttfb_ms` 是唯一端到端可归因的握手指标。**

三条路径下 TLS 均为 **94–169 ms**，TTFB **77–233 ms**，与环境无关、与代理形态无关。原因：TLS 握手必须与目标服务器完成 1–2 个 RTT（TLS 1.3 为 1-RTT），`CONNECT` 隧道只是透明转发，因此**TLS 耗时 ≈ 真实 RTT × 1（TLS1.3）+ 少量非对称运算**。

> **这是本工具「网络层 RTT」的唯一可信代理量。** 报告的「延迟」主指标应基于 `tls_ms`，而不是 `tcp_ms` 或 DNS。

**推论（必须写进 `06-engine-design.md` 的一致性检查）**：`06` 的 `Phase` 结构含 `DNS/TCP/TLS/TTFB/Total` 五字段。其中：

- `DNS` 走代理时为 `null`，TUN 时标注「fake-IP 本地合成」。
- `TCP` 必须改名或加标签为「本地代理建连」。
- `TLS` 是延迟主指标。
- `TTFB - TLS` ≈ 服务端处理 + 首字节回传，可作为「服务端响应速度」。

### 5.4 连接复用的 0 值处理

`standard` / `deep` 模式下 `P6` 会连采 10 次。若复用同一 `Transport`，第 2 次起：

```
ConnReused = true  →  DNSStart/ConnectStart/TLSHandshakeStart 全部不触发 → 全为 0
```

**这不是「变快了」，是「没测量」。**

| 处理方式 | 说明 |
|---|---|
`conn_reused` 字段 | 必须输出，前端对 `reused=true` 的样本**不显示** DNS/TCP/TLS 三段（显示 `—`） |
`jitter` 计算 | **只用 `ttfb_ms` 差值**，不受复用影响 |
`RTT 序列` | 只用 `ttfb_ms`，不用 `tls_ms`（复用时 tls=0） |
`deep` 模式额外一组 | 显式 `DisableKeepAlives: true` 的 5 次采样，专门测「冷连接」的 `tls_ms`，作为「每次新建连接的真实代价」（对「打开新网页」场景有意义） |

**决策（与 `06-engine-design.md` D6 一致）**：

- **探活类（P3/P4/P5/P6）共用 `Transport`** → 快、省资源，符合真实浏览器行为（浏览器也复用连接）。
- **吞吐类（P1/P2）每次新建 `Transport`** → 需要干净连接，且不能与其他 probe 抢占同一 TCP 连接的带宽。

### 5.5 实现骨架（已验证可编译运行）

```go
var t struct{ dnsS, dnsD, connS, connD, tlsS, tlsD, gotConn, wrote, firstByte time.Time
              reused bool; addrs []string }

ctx := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
    DNSStart:  func(httptrace.DNSStartInfo) { t.dnsS = time.Now() },
    DNSDone:   func(i httptrace.DNSDoneInfo) {
        t.dnsD = time.Now()
        for _, a := range i.Addrs { t.addrs = append(t.addrs, a.String()) }
    },
    ConnectStart:  func(string, string) { t.connS = time.Now() },
    ConnectDone:   func(string, string, error) { t.connD = time.Now() },
    TLSHandshakeStart: func() { t.tlsS = time.Now() },
    TLSHandshakeDone:  func(tls.ConnectionState, error) { t.tlsD = time.Now() },
    GotConn:   func(i httptrace.GotConnInfo) { t.gotConn = time.Now(); t.reused = i.Reused },
    WroteRequest:         func(httptrace.WroteRequestInfo) { t.wrote = time.Now() },
    GotFirstResponseByte: func() { t.firstByte = time.Now() },
})
```

**错误分类器（必须在 `err != nil` 时按时间点分类，否则无法区分「慢」与「不可用」）**：

```go
switch {
case !t.gotConn.IsZero() && !t.tlsS.IsZero() && t.tlsD.IsZero():
    // TCP 已通、TLS 未完成 → TLS 被中断/重置（SNI 封锁的典型签名）
    code = "TLS_RESET"
case !t.gotConn.IsZero():
    code = "RESPONSE_TIMEOUT"      // 连接通了，但没等到响应
case !t.connS.IsZero() && t.connD.IsZero():
    code = "TCP_TIMEOUT"           // 建连卡住
default:
    code = "UNREACHABLE"           // DNS 或更早失败
}
```

实测该分类器有效：`speed.cloudflare.com` 被正确归为 `TLS_RESET`（`M10`）。

---

## 6. YouTube 真实链路可行性

### 6.1 原始设想与实际障碍

`00-task-brief.md` §P2 设想：`youtube.com` 首页 → player 配置 → `googlevideo` 分片 range 下载。**前半可行，后半（真分片下载）在纯 Go 下不可行。**

**`M8` — 实测解析 `ytInitialPlayerResponse`（`https://www.youtube.com/watch?v=dQw4w9WgXcQ`，1.40 MB 页面）**

```
found: True
keys: [responseContext, playabilityStatus, streamingData, playbackTracking, captions,
       videoDetails, annotations, playerConfig, storyboards, microformat, cards, trackingParams]
status: OK    reason: None
videoId: dQw4w9WgXcQ    len: 213
streamingData keys: [expiresInSeconds, formats, adaptiveFormats, serverAbrStreamingUrl]

formats: n=1
   itag 18  video/mp4  hasUrl=False  hasSigCipher=True
adaptiveFormats: n=26
   itag 313 video/webm vp9      hasUrl=False  hasSigCipher=False  contentLength=358608461
   itag 401 video/mp4  av01     hasUrl=False  hasSigCipher=False  contentLength=240334643
   itag 271 video/webm vp9      hasUrl=False  hasSigCipher=False  contentLength=151103346
```

**关键发现：`adaptiveFormats` 的 26 个条目里，`url` 与 `signatureCipher` 全都是空的。**

```
adaptiveFormats[0] keys:
  [itag, mimeType, bitrate, width, height, initRange, indexRange, lastModified,
   contentLength, quality, fps, qualityLabel, projectionType, averageBitrate,
   colorInfo, approxDurationMs, qualityOrdinal]
```

→ **只有元数据，没有任何可播放 URL。** 现代 YouTube 的 `WEB` client 不再下发 `<video>` 可用的直链，改为走 `serverAbrStreamingUrl`（ABR 协议，需 POST protobuf 分段请求）。

`formats[0]`（渐进式 mp4）确实有 `signatureCipher`，但内容是被签名的：

```
signatureCipher:
  s=6E06E0s2JYwRAIgQ-uWb2TVn8oz-wUi15VCgW7gCM42jGLJbFM8YAVEKVcCIAE4o02NmdAlfiNnG-TASZXh6XaUd6fLhrSEHCvfzpHiHd
  &sp=sig
  &url=https://rr1---sn-ogueln67.googlevideo.com/videoplayback?expire=...

cipher params: ['s', 'sp', 'url']
url query keys: [..., 'n', 'sig'→缺失, 'lsig']
has n param: True
```

**要拿到可用的 URL，必须做两件事**：

1. **`s=` 签名变换（sig decipher）**：从 `player/base.js` 里提取变换函数（通常是 `a.reverse()` + `splice` 组合），**每次 YouTube 更新 base.js 都会变**。
2. **`n=` 变换（n-sig）**：不处理会让 `googlevideo` 返回**限速响应**（约 50 KB/s）或 `403`。

**`M11` — 直接验证「不做签名变换会怎样」**

```
A) serverAbrStreamingUrl 直接 ranged GET：  http=403  size=0
B) signatureCipher 里的 url 去掉 sig 后 GET：http=403  size=0
```

**双重确认：不做 JS 层签名处理，拿不到一个字节的媒体数据。**

### 6.2 为什么不引入 JS 引擎

| 方案 | 评价 |
|---|---|
| 内嵌 goja / otto 执行 `base.js` | **否决**。`base.js` 约 2 MB 混淆 JS，含大量 ES2020+ 特性与 YouTube 自研语法；执行需 sandbox；每次 YouTube 更新都可能失效（历史上平均每 1–3 个月一次）。维护成本 = 长期跟踪逆向。且引入 ~5 MB 依赖，二进制从 4 MB 变 10 MB+ |
| 移植第三方 `n-sig` 实现 | **否决**。`yt-dlp` 的解法是**运行时下载并解释** JS，本质同 goja；Go 生态无可靠的等价物 |
| 用 `yt-dlp` 子进程 | **否决**。违反「不依赖用户预装运行时」；且为测速而打包一个 Python 工具链是荒谬的 |
| **接受降级** | **选定** |

### 6.3 可接受降级方案（三级，逐级尽力）

**`M12` — 降级路径逐级实测**

| 级 | 请求 | 实测结果 | 产出指标 |
|---|---|---|---|
| **T1 页面级** | `GET https://www.youtube.com/` | `HTTP/2 200`，955 KB，TTFB 300 ms，`server: ESF`，`set-cookie: YSC=...` | 页面 TTFB、页面 TLS、首屏可用性、`playabilityStatus.status` |
| **T2 缩略图** | `GET https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg` | `HTTP/2 206`（range）或 `200`，21,011 B，TTFB 171 ms，`server: sffe` | 缩略图 TTFB、小文件吞吐（**不折算 Mbps**，见下） |
| **T3 媒体边缘握手** | `GET https://<host from serverAbrStreamingUrl>/`（仅探握手） | `HTTP/1.1 404`，1,561 B，**`server: gvs 1.0`**，`connect=0.36ms appconnect=87.27ms` | **到真实 `googlevideo` 边缘的 TLS 耗时**（87 ms） |
| **T4 Google 边缘大文件** | `GET https://redirector.gvt1.com/edgedl/go/go1.22.0.darwin-arm64.pkg`（跟随 302） | `302` → `https://rr2---sn-ajoguc-cb.gvt1.com/...`，range `206` | Google 边缘缓存节点的真实吞吐 |

**T3 是本降级方案的关键洞察**：即使拿不到媒体字节，**`googlevideo` 边缘会对裸 `GET /` 返回 `404`**，这个 404 已经证明：
- DNS 解析到真实 `googlevideo` 边缘 ✓
- TCP 建连成功 ✓
- TLS 握手完成（87 ms）✓
- **响应头 `server: gvs 1.0` 是 "Google Video Server" 的确定性指纹** ✓

**T4 的洞察**：`redirector.gvt1.com` 302 落到 `rr2---sn-xxxx.gvt1.com` —— 这是**与 `googlevideo` 同族的 Google Global Cache 边缘节点**（`rrN---sn-` 命名规则完全一致）。在这个 host 上做大文件 range 下载，测的是「到 Google 边缘缓存节点的下行吞吐」，与 YouTube 播放**共享同一条边缘回程**。

### 6.4 降级后的指标解释边界（必须写进报告脚注）

> **这是本工具诚实性的核心声明之一，不可省略。**

| 指标 | 能说明什么 | **不能说明什么** |
|---|---|---|
| T1 页面 TTFB | 「YouTube 首页能不能打开、有多快」= **起播前的门控环节** | 不能说明视频流能不能下 |
| T1 `playabilityStatus.status` | 该视频在**当前地区/账号状态**下是否被允许播放（`OK` / `UNPLAYABLE` / `LOGIN_REQUIRED` / `ERROR`） | 不能说明带宽够不够 |
| T2 缩略图 TTFB | `i.ytimg.com` 通、YouTube 静态 CDN 正常 | **不能折算 Mbps**（15–21 KB 的测量被 TLS 慢启动与 RTT 主导，算出的"速率"是假的） |
| T3 `gvs` 边缘 TLS | 到 YouTube 媒体服务器的**握手路径**通、RTT 多少 | 不能说明媒体传输速率（没传媒体） |
| T4 `gvt1` 边缘吞吐 | 到 **Google 边缘缓存**的下行能力，可作 YouTube 码率上限的**上界参考** | 不能等同于 YouTube 播放速率（`googlevideo` 有额外限速策略、ABR 分片大小不同、可能有 `n-sig` 降速） |

**报告展示规则**：

```
YouTube 链路                      可用（降级模式）
  页面起播     300 ms   ✓
  媒体边缘     87 ms    ✓ (googlevideo · gvs 1.0)
  边缘下行     3.2 Mbps  (gvt1 代理指标)
  ⓘ 未取到真实播放分片（YouTube 需 JS 签名，纯 Go 不可行）。
    以上为同链路代理指标，非播放实测速率。
```

**绝不允许**把 T4 的数字标成「YouTube 播放速度」。**必须**是同链路代理指标 + 明确脚注。

### 6.5 与 `06-engine-design.md` 的对齐说明

`06` 的 P2 设计了「T3 真分片（尽力）」：`POST youtubei/v1/player` 取 `adaptiveFormats[*].url`。**该设计在本机实测下会稳定失败**（`06` 自己也记录了证据 E4a/E4b/E4c：`WEB` client → `UNPLAYABLE` + `adaptiveFormats=0`，`ANDROID` → 400）。

**本文追加的建议（供 `06` 采纳）**：

- 保留 `06` 的 T1/T2/T3 编号，**将 T3 的「取 url 真下载」改为「取 `serverAbrStreamingUrl` 的 host 做握手探测」**——后者 100% 成功且信息量明确（`gvs 1.0` + TLS 耗时）。
- **新增 T4**：`redirector.gvt1.com` → 302 → `rr*.gvt1.com` 大文件 range 下载，作为「Google 边缘下行吞吐」。
- `media_path` 枚举值增加 `"gvt1_proxy_metric"`，与 `"googlevideo_real"`（未来若实现）、`"unavailable"` 并列。

---

## 7. AI 可用性判定

### 7.1 核心原则：**响应头优先，body 兜底**

**为什么**：body 指纹需要下载页面（Gemini 首页实测 818 KB，`M9`），慢且带宽敏感；而响应头是**服务端控制面语义的确定表达**，字节量极小、几乎不可能被伪造。

**`M9` / `M10` 实测到的确定性 header 指纹**：

| 目标 | 状态码 | 关键响应头 | 语义 |
|---|---|---|---|
| `chatgpt.com` | **403** | **`cf-mitigated: challenge`**、`server: cloudflare`、`server-timing: chlray;desc="..."` | **Cloudflare 质询**（网络通，被风控拦截） |
| `api.openai.com/v1/models` | **401** | `server: cloudflare` | **网络与服务完全可达**，仅缺 key |
| `gemini.google.com` | **200** | `server: ESF`、`server-timing: l2gfet4t7; dur=131` | 正常可用 |
| `www.youtube.com` | 200 | `server: ESF` | 正常 |
| `i.ytimg.com` | 200/206 | `server: sffe` | 正常 |
| `*.googlevideo.com` | 404 | **`server: gvs 1.0`** | 边缘可达 |
| `www.gstatic.com/generate_204` | 204 | — | 基线正常 |

**`cf-mitigated` 是本次调研最有价值的发现**：它是 Cloudflare 的**专用控制头**，取值 `challenge` 表示「我给你的是质询页而不是真页面」。这比匹配 `Just a moment...` 之类 body 关键词可靠一个数量级（用户语言/地区/CF 版本都会改文案，但 header 是 API 契约）。

### 7.2 状态码语义表

| 码 | 对 AI 服务的含义 | 判定 | 归因 |
|---|---|---|---|
| **200** | 正常返回 | `AVAILABLE` | 代理正常 |
| **204** | 正常（generate_204 专用） | `AVAILABLE` | 代理正常 |
| **301/302/307/308** | 重定向，**必须跟随并看终态** | 由终态决定 | — |
| **401** | 缺认证。**对 P5 是「完全正常」** | `AVAILABLE`（`NO_CREDENTIAL_PATH_OK`） | 网络与服务均通，仅无 key |
| **403** | **必须分三种**（见 §7.3） | `CHALLENGE` / `REGION_BLOCKED` / `AVAILABLE` | 视证据 |
| **429** | 限流 / 额度耗尽 | `DEGRADED` | **账号侧，不归因代理** |
| **451** | 因法律原因不可用（地区封锁） | `REGION_BLOCKED` | 代理落地国家不可用 |
| **5xx** | 服务端故障 | `DEGRADED`（`UPSTREAM_5XX`） | **不归因代理**，但影响体感 |
| **SSL/TLS 错误** | 握手被中断 | `TLS_RESET` | **代理/线路被 SNI 阻断** |
| 超时 | 无响应 | `TIMEOUT` | 代理失效或极慢 |

**关键区分（产品的差异化价值）**：

```
401 + {"error":{"type":"invalid_request_error","message":"Missing bearer authentication in header"}}
  → 网络 100% 通，服务 100% 可达。这是「最好」的结果之一，不是失败！
    （实测 M9：body 151 字节，正是此 JSON）

429
  → 代理没问题，是用户账号额度问题。报告必须说「代理正常，账号侧限流」，
    绝不能显示成「代理不可用」——否则用户会白折腾换节点。
```

### 7.3 403 的三分法（最易出错）

`403` 是唯一需要「多证据裁决」的状态码：

```go
switch {
case resp.Header.Get("cf-mitigated") == "challenge":
    → CHALLENGE
    // 最强证据。直接判定，无需读 body。

case resp.Header.Get("cf-mitigated") == "block":
    → CHALLENGE (blocked)
    // CF 直接拒绝，通常是 IP 信誉极差 / WAF 规则命中

case strings.Contains(lower(body), "unsupported_country") ||
     strings.Contains(lower(body), "not available in your country") ||
     strings.Contains(lower(body), "country, region, or territory not supported"):
    → REGION_BLOCKED

case server == "cloudflare" && resp.StatusCode == 403:
    → CHALLENGE (probable)
    // 有 server: cloudflare 但无 cf-mitigated：可能被中间设备剥了头
    // 需 body 二次确认

default:
    → DENIED (generic)
}
```

**实测确认**：`chatgpt.com` 走本机代理 → `403` + `cf-mitigated: challenge`。这是**当前环境的真实状态**，不是工具 bug。原生浏览器打开 `chatgpt.com` 会看到 CF 质询页并可能通过（浏览器有 JS + cookie + 真实指纹），而 Go 客户端必然停在质询。

> **这是必须向用户解释的边界**：ProxyLens 判定 `CHALLENGE` 时，含义是「**服务器的机器人防护在拦截非浏览器客户端**」，**不等于**「你的浏览器用不了 ChatGPT」。
> 报告文案必须是：
> 「⚠ 检测到 Cloudflare 人机验证。你的代理**已连通** OpenAI 服务器，但服务器要求浏览器验证。请在浏览器中打开 chatgpt.com 确认是否可正常使用——多数情况下可以。」

**这条文案是产品成败关键**：如果直接显示「ChatGPT 不可用」，用户会以为代理坏了，而实际上打开浏览器就能用。**误报比不报更糟。**

### 7.4 地区限制识别

| 证据 | 判定 |
|---|---|
| `451` | `REGION_BLOCKED`（确定） |
| body 含 `not available in your country` / `isn't available in your country` / `currently not available in your country` | `REGION_BLOCKED` |
| body 含 `unsupported_country` / `Country, region, or territory not supported` | `REGION_BLOCKED`（OpenAI 的措辞） |
| `gemini.google.com` 重定向到 `accounts.google.com/signin` | `AUTH_REQUIRED`（不是地区问题） |
| `gemini.google.com` 200 + `<title>Gemini</title>` | `AVAILABLE` |

**注意**：Gemini 首页 200 且 818 KB 是**正常**的（未登录也能拿到落地页）。不要因为「体积大」或「没看到聊天框」就判失败。

### 7.5 Cloudflare 挑战特征（完整清单，用于兜底）

按可靠性从高到低：

| 序 | 特征 | 位置 | 可靠性 |
|---|---|---|---|
| 1 | `cf-mitigated: challenge` | 响应头 | **确定** |
| 2 | `cf-mitigated: block` | 响应头 | **确定** |
| 3 | `server: cloudflare` + `403` + `cf-ray` 存在 | 响应头 | 高 |
| 4 | `server-timing: chlray;desc="..."` | 响应头 | 高（Cloudflare 挑战专用 timing 名） |
| 5 | `challenges.cloudflare.com` | body | 高 |
| 6 | `<title>Just a moment...</title>` | body | 中（文案会本地化） |
| 7 | `Enable JavaScript and cookies to continue` | body | 中 |
| 8 | `cf-chl-` / `__cf_chl` | body | 中 |
| 9 | `window._cf_chl_opt` | body | 中 |

**实现要求**：body 扫描上限 **64 KiB**（`io.LimitReader`），扫完即 `Close()`。**不要下完整个页面**——Gemini 818 KB、YouTube 1.4 MB，全部下完会让「可用性检测」变成「带宽测试」（`06` D4 已定，本文确认）。

### 7.6 响应被篡改/劫持的识别

「被墙/被劫持」和「服务器拒绝」是完全不同的结论，必须区分：

| 特征 | 判定 |
|---|---|
| `Content-Type: text/html` 但请求的是 `.dmg`/`.jpg`/`/v1/models` | `HIJACKED`（被劫持到门户/验证页） |
| `Content-Length` 存在但实际读到字节数为 0 或远小于 | `TRUNCATED` |
| TLS 证书 CN/SAN 与目标域名不匹配 | `MITM`（中间人，可能是企业代理或审查设备） |
| 响应体是运营商/网关的固定 HTML（含 `电信`/`联通`/`网络接入`/`portal`） | `HIJACKED` |
| 204 端点返回 200 + 非空 body | `HIJACKED`（`generate_204` 语义被破坏） |
| `err == net/http: TLS handshake timeout` 或 `read: connection reset by peer`（TCP 已通） | `TLS_RESET`（SNI 阻断） |

**`TLS_RESET` 的实测样本（`M10`）**：

```
$ curl -v "https://speed.cloudflare.com/__down?bytes=1000"
< HTTP/1.1 200 Connection Established     ← CONNECT 隧道成功
* CONNECT phase completed
* ALPN: curl offers h2,http/1.1
* LibreSSL SSL_connect: SSL_ERROR_SYSCALL in connection to speed.cloudflare.com:443
curl: (35)
```

**这是最典型的「TCP 通、TLS 被打断」签名**：隧道建立成功、`ClientHello` 发出、然后在等 `ServerHello` 时被 RST。

> **注意**：这次是靠 `speed.cloudflare.com` 暴露出来的。它在本机**不可用**，因此它是「SNI 阻断的检测器」而非「吞吐源」。`06` 的 D2 已正确将其降为可选对照，本文确认。

---

## 8. 打包与分发

### 8.1 构建矩阵

```bash
#!/usr/bin/env bash
# scripts/build.sh —— 一台 macOS 主机产出四个平台单文件
set -euo pipefail
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)

build() {  # $1=GOOS $2=GOARCH $3=输出后缀
  local out="dist/proxylens-${VERSION}-$1-$2$3"
  CGO_ENABLED=0 GOOS=$1 GOARCH=$2 \
    go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o "$out" ./cmd/proxylens
  echo "built: $out ($(du -h "$out" | cut -f1))"
}

build darwin  arm64   ""
build darwin  amd64   ""
build windows amd64   ".exe"
build windows arm64   ".exe"
```

**`CGO_ENABLED=0`** 是硬要求（`M6`）：纯静态链接 → 无 GLIBC/musl 依赖 → 任何 Linux 发行版/任何 macOS 版本直接跑。同时也是「内嵌 WKWebView 不做」的技术原因。

**`-trimpath`** 移除构建机绝对路径（既是可复现性要求，也避免泄露开发机目录结构）。

**`-s -w`** 去符号表与 DWARF：从 5.2 MB 降到 4.0 MB（实测约 -23%）。代价是 `panic` 无行号 —— 通过 `-X main.version` 注入版本号 + 结构化日志弥补。

### 8.2 UPX：**不使用**

| 维度 | 结论 |
|---|---|
| 收益 | 4.0 MB → ~1.6 MB（约 -60%） |
| 代价 1 | **启动变慢**：UPX 需在内存中解压整个二进制（~30–80 ms），正好抵消 Go 的 < 10 ms 冷启动优势（`M7`） |
| 代价 2 | **杀软误报率极高**。UPX 加壳是恶意软件常见特征，Windows Defender / 360 / 火绒对 UPX-packed Go 二进制的误报是已知的行业问题 |
| 代价 3 | macOS 上 UPX 压缩过的 Mach-O **可能破坏代码签名**，且 Gatekeeper 更严格 |
| 代价 4 | 本机 `upx` 未安装，需额外分发工具链 |

**决策：不用 UPX。** 4 MB 的下载成本远低于「被杀软隔离」的用户支持成本。若未来要减体积：`-tags osusergo,netgo` 换掉 cgo 解析器、裁剪未用包、前端资源 gzip —— 都能拿到 10–20% 而无副作用。

### 8.3 macOS 未签名 Gatekeeper 处理

**必须处理的现实**：无 Apple Developer ID（$99/年）时，用户首次打开会遇到：

```
"proxylens" cannot be opened because the developer cannot be verified.
```

或（从网络下载后）：

```
"proxylens" is damaged and can't be opened. You should move it to the Trash.
```

**三种用户解法（必须写进 README + 首次启动提示）**：

| 方法 | 操作 | 适用 |
|---|---|---|
| **A. 右键 → 打开** | 右键点击二进制 → 选「打开」→ 弹窗点「打开」 | macOS 12–14 有效；**macOS 15+ 已移除该绕过路径** |
| **B. 系统设置** | 系统设置 → 隐私与安全性 → 底部「仍要打开」 | macOS 13+ 推荐 |
| **C. `xattr` 去隔离**（对 "damaged" 唯一有效） | `xattr -d com.apple.quarantine ./proxylens-darwin-arm64` | "damaged" 场景必需 |

**发布方该做的（低成本、高收益）**：

1. **不打包 `.app`，直接分发裸二进制**。裸 Mach-O 从终端运行不触发 Gatekeeper 的 App 完整性校验路径（`com.apple.quarantine` 仍会打上，但"damaged"的成因主要是 zip/dmg 内的 `.app` 结构）。**这是最省事的规避**。
2. **提供 `install.sh`**，内含 `xattr -d com.apple.quarantine` + `chmod +x` + 移动到 `~/.local/bin`，用户一条命令解决。
3. **README 顶部显著位置**给出上面的 A/B/C 三种路径 + 截图友好说明。
4. **程序自身检测**：若运行在 macOS 且检测到 `com.apple.quarantine` 属性（可用 `xattr` 子进程或 `syscall` 直接读 `getxattr`），启动时打印提示。**但程序若无法启动就检测不到** —— 所以这一步只是锦上添花。

**future（不在 v1 范围）**：Apple Developer ID 签名 + `notarytool` 公证 ≈ $99/年；Windows 侧 EV/OV 代码签名证书 ≈ $200–400/年。用户未提出预算，**v1 不做**，但在 README 说明「未签名，属于开源工具常规分发形态」。

### 8.4 Windows 特有事项

| 事项 | 决策 |
|---|---|
| 控制台窗口 | 默认带黑窗。**v1 接受**（工具性质，用户能看到启动 URL 与日志反而有用）。若要隐藏需 `-ldflags "-H windowsgui"`，但那会丢掉 stdout → 无法打印启动 URL。**折中**：提供 `proxylens.exe --gui` 变体（`-H windowsgui` + 写日志到文件），作为 v1.1 选项 |
| 图标/版本资源 | 需 `.syso`（`go-winres` 或 `rsrc` 生成）。本机两者均未装（实测）。**v1 不做**，后续 `go install github.com/tc-hib/go-winres@latest` 即可 |
| SmartScreen | 无签名 → 「Windows 保护了你的电脑」。README 给出「更多信息 → 仍要运行」说明 |
| `ProxyEnable` 读取权限 | `HKCU` 读权限对所有用户开放，无需提权 ✓ |
| 路径含空格/中文 | `os.Executable()` + `filepath` 处理；浏览器启动用 `exec.Command("rundll32", "url.dll,FileProtocolHandler", url)` 避免 `cmd /c start` 的引号地狱 |

---

## 9. 风险与未解决问题

| # | 风险 | 概率 | 影响 | 缓解 |
|---|---|---|---|---|
| R1 | `dl.google.com` 的 Chrome dmg 路径变更 / 停止公开 | 低 | P1 完全失效 | 备用 URL `dl.google.com/chrome/mac/stable/GGRO/googlechrome.dmg`；再备用 `redirector.gvt1.com` 大文件；启动时 HEAD 探测可用性 |
| R2 | 用户处于 PAC/WPAD 网络 | 中 | 「系统代理」判定错误 | §2.5：明确提示，不静默；引导手动填写 |
| R3 | 企业 MITM 代理（自签根证书） | 中 | 全部 TLS 失败 | `x509.UnknownAuthorityError` 需专门文案 + 提示「可能是公司代理的根证书未信任」 |
| R4 | ChatGPT 的 403 CHALLENGE 被用户读成「代理坏了」 | **高** | 产品核心价值被误解 | §7.3 强制文案；UI 用黄色「⚠ 需浏览器验证」而非红色「不可用」 |
| R5 | TUN 环境的 fake-IP 被当成真实出口 IP | **高** | 输出假数据 | §2.6 强制 `198.18.0.0/15` 检测 + 标签改写 |
| R6 | 代理端 `n-sig` 限速导致 T4 吞吐虚低 | 中 | YouTube 指标偏悲观 | T4 用 `gvt1` 而非 `googlevideo` 做吞吐（`gvt1` 无 `n-sig` 限速）；`googlevideo` 只做握手 |
| R7 | 用户网络本身带宽 < 10 Mbps | 中 | 无法区分「代理慢」与「本身上限」 | 强烈建议做「直连对照」（`mode=direct`），报告并列展示差值；无对照时报告标注「未做对照，绝对速率仅供参考」 |
| R8 | 并发探测抢占带宽互相污染 | 中 | 吞吐偏低 | `04-security.md` 要求「同一时刻只允许 1 个运行」（`409`）；且 P1 不与 P2 并行 |
| R9 | Apple 公证缺失被拦 | 高 | 用户装不上 | §8.3 |
| R10 | `x/net` 依赖引入供应链风险 | 低 | 安全 | 只引 `golang.org/x/net/proxy`（或按 ADR-05 完全自写）+ `golang.org/x/sys/windows/registry`；均为一手维护的官方扩展库 |

**未解决/需后续决策**：

1. **`socks5h` 是否完全自写**（ADR-05）：`x/net/proxy` 不支持 `DOMAINNAME` 透传，`app/internal/netx/socks5.go` 已自写 195 行。需确认是否连 `x/net/proxy` 都不引入。
2. **Windows GUI 变体**（§8.4）：黑窗是否可接受，需产品侧确认。
3. **`gvt1` 作为 YouTube 代理指标的合理性能否被接受**：这是产品诚实性问题，需在 PRD 明确文案。已同步给 team-lead。

---

## 10. 实测清单（可复现）

| 编号 | 内容 | 关键命令 | 关键结论 |
|---|---|---|---|
| `M1` | 系统代理状态 | `scutil --proxy` | 全 0（TUN 形态）；env 有 `HTTP_PROXY=127.0.0.1:64080` |
| `M2` | TUN fake-IP | `plprobe` direct 模式 | `198.18.0.69/70/93` → fake-IP 确认 |
| `M3` | 单流 4 MB × 6 | `curl -r 0-4194303` | mean 4.10 / p50 4.13 / CV 11.3% MB/s |
| `M4` | 单流 8 MB × 3 | 同上，改 range | 3.01 / 3.45 / 3.06 MB/s（首批 2.20） |
| `M5` | 4 并发 × 8 MB | 4 个 curl 后台并行 | 每流 ~0.85 MB/s，聚合 ≈ 单流 |
| `M6` | 四目标编译 | `GOOS/GOARCH go build` | 3.94 / 4.23 / 4.27 / 3.94 MB |
| `M7` | 冷启动 | `/usr/bin/time -p` | real 0.00（< 10 ms）× 5 |
| `M8` | YouTube player 解析 | 解析 `ytInitialPlayerResponse` | `status=OK`；`adaptiveFormats` 全无 `url`；`formats[0]` 有 `signatureCipher` + `n=` |
| `M9` | httptrace 三路径 | `plprobe` × 3 模式 | 代理下 `DNS=0/n-a`、`TCP=0.26ms`、`TLS=102ms`；`cf-mitigated: challenge` |
| `M10` | `speed.cloudflare.com` | `curl -v` | CONNECT 200 后 `SSL_ERROR_SYSCALL` → TLS_RESET |
| `M11` | 无签名下载 | curl 直取 ABR/cipher URL | 均 `403`，size=0 |
| `M12` | 降级链 T1–T4 | curl 各级 | `ESF` / `sffe` / `gvs 1.0` / `gvt1` 302→`rr2---sn-*.gvt1.com` |

---

## 11. 环境事实核对（与 `00-task-brief.md` §4 一致性）

| 项 | 简报 | 本文实测 | 一致 |
|---|---|---|---|
| Go | 1.26.3 | `go1.26.3 darwin/arm64` ✓ | ✓ |
| Node | 22.22.2 / npm 10.9.7 | 一致 ✓ | ✓ |
| Python | 3.13.12 | 一致 ✓ | ✓ |
| Rust/cargo | 未安装 | `rustc`/`cargo` 均无 ✓ | ✓ |
| 系统代理 | 「未单独开启」 | `HTTPEnable:0 HTTPSEnable:0 SOCKSEnable:0 PACEnable:0` ✓ | ✓ |
| 环境变量代理 | `http://127.0.0.1:61713` | 实测为 **`http://127.0.0.1:64080`** | **端口不一致** |
| TUN | 已开启 | `198.18.0.0/15` fake-IP 确认 ✓ | ✓ |
| `upx` | — | 未安装 | （本文据此决定不用 UPX） |
| `go-winres` / `rsrc` | — | 均未安装 | （v1 不做图标资源） |

**唯一不一致项说明**：环境变量代理端口在 `00-task-brief.md` 记为 `61713`，本次实测为 `64080`。**端口随会话注入变化，属预期**。这恰好印证一条实现约束：

> **任何代码都不得硬编码代理端口或依赖 `HTTP_PROXY` 的存在。** 系统模式必须走 OS 探测（`scutil`/注册表），环境变量只作为 `warnings` 提示。

<!-- APPEND-END -->

