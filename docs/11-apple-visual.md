# 11 · Apple 视觉规范与桌面主题

| 项 | 值 |
|---|---|
| 文档版本 | v1.0 |
| 状态 | 已实现，可直接对照代码验收 |
| 适用范围 | `app/web/`（页面主题与动效）、`app/desktop/`（Electron 窗口） |
| 上游依赖 | `08-ui-spec.md`（颜色/字号/间距基线）、`07-ux-interaction.md`（状态机） |
| 实现位置 | `web/css/tokens.css`、`web/css/ui.css`、`web/js/core/theme.js`、`web/index.html`、`desktop/main.js` |

> 本文只定义「桌面外壳 + 主题 + 动效」这一层。颜色对比度、字号阶梯、间距系统仍以 `08-ui-spec.md` 为准，本文不重复。

---

## 1. 设计目标与边界

1. **像 macOS 原生应用，而不是套了壳的网页**：窗口用 `hiddenInset` 红绿灯、系统 vibrancy 材质、可拖拽标题栏。
2. **主题跟随系统，但用户说了算**：默认 `auto`；用户显式选择后不再被系统改回去。
3. **动效只服务状态变化**：数字不跳动，只有曲线、进度、脉冲承担动画。
4. **不做平台特判的视觉分叉**：Windows 用默认标题栏，页面视觉与 macOS 完全一致。

非目标：不做窗口内自绘标题栏、不做自定义红绿灯、不做毛玻璃卡片（卡片保持不透明以保证数字可读）。

---

## 2. 自动主题

### 2.1 三态模型

| 偏好值 | 行为 | 存储 |
|---|---|---|
| `auto`（默认） | 跟随 `prefers-color-scheme` | `localStorage["proxylens.theme"]` |
| `light` | 强制亮色 | 同上 |
| `dark` | 强制暗色 | 同上 |

顶栏按钮按 `自动 → 亮色 → 暗色 → 自动` 循环，按钮文案同步显示当前态（`◐ 自动` / `☀ 亮色` / `☾ 暗色`）。

### 2.2 首帧不闪

`index.html` 在 `<head>` 里用一段同步脚本先定主题，再加载样式：

```js
var pref = localStorage.getItem('proxylens.theme') || 'auto';
var dark = pref === 'dark' ||
  (pref === 'auto' && window.matchMedia('(prefers-color-scheme: dark)').matches);
document.documentElement.dataset.theme = dark ? 'dark' : 'light';
document.documentElement.dataset.themePref = pref;
```

- `data-theme` 只取 `light` / `dark`，CSS 不需要认识 `auto`。
- `data-theme-pref` 保留用户原始偏好，供调试与后续扩展。
- 若把这段逻辑放到 `main.js`（模块加载后执行），首帧会先按 `index.html` 的 `data-theme="dark"` 绘制，再切成亮色，出现一次白闪/黑闪。

### 2.3 系统主题变化

`web/js/core/theme.js` 监听 `matchMedia('(prefers-color-scheme: dark)')` 的 `change`：

- 当前偏好为 `auto` → 立即重新应用；
- 当前偏好为 `light` / `dark` → 不响应。

显式选择优先，否则用户点了「亮色」又被系统改回去，会以为按钮坏了。

### 2.4 令牌映射

主题切换只改 `data-theme`，所有颜色走 `tokens.css` 的 CSS 变量。新增颜色必须同时提供 dark / light 两套值，禁止在组件里写死十六进制。

---

## 3. 桌面窗口（Electron）

### 3.1 窗口参数

| 参数 | macOS | Windows |
|---|---|---|
| `titleBarStyle` | `hiddenInset` | `default` |
| `trafficLightPosition` | `{ x: 16, y: 14 }` | — |
| `vibrancy` | `under-window` | — |
| `visualEffectState` | `active` | — |
| `backgroundColor` | `#00000000` | `#0b0c0e` |
| 默认尺寸 | 1280 × 860，最小 960 × 640 | 同左 |

### 3.2 标题栏安全区

`hiddenInset` 下红绿灯直接压在网页内容上，必须让出安全区：

```css
:root { --titlebar-h: 38px; }

[data-shell="electron"] .app-shell { padding-top: var(--titlebar-h); }
[data-shell="electron"] .topbar { padding-left: 44px; -webkit-app-region: drag; }
[data-shell="electron"] .topbar button,
[data-shell="electron"] .topbar a { -webkit-app-region: no-drag; }
```

- `padding-top: 38px`：给标题栏留出高度，内容不从窗口最顶端开始。
- `padding-left: 44px`：叠加 `.app-shell` 自身的 32px 左内边距，红绿灯（约 70px 宽）不会压住品牌标识。
- 顶栏整体可拖拽；按钮必须 `no-drag`，否则点不动。

### 3.3 外壳握手

Electron 加载 `ready.url + '&shell=electron'`。页面在首帧脚本里读取该参数并写入 `data-shell="electron"`，CSS 据此启用安全区与拖拽区。浏览器直接访问时没有该参数，布局保持原样。

### 3.4 vibrancy 与可读性

```css
html[data-shell="electron"] { background: transparent; }
html[data-shell="electron"] body { background: color-mix(in srgb, var(--bg) 78%, transparent); }
```

- 页面底色半透明，系统材质才能透出来；
- 卡片、输入框、抽屉仍使用不透明 `--surface` / `--bg`，保证数字与正文对比度不下降；
- 不使用 `backdrop-filter` 叠加在卡片上：多层模糊会让等宽数字发虚。

### 3.5 窗口生命周期

- 主进程 `spawn` sidecar（`-no-open -emit-ready`），按行解析 `PROXYLENS_READY {json}`，拿到真实端口与 token；
- 20 秒未就绪 → 弹错误框并退出；
- `before-quit` 发 `SIGTERM`，3 秒未退再 `SIGKILL`，避免残留进程占端口；
- 外链一律 `shell.openExternal`，应用内不开新窗口；
- `will-navigate` 只允许停留在 sidecar 同源地址。

---

## 4. 动效

### 4.1 曲线

| Token | 值 | 用途 |
|---|---|---|
| `--ease` | `cubic-bezier(0.22, 0.61, 0.36, 1)` | 通用过渡（颜色、边框、背景） |
| `--ease-out` | `cubic-bezier(0.16, 1, 0.3, 1)` | 入场、抽屉滑入、Toast 出现 |
| `--ease-spring` | `cubic-bezier(0.34, 1.56, 0.64, 1)` | 需要轻微过冲的强调动作（如按钮回弹） |

### 4.2 时长

| Token | 值 | 用途 |
|---|---|---|
| `--dur-1` | 120ms | hover、按下、焦点环 |
| `--dur-2` | 200ms | 卡片状态切换、Toast |
| `--dur-3` | 320ms | 抽屉滑入、进度条填充 |

### 4.3 使用规则

1. **数字不跳动**：实时吞吐、计时器、分数更新不做过渡动画，避免每秒多次重排造成视觉抖动。
2. **只有三类元素承担动效**：曲线（sparkline / 时间线）、进度（评分环、进度条）、脉冲（运行中卡片扫光）。
3. **入场用 `--ease-out`，出场可以更快**：关闭抽屉不需要和打开一样长。
4. **禁止无限循环动画用于非运行态**：只有 `phase === 'running'` 才允许旋转/扫光。

### 4.4 降低动效

```css
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }
}
```

系统开启「减弱动态效果」后，所有动画近似瞬时完成，功能不受影响。

---

## 5. 无障碍

| 项 | 要求 | 实现 |
|---|---|---|
| 键盘导航 | 所有交互可 Tab 到达 | 原生 `button` / `input`，卡片 `tabindex="0"` |
| Tab 切换 | 左右方向键切换 | `Tabs.js` 处理 `ArrowLeft` / `ArrowRight` |
| 焦点可见 | 2px 品牌色焦点环 | `.btn:focus-visible`、`.tab:focus-visible` |
| 语义 | 状态 = 颜色 + 文字 | `StatusBadge` 始终带文字 |
| 对比度 | 正文 ≥ 4.5:1 | 沿用 `08-ui-spec.md` 实测值 |
| 动效偏好 | 尊重 `prefers-reduced-motion` | 见 §4.4 |

---

## 6. 验收清单

- [ ] 系统切到深色/浅色，`auto` 模式下页面跟随，无闪烁
- [ ] 手动切到「亮色」后，系统再切深色，页面保持亮色
- [ ] 刷新页面，主题与偏好保持
- [ ] Electron 窗口红绿灯不遮挡品牌标识，顶栏可拖动窗口
- [ ] 顶栏按钮可点击（未被拖拽区吞掉）
- [ ] 窗口背景能透出系统材质，卡片内文字清晰
- [ ] 关闭窗口后 sidecar 进程退出（`ps aux | grep proxylens` 无残留）
- [ ] 开启「减弱动态效果」后无持续动画
