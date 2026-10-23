import { h, clear, fmt, NONE } from './core/dom.js';
import { store, resetForRun } from './core/store.js';
import { api } from './core/api.js';
import { applyTheme, watchSystemTheme } from './core/theme.js';
import { ORDER, NAME, resolveProbe } from './core/probes.js';
import { Sidebar } from './components/Sidebar.js';
import { TopBar } from './components/TopBar.js';
import { VerdictPanel } from './components/VerdictPanel.js';
import { ProbeGrid } from './components/ProbeGrid.js';
import { DetailDrawer } from './components/DetailDrawer.js';
import { SettingsPanel } from './components/SettingsPanel.js';
import { BandwidthPage } from './components/BandwidthPage.js';
import { StatusBadge } from './components/StatusBadge.js';
import { toast } from './components/Toast.js';

const root = document.getElementById('app');
applyTheme(store.state.theme);
watchSystemTheme(() => {
  if (store.state.theme === 'auto') applyTheme('auto');
});

const PRESET_LABEL = { quick: '快速档', standard: '标准档', deep: '深度档' };

function modeLabel(cfg, exit) {
  if (cfg.mode === 'custom') return '自定义代理';
  if (cfg.mode === 'direct') return '直连对照';
  return '系统代理' + (exit && exit.detail ? ' · ' + exit.detail : '');
}

function exportReport(format) {
  if (!store.state.summary) {
    toast('还没有可导出的结果', 'error');
    return;
  }
  window.open(api.reportUrl(format), '_blank');
}

// URL hash 承接「当前在哪一页」：刷新后停在原地，也能把某一项直接分享出去。
// 只在启动时读一次；之后切页只单向把状态写进 hash，不监听 hashchange——
// store 已经是唯一真源，再让它反过来驱动状态只会绕成环。
function viewFromHash() {
  const m = /^#(verdict|probes|bandwidth|settings)(?:\/(P[1-6]))?$/.exec(location.hash || '');
  if (!m) return null;
  const out = { tab: m[1] };
  if (m[2]) Object.assign(out, { probeTarget: m[2], openPanel: m[2] });
  return out;
}

function syncHash(state) {
  const tail = state.tab === 'probes' && state.openPanel ? '/' + state.openPanel : '';
  const want = '#' + state.tab + tail;
  if (location.hash !== want) history.replaceState(null, '', want);
}

// —— 装配 ——
const dock = Sidebar({ onChange: (k) => store.patch({ tab: k }) });
const topbar = TopBar({
  onRun: () => startRun(store.state.config),
  onCancel: cancelRun,
  onExport: exportReport,
});
const verdict = VerdictPanel({
  // 点一颗卵石 = 去分评看那一项，并把它的参数面板一起打开
  onPickProbe: (id) => store.patch({ tab: 'probes', probeTarget: id, openPanel: id }),
});
const grid = ProbeGrid({ onOpen: openDetail, onProbeOnly: probeOnly });
const settings = SettingsPanel();
// 带宽页自己管起停：那一档一跑就是半小时起，按钮必须贴着参数放
const bwPage = BandwidthPage({ onStart: startBandwidth, onStop: cancelRun });

// —— 总评页：逐项证据 ——
// 六行常驻实例，只更新文字：P1 采样时每秒都会 patch，整页重建会闪。
const evidence = h('div', { class: 'rows' });
const evidenceRows = new Map();
for (const id of ORDER) {
  const tag = h('span', { class: 'tag tag-idle' }, '待测');
  const value = h('span', { class: 'row-value' }, NONE);
  const detail = h('span', { class: 'row-detail' }, '');
  const row = h('button', {
    class: 'row',
    type: 'button',
    'aria-label': id + ' ' + NAME[id] + ' 详情',
    onclick: () => openDetail(id),
  },
    h('span', { class: 'row-id' }, id),
    h('span', { class: 'row-name', title: NAME[id] }, NAME[id]),
    tag,
    value,
    detail,
  );
  evidenceRows.set(id, { row, tag, value, detail });
  evidence.appendChild(row);
}

// 整体设置的摘要条：总评页不该让用户为了确认「按什么条件跑的」跳到设置页
const stripMain = h('span', { class: 'strip-main' }, '');
const setupStrip = h('button', {
  class: 'setup-strip',
  type: 'button',
  onclick: () => store.patch({ tab: 'settings' }),
},
  stripMain,
  h('span', { class: 'strip-go' }, '去改'),
);

// 单项复测不重算总评，但必须在总评页说清楚，否则用户会以为总评被刷新了
const summaryNote = h('div', { class: 'hint', hidden: true });

const verdictView = h('section', { class: 'view' },
  verdict.el,
  summaryNote,
  h('div', { class: 'block' },
    h('h3', {}, '逐项证据'),
    evidence,
  ),
  setupStrip,
);

// —— 分评页 ——
const runClock = h('span', { class: 'hint' }, '');
const probesView = h('section', { class: 'view' },
  h('div', { class: 'block' },
    h('div', { class: 'block-head' },
      h('h3', {}, '六项真实探测'),
      runClock,
    ),
    h('p', { class: 'hint' }, '每项都能单独调参数。「这项设置」里改的只对这一项生效，不动整体设置；六项一起跑仍按整体设置。'),
    grid.el,
  ),
);

const footer = h('footer', { class: 'slab-foot' },
  h('span', {}, 'ProxyLens · 本地运行 · 数据不外传'),
  h('span', {}, '结果只代表本次链路状态'),
);

clear(root);
root.appendChild(dock.el);
root.appendChild(h('main', { class: 'stage' },
  topbar.el,
  h('div', { class: 'slab' }, verdictView, probesView, bwPage.el, settings.el, footer),
));

let drawer = null;

function openDetail(id) {
  const r = store.state.results.find((x) => x.id === id);
  if (!r) return;
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

function proxyCheck(v) {
  if (!v.trim()) return { ok: false, msg: '选择了自定义代理，但代理地址为空' };
  if (!/^(https?|socks5h?):\/\//i.test(v.trim())) return { ok: false, msg: '代理地址需要以 http:// https:// socks5:// socks5h:// 开头' };
  if (!/:\d{1,5}\s*$/.test(v.trim())) return { ok: false, msg: '代理地址缺少端口，例如 127.0.0.1:7890' };
  return { ok: true };
}

async function startRun(cfg) {
  if (store.state.phase === 'running') return;
  const v = proxyCheck(cfg.proxyUrl);
  if (cfg.mode === 'custom' && (!cfg.proxyUrl.trim() || !v.ok)) {
    toast(v.msg || '请先填写代理地址', 'error');
    store.patch({ tab: 'settings' });
    return;
  }
  // 完整探测永远按整体设置跑：长跑只属于 P1 的单测，不进这里
  const full = { ...cfg, probes: [...ORDER], only: false, longRun: false };
  try {
    resetForRun(full, store.state.exit);
    store.patch({ tab: 'probes' });
    const res = await api.run(full);
    store.patch({ runId: res.runId });
  } catch (err) {
    store.patch({ phase: 'error', error: err.message });
    toast(err.message, 'error');
  }
}

// 单项复测：只跑这一项，保留其它项的结果，也不动总评。
// 参数 = 整体设置 + 这项的覆盖项；长跑开关只在这里能生效。
async function probeOnly(id) {
  const st = store.state;
  if (st.phase === 'running') return;
  const prevPhase = st.phase;
  const resolved = resolveProbe(st.config, id, st.probeConfig);
  const cfg = { ...resolved, probes: [id], only: true };
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

// 独立带宽长跑。参数是"小时 / GB"，走 bandwidth:true 的专用入口，
// 既不进六项结果，也不参与总评——跑两小时不该改写"这套代理能不能用"。
async function startBandwidth(cfg) {
  const st = store.state;
  if (st.phase === 'running') {
    toast('综合测试正在跑。带宽长跑要独占带宽，并行会让两边的数都不准', 'error');
    return;
  }
  const v = proxyCheck(cfg.proxyUrl);
  if (cfg.mode === 'custom' && (!cfg.proxyUrl.trim() || !v.ok)) {
    toast(v.msg || '请先填写代理地址', 'error');
    store.patch({ tab: 'settings' });
    return;
  }
  try {
    bwPage.reset();
    store.patch({
      bandwidth: {
        runId: null,
        running: true,
        startedAt: new Date().toISOString(),
        result: null,
        error: '',
        config: cfg,
      },
    });
    const res = await api.run(cfg);
    store.patch({ bandwidth: { ...store.state.bandwidth, runId: res.runId } });
  } catch (err) {
    store.patch({ bandwidth: { ...(store.state.bandwidth || {}), running: false } });
    toast(err.message, 'error');
  }
}

// —— 状态驱动渲染 ——
let verdictSig = '';

// 结论区只在语义变化时重绘：每次采样都重建会重置过渡、造成闪烁
function renderVerdict(state) {
  const s = state.summary;
  const sig = s
    ? [s.total, s.state, s.grade, (s.caps || []).length, (s.advice || []).length].join('|')
    : 'empty|' + state.phase;
  if (sig === verdictSig) return;
  verdictSig = sig;
  verdict.render(state);
}

function renderEvidence(state) {
  const results = state.results || [];
  const running = state.phase === 'running';
  for (const id of ORDER) {
    const r = results.find((x) => x.id === id);
    const ui = evidenceRows.get(id);
    clear(ui.tag);
    if (r) {
      const badge = StatusBadge(r.status);
      ui.tag.className = badge.className;
      ui.tag.textContent = badge.textContent;
    } else {
      ui.tag.className = 'tag tag-idle';
      ui.tag.textContent = running ? '排队中' : '待测';
    }
    ui.value.textContent = r ? (r.headline || NONE) : NONE;
    // 证据列放「花了多久 / 有没有中途断流」，不放状态码——那是细节里的东西
    const bits = [];
    if (r && r.durationMs) bits.push(fmt.dur(r.durationMs));
    if (r && r.stalled) bits.push('中途断流');
    if (r && r.only) bits.push('单项复测');
    ui.detail.textContent = bits.join(' · ');
  }
}

function renderStrip(state) {
  const cfg = state.config;
  const count = (cfg.probes || ORDER).length;
  stripMain.textContent = '整体设置 · ' + modeLabel(cfg, state.exit) +
    ' · ' + (PRESET_LABEL[cfg.preset] || '快速档') + ' · ' + count + ' 项一起跑';
}

function renderViews(state) {
  dock.render();
  topbar.render();

  const t = state.tab;
  verdictView.hidden = t !== 'verdict';
  probesView.hidden = t !== 'probes';
  bwPage.el.hidden = t !== 'bandwidth';
  settings.el.hidden = t !== 'settings';

  settings.render();
  // 带宽页只在被看见时重绘：它里面有 canvas，藏起来时画了也是白画
  if (t === 'bandwidth') bwPage.render();
  renderEvidence(state);
  renderStrip(state);
  syncHash(state);

  const retested = (state.results || []).filter((r) => r.only);
  summaryNote.hidden = retested.length === 0;
  if (retested.length) {
    summaryNote.textContent = '分评里有 ' + retested.length + ' 项是单项复测（' +
      retested.map((r) => r.id).join('、') + '），总评仍来自上一次完整探测，未重算。';
  }
}

store.subscribe((state) => {
  renderVerdict(state);
  renderViews(state);
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
    runClock.textContent = '本次 ' + ((Date.now() - st.startedAt) / 1000).toFixed(1) + 's';
  } else if (st.elapsedMs) {
    runClock.textContent = '上次 ' + (st.elapsedMs / 1000).toFixed(1) + 's';
  } else {
    runClock.textContent = '';
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
      // 带宽长跑的进度只从快照认：事件流断线重连之后，靠它把状态接回来
      if (st.bandwidth) patch.bandwidth = { ...(store.state.bandwidth || {}), ...st.bandwidth };
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
      resetForRun({ ...store.state.config, ...(p.config || {}) }, p.exit);
      store.patch({ env: p.env, tab: 'probes' });
    },
    probe_start: (p) => {
      store.upsertResult({ id: p.id, name: p.name, group: p.group, status: 'running', headline: '探测中…', subline: '', phases: {}, samples: [] });
    },
    sample: (p) => {
      store.updateResult(p.id, (r) => {
        const samples = [...(r.samples || []), p.sample].slice(-240);
        return { ...r, samples, headline: p.sample.mbps.toFixed(1) + ' Mbps' };
      });
    },
    probe_result: (r) => store.upsertResult(r),
    score: (s) => store.patch({ summary: s }),
    // ── 独立带宽长跑：与上面六个事件完全平行的另一条通道 ──
    bw_start: (p) => {
      bwPage.reset();
      store.patch({
        bandwidth: {
          runId: p.runId,
          running: true,
          startedAt: new Date().toISOString(),
          result: null,
          error: '',
          config: p.config || null,
          exit: p.exit || null,
        },
      });
    },
    // 不走 store：4 小时是 57,600 个点，每来一个就 patch 一次状态，
    // 等于每秒四次把整棵树重新发布给所有订阅者。曲线自己管缓冲。
    bw_sample: (p) => bwPage.push(p.sample),
    bw_result: (p) => store.patch({
      bandwidth: { ...(store.state.bandwidth || {}), result: p.result },
    }),
    bw_end: (p) => store.patch({
      bandwidth: {
        ...(store.state.bandwidth || {}),
        running: false,
        error: p.error || '',
        elapsedMs: p.elapsedMs || 0,
      },
    }),
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
// Enter 只在没有聚焦任何控件时启动：在输入框里按 Enter 应该是「填完了」而不是「开始跑」
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') { closeDetail(); return; }
  const tag = document.activeElement && document.activeElement.tagName;
  if (e.key === 'Enter' && !['INPUT', 'TEXTAREA', 'BUTTON'].includes(tag)) {
    if (store.state.phase !== 'running') startRun(store.state.config);
  }
});

// —— 启动 ——
(async () => {
  // 先认地址栏，再拉状态：否则完整探测结束时的「回到总评」会把深链冲掉
  const deep = viewFromHash();
  if (deep) store.patch(deep);
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
    if (st.bandwidth) patch.bandwidth = st.bandwidth;
    store.patch(patch);
  } catch (err) {
    toast('初始化失败：' + err.message, 'error');
  }
  bindStream();
  const st = store.state;
  renderVerdict(st);
  renderViews(st);
  grid.render(st);
})();
