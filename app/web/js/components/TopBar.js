import { h, svg } from '../core/dom.js';
import { store } from '../core/store.js';
import { applyTheme, saveTheme } from '../core/theme.js';

// 三态循环：自动 → 亮色 → 暗色。自动跟随系统，显式选择优先。
const THEME_LABEL = { auto: '◐ 自动', light: '☀ 亮色', dark: '☾ 暗色' };
const THEME_NEXT = { auto: 'light', light: 'dark', dark: 'auto' };

export function TopBar({ onSettings, onExport }) {
  const themeBtn = h('button', {
    class: 'btn btn-ghost btn-sm',
    title: '切换主题：自动 / 亮色 / 暗色',
    onclick: () => {
      const next = THEME_NEXT[store.state.theme] || 'auto';
      applyTheme(next);
      saveTheme(next);
      store.patch({ theme: next });
    },
  }, THEME_LABEL[store.state.theme] || THEME_LABEL.auto);

  const exportBtn = h('button', {
    class: 'btn btn-sm',
    onclick: () => onExport('md'),
  }, '导出报告');

  const el = h('header', { class: 'topbar' },
    h('div', { class: 'brand' },
      h('div', { class: 'brand-mark' },
        svg('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: '#fff', 'stroke-width': 2, 'stroke-linecap': 'round' },
          svg('path', { d: 'M3 12h4l3-7 4 14 3-7h4' }),
        ),
      ),
      h('div', {},
        h('div', { class: 'brand-name' }, 'ProxyLens'),
        h('div', { class: 'brand-sub' }, 'system proxy capability probe'),
      ),
    ),
    h('div', { class: 'topbar-right' },
      themeBtn,
      h('button', { class: 'btn btn-ghost btn-sm', onclick: onSettings }, '⚙ 设置'),
      exportBtn,
    ),
  );

  // 没有结果时导出按钮不可点，避免"点了才知道没东西"的空点击
  function render() {
    const has = !!store.state.summary;
    exportBtn.disabled = !has;
    exportBtn.title = has ? '导出本次探测报告（Markdown）' : '先完成一次探测才能导出';
    themeBtn.textContent = THEME_LABEL[store.state.theme] || THEME_LABEL.auto;
  }

  render();
  return { el, render };
}
