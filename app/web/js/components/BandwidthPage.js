// 独立带宽长跑页。
//
// 这一页和总评 / 分评没有任何共享结果：它测的不是"这套代理能不能用"，
// 而是"连续跑两小时，它会不会自己掉下来"。两件事混在一起存，
// 分评页那张 P1 卡就不再是综合测试测出来的那个数了。

import { h, clear, fmt } from '../core/dom.js';
import { store } from '../core/store.js';
import { BW, durationLabel, budgetLabel, budgetVerdict } from '../core/bandwidth.js';
import { createBandwidthChart } from './BandwidthChart.js';
import { toast } from './Toast.js';

// 时长说法必须覆盖到小时。
//
// fmt.dur 只到分钟，这条链路上跑 4 小时会读成「已跑 240分」——对一个
// 专门做长跑的页面来说，这是把最重要的一位数说拧了。
function longDur(ms) {
  const total = Math.max(0, Math.round((ms || 0) / 1000));
  const hr = Math.floor(total / 3600);
  const min = Math.floor((total % 3600) / 60);
  const sec = total % 60;
  if (hr) return min ? `${hr}小时${min}分` : `${hr}小时`;
  if (min) return sec ? `${min}分${sec}秒` : `${min}分`;
  return `${sec}秒`;
}

// 采样点上限。后端每 250ms 一个点，4 小时是 57,600 个——
// 全留会拖死内存和绘制。超限就两两合并：形状保住、点数减半，
// 合并是摊还 O(1)，不会随着跑得越久每次插入越慢。
const MAX_POINTS = 1800;

export function BandwidthPage({ onStart, onStop }) {
  const chart = createBandwidthChart();

  let pts = [];
  let last = null;

  // 有效吞吐与断流必须按 5 秒窗口算，和后端 concludeLongRun 同一口径。
  // 按 250ms 采样点算会得出两套互相打脸的数：实测同一次下载，
  // 250ms 口径说"有效时长 50%"，5 秒窗口口径说"100%"——后者才对，
  // 因为 250ms 的空点是 Range 请求之间的正常间隙，不是链路停了。
  const WIN_MS = 5000;
  let winStartT = -1;
  let winStartBytes = 0;
  let winRates = [];
  let winZeroRun = 0;
  let stalled = false;
  let sustain = 0;

  // ── 读数区 ──
  const nowNum = h('span', { class: 'bw-now-num' }, '—');
  const nowUnit = h('span', { class: 'bw-now-unit' }, 'Mbps');
  const nowSub = h('span', { class: 'bw-now-sub' }, '尚未开始');

  function stat(label) {
    const v = h('span', { class: 'bw-stat-v' }, '—');
    return { v, el: h('div', { class: 'bw-stat' }, h('span', { class: 'bw-stat-k' }, label), v) };
  }
  const sAvg = stat('全程平均');
  const sPeak = stat('峰值');
  const sBytes = stat('已下载');
  const sSustain = stat('有效时长占比');
  const sStall = stat('断流');

  // 两根进度条：时间走完和流量用完，哪个先到头就是这次结束的原因。
  // 长跑最怕的就是"以为还在跑，其实早就被预算截断了"。
  const timeBar = h('div', { class: 'bw-meter-bar' });
  const timeVal = h('span', { class: 'bw-meter-v' }, '—');
  const budBar = h('div', { class: 'bw-meter-bar' });
  const budVal = h('span', { class: 'bw-meter-v' }, '—');

  const meter = (k, bar, val) => h('div', { class: 'bw-meter' },
    h('span', { class: 'bw-meter-k' }, k),
    h('div', { class: 'bw-meter-track' }, bar),
    val,
  );

  const liveBox = h('div', { class: 'bw-live' },
    h('div', { class: 'bw-now' }, nowNum, nowUnit, nowSub),
    chart.el,
    h('div', { class: 'bw-stats' },
      sAvg.el, sPeak.el, sBytes.el, sSustain.el, sStall.el,
      h('div', { class: 'bw-stat bw-stat-wide' },
        h('span', { class: 'bw-stat-k' }, '进度'),
        h('div', { class: 'bw-meters' }, meter('时间', timeBar, timeVal), meter('流量', budBar, budVal)),
      ),
    ),
  );

  // ── 配置区 ──
  const segDur = h('div', { class: 'seg seg-sm bw-seg', role: 'group', 'aria-label': '长跑时长' });
  const segBud = h('div', { class: 'seg seg-sm bw-seg', role: 'group', 'aria-label': '流量预算' });
  const segCon = h('div', { class: 'seg seg-sm bw-seg', role: 'group', 'aria-label': '并发连接' });

  const durBtns = new Map();
  for (const d of BW.durations) {
    const b = h('button', { type: 'button', onclick: () => setCfg({ durationSec: d.sec }) }, d.label);
    durBtns.set(d.sec, b);
    segDur.appendChild(b);
  }
  const budBtns = new Map();
  for (const d of BW.budgets) {
    const b = h('button', { type: 'button', onclick: () => setCfg({ budgetMB: d.mb }) }, d.label);
    budBtns.set(d.mb, b);
    segBud.appendChild(b);
  }
  const conBtns = new Map();
  for (const c of BW.concurrencies) {
    const b = h('button', { type: 'button', onclick: () => setCfg({ concurrency: c }) },
      c === 1 ? '单流' : c + ' 条');
    conBtns.set(c, b);
    segCon.appendChild(b);
  }

  const budgetNote = h('p', { class: 'bw-note' }, '');

  const startBtn = h('button', {
    class: 'btn btn-go bw-start',
    type: 'button',
    onclick: () => beginRun(),
  }, '开始长跑');

  const stopBtn = h('button', {
    class: 'btn bw-stop',
    type: 'button',
    hidden: true,
    onclick: () => onStop(),
  }, '停止');

  const setupBox = h('div', { class: 'bw-setup' },
    h('div', { class: 'bw-row' }, h('span', { class: 'bw-k' }, '跑多久'), segDur),
    h('div', { class: 'bw-row' }, h('span', { class: 'bw-k' }, '流量预算'), segBud),
    h('div', { class: 'bw-row' }, h('span', { class: 'bw-k' }, '并发'), segCon),
    budgetNote,
    h('div', { class: 'bw-acts' }, startBtn, stopBtn),
  );

  const resultBox = h('div', { class: 'bw-result', hidden: true });

  const el = h('section', { class: 'view bw-page' },
    h('div', { class: 'bw-head' },
      h('h2', {}, '长时间大带宽'),
      h('p', { class: 'lede' }, '连续下载，看这条链路能不能一直跑到该有的速度。中途掉下去的那几段会被单独标出来——短测的均值会把「跑了 40 秒就停」摊成一个还看得过去的数字。'),
    ),
    setupBox,
    liveBox,
    resultBox,
  );

  // ── 配置读写 ──

  function cfg() {
    return store.state.bwConfig || { ...BW.default };
  }

  // 已经跑起来（或跑完了）之后，进度条和坐标轴必须按「那一跑」的实际参数算。
  // 用页面草稿算的话，跑完 25 秒的那次会把曲线压在 30 分钟坐标轴最左边一条缝里，
  // 进度条还显示「0 秒 / 30 分钟」——看起来像一次失败的测试。
  function activeCfg() {
    const bw = store.state.bandwidth;
    const c = cfg();
    if (bw && bw.config && (bw.running || bw.result)) {
      // 后端 config 的字段名是 budgetMb（历史拼法），页面自己的草稿用的是
      // budgetMB。两个都要认——只认一个会让进度条回落到草稿值。
      return {
        durationSec: bw.config.durationSec || c.durationSec,
        budgetMB: bw.config.budgetMB || bw.config.budgetMb || c.budgetMB,
        concurrency: bw.config.concurrency || 1,
      };
    }
    return c;
  }

  function setCfg(patch) {
    const st = store.state;
    if (st.bandwidth && st.bandwidth.running) return;
    store.patch({ bwConfig: { ...cfg(), ...patch } });
  }

  function beginRun() {
    const c = cfg();
    const st = store.state;
    if (st.phase === 'running') {
      toast('综合测试正在跑。带宽长跑要独占带宽，并行会让两边的数都不准', 'error');
      return;
    }
    if (st.bandwidth && st.bandwidth.running) return;
    onStart({
      bandwidth: true,
      mode: st.config.mode,
      proxyUrl: st.config.proxyUrl,
      durationSec: c.durationSec,
      budgetMB: c.budgetMB,
      concurrency: c.concurrency,
    });
  }

  // ── 实时采样 ──

  function pct50(arr) {
    if (!arr.length) return 0;
    const s = arr.slice().sort((a, b) => a - b);
    const i = (s.length - 1) / 2;
    const lo = Math.floor(i);
    const hi = Math.ceil(i);
    return lo === hi ? s[lo] : (s[lo] + s[hi]) / 2;
  }

  function closeWindow(sample) {
    const dt = (sample.t - winStartT) / 1000;
    if (!(dt > 0)) return;
    const rate = ((sample.bytes - winStartBytes) * 8) / 1e6 / dt;
    winRates.push(rate);
    if (rate <= 0.05) {
      winZeroRun++;
      // 连续两个空窗（10 秒）才算断流，与后端 dlStallWindows 一致。
      if (winZeroRun >= 2) stalled = true;
    } else {
      winZeroRun = 0;
    }
    // 阈值 = 已结算窗口中位数的 20%，和后端「有效吞吐」同一把尺子。
    const th = pct50(winRates) * 0.2;
    const good = winRates.filter((r) => r > th).length;
    sustain = (good / winRates.length) * 100;
    winStartT = sample.t;
    winStartBytes = sample.bytes;
  }

  function push(sample) {
    if (!sample || typeof sample.mbps !== 'number') return;
    last = sample;
    if (winStartT < 0) {
      winStartT = sample.t;
      winStartBytes = sample.bytes;
    } else if (sample.t - winStartT >= WIN_MS) {
      closeWindow(sample);
    }
    pts.push({ t: sample.t, mbps: sample.mbps });
    if (pts.length >= MAX_POINTS * 2) {
      const merged = [];
      for (let i = 0; i < pts.length; i += 2) {
        const a = pts[i];
        const b = pts[i + 1] || a;
        merged.push({ t: (a.t + b.t) / 2, mbps: (a.mbps + b.mbps) / 2 });
      }
      pts = merged;
    }
    paintLive();
  }

  function reset() {
    pts = [];
    last = null;
    winStartT = -1;
    winStartBytes = 0;
    winRates = [];
    winZeroRun = 0;
    stalled = false;
    sustain = 0;
  }

  // 中途才打开页面时，SSE 只能给出"从这一刻起"的增量。
  // 快照里带着已经跑过的采样序列，用它把前半段补齐——
  // 否则下了 628 MB 的十分钟长跑，看起来像刚点了开始。
  function groupWindows(list) {
    const out = [];
    let base = null;
    for (const s of list) {
      if (!base) { base = s; continue; }
      const dt = (s.t - base.t) / 1000;
      if (dt >= WIN_MS / 1000) {
        out.push(((s.bytes - base.bytes) * 8) / 1e6 / dt);
        base = s;
      }
    }
    const th = pct50(out) * 0.2;
    sustain = out.length ? (out.filter((r) => r > th).length / out.length) * 100 : 0;
    let run = 0;
    stalled = false;
    for (const r of out) {
      if (r <= 0.05) {
        run++;
        if (run >= 2) stalled = true;
      } else {
        run = 0;
      }
    }
    // 把这半段末尾的空窗计数交出去，后续 closeWindow 才能接着数下去；
    // 否则刷新恰好落在断流中段时，断流会被判丢。
    winZeroRun = run;
    return out;
  }

  function seedRun(bw) {
    const list = bw && bw.samples;
    if (!Array.isArray(list) || list.length < 2 || pts.length) return;
    pts = list.map((s) => ({ t: s.t, mbps: s.mbps }));
    const tail = list[list.length - 1];
    last = { t: tail.t, mbps: tail.mbps, bytes: tail.bytes || 0 };
    winRates = groupWindows(list);
    winStartT = tail.t;
    winStartBytes = tail.bytes || 0;
  }

  // 刷新页面后本地缓冲是空的，用后端结果里的窗口序列补上，
  // 否则"跑完了但图是白板"——数据其实一直在结果里。
  function seedFromResult(res) {
    if (!res || !Array.isArray(res.samples) || !res.samples.length) return;
    if (pts.length) return;
    pts = res.samples.map((s) => ({ t: s.t, mbps: s.mbps }));
    const finalBytes = res.bytes || (res.samples[res.samples.length - 1] || {}).bytes || 0;
    last = { t: res.samples[res.samples.length - 1].t, mbps: res.mbps || 0, bytes: finalBytes };
    winRates = res.samples.map((s) => s.mbps);
    stalled = !!res.stalled;
    sustain = res.sustainPct || 0;
  }

  function paintLive() {
    const c = activeCfg();
    const totalMs = c.durationSec * 1000;
    const budgetBytes = c.budgetMB * 1024 * 1024;
    const bwState = store.state.bandwidth || {};
    const done = bwState.result && !bwState.running ? bwState.result : null;
    // 跑完之后「已跑多久」只认服务端结算的时长。
    // 最后一个采样点总比收尾早几秒，两边各报一个数会让人怀疑测试本身不准。
    const t = (done && done.durationMs) || (last ? last.t : 0);
    const bytes = last ? last.bytes : 0;

    nowNum.textContent = last ? last.mbps.toFixed(last.mbps >= 100 ? 0 : 1) : '—';
    // 此刻恰好 0 的时候必须说清楚"是现在没数据"，而不是让一个 0.0
    // 孤零零站在平均 27 Mbps 旁边——看起来像页面算错了。
    const idle = !!last && last.mbps <= 0.05;
    nowNum.classList.toggle('is-zero', idle);
    nowSub.textContent = !last
      ? '尚未开始'
      : (done
        // 跑完了就没有"当前速率"这回事了，改说这是收尾那一刻的读数，
        // 免得它和下面结果里的「全程平均」被当成同一件事。
        ? `结束时的速率 · 全程 ${longDur(t)}`
        : (idle
          ? `此刻没有数据进来 · 已跑 ${longDur(t)}`
          : `当前速率 · 已跑 ${longDur(t)}${stalled ? ' · 正在断流' : ''}`));

    sAvg.v.textContent = t > 0 && bytes > 0 ? fmt.mbps((bytes * 8) / 1e6 / (t / 1000)) : '—';
    sPeak.v.textContent = pts.length ? fmt.mbps(Math.max(...pts.map((p) => p.mbps))) : '—';
    sBytes.v.textContent = bytes ? fmt.bytes(bytes) : '—';
    sSustain.v.textContent = winRates.length ? fmt.pct(sustain) : '—';
    sSustain.v.classList.toggle('is-warn', winRates.length > 0 && sustain < 90);
    sStall.v.textContent = stalled ? '有' : (winRates.length ? '无' : '—');
    sStall.v.classList.toggle('is-bad', stalled);

    const tPct = totalMs > 0 ? Math.min(100, (t / totalMs) * 100) : 0;
    timeBar.style.width = tPct.toFixed(1) + '%';
    timeVal.textContent = `${longDur(t)} / ${longDur(c.durationSec * 1000)}`;

    const bPct = budgetBytes > 0 ? Math.min(100, (bytes / budgetBytes) * 100) : 0;
    budBar.style.width = bPct.toFixed(1) + '%';
    budBar.classList.toggle('is-near', bPct >= 92);
    budVal.textContent = `${fmt.bytes(bytes)} / ${budgetLabel(c.budgetMB)}`;

    chart.setData(pts, { totalMs, running: !!(store.state.bandwidth && store.state.bandwidth.running) });
  }

  // ── 配置区状态 ──

  function paintSetup(running) {
    const c = cfg();
    const bw = store.state.bandwidth;
    const p1 = (store.state.results || []).find((r) => r.id === 'P1');
    const measured = (bw && bw.result && bw.result.mbps) || (p1 && p1.mbps) || 0;

    for (const [k, b] of durBtns) b.setAttribute('aria-pressed', String(c.durationSec === k));
    for (const [k, b] of budBtns) b.setAttribute('aria-pressed', String(c.budgetMB === k));
    for (const [k, b] of conBtns) b.setAttribute('aria-pressed', String(c.concurrency === k));
    for (const b of [...durBtns.values(), ...budBtns.values(), ...conBtns.values()]) b.disabled = running;

    const v = budgetVerdict(measured, c.durationSec, c.budgetMB);
    budgetNote.textContent = v.text;
    budgetNote.dataset.tone = v.tone;

    const busyOther = store.state.phase === 'running';
    startBtn.hidden = running;
    stopBtn.hidden = !running;
    startBtn.disabled = busyOther;
    startBtn.textContent = busyOther ? '综合测试进行中…' : '开始长跑';
    startBtn.title = busyOther
      ? '综合测试正在占用带宽，并行会让两边都不准'
      : `按 ${durationLabel(c.durationSec)} / ${budgetLabel(c.budgetMB)} 跑一次大文件下载`;

    // 并发只是压测手段，不是常态：默认单流，改高时把代价说清楚。
    segCon.title = c.concurrency > 1
      ? '多条流会互相掩盖断流——某一条断了，总吞吐还在，图上看不出来'
      : '单流最接近"打开一个文件下载"的真实体感';
  }

  // ── 结论 ──

  function paintResult() {
    const bw = store.state.bandwidth;
    const r = bw && bw.result;
    clear(resultBox);
    if (!r) {
      resultBox.hidden = true;
      return;
    }
    resultBox.hidden = false;

    const state = r.stalled ? 'bad' : (r.sustainPct >= 90 ? 'ok' : 'warn');
    const verdict = r.stalled
      ? `第 ${longDur(r.stallAtMs)} 之后链路不再出数据`
      : (r.sustainPct >= 90
        ? '全程都在出数据，没有掉下去'
        : `有效吞吐只占了 ${fmt.pct(r.sustainPct)} 的时长`);

    const fact = (k, v, tone) => h('div', { class: 'bw-fact' },
      h('span', { class: 'bw-fact-k' }, k),
      h('span', { class: 'bw-fact-v' + (tone ? ' is-' + tone : '') }, v),
    );

    resultBox.appendChild(h('div', { class: 'bw-verdict', dataset: { tone: state } },
      h('div', { class: 'bw-verdict-main' },
        h('span', { class: 'bw-tag', dataset: { tone: state } }, r.stalled ? '中途断流' : '跑完了'),
        h('h3', {}, verdict),
        h('p', { class: 'lede' }, r.subline || ''),
      ),
      h('div', { class: 'bw-facts' },
        fact('平均', fmt.mbps(r.mbps)),
        fact('P50', fmt.mbps(r.p50Mbps)),
        fact('P90', fmt.mbps(r.p90Mbps)),
        fact('峰值', fmt.mbps(r.peakMbps)),
        fact('总下载', fmt.bytes(r.bytes)),
        fact('有效时长', fmt.pct(r.sustainPct), r.sustainPct >= 90 ? 'ok' : 'warn'),
        fact('波动系数', r.cv ? r.cv.toFixed(2) : '—'),
        fact('断流时刻', r.stallAtMs ? longDur(r.stallAtMs) : '无', r.stallAtMs ? 'bad' : ''),
      ),
      (r.evidence || []).length
        ? h('div', { class: 'bw-evidence' },
          h('span', { class: 'bw-fact-k' }, '过程记录'),
          h('div', { class: 'bw-evidence-list' }, ...r.evidence.map((t) => h('p', {}, t))),
        )
        : null,
    ));
  }

  // ── 对外 ──

  function render() {
    const bw = store.state.bandwidth;
    const running = !!(bw && bw.running);
    // 正在跑但本地缓冲是空的（中途才打开/刚刷新）→ 用后端快照里的序列补前半段。
    // 必须排在 seedFromResult 前面：两者都只在 pts 为空时生效，先到先得。
    if (bw && running) seedRun(bw);
    if (bw && bw.result) seedFromResult(bw.result);
    paintSetup(running);
    paintLive();
    paintResult();
    el.dataset.state = running ? 'run' : (bw && bw.result ? (bw.result.stalled ? 'bad' : 'ok') : 'idle');
  }

  render();
  return { el, render, push, beginRun, reset, repaint: chart.repaint };
}
