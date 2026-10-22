import { h } from '../core/dom.js';

// 开关：用 role="switch" 而不是 checkbox，屏幕阅读器会读成「开/关」而不是「勾选」。
// props 进（checked / disabled / label）、onChange 回调出，自身不读写 store。
export function Toggle({ checked = false, disabled = false, label = '', onChange }) {
  const el = h('button', {
    type: 'button',
    class: 'switch',
    role: 'switch',
    'aria-checked': String(!!checked),
    'aria-label': label,
    onclick: () => {
      if (el.disabled) return;
      onChange?.(el.getAttribute('aria-checked') !== 'true');
    },
  }, h('span', { class: 'switch-knob' }));

  el.setAttribute('aria-checked', String(!!checked));
  el.disabled = !!disabled;
  return el;
}

// 供外部在受控场景下同步状态（store.patch 后再渲染时不重建节点，避免焦点丢失）
export function setToggle(el, checked, disabled = false) {
  el.setAttribute('aria-checked', String(!!checked));
  el.disabled = !!disabled;
}
