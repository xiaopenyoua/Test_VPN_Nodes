import { h, clear, fmt } from '../core/dom.js';
import { StatusBadge } from './StatusBadge.js';
import { drawSpeed } from './SpeedChart.js';

// 单个探测卡：自渲染 + 就地更新，避免整页重绘造成闪烁。
export function ProbeCard({ onOpen }) {
  const el = h('article', {
    class: 'card probe',
    tabindex: '0',
    role: 'button',
    'aria-label': '查看探测详情',
  });
  let lastKey = '';

  function activate() {
    if (el.dataset.id) onOpen(el.dataset.id);
  }
  el.addEventListener('click', activate);
  el.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); activate(); }
  });

  function update(r) {
    if (!r) return;
    const key = `${r.status}|${r.headline}|${r.subline}|${r.phases?.totalMs}|${r.samples?.length}`;
    el.dataset.id = r.id;
    el.classList.toggle('is-running', r.status === 'running');
    if (key === lastKey) return;
    lastKey = key;

    clear(el);
    el.appendChild(h('div', { class: 'probe-head' },
      h('span', { class: 'probe-id' }, r.id),
      h('span', { class: 'probe-name' }, r.name),
      StatusBadge(r.status, 'dot'),
    ));

    el.appendChild(h('div', { class: 'probe-main' },
      h('span', { class: 'probe-value' }, r.headline || '—'),
    ));
    // 首屏不出现裸状态码：状态码是证据，不是给用户看的语言，只在详情抽屉里呈现
    const sub = (r.subline || '').replace(/\s*·\s*HTTP\s*\d{3}/gi, '');
    if (sub) el.appendChild(h('div', { class: 'probe-sub' }, sub));

    if ((r.samples && r.samples.length > 1) || r.status === 'running') {
      const cv = h('canvas', { class: 'sparkline' });
      el.appendChild(cv);
      requestAnimationFrame(() => drawSpeed(cv, r.samples || [], { empty: '采样中…' }));
    }

    // 卡片正面只要一个主数字 + 一句次指标 + 一个详情入口。
    // 原来这里有 DNS/TCP/TLS/TTFB 四格阶段表（6 卡共 24 个数字），是首屏最大的噪声源，
    // 且 TTFB 等属于《07》§1.4 的禁用词——它们现在只在详情抽屉里出现。
    el.appendChild(h('div', { class: 'probe-foot' },
      h('span', {}, ''),
      h('span', { class: 'probe-more' }, '详情 ›'),
    ));
  }

  return { el, update };
}

function phaseCell(label, v) {
  return h('div', { class: 'phase-cell' }, label, h('b', {}, v ? `${Math.round(v)}ms` : '—'));
}
