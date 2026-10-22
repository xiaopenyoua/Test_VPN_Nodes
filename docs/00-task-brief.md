# 任务简报：AI 代理能力探测工具（暂名 ProxyLens）

> 本文件是团队共享的唯一事实来源（Source of Truth）。所有成员开工前必读，产出必须与本文件的需求基线一致。由主理人维护，成员不得改写。

## 1. 背景与用户原话要点

用户要做的**不是一个节点测速器**（不是 ping 一个链接、也不是测某个 Clash 节点的延迟），而是一个**验证「系统代理整体能力」的工具**：

- 通过**真实访问 OpenAI / Gemini 网页、下载 Google 大型镜像、播放 YouTube 视频**等真实行为，
  测量**网络速度、稳定性、AI 可用性**三类结果。
- 不是裸测带宽，而是回答一个用户真正关心的问题：**"我现在这套代理，能不能顺畅用 AI，卡不卡、稳不稳、会不会被墙/被风控？"**
- 可以填代理地址，**也可以不填**；不填时走**系统代理**（当前环境为 TUN 模式，全局接管流量）。
- 必须支持 **Windows 与 macOS**。
- 要求**界面美观、性能好**。
- 交付范围包含：产品调研、产品设计、技术调研、技术选型、产品文档、UI 设计与交互，并**落地为可运行的工具**，最后做实测验收。

## 2. 需求基线（Must Have）

### 2.1 探测项（Probe）——真实业务行为，而非 ICMP/TCP 连通性

| # | 探测项 | 真实行为 | 主要指标 |
|---|--------|----------|----------|
| P1 | Google 大文件下载 | 从 Google 官方 CDN（如 `dl.google.com` 的 Chrome 安装包镜像 / `speed.cloudflare.com` 作为对照）流式下载固定字节量 | 吞吐 Mbps（p50/p90）、TTFB、连接建立耗时、抖动、丢速曲线 |
| P2 | YouTube 播放链路 | 取 `youtube.com` → `googlevideo.com` 播放清单，下载分片（range 请求），模拟起播 | 起播耗时、分片下载速率、卡顿/降速、可用性 |
| P3 | ChatGPT 网页可达性 | `https://chatgpt.com/`（含 `cdn.oaistatic.com` 静态资源） | HTTP 状态、Cloudflare 挑战识别、首字节、页面关键指纹 |
| P4 | Gemini 网页可达性 | `https://gemini.google.com/` | 同上，含地区不可用识别 |
| P5 | OpenAI API 端点 | `https://api.openai.com/v1/models`（无 key 时的 401 也视为"网络通、服务可达"） | 可达性、TLS 握手、响应码语义 |
| P6 | 通用国际链路基线 | `https://www.google.com/generate_204`、`https://www.gstatic.com/generate_204` | RTT、丢包率（多次采样）、失败率 |

### 2.2 指标模型（必须区分「慢」与「不可用」）

- **网络层**：DNS 解析、TCP 建连、TLS 握手（含 TLS 版本/证书链耗时）、TTFB
- **吞吐层**：平均/峰值吞吐、p50/p90、速度稳定性（变异系数）、吞吐衰减曲线
- **稳定性层**：N 次采样成功率、超时率、抖动（jitter）、连续失败次数、长时（可选 60s+）持续测试
- **AI 可用性层（关键差异化）**：HTTP 状态码语义化（200 / 401 / 403 / 429 / 451 / 5xx）、
  Cloudflare/Akamai 人机验证页识别、地区限制页识别（"not available in your country" 等特征）、
  页面关键 DOM/标题指纹、响应体是否被劫持或截断
- **综合评分**：把上述维度折算为可解释的分项分 + 总分（评分模型必须写明权重与归一化方式，禁止黑箱）

### 2.3 代理配置

- 代理地址**可留空** → 走系统代理（macOS：系统网络设置 / TUN 已接管时即直连语义；Windows：WinINET/系统代理 / TUN）
- 支持手动填写：`http://`、`https://`、`socks5://`、`socks5h://`（远程 DNS），支持 `user:pass@`
- 支持「直连」显式模式（绕过系统代理），便于对照：**开代理 vs 直连** 的差值对比
- 代理凭据**不得明文落盘**（见安全要求）

### 2.4 跨平台

- Windows 10/11（x64、arm64）、macOS 12+（Intel、Apple Silicon）
- 单文件分发优先；安装包形态可后续迭代。**不允许依赖用户预装运行时**（如 Node/Python/JVM）
- 系统代理探测需分别适配：macOS `CFNetworkCopySystemProxySettings`/`scutil`、Windows `WinINET`/注册表

### 2.5 体验与性能

- 界面美观：现代、克制、有质感；暗色优先（网络工具使用场景），亮色可切换
- 实时性：下载/检测过程中**流式刷新**指标（不阻塞 UI，不等待全部完成）
- 性能：静态资源本地内嵌、无外部 CDN 依赖；空闲 CPU 近 0；单次轻量检测 < 10s，深度检测可配置
- 可中断、可重试、可导出报告（JSON / Markdown / 截图友好摘要）

## 3. 明确不做（Out of Scope，避免范围蔓延）

- 不做节点管理、订阅解析、Clash 规则编辑（那是别的工具的事）
- 不做 IP 落地/流媒体解锁的完整数据库判定（可做轻量地区推断，但不承诺 Netflix/Disney 解锁结论）
- 不做自动化定时压测集群、不做服务端 / 云账号体系
- 首版不做移动端

## 4. 运行环境事实（实测，非假设）

- 开发机：macOS 26.3.1，arm64（Apple Silicon）
- **Go 1.26.3 已安装**（`/usr/local/go/bin/go`）→ 可用 `GOOS=windows/darwin` 交叉编译单文件
- Node.js 22.22.2 + npm 10.9.7 已安装（前端构建可用）
- Python 3.13.12 可用
- **Rust / cargo 未安装** → 若无必要不要选 Tauri（安装工具链成本高、风险大）
- 当前网络：**TUN 模式代理已开启**，全局接管；`scutil --proxy` 显示系统 HTTP/SOCKS 代理未单独开启，
  但环境变量存在 `HTTP_PROXY/HTTPS_PROXY=http://127.0.0.1:61713`（工作环境注入）。
  → 结论：**代理地址留空时，应做到"走操作系统默认出网路径"**，且必须能识别并展示当前实际出网方式。
- 仓库历史：曾有一个 Go 写的 `clash-speedtest`（节点测速 + TUI），已删除。本工具是**不同定位的新产品**，
  可借鉴其工程结构（Go 单文件 + 内嵌 Web 资源），但**不要**沿用"测节点"的思路。

## 5. 交付物清单（按阶段）

```
docs/
  00-task-brief.md          ← 本文件（主理人）
  01-product-research.md    ← 竞品与用户需求调研
  02-tech-research.md       ← 技术调研与选型对比
  03-architecture.md        ← 系统架构 + ADR（选型定稿）
  04-security.md            ← 威胁建模与安全设计
  05-prd.md                 ← 产品需求文档（含验收标准）
  06-engine-design.md       ← 探测引擎详细设计（指标/算法/时序）
  07-ux-interaction.md      ← 交互架构、页面流、状态机、组件树
  08-ui-spec.md             ← 视觉规范与关键界面规格
  09-test-plan.md           ← 测试方案与用例
  10-acceptance-report.md   ← 实测验收报告（真实跑通数据）
app/                        ← 可运行实现
```

## 6. 协作纪律

1. 每位成员只产出自己专业域的文档，**不要**替别人写。
2. 所有产出写到上表指定的文件路径（绝对路径 `/Volumes/ssd/projects-github/Test_VPN_Nodes/...`）。
3. 完成后用 SendMessage 向 `team-lead` 回传：产出文件路径 + 300 字以内要点 + 给下一阶段的明确输入/约束。
4. 有分歧或发现上游方案不可行，**必须在回传中明确指出**，不要沉默顺从。
5. 不要执行任何 git 提交操作（用户明确禁止 git add/commit/push）。
6. 中文输出，术语保留英文（TTFB、TLS、socks5h 等）。
