import { h } from '../core/dom.js';

// 一级导航：总评与分评是两个独立功能页，不是同一页的两块内容。
// 总评回答「这套代理整体怎么样」，分评回答「每一项现在怎么样」。
export function Tabs({ items, value, onChange }) {
  const el = h('nav', { class: 'tabs', role: 'tablist', 'aria-label': '结果视图' });
  const btns = new Map();
  let current = value;

  for (const it of items) {
    const b = h('button', {
      class: 'tab',
      type: 'button',
      role: 'tab',
      'aria-selected': String(it.key === current),
      tabindex: it.key === current ? '0' : '-1',
      onclick: () => onChange(it.key),
      onkeydown: (e) => {
        if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
        e.preventDefault();
        const keys = items.map((x) => x.key);
        const i = keys.indexOf(current);
        const step = e.key === 'ArrowRight' ? 1 : keys.length - 1;
        const next = keys[(i + step) % keys.length];
        onChange(next);
        btns.get(next).focus();
      },
    }, it.label);
    btns.set(it.key, b);
    el.appendChild(b);
  }

  function render(v) {
    current = v;
    for (const [k, b] of btns) {
      b.setAttribute('aria-selected', String(k === v));
      b.tabIndex = k === v ? 0 : -1;
    }
  }

  return { el, render };
}
