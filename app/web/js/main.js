import { h, clear } from './core/dom.js';
import { store, resetForRun } from './core/store.js';
import { api } from './core/api.js';
import { TopBar } from './components/TopBar.js';
import { ControlPanel } from './components/ControlPanel.js';
import { VerdictPanelFactory } from './components/VerdictPanel.js';
import { ProbeGrid } from './components/ProbeGrid.js';
import { DetailDrawer } from './components/DetailDrawer.js';
import { SettingsSheet } from './components/SettingsSheet.js';
import { toast } from './components/Toast.js';

const root = document.getElementById('app');
document.documentElement.dataset.theme = store.state.theme;

function exportReport(format) {
  if (!store.state.summary) {
    toast('还没有可导出的结果', 'error');
    return;
  }
  window.open(api.reportUrl(format), '_blank');
}

const topbar = TopBar({ onSettings: () => openSettings(), onExport: exportReport });
// 注意：TopBar 返回 { el, render }，下方统一用 topbar.el 挂载
const control = ControlPanel({ onRun: startRun, onCancel: cancelRun });
const verdict = VerdictPanelFactory({ onExport: exportReport });
const grid = ProbeGrid({ onOpen: (id) => openDetail(id) });

const metaLine = h('div', { class: 'section-title' }, '探测项');
const runClock = h('div', { class: 'section-title', style: 'font-family:var(--font-mono)' }, '');

const footer = h('footer', { class: 'footer' },
  h('span', {}, 'ProxyLens · 本地运行 · 数据不外传'),
  h('span', {}, '探测目标固定为官方站点；结果仅代表本次链路状态'),
);

// 先清掉 index.html 里的启动占位（.boot），否则它会永久留在页面顶部
clear(root);
root.appendChild(topbar.el);
root.appendChild(control.el);
root.appendChild(verdict.el);
root.appendChild(h('div', { class: 'section-head' }, metaLine, runClock));
root.appendChild(grid.el);
root.appendChild(footer);

let drawer = null;
let settings = null;

function openDetail(id) {
  const r = store.state.results.find((x) => x.id === id);
  if (!r) return;
  closeSettings();
  if (drawer) drawer.el.remove();
  drawer = DetailDrawer({ onClose: closeDetail });
  drawer.render(r);
  document.body.appendChild(drawer.el);
  store.patch({ detailId: id });
}

function closeDetail() {
  if (drawer) { drawer.el.remove(); drawer = null; }
  store.patch({ detailId: null });
}

function openSettings() {
  closeDetail();
  if (settings) settings.el.remove();
  settings = SettingsSheet({ onClose: closeSettings });
  document.body.appendChild(settings.el);
}

function closeSettings() {
  if (settings) { settings.el.remove(); settings = null; }
}

async function startRun(cfg) {
  try {
    resetForRun(cfg, store.state.exit);
    const res = await api.run(cfg);
    store.patch({ runId: res.runId });
  } catch (err) {
    store.patch({ phase: 'error', error: err.message });
    toast(err.message, 'error');
  }
}

async function cancelRun() {
  try {
    await api.cancel();
    toast('已请求取消');
  } catch (err) {
    toast(err.message, 'error');
  }
}

// —— 状态驱动渲染 ——
let lastPhase = '';
let verdictSig = '';

// 结论区只在语义变化时重绘：每次采样都重建会重置圆环过渡、造成闪烁
function renderVerdict(state) {
  const s = state.summary;
  const sig = s
    ? [s.total, s.state, s.grade, (s.caps || []).length, (s.advice || []).length].join('|')
    : 'empty|' + state.phase;
  if (sig === verdictSig) return;
  verdictSig = sig;
  verdict.render(state);
}

store.subscribe((state) => {
  if (state.phase !== lastPhase) {
    lastPhase = state.phase;
    control.render();
    topbar.render();
  }
  renderVerdict(state);
  grid.render(state);
  if (drawer && state.detailId) {
    const r = state.results.find((x) => x.id === state.detailId);
    if (r) drawer.render(r);
  }
});

// —— 计时器 ——
setInterval(() => {
  const st = store.state;
  if (st.phase === 'running') {
    const secs = (Date.now() - st.startedAt) / 1000;
    runClock.textContent = `${secs.toFixed(1)}s`;
  } else if (st.elapsedMs) {
    runClock.textContent = `${(st.elapsedMs / 1000).toFixed(1)}s`;
  }
}, 100);

// —— 事件流 ——
let es = null;
function bindStream() {
  es = api.subscribe({
    snapshot: (st) => {
      store.patch({
        phase: st.running ? 'running' : (st.summary ? 'done' : 'idle'),
        config: { ...store.state.config, ...(st.config || {}) },
        exit: st.exit || null,
        env: st.env || null,
        results: st.results || [],
        summary: st.summary || null,
      });
    },
    run_start: (p) => {
      resetForRun(p.config, p.exit);
      store.patch({ env: p.env });
    },
    probe_start: (p) => {
      store.upsertResult({ id: p.id, name: p.name, group: p.group, status: 'running', headline: '探测中…', subline: '', phases: {}, samples: [] });
    },
    sample: (p) => {
      store.updateResult(p.id, (r) => {
        const samples = [...(r.samples || []), p.sample].slice(-240);
        return { ...r, samples, headline: `${p.sample.mbps.toFixed(1)} Mbps` };
      });
    },
    probe_result: (r) => store.upsertResult(r),
    score: (s) => store.patch({ summary: s }),
    run_end: (p) => store.patch({
      phase: p.status === 'cancelled' ? 'cancelled' : 'done',
      elapsedMs: p.elapsedMs,
    }),
    error: (p) => {
      store.patch({ error: p.message || '运行异常' });
      toast(p.message || '运行异常', 'error');
    },
    stream_error: () => {
      if (store.state.streamOk) {
        store.patch({ streamOk: false });
        toast('事件流断开，正在重连…', 'error');
      }
    },
  });
  es.onopen = () => store.patch({ streamOk: true });
}

// —— 快捷键 ——
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') { closeDetail(); closeSettings(); return; }
  if (e.key === 'Enter' && !['INPUT', 'TEXTAREA'].includes(document.activeElement?.tagName)) {
    if (store.state.phase !== 'running') startRun(store.state.config);
  }
});

// —— 启动 ——
(async () => {
  try {
    const st = await api.state();
    store.patch({
      phase: st.running ? 'running' : (st.summary ? 'done' : 'idle'),
      config: { ...store.state.config, ...(st.config || {}) },
      exit: st.exit || null,
      env: st.env || null,
      results: st.results || [],
      summary: st.summary || null,
    });
  } catch (err) {
    toast('初始化失败：' + err.message, 'error');
  }
  bindStream();
  const st = store.state;
  renderVerdict(st);
  grid.render(st);
  control.render();
  topbar.render();
})();
