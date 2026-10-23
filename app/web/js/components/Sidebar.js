import { h } from '../core/dom.js';
import { store } from '../core/store.js';
import { applyTheme, saveTheme } from '../core/theme.js';

// 三态主题：自动跟随系统，亮色 / 暗色是显式选择。
const THEMES = [
  { key: 'auto', label: '自动' },
  { key: 'light', label: '亮' },
  { key: 'dark', label: '暗' },
];

// 四个独立目的地，不是同一页的分段。
// 「带宽」单独占一项：它测的是另一件事（连续跑两小时会不会掉），
// 参数也不一样（小时 / GB），塞进分评里会让"六项"这个说法失效。
const NAV = [
  { key: 'verdict', label: '总评' },
  { key: 'probes', label: '分评' },
  { key: 'bandwidth', label: '带宽' },
  { key: 'settings', label: '设置' },
];

// 标识：一枚镜头 —— 外环 + 斜向通光口。放在粘土球里，是整页唯一的圆形强调。
const MARK = '<svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">' +
  '<circle cx="11" cy="11" r="6.4" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"/>' +
  '<path d="M15.8 15.8 20 20" stroke="currentColor" stroke-width="2.8" stroke-linecap="round"/>' +
  '</svg>';

export function Sidebar({ value, onChange }) {
  const nav = h('nav', { class: 'nav', 'aria-label': '主导航' });
  const navBtns = new Map();
  for (const it of NAV) {
    // 长跑可能挂着两小时，用户会切到别的页去看结果。
    // 靠一枚呼吸点提示"它还在跑"，比顶栏常驻一条横幅安静得多。
    const dot = h('i', { class: 'nav-dot', hidden: true, 'aria-hidden': 'true' });
    const b = h('button', {
      type: 'button',
      onclick: () => onChange(it.key),
    }, it.label, dot);
    b._dot = dot;
    navBtns.set(it.key, b);
    nav.appendChild(b);
  }

  const themeSeg = h('div', { class: 'seg seg-sm', role: 'group', 'aria-label': '外观' });
  const themeBtns = new Map();
  for (const t of THEMES) {
    const b = h('button', {
      type: 'button',
      onclick: () => {
        applyTheme(t.key);
        saveTheme(t.key);
        store.patch({ theme: t.key });
      },
    }, t.label);
    themeBtns.set(t.key, b);
    themeSeg.appendChild(b);
  }

  const el = h('aside', { class: 'dock' },
    h('div', { class: 'brand' },
      h('span', { class: 'blob', 'aria-hidden': 'true', html: MARK }),
      h('span', { class: 'brand-text' },
        h('b', {}, 'ProxyLens'),
        h('small', {}, '代理能力探测'),
      ),
    ),
    nav,
    h('div', { class: 'dock-foot' },
      h('span', { class: 'dock-label' }, '外观'),
      themeSeg,
    ),
  );

  function render() {
    const tab = store.state.tab;
    for (const [k, b] of navBtns) {
      const on = k === tab;
      b.classList.toggle('is-current', on);
      if (on) b.setAttribute('aria-current', 'page');
      else b.removeAttribute('aria-current');
    }
    const bwRunning = !!(store.state.bandwidth && store.state.bandwidth.running);
    const bwBtn = navBtns.get('bandwidth');
    if (bwBtn && bwBtn._dot) bwBtn._dot.hidden = !bwRunning;
    if (bwBtn) bwBtn.title = bwRunning ? '带宽长跑进行中' : '长时间大带宽下载';
    for (const [k, b] of themeBtns) b.setAttribute('aria-pressed', String(store.state.theme === k));
  }

  render();
  return { el, render };
}
