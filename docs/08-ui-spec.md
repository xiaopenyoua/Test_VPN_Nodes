# ProxyLens 视觉规范与关键界面规格（UI Spec）

> 版本：v1.0 ｜ 作者：UI Designer ｜ 状态：可交付实现
> 上游依赖：`00-task-brief.md`（需求基线）、`07-ux-interaction.md`（页面流与状态机；本文件按其占位接口设计——状态机冲突以交互文档为准，视觉以本文件为准）
> 实现前提：原生 ES Module + CSS 变量，无 React / Tailwind / 构建链；单页本地运行；主打 1920×1080，最小 1024×700。
> 本文件所有数值均为可直接写入 CSS 的确定值。**唯一允许的渐变**是全站速度曲线下方的垂直面积填充（见 §4.1），其余一律纯色。

---

## 1. 设计原则

1. **结论先行，数字居中**：任何屏幕首要回答"能不能顺畅用 AI"；评分与一句话结论永远在最上方，证据与原始 JSON 向下折叠。
2. **一个强调色**：品牌青绿 `#21BFAE` 只用于主 CTA、当前选中、运行中脉冲、主曲线。语义状态色只出现在状态位，不做装饰。
3. **靠描边而非明度分层**：深色四层表面亮度差极小（surface/canvas = 1.05:1），层级由 1px 描边 + 微阴影 + 留白承担，避免"色块仪表盘"。
4. **数字不跳动**：实时数值用等宽 + `tabular-nums`，数值更新不做过渡动画；只有曲线、进度、脉冲承担动效，规避每秒多次重排导致的视觉抖动。
5. **颜色永不独自承载语义**：每个状态 = 颜色 + 图标形状 + 文字标签，色盲与灰度截图下仍可读。

---

## 2. Design Token

### 2.1 背景层级

四层背景亮度差本身极小（刻意克制），层级由描边承担。"比值"为该色与 `--canvas` 的对比度，仅用于确认层级顺序。

| Token | Dark | 比值 | Light | 用途 |
|---|---|---|---|---|
| `--canvas` | `#0A0B0D` | 1.00 | `#FAFAFB` | 页面最底层 |
| `--surface` | `#101216` | 1.05 | `#FFFFFF` | 卡片、面板、表格体 |
| `--surface-raised` | `#15181D` | 1.11 | `#F4F5F7` | 卡片内嵌区、表头、代码块外框 |
| `--surface-overlay` | `#1B1F26` | 1.19 | `#FFFFFF` + 阴影 | 抽屉、Popover、Tooltip、Modal |
| `--surface-inset` | `#0C0E12` | 1.02 | `#F7F8FA` | 输入框内陷底、日志视图 |
| `--surface-hover` | `#191D24` | 1.16 | `#F1F2F4` | 行/卡 hover |
| `--surface-active` | `#21262F` | 1.30 | `#E9EBEF` | 按下态、行选中 |

### 2.2 文字层级（右侧为在 `--surface` 上的实测对比度）

| Token | Dark | 对比度 | Light | 对比度 | 用途 |
|---|---|---|---|---|---|
| `--text-primary` | `#E6E9EF` | **15.41:1** | `#14161A` | **18.11:1** | 标题、数值、正文主体 |
| `--text-secondary` | `#A3ABB8` | **8.10:1** | `#4A5058` | **8.14:1** | 说明、副标题、表格次列 |
| `--text-tertiary` | `#7E8794` | **5.16:1** | `#666E7A` | **5.15:1** | 单位、时间戳、脚注、占位符 |
| `--text-disabled` | `#525A67` | 2.69:1 | `#9AA1AC` | 2.60:1 | 仅禁用控件（非信息性，豁免） |
| `--text-inverse` | `#0A0B0D` | — | `#FFFFFF` | — | 落在品牌/语义实心块上的文字 |

> `--text-tertiary` 在 dark 的 surface/raised 上分别为 5.16:1 / 4.90:1，均 ≥4.5:1。禁止再调低；需要更弱视觉时用 `--text-disabled`，但该文字不得承载必需信息。

### 2.3 描边

| Token | Dark | Light | 用途 |
|---|---|---|---|
| `--line-subtle` | `#1C2027` | `#ECEDF0` | 分隔线、表格横线、图表网格 |
| `--line` | `#272C35` | `#D9DCE2` | 卡片/面板默认描边（装饰性） |
| `--line-strong` | `#3A414D` | `#B4BAC6` | 强调描边、分段控件外框 |
| `--line-control` | `#5C6470` | `#838B99` | **交互控件边界**（输入框、勾选框、开关轨） |
| `--line-focus` | `#21BFAE` | `#0F766E` | 焦点环 |

**关键约束**：`--line` / `--line-strong` 是装饰性描边，不可用于标识控件边界。凡"用户需靠它认出这是可操作控件"的边界必须用 `--line-control`。实测：dark `#5C6470` on `#101216` = **3.14:1**；light `#838B99` on `#FFFFFF` = **3.43:1**，均满足 WCAG 1.4.11 的 3:1。

### 2.4 品牌色

| Token | Dark | Light | 用途 |
|---|---|---|---|
| `--brand` | `#21BFAE` | `#0F766E` | 主 CTA 填充、运行中、主曲线、焦点环 |
| `--brand-hover` | `#2ED4C1` | `#0D6A62` | hover |
| `--brand-active` | `#17A091` | `#0A5A53` | active |
| `--brand-ink` | `#04130F` | `#FFFFFF` | 品牌实心块上的文字（dark `#04130F`/`#21BFAE` = **8.26:1**；light `#FFFFFF`/`#0F766E` = **5.47:1**） |
| `--brand-soft` | `#122728` | `#E7F1F0` | 选中行底、品牌淡底 |
| `--brand-line` | `#154947` | `#BCD9D6` | 品牌淡底上的描边 |
| `--brand-text` | `#21BFAE` | `#0F766E` | 品牌色文字（8.15:1 / 5.47:1） |

> dark 品牌用"青绿底 + 近黑字"（8.26:1），light 用"深青绿底 + 白字"（5.47:1）。**不要**在暗色用白字压 `#21BFAE`（仅 2.30:1，不合格）。

### 2.5 语义色（text / bg / border 三档）

Dark（底为 `--surface` `#101216`）：

| 语义 | `-text` | 对 surface | `-bg`（12% 叠底） | text on bg | `-border`（32% 叠底） |
|---|---|---|---|---|---|
| success | `#3FB950` | **7.38:1** | `#16261D` | **6.22:1** | `#1F4729` |
| warning | `#E0A63C` | **8.65:1** | `#29241B` | **7.11:1** | `#534122` |
| danger | `#F2555A` | **5.56:1** | `#2B1A1E` | **4.90:1** | `#58272C` |
| info | `#6EA8FE` | **7.76:1** | `#1B2432` | **6.46:1** | `#2E4260` |

Light（底为 `#FFFFFF`）：

| 语义 | `-text` | 对 surface | `-bg`（10% 叠底） | text on bg | `-border`（28% 叠底） |
|---|---|---|---|---|---|
| success | `#15682D` | **6.52:1** | `#E8F0EA` | **5.75:1** | `#BDD5C4` |
| warning | `#7D5300` | **5.63:1** | `#F2EEE6` | **5.63:1** | `#DBCFB8` |
| danger | `#A81E17` | **5.97:1** | `#F6E8E8` | **5.97:1** | `#E7C0BE` |
| info | `#1449A8` | **6.77:1** | `#E8EDF6` | **6.77:1** | `#BDCCE7` |

> 语义底 `-bg` 只用于徽章、结论卡、行内提示；单屏同时出现的语义底色**不超过 2 处**。

### 2.6 图表色序列（按序取用，不得跳序）

| # | Dark | on surface | Light | 语义建议 |
|---|---|---|---|---|
| c1 | `#21BFAE` | 8.15:1 | `#0F766E` | 主序列 / 当前代理 |
| c2 | `#6EA8FE` | 7.76:1 | `#2563EB` | 对照序列 / 直连基线 |
| c3 | `#A78BFA` | 6.20:1 | `#7C3AED` | 第三序列（多节点对比） |
| c4 | `#F0B429` | 8.92:1 | `#B45309` | 第四序列 |
| c5 | `#F0716B` | 5.30:1 | `#BE123C` | 第五序列 |
| c6 | `#8FD14F` | 9.10:1 | `#4D7C0F` | 第六序列 |

排序同时拉开**亮度**，保证红绿色盲下靠明暗仍可区分。超过 6 条曲线不再增色，改用实线/虚线/点线三种线型复用 c1–c6。

### 2.7 字体

```css
--font-sans: -apple-system, BlinkMacSystemFont, "Segoe UI Variable Text", "Segoe UI",
             "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei UI", "Microsoft YaHei",
             "Noto Sans SC", system-ui, sans-serif;
--font-mono: "SF Mono", SFMono-Regular, ui-monospace, "JetBrains Mono",
             "Cascadia Mono", "Roboto Mono", Menlo, Consolas, monospace;
```

- **中文**：`PingFang SC`（macOS）/ `Microsoft YaHei UI`（Windows）优先，缺失回落 `Noto Sans SC` / `system-ui`。
- **Latin 与数字**：`-apple-system` / `Segoe UI Variable` 排在中文族之前，让 SF Pro / Segoe 处理西文与数字，中文由 PingFang / YaHei 兜底（避免中文族自带的西文字形）。
- **数字一律等宽**：指标、时间、字节、百分比、IP、JSON 全部用 `--font-mono` + `font-variant-numeric: tabular-nums; font-feature-settings: "tnum" 1, "zero" 1;`。`zero`（斜杠零）在代理地址、端口、状态码处强制开启。
- 无 webfont、不联网加载字体（离线单文件要求）。
- 字距：正文 `0`；`--fs-micro` 全大写标签 `0.06em`；≥24px 的数字用 `-0.02em` 收紧。

### 2.8 字号阶梯

| Token | size / line-height | weight | letter-spacing | 用途 |
|---|---|---|---|---|
| `--fs-score` | 44 / 48 | 600 mono | -0.02em | 总分环中心数字 |
| `--fs-display` | 32 / 40 | 600 mono | -0.02em | 详情页关键指标 |
| `--fs-h1` | 24 / 32 | 600 | -0.01em | 页面主标题 |
| `--fs-h2` | 19 / 28 | 600 | 0 | 区块标题（"分项评分"） |
| `--fs-h3` | 15 / 22 | 600 | 0 | 卡片标题（"Google 大文件下载"） |
| `--fs-body` | 14 / 22 | 400 | 0 | 正文、按钮、输入框、表格单元 |
| `--fs-body-strong` | 14 / 22 | 500 | 0 | 表格首列、结论句 |
| `--fs-small` | 13 / 20 | 400 | 0 | 说明文字、次列、日志 |
| `--fs-caption` | 12 / 16 | 500 | 0.01em | 徽章、字段标签、坐标轴标注 |
| `--fs-micro` | 11 / 14 | 600 | 0.06em | 全大写小标题（`TTFB`、`P50`）、单位 |
| `--fs-metric-lg` | 24 / 30 | 600 mono | -0.01em | 卡片内主指标（`84.2 Mbps`） |
| `--fs-metric-md` | 17 / 22 | 600 mono | 0 | 卡片内次指标 |
| `--fs-metric-sm` | 13 / 18 | 500 mono | 0 | 行内数据、表格数值 |

字重只用 **400 / 500 / 600**，不用 700（中文 700 在 Windows 常被合成，发虚）。

### 2.9 间距阶梯（4 的倍数）

| Token | px | Token | px | Token | px |
|---|---|---|---|---|---|
| `--sp-0` | 0 | `--sp-4` | 16 | `--sp-10` | 40 |
| `--sp-0-5` | 2 | `--sp-5` | 20 | `--sp-12` | 48 |
| `--sp-1` | 4 | `--sp-6` | 24 | `--sp-16` | 64 |
| `--sp-1-5` | 6 | `--sp-7` | 28 | `--sp-20` | 80 |
| `--sp-2` | 8 | `--sp-8` | 32 | `--sp-24` | 96 |
| `--sp-3` | 12 | | | | |

固定用法（不再自由发挥）：

- 卡片 padding `--sp-5`（20px）；卡片内区块间距 `--sp-4`（16px）；卡片网格 gap `--sp-4`（16px）。
- 页面区块之间 `--sp-10`（40px）；顶栏到首个区块 `--sp-8`（32px）。
- 表单行间距 `--sp-4`；label 与控件之间 `--sp-2`（8px）。
- 图标与文字间隙 `--sp-2`（8px）；徽章内 padding `2px 8px`。

### 2.10 圆角

| Token | px | 用途 |
|---|---|---|
| `--r-xs` | 4 | 徽章内小标签、图表 tooltip |
| `--r-sm` | 6 | 输入框、下拉项、小按钮（28px 高） |
| `--r-md` | 8 | 按钮（32/36px 高）、分段控件、卡片内嵌块 |
| `--r-lg` | 12 | 卡片、面板 |
| `--r-xl` | 16 | 抽屉、Modal |
| `--r-pill` | 999 | 开关、状态点、进度条 |

### 2.11 阴影与发光

暗色下阴影几乎不可见，因此**阴影只用于浮层，桌面区靠描边**。

| Token | Dark | Light |
|---|---|---|
| `--shadow-sm` | `0 1px 2px rgba(0,0,0,.40)` | `0 1px 2px rgba(16,22,34,.06)` |
| `--shadow-md` | `0 4px 12px -2px rgba(0,0,0,.50)` | `0 4px 12px -2px rgba(16,22,34,.10)` |
| `--shadow-lg` | `0 16px 48px -8px rgba(0,0,0,.60)` | `0 16px 48px -8px rgba(16,22,34,.16)` |
| `--shadow-overlay` | `0 24px 64px -12px rgba(0,0,0,.72), 0 0 0 1px #272C35` | `0 24px 64px -12px rgba(16,22,34,.20), 0 0 0 1px #D9DCE2` |
| `--glow-running` | `0 0 0 3px rgba(33,191,174,.14), 0 0 24px -6px rgba(33,191,174,.35)` | `0 0 0 3px rgba(15,118,110,.12)` |
| `--focus-ring` | `0 0 0 2px #0A0B0D, 0 0 0 4px #21BFAE` | `0 0 0 2px #FFFFFF, 0 0 0 4px #0F766E` |

### 2.12 动效 token

| Token | 值 | 用途 |
|---|---|---|
| `--dur-1` | 80ms | 颜色/hover 过渡 |
| `--dur-2` | 140ms | 按钮按下、开关、Segmented 滑块 |
| `--dur-3` | 200ms | Tooltip、Popover、徽章变化 |
| `--dur-4` | 280ms | 抽屉滑入、Modal 缩放、Toast 进出 |
| `--dur-5` | 420ms | 评分环绘制、页面首屏入场 |
| `--dur-pulse` | 1600ms | 运行中脉冲循环 |
| `--ease-out` | `cubic-bezier(.16,1,.30,1)` | 元素进入（默认） |
| `--ease-in` | `cubic-bezier(.55,0,1,.45)` | 元素退出 |
| `--ease-std` | `cubic-bezier(.40,0,.20,1)` | 位置/尺寸变化 |
| `--ease-linear` | `linear` | 进度条 determinate、曲线绘制 |

### 2.13 栅格与断点

断点与列数**按 `07-ux-interaction.md` §11 的布局变化归属**，本文件只补像素归属与每档的具体尺寸。

| Token | px | 说明 |
|---|---|---|
| `--shell-max` | 1360 | 内容最大宽度 |
| `--gutter` | 24（<1440）/ 32（≥1440） | 页面左右安全边距 |
| `--topbar-h` | 56 | 顶栏高度 |
| `--grid-gap` | 16（<1440）/ 20（≥1440） | 栅格列间距 |
| `--card-min` | 360 | 探测卡最小宽度，低于此不允许再挤 |
| `--rail-w` | 480 | 详情并列栏宽度（≥1920 才常驻） |

| 断点 | 视口宽 | 内容宽 | 卡片列数 | 卡宽（3 列时） | 详情形态 | 视觉差异 |
|---|---|---|---|---|---|---|
| `bp-xs` | 960–1023 | `100vw − 32` | 1 | 撑满 | 全屏 Sheet（`100vw × 100vh`，`--r-xl` 归零） | 见下方「最小窗口」 |
| `bp-sm` | 1024–1279 | `100vw − 48` | 2 | (960−16)/2 ≈ 472 | 覆盖抽屉 480px | 次指标 3 行全留 |
| `bp-md` | 1280–1439 | 1232 | 3 | (1232−32)/3 ≈ 400 | 覆盖抽屉 480px | 次指标**减 1 行**（只留 2 条） |
| `bp-lg` | 1440–1919 | ≤1360 | 3 | ≈ 440 | 覆盖抽屉 480px | 右栏不常驻，结论卡整宽置顶 |
| `bp-xl` | ≥1920 | ≤1360 + 620（左栏 340 结论 + 右栏网格） | 2（右栏内） | ≈ 380 | 常驻并列 | 左栏结论常驻，右栏 2 列网格 |

- CSS 只需三档媒体查询：`(min-width:1280px)`、`(min-width:1440px)`、`(min-width:1920px)`；`1024–1279` 为默认档。
- **最小窗口 960×640**：`<960px` 或 `<640px` 高时不压缩布局，改为在 `topbar` 下方插入一条常驻提示条（`--warning-bg` / `--warning-text` / 高 40px / "窗口过小，请调整到 960×640 以上"），内容区 `pointer-events: none; opacity: .5`。
- 空间不足降级顺序（与 ux-arch §11.3 一致，此为视觉实现口径）：① 卡片 sparkline 隐藏（28px）→ ② 次指标由 3 行减到 2 行再减到 1 行（每行 18px）→ ③ 环境细节行隐藏（顶栏 Pill 收为纯图标）→ ④ 详情内 Tab 文字收为图标 → ⑤ Compare 方向列隐藏。**永不砍**：V1 结论句、V2 三态行、V4 出网方式、取消/停止、重测入口、卡片主数字。

### 2.14 可直接使用的 CSS 变量

```css
/* ============ tokens.css ============ */
:root {
  /* 表面 */
  --canvas: #0A0B0D;
  --surface: #101216;
  --surface-raised: #15181D;
  --surface-overlay: #1B1F26;
  --surface-inset: #0C0E12;
  --surface-hover: #191D24;
  --surface-active: #21262F;

  /* 文字 */
  --text-primary: #E6E9EF;
  --text-secondary: #A3ABB8;
  --text-tertiary: #7E8794;
  --text-disabled: #525A67;
  --text-inverse: #0A0B0D;

  /* 描边 */
  --line-subtle: #1C2027;
  --line: #272C35;
  --line-strong: #3A414D;
  --line-control: #5C6470;
  --line-focus: #21BFAE;

  /* 品牌 */
  --brand: #21BFAE;
  --brand-hover: #2ED4C1;
  --brand-active: #17A091;
  --brand-ink: #04130F;
  --brand-soft: #122728;
  --brand-line: #154947;
  --brand-text: #21BFAE;

  /* 语义 */
  --success-text: #3FB950; --success-bg: #16261D; --success-border: #1F4729;
  --warning-text: #E0A63C; --warning-bg: #29241B; --warning-border: #534122;
  --danger-text:  #F2555A; --danger-bg:  #2B1A1E; --danger-border:  #58272C;
  --info-text:    #6EA8FE; --info-bg:    #1B2432; --info-border:    #2E4260;
  /* blocked 是第七个 probe 状态，不属于四语义，单独一组 */
  --blocked-text: #A78BFA; --blocked-bg: #211C33; --blocked-border: #3E3460;

  /* 图表序列 */
  --chart-1: #21BFAE; --chart-2: #6EA8FE; --chart-3: #A78BFA;
  --chart-4: #F0B429; --chart-5: #F0716B; --chart-6: #8FD14F;
  --chart-grid: #1C2027;          /* 网格线 */
  --chart-axis: #7E8794;          /* 轴标注文字 */
  --chart-cursor: #E6E9EF;        /* 当前值游标 */
  --chart-fill-from: rgba(33,191,174,.16);  /* 唯一允许的面积渐变 */
  --chart-fill-to:   rgba(33,191,174,0);

  /* 字体 */
  --font-sans: -apple-system, BlinkMacSystemFont, "Segoe UI Variable Text", "Segoe UI",
               "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei UI", "Microsoft YaHei",
               "Noto Sans SC", system-ui, sans-serif;
  --font-mono: "SF Mono", SFMono-Regular, ui-monospace, "JetBrains Mono",
               "Cascadia Mono", "Roboto Mono", Menlo, Consolas, monospace;

  /* 字号 */
  --fs-score: 44px;    --lh-score: 48px;
  --fs-display: 32px;  --lh-display: 40px;
  --fs-h1: 24px;       --lh-h1: 32px;
  --fs-h2: 19px;       --lh-h2: 28px;
  --fs-h3: 15px;       --lh-h3: 22px;
  --fs-body: 14px;     --lh-body: 22px;
  --fs-small: 13px;    --lh-small: 20px;
  --fs-caption: 12px;  --lh-caption: 16px;
  --fs-micro: 11px;    --lh-micro: 14px;
  --fs-metric-lg: 24px; --lh-metric-lg: 30px;
  --fs-metric-md: 17px; --lh-metric-md: 22px;
  --fs-metric-sm: 13px; --lh-metric-sm: 18px;

  /* 间距 */
  --sp-0: 0px;   --sp-0-5: 2px; --sp-1: 4px;  --sp-1-5: 6px;
  --sp-2: 8px;   --sp-3: 12px;  --sp-4: 16px; --sp-5: 20px;
  --sp-6: 24px;  --sp-7: 28px;  --sp-8: 32px; --sp-10: 40px;
  --sp-12: 48px; --sp-16: 64px; --sp-20: 80px; --sp-24: 96px;

  /* 圆角 */
  --r-xs: 4px; --r-sm: 6px; --r-md: 8px; --r-lg: 12px; --r-xl: 16px; --r-pill: 999px;

  /* 阴影 */
  --shadow-sm: 0 1px 2px rgba(0,0,0,.40);
  --shadow-md: 0 4px 12px -2px rgba(0,0,0,.50);
  --shadow-lg: 0 16px 48px -8px rgba(0,0,0,.60);
  --shadow-overlay: 0 24px 64px -12px rgba(0,0,0,.72), 0 0 0 1px var(--line);
  --glow-running: 0 0 0 3px rgba(33,191,174,.14), 0 0 24px -6px rgba(33,191,174,.35);
  --focus-ring: 0 0 0 2px var(--canvas), 0 0 0 4px var(--brand);

  /* 动效 */
  --dur-1: 80ms; --dur-2: 140ms; --dur-3: 200ms;
  --dur-4: 280ms; --dur-5: 420ms; --dur-pulse: 1600ms;
  --ease-out: cubic-bezier(.16,1,.30,1);
  --ease-in: cubic-bezier(.55,0,1,.45);
  --ease-std: cubic-bezier(.40,0,.20,1);
  --ease-linear: linear;

  /* 布局 */
  --shell-max: 1360px;
  --gutter: 24px;
  --topbar-h: 56px;
  --grid-gap: 16px;
  --card-min: 360px;
}

[data-theme="light"] {
  --canvas: #FAFAFB;
  --surface: #FFFFFF;
  --surface-raised: #F4F5F7;
  --surface-overlay: #FFFFFF;
  --surface-inset: #F7F8FA;
  --surface-hover: #F1F2F4;
  --surface-active: #E9EBEF;

  --text-primary: #14161A;
  --text-secondary: #4A5058;
  --text-tertiary: #666E7A;
  --text-disabled: #9AA1AC;
  --text-inverse: #FFFFFF;

  --line-subtle: #ECEDF0;
  --line: #D9DCE2;
  --line-strong: #B4BAC6;
  --line-control: #838B99;
  --line-focus: #0F766E;

  --brand: #0F766E;
  --brand-hover: #0D6A62;
  --brand-active: #0A5A53;
  --brand-ink: #FFFFFF;
  --brand-soft: #E7F1F0;
  --brand-line: #BCD9D6;
  --brand-text: #0F766E;

  --success-text: #15682D; --success-bg: #E8F0EA; --success-border: #BDD5C4;
  --warning-text: #7D5300; --warning-bg: #F2EEE6; --warning-border: #DBCFB8;
  --danger-text:  #A81E17; --danger-bg:  #F6E8E8; --danger-border:  #E7C0BE;
  --info-text:    #1449A8; --info-bg:    #E8EDF6; --info-border:    #BDCCE7;
  --blocked-text: #6D28D9; --blocked-bg: #EFEAFB; --blocked-border: #CFC3EE;

  --chart-1: #0F766E; --chart-2: #2563EB; --chart-3: #7C3AED;
  --chart-4: #B45309; --chart-5: #BE123C; --chart-6: #4D7C0F;
  --chart-grid: #ECEDF0;
  --chart-axis: #666E7A;
  --chart-cursor: #14161A;
  --chart-fill-from: rgba(15,118,110,.14);
  --chart-fill-to:   rgba(15,118,110,0);

  --shadow-sm: 0 1px 2px rgba(16,22,34,.06);
  --shadow-md: 0 4px 12px -2px rgba(16,22,34,.10);
  --shadow-lg: 0 16px 48px -8px rgba(16,22,34,.16);
  --shadow-overlay: 0 24px 64px -12px rgba(16,22,34,.20), 0 0 0 1px var(--line);
  --glow-running: 0 0 0 3px rgba(15,118,110,.12);
  --focus-ring: 0 0 0 2px var(--surface), 0 0 0 4px var(--brand);
}

/* 基础重置（app.css 顶部） */
*, *::before, *::after { box-sizing: border-box; }
html, body { height: 100%; }
body {
  margin: 0;
  background: var(--canvas);
  color: var(--text-primary);
  font-family: var(--font-sans);
  font-size: var(--fs-body);
  line-height: var(--lh-body);
  -webkit-font-smoothing: antialiased;
  text-rendering: optimizeLegibility;
}
.num {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
  font-feature-settings: "tnum" 1, "zero" 1;
}
```

---

## 3. 关键界面高保真规格

### 3.0 全局骨架（所有界面共用）

```
┌────────────────────────────────────────────────────────────────────────────────┐ 0
│  ▣ ProxyLens │ 代理能力诊断          [⛨ 系统代理 · TUN]   [☾]  [⚙]              │ 56  ← topbar
├────────────────────────────────────────────────────────────────────────────────┤
│                                              ← gutter 24 →                       │
│        ┌───────────────────── shell-max 1360，居中 ─────────────────────┐        │ 88
│        │  页面内容（padding-top 32）                                    │        │
│        └────────────────────────────────────────────────────────────────┘        │
└────────────────────────────────────────────────────────────────────────────────┘
```

| 元素 | 规格 |
|---|---|
| `topbar` | `height: 56px`；`background: var(--surface)`；`border-bottom: 1px solid var(--line-subtle)`；`padding: 0 24px`；`position: sticky; top: 0; z-index: 40` |
| Logo 标记 | 20×20 SVG（菱形 + 内部一条曲线），`fill: var(--brand)`；与 wordmark 间距 8px |
| Wordmark | `14px/600`，`color: var(--text-primary)`，`letter-spacing: -0.01em` |
| 竖分隔 | `1px × 16px`，`background: var(--line)`，左右各 12px |
| 页面标题（topbar 内） | `14px/400`，`color: var(--text-secondary)` |
| 出网方式 Pill | 高 28px，`padding: 0 10px`，`--r-pill`，`background: var(--surface-raised)`，`border: 1px solid var(--line)`；14px 图标 + `12px/500` 文本，`color: var(--text-secondary)` |
| 图标按钮 | 32×32，`--r-md`，图标 18px，`color: var(--text-secondary)`；hover `background: var(--surface-hover); color: var(--text-primary)` |
| 内容容器 | `max-width: 1360px; margin: 0 auto; padding: 32px var(--gutter) 96px;` |
| 区块间距 | 同层区块 `margin-bottom: 40px`；区块标题与内容 `16px` |

### 3.1 S1 首屏（未测试）

纵向三段：标题区（72px）→ 控制卡（约 188px）→ 探测项预览（约 140px）。

```
        ┌──────────────────────────────────────────────────────────────────────┐
  88 →  │ 代理能力诊断                                                 24/600  │
        │ 用真实访问验证这套代理能否顺畅使用 AI，而不只是测一个节点延迟。      │
        │                                                     14/400 tertiary  │
 160 →  │ ┌──────────────── 控制卡 surface / r-lg / 1px line ─────────────────┐│
        │ │ ← padding 20 →                                                    ││
        │ │ ┌──────────────────────────────────────────────┐                  ││
        │ │ │ 系统代理 │ 自定义代理 │ 直连对照 │  Segmented 32 高，段 118/118/104││
        │ │ └──────────────────────────────────────────────┘                  ││
        │ │                                        ← 间距 16 →                 ││
        │ │ ┌────────────────────────────────────┐ ┌────────┐ ┌──────────┐    ││
        │ │ │ http://127.0.0.1:7890              │ │ 时长30s │ │ ▶ 开始测试│    ││
        │ │ │ 输入 h40 flex-1  mono 13           │ │h40 w140 │ │h40 w132  │    ││
        │ │ └────────────────────────────────────┘ └────────┘ └──────────┘    ││
        │ │ ⓘ 留空则使用系统代理（已检测到 TUN 模式，全局接管） 12/400 tertiary ││
        │ └────────────────────────────────────────────────────────────────────┘│
 372 →  │ 将测试以下 6 项                                            11/600 micro│
        │ ┌──────────────┐┌──────────────┐┌──────────────┐                      │
 392 →  │ │ ⬇ Google     ││ ▶ YouTube    ││ ✦ ChatGPT    │  3 列 gap 16         │
        │ │   大文件下载 ││   播放链路   ││   网页可达性 │  卡片 h 52 pad 0 12  │
        │ └──────────────┘└──────────────┘└──────────────┘                      │
        │ │ ✦ Gemini     ││ ⚡ OpenAI    ││ ◎ 国际基线   │                      │
        │ └──────────────┘└──────────────┘└──────────────┘                      │
        └──────────────────────────────────────────────────────────────────────┘
```

**控制卡明细**

| 区块 | 规格 |
|---|---|
| 卡片 | `background: var(--surface)`；`border: 1px solid var(--line)`；`border-radius: var(--r-lg)`；`padding: 20px`；`box-shadow: var(--shadow-sm)` |
| Segmented | 高 32px；整体 `background: var(--surface-raised)`；`border: 1px solid var(--line)`；`--r-md`；内 padding 2px；每段 `padding: 0 14px`、`13px/500`；选中段 `background: var(--surface-overlay)` + `box-shadow: var(--shadow-sm)` + `color: var(--text-primary)`；未选中 `color: var(--text-secondary)` |
| 行 2 | `display: flex; gap: 12px; align-items: flex-start; margin-top: 16px` |
| 代理输入框 | `flex: 1 1 auto`；`height: 40px`；`padding: 0 12px`；`font: 13px var(--font-mono)`；`--r-sm`；`background: var(--surface-inset)`；`border: 1px solid var(--line-control)` |
| 时长 Select | `width: 140px; height: 40px`；箭头 12px，`right: 12px` 居中 |
| 主 CTA | `width: 132px; height: 40px`；`background: var(--brand)`；`color: var(--brand-ink)`；`14px/600`；`--r-md`；图标 `play` 16px，与文字间距 6px |
| 提示行 | `margin-top: 8px`；`12px/400`；`color: var(--text-tertiary)`；`display: flex; gap: 6px`；图标 14px |
| 预览标题 | `11px/600`，`letter-spacing: .06em`，`color: var(--text-tertiary)`，`margin-bottom: 12px` |
| 预览卡 | `height: 52px`；`padding: 0 12px`；`background: var(--surface)`；`border: 1px solid var(--line)`；`--r-md`；图标 16px `--text-secondary` + `13px/500` `--text-primary`；`gap: 8px` |

**S1 交互态**

- 焦点顺序：Segmented → 输入框 → 时长 → CTA（预览卡不可聚焦）。
- 选中 `自定义代理`：输入框自动聚焦；`placeholder: "http://127.0.0.1:7890 或 socks5h://user:pass@host:port"`，placeholder 色 `--text-tertiary`。
- 选中 `系统代理` / `直连`：输入框 `disabled`（`opacity: .55`、`cursor: not-allowed`、边框降为 `--line`）。
- 输入非法：边框 `--danger-text`，下方 8px 出现 `12px` 错误文本 + `alert-circle` 图标（`--danger-text`），以 `--dur-2 --ease-out` 的 4px 下移淡入。
- CTA：hover `--brand-hover`；active `translateY(1px)` + `--brand-active`；运行中 `opacity: .5`、`cursor: progress`、文案 `测试中…`。

### 3.2 S2 测试中

自上而下：状态行 + 全局进度（44px）→ 实时速度面板（约 232px）→ 6 卡运行网格。

```
        ┌──────────────────────────────────────────────────────────────────────┐
  88 →  │ 正在测试 · 4/6 完成 · 预计剩余 12s          [ ⏹ 停止 ]       14/500  │
        │ ████████████████████████████████████░░░░░░░░░░░░░░░░░░  6px pill     │
 132 →  │ ┌───────── 实时速度 surface / r-lg / padding 20 ───────────────────┐ │
        │ │ 当前下载                                  ← 11/600 micro uppercase │ │
        │ │ 84.2  Mbps                                ← 32/600 mono + 14 unit  │ │
        │ │ P50 78.4 · P90 96.1 · jitter 6.3ms        ← 13 mono tertiary       │ │
        │ │ ┌──────────────────────────────────────────────────────────────┐ │ │
        │ │ │        ╱╲    ╱╲╱╲                                            │ │ │
        │ │ │   ╱╲╱╲╱  ╲╱╲╱    ╲╱╲╱╲╱╲╱     ← Canvas 高 120（无填充边框）   │ │ │
        │ │ │ ╱╱                          ╲╱                                │ │ │
        │ │ └──────────────────────────────────────────────────────────────┘ │ │
        │ └──────────────────────────────────────────────────────────────────┘ │
 380 →  │ ┌───────────────┐┌───────────────┐┌───────────────┐                  │
        │ │ ⬇ Google   ⏳ ││ ▶ YouTube  ✓  ││ ✦ ChatGPT  ⏳ │  3 列 gap 16      │
        │ │ 大文件下载    ││ 播放链路      ││ 网页可达性    │  卡片 h 148       │
        │ │ 84.2 Mbps    ││ 1.2s 起播     ││ TTFB 312ms    │                  │
        │ │ ▁▂▃▅▆▇▆▅▆    ││ 5.8 Mbps      ││ cdn ✓         │                  │
        │ └───────────────┘└───────────────┘└───────────────┘                  │
        │ ┌───────────────┐┌───────────────┐┌───────────────┐                  │
        │ │ ✦ Gemini   ⏳ ││ ⚡ OpenAI  ·  ││ ◎ 基线     ·  │                  │
        │ └───────────────┘└───────────────┘└───────────────┘                  │
        └──────────────────────────────────────────────────────────────────────┘
```

| 元素 | 规格 |
|---|---|
| 状态行 | `14px/500`；主文 `--text-primary`，分隔符 `·` 与次文 `--text-tertiary`；数字 mono |
| 停止按钮 | `secondary` 按钮：`height: 32px; padding: 0 12px`；`stop` 图标 14px + `13px/500` |
| 全局进度条 | `height: 6px`；`--r-pill`；轨道 `--line-subtle`；条 `--brand`；`transition: width 240ms linear`（跟随完成比例，**不**做缓动内插，避免"假进度"） |
| 进度条下方间距 | 24px 到实时面板 |
| 实时面板 | `padding: 20px`；图表 `margin-top: 12px`；图表内宽 = 1360 − 2×24(gutter) − 2×20(padding) = **1272px**，高 `120px` |
| 主数值 | `--fs-display`(32/600 mono)；单位 `14px/500` `--text-tertiary`，基线对齐、左距 6px |
| 次指标行 | `13px mono`；label `--text-tertiary`，值 `--text-secondary`；项间 12px |
| 运行中卡片 | `border-color: var(--brand-line)`；`box-shadow: var(--glow-running)`；徽章为 `running`（脉冲点） |
| mini sparkline | 高 `28px`，宽撑满卡片内宽（≈ 320px）；无坐标轴、无网格；1px 线（`--chart-1`）+ 8% 面积 |
| 未产出指标的槽位 | 骨架条：`height: 12px; width: 64px`，`background: var(--line-subtle)`，`--r-xs`，`animation: skeleton 1400ms var(--ease-std) infinite`；只对**值**占位，标题不占位 |
| 网格 | `display: grid; grid-template-columns: repeat(3, minmax(0,1fr)); gap: 16px`；1200px 以下改 2 列 |

**性能约束（实现必读）**

- 每次数据帧只更新**文本节点 `textContent`** 与**单条 canvas 重绘**；禁止改 DOM 结构、禁止 `innerHTML`、禁止对布局属性做动画。
- 6 张卡片 + 主曲线合并到同一个 `requestAnimationFrame` 批次执行；数据源频率高于 60Hz 时按帧丢弃（保留最新值）。
- 运行中禁止滚动容器尺寸变化（sparkline 高度固定，数值区高度固定 `--fs-metric-lg` 行高 30px）。

### 3.3 S3 结果页

三段式：综合结论卡（168px）→ 三分项（104px）→ 六卡网格（2 行 × 172px + gap）。

```
        ┌──────────────────── 综合结论 surface / r-lg / padding 24 ───────────────────┐
        │  ╭─────────╮   可用于 AI 工作流，但视频会偶发卡顿            24/600 →      │
        │  │   82    │                                                [导出 ▾][重测] │
        │  │  / 100  │   网络 91 · 稳定 78 · AI 可用 76                    14 mono  │
        │  ╰─────────╯   ⚠ p90 96.1Mbps 但 jitter 6.3ms 偏高；ChatGPT 首字节 312ms  │
        │   ↑ 132×132    14/400 secondary                                          │
        └──────────────────────────────────────────────────────────────────────────┘
        ┌───────────────┐┌───────────────┐┌───────────────┐
        │ 网络质量   91 ││ 稳定性     78 ││ AI 可用性  76 │  h 104, pad 16
        │ ██████████░   ││ ████████░░    ││ ███████░░░    │  条 6px pill
        │ 吞吐 84.2Mbps ││ 成功率 96%    ││ 4/5 服务可达  │  13 mono
        └───────────────┘└───────────────┘└───────────────┘
        ┌───────────────────────────────────┐┌───────────────────────────────────┐
        │ ⬇ Google 大文件下载        ✓ 正常 ││ ▶ YouTube 播放链路       ⚠ 降级  │
        │                                   ││                                   │
        │ 84.2 Mbps                         ││ 1.9 s                             │
        │ 平均 83.1 · p90 96.1 · TTFB 198ms ││ 起播 · 分片 5.8Mbps · 卡顿 2 次   │
        │ ▁▂▃▅▆▇▆▅▆▇▆▅▆                     ││ ▂▃▂▅▃▂▁▂▃▂▁▂                      │
        │ ─────────────────────────────     ││ ─────────────────────────────     │
        │ TLS 84ms                详情 ›    ││ 首片 1.2s              详情 ›      │
        └───────────────────────────────────┘└───────────────────────────────────┘
        ┌───────────────────────────────────┐┌───────────────────────────────────┐
        │ ✦ ChatGPT 网页可达性       ✓ 正常 ││ ✦ Gemini 网页可达性      ⊘ 拦截  │
        └───────────────────────────────────┘└───────────────────────────────────┘
        ┌───────────────────────────────────┐┌───────────────────────────────────┐
        │ ⚡ OpenAI API 端点         ✓ 可达 ││ ◎ 国际链路基线           ✓ 正常  │
        └───────────────────────────────────┘└───────────────────────────────────┘
```

**综合结论卡**

| 元素 | 规格 |
|---|---|
| 卡片 | `padding: 24px`；`display: grid; grid-template-columns: 132px 1fr auto; gap: 24px; align-items: center` |
| 评分环 | 132×132 SVG；`r = 56`，`stroke-width = 10`，`viewBox="0 0 132 132"`；轨道 `--line-subtle`(`#1C2027`)；进度 `--brand`，`stroke-linecap="round"`，`transform: rotate(-90deg)`；`stroke-dasharray: 351.86`（=2πr），`stroke-dashoffset` 由分数驱动，入场 `transition: stroke-dashoffset var(--dur-5) var(--ease-out)`，**只播一次** |
| 环内数字 | `--fs-score`(44/600 mono)，`--text-primary`；`/ 100` 为 `12px/500` `--text-tertiary`，与数字底边对齐、间距 4px |
| 结论句 | `24px/32`，`weight 600`，`--text-primary`；最多 2 行，超出省略并展开 tooltip |
| 分项摘要 | `14px mono`，项间 `·`，label `--text-tertiary` 值 `--text-primary` |
| 风险提示行 | `14px/400`，`--text-secondary`，前置 16px `alert-triangle`（`--warning-text`）；无风险时改为 `check-circle` + `--success-text` |
| 操作区 | 右对齐，两个按钮间距 8px：`导出报告 ▾`（secondary，h 36，带 `chevron-down`）、`重测`（primary，h 36） |
| 分数语义阈值 | ≥85 优（`--success-text`）；70–84 良（`--brand`）；50–69 可用但有短板（`--warning-text`）；<50 不可用（`--danger-text`）。环的进度色随阈值切换，数字色不变（保持 `--text-primary`） |

**三分项卡**

| 元素 | 规格 |
|---|---|
| 卡片 | `height: 104px`；`padding: 16px`；`background: var(--surface)`；`border: 1px solid var(--line)`；`--r-lg` |
| 标题行 | 左：`13px/500` `--text-secondary`（"网络质量"）；右：分数 `--fs-metric-lg`(24/600 mono) `--text-primary` |
| 分数条 | `margin-top: 10px`；`height: 6px`；`--r-pill`；轨道 `--line-subtle`；填充 = 分数色（同阈值规则）；`transition: width 320ms var(--ease-out)` |
| 脚注 | `margin-top: 10px`；`13px` mono，label `--text-tertiary` + 值 `--text-secondary` |

**探测卡（六卡网格）**

| 元素 | 规格 |
|---|---|
| 网格 | `grid-template-columns: repeat(2, minmax(0,1fr)); gap: 16px`（1200px 以下）；`repeat(3, ...)`（1200px 以上）。**注意**：简报要求六卡，3 列 × 2 行在 1360 下每卡 442px，信息密度最佳 |
| 卡高 | `172px`，固定（不因内容撑高，保证网格对齐）；`padding: 16px` |
| Header | `height: 24px`；图标 16px `--text-secondary` + 标题 `15px/600` `--text-primary` + 右侧徽章 `margin-left: auto` |
| 主指标 | `--fs-metric-lg`(24/30 600 mono)，`margin-top: 14px`；单位 `12px/500` `--text-tertiary`，左距 4px |
| 次指标 | `13px mono`，`--text-tertiary`，`margin-top: 4px`，单行省略 |
| Sparkline | `height: 28px`，`margin-top: 8px`，宽度撑满 |
| 分隔线 | `1px solid var(--line-subtle)`，`margin-top: 10px` |
| Footer | `height: 24px`；左 `13px mono` `--text-tertiary`；右 `详情 ›` `13px/500` `--text-secondary`，hover 变 `--brand-text` |
| 整卡交互 | hover：`border-color: var(--line-strong)`，`background: var(--surface-hover)`，`--dur-1`；点击整卡打开 S4；`cursor: pointer` |
| 失败/拦截卡 | 主指标位置替换为状态词（"地区限制"），`--danger-text`；`border-color: var(--danger-border)`；`background: var(--danger-bg)` |

### 3.4 S4 探测详情（右侧抽屉）

1600px 以上以「右侧固定栏」形式展开（宽 480px，与网格并列，不遮挡卡片）；1024–1599 以抽屉覆盖（宽 `min(520px, 92vw)`）。两形态共用同一 DOM。

```
┌──────────────────────────────── 抽屉 surface-overlay / r-xl(左) / 宽 480 ────────┐
│ ⬇ Google 大文件下载                                      ✓ 正常      [ ✕ ]     │ 56
│ dl.google.com · 20 MB · 30s · 6 次采样                                          │
├─────────────────────────────────────────────────────────────────────────────────┤  ← line-subtle
│ [ 概览 ] [ 阶段 ] [ 速度 ] [ 证据 ]            ← Tabs h 40，选中下划 2px brand    │ 40
├─────────────────────────────────────────────────────────────────────────────────┤
│ 阶段瀑布                                    单位 ms                 11/600 micro│
│                                                                                 │
│ DNS      ▏▎                                         12                         │
│ TCP      ▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎                  118                         │
│ TLS      ▏▎▏▎▏▎▏▎                                84                           │
│ TTFB     ▏▎▏▎                                    41                           │
│ 传输     ▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎▏▎        1 840                  │
│          0        400       800      1200     1600     2000 ms                │
│ ─────────────────────────────────────────────────────────────────────────────  │
│ 速度曲线                                                                        │
│ ┌─────────────────────────────────────────────────────────────────────────┐   │
│ │  100 ┤ - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -  │   │
│ │   75 ┤ - - - - - - -╱╲- - - -╱╲- - - - - - - - - - - - - - - - - - - -  │   │
│ │   50 ┤ - - - -╱╲╱╲╱  ╲╱╲╱  ╲╱╲╱╲ - - - - - - - - - - - - - - - - - - -  │   │
│ │   25 ┤ - ╱╲╱                    ╲╱╲╱╲╱╲- - - - - - - - - - - - - - - -  │   │
│ │    0 └───────────────────────────────────────────────────────────────── │   │
│ │       0s        5s        10s       15s       20s       25s       30s    │   │
│ └─────────────────────────────────────────────────────────────────────────┘   │
│ ─────────────────────────────────────────────────────────────────────────────  │
│ 证据                                                            复制 ⧉ / 导出   │
│ ┌─ surface-raised / r-md / padding 12 / mono 12 / 行高 20 ────────────────────┐ │
│ │ {                                                                          │ │
│ │   "probe": "P1.google_bulk",                                              │ │
│ │   "http_status": 206,                                                      │ │
│ │   "content_length": 20971520,                                              │ │
│ │   "throughput_mbps": { "p50": 78.41, "p90": 96.12 },                       │ │
│ │   "tls": { "version": "TLSv1.3", "alpn": "h2", "cert_ms": 41 }              │ │
│ │ }                                                                          │ │
│ └────────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────────┘
```

| 元素 | 规格 |
|---|---|
| 抽屉 | 宽 480px；`background: var(--surface-overlay)`；`border-left: 1px solid var(--line)`；`border-radius: var(--r-xl) 0 0 var(--r-xl)`（覆盖形态则 `--r-xl` 全角）；`box-shadow: var(--shadow-overlay)`；`z-index: 60` |
| 入场 | 覆盖形态：`translateX(24px)` + `opacity 0` → 0，`var(--dur-4) var(--ease-out)`；并列形态：宽度从 0 → 480，`var(--dur-4) var(--ease-std)`，同时主网格让位（不做缩放） |
| 遮罩 | `rgba(0,0,0,.48)`，覆盖形态才有；点击遮罩关闭 |
| 抽屉头 | `height: 56px`；`padding: 0 16px`；标题 `15px/600`；副标题 `12px/500` `--text-tertiary`（`mono`）+ 徽章右对齐；关闭按钮 28×28 |
| Tabs | `height: 40px`；`padding: 0 16px`；每项 `13px/500`，`padding: 0 12px`，未选中 `--text-secondary`；选中 `--text-primary` + 底部 2px `--brand` 指示条（`transition: transform var(--dur-2) var(--ease-std)`） |
| 内容区 | `padding: 16px`；纵向 `gap: 24px`；`overflow-y: auto`；滚动条宽 8px，thumb `--line-strong`，`--r-pill` |
| 分组标题 | `11px/600`，`letter-spacing: .06em`，`--text-tertiary`，`margin-bottom: 10px`；右侧单位/操作 `11px/600` 或链接 |

**阶段瀑布图（DOM 实现，不用 Canvas）**

| 项 | 规格 |
|---|---|
| 行高 | 每阶段行 `28px`（含 8px 间隙），5 行共 140px |
| 标签列 | 固定宽 `64px`，`12px/500` `--text-secondary`，右对齐，`padding-right: 12px` |
| 数值列 | 固定宽 `64px`，`12px/500 mono` `--text-tertiary`，右对齐 |
| 轨道 | 宽 = 内容宽 − 64 − 64 − padding，高 `10px`，`background: var(--line-subtle)`，`--r-xs`（视觉基线） |
| 条形 | `height: 10px`；`--r-xs`；`left = startOffset / totalSpan × 100%`；`width = duration / totalSpan × 100%`，**最小 3px**（不足时用 3px + 右侧 1px 渐隐缺口表示溢出） |
| 颜色 | DNS `--chart-2`；TCP `--chart-1`；TLS `--chart-3`；TTFB `--chart-4`；传输 `--chart-6`。**不**用状态色（阶段本身无优劣） |
| 轴 | 6 个刻度（0 / 20% … 100%）；`11px/500 mono` `--chart-axis`；刻度线 `1px dashed var(--chart-grid)` 仅贯穿轨道区 |
| 对齐规则 | **所有条形共用同一 x=0 原点与同一总时长标尺**（`totalSpan` = 最后一个阶段结束时刻），跨阶段累计偏移，不是每行独立归一化 —— 这样"哪一段吃掉了时间"一眼可见 |
| 悬停 | 条形 `opacity: .8`；同色 Tooltip 显示 `阶段 / 起止 / 耗时` |

### 3.5 S5 设置面板（居中 Modal）

宽 `560px`；`background: var(--surface-overlay)`；`border: 1px solid var(--line)`；`border-radius: var(--r-xl)`；`box-shadow: var(--shadow-overlay)`。

```
        ┌──────────────── 设置 surface-overlay / r-xl / 宽 560 ────────────────┐
        │ 设置                                                       [ ✕ ]     │ 52
        ├──────────────────────────────────────────────────────────────────────┤
        │ 测试参数                                          11/600 micro        │
        │ 并发探测数      [ 3 ▾ ]        同时进行的探测项，过高会互相抢带宽    │ 行高 44
        │ 单次时长        [ 30s ▾ ]      每项探测的采样窗口                    │
        │ 下载字节预算    [ 20 MB ▾ ]    单次下载上限，影响流量消耗            │
        │ 单请求超时      [ 10s ▾ ]      超时即判定该项失败                    │
        │ 采样间隔        [ 250ms ▾ ]    流式指标刷新粒度                      │
        ├──────────────────────────────────────────────────────────────────────┤
        │ 外观与语言                                                           │
        │ 主题            [ 深色 │ 浅色 │ 跟随系统 ]   Segmented               │
        │ 语言            [ 简体中文 ▾ ]                                       │
        ├──────────────────────────────────────────────────────────────────────┤
        │ 安全                                                                 │
        │ ☑ 代理凭据仅保留在内存，不写入磁盘                                   │
        │ ☐ 允许在报告中包含原始响应片段                                       │
        ├──────────────────────────────────────────────────────────────────────┤
        │                                        [ 恢复默认 ]   [ 保存 ]        │ 64
        └──────────────────────────────────────────────────────────────────────┘
```

| 元素 | 规格 |
|---|---|
| Modal 入场 | `scale(.98)` + `opacity 0` → `scale(1)` + `1`，`var(--dur-4) var(--ease-out)` |
| 头 | `height: 52px`；`padding: 0 20px`；标题 `15px/600`；`border-bottom: 1px solid var(--line-subtle)` |
| 分组标题 | `11px/600` `.06em` `--text-tertiary`；上方距 20px、下方 8px；分组之间 `border-top: 1px solid var(--line-subtle)` + `padding-top: 16px` |
| 设置行 | `height: 44px`；`display: grid; grid-template-columns: 112px 140px 1fr; align-items: center; gap: 12px`；label `13px/500` `--text-primary`；控件；说明 `12px/400` `--text-tertiary` 单行省略 |
| Select | `height: 32px`；`padding: 0 28px 0 10px`；`--r-sm`；`background: var(--surface-inset)`；`border: 1px solid var(--line-control)`；`13px` |
| 页脚 | `height: 64px`；`padding: 0 20px`；右对齐；`恢复默认`（ghost）+ `保存`（primary，h 36，间距 8px）；`border-top: 1px solid var(--line-subtle)` |
| 未保存态 | 页脚右上出现 `12px` `--warning-text` "有未保存的更改"，保存后 `--dur-3` 淡出 |

### 3.6 S6 报告导出预览

同一 Modal 容器，宽 `720px`，左侧选项栏 200px + 右侧预览区 520px（横向两栏）。

```
        ┌────────────── 导出报告 surface-overlay / r-xl / 宽 720 ───────────────┐
        │ 导出报告                                                    [ ✕ ]     │ 52
        ├────────────────┬──────────────────────────────────────────────────────┤
        │ 格式           │  ┌── 预览（surface-inset，可滚动，mono 12/20）──────┐ │
        │ ● JSON         │  │ ProxyLens 诊断报告                                │ │
        │ ○ Markdown     │  │ 2026-10-22 08:41:12  ·  系统代理 (TUN)            │ │
        │ ○ 摘要卡片     │  │ ───────────────────────────────────────────────── │ │
        │                │  │ 综合 82 / 100 · 可用于 AI 工作流，视频偶发卡顿     │ │
        │ 内容           │  │ 网络 91 · 稳定 78 · AI 可用 76                    │ │
        │ ☑ 指标明细     │  │ ───────────────────────────────────────────────── │ │
        │ ☑ 阶段耗时     │  │ P1 Google 下载   84.2 Mbps   ✓                   │ │
        │ ☐ 原始 JSON    │  │ P2 YouTube       1.9s 起播   ⚠                   │ │
        │ ☐ 响应片段     │  │ P3 ChatGPT       200 · 312ms  ✓                  │ │
        │                │  │ ...                                              │ │
        │ ☑ 脱敏代理地址 │  └──────────────────────────────────────────────────┘ │
        ├────────────────┴──────────────────────────────────────────────────────┤
        │                              [ 复制到剪贴板 ]   [ 保存文件… ]           │ 64
        └───────────────────────────────────────────────────────────────────────┘
```

| 元素 | 规格 |
|---|---|
| 左栏 | 宽 200px；`padding: 16px`；`border-right: 1px solid var(--line-subtle)` |
| 单选组 | 每行 `height: 32px`；radio 16px + `13px/500`；选中文字 `--text-primary`，未选中 `--text-secondary` |
| 复选项 | 同上，checkbox 16px；`--r-xs` |
| 预览区 | 宽 520px；`background: var(--surface-inset)`；`padding: 16px`；`font: 12px/20 var(--font-mono)`；`color: var(--text-secondary)`；最大高 `420px` 滚动；顶部/底部 `mask-image` 8px 渐隐（**唯一允许的第二处渐变，仅作为滚动提示**） |
| 分隔线（预览内） | 用 `─` 重复字符模拟，色 `--line`（不是真 border，保证复制出来仍是纯文本） |
| 页脚 | 同 S5；`保存文件…` 为 primary |

---

### 3.7 ConfigSummary（`done` 态折叠配置条）

`done` 态下 S1 的整块控制卡必须折叠为**一行**（ux-arch §1.4 / §6 硬约束），这是首屏腾出空间的唯一手段。它是独立组件，不参与 hover 展开。

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ ⛨ 系统代理 · TUN   │  30s  │  20 MB  │  并发 3        [ 查看/修改 ▾ ]  [ 重测 ]  │ 40
└──────────────────────────────────────────────────────────────────────────────────┘
   13 mono tertiary      13 mono      13 mono              ghost sm        primary sm
```

| 元素 | 规格 |
|---|---|
| 容器 | `height: 40px`；`padding: 0 4px 0 12px`；`background: transparent`；`border-bottom: 1px solid var(--line-subtle)`；`display: flex; align-items: center; gap: 10px`。**无边框无底色**（避免与结论卡抢视觉重量） |
| 模式段 | 图标 14px `--text-tertiary` + 文本 `13px/500 mono` `--text-secondary`；代理地址**脱敏**：`socks5h://u***@1.2.3.4:1080` |
| 分隔符 | `│` 用 1px × 12px 的 `background: var(--line)`，或 `·` 字符（`--text-tertiary`）；同屏统一用竖线 |
| 其余段 | `13px/400 mono` `--text-tertiary`；label 省略（`30s` / `20 MB` / `并发 3` 自解释） |
| 右侧操作 | `查看/修改 ▾`（ghost，h 28，`13px/500`）+ `重测`（primary，h 28）；间距 8px；容器 `padding-right: 4px` |
| 展开 | 点击 `查看/修改` 就地展开为完整控制卡，`max-height` 动画 `--dur-4 var(--ease-std)`；展开时折叠条变 `--text-primary` 保持可见作为标题 |
| `running` 态 | 同一组件保留但右侧换成 `停止`（secondary，h 28）；各段数值**冻结**为本次运行参数，不允许中途改动（可点 `查看/修改` 查看但控件 disabled） |

### 3.8 卡片主数字唯一性（硬规则）

每张探测卡正面**只允许一个主数字**：判定规则 = **哪个数字决定徽章颜色，哪个就是主位**。

| 卡 | 主位内容 | 字号 | 次位（`13px mono tertiary`，最多 3 条） |
|---|---|---|---|
| P1 Google 下载 | `84.2 Mbps` | `--fs-metric-lg` | 平均 / p90 / TTFB* |
| P2 YouTube | `1.9 s`（起播） | `--fs-metric-lg` | 分片速率 / 卡顿次数 / 首片* |
| P3 ChatGPT | **结论短语** `可用` / `受限` / `不可用`（`17px/600`，`--text-primary`） | — | 首字节 / CDN 资源 / 挑战识别*（数字只进详情） |
| P4 Gemini | 同 P3 | — | 同 P3 |
| P5 OpenAI API | 同 P3（`可达 401` 这类语义化短语，不写裸状态码） | — | 握手 / 端点 / 语义* |
| P6 国际基线 | `96 %`（成功率） | `--fs-metric-lg` | RTT / 丢包 / 抖动* |

> `*` 标记项在 `bp-md`（1280–1439）起按降级顺序隐藏。P3/P4/P5 **不得**把 `200`、`403`、`312ms` 放在主位——那是状态码与内部指标，只允许出现在 §3.4 证据区。

---

### 3.9 S7 截图友好摘要（报告版）

这是 R3 技术支持型用户**转发**用的界面：要像"一份体检报告"，不像"一张软件截图"。三个硬约束：**1280×800 一屏不滚动**、**125% 缩放（逻辑 1024×640）同样成立**、**强制浅色 + 灰度可辨 + 关键信息不依赖悬停/点击**。

版面按 **1024×640 逻辑尺寸**做高度预算（这是最紧的一档），1280×800 只是同版面的放大，绝不出现第二套布局。

```
┌─ 报告容器：宽 min(1200, 100vw−48)，浅色强制，白底，无水印 ─────────────────┐
│ ProxyLens 代理能力诊断                                    2026-10-22 08:41  │ 48
│ 出网方式：系统代理（TUN） · 时长 30s · 预算 20 MB · 6 项全部执行            │ 20
├────────────────────────────────────────────────────────────────────────────┤ 1
│  82 · 良        可用于 AI 工作流；看 1080p 视频会偶发缓冲。      24/600      │ 120
│  ██████████████████████░░░  65px 五档条 · 光标落在 B 档                     │
│  网络 91 · 优     稳定 78 · 良     AI 可用 76 · 良                          │
├────────────────────────────────────────────────────────────────────────────┤ 16
│ ChatGPT   ● 可用        Gemini  ◐ 受限：人机验证      API  ● 可用           │ 96
│ （三行 A 轴，含旁注；形状区分，灰度可分）                                    │
├────────────────────────────────────────────────────────────────────────────┤ 16
│ 速度：够用 —— 下载 96 Mbps，1080p 起播 1.9 秒。                              │ 80
│ 分段条 3/3 · B 轴不提徽章                                                    │
├────────────────────────────────────────────────────────────────────────────┤ 16
│ P1 Google 下载  84.2 Mbps     P2 YouTube   1.9s 起播     P3 ChatGPT  312ms  │ 132
│ P4 Gemini  地区限制           P5 OpenAI    401 可达      P6 基线   96%      │
│ （2 列 × 3 行小表，每格：名称 12px / 值 13px mono / 状态图形）               │
├────────────────────────────────────────────────────────────────────────────┤ 16
│ 由 ProxyLens 生成 · 未采集任何账号信息 · 报告仅反映本次测试时段的网络路径      │ 32
└────────────────────────────────────────────────────────────────────────────┘
```

| 项 | 规格 |
|---|---|
| 容器 | 宽 `min(1200px, 100vw − 48px)`；`background: #FFFFFF`；`border: 1px solid #D9DCE2`；`--r-lg`；`padding: 24px`；**强制浅色**（无论当前主题，导出时读 `[data-theme="light"]` 的 token 快照） |
| 高度预算（1024×640 档） | `48 + 1 + 120 + 16 + 96 + 16 + 80 + 16 + 132 + 16 + 32 = 573`，加 `padding 24×2 = 621 ≤ 640` **成立**；1280×800 档只增加行间留白（`gap 16 → 24`、`padding 24 → 32`），不增删元素 |
| 五档条 | `height: 6px`；`width: 200px`；五段均分（`38px` 每段，`gap: 2px`）；归档段 `#1A7F37`（A）`#0F766E`(B) `#B45309`(C) `#BE123C`(D) `#7C2D12`(E)；非归档段 `#E9EBEF`；**当前档位用 3×12 的实心小三角指示器**（不是描边），灰度下靠三角形位置辨认 |
| 三行 A 轴 | 行高 `32px`，`●` 实心点 / `◐` 缺口环 / `⊘` 斜杠环；`12px/500`；旁注 `12px/400 #4A5058` |
| 六项小表 | `grid-template-columns: repeat(3, 1fr)`；3 行；每格 `height: 40px`；格内三元素**横排**（名称 / 值 / 状态图形），`column-gap: 8px`；`border-bottom: 1px solid #ECEDF0` |
| 字号 | 与正文一致（不缩字号讨好打印）；**不使用小于 11px 的任何文字** |
| 灰度验收 | 去掉颜色后：A 轴三态靠 `●/◐/⊘`；档位靠三角位置；六项状态靠 12px 图形；B 轴靠分段条 3/2/1 段 —— 全部成立，无一处仅依赖颜色 |
| 打印 | `@media print { body { background: #FFF } .report { border: 0; box-shadow: none } }`；`print-color-adjust: exact` 保证浅色底不被浏览器剥色 |
| 无交互依赖 | 所有 Tooltip 内容在报告里改为**行内文字**（如 `DegradedFlag` 直接写出"样本 4 次，建议 ≥6 次"）；不出现"点击查看"字样 |

---

## 4. 数据可视化规格

通用：所有图表 **背景透明**（继承卡片表面），无外框、无圆角、无图例框；坐标轴只有文字，无轴线；网格仅水平线。Canvas 一律按 `devicePixelRatio` 缩放（`canvas.width = cssW * dpr`，`ctx.scale(dpr, dpr)`），`image-rendering: auto`。

### 4.1 速度曲线（实时 + 结果共用同一渲染器）

| 项 | 规格 |
|---|---|
| 尺寸 | 实时面板：宽 1272 × 高 120；详情页：宽 = 抽屉内宽 448 × 高 120；卡片 sparkline 复用同逻辑但关掉轴与网格 |
| 内边距 | `pad = { top: 8, right: 8, bottom: 20, left: 40 }`（左侧留给 y 轴标签，底部留给时间标签） |
| 绘制区 | `w = width - 48`，`h = height - 28` |
| X 轴（实时） | 固定滑动窗口 **最近 30s**；`x(t) = pad.left + (1 - (now - t)/30000) * w`；超窗数据丢弃（保留 60s 环形缓冲，便于窗口切换） |
| X 轴（结果） | 全时段 0 → 实际时长，刻度取 5s 整数倍，最多 7 个 |
| Y 轴 | 下限固定 0；上限 = `niceCeil(max * 1.15)`，`niceCeil` 取 1/2/5×10ⁿ 序列（例：96.1 → 120）；刻度 4 条（0 / 33% / 66% / 100%），`11px/500 mono`，`--chart-axis`，右对齐于 `x = pad.left - 8` |
| 网格 | 水平，`1px`，`--chart-grid`，`dash [2,4]`；不画竖线 |
| 曲线 | 线宽 **1.5px**，`--chart-1`，`lineJoin: round`，`lineCap: round`；不加阴影；点数 > 绘制区宽度时按 x 像素聚合（取区间最大值，保证尖峰不丢） |
| 面积填充 | 从曲线到基线的垂直渐变 `--chart-fill-from` → `--chart-fill-to`（`y: pad.top` → `y: pad.top+h`）；**这是全站唯一的装饰渐变** |
| 当前值游标 | 竖线 `1px solid var(--chart-cursor)`，`dash [3,3]`，距右边缘 `8px`；顶点实心圆 `r=3`，`fill: var(--chart-1)`，外描 `2px var(--surface)`（保证反差）；游标仅实时态出现 |
| 悬停 | 十字线（竖 1px `--line-strong` + 横 1px `--line-strong`），最近点高亮 `r=3.5`；Tooltip 跟随鼠标，偏移 `+12 / -12`，超出右边界则翻转到左侧 |
| 空态 | 绘制区居中 `13px` `--text-tertiary` "等待数据…"；前 3s 内曲线不足 2 点时不画线，只画一个脉动点 |
| 降级 | `prefers-reduced-motion` 下取消游标脉冲，曲线仍实时重绘（数据变化本身必要） |

### 4.2 阶段瀑布

几何与对齐规则见 §3.4。补充：

- `totalSpan` = 最后一个阶段结束时刻（含传输），**不做 per-row 归一化**。
- 当 `传输` 占总时长 > 70% 时，其余四段会退化成 3px 窄条：此时在轨道上方补一行 `11px` 提示"其余阶段合计 255ms（12%）"，并把前四段合并为一根灰色条 `--line-strong` 显示。
- 数值列在 `duration < 1ms` 时显示 `<1`，不显示 `0.4`。

### 4.3 分项评分（环形 + 条形，两种同源）

| 形态 | 用途 | 几何 | 颜色 |
|---|---|---|---|
| 大环 | 总分（S3） | `viewBox 132×132`，`r=56`，`stroke-width=10`，`dasharray=351.86` | 阈值色（见 §3.3） |
| 条形 | 分项（S3 三分项卡）、探测卡内评分条 | 高 `6px`，`--r-pill`，轨道 `--line-subtle` | 阈值色 |
| 迷你环 | 抽屉内分项（可选） | `viewBox 40×40`，`r=16`，`stroke-width=4` | 阈值色 |

阈值：≥85 `--success-text`；70–84 `--brand`；50–69 `--warning-text`；<50 `--danger-text`。条形填充宽 = `score%`，**视觉上不小于 3px**（0 分时显示 0）。

### 4.4 状态徽章（Badge）

| 项 | 规格 |
|---|---|
| 尺寸 | 高 `22px`；`padding: 0 8px`；`--r-sm`(6px)；`gap: 4px`；图标 12px；文字 `12px/500` |
| 变体 `soft`（默认） | `background: var(--x-bg)`；`color: var(--x-text)`；`border: 1px solid var(--x-border)` |
| 变体 `dot`（卡片头，紧凑） | 仅 `8px` 圆点 + 文字，无底色无边框，`padding: 0` |
| 数字徽章 | 高 `18px`；`min-width: 18px`；`padding: 0 5px`；`--r-pill`；`12px/600 mono`；`background: var(--surface-raised)`；`color: var(--text-secondary)`；`border: 1px solid var(--line)` |

### 4.5 Sparkline（卡片内迷你趋势）

- 高 `28px`；宽撑满；`pad = 0`；无轴、无网格、无填充或 8% 面积（二者择一，同一页统一用 8%）。
- 线宽 `1px`；颜色 = 该卡主指标序列色（默认 `--chart-1`）。
- Y 轴按自身 min/max 自适应，但**最小值不低于 0**；同页多卡的 sparkline **不共享 Y 轴**（避免误读为可比）。
- 最后一个点画 `r=2` 实心点，颜色同线，外描 1px `--surface`。
- 失败/无数据：画一条 `1px dashed var(--line-strong)` 的水平线 + 右侧 `11px` `--text-tertiary` "无数据"。

### 4.6 图表 Tooltip

`background: var(--surface-overlay)`；`border: 1px solid var(--line-strong)`；`--r-xs`(4px)；`padding: 6px 8px`；`box-shadow: var(--shadow-md)`；`12px` 文本；数值 `mono`；出现 `--dur-1`、位移 `4px` 淡入；`pointer-events: none`；`z-index: 70`。

---

### 4.7 曲线的文本替代视图（a11y + 可复制）

ux-arch §8.2 要求在速度曲线下方提供"以文本方式查看数据"。排布与规格：

```
 速度曲线                                        [ 以文本查看 ]  ← ghost sm, h24
 （Canvas 或表格，二者互斥替换，高度锁定 120px）
```

| 形态 | 规格 |
|---|---|
| 触发 | 图表右上角 `以文本查看` / `看图`（ghost，h 24，`12px/500`，右侧 `chevron-down` 旋转 180°） |
| 表格 | 高 `120px`（与 Canvas 等高，切换时**不改变**容器高度，避免页面跳动）；`overflow-y: auto`；列：`时间` / `Mbps` / `阶段`；`12px mono`；`padding: 0 8px`；行高 `20px`；表头 sticky，`--surface-raised` |
| 采样 | 每 `250ms` 一行，30s 窗口 = 120 行；`<caption>` 写 `最近 30 秒下载速率采样` |
| `<canvas>` 无障碍 | Canvas 必须带 `role="img"` + `aria-label`（如 `下载速率曲线，30 秒内从 82 升到 96 Mbps，峰值 96.1`）+ 紧邻的可聚焦 `<a href="#speed-table">以文本查看</a>` —— 视障用户通过表格获取全量数据 |
| 切换态记忆 | 存 `localStorage.diag.speedView = 'chart' \| 'table'`，全局生效 |

---

## 5. 组件视觉规格

尺寸基准：`sm` 高 28（`--r-sm`）、`md` 高 32（`--r-md`）、`lg` 高 36（`--r-md`）、`xl` 高 40（`--r-md`）。默认 `lg`。图标 16px（`lg` 以下 14px），图标与文字间距 6px。所有按钮 `font-weight: 500`、`font-size: 13px`（`xl` 用 14px）、`letter-spacing: 0`、`white-space: nowrap`。

### 5.1 按钮四变体 × 五态

| 变体 | 默认 | hover | active | disabled | focus-visible |
|---|---|---|---|---|---|
| `primary` | `bg --brand`<br>`fg --brand-ink` | `bg --brand-hover` | `bg --brand-active`，`translateY(1px)` | `bg --brand` `opacity .45`，`cursor not-allowed` | `box-shadow: var(--focus-ring)` |
| `secondary` | `bg --surface-raised`<br>`fg --text-primary`<br>`border 1px --line-strong` | `bg --surface-hover`<br>`border --line-control` | `bg --surface-active`，`translateY(1px)` | `opacity .45`，边框 `--line` | 同上 |
| `ghost` | `bg transparent`<br>`fg --text-secondary` | `bg --surface-hover`<br>`fg --text-primary` | `bg --surface-active` | `opacity .4` | 同上 |
| `danger` | `bg --danger-bg`<br>`fg --danger-text`<br>`border 1px --danger-border` | `bg --danger-text` `fg --brand-ink`（`#F2555A` 上写字用 `#0A0B0D`） | 再暗 8%：`filter: brightness(.92)` | `opacity .45` | 同上 |

补充规则：

- `primary` hover **不加阴影、不做位移**（Linear 式克制）；只有 active 允许 `translateY(1px)`。
- 所有过渡：`transition: background-color var(--dur-1), border-color var(--dur-1), color var(--dur-1), transform var(--dur-2) var(--ease-std)`。
- 焦点环用 `:focus-visible` 三层 box-shadow（见 `--focus-ring`），**不使用** `outline`（避免 Windows 上圆角丢失）。
- 只有图标的按钮：正方形（32×32 / 28×28 / 40×40），必须带 `aria-label` + Tooltip（延迟 400ms）。
- 加载中按钮：文字保留，左侧图标替换为 14px 旋转弧（`animation: spin 720ms linear infinite`），宽度**不变**（用 `min-width` 锁定）。

```css
.btn {
  display: inline-flex; align-items: center; justify-content: center; gap: 6px;
  height: var(--btn-h, 36px); padding: 0 12px;
  font: 500 13px/1 var(--font-sans); white-space: nowrap;
  border-radius: var(--r-md); border: 1px solid transparent;
  transition: background-color var(--dur-1), border-color var(--dur-1),
              color var(--dur-1), transform var(--dur-2) var(--ease-std);
  cursor: pointer; -webkit-user-select: none; user-select: none;
}
.btn--xl { --btn-h: 40px; font-size: 14px; padding: 0 16px; }
.btn--sm { --btn-h: 32px; font-size: 13px; padding: 0 10px; }
.btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
.btn:disabled, .btn[aria-disabled="true"] { opacity: .45; cursor: not-allowed; pointer-events: none; }
.btn--primary   { background: var(--brand); color: var(--brand-ink); }
.btn--primary:hover  { background: var(--brand-hover); }
.btn--primary:active { background: var(--brand-active); transform: translateY(1px); }
.btn--secondary { background: var(--surface-raised); color: var(--text-primary); border-color: var(--line-strong); }
.btn--secondary:hover  { background: var(--surface-hover); border-color: var(--line-control); }
.btn--secondary:active { background: var(--surface-active); transform: translateY(1px); }
.btn--ghost   { background: transparent; color: var(--text-secondary); }
.btn--ghost:hover  { background: var(--surface-hover); color: var(--text-primary); }
.btn--ghost:active { background: var(--surface-active); }
.btn--danger  { background: var(--danger-bg); color: var(--danger-text); border-color: var(--danger-border); }
.btn--danger:hover { background: var(--danger-text); color: var(--brand-ink); }
```

### 5.2 输入框

| 状态 | 规格 |
|---|---|
| 默认 | `height: 40px`（表单）/ `32px`（紧凑）；`padding: 0 12px`；`font: 14px var(--font-sans)`；`--r-sm`；`background: var(--surface-inset)`；`border: 1px solid var(--line-control)`；`color: var(--text-primary)`；`::placeholder` 用 `--text-tertiary` |
| hover | `border-color: var(--line-strong)` 加深一档 → 实际用 `filter` 不可靠，直接 `border-color: color-mix(in oklab, var(--line-control), var(--text-secondary) 30%)`；若不用 `color-mix`，则维持 `--line-control` 不变（可接受） |
| focus | `border-color: var(--brand)`；`box-shadow: 0 0 0 3px var(--brand-soft)`；无 outline |
| 错误 | `border-color: var(--danger-text)`；`box-shadow: 0 0 0 3px var(--danger-bg)`；下方 8px 处 `12px` `--danger-text` + `alert-circle` 14px |
| 禁用 | `background: var(--surface)`；`border-color: var(--line)`；`color: var(--text-disabled)`；`cursor: not-allowed` |
| 只读 | 同禁用但 `color: var(--text-secondary)` |
| 等宽输入（代理地址、端口） | `font-family: var(--font-mono)`；`font-size: 13px`；`letter-spacing: 0`；`font-feature-settings: "tnum" 1, "zero" 1` |
| 前后缀 | 前缀图标 16px 绝对定位 `left: 12px`，输入 `padding-left: 36px`；后缀按钮 24×24 `right: 8px` |

### 5.3 Select / Segmented / 开关

**Select**：外观同输入框（高 32/40）；右侧 `chevron-down` 12px 位于 `right: 10px`，`--text-tertiary`；原生 `<select>` 基础上 `appearance: none`。
下拉浮层：`background: var(--surface-overlay)`、`border: 1px solid var(--line-strong)`、`--r-md`、`padding: 4px`、`box-shadow: var(--shadow-overlay)`；项高 `32px`、`padding: 0 8px`、`--r-sm`、`13px`；hover `--surface-hover`；选中项左侧 2px `--brand` 竖条 + 文字 `--text-primary`。

**Segmented**：容器高 32（`--r-md`，`background: var(--surface-raised)`，`border: 1px solid var(--line)`，`padding: 2px`，`gap: 2px`）；段 `padding: 0 14px`、`13px/500`、`--r-sm`；选中 `background: var(--surface-overlay)` + `--shadow-sm` + `--text-primary`；未选 `--text-secondary`；hover（未选）`--text-primary`；滑块用 `transform: translateX()` 移动，`var(--dur-2) var(--ease-std)`（不是背景色切换）。禁用整体 `opacity: .5`。

**开关（Switch）**：轨道 `36×20`，`--r-pill`；关 `background: var(--line-strong)`；开 `background: var(--brand)`；滑块 `16×16` 白色/`--surface`，`top: 2px`，`left: 2px`，开时 `translateX(16px)`；`transition: var(--dur-2) var(--ease-std)`；焦点环同按钮；点击区域最小 `44×44`（用 padding 扩大，不用视觉尺寸）。

### 5.4 Tooltip / Toast

**Tooltip**：`background: var(--surface-overlay)`；`border: 1px solid var(--line-strong)`；`--r-xs`；`padding: 6px 8px`；`12px/16`；`color: var(--text-primary)`；`max-width: 260px`；`box-shadow: var(--shadow-md)`；无箭头（避免像素对不齐）。出现延迟 `400ms`，消失 `0ms`；进入 `opacity 0 → 1` + `translateY(2px → 0)`，`--dur-3 var(--ease-out)`。`z-index: 80`。

**Toast**：右下角堆叠，`right: 24px; bottom: 24px`，宽 `320px`，`gap: 8px`；`background: var(--surface-overlay)`；`border: 1px solid var(--line-strong)`；`border-left: 3px solid var(--x-text)`；`--r-md`；`padding: 12px 12px 12px 14px`；`box-shadow: var(--shadow-lg)`；标题 `13px/600`，正文 `12px/20 --text-secondary`；右侧关闭 20×20。停留 `4000ms`（错误 8000ms，或常驻直到手动关闭）。入场 `translateY(8px)` + 淡入 `--dur-4 var(--ease-out)`；退场 `opacity → 0` `--dur-3 var(--ease-in)`；同时最多 3 条，超出则最早一条被顶出。

### 5.5 卡片 / 抽屉 / 表格

**卡片**：`background: var(--surface)`；`border: 1px solid var(--line)`；`--r-lg`；`padding: 20px`（探测卡 16px）；`box-shadow: var(--shadow-sm)`。可点击卡片 hover：`border-color: var(--line-strong)` + `background: var(--surface-hover)`，`--dur-1`；**不做上浮位移**（避免网格抖动）。

**抽屉**：见 §3.4。`z-index: 60`，遮罩 50，Modal 65，Toast 90，Tooltip 80。

**表格**（证据列表 / 报告校验表）：
`width: 100%`；`border-collapse: separate; border-spacing: 0`；表头 `height: 36px`，`11px/600 .06em uppercase`，`--text-tertiary`，`background: var(--surface-raised)`，`border-bottom: 1px solid var(--line)`；行 `height: 40px`；单元格 `padding: 0 12px`，`13px`；数值列右对齐 + mono；行 hover `background: var(--surface-hover)`；斑马纹**不用**（与 hover 冲突），改用 `border-bottom: 1px solid var(--line-subtle)`；首列 `--text-primary` `500`，其余 `--text-secondary`。

---

## 6. 状态体系

按 `07-ux-interaction.md` §6.6–§6.8 与 PRD §3.1–§3.2，界面上有**五个互不替代的视觉轴**。任何"把两条轴合成一个色块"的实现都视为违反产品定义。

| 轴 | 取值 | 出现位置 | 样式族 |
|---|---|---|---|
| **执行态** Execution | 进行中 / 已完成（由 Verdict 接管）/ 未执行 | 六卡网格、S2 | 状态点 + 进度形态 |
| **Verdict 九态** | `PASS / PASS_WITH_NOTE / SLOW / BLOCKED_BY_POLICY / CHALLENGED / RATE_LIMITED / TIMEOUT / NET_FAIL / SKIPPED` | 卡片徽章、详情结论 | `.badge[data-verdict]` |
| **A 轴** AI 可用性 | 可用 / 受限 / 不可用（仅 P3/P4/P5） | V2 三行、结论句 | `.axis-a` |
| **B 轴** 速度 | 够用 / 勉强 / 不够（仅 P1/P2） | V3 语义句 | `.axis-b` |
| **结果档位** | A 优 / B 良 / C 可用 / D 较差 / E 不可用 + 未评级 | 总分、三个分项 | `.grade` |

> **A 轴与 B 轴不得合并**：首屏「A 轴红 + B 轴绿」是常态（能连、够快，但 AI 被拦），反之「A 轴绿 + B 轴黄」（能用 AI 但看不了 1080p）同样常见。`SLOW` **只属于 B 轴**，永远不映射到 A 轴的"受限"。

### 6.1 执行态（3 态，只描述"跑没跑"）

| 执行态 | 文案 | 视觉 | 说明 |
|---|---|---|---|
| `idle` | 待测 | 空心虚线环 12px（`circle-dashed`），`--text-tertiary` | 与 `SKIPPED` 的区别见 §6.6 |
| `running` | 测试中 | 6px 脉冲点 + 旋转弧，`--brand`，`--glow-running`，左上 2px `--brand-line` 竖条 | 页面上唯一在动的东西 |
| `settled` | —（由 Verdict 接管） | 徽章换成九态之一 | 不给"已完成"单独文案 |

### 6.2 Verdict 九态（PRD §3.1，一个不多一个不少）

| Verdict | 徽章文案 | 归属 | 颜色 token | Dark hex | 图标（12px） | 形状特征（灰度可辨） | 卡片主位显示 |
|---|---|---|---|---|---|---|---|
| `PASS` | 可用 | A 轴·可用 | `--success-text` | `#3FB950` | `check-circle` | 圆内**对勾** | 结论短语 `可用` |
| `PASS_WITH_NOTE` | 可用 · 有附注 | A 轴·可用 | `--success-text` | `#3FB950` | `check-circle` + 右上 4px `alert-triangle` | 对勾 + 角标（**双图形**，灰度下与纯 PASS 可分） | `可用` + 脚注 1 行 |
| `SLOW` | 慢 | **B 轴·勉强** | `--warning-text` | `#E0A63C` | `gauge` | **半圆仪表**外形 | `1.9 s`（数字本身即证据） |
| `BLOCKED_BY_POLICY` | 地区限制 | A 轴·受限 | `--blocked-*` | `#A78BFA` | `slash-circle` | 圆内**斜杠** | `地区限制` |
| `CHALLENGED` | 人机验证 | A 轴·受限 | `--warning-text` | `#E0A63C` | `ring-gap` | **缺口环** | `人机验证` |
| `RATE_LIMITED` | 限流 | A 轴·受限 | `--info-text` | `#6EA8FE` | `ring-gap` + 内部双竖 | 缺口环 + 内符号（≥20px 才显示内符号） | `限流` |
| `TIMEOUT` | 超时 | A 轴·不可用 | `--danger-text` | `#F2555A` | `clock` | **钟表**圆形＋指针（与叉形明确可分） | `超时` |
| `NET_FAIL` | 连不上 | A 轴·不可用 | `--danger-text` | `#F2555A` | `x-circle` | 圆内**叉** + 断裂底环 | `连不上` + 旁注 `subReason` |
| `SKIPPED` | 本次未测 | 不入 A 轴 | `--text-disabled` | `#525A67` | `minus-circle` | 圆内**横线** + 虚线描边 | `本次未测` |

规则：

- **琥珀色在 A 轴（`CHALLENGED`）与 B 轴（`SLOW`）各出现一次**，但两轴永不落在同一张卡片上，不构成歧义；一旦需要同卡展示（P2 卡同时给起播结论与速度结论），B 轴降级为**纯文字**（无徽章），只保留 A 轴徽章。
- `TIMEOUT` 与 `NET_FAIL` 同为红，靠形状+文案区分（超时是"有应答但太慢"，连不上是"没应答"）；两者旁注第一行必须写明责任归因（§6.8）。
- **受限态不能用失败色而不给动作**：`BLOCKED_BY_POLICY` / `CHALLENGED` / `RATE_LIMITED` 三者卡内必须各有一个动作位（§6.3.1 / §6.8），无动作位的受限态视为实现缺陷。

```css
.badge { position: relative; display: inline-flex; align-items: center; gap: 4px;
         height: 22px; padding: 0 8px; border-radius: var(--r-sm);
         font: 500 12px/1 var(--font-sans); border: 1px solid transparent;
         white-space: nowrap; }
.badge .icon { width: 12px; height: 12px; }

/* ---- Verdict 九态 ---- */
.badge[data-verdict="PASS"]               { color: var(--success-text);  background: var(--success-bg);  border-color: var(--success-border); }
.badge[data-verdict="PASS_WITH_NOTE"]     { color: var(--success-text);  background: var(--success-bg);  border-color: var(--success-border); }
.badge[data-verdict="PASS_WITH_NOTE"]::after {           /* 角标：与纯 PASS 的唯一形状差 */
  content: ""; position: absolute; right: -2px; top: -2px; width: 6px; height: 6px;
  background: var(--warning-text);
  clip-path: polygon(50% 0, 100% 100%, 0 100%); }
.badge[data-verdict="SLOW"]               { color: var(--warning-text);  background: var(--warning-bg);  border-color: var(--warning-border); }
.badge[data-verdict="BLOCKED_BY_POLICY"]  { color: var(--blocked-text);  background: var(--blocked-bg);  border-color: var(--blocked-border); }
.badge[data-verdict="CHALLENGED"]         { color: var(--warning-text);  background: var(--warning-bg);  border-color: var(--warning-border); }
.badge[data-verdict="RATE_LIMITED"]       { color: var(--info-text);     background: var(--info-bg);     border-color: var(--info-border); }
.badge[data-verdict="TIMEOUT"]            { color: var(--danger-text);   background: var(--danger-bg);   border-color: var(--danger-border); }
.badge[data-verdict="NET_FAIL"]           { color: var(--danger-text);   background: var(--danger-bg);   border-color: var(--danger-border); }
/* SKIPPED：不计分、不得用通过色、必须虚线描边 */
.badge[data-verdict="SKIPPED"]            { color: var(--text-tertiary); background: transparent;
                                            border: 1px dashed var(--line-strong); }

/* ---- 执行态 ---- */
.badge[data-exec="idle"]    { color: var(--text-tertiary); background: var(--surface-raised); border-color: var(--line); }
.badge[data-exec="running"] { color: var(--brand-text);    background: var(--brand-soft);     border-color: var(--brand-line); }
```

> `--blocked-*` 已定义于 §2.14：dark `#A78BFA / #211C33 / #3E3460`；light `#6D28D9 / #EFEAFB / #CFC3EE`。`SKIPPED` 刻意**无底色**，用虚线边框与 `idle` 的实线边框形成灰度可分差异（虚线 vs 实线，不依赖颜色）。

```css
@keyframes pulse { 0%,100% { opacity: 1 } 50% { opacity: .35 } }
.badge[data-exec="running"] .dot {
  width: 6px; height: 6px; border-radius: var(--r-pill);
  background: var(--brand); animation: pulse var(--dur-pulse) var(--ease-std) infinite;
}
```

### 6.3 A 轴：AI 可用性三态（只作用于 P3/P4/P5）

**对 ux-arch 的明确回答：「半填充」在小尺寸下不可辨，不采用。**

实测判断：`8px` 圆点内做 50% 半填充，在 2× DPR 下填充区仅 16×8 物理像素，边界锯齿会把读成"脏点"而非"半个"；`10px` 仍临界；`14px` 以上才稳定，但 V2 每行主行仅 22px，14px 标记会把行撑开。**裁定：改用「缺口圆环（90° 缺口，缺口居中于 135° 左上）」**，12px 下限——同尺寸下"缺一段弧"的可辨性比"填一半"高约一个数量级，且与 `NET_FAIL` 的叉圆同母题，全站形状语言自洽。

| A 轴 | 形状（12px，stroke 1.5） | 形状命名 | 颜色（唯一彩色通道） | 文案 | 旁注行 | V2 进度形态 |
|---|---|---|---|---|---|---|
| 可用 | 实心圆 `r=4`，无外环 | `solid-dot` | `--success-text` | 可用 | **无** | 实线到 100% |
| 受限 | 空心环 `r=5`，扫过 270° | `ring-gap` | `--warning-text` | 受限 | **有**（1 行，说明"被什么拦住"） | 实线到 60% + 末端 2px 渐隐缺口 |
| 不可用 | 空心环 + 贯穿斜杠 | `ring-slash` | `--blocked-text` | 不可用 | 有（1 行 + 动作位） | 只画虚线轨道，无填充 |

**视觉通道计数（要求 ≥2、彩色最多占 1）**：① 形状（实心点 / 缺口环 / 斜杠环）② 文案首字（可 / 受 / 不）③ 旁注行存在性（无 / 有 / 有+动作）④ 进度形态（满 / 断口 / 空缺）⑤ 颜色（1 个）。共 5 通道、彩色 1 个 —— 灰度截图与三类色盲下均成立。

**禁止**：退回红绿二元；用底色块铺满整行；给三态各配 emoji 或独立色块。

#### 6.3.1 受限带内的三个子变体（能否在 16–20px 做出来）

ux-arch 建议"同一图形母题 + 不同内部符号"，并要我判断可行性。**结论：16px 以下不可行，20px 及以上可行 —— 因此采用双尺度策略。**

计算依据：圆内可画区直径 = `D − 2×1.5`。`CHALLENGED` 需要画"?"，其最小可辨高度约 7px（问号上下两个笔画间距需 ≥1px 才不糊）；`16px` 时内径仅 13px，问号笔画会被压到 0.8px，判为不可辨。`20px` 时内径 17px，问号 9px 高可辨。因此：

| 尺寸 | `BLOCKED_BY_POLICY` | `CHALLENGED` | `RATE_LIMITED` | 变体识别靠什么 |
|---|---|---|---|---|
| **12–16px**（卡片徽章、V2 三行） | 斜杠环 | 纯缺口环（**无内符号**） | 纯缺口环（**无内符号**） | 母题统一为"环"，变体靠 ① 文案（地区限制 / 人机验证 / 限流，首字「地/人/限」互异）② 动作前缀（换线路 / 手动过验证 / 稍后重试）③ 旁注行内容 ④ 颜色（紫/琥珀/蓝） |
| **≥20px**（详情结论、截图摘要） | 斜杠环 | 缺口环 + 内问号 | 缺口环 + 内双竖 | 母题同一，内部符号区分，颜色仅辅助 |

这样"带级识别"（受限）在任何尺寸都成立（环 = 受限），"变体识别"在小尺寸退回 4 个非颜色通道、在大尺寸升级为符号 —— **不退回二元，也不假装 16px 能画出问号**。

```css
.axis-a { display: inline-flex; align-items: center; gap: 6px; }
.axis-a__mark { width: 12px; height: 12px; flex: 0 0 12px; }
.axis-a__text { font: 500 14px/22px var(--font-sans); color: var(--text-primary); }
.axis-a[data-a="ok"]      .axis-a__mark { color: var(--success-text); }
.axis-a[data-a="limited"] .axis-a__mark { color: var(--warning-text); }
.axis-a[data-a="down"]    .axis-a__mark { color: var(--blocked-text); }
/* 旁注行：ok 时仍占位（visibility:hidden），保证三行等高 */
.axis-a__note { height: 18px; font: 400 12px/18px var(--font-sans); color: var(--text-secondary); }
.axis-a__note:empty { visibility: hidden; }
/* ≥20px 才渲染内符号 */
.axis-a--lg .axis-a__mark { width: 20px; height: 20px; }
.axis-a--lg[data-variant="challenged"] .axis-a__inner,
.axis-a--lg[data-variant="rate_limited"] .axis-a__inner { display: inline; }
.axis-a__inner { display: none; }
```

占比与对齐：

- V2 三行每行 `height: 40px`（22px 主行 + 18px 旁注行），`display: grid; grid-template-columns: 12px 1fr auto; column-gap: 8px; align-content: center`；`可用` 行旁注用 `visibility:hidden` 占位，行高恒 40px —— **不许**压缩，否则三行基线不齐。
- 三行之间 `border-bottom: 1px solid var(--line-subtle)`，最后一行无；三行整体左对齐于结论卡主列，不居中。

### 6.4 B 轴：速度三值（只作用于 P1/P2，永不映射到 A 轴）

B 轴不使用徽章，使用 `SpeedVerdict` 语义句 + **三段条**（形状即通道，不靠颜色）。原因：速度是连续量，用徽章会诱导用户把它与"可用性"并列比较。

| B 轴 | 语义句（示例，数字用业务语言） | 三段条 | 色 | 图标 |
|---|---|---|---|---|
| 够用 | 够开 1080p 视频，下载峰值 96 Mbps | 3/3 段填充 | `--success-text` | `gauge` 指针偏右 2/3 |
| 勉强 | 能用，但 4K 会缓冲，1080p 起播要等 1–2 秒 | 2/3 段填充 | `--warning-text` | `gauge` 指针居中 |
| 不够 | 打开 AI 网页都费力，建议换线路 | 1/3 段填充 | `--danger-text` | `gauge` 指针偏左 1/3 |

- 三段条：`width: 72px; height: 6px`，由 3 个 `22px × 6px` 的 `--r-xs` 小段组成，`gap: 3px`；未填充段 `--line-subtle`。**段数**是主通道，颜色是辅助。
- 句内**禁止**出现 `p50 / p90 / TTFB / CV`（ux-arch 首屏禁用词）；这些只在详情层指标表出现。
- 语义句字号 `15px/22 500`，`--text-primary`；括号内的补充说明 `13px 400 --text-tertiary`。

### 6.5 `NET_FAIL` 的六个子原因（文案 + 唯一动作）

`subReason` 不占独立图标（避免图标爆炸），由**卡片旁注行首词 + 详情结论句**承载；`tls_cert` 与 `truncated` 两条必须与普通失败在文案上有质差。

| `subReason` | 旁注首词 | 完整文案 | 唯一动作（禁止给第二个动作） |
|---|---|---|---|
| `dns_fail` | DNS 解析失败 | 域名解析失败。可能是 DNS 污染，或代理未接管 DNS 请求。 | `改用 socks5h（远程解析）重测` |
| `tcp_refused` | 端口拒绝 | 目标端口拒绝连接。通常是对端未监听，或代理规则主动拒绝了该请求。 | `检查代理规则` |
| `tcp_reset` | 连接被重置 | 连接被中途重置。常见于中间网络设备阻断。 | `更换线路重测` |
| `tls_cert` | 证书校验失败 | 证书校验失败。可能是系统时间偏差，或代理正在解密流量。 | **只给** `查看证书详情` —— 严禁出现"忽略证书继续"（PRD 第 6 章 #5） |
| `tls_handshake` | 握手未完成 | TLS 握手未完成。可能是协议版本不兼容或 SNI 被干扰。 | `查看握手详情` |
| `truncated` | 内容被截断 | 返回了正常状态码，但页面内容不符合预期，可能被中间层替换或截断。 | `查看响应片段` |

- `tls_cert` 的旁注行额外加一个 `shield-check` 14px 图标（`--warning-text`），提示"这是安全相关"，但不升级为独立状态。
- `truncated` 是"假通"：**其 Verdict 必须是 `NET_FAIL`，绝不能判 `PASS`**；卡片主位显示 `连不上` 会误导，故该子原因的主位改为 `内容异常`（`--danger-text`），旁注首词 `内容被截断`。

### 6.6 `SKIPPED` 卡形态 vs 加载中骨架（两者都不抢注意力，但语义相反）

| 维度 | `SKIPPED`（本次未测） | 骨架（正在等数据） |
|---|---|---|
| 卡片描边 | `1px dashed var(--line-strong)` | `1px solid var(--line)` |
| 底色 | `transparent`（比正常卡更"空"） | `var(--surface)` |
| 主位 | 文案 `本次未测`（`17px/600 --text-tertiary`） | 骨架条 `12×64px`（`--line-subtle`，1400ms opacity 呼吸） |
| 徽章 | `本次未测`（虚线边框、无底色） | `测试中`（brand 底 + 脉冲点） |
| 动效 | **无任何动效** | 有呼吸动效 |
| 卡片整体 | `opacity: .72`（不降到 .6，否则文字对比度跌破 4.5） | `opacity: 1` |
| 脚注 | 必写原因：`因字节预算跳过 / 因并发限制跳过` | 无 |

**硬规则**：`SKIPPED` 不计分（不参与总分与分项分），**不得使用任何通过色**（绿），且必须显式出现"未测"字样——只画一个灰点不算。

### 6.7 结果档位 A–E（PRD §3.2，分数旁必须有档位词）

| 档位 | 分数区间 | 档位词 | 色 | 位置 |
|---|---|---|---|---|
| A | 85–100 | 优 | `--success-text` | 总分环内数字右侧、三分项卡分数右侧 |
| B | 70–84 | 良 | `--brand-text` | 同上 |
| C | 55–69 | 可用 | `--warning-text` | 同上 |
| D | 30–54 | 较差 | `--danger-text` | 同上 |
| E | 0–29 | 不可用 | `--danger-text`（加重：数字同色） | 同上 |
| — | 有效项 <3 | **未评级** | `--text-tertiary` | 同上，且整块降为 `opacity .85` |

**组合形态（强制）**：`分数 + 档位词` 双元素，例 `72 · 良`、`82 · 良`。只给数字 = 违反 PRD。

```css
.grade { display: inline-flex; align-items: baseline; gap: 6px; }
.grade__score { font: 600 24px/30px var(--font-mono); font-variant-numeric: tabular-nums; color: var(--text-primary); }
.grade__word  { font: 500 13px/20px var(--font-sans); padding: 1px 6px; border-radius: var(--r-xs);
                border: 1px solid currentColor; }        /* 描边胶囊，不是实心底 */
.grade[data-g="A"] .grade__word { color: var(--success-text); }
.grade[data-g="B"] .grade__word { color: var(--brand-text); }
.grade[data-g="C"] .grade__word { color: var(--warning-text); }
.grade[data-g="D"] .grade__word { color: var(--danger-text); }
.grade[data-g="E"] .grade__word { color: var(--danger-text); font-weight: 600; }
.grade[data-g="none"] .grade__word { color: var(--text-tertiary); border-style: dashed; }
.grade[data-g="none"] .grade__score { display: none; }    /* 未评级不显示分数，避免"0 分"误读 */
```

**与三态的词面冲突处理（ux-arch 要求）**：档位词与 A 轴三态词都有"可用/不可用"。两者**样式族完全不同、位置不同、永不并列**：

| | 档位 | A 轴三态 |
|---|---|---|
| 形状 | 描边胶囊文字（`1px` 边框、无图形） | 实心点 / 缺口环 / 斜杠环（纯图形 + 文字） |
| 位置 | 分数**右侧紧邻**（`gap: 6px`） | 卡片徽章位（卡片头右上）、V2 三行行首 |
| 字号 | `13px/500` | `14px/500`（徽章 `12px/500`） |
| 前缀 | 必带分数（`72 · 良`） | 永不带分数 |
| 文案集合 | 优/良/可用/较差/不可用/未评级 | 可用/受限/不可用 |

即：**看到数字跟一个描边小词 = 档位；看到图形跟一个词 = 三态**。两者同屏时至少相隔一整块布局（结论卡 vs V2 行）。

**"样本不足，暂不评级"独立形态**：`data-g="none"`，只显示 `样本不足，暂不评级`（`13px/500 --text-tertiary` + 虚线胶囊），**不复用 E 档视觉**（E 是红色、有分数），也不显示 `0`。

---

### 6.8 `FailureAttribution` 失败归因（五类，party 必填）

每个失败/受限探测项都要给出三段式：**归因行 → 动作位 → 排除法建议**。它不是 Toast，而是**卡内常驻块**（卡片被展开时）或**详情结论段**。

| `party` | chip 文案 | 图标 | 一句话（归因依据） | 排除法建议（`13px --text-secondary`） |
|---|---|---|---|---|
| `user_config` | 用户配置 | `sliders` | 代理地址/认证/规则配置有误 | 先切「直连对照」跑一遍，若直连正常则问题在配置 |
| `proxy` | 代理链路 | `shield-check` | 代理进程或上游节点异常 | 关闭代理直连重测；若结果反转，问题在代理 |
| `target` | 目标服务 | `alert-triangle` | 对端服务不可达或拒绝当前地区 | 用同一代理访问其他站点，若正常则问题在目标 |
| `system` | 系统环境 | `gauge` | 系统时间/DNS/证书库/IPv6 状态异常 | 检查系统时间与 DNS 设置后重测 |
| `network` | 网络运营商 | `globe` | 出口链路上的中间设备干扰 | 换手机热点对比，若正常则问题在运营商 |

**排版按最长的 party 设计**：`网络运营商`（5 字 × 13px ≈ 65px）为基准，chip `min-width: 96px`（含图标 14px + gap 6px + 左右 padding 8px），五个 chip 因此等宽对齐，不与后面的动作位抢位。

```css
.attr { display: grid; grid-template-columns: 96px 1fr; column-gap: 12px; row-gap: 6px; align-items: center; }
.attr__chip { display: inline-flex; align-items: center; justify-content: center; gap: 6px;
              height: 24px; padding: 0 8px; border-radius: var(--r-sm);
              background: var(--surface-raised); border: 1px solid var(--line);
              color: var(--text-secondary); font: 500 13px/1 var(--font-sans); white-space: nowrap; }
.attr__say  { font: 400 13px/20px var(--font-sans); color: var(--text-secondary); }
.attr__act  { grid-column: 2; height: 24px; display: inline-flex; align-items: center; gap: 4px;
              font: 500 13px/1 var(--font-sans); color: var(--text-primary);
              background: none; border: 0; padding: 0; cursor: pointer; }
.attr__act:hover { color: var(--brand-text); }
.attr__hint { grid-column: 2; font: 400 12px/18px var(--font-sans); color: var(--text-tertiary); }
```

规则：

- 五个 party **一律中性色**，不给各自配色（否则五色 = 仪表盘）；识别靠图标 + 文案。
- `party` 必填，**不设"未知"兜底位**；若引擎无法判定，前端渲染 `系统环境` 并附 `判定不充分` 的 `DegradedFlag`，而不是留空。
- 一组失败最多 1 个动作位；同一屏 ≥3 条红灯时只第一条渲染动作，其余折叠为 `共 3 项需处理 ›`。

### 6.9 四种进度形态（不可统一为一个进度条）

ux-arch §4.3 明确四类进度结构不同、高度不同。它们**同时出现**在 S2 上，必须能一眼分清，因此形态差异远大于颜色差异。

| 类型 | 用于 | 结构 | 高度 | 主通道 |
|---|---|---|---|---|
| **吞吐类** | P1 大文件下载 | 时间进度条（`已用/总时长`）+ 右侧字节辅信息 `12.4 / 20 MB` | 条 6px，整块 24px | 条长（时间） |
| **可达类** | P3/P4/P5 | 五段 stepper（`DNS / TCP / TLS / TTFB / 传输`），段间 `4px` 间隙 | 段 4px，整块 20px | **段数点亮**（不连续） |
| **采样类** | P6 基线 | `n / N` 点阵（每个点 4px 圆，`gap 3px`），20 次采样 = 20 点 | 14px | **点数** |
| **全局类** | 顶部总览 | 连续条，按"已完成项数 / 期望项数"取值 | 6px | 条长（离散跳变） |

- 吞吐 / 全局用连续条，但全局条的 `transition: width 240ms linear` 只在**整项完成**时跳一格，不做平滑内插（防"假进度"）。
- 可达 stepper 的段颜色：已过 `--brand`；当前段 `--brand` + 600ms 脉冲；未到 `--line-subtle`；失败段 `--danger-text` 且 stepper 之后不再点亮（**中断位置本身是信息**，与 A 轴进度形态一致）。
- 采样点阵：已完成点 `--brand`；失败点 `--danger-text`；未测点 `--line-subtle` 空心。
- 四者**都不含百分比文字**（百分比只在全局条右侧以 `4 / 6` 形式出现，避免 "67%" 这种无意义精度）。

### 6.10 其余新增视觉位

| 组件 | 用途 | 关键规格 |
|---|---|---|
| `DegradedFlag` | 降级标记（跳过证书/明文代理/疑似劫持/样本不足/凭据未落盘/时钟漂移） | 14px 图标（`alert-triangle`）+ `12px/500 --warning-text`，**无底色无边框**，`cursor: help` + Tooltip 400ms；常驻但零动效。出现位置：`ConfigSummary` 段尾、详情对应行行尾、三分项卡脚注前。强度序列：failed（红底徽章）> `DegradedFlag`（无底黄字）> 普通提示（灰字） |
| `TrafficBudget` | 预估 / 已用 / 实际流量 | 单行 `12px mono`：`预估 22 MB · 已用 18.4 MB · 实际 19.1 MB`；仅在 S2 实时面板右下角与报告页出现；超预估 15% 时"实际"数字转 `--warning-text` |
| `ComparePanel` | 代理 vs 直连双列 | 两列各 `1fr`，`column-gap: 24px`；每行：指标名（`13px --text-secondary`）+ 左值 + 右值 + **Δ 列** + **方向列**。方向列宽 `56px`，用 `↑` / `↓` / `=` + 语义（`快 32%`），**箭头颜色按"好/坏"而非数值大小**：变好 `--success-text`、变坏 `--danger-text`、持平 `--text-tertiary`。方向列不可省略（不同指标好坏方向相反，如延迟下降是好事） |
| `SpeedVerdict` | V3 业务语义句 | 见 §6.4；`15px/22 500` + 三段条 |
| 直连窄色带 | 直连模式的常驻标识 | 见下 |

**直连模式三处常驻标识**（同一模式，三个不同强度，不重复喊）：

1. **Header 徽章**（28px 高 pill）：`⇢ 直连（绕过代理）`，`--surface-raised` 底 + `--line-strong` 描边 + `--text-primary` 文字，图标 `slash-circle` 14px。**不使用黄色或红色**（那是告警语义）。
2. **ConfigSummary 首词**：`直连` 作为第一段文字（`13px/600`，`--text-primary`），其余段照常。
3. **结论区顶部窄色带**：`height: 3px; width: 100%`，`background: var(--line-strong)`（**中性灰，不是彩色**），置于结论卡 `border-radius` 之上（`border-top-left/right-radius` 同步 12px）；Tooltip 全文 `本次测试走直连，未经过代理`。它必须"持续可见但不像告警"——做法是：**只有 3px、只有中性灰、只占结论卡顶边**，靠位置（总在内容最上方）而非颜色被感知。

> 若对比测试同时存在代理与直连两列，则第 3 项窄色带改为左右双色（左 `--brand`、右 `--line-strong`），各占 50%，并在 `ComparePanel` 表头重复一次列标识。

---

## 7. 图标

风格：线性 stroke，`stroke-width: 1.5`，24×24 viewBox，`linecap/linejoin: round`，`fill: none`，`currentColor`。显示尺寸只允许 **12 / 14 / 16 / 18 / 20**（整数，与字号匹配）。禁止在同一界面混用实心与线性图标（**例外**：三态标记中的 `solid-dot` 为实心，因为它是唯一的"闭合"语义，且不与线性图标并列成组）。

| # | 名称 | 用途 | 尺寸 |
|---|---|---|---|
| 1 | `play` | 开始测试 | 16 |
| 2 | `stop` | 停止 | 14 |
| 3 | `refresh` | 重测 | 14 |
| 4 | `sliders` | 设置 | 18 |
| 5 | `download` | P1 / 导出 | 16 |
| 6 | `gauge` | 速度 | 16 |
| 7 | `globe` | 国际链路 / 本地归因 | 14 / 16 |
| 8 | `shield-check` | 代理 / 凭据降级 | 14 / 15 |
| 9 | `alert-triangle` | 降级 / 目标归因 | 14 |
| 10 | `check-circle` | `ok` | 12 / 16 |
| 11 | `x-circle` | `failed` | 12 / 16 |
| 12 | `slash-circle` | `blocked` / `ring-slash`（不可用） | 12 / 16 |
| 13 | `ring-gap` | 受限（Verdict） | 12 |
| 14 | `solid-dot` | 可用（Verdict） | 12 |
| 15 | `circle-dashed` | `pending` | 12 |
| 16 | `minus-circle` | `skipped` | 12 |
| 17 | `chevron-down` | 下拉 / 详情（`chevron-right` = 旋转 −90°） | 12 |
| 18 | `copy` | 复制 JSON | 14 |
| 19 | `clock` | `TIMEOUT` | 12 / 16 |

```html
<svg width="0" height="0" style="position:absolute" aria-hidden="true" focusable="false">
 <defs>
  <symbol id="i-play" viewBox="0 0 24 24"><path d="M9 6.5 18 12l-9 5.5z"/></symbol>
  <symbol id="i-stop" viewBox="0 0 24 24"><rect x="7" y="7" width="10" height="10" rx="1.5"/></symbol>
  <symbol id="i-refresh" viewBox="0 0 24 24"><path d="M20 12a8 8 0 1 1-2.34-5.66"/><path d="M20 4v4h-4"/></symbol>
  <symbol id="i-sliders" viewBox="0 0 24 24"><path d="M4 8h5M13 8h7M4 16h3M11 16h9"/><circle cx="11" cy="8" r="2"/><circle cx="9" cy="16" r="2"/></symbol>
  <symbol id="i-download" viewBox="0 0 24 24"><path d="M12 4v10M8 11l4 4 4-4M5 19h14"/></symbol>
  <symbol id="i-gauge" viewBox="0 0 24 24"><path d="M4 17a8 8 0 1 1 16 0M12 17l4-5"/></symbol>
  <symbol id="i-globe" viewBox="0 0 24 24"><circle cx="12" cy="12" r="8"/><path d="M4 12h16M12 4c3 3.5 3 12.5 0 16M12 4c-3 3.5-3 12.5 0 16"/></symbol>
  <symbol id="i-shield-check" viewBox="0 0 24 24"><path d="M12 3l7 3v5c0 4.5-3 7.5-7 9-4-1.5-7-4.5-7-9V6z"/><path d="M9 12l2 2 4-4"/></symbol>
  <symbol id="i-alert-triangle" viewBox="0 0 24 24"><path d="M12 4.5 21 19H3z"/><path d="M12 10v4M12 16.5h.01"/></symbol>
  <symbol id="i-check-circle" viewBox="0 0 24 24"><circle cx="12" cy="12" r="8"/><path d="M8.5 12.5l2.5 2.5 4.5-5"/></symbol>
  <symbol id="i-x-circle" viewBox="0 0 24 24"><circle cx="12" cy="12" r="8"/><path d="M9.5 9.5l5 5M14.5 9.5l-5 5"/></symbol>
  <symbol id="i-slash-circle" viewBox="0 0 24 24"><circle cx="12" cy="12" r="8"/><path d="M6.5 17.5l11-11"/></symbol>
  <symbol id="i-circle-dashed" viewBox="0 0 24 24"><circle cx="12" cy="12" r="8" stroke-dasharray="2.5 3"/></symbol>
  <symbol id="i-minus-circle" viewBox="0 0 24 24"><circle cx="12" cy="12" r="8"/><path d="M8.5 12h7"/></symbol>
  <symbol id="i-chevron-down" viewBox="0 0 24 24"><path d="M6 9.5l6 6 6-6"/></symbol>
  <symbol id="i-copy" viewBox="0 0 24 24"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M6 15H5a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h9a1 1 0 0 1 1 1v1"/></symbol>
  <symbol id="i-ring-gap" viewBox="0 0 24 24">
    <path d="M17.5 6.5A7 7 0 1 0 6.5 17.5"/></symbol>
  <symbol id="i-solid-dot" viewBox="0 0 24 24"><circle cx="12" cy="12" r="5" fill="currentColor" stroke="none"/></symbol>
  <symbol id="i-spinner" viewBox="0 0 24 24"><path d="M12 4a8 8 0 0 1 8 8" /></symbol>
  <symbol id="i-clock" viewBox="0 0 24 24"><circle cx="12" cy="12" r="8"/><path d="M12 8v4.5l3 1.8"/></symbol>
 </defs>
</svg>
```

使用：

```html
<svg class="icon" width="14" height="14" aria-hidden="true"><use href="#i-globe"/></svg>
```

必须配这条 CSS（否则 symbol 内路径没有描边）：

```css
.icon {
  fill: none; stroke: currentColor; stroke-width: 1.5;
  stroke-linecap: round; stroke-linejoin: round;
  vertical-align: -0.125em; flex: 0 0 auto;
}
.icon--filled { fill: currentColor; stroke: none; }  /* 仅 #i-solid-dot 使用 */
```

- `#i-ring-gap` 的缺口在左上 135°：整圆 360° − 90° = 270° 弧，起点 `17.5 6.5`（≈ -45°→ +135° 方向），12px 下缺口宽度 = 3px，可辨。
- `#i-ring-slash` 改用 `#i-slash-circle`（同一个 symbol），保证"不可用"与"blocked"形状同源。
- 所有 `<symbol>` 内路径继承 `stroke-width: 1.5`；`12px` 渲染时**不要**改 `stroke-width`（会变成 0.75 CSS px，在 2× 屏仍清晰，在 1× 屏略细但可接受）。
- 图标与文字必须 `vertical-align: -0.125em` 或 `align-items: center`；不允许出现 1px 视觉错位。

---

## 8. 动效规范

总原则：**动效只解释状态变化，不装饰**。同一时刻页面上并行运行的动画不超过 2 个（`running` 脉冲 + 1 个进度），超过即视为噪声。

| 场景 | 属性 | 时长 / 缓动 | 说明 |
|---|---|---|---|
| 首屏入场 | `opacity 0→1` + `translateY(8px)→0` | `280ms / var(--ease-out)` | 标题区、控制卡、预览网格依次 `stagger 40ms`；总时长 ≤ 400ms，**不**做逐字入场 |
| 结果页入场 | 同上，区块 stagger `60ms` | `280ms / var(--ease-out)` | 结论卡 → 三分项 → 卡片网格，三批 |
| 评分环绘制 | `stroke-dashoffset` | `420ms / var(--ease-out)` | 只播一次；`prefers-reduced-motion` 下直接跳到终值 |
| 分项进度条 | `width` | `320ms / var(--ease-out)` | 结果页入场时延迟 120ms 启动 |
| 实时进度条 | `width` | `240ms / linear` | 跟随真实完成度，**不做**缓动内插（禁止"假进度"） |
| 数值更新 | **无动画** | 0ms | 只改 `textContent`；等宽字体保证零位移。全站唯一例外：结论句首字的显隐用 `120ms` 淡入 |
| 卡片状态切换 | `border-color` + `box-shadow` | `140ms / var(--ease-std)` | 对比度变化避免闪烁；`running → ok` 时徽章交叉淡入 `120ms` |
| 进度条完成瞬间 | 一次 `scaleY(1→1.4→1)` | `220ms / var(--ease-out)` | 仅全局进度条，做一次，不循环 |
| 骨架屏 | `opacity .45→.85→.45` | `1400ms / var(--ease-std) infinite` | 只动 opacity，不动背景位置（避免 `background-position` 触发重绘） |
| 抽屉进入（覆盖） | `translateX(24px)→0` + `opacity` | `280ms / var(--ease-out)` | 遮罩 `opacity 0→0.48` 同步 |
| 抽屉进入（并列） | `width 0→480px` | `280ms / var(--ease-std)` | 网格 `margin` 跟随，不用 `translate` |
| 抽屉退出 | `opacity →0`，`translateX(0→12px)` | `200ms / var(--ease-in)` | 退出永远比进入快 |
| Modal | `scale(.98)→1` + `opacity` | `280ms / var(--ease-out)` | 原点居中 |
| Tooltip | `opacity` + `translateY(2px→0)` | `200ms / var(--ease-out)` | 出现延迟 400ms；消失 0ms 立即 |
| Toast 进入 | `translateY(8px)→0` + `opacity` | `280ms / var(--ease-out)` | 堆叠用 `gap` 而非位移补间 |
| Toast 退出 | `opacity` + `translateX(8px)` | `200ms / var(--ease-in)` | 移除后其余项用 `140ms / var(--ease-std)` 补位 |
| Tab 指示条 | `transform: translateX + width` | `200ms / var(--ease-std)` | 不用 `left/width` 触发重排（用 `transform` + `scaleX` 或 CSS 变量） |
| 展开/折叠（ConfigSummary） | `max-height` + `opacity` | `280ms / var(--ease-std)` | `max-height` 需给确定终值（如 `200px`），否则缓动失真 |
| `running` 脉冲 | `opacity 1→.35→1` | `1600ms / var(--ease-std) infinite` | 只作用于 6px 状态点，不作用于整卡 |
| 焦点环出现 | 立即（`0ms`） | — | 键盘可达性不允许延迟 |

```css
@keyframes skeleton { 0%,100% { opacity: .45 } 50% { opacity: .85 } }
@keyframes spin { to { transform: rotate(360deg) } }

@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    animation-duration: .01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: .01ms !important;
    scroll-behavior: auto !important;
  }
  /* 例外：进度条与曲线仍需反映真实数据，但用瞬时更新 */
  .chart-cursor-pulse, .badge[data-exec="running"] .dot { animation: none; }
}
```

> 实时曲线在减弱动效下**照常重绘**（数据变化是信息本身，不是动画）；因此 `canvas` 的 `requestAnimationFrame` 循环不受 `reduce` 影响，但需把帧率限制到最多 10fps 以降低视觉刺激。

---

## 9. 浅色模式差异要点

浅色不是"深色反相"，以下 8 条是必须单独处理的差异，其余 token 直接替换即可。

1. **层级方向不变，但承担者变了**：浅色下 `--surface`(#FFFFFF) 比 `--canvas`(#FAFAFB) 更亮，卡片"浮起"靠 `--shadow-sm` + `--line` 双层；深色下卡片几乎无阴影、只靠描边。因此 `--shadow-sm/md` 在浅色必须真的可见（已给带蓝调的 `rgba(16,22,34,.06/.10)`，不用纯黑，避免发灰）。
2. **描边加深**：浅色 `--line` `#D9DCE2` 在纯白上只有 1.37:1，靠它分区会发虚；因此浅色模式下卡片描边统一升到 `--line`（不要降级到 `--line-subtle`），而 `--line-subtle` 只用于表格横线。
3. **语义底必须更淡**：浅色 `-bg` 用 **10%** 叠底（深色用 12%），否则色块过重、文字对比度反而下降；且浅色 `-text` 比深色版深得多（如 success `#15682D` vs `#3FB950`），**不能**复用深色 hex。
4. **品牌动作**：深色 `#21BFAE` + 近黑字；浅色 `#0F766E` + 白字（5.47:1）。浅色**不要**用亮青绿做填充（白字压 `#21BFAE` 只有 2.3:1）。
5. **发光全部禁用**：`--glow-running` 在浅色降为无模糊的 `0 0 0 3px rgba(15,118,110,.12)`；`--shadow-overlay` 加重到 `.20` 以保证抽屉与白底分层。
6. **图表面积填充减弱**：`--chart-fill-from` 由 .16 降到 .14（浅色下同样的 alpha 会显得更重）；网格线保持 `#ECEDF0`，不加深。
7. **hover 方向相反**：深色 hover = 变亮（`--surface-hover` 更浅），浅色 hover = 变暗（`#F1F2F4`）；active 同理。`--surface-hover/active` 已按此定义，实现时不要用 `filter: brightness()` 统一处理。
8. **状态点与徽章**：浅色下 `blocked` 由紫 `#A78BFA` 改为 `#6D28D9`、底 `#EFEAFB`、边 `#CFC3EE`；`skipped` 文字 `#9AA1AC` 在 `#FFFFFF` 上仅 2.6:1，**只允许**出现在已置灰（`opacity .6`）且不可交互的卡上。

切换实现：`<html data-theme="light">`；初始值由 `prefers-color-scheme` 决定（`跟随系统` 档监听 `matchMedia` 变化）；切换时不加任何过渡（避免整页闪白），只对 `background-color`/`color` 给 `120ms` 过渡。

---

## 10. 无障碍

### 10.1 对比度实测（关键组合）

| 组合 | Dark | Light | 判定 |
|---|---|---|---|
| `text-primary` / surface | **15.41:1** | **18.11:1** | AAA |
| `text-secondary` / surface | **8.10:1** | **8.14:1** | AAA |
| `text-tertiary` / surface | **5.16:1** | **5.15:1** | AA（正文） |
| `text-tertiary` / raised | 4.90:1 | **4.72:1** | AA |
| `brand-ink` / brand（主按钮） | **8.26:1** | **5.47:1** | AAA / AA |
| `brand-text` / surface | **8.15:1** | **5.47:1** | AAA / AA |
| success text / 其 12% 底 | **6.22:1** | **5.75:1** | AA |
| warning text / 其 12% 底 | **7.11:1** | **5.63:1** | AAA / AA |
| danger text / 其 12% 底 | **4.90:1** | **5.97:1** | AA |
| info text / 其 12% 底 | **6.46:1** | **6.77:1** | AA |
| `line-control` / surface（控件边界，需 3:1） | **3.14:1** | **3.43:1** | AA(1.4.11) |
| 焦点环 `brand` / canvas | **8.56:1** | **3.64:1** | AA(3:1) |

不达标项与处置：`text-disabled` 2.60–2.69:1 —— 仅用于 disabled 控件（WCAG 豁免）；`line`/`line-subtle` 1.2–1.4:1 —— 纯装饰分隔线，不承载边界语义。**除这两类外，任何承载信息的文字与边界都不得低于 4.5:1 / 3:1。**

### 10.2 焦点与键盘

- 焦点环：`box-shadow: var(--focus-ring)`（内层 `canvas`/`surface` 色 2px + 外层 brand 2px），**不使用** `outline`，保证圆角完整；`outline: none` 只在同时给出 `box-shadow` 时允许。
- 焦点顺序 = 视觉顺序：`topbar 出网 Pill → 主题 → 设置 → Segmented → 输入框 → 时长 → CTA → 六卡（每卡一个 tabindex=0，内部不单独 tab）→ 详情关闭 → Tabs → 内容`。
- `Esc` 关闭抽屉/Modal/Tooltip；抽屉内 `Tab` 循环不逃逸（focus trap），关闭后焦点回到触发卡片。
- 卡片作为复合控件：`role="button"` + `aria-expanded`，内部 `<h3>` 用 `aria-labelledby` 关联，避免读屏读出"一坨文本"。
- 最小命中区：桌面图标按钮视觉 28×28，实际命中区 `≥24×24`（WCAG 2.5.8）；表单控件高度 ≥32px。

### 10.3 流式更新的读屏策略

- 实时数值区**不加** `aria-live`（每 250ms 播报会致灾）。
- 只给两处 `aria-live="polite"`：① 全局状态行（"正在测试 4/6 完成"）—— 用节流，最多每 **3s** 更新一次文本节点；② 结论句（结果页出现时一次性播报）。
- `role="status"` 用于 Toast 容器，`role="alert"` 仅用于致命错误。
- 进度条：`<div role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow="67" aria-valuetext="4 / 6 项完成">`。

### 10.4 其他

- 文本缩放：所有尺寸用 `px`（本工具是桌面固定密度），但布局必须能在浏览器 **200% 缩放**下不重叠 —— 卡片 `min-width: 360px` 是关键防线；`line-height` 全部用 `px` 而非 `1.x`，避免缩放时行高漂移。
- 色盲：见 §6 的多轴方案（九态 + A/B 两轴 + 三子变体均为多通道）；另提供 `prefers-contrast: more` 时把 `--line` 提升到 `--line-control`、`--text-tertiary` 提升到 `--text-secondary`。
- 灰度可读性验收：截图去色后，**九态 + A/B 两轴 + 受限三子变体 + 五档档位**必须仍能靠形状/位置/文字区分，无一处仅依赖颜色（验收用例见 `09-test-plan.md`）。

---

## 11. 前端落地清单（按实现顺序）

### 11.1 P0 — 一天内必须跑通（没有它后面全是返工）

1. `tokens.css`：§2.14 全量变量（含 `[data-theme="light"]`）+ 3 行 reset。**先落 token，任何组件里出现裸 hex 视为 bug**（评审用 `grep -E '#[0-9A-Fa-f]{6}' *.css` 卡）。
2. `icons.svg` sprite（§7）+ `.icon` 工具类 + `<use>` 封装函数。
3. `.btn` 四变体五态（§5.1）+ `.badge` 九态（§6.2）+ `.num`（mono + tabular-nums）。
4. 布局骨架：`topbar`(56) + `content`(max 1360 / gutter 24-32) + 三档断点（1280 / 1440 / 1920）。

### 11.2 P1 — 首屏可用

5. `ConfigBar`（S1 控制卡）：Segmented + 等宽输入 + Select + 主 CTA；含错误态。
6. `ConfigSummary`（§3.7）：`done` 态一行折叠，含展开动画。
7. `ProbeCard`（§3.3）骨架：header + 主数字位 + 次指标 + footer；**先固定高度 172px**，再填内容。
8. `VerdictBadge` 九态接线（`data-verdict` 驱动，§6.2）+ `Grade`（分数·档位词，§6.7）+ `AxisA` / `AxisB`（§6.3 / §6.4）—— 这四组必须先定死，因为结论卡、卡片、摘要三处都要用。
8b. 灰度截图自测：九态 + 三档 + 三子变体在无彩情况下全部可辨（不通过则回改形状，不许回头加颜色）。

### 11.3 P2 — 数据可视化（最容易被低估的工作量）

9. `SpeedChart`：Canvas + DPR 缩放 + `niceCeil` + 30s 滑动窗口 + 游标；先做静态渲染，再接 rAF 批量更新。
10. `Sparkline`（复用 SpeedChart 的绘制函数，关轴关网格）。
11. `Waterfall`（DOM `<dl>` 版，§3.4）+ 主要耗时标注位。
12. `ScoreRing` / `ScoreBar`（SVG dasharray + 阈值色）+ `Grade` 双元素排版位。
13. 曲线文本替代（§4.7）—— 与 Canvas 同高容器，切换不跳动。
14. 四种进度形态（§6.9）：吞吐条 / 五段 stepper / n-N 点阵 / 全局离散条 —— **不要抽成一个 `<Progress>` 组件**，共用数据不同结构。

### 11.4 P3 — 详情与浮层

15. `Drawer`（覆盖 + 并列两形态，同一 DOM）+ 遮罩 + focus trap + `Esc`；内部按七段骨架排布（§3.4）。
16. `FailureAttribution`（五类 party，§6.8）+ `DegradedFlag` + `TrafficBudget`（§6.10）。
17. `Tabs`（transform 指示条）+ `Modal`（S5 设置）+ `Select` 浮层。
18. `Toast` + `Tooltip`（延迟 400ms）+ 层级 z-index：遮罩 50 / 抽屉 60 / Modal 65 / Tooltip 80 / Toast 90。
19. `ComparePanel`（双列 + Δ + **方向列 56px**，§6.10）。

### 11.5 P4 — 收尾

20. 浅色模式 8 条差异（§9）逐条对照；直连三处标识中的窄色带（§6.10）在浅色下复检。
21. `prefers-reduced-motion` / `prefers-contrast` / 200% 缩放 / 灰度四类验收（§10）。
22. 导出预览（S6）+ 复制/保存。
23. **S7 截图友好摘要**（§3.9）：按 1024×640 逻辑尺寸校验高度预算，`@media print` 与 `print-color-adjust: exact` 单独验。

### 11.6 性能红线（不达标即重做）

- 单帧预算 `16.6ms`；6 卡 + 主曲线合并到一个 `rAF`，禁止多次独立重绘。
- 流式更新只允许改 `textContent` / `canvas` / CSS 自定义属性；**禁止** `innerHTML`、禁止改 `class` 触发大面积重排（状态切换除外）。
- 空闲（idle/done）时 CPU 近 0：无数据时不跑 rAF 循环，`IntersectionObserver` 控制 Canvas 重绘。
- 卡片高度、图表容器高度**恒定**，任何数据变化不得引发布局抖动（CLS 目标 0）。

---

## 附录 A：与 `07-ux-interaction.md` 的对齐与两处明确裁定

| 议题 | ux-arch 要求 | 本文件处理 |
|---|---|---|
| 三态标记 | 可用=实心点 / 受限=半填充或缺环 / 不可用=叉或断环；「若半填充小尺寸不可辨，改文案首字+旁注+进度条中断」 | **裁定：不用半填充**，改用 `ring-gap`（90° 缺口环，12px 下限）。理由与几何见 §6.3。文案沿用 `可用/受限/不可用`（首字 可/受/不 互异，满足"首字差异"要求，无需改成 被/未） |
| 首屏一屏容纳 | 1440×900 一屏装下 V1–V4 + ConfigBar | 已核算：`topbar 56 + padding 32 + ConfigSummary 40 + 结论卡 168 + V2 三行 120 + 三分项 104 + 区块间距 3×32 = 648px`，余 252px 给卡片首行（172px），**成立** |
| 卡片主数字 | 每卡 1 个主数字，判定规则=决定徽章颜色者 | 采纳并落表（§3.8）；P3/P4/P5 主位改为结论短语，状态码仅进证据区 |
| 详情七段顺序 | 结论句→瀑布→曲线→指标表→证据→折叠 JSON→动作 | 采纳；S4 的 Tab（概览/阶段/速度/证据）仅**分组跳转**，滚动视图仍按七段顺序完整排列，不改变顺序 |
| 降级顺序 | 微曲线→次指标→环境细节→stepper 标签→Compare 列 | 采纳并补像素值（§2.13） |
| 断点 | 未给像素归属 | 本文件补齐：1280 / 1440 / 1920 三档 + 960×640 最小窗口提示层（§2.13） |

**一处需要 team-lead 知晓的分歧**：ux-arch 给的"1440×900 一屏"依赖 `ConfigSummary` 折叠到 40px；若实现保留完整控制卡（188px），首屏将溢出 ~148px。因此 §3.7 的 `ConfigSummary` **属于 P1 阻塞项**，不能延后到 P3。

---

## 附录 B：第二次对齐（PRD 定稿后新增的 10 条）

| # | 新增约束 | 本文件落点 |
|---|---|---|
| 1 | 两条独立三值轴（A 轴 AI 可用性 / B 轴 速度）不得合并，`SLOW` 只属 B 轴 | §6 轴表 + §6.3 + §6.4；`SLOW` 在 §6.2 明确标注"归属 B 轴"，A 轴表内不出现 |
| 2 | Verdict 由 10 态收敛为 PRD 9 态，`TLS_ERROR` 并入 `NET_FAIL`、`UNSTABLE` 废除 | §6.2 九态表；`tls_cert` 作为 `subReason` 进入 §6.5（文案独立、徽章不独立） |
| 3 | `NET_FAIL` 六个子原因各一条中文文案，`tls_cert` 严禁"忽略证书继续"、`truncated` 不得判 PASS | §6.5 表；`tls_cert` 动作位唯一 = `查看证书详情`；`truncated` 主位改 `内容异常` |
| 4 | 受限带三个子变体（人机验证 / 地区策略 / 限流）不得合并为"被墙"，判断"同母题 + 内符号"在 16–20px 是否可行 | §6.3.1：**16px 以下不可行**（问号内径不足 7px），采用双尺度 —— 小尺寸退回 4 个非颜色通道，≥20px 加内符号；母题始终为"环" |
| 5 | 5 档档位 A–E 必须"分数 + 档位词"，且与三态词面冲突需靠样式区分；另需"样本不足，暂不评级"独立形态 | §6.7：`.grade`（描边胶囊 + 分数）vs `.axis-a`（图形 + 文字），从形状/位置/字号/前缀四维区分；`data-g="none"` 不显示 0 分、不复用 E 档 |
| 6 | 失败归因五类（不是三类），party 必填无"未知"兜底，排版按最长词设计 | §6.8：`网络运营商` 为基准 → chip `min-width: 96px`；无法判定时渲染"系统环境"+`DegradedFlag` |
| 7 | 进度四形态不可统一（吞吐/可达/采样/全局） | §6.9 四形态表：条长 / 段数 / 点数 / 离散跳格，四者形态差异远大于颜色差异，且都不显示百分比文字 |
| 8 | 新增界面：截图友好摘要（1280×800、125% 缩放成立、强制浅色、灰度可辨） | §3.9 S7，按 **1024×640 逻辑尺寸**做高度预算（573 + 48 padding = 621 ≤ 640），不设第二套布局 |
| 9 | 直连模式三处常驻标识，其中结论区窄色带"持续可见但不像告警" | §6.10：Header 中性 pill / ConfigSummary 首词 / 结论卡顶部 `3px` **中性灰**色带（靠位置而非颜色被感知） |
| 10 | `DegradedFlag` / `TrafficBudget` / `ComparePanel`（含方向列）/ `SpeedVerdict` 四个新视觉位 | §6.10 表；`ComparePanel` 方向列宽 `56px` 且**按好坏着色而非数值大小** |

**仍待 ux-arch / PM 确认的一处**：`SKIPPED` 我按"虚线边框 + 无底色 + 无动效 + 不计分"实现（§6.6），它与 `idle` 的骨架靠"虚线 vs 实线 + 有无呼吸动效"区分。如果交互上要求 `SKIPPED` 与"未开始"在同一屏共存，需要额外确认两者的徽章文案是否都允许出现"未测/待测"字样——目前我用了 `本次未测` 与 `待测` 两个不同词以避免同字。

---

**UI Designer** ｜ v1.1 ｜ 配套实现文件建议：`app/web/css/tokens.css`、`components.css`、`app/web/js/chart.js`、`icons.svg`

