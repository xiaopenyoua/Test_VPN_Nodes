# ProxyLens · 代理能力探测

不是节点测速器，也不 ping 一个 URL 就下结论。ProxyLens 用**真实业务行为**测你当前的系统代理：
真下载 Google 大文件、真访问 ChatGPT / Gemini 网页、真打 OpenAI API、真走 YouTube 链路，
然后给出「能不能顺畅用 AI」的可执行结论。

单文件二进制，内嵌前端，零外部依赖，Windows / macOS 双平台。

---

## 快速开始

```sh
cd app
go build -trimpath -ldflags "-s -w" -o build/proxylens .
./build/proxylens            # 自动打开浏览器
```

终端会打印带访问令牌的地址：

```
  ProxyLens 1.0.0  ·  darwin/arm64
  ➜  http://127.0.0.1:18770/?t=<token>
```

打开后：出口模式选「系统代理」，**代理地址留空**（TUN 模式下无需填写），强度选「快速」，
点「开始探测」。约 8 秒出结论。

常用参数：

| 参数 | 说明 |
|---|---|
| `-port 18770` | 指定端口（默认随机可用端口） |
| `-no-open` | 不自动打开浏览器 |
| `-addr 127.0.0.1:18770` | 显式指定监听地址；**只接受 127.0.0.1** |

---

## 三种出口模式

| 模式 | 行为 | 什么时候用 |
|---|---|---|
| **系统代理**（默认） | 走操作系统默认出网路径。TUN 模式下即按系统路由直出 | 日常体检。**刻意忽略** `HTTP_PROXY` 等环境变量，避免测到终端注入的代理而非你的系统代理 |
| **自定义代理** | 手动填 `http://` / `https://` / `socks5://` / `socks5h://`，支持 `user:pass@` | 对比不同节点、验证某个机场是否可用 |
| **直连对照** | 忽略所有代理设置，强制直连 | 排查"是代理的问题还是本地网络的问题" |

`socks5` 与 `socks5h` 的区别：前者在本地做 DNS 解析，后者把域名交给代理解析（更抗 DNS 污染）。

---

## 三档强度

| 档位 | 并发 | 下载预算 | 实测耗时 |
|---|---|---|---|
| 快速 | 5 | 20 MB | ~8 s |
| 标准 | 6 | 64 MB | ~20 s |
| 深度 | 8 | 160 MB | ~34 s |

下载全程 `io.Discard`，**不写磁盘**。

---

## 六个探测项

| ID | 项 | 实测内容 |
|---|---|---|
| P1 | Google 大文件下载 | 多并发 Range 流式下载 `dl.google.com`，算 P50 / P90 / 峰值 / 变异系数 / 前后段衰减 |
| P2 | YouTube 链路 | 首页可达性 + 缩略图 + 尝试取视频分片直链 |
| P3 | ChatGPT 网页 | `chatgpt.com` 状态码语义 + `cf-mitigated` 风控头 + 正文/标题指纹 |
| P4 | Gemini 网页 | `gemini.google.com` 同上（采样窗口 200 KB，覆盖标题位置） |
| P5 | OpenAI API | `api.openai.com/v1/models`，401 判为"可达·需 Key"而非失败 |
| P6 | 国际链路基线 | 固定目标多次采样，算 RTT / 抖动 / 成功率 |

每项都带分阶段耗时（DNS / TCP / TLS / TTFB），点卡片可看瀑布图和完整证据链。

---

## 评分口径：先定闸门，再算分数

这是 ProxyLens 与其他测速工具最大的区别。

**不允许出现"综合 88 分、一片绿，但 ChatGPT 打不开"的结果。** 所以评分是 gate-not-weight：

1. 先看**可用性事实**定闸门：`usable` / `limited` / `blocked`
2. 分数**服从闸门上限**：
   - `blocked`（AI 站点实质性不可用）→ 总分**封顶 29**
   - `limited`（可用但有硬伤，如 ChatGPT 被风控）→ 总分**封顶 69**
   - `usable` → 正常加权
3. 权重：速度 40% / 稳定性 30% / AI 可用性 30%

闸门只看事实、不看分数，因此不会出现"分数高到把闸门顶开"的情况。该规则由 5 个单元测试钉死
（`internal/score/score_test.go`）。

速度分断点公开可解释：`0→0, 1→25, 2→40, 5→58, 10→72, 20→85, 50→95, 100→100 Mbps`。
即 100 Mbps 满分，20 Mbps 已算"够用"（4K 视频约需 25 Mbps）。

---

## 四平台构建

```sh
cd app
./build.sh          # 全部平台 → dist/
./build.sh local    # 只构建当前平台 → build/
```

产物：

| 平台 | 文件 |
|---|---|
| macOS Apple Silicon | `proxylens-1.0.0-darwin-arm64` |
| macOS Intel | `proxylens-1.0.0-darwin-amd64` |
| Windows x64 | `proxylens-1.0.0-windows-amd64.exe` |
| Windows ARM64 | `proxylens-1.0.0-windows-arm64.exe` |

### 首次运行的系统提示

二进制未做代码签名，两个平台都会拦一下：

**macOS**（提示"无法打开，因为无法验证开发者"）：

```sh
xattr -d com.apple.quarantine ./proxylens-1.0.0-darwin-arm64
```

或：系统设置 → 隐私与安全性 → 仍要打开。

**Windows**（SmartScreen"已保护你的电脑"）：点"更多信息" → "仍要运行"。

---

## 已知限制

- **P2 YouTube 只能证明"到站可达"，不能证明"播放流畅"**。YouTube 已下线旧 innertube 取流接口，
  取不到分片直链时该项会标为「受限 · 未取到分片直链」，**不会编造一个吞吐数字**。
- 工具发的是无 Cookie、无登录态的裸请求，比你的浏览器更容易触发 Cloudflare 人机验证。
  因此结论**偏保守**：判"被风控"不代表你打不开，但判"可用"你一定打得开。
- Windows 端已编译通过，尚未在真机做完整回归。
- 无代码签名。

---

## 安全

- 只监听 `127.0.0.1`，传入 `0.0.0.0` 会直接拒绝启动
- Host 头白名单（防 DNS rebinding）、无 CORS 通配头
- 启动时用 `crypto/rand` 生成一次性访问令牌，API 与 SSE 均需校验
- 代理密码在界面与导出报告中打码（`maskURL`）
- 下载数据不落盘；无账号体系、无云服务、不上报任何数据

探测目标固定为官方站点，结果仅代表**本次链路状态**。

---

## 文档

| 文件 | 内容 |
|---|---|
| `docs/00-task-brief.md` | 需求基线与边界 |
| `docs/01-product-research.md` | 产品调研、竞品、痛点机理 |
| `docs/02-tech-research.md` | 运行时与实现方案对比 |
| `docs/03-architecture.md` | 架构决策记录（ADR） |
| `docs/04-security.md` | STRIDE 威胁建模与安全基线 |
| `docs/05-prd.md` | PRD、功能清单、验收标准 |
| `docs/06-engine-design.md` | 探测引擎详细设计（参数与公式） |
| `docs/07-ux-interaction.md` | 交互架构与状态机 |
| `docs/08-ui-spec.md` | 视觉规范（设计令牌到组件） |
| `docs/08a-visual-audit.md` | 视觉走查记录 |
| `docs/09-test-plan.md` | 测试方案与用例 |
| `docs/10-acceptance-report.md` | 验收报告（含人工对照验证） |
