import { h } from '../core/dom.js';

// 侧栏导航：总评 / 分评 / 设置 是三个独立目的地，不是同一页的分段。
// 当前项只靠标签从灰变白标记，不加色块、不加色条。
export function SideNav({ items, value, onChange }) {
  const el = h('nav', { class: 'nav', 'aria-label': '主导航' });
  const btns = new Map();

  for (const it of items) {
    const b = h('button', {
      class: 'nav-item',
      type: 'button',
      'aria-current': it.key === value ? 'page' : null,
      onclick: () => onChange(it.key),
    }, it.label);
    btns.set(it.key, b);
    el.appendChild(b);
  }

  function render(v) {
    for (const [k, b] of btns) {
      if (k === v) b.setAttribute('aria-current', 'page');
      else b.removeAttribute('aria-current');
    }
  }

  return { el, render };
}
