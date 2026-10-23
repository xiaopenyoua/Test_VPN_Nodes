import { h } from '../core/dom.js';

let wrap = null;

function ensure() {
  if (!wrap) {
    wrap = h('div', { class: 'toast-wrap' });
    document.body.appendChild(wrap);
  }
  return wrap;
}

export function toast(msg, kind = 'info', ms = 2600) {
  const el = h('div', { class: 'toast', dataset: { kind } }, msg);
  ensure().appendChild(el);
  setTimeout(() => {
    el.style.opacity = '0';
    el.style.transform = 'translateY(6px)';
    el.style.transition = 'opacity 200ms, transform 200ms';
    setTimeout(() => el.remove(), 220);
  }, ms);
}
