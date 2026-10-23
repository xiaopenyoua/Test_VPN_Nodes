import { h, clear } from './core/dom.js';
import { store, resetForRun } from './core/store.js';
import { api } from './core/api.js';
import { applyTheme, watchSystemTheme } from './core/theme.js';
import { TopBar } from './components/TopBar.js';
import { ControlPanel } from './components/ControlPanel.js';
import { VerdictPanelFactory } from './components/VerdictPanel.js';
import { ProbeGrid } from './components/ProbeGrid.js';
import { DetailDrawer } from './components/DetailDrawer.js';
import { SettingsSheet } from './components/SettingsSheet.js';
import { ProbeSettingsPanel } from './components/ProbeSettingsPanel.js';
import { Tabs } from './components/Tabs.js';
import { toast } from './components/Toast.js';

const root = document.getElementById('app');
applyTheme(store.state.theme);
watchSystemTheme(() => {
  if (store.state.theme === 'auto') applyTheme('auto');
});

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
const grid = ProbeGrid({ onOpen: (id) => openDetail(id), onProbeOnly: (id) => probeOnly(id) });
const probeSettings = ProbeSettingsPanel({
  onProbeOnly: (id) => probeOnly(id),
  onGoOverall: () => store.patch({ tab: 'verdict' }),
});

const metaLine = h('div', { class: 'section-title' }, '探测项');
const runClock = h('div', { class: 'section-title', style: 'font-family:var(--font-mono)' }, '');

// 总评与分评是两个独立功能页：总评回答「整体怎么样」，分评回答「每一项现在怎么样」。
const tabs = Tabs({
  items: [{ key: 'verdict', label: '总评' }, { key: 'probes', label: '分评' }],
  value: store.state.tab,
  onChange: (k) => store.patch({ tab: k }),
});

// 总评设置：影响一次完整探测（P1–P6），与分评的单项设置完全分开
const overallHead = h('div', { class: 'panel-head' },
  h('div', {},
    h('div', { class: 'panel-title' }, '总评设置'),
    h('div', { class: 'panel-desc' }, '影响完整探测（P1–P6），决定总评分数。'),
  ),
);

// 单项复测不重算总评，但必须在总评页说清楚，否则用户会以为总评被刷新了
const summaryNote = h('div', { class: 'summary-note', hidden: true });
const runMeta = h('div', { class: 'run-meta', hidden: true });
const verdictPanel = h('div', { class: 'tab-panel', role: 'tabpanel', 'aria-label': '总评' },
  overallHead,
  control.el,
  verdict.el,
  runMeta,
  summaryNote,
);
const probesPanel = h('div', { class: 'tab-panel', role: 'tabpanel', 'aria-label': '分评' },
  probeSettings.el,
  h('div', { class: 'section-head' }, metaLine, runClock),
  grid.el,
);

const footer = h('footer', { class: 'footer' },
  h('span', {}, 'ProxyLens · 本地运行 · 数据不外传'),
  h('span', {}, '探测目标固定为官方站点；结果仅代表本次链路状态'),
);

// 先清掉 index.html 里的启动占位（.boot），否则它会永久留在页面顶部
clear(root);
root.appendChild(topbar.el);
root.appendChild(tabs.el);
root.appendChild(verdictPanel);
root.appendChild(probesPanel);
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
    store.patch({ tab: 'probes' });
    const res = await api.run(cfg);
    store.patch({ runId: res.runId });
  } catch (err) {
    store.patch({ phase: 'error', error: err.message });
    toast(err.message, 'error');
  }
}

// 单项复测：只跑这一项，保留其它卡的结果，也不动总评。
// 参数取自分评自己的 probeConfig，不读总评的时长/流量/并发。
async function probeOnly(id) {
  const st = store.state;
  if (st.phase === 'running') return;
  const prevPhase = st.phase;
  const pc = st.probeConfig[id] || {};
  // longRun 强制关闭：一次「只测这项」不应该跑 30 分钟
  const cfg = { ...st.config, probes: [id], only: true, longRun: false, ...pc };
  try {
    store.patch({ phase: 'running', onlyId: id, probeTarget: id, startedAt: Date.now(), error: '', tab: 'probes' });
    const res = await api.run(cfg);
    store.patch({ runId: res.runId });
  } catch (err) {
    store.patch({ phase: prevPhase, onlyId: null });
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

function renderRunMeta(state) {
  if (!state.summary) {
    runMeta.hidden = true;
    return;
  }
  runMeta.hidden = false;
  const preset = { quick: '快速档', standard: '标准档', deep: '深度档' }[state.config.preset] || '快速档';
  const mode = state.config.mode === 'custom'
    ? '自定义代理'
    : state.config.mode === 'direct' ? '直连对照' : '系统代理';
  const secs = state.summaryElapsedMs ? (state.summaryElapsedMs / 1000).toFixed(1) + 's' : '—';
  runMeta.textContent = `${preset} · ${mode} · ${(state.results || []).length} 项 · 用时 ${secs}`;
}

function renderTabs(state) {
  tabs.render(state.tab);
  const onVerdict = state.tab === 'verdict';
  verdictPanel.hidden = !onVerdict;
  probesPanel.hidden = onVerdict;

  probeSettings.render(state);
  renderRunMeta(state);

  const retested = (state.results || []).filter((r) => r.only);
  summaryNote.hidden = retested.length === 0;
  if (retested.length) {
    summaryNote.textContent = `分评中有 ${retested.length} 项单项复测（${retested.map((r) => r.id).join('、')}），总评仍来自上一次完整探测，未重算。`;
  }
}

store.subscribe((state) => {
  if (state.phase !== lastPhase) {
    lastPhase = state.phase;
    control.render();
    topbar.render();
  }
  renderVerdict(state);
  renderTabs(state);
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
    runClock.textContent = `本次 ${secs.toFixed(1)}s`;
  } else if (st.elapsedMs) {
    runClock.textContent = `上次 ${(st.elapsedMs / 1000).toFixed(1)}s`;
  }
}, 100);

// —— 事件流 ——
let es = null;
function bindStream() {
  es = api.subscribe({
    snapshot: (st) => {
      const patch = {
        phase: st.running ? 'running' : (st.summary ? 'done' : 'idle'),
        exit: st.exit || null,
        env: st.env || null,
        results: st.results || [],
        summary: st.summary || null,
        onlyId: st.running && st.only ? ((st.config && st.config.probes && st.config.probes[0]) || null) : null,
        summaryElapsedMs: st.only ? store.state.summaryElapsedMs : (st.elapsedMs || 0),
      };
      // 单项复测的 config 只含一个探测项，不能覆盖用户当前的完整配置
      if (!st.only) patch.config = { ...store.state.config, ...(st.config || {}) };
      store.patch(patch);
    },
    run_start: (p) => {
      if (p.only) {
        store.patch({
          phase: 'running',
          onlyId: (p.config && p.config.probes && p.config.probes[0]) || null,
          startedAt: Date.now(),
          tab: 'probes',
        });
        return;
      }
      resetForRun(p.config, p.exit);
      store.patch({ env: p.env, tab: 'probes' });
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
      // 单项复测不覆盖「上一次完整探测」的耗时
      summaryElapsedMs: p.only ? store.state.summaryElapsedMs : p.elapsedMs,
      onlyId: null,
      // 单项复测留在分评页看结果；完整运行结束后回到总评
      tab: p.only ? 'probes' : 'verdict',
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
    const patch = {
      phase: st.running ? 'running' : (st.summary ? 'done' : 'idle'),
      exit: st.exit || null,
      env: st.env || null,
      results: st.results || [],
      summary: st.summary || null,
      onlyId: st.running && st.only ? ((st.config && st.config.probes && st.config.probes[0]) || null) : null,
      summaryElapsedMs: st.only ? store.state.summaryElapsedMs : (st.elapsedMs || 0),
    };
    if (!st.only) patch.config = { ...store.state.config, ...(st.config || {}) };
    store.patch(patch);
  } catch (err) {
    toast('初始化失败：' + err.message, 'error');
  }
  bindStream();
  const st = store.state;
  renderVerdict(st);
  renderTabs(st);
  grid.render(st);
  control.render();
  topbar.render();
})();
