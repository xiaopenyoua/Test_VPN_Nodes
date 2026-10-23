import { h, clear } from '../core/dom.js';
import { store } from '../core/store.js';

// 分评页的「单项设置」：只影响「只测这项」，与总评的整体设置完全分开。
// 总评设置决定一次完整探测（P1–P6）；这里决定单跑某一项时用多少时长/流量/并发。
const PROBES = [
  { id: 'P1', name: 'Google 大文件下载' },
  { id: 'P2', name: 'YouTube 链路' },
  { id: 'P3', name: 'ChatGPT 网页' },
  { id: 'P4', name: 'Gemini 网页' },
  { id: 'P5', name: 'OpenAI API' },
  { id: 'P6', name: '国际链路基线' },
];

const FIELDS = {
  P1: [
    { key: 'durationSec', label: '下载时长', unit: '秒', min: 3, max: 120 },
    { key: 'budgetMb', label: '流量预算', unit: 'MB', min: 4, max: 4096 },
    { key: 'concurrency', label: '并发连接', unit: '条', min: 1, max: 16 },
  ],
  default: [
    { key: 'timeoutSec', label: '单请求超时', unit: '秒', min: 5, max: 60 },
  ],
};

export function ProbeSettingsPanel({ onProbeOnly, onGoOverall }) {
  const el = h('section', { class: 'panel probe-settings' });
  let lastTarget = '';
  let runBtn = null;
  let exitText = null;
  let segBtns = new Map();

  function build(state) {
    clear(el);
    const target = state.probeTarget;
    const cfg = state.probeConfig[target] || {};
    const fields = FIELDS[target] || FIELDS.default;

    const seg = h('div', { class: 'segmented probe-seg', role: 'group', 'aria-label': '选择探测项' });
    segBtns = new Map();
    for (const p of PROBES) {
      const b = h('button', {
        type: 'button',
        'aria-pressed': String(p.id === target),
        title: p.name,
        onclick: () => store.patch({ probeTarget: p.id }),
      }, p.id);
      segBtns.set(p.id, b);
      seg.appendChild(b);
    }

    const fieldEls = fields.map((f) => h('label', { class: 'ps-field' },
      h('span', { class: 'ps-label' }, f.label),
      h('span', { class: 'ps-input-wrap' },
        h('input', {
          class: 'input num-input',
          type: 'number',
          min: f.min,
          max: f.max,
          value: cfg[f.key],
          oninput: (e) => {
            let v = parseInt(e.target.value, 10);
            if (Number.isNaN(v)) return;
            v = Math.max(f.min, Math.min(f.max, v));
            const next = {
              ...store.state.probeConfig,
              [target]: { ...store.state.probeConfig[target], [f.key]: v },
            };
            store.patch({ probeConfig: next });
          },
        }),
        h('span', { class: 'ps-unit' }, f.unit),
      ),
    ));

    exitText = h('span', { class: 'ps-exit' });
    runBtn = h('button', {
      class: 'btn btn-primary',
      type: 'button',
      onclick: () => onProbeOnly(target),
    }, `只测 ${target}`);

    el.appendChild(h('div', { class: 'ps-head' },
      h('div', {},
        h('div', { class: 'ps-title' }, '单项复测设置'),
        h('div', { class: 'ps-desc' }, '只影响「只测这项」，不改动总评；总评始终来自一次完整探测。'),
      ),
      h('button', { class: 'btn btn-ghost btn-sm', type: 'button', onclick: onGoOverall }, '总评设置 ›'),
    ));

    el.appendChild(h('div', { class: 'ps-row' },
      h('div', { class: 'ps-field' }, h('span', { class: 'ps-label' }, '探测项'), seg),
      ...fieldEls,
      h('div', { class: 'ps-actions' }, runBtn),
    ));

    el.appendChild(h('div', { class: 'ps-foot' }, exitText));
    sync(state);
  }

  function sync(state) {
    const running = state.phase === 'running';
    if (runBtn) {
      runBtn.disabled = running;
      runBtn.textContent = running ? '测试中…' : `只测 ${state.probeTarget}`;
    }
    for (const [id, b] of segBtns) {
      b.setAttribute('aria-pressed', String(id === state.probeTarget));
      b.disabled = running;
    }
    if (exitText) {
      const mode = state.config.mode === 'custom'
        ? '自定义代理'
        : state.config.mode === 'direct' ? '直连对照' : '系统代理';
      const detail = state.exit && state.exit.detail ? ' · ' + state.exit.detail : '';
      exitText.textContent = `出网方式沿用总评设置：${mode}${detail}`;
    }
  }

  function render(state) {
    if (state.probeTarget !== lastTarget) {
      lastTarget = state.probeTarget;
      build(state);
      return;
    }
    sync(state);
  }

  return { el, render };
}
