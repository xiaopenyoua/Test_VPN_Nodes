# ProxyLens 视觉走查（Visual Audit）

> 走查人：UI Designer ｜ 依据：`08-ui-spec.md` v1.1 ＋ `07-ux-interaction.md` §1.4 ｜ 性质：只报偏差，不改代码
> 编号说明：§1 P0、§2 P1、§3 P2、§4 偏差表（你要求的四项）；§5 为你点名的 5 个疑问的逐条判断；§6 一句话结论。

## 0. 审计口径（先说清楚我看的是什么）

| 项 | 值 |
|---|---|
| 截图 A（结果态） | `bsk-screenshot-1792633957123.png`，**2006×1009 @1x**（非 1440，注意） |
| 截图 B（初始态） | 我用无头 Chrome 对**当前源码**重拍：`1440×900` 与 `2006×1009` 各一张（`/tmp/fresh_1440.png`、`/tmp/fresh_2006.png`） |
| 源码快照 | `app/web/css/tokens.css`（09:03）、`ui.css`（09:54）、`js/components/*`（09:13–09:54）、`internal/score/score.go`（09:56） |
| 旧截图 | 09:43 三张（含 running 态）。**其中部分现象（结论句自相矛盾、出网方式长黄句）在 09:54–09:56 的源码里已修**，我在条目里标注了「已修」，避免返工 |
| 未走查 | 浅色主题（无截图，只能用 token 算比值）、S5 设置面板、S6 导出预览、S4 抽屉、Tooltip/Toast 实际渲染 |

**一个前提判断**：审计用的结果态截图是 2006px 宽。这个宽度落在我的 `bp-xl`(≥1920) 区间，而实现没有 bp 概念——它只有 `1240px` 与 `860px` 两个断点，所以 2006 下仍是「整宽三列」。因此**不能**用这张截图去验收 1440×900 的首屏预算，那一条我按 1440 的算术单列（P0-10）。

---

## 1. P0 必修（影响美观或可读性，必须改）

### P0-1 整页 48px 网格底纹，把产品做成了线框图

- **现象**：整页铺满淡方格（`body::before` + `--bg-grid`），1440 下尤其明显。它和卡片的 1px 描边形成**两套互相干扰的网格**，视觉上"未完成的原型"。截图里最扎眼的问题就是它。
- **规范**：《08》§1 原则 3「靠描边而非明度分层」；文件头「唯一允许的渐变是速度曲线下的面积填充，其余一律纯色」。规范全文没有任何背景图案。
- **改法**：
  - `app/web/css/ui.css:10-15` 删除整块 `body::before`（`content/position/inset/background-image/background-size/z-index` 全删）。
  - `app/web/css/tokens.css:4` 删 `--bg-grid`；`:81` 删 light 的同名 token。
  - 连带：`ui.css:19-22` `.app-shell{position:relative;z-index:1}` 只为压住底纹，可一并去掉。

### P0-2 品牌色是"AI 聊天蓝"，不是规范的青绿

- **现象**：logo、主按钮、分项条、选中段、曲线全用 `#5b8cff`；同屏还有 6 个绿徽章 + 蓝紫渐变 logo，**三个色系在争**。产品读起来像通用 SaaS 后台，不像"网络诊断仪表"。
- **规范**：§2.4 `--brand: #21BFAE`；§1 原则 2「一个强调色…语义状态色只出现在状态位，不做装饰」。
- **改法**：`tokens.css:15-17`
  ```css
  --brand:#21BFAE; --brand-hover:#2ED4C1; --brand-active:#17A091;
  --brand-weak:rgba(33,191,174,.12); --brand-ink:#04130F;
  ```
  light（`:92-94`）：`--brand:#0F766E; --brand-hover:#0D6A62; --brand-active:#0A5A53; --brand-ink:#FFFFFF;`
- **附带必须同批做**：主按钮文字色（P0-3）。白字压青绿只有 **2.30:1**，比现在的 3.16:1 更差。

### P0-3 主按钮白字 3.16:1，hover 2.49:1（不达 4.5:1）

- **现象**：`开始探测 / 重新探测` 是 13px/600 白字压 `#5b8cff` = **3.16:1**；hover `#7aa2ff` = **2.49:1**。这是全页唯一的主 CTA，却是对比度最差的一块。
- **规范**：§2.4「dark 品牌用青绿底 + 近黑字（8.26:1）；不要在暗色用白字压品牌色（仅 2.30:1，不合格）」；§10.1 对比度实测表。
- **改法**：`ui.css:56-59` → `.btn-primary{ color: var(--brand-ink); font-weight:600 }`；`tokens.css` 增 `--brand-ink`（见 P0-2）。`.btn-primary:hover` 不需另改，`--brand-hover` 更深/更亮都仍 ≥7:1。
- **注**：light 主题白字压 `#2f6bf0` = 4.69:1 勉强过线，改 `--brand-ink:#FFFFFF` 后维持不变。

### P0-4 三级文字 3.83:1 + 11px，全站"次要信息"发灰到读不清

- **现象**：`--text-3 #6a7280` 在 surface 上 **3.83:1**、在 raised 上 **3.59:1**，而它承担了几乎所有 11px 信息：`.dim-detail`（分项脚注）、`.probe-sub`、`.phase-cell`、`.field-hint`、`.ring-grade`、`.wf-label`、`.chart-legend`、`.brand-sub`、`.footer`。结果态截图里分项脚注要凑近才看得清，这就是"不够专业"的观感来源之一。浅色更差：`#8b93a1` on `#fff` = **3.09:1**。
- **规范**：§2.2 `--text-tertiary #7E8794` = **5.16:1**（surface）/ 4.90:1（raised）；「禁止再调低」。light `#666E7A` = 5.15:1。
- **改法**：`tokens.css:13` → `#7E8794`；`tokens.css:90` → `#666E7A`。
- **不要**用 `opacity: .7` 降权来"看起来弱一点"——那等于又把它打到 3:1 以下。

### P0-5 输入框 / 次级按钮边界 1.26:1，控件认不出来

- **现象**：`.input` 边框 `--border #24282f` 对 surface = **1.26:1**；`.btn` 边框 `--border-strong #333941` = **1.59:1**。1440 初始态里输入框几乎"无边"，用户只能靠 placeholder 猜它是输入框；`导出报告` 按钮边界也几乎隐形。浅色：1.25:1 / 1.48:1。
- **规范**：§2.3 `--line-control #5C6470`=**3.14:1**（light `#838B99`=3.43:1），且明确「`--line`/`--line-strong` 是装饰性描边，**不可**用于标识控件边界」——WCAG 1.4.11 要求 3:1。
- **改法**：
  - `tokens.css` 增 `--line-control:#5C6470`（light `#838B99`）。
  - `ui.css:85` `.input{ border:1px solid var(--line-control) }`。
  - `ui.css:46` `.btn{ border-color:var(--line-control) }`（`.btn-ghost` 保持 transparent）。
  - 装饰性描边（`.card`、`.probe-phases`、`.wf-track`、`.dim-bar` 轨道）继续用 `--border`，不要动。

### P0-6 初始态「探测项」下面是空的：页面像没加载完

- **现象**：1440×900 下，`探测项` 标题在 y≈410，紧接着 y≈475 就是页脚，**下方 440px 全空**（约 45% 首屏）。2006×1009 同样空。根因：`ProbeGrid.render()` 在 `results.length===0 && !running` 时直接 return，网格是个空 `div`。
- **规范**：§3.1 S1 要求「将测试以下 6 项」+ 3×2 张 `height:52px` 预览卡（图标 16px + 13px/500 名称，**不可聚焦**）；§11.1 P0 清单含此项。「空标题 + 空白」在验收里等价于功能缺失。
- **改法**：
  - `app/web/js/components/ProbeGrid.js:14-31` 前加分支：`if (!results.length && !running)` → 按 `ORDER` 渲染 6 个 `.probe-preview`（不要用 `.card`，不要 `tabindex`）。
  - `ui.css` 新增：
    ```css
    .probe-preview{height:52px;padding:0 12px;display:flex;align-items:center;gap:8px;
      background:var(--surface);border:1px dashed var(--border);border-radius:var(--r-md);
      font-size:13px;font-weight:500;color:var(--text-2)}
    ```
  - `main.js:32` 的 `探测项` 文案改「将测试以下 6 项」。
- **对应你的疑问 5**：见 §5.5。结论是「**不该是 6 个骨架，该是 6 张预告卡**」。

### P0-7 初始态评分环读起来像"坏了"

- **现象**：`ScoreRing(null,'等待检测','idle')` 时 `pct=0`，但 `stroke-linecap:'round'` 让 0 长度描边画出**一个孤立绿点**（12 点方向）；环心是 44px 的 `—`，渲染成一根**粗绿横杠**，配「等待检测」灰字。第一眼像"故障指示 / 减号 / 加载失败"，而不是"尚未开始"。
- **规范**：§4.3「环色 = 阈值色」——没有分数就没有阈值，不应有颜色；§6.6「`SKIPPED` 与加载中骨架必须可区分」的同一条精神。
- **改法**：
  - `ScoreRing.js:18-21`：`!hasScore` 时不渲染进度圆（或 `'stroke-linecap':'butt'`）。
  - `ScoreRing.js:10-12`：`state==='idle'` 时 `color` 用 `var(--text-tertiary)`，不要落到默认的 `--ok`。
  - `ScoreRing.js:26`：无分数时 `ring-num` 不输出 `—`，改输出 `未测`（`ui.css:145` 加 `.ring.is-idle .ring-num{font-size:var(--fs-14);color:var(--text-tertiary)}`）。
  - `ScoreRing.js:27`：`ring-grade` 文案保留「等待检测」，但色由 `--text-3` 升为 `--text-2`（11px 太小，见 P1-6）。

### P0-8 首屏命中 3 类"禁用词"（07 判定为"出现即不通过"）

- **现象**（都对得上《07》§1.4 的清单）：
  - 6 张卡底部固定渲染 `DNS / TCP / TLS / TTFB` 四格（`ProbeCard.js:44-49`）→ **TTFB 出现在 6 处**；
  - P3 次指标 `chatgpt.com/ · HTTP 200`、P5 `api.openai.com/v1/models · HTTP 401` → **裸状态码 2 处**；
  - 分项脚注 `P50 28.8 Mbps · 峰值 110.6 Mbps`（`score.go` 的 `speedDetail`）→ **P50 1 处**。
- **规范**：《07》§1.4「首屏禁用词（验收项，出现即不通过）：TTFB、CV、p90、p50、TLS 版本、证书 issuer、任何裸状态码数字」＋「状态码只能出现在详情层。这是硬边界：状态码是本产品的证据，不是本产品的语言」；决策 2「一张卡正面只允许一个主数字…**禁止正面塞表格**」。
- **改法**：
  - `ProbeCard.js:43-49` **删除 `probe-phases` 整块**（4 格 × 6 卡 = 24 个数字，是首屏最大的噪声源，也是截图里"像调试面板"的主因）。`ui.css:176-178` 的 `.probe-phases/.phase-cell` 一并删或留作抽屉复用。
  - 腾出的位置按 §3.3 补两样：**sparkline（28px，仅 P1/P2/P6）** 与 **footer（24px：左 13px mono 次指标｜右 `详情 ›`）**。
  - 裸状态码：`ProbeCard.js` 的 `r.subline` 由后端拼，改 `score.go`/`probe` 侧对首屏字段剥离 `HTTP \d{3}`，状态码只在 `DetailDrawer` 的「关键指标」出现（那里已经有 `HTTP 状态` 一行）。
  - 分项脚注：`score.go` 的 `speedDetail` 从 `P50 %.1f Mbps · 峰值 %.1f Mbps` 改为业务语义 `约 29 Mbps · 够开 1080p`（这条同时解 P1-1）。
- **备注**：这一条**部分根因在我的规范里**——`08` §3.2 的线框写了 `TTFB 312ms`、§3.3 的 P1 卡次指标写了 `平均 83.1 · p90 96.1 · TTFB 198ms`，实现是照着抄的。见 §4.3 的规范修订项。

### P0-9 结果态：分数说了 3 遍、档位说了 2 遍，档位字母 A–E 还缺席

- **现象**：环里 `94`、胶囊里 `综合 94 分 · 优秀`、结论句尾 `综合得分 94（优秀）` —— **同一信息 3 遍**；档位词 2 遍；而 PRD 要求的 **A–E 字母一次都没有**（`score.go:grade()` 只返回「优秀/良好/一般/较差/不可用」）。
- **规范**：§6.7「档位 A–E 必须显式可见，形态是『分数 + 档位词』…只给数字不给档位词 = 违反 PRD」（ux-arch 二轮硬约束）；§3.3 综合结论卡「分项摘要 14px mono」。
- **改法**：
  - `VerdictPanel.js:48` 胶囊改 `A · 优秀`（字母 + 词，`--r-sm` 描边胶囊，`12px/500`，色随档位：A `--success-text`、B `--brand-text`、C `--warning-text`、D/E `--danger-text`）。`ui.css:182-186` 的 `.badge` 圆角从 `--r-full` 改为 `--r-sm`。
  - `VerdictPanel.js:51` 结论句**删掉** `，综合得分 %.0f（%s）`（`score.go:verdict()` 末句），分数只由环承担。
  - `score.go:grade()` 补字母映射，并把词对齐 PRD：`A 优(85-100) / B 良(70-84) / C 可用(55-69) / D 较差(30-54) / E 不可用(0-29)`（当前 C 档叫「一般」、D 档从 40 起，都与 PRD 不一致）。
  - `ScoreRing.js:27` 的 `ring-grade` 改显示 `A 优`（与胶囊同源，别造两套文案）。

### P0-10 `done` 态配置卡没有折叠，1440×900 首屏必然溢出

- **现象**：结果态截图里，控制卡仍然是完整的（出口模式 Segmented + 输入框 + 强度 + 大按钮 + 提示行 + 出网行），实测占 **~130px**（spec 预算 188px，实现略省但仍是"整卡"）。加上结论卡 ~170px、探测项标题 ~40px、两行卡 ~344px、间距 —— 1440×900 装不下。
- **规范**：《07》§1.4「B 区 Config 在 `done` 态必须折叠为一行摘要（40px）…否则首屏必然溢出」；§3.7 `ConfigSummary`，且**出网方式是不可折叠项**。`08` §11.1 把「ConfigSummary」列为 P0。
- **改法**：`ControlPanel.js:97-142` 的 `render()` 增加 `st.phase==='done'` 分支：整卡替换为一行 `.config-summary`（`height:40px`），内容 = `系统代理 · TUN 或全局接管` ｜ `快速档` ｜ 右端 `修改配置 ›`；`.exit-note` 收进这一行（V4 必须同屏、非 hover 可见）。省下约 **90–110px**，正好补上溢出。
- **验收**：改完后在 1440×900 下滚轮位置为 0 时，`探测项` 第二行卡片的**主数字**必须在视口内（这是"首屏"的定义）。

### P0-11 六个绿胶囊徽章 = 满屏绿，把"受限"那张淹了

- **现象**：6 张卡右上角都是带底色的绿 `可用` 胶囊，P5 蓝、P2 琥珀。结果是"每张卡都在喊同一句话"，而唯一需要被看见的 **P2 受限** 反而被同质化。徽章还和下面的绿环、绿的「AI 可用」徽章叠成一片。
- **规范**：§4.4 定义了两个变体：`soft`（有底色有描边）与 **`dot`（仅 8px 圆点 + 文字，无底色无边框，`padding:0`）**，并注明「`dot` 变体用于卡片头，紧凑」。§1 原则 5 要求三态至少两条非颜色通道。
- **改法**：`ui.css` 增 `.probe .badge{background:transparent;border:0;padding:0;height:auto;gap:6px}`，即卡片头统一用 dot 变体；`StatusBadge.js:25` 增加第二参数 `variant`，`ProbeCard` 传 `'dot'`。结论区的徽章保留 `soft`。
- **效果**：一屏从 6 个色块降到 2 个色块（P2 琥珀 + P5 蓝），"受限"立刻显形。这条是**性价比最高的美观修复**之一（只改 CSS 一行 + 一个可选参数）。

---

## 2. P1 建议（影响可信度与专业感，本轮应改）

### P1-1 V3 缺"业务语义句"：只有 Mbps，没有"够干什么"

- **现象**：结论句写「平均 28.8 Mbps 但波动较大」，分项脚注写 `P50 28.8 Mbps · 峰值 110.6 Mbps`。用户不知道 28.8 是"够开 1080p"还是"只能聊天"。
- **规范**：《07》§1.4 V3「速度：『够用/勉强/不够』+ 一个数字，数字必须绑定业务语义：『够开 1080p 视频』而非『p50 42.6 Mbps』」；§3.3 卡次指标样例 `起播 · 分片 5.8Mbps`。
- **改法**：`score.go` 速度维度 detail 改为 `约 29 Mbps · 够开 1080p`（阈值：≥25 够 4K / ≥10 够 1080p / ≥4 够 720p / <4 仅聊天），并在 `verdict()` 的速度分句里带上同一句话；`Cap` 与 `advice` 复用同一映射，别各写一份。

### P1-2 分项条形现在是"数字的图形重述"，而且三根条同色

- **现象**：`88 / 98 / 98` 三个数字右边各一根 `--brand` 蓝条，长度 = 分数、颜色全一样。条形**没有提供任何数字之外的信息**，88 和 98 的条只差 10% 长度，肉眼几乎无差别——这就是你问的"是否冗余"。
- **规范**：§4.3「条形填充宽 = `score%`，**颜色 = 阈值色**：≥85 `--success-text`；70–84 `--brand`；50–69 `--warning-text`；<50 `--danger-text`」；§3.3 三分项卡「条 `6px` pill、轨道 `--line-subtle`」。
- **改法**（两步，缺一不可）：
  1. 色：`ui.css:135` `.dim-bar > i{background:var(--brand)}` → 由 JS 按阈值给 `data-tone="a|b|c|d"`，CSS 分别映射四个语义色。
  2. 信息：轨道上加 **4 个档位刻度**（`--line-strong`，1px 竖线，位于 30/55/70/85%），条的 `6px` 高度维持。这样条回答的是"离上一档还差多少"，而不是把数字再画一遍。
  - 若不想加刻度，**删掉条形**也比现在好：一个 24px 的分数 + 一行脚注已经足够，条形留着只会稀释。

### P1-3 「出网方式」现在是自由文本长句，V4 要的是枚举值

- **现象**（截图 A）：`出网方式：未检测到系统代理设置，按系统默认路由直接出网（TUN / 透明代理场景下属正常）` + 一段 `检测到环境变量代理 http://127.0.0.1:51717，系统模式下不会使用它（避免误收终端注入的代理），如需测试请切换到自定义代理`。整行 ~90 个汉字 + 一个 URL。
  - 注：`ui.css:107-115` 的 `.exit-warn{flex:1 0 100%}` 已经让警告独占一行（09:54 改的），**换行问题已修**，剩下的是"该不该这么长"。
- **规范**：《07》§1.4 V4「出网方式：**枚举值，非自由文本**」＋折叠行规格「`模式 + 出网方式枚举值`，例：`系统代理 · TUN 或全局接管`」；§2.4 决策「出网方式是**不可折叠项**…不靠 tooltip、不缩为图标」。
- **改法**：
  - `ControlPanel.js:133` 的 `.exit-main` 只放**枚举值**：`出网方式：直连（系统默认路由）` / `系统代理 · TUN 或全局接管` / `自定义 · socks5h://127.0.0.1:7890`。
  - `ControlPanel.js:135-137` 的 warnings 改为**结构化短句 + 可展开**：`⚠ 检测到终端注入的代理变量` + 右侧 `查看`（点击展开 URL 全文）。首行 `max-width` 限制到 ~52ch，`text-overflow: ellipsis`。
  - `ui.css:108-111` `.exit-warn`：`font-size:var(--fs-12)`（现在 11px）、`color:var(--warning-text)`（现在 `--warn` 偏亮，10.24:1 过亮，在暗底上发荧光）。

### P1-4 「复测波动 ±15 分内属正常」挂在结论旁，等于自己给结论打折扣

- **现象**：它是和 `综合 94 分 · 优秀` 并排的**同级胶囊**。读者接收到的潜台词是"这个 94 分随时会变成 79 分"。
- **规范**：§6.10 的 `DegradedFlag`/旁注规则——**免责与方法说明属于第三层（tertiary）**，不能与结论同级。
- **改法**：`VerdictPanel.js:49` 从 `.verdict-head` 移出，改为结论卡右下角的 `11px --text-tertiary` 一行（或塞进「本次探测方法」Tooltip）。`ui.css` 不需要新类，用 `.verdict-note{font-size:var(--fs-11);color:var(--text-3)}`。

### P1-5 卡片正面没有趋势、也没有"详情"入口

- **现象**：我拍的结果态里 6 张卡**一张 sparkline 都没有**（`ProbeCard.js:51` 要求 `samples.length>1`，而 `done` 帧的 result 不带 samples），也没有规范的 `详情 ›` footer；整张卡是"一个词 + 4 个更小的数字"。用户不知道可以点开。
- **规范**：§3.3 探测卡「Sparkline `28px`…Footer `24px`：左 13px mono、右 `详情 ›` `13px/500` `--text-secondary`，hover 变 `--brand-text`」；§4.5。
- **改法**：
  - footer：`ProbeCard.js` 末尾加 `.probe-foot`（左 `r.subline`，右 `详情 ›`），`ui.css` 增 `.probe-foot{display:flex;justify-content:space-between;align-items:center;margin-top:10px;padding-top:10px;border-top:1px solid var(--border);font-size:var(--fs-12);color:var(--text-3)}`。
  - sparkline：`ui.css:179` `.sparkline{height:34px}` → `28px`；并确保 `done` 帧携带降采样后的 `samples`（或让 P1 卡直接用 `p50/peak` 画 2 点线的最小占位，别留空）。

### P1-6 11px 是字号的默认值，全站没有字号阶梯

- **现象**：`.badge/.probe-id/.probe-sub/.phase-cell/.dim-detail/.field-hint/.ring-grade/.wf-label/.kv` 全是 `--fs-11`（11px）。11px 中文在 1x 屏上等于"注释的注释"，而它承载的是**分项脚注、阶段耗时、徽章文案**等一等信息。字号阶梯也不完整（缺 24/30/36，`--fs-28` 定义了但没人用）。
- **规范**：§2.8 字号阶梯与用途——`12/500` 徽章、`13` 次指标与表格、`11` 仅限 `micro`（全大写短标签，且不超过 3 个词）。
- **改法**：`ui.css` 逐条替换：`.badge`→`--fs-12`；`.probe-sub`→`--fs-12`；`.dim-detail`→`--fs-12`；`.phase-cell` 直接删（P0-8）；`.field-hint`→`--fs-12`；`.ring-grade`→`--fs-12`；`.wf-label`→`--fs-12`；`.kv`→`--fs-13`。保留 11px 的只留 `.probe-id`、`.brand-sub`、`.footer`（真·micro）。

### P1-7 瀑布图配色与规范不一致，且没有"主要耗时"标注

- **现象**：`ui.css:225-228` 是 `dns→chart-4`、`tcp→chart-1`、`tls→chart-3`、`ttfb→chart-2`。规范要求 DNS=chart-2、TCP=chart-1、TLS=chart-3、TTFB=chart-4、传输=chart-6（按序取用不得跳序），且**最耗时的一段必须有标注**。
- **规范**：§2.6「图表色序列（按序取用，不得跳序）」；§4.2「`totalSpan` 不做 per-row 归一化…当传输 >70% 时补一行 `11px` 提示」；§3.4 瀑布 `dl` 语义。
- **改法**：`ui.css:225-228` 改为 `.wf-bar.dns{--chart-2}.tcp{--chart-1}.tls{--chart-3}.ttfb{--chart-4}`，并补 `.wf-bar.tx{--chart-6}`；`DetailDrawer.js:55-59` 在最长的那根条右侧加 `主要耗时` 徽章；容器改 `<dl>`/`<dt>`/`<dd>`（现在全是 `div`，读屏拿不到键值关系）。
- **顺带**：`ui.css:223` `.wf-track{height:12px}` → `10px`（§3.4），`ui.css:221` 第三列 `64px` 在 11px 下有 `>9999ms` 溢出风险，改 `72px`。

### P1-8 三处装饰性渐变/发光，超出「唯一允许的渐变」

- **现象**：
  - `ui.css:32-33` `.brand-mark` = `linear-gradient(145deg,#5b8cff,#8f6bff)` + `box-shadow:0 4px 14px rgba(91,140,255,.35)` —— 蓝紫渐变 + 蓝色光晕，是"AI 产品"视觉符号，不是网络工具；
  - `ui.css:120` `.verdict` = `radial-gradient(120% 140% at 0% 0%, var(--brand-weak), transparent 55%)` —— 结论卡左上角一块品牌色晕；
  - `ui.css:58` `.btn-primary` 的 `box-shadow: 0 4px 16px rgba(...)`。
- **规范**：文件头「**唯一允许的渐变**是速度曲线下的面积填充，其余一律纯色」；§2.11 阴影只有 `--shadow-sm/md` 三档，主按钮默认态不带投影。
- **改法**：`.brand-mark{background:var(--brand);box-shadow:none;border-radius:var(--r-sm)}`（16px 图标改 `--brand-ink` 描边色）；`.verdict{background:var(--surface)}`；`.btn-primary{box-shadow:none}`，hover 只换 `--brand-hover`。

### P1-9 圆角整体偏大一档，卡片高度不固定导致两行不齐

- **现象**：`--r-md:10 / --r-lg:14 / --r-xl:20`；`.verdict` 用 `--r-xl`(20px)，`.card` 用 `--r-lg`(14px)。20px 圆角 + 24px padding 在 1440 下显得"圆润/消费级"，与"专业诊断"不符。卡高由内容撑开，P2 卡（无 subline 时）与 P3 卡高度不同 → 网格行内错位。
- **规范**：§2.10 圆角阶梯 `--r-sm 6 / --r-md 8 / --r-lg 12 / --r-xl 16`；§3.3「卡高 `172px` 固定（不因内容撑高，保证网格对齐）」。
- **改法**：`tokens.css:54-58` 改 `--r-md:8px; --r-lg:12px; --r-xl:16px`；`ui.css:158-161` `.probe{min-height:172px;display:flex;flex-direction:column}`，并让 `.probe-foot` `margin-top:auto`。

### P1-10 运行中用的是"扫光"动画，规范要的是脉冲点 + 描边 + 微光

- **现象**：`ui.css:164-169` `.probe.is-running::after` 一条 1.6s 的线性渐变横扫整卡。视觉上像"加载条在卡里乱窜"，且 6 张卡同时扫 = 屏幕一直在动，违反"克制"。
- **规范**：§3.2「运行中卡片：`border-color: var(--brand-line)`；`box-shadow: var(--glow-running)`；徽章为 `running`（脉冲点）」；§8「只有曲线、进度、脉冲承担动效」。
- **改法**：删 `ui.css:164-169` 的 `::after` 与 `@keyframes sweep`；`.probe.is-running{border-color:var(--brand-ink);box-shadow:0 0 0 1px var(--brand-weak)}`；脉冲只放在 `StatusBadge` 的 6px 圆点上（`animation: pulse 1600ms ease-in-out infinite`，`prefers-reduced-motion` 下停）。

---

## 3. P2 可选（打磨项，不阻塞发布）

### P2-1 内容最大宽 1440，超宽屏下结论卡被拉成"横幅"

- **现象**：`.app-shell{max-width:1440px}`（`ui.css:20`）。在 2006 宽下，结论卡内容宽 1376px，环在左、三行文字在右，中间大片空白；分项三卡各 440px 但内容只占左侧 1/3。
- **规范**：§2.13 `--shell-max:1360`，且 bp-xl(≥1920) 要求「左栏 340 结论 + 右栏 2 列网格」而不是整宽三列。
- **改法**：`ui.css:20` 改 `max-width:1360px`；≥1920 增加一条 `@media (min-width:1920px){ .app-shell{max-width:1980px} .verdict{grid-column:1} .probe-grid{grid-template-columns:repeat(2,minmax(0,1fr))} }`（真要按 bp-xl 做才加，否则**只改 1360 就够**）。

### P2-2 断点是 1240 / 860，与规范的 1280 / 1440 / 1920 无关

- **现象**：`ui.css:152` `(max-width:1240px)` → 2 列；`ui.css:153` `(max-width:860px)` → 1 列 + 收窄 padding。规范里 960 是**最小窗口**（不是换列点），低于 960 应出提示条而不是重排。
- **规范**：§2.13 断点表 + 「最小窗口 960×640：`<960px` 或 `<640px` 高不压缩布局，改插一条 40px 提示条」。
- **改法**：`ui.css:152` 改 `(max-width:1279px)`；`ui.css:153` 整块改为 `@media (max-width:959px){ .size-warning{display:flex} }`，并在 `main.js` 顶栏下插入 `.size-warning`（`--warning-bg/--warning-text/40px`）。860 的 1 列重排删除。

### P2-3 焦点环用 `outline`，且没有 `prefers-contrast`

- **现象**：`ui.css:55` `.btn:focus-visible{outline:2px solid var(--brand);outline-offset:2px}` —— `outline` 不跟随 `border-radius`，在 `--r-md` 10px 的按钮上会画出方角；`.input:focus` 用的是 `box-shadow:0 0 0 3px var(--brand-weak)`（品牌色 12% 透明 ≪3:1）。全站无 `prefers-contrast: more`。
- **规范**：§10.2「焦点环用**两层 `box-shadow`** 而非 `outline`，以保留圆角：`0 0 0 2px var(--surface), 0 0 0 4px var(--line-focus)`」；§10.4 要求 `prefers-contrast`。
- **改法**：统一 `:focus-visible{outline:none;box-shadow:0 0 0 2px var(--surface),0 0 0 4px var(--brand)}`；`.input:focus` 同款；补 `@media (prefers-contrast:more){ :root{--text-3:#A3ABB8;--border:#3A414D;--line-control:#8A93A2} }`。

### P2-4 探测项标题右侧的「19.9s」是裸数字

- **现象**：`main.js:33` 的 `runClock` 显示 `19.9s`，无标签、无单位说明。用户要猜这是"总耗时"还是"剩余"。
- **规范**：§3.2 状态行要求「`4/6 完成 · 预计剩余 12s`」这类**带语义**的时间表达。
- **改法**：`main.js` 文案改 `总耗时 19.9s`（`--text-tertiary` 的 label + `--text-secondary` 的值），或直接改成 `6/6 项完成 · 19.9s`。

### P2-5 浅色主题有 4 处不达标的 token（按计算，无截图）

未拿到浅色截图，以下按 `tokens.css` 的 `[data-theme="light"]` 计算：

| 组合 | 实现 | 实测 | 规范 | 判定 |
|---|---|---|---|---|
| `--text-3` on `#fff` | `#8b93a1` | **3.09:1** | `#666E7A` 5.15:1 | 不达标（11px 全站） |
| `--border` on `#fff` | `#e3e6ec` | **1.25:1** | 控件需 ≥3:1 | 控件边界消失 |
| `--border-strong` on `#fff` | `#cfd5df` | **1.48:1** | `--line-control #838B99` 3.43:1 | 同上 |
| `--ok` on `#fff`（徽章文字 11px） | `#0f9d6e` | **3.46:1** | `#15682D` 5.63:1 | 不达标 |
| `--warn` on `#fff` | `#b7791f` | **3.64:1** | `#7D5300` 6.00:1 | 不达标 |

- **改法**：`tokens.css:88-110` 按规范 §2.2/§2.5 的 light 列整体替换（差异不止上面 5 行，建议整段复制 `08` §2.14 的 light 块）。**这条在发布前必须做**——浅色主题是"想切过去看看"的第一动作，一进去字发灰会立刻失去信任。

### P2-6 卡片上的 P-ID 顺序是 3,4,5,6,2,1

- **现象**：`ProbeGrid.js:4` `ORDER=['P3','P4','P5','P6','P2','P1']`。按重要性排（AI 三项优先）是对的，但**编号是乱的**：第二行以 P6 开头、P1 收尾，读者会以为"列表没排完/有遗漏"。
- **规范**：§3.3 六卡网格顺序为 `P1…P6`；若要"AI 优先"，就得让编号跟着视觉顺序走。
- **改法**：二选一，推荐 (a)：**(a)** 卡片上**不显示 P-ID**（`ProbeCard.js:33` 删 `probe-id`，ID 只留在抽屉标题 `P3 · ChatGPT 网页` 里）；**(b)** 把引擎侧 ID 改成按展示顺序编号（改动面大，不建议）。

### P2-7 图标是"字符"不是图形：◐ ⚙ ⚠ → ○ ◍ ✓ ◐ !

- **现象**：`TopBar.js:14` `'◐ 主题'`、`:35` `'⚙ 设置'`、`DetailDrawer.js:19` `'关闭 ✕'`、`StatusBadge.js:4-16` 用 `○ ◍ ✓ ◐ ! ? ~ ⚠ ⊘ ✕ –`、`ControlPanel.js:136` `'⚠ ' + w`、`advice-item::before{'→'}`。这些字符在不同字体/系统下**基线、字重、视觉尺寸全不同**（截图里 ⚙ 和 ◐ 明显一大一小），是"不精致"的直接来源。
- **规范**：§7 要求 19 个 `24` 网格、`1.5px` 描边的 SVG sprite（`<symbol>` + `<use>`），并给三态专用图形 `#i-solid-dot` / `#i-ring-gap` / 叉。
- **改法**：按 `08` §7 建 `sprite.svg`（或内联 `<svg style="display:none">`），`TopBar/StatusBadge/ControlPanel` 用 `<use href="#i-theme">` 等替换字符；`StatusBadge` 的三态直接用 `#i-solid-dot / #i-ring-gap / #i-cross`（这一步同时完成"三态不能红绿二元"的验收，见 §4.1）。

### P2-8 顶栏副标题是英文 mono，占了一行却没提供信息

- **现象**：`system proxy capability probe` 11px mono 灰字。中文产品首屏出现一行英文小字，读起来像模板残留。
- **规范**：§3.0 全局骨架的顶栏只允许「品牌 + 环境 Pill + 操作区」；V1–V4 都不含副标题。
- **改法**：`TopBar.js:30` 删除 `.brand-sub`，该位置改放**环境 Pill**（`macOS · Chrome 141`，`12px`，`--surface-raised` 底），这是 §6.10 要求的"环境明细"，比英文副标题有用。

---

## 4. 规范 vs 实现偏差表

### 4.1 规范写了、实现没有（按影响排序）

| # | 规范条款 | 要求 | 实现现状 | 影响 |
|---|---|---|---|---|
| 1 | §6.3 A 轴三态 | 三态 = `#i-solid-dot`（实心点）/ `#i-ring-gap`（缺口环）/ 叉，颜色最多占 1 通道 | `StatusBadge` 全是"6px 圆点 + 文字"，三态只靠颜色区分 | **红绿二元 + 灰度不可辨**，ux-arch 的验收项直接不过 |
| 2 | §6.2 Verdict 九态 | 九态 + `data-verdict` | 只有 `usable/limited/blocked` 三态 + `data-status`；`region_blocked`/`challenge`/`degraded` 三个不同语义**全映射到 `--warn` 一个颜色** | 六种受限原因视觉上不可区分 |
| 3 | §6.7 档位 A–E | `A 优 / B 良 / C 可用 / D 较差 / E 不可用`，`分数 + 档位词` | 只有「优秀/良好/一般/较差/不可用」，**无字母**；C 档词是"一般"；D 档门槛 40 而 PRD 是 30 | PRD 硬约束未落地 |
| 4 | §6.5 NET_FAIL 六子原因 | 六条中文文案 + 各自唯一动作（`tls_cert` 只给「查看证书详情」） | 未实现，`failed` 统一「不可达」 | 失败时用户拿不到下一步 |
| 5 | §6.8 FailureAttribution | 五类 `party`，chip `min-width:96px`，必填无"未知" | `DetailDrawer.js:5-10` 只有 4 类，且用普通 badge | "网络运营商"缺失，排版未按最长项设计 |
| 6 | §6.6 SKIPPED 卡形态 | 虚线 + 去饱和 + `本次未测`，且与骨架可区分 | 只有 `skipped→'未执行'` 灰徽章 | 跳过项看起来像"没跑完" |
| 7 | §6.9 四种进度形态 | 全局 / 单项 / 阶段 / 未知 | 只有 `.progress-track`（2px）且 `main.js` 没使用 | 运行中无全局进度 |
| 8 | §4.1 速度曲线 | 首屏 Canvas 曲线 + `role="img"` + `aria-label` +「以文本查看」 | 曲线只在抽屉；首屏 0 个数据可视化 | 首屏缺最核心的"证据图" |
| 9 | §3.5 S5 设置 / §3.6 S6 导出预览 | 两屏完整规格 | 设置是简易 sheet，导出直接 `window.open` | 未验收（无截图） |
| 10 | §3.9 S7 截图摘要 | 1280×800 一屏不滚动的报告版 | 未实现 | "分享给同事"路径缺失 |
| 11 | §2.9 间距阶梯 | 4 的倍数，到 `--sp-16` | 缺 `28/36/48/64`，自造了 `--sp-10:40` | 影响小，补齐即可 |
| 12 | §2.12 动效 | `80/140/200/280/420ms`，`--ease-out cubic-bezier(.16,1,.30,1)` | `120/200/320ms`，`cubic-bezier(.22,.61,.36,1)` | 动效偏"拖"，收不到"利落" |

### 4.2 实现有、超出规范（需决定去留）

| # | 实现 | 位置 | 判断 |
|---|---|---|---|
| 1 | 整页 48px 网格底纹 | `ui.css:10-15` | **删**（P0-1）：制造"线框图"观感，与 1px 描边层级冲突 |
| 2 | logo 蓝紫渐变 + 光晕 | `ui.css:32-33` | **删**（P1-8） |
| 3 | 结论卡左上角品牌色晕 | `ui.css:120` | **删**（P1-8） |
| 4 | 主按钮投影 | `ui.css:58` | **删**（P1-8） |
| 5 | 运行中"扫光"动画 | `ui.css:164-169` | **删**（P1-10） |
| 6 | 卡片底部 4 格阶段耗时 | `ProbeCard.js:44-49` | **删**（P0-8，命中禁用词，信息下沉抽屉） |
| 7 | 「综合 N 分 · 档位」+「复测波动」两个同级胶囊 | `VerdictPanel.js:48-49` | **改**（P0-9 / P1-4） |
| 8 | 抽屉 `backdrop-filter: blur(3px)` + 头部 `blur(8px)` | `ui.css:200,213` | **留，降为 2px**。遮罩层轻度毛玻璃可接受，规范只禁"玻璃拟态过度" |
| 9 | 分项脚注用 mono | `ui.css:136` | **改** mono→sans：中文句子等宽很难看，只有纯数字串才该 mono |
| 10 | `.dim` 卡片（raised 底 + 描边 + 4px 条） | `ui.css:130-136` | **留但调**：条 6px + 阈值色 + 档位刻度（P1-2） |

### 4.3 规范本身需要修订（我认的错，共 5 条）

| # | 问题 | 原文 | 修订为 |
|---|---|---|---|
| 1 | **我自己在卡片上放了禁用词** | §3.2 线框 `TTFB 312ms`；§3.3 P1 卡次指标 `平均 83.1 · p90 96.1 · TTFB 198ms` | 卡片**只允许** `Mbps` + 业务语义句；`p90/TTFB` 一律下沉抽屉。实现是照抄这两处，P0-8 一半根因在我 |
| 2 | 断点在同一份文件里矛盾 | §2.13 说 2 列在 `1024–1279`，§3.3 表写「1200px 以下改 2 列」 | 统一为 **1280**（`≤1279 = 2 列`），全文替换 |
| 3 | 分项阈值与 PRD 档位不一致 | §4.3 阈值 `≥85 / 70–84 / 50–69 / <50`（四档） | 对齐 PRD 五档：`≥85 A / 70–84 B / 55–69 C / 30–54 D / <30 E`（"50"改"55"，补 30） |
| 4 | 未定义"无分数时环的形态" | §3.3 只写「`--fs-score` 44/600 + `/100`」 | 补「无分数：不画进度弧、不显示 `—`；环心 `未测`（`12px/500 --text-tertiary`）+ `等待检测`」。否则实现只能自造（于是出现 P0-7 的绿点） |
| 5 | 缺"阶段耗时不得上卡"与"有结论无采样时 sparkline 画什么" | §3.3 有 footer，但没说阶段行可否上卡；§4.5 只定义"无数据 → 虚线 + 无数据" | 明确：阶段耗时**不得**上卡；卡 footer 只放 1 条次指标 + `详情 ›`；"有结论无采样"时画最后 2 个已知值的 2 点线 + 虚线尾段 |

---

## 5. 你点名的 5 个疑问，逐条判断

### 5.1 「AI 可用性」那行的浅黄底色 —— 是 `.dim-detail` 串样式吗？

**不是。** `.dim-detail`（`ui.css:136`）只有 `font-size / color:var(--text-3) / mono / ellipsis`，没有任何语义色分支，也没有 `nth-child` 之类。

那抹浅黄的真实来源是 **`VerdictPanel.js:60` 的内联样式行**：

```js
(s.caps && s.caps.length)
  ? h('div', { class:'verdict-text', style:'color:var(--warn);font-size:12px' }, '⚠ ' + s.caps.join('；'))
  : null
```

`s.caps` 是后端封顶规则的提示（如「AI 分封顶 60」「总分封顶 55」）。它**紧贴在三张分项卡下方**、又是 12px 琥珀色，视觉上就像第三张卡"被染黄了"。09:43 那张截图触发了 caps，09:52 那张没触发——黄色也随之消失，**这印证了它是数据驱动，不是样式串**。

**判断：位置错 + 内联色，两个问题叠加，不是串样式。**

改法（三步）：
1. `VerdictPanel.js:60` 删掉内联 `style`，改 `class:'degraded-flag'`。
2. `ui.css` 增：
   ```css
   .degraded-flag{display:flex;gap:8px;align-items:flex-start;margin-top:12px;padding:8px 12px;
     background:var(--warn-bg);border:1px solid var(--warn-border);border-left:3px solid var(--warn);
     border-radius:var(--r-sm);font-size:var(--fs-13);color:var(--warn-text)}
   ```
3. 位置从"分项卡下面"移到结论卡**底部、操作区之上**，冠一个 11px 的 `封顶说明` label——它是"分数的解释"，不是"分项之一"。

顺带：`--warn #f5b544` 在暗底上是 **10.24:1**，亮到发光，小字上尤其刺眼；规范用的是更暗一档的 `--warning-text`。**颜色与位置都要换。**

### 5.2 分项分数右边的大数字（86/93/98）和进度条右侧数字重复 —— 冗余吗？

**数字不冗余，条形冗余。**

「数字 + 条形」这个组合本身是对的（规范 §4.3 明确"两种同源"）：数字给精度，条形给**档位感**。但现在条形做了三件错事，退化成"把数字再画一遍"：

1. **没有刻度**：看不出 88 和 98 差在哪（视觉上都"几乎满"）；
2. **没有阈值色**：三根条全是品牌蓝，88 与 98 颜色相同，"档位"信息量为 0；
3. **条太细（4px）+ 轨道对比低**：视觉重量不如数字，读者会自动忽略。

**结论：保留数字，改造条形（或删掉）。**

- **方案 A（推荐，~15 行）**：条高 `4px → 6px`；轨道 `--border`；填充按阈值上色（≥85 绿 / 70–84 品牌 / 55–69 琥珀 / <30 红）；轨道上加 4 个 1px 竖刻度（30/55/70/85%，`--line-strong`）；分数字号 `16 → 24`（`--fs-metric-lg`），让它成为这张卡的唯一主数字。
- **方案 B（最省）**：删掉 `.dim-bar`，只留 `88` + 脚注。页面立刻安静，但失去"离 A 档还差多少"的表达，且分项区会变成三行纯文字、与下方 6 卡的重心打架。

**选 A。**

### 5.3 「平均 24.4 Mbps 但波动较大…综合得分 92（优秀）」 —— 矛盾吗？

**在 09:52 那张截图里是矛盾的，而且是最伤信任的一句。但代码已在 09:56 修好。** 我核了 `score.go:260-268`：

```go
} else if p1.CV > 0.5 {
    // 语气必须服从裁定：usable 时不能说"忽快忽慢"，否则与"总分优秀"直接打架
    if state == "usable" {
        b.WriteString("；速度有起伏（平均 %.1f / 峰值 %.1f Mbps），日常够用但大文件耗时不稳")
    } else {
        b.WriteString("；平均 %.1f Mbps 但波动较大，体感会忽快忽慢")
    }
}
```

注释里那句"语气必须服从裁定"就是对的。**这条不用再改**——但还有两个遗留：

1. **一句话里塞了两件事**（结论 + 风险），读者仍会觉得"一句在打自己"。
   改法：拆成两槽位——
   - 结论行（`--fs-20/600 --text-primary`）：`可以正常使用 AI 服务，带宽属于"够开 1080p"。`
   - 风险行（`--fs-13 --text-secondary` + 16px `alert-triangle --warning-text`）：`速度有起伏（CV 0.52）：大文件下载耗时不稳定。`
   这正是规范 §3.3 的「结论句 + 风险提示行」，实现把它们压成了一行。
2. **句尾的「综合得分 92（优秀）」必须删**——分数在环里、档位在胶囊里，句尾是第三次重复（P0-9）。

### 5.4 「出网方式」那行长黄字 —— 截断、换行，还是折叠？

**都不是。先改内容，再谈排版。**

这句话之所以长，是因为它把**三件不同层级的事**串成了一句：①当前出网枚举值 ②方法学解释（"TUN/透明代理场景下属正常"）③一条环境变量警告（含 URL）。

| 动作 | 具体做法 | 理由 |
|---|---|---|
| **① 内容分层（必做）** | `.exit-main` 只留**枚举值**：`出网方式：直连（系统默认路由）`。方法学解释移进 `?` Tooltip。警告改为独立短句 `⚠ 检测到终端注入的代理变量` + `查看` | 《07》§1.4 V4「枚举值，非自由文本」 |
| **② 警告换行（已做）** | `ui.css:108` `.exit-warn{flex:1 0 100%}` 已独占一行（09:54 改的） | 换行问题已修 |
| **③ 截断（部分）** | 枚举行 `max-width:52ch; overflow:hidden; text-overflow:ellipsis`；URL 只在展开/`title` 中出现 | URL 是**证据**，不该是首屏语言（同"状态码只在详情层"） |
| **④ 折叠（不做）** | 不要把整行折成按钮 | V4 是"不可折叠项"：结论与出网方式必须同屏对照 |

一句话：**这行的问题不是太长，是"一句话承担了三个层级"。** 改完枚举值后长度自然降到 20 字以内，不需要折叠。

### 5.5 初始态「探测项」标题下没有任何卡片 —— 应该有 6 个骨架吗？

**不该是骨架，该是 6 张"预告卡"。**

理由是**语义**，不是审美：

| | 骨架屏的含义 | 预告卡的含义 |
|---|---|---|
| 说给用户的话 | "内容正在加载，请等" | "接下来会测这 6 项，点开始就跑" |
| 触发条件 | 数据在路上 | 数据**不存在**，且要用户动作才产生 |
| 违反后果 | 用户会等，等不到 → 认为坏了 | 用户知道下一步按哪个按钮 |

初始态属于第二种：**没有任何数据在路上**，用户还必须先按「开始探测」。给骨架等于承诺一件不会发生的事。这与规范 §6.6 单列「`SKIPPED` 与加载中骨架必须可区分」是同一条逻辑——**"没测"和"正在测"不能长得一样**。

再看实际伤害：1440×900 下 `探测项` 在 y≈410、页脚 y≈475，**下方 440px 全空（约 45% 首屏）**。这不是留白，是"页面没做完"的观感。

改法（即 P0-6）：

```js
// ProbeGrid.js —— 插在现有 running 分支之前
if (!results.length && !running) {
  clear(el); cards.clear();
  el.classList.add('is-preview');
  for (const id of ORDER) {
    el.appendChild(h('div', { class: 'probe-preview' }, nameOf(id)));
  }
  return;
}
el.classList.remove('is-preview');
```

```css
/* ui.css */
.probe-preview{height:52px;padding:0 12px;display:flex;align-items:center;gap:8px;
  background:var(--surface);border:1px dashed var(--border);border-radius:var(--r-md);
  font-size:13px;font-weight:500;color:var(--text-2)}
```

三个细节别漏：
1. **虚线边框**——一眼区分"预告"（虚线，未成事实）与"结果"（实线，已成事实）；
2. **不可聚焦、不响应 hover**——它们不是按钮，只是清单；
3. **标题文案改「将测试以下 6 项」**（`main.js:32`），别只留一个「探测项」。

---

## 6. 一句话结论

**现在的界面在"能看懂"这一层已经过关（信息齐全、层级大体正确、深色底不掉价），但在"像一件完成品"这一层还差一次减法**：删掉整页网格底纹与四处装饰渐变/光效，把品牌色从蓝换回青绿，把 11px 灰字提到 4.5:1、控件边界提到 3:1，拿掉卡片上那 24 个阶段数字、把 6 个绿胶囊降成 6 个圆点 —— **这五处全是改 CSS / 删代码，一天可完**，做完就从"内部工具"跨到"能拿给用户看的产品"；剩下的 P1（分项条形改造、V3 业务语义句、ConfigSummary 折叠、sparkline + 详情入口）是"专业感"的最后一公里，约 2 天。

**只有一天时的顺序**：P0-1 删格纹 → P0-2+P0-3 换品牌色与按钮 ink → P0-4+P0-5 提两档对比度 → P0-11 徽章降噪 → P0-8 删阶段行 → P0-6 预告卡。这 6 步的代码量加起来不到 80 行，截图的"完成度"会跳一个档。
