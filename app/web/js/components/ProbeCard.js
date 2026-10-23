import { h, clear, NONE } from '../core/dom.js';
import { StatusBadge } from './StatusBadge.js';
import { drawSpeed } from './SpeedChart.js';
import { SustainTimeline, isLongRun } from './SustainTimeline.js';

// 单个探测卡：自渲染 + 就地更新，避免整页重绘造成闪烁。
// onProbeOnly(id) 由网格透传：用当前配置只重跑这一项，不打断其它卡的结果。
export function ProbeCard({ onOpen, onProbeOnly }) {
  const el = h('article', {
    class: 'probe',
    tabindex: '0',
    role: 'button',
    'aria-label': '查看探测详情',
  });
  let lastKey = '';

  // 按钮常驻实例：卡片重绘时把它重新挂到 foot 里，避免反复解绑点击事件，
  // 也避免"重绘后按钮闪一下、焦点丢失"。
  const onlyBtn = h('button', {
    class: 'btn btn-ghost btn-sm probe-only',
    type: 'button',
    title: '用当前配置只重跑这一项',
    // 卡片本身是 role=button：内层按钮必须掐断冒泡，否则点"只测这项"会顺带打开详情抽屉
    onclick: (e) => {
      e.stopPropagation();
      if (onlyBtn.disabled) return;
      if (el.dataset.id && onProbeOnly) onProbeOnly(el.dataset.id);
    },
    onkeydown: (e) => e.stopPropagation(),
  }, '只测这项');

  // 只测这一项的状态：running=全局有 run 在跑（后端拒绝并发），busyId=正在跑的就是我
  function syncOnlyBtn(r, opts) {
    const busy = opts.busyId && opts.busyId === r.id;
    const blocked = (opts.running || opts.busyId) && !busy;
    onlyBtn.disabled = blocked || busy;
    onlyBtn.textContent = busy ? '测试中…' : '只测这项';
    onlyBtn.setAttribute('aria-label', busy ? `${r.id} 正在单项复测` : `只测 ${r.id} 这一项`);
  }

  function activate() {
    if (el.dataset.id) onOpen(el.dataset.id);
  }
  el.addEventListener('click', activate);
  el.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); activate(); }
  });

  function update(r, opts = {}) {
    if (!r) return;
    syncOnlyBtn(r, opts);
    const longRun = isLongRun(r);
    const key = [
      r.status, r.headline, r.subline, r.phases?.totalMs,
      r.samples?.length, longRun ? 'L' : '',
      r.durationMs, r.stalled, r.sustainPct,
      r.region, r.colo, r.warp, r.only,
    ].join('|');
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
      h('span', { class: 'probe-value' }, r.headline || NONE),
    ));
    // 首屏不出现裸状态码：状态码是证据，不是给用户看的语言，只在详情抽屉里呈现
    const sub = (r.subline || '').replace(/\s*·\s*HTTP\s*\d{3}/gi, '');
    if (sub) el.appendChild(h('div', { class: 'probe-sub' }, sub));

    // 出口身份是证据，不是结论：放在副标题下方的小字区，不动主标题文案
    const identity = exitIdentity(r);
    if (identity) el.appendChild(identity);

    // 单项复测的结果必须自报身份：它不参与总评，不能让人误以为总评被刷新了
    if (r.only) {
      el.appendChild(h('div', { class: 'probe-only-tag' }, '单项复测 · 不参与总评'));
    }

    // 长跑用时间线（40px、带断流标记）；短测仍用 34px sparkline，避免两套图同时出现
    if (longRun) {
      const timeline = SustainTimeline(r);
      if (timeline) el.appendChild(timeline);
    } else if ((r.samples && r.samples.length > 1) || r.status === 'running') {
      const cv = h('canvas', { class: 'sparkline' });
      el.appendChild(cv);
      requestAnimationFrame(() => drawSpeed(cv, r.samples || [], { empty: '采样中…' }));
    }

    // 卡片正面只要一个主数字 + 一句次指标 + 一个详情入口。
    // 阶段耗时（DNS/TCP/TLS/TTFB）只在详情抽屉里出现。
    el.appendChild(h('div', { class: 'probe-foot' },
      onlyBtn,
      h('span', { class: 'probe-more' }, '详情 ›'),
    ));
    syncOnlyBtn(r, opts);
  }

  return { el, update };
}

// P3 的出口身份：loc/colo 来自 Cloudflare /cdn-cgi/trace，是目标站自己报的。
// warp 为真时 OpenAI 会直接拒绝这类共享出口，所以用警告色徽章而不是普通小字。
function exitIdentity(r) {
  const region = (r.region || '').trim();
  if (!region && !r.warp) return null;
  const parts = [];
  if (region) parts.push(`出口 loc=${region}`);
  if (r.colo) parts.push(`colo=${r.colo}`);

  return h('div', { class: 'probe-ident' },
    parts.length ? h('span', {}, parts.join(' · ')) : null,
    r.warp ? h('span', {
      class: 'badge probe-warp',
      dataset: { status: 'degraded' },
      title: 'Cloudflare WARP 共享 IP，OpenAI 会直接拒绝这类出口',
    }, 'WARP 出口') : null,
  );
}
