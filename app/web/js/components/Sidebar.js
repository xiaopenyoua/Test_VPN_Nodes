import { h } from '../core/dom.js';
import { store } from '../core/store.js';
import { applyTheme, saveTheme } from '../core/theme.js';
import { SideNav } from './SideNav.js';

// 三态主题：自动跟随系统，亮色 / 暗色是显式选择。首次打开默认暗色。
const THEMES = [
  { key: 'auto', label: '自动' },
  { key: 'light', label: '亮色' },
  { key: 'dark', label: '暗色' },
];

const NAV = [
  { key: 'verdict', label: '总评' },
  { key: 'probes', label: '分评' },
  { key: 'settings', label: '设置' },
];

export function Sidebar({ value, onChange }) {
  const nav = SideNav({ items: NAV, value, onChange });

  const themeWrap = h('div', { class: 'theme-switch', role: 'group', 'aria-label': '主题' });
  const themeBtns = new Map();
  for (const t of THEMES) {
    const b = h('button', {
      type: 'button',
      'aria-pressed': String(store.state.theme === t.key),
      onclick: () => {
        applyTheme(t.key);
        saveTheme(t.key);
        store.patch({ theme: t.key });
        render();
      },
    }, t.label);
    themeBtns.set(t.key, b);
    themeWrap.appendChild(b);
  }

  const el = h('aside', { class: 'sidebar' },
    h('div', { class: 'brand' },
      h('div', { class: 'brand-name' }, 'ProxyLens'),
      h('div', { class: 'brand-sub' }, '代理能力探测'),
    ),
    nav.el,
    h('div', { class: 'sidebar-foot' },
      h('div', { class: 'sidebar-label' }, '主题'),
      themeWrap,
    ),
  );

  function render() {
    nav.render(store.state.tab);
    for (const [k, b] of themeBtns) b.setAttribute('aria-pressed', String(store.state.theme === k));
  }

  return { el, render };
}
