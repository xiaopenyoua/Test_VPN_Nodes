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

// MB/s = Mbps ÷ 8。Mbps 是行业口径（运营商、测速站都用它），
// MB/s 才是"这个速度多久能下完一个 2 GB 的文件"的直觉单位。两个都给，不做换算题。
// 有效位数按压力量级取：2.43 MB/s 的第三位没有意义，只会让数字来回跳。
function mbsText(mbps) {
  const v = Math.max(0, mbps || 0) / 8;
  if (v >= 100) return `${v.toFixed(0)} MB/s`;
  if (v >= 10) return `${v.toFixed(1)} MB/s`;
  return `${v.toFixed(2)} MB/s`;
}

// 采样点上限不需要额外防护：最长 4 小时 ÷ 5 秒窗口 = 2,880 个，
// 由后端配置的硬上限天然约束，合并/截断反而会把峰值抹平。

export function BandwidthPage({ onStart, onStop }) {
  const chart = createBandwidthChart();

  let pts = [];
  let last = null;

  // 采样口径：后端报上来的每个点**本身就是一个 5 秒窗口**（bw_window），
  // 与 concludeLongRun 的结论是同一套聚合。前端不再自己切窗——
  // 之前前端拿 250ms 瞬时点自己合成 5 秒窗，两边只差一点点舍入，
  // 跑动中显示的峰值和跑完给出的峰值就不是一个数了。
  let stalled = false;
  let sustain = 0;

  // ── 读数区 ──
  const nowNum = h('span', { class: 'bw-now-num' }, '—');
  const nowUnit = h('span', { class: 'bw-now-unit' }, 'Mbps');
  const nowAlt = h('span', { class: 'bw-now-alt' }, '');
  const nowSub = h('span', { class: 'bw-now-sub' }, '尚未开始');

  function stat(label) {
    const v = h('span', { class: 'bw-stat-v' }, '—');
    // MB/s 是副读数：同一条链路两种单位，Mbps 是行业口径（运营商/测速站
    // 都用它），MB/s 是人看文件大小时的直觉单位。两个都给，不做换算题。
    const alt = h('span', { class: 'bw-stat-alt' }, '');
    return { v, alt, el: h('div', { class: 'bw-stat' }, h('span', { class: 'bw-stat-k' }, label), v, alt) };
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
    h('div', { class: 'bw-now' }, nowNum, nowUnit, nowAlt, nowSub),
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

  // 每次都从全量窗口重算，不做增量。
  //
  // 中位数不是可增量合并的量：增量维护要么算错，要么得自己写一套
  // "带偏移的中位数"，两种都会让"有效时长占比"跑到后半段时悄悄失真。
  // 4 小时 = 2,880 个窗口、5 秒重算一次，代价可以忽略。
  function recompute() {
    const n = pts.length;
    if (!n) {
      sustain = 0;
      stalled = false;
      return;
    }
    const rates = pts.map((p) => p.mbps);
    const med = pct50(rates);
    // 中位数为 0 说明过半窗口根本没有数据。此时"低于基准 20%"这个判据塌了
    // （任何窗口都 >= 0），naive 实现会把彻底断流的一跑算成 100% 有效，
    // 正好把最该报警的情形洗白。后端在这种情况下直接给 0，这里跟它一致。
    if (med <= 0) {
      sustain = 0;
    } else {
      const th = med * 0.2;
      sustain = (rates.filter((r) => r >= th).length / n) * 100;
    }
    // 断流 = 连续两个窗口零字节（10 秒）。单窗空档多半只是重传或拥塞窗口收缩。
    let run = 0;
    stalled = false;
    for (const r of rates) {
      if (r <= 0.05) {
        run++;
        if (run >= 2) stalled = true;
      } else {
        run = 0;
      }
    }
  }

  function push(sample) {
    if (!sample || typeof sample.mbps !== 'number') return;
    last = sample;
    pts.push({ t: sample.t, mbps: sample.mbps, bytes: sample.bytes || 0 });
    recompute();
    paintLive();
  }

  function reset() {
    pts = [];
    last = null;
    stalled = false;
    sustain = 0;
  }

  // 中途才打开页面时，SSE 只能给出"从这一刻起"的增量。
  // 快照里带着已经跑过的窗口序列（engine 存的也是 5 秒窗口），用它把前半段补齐——
  // 否则下了 628 MB 的十分钟长跑，看起来像刚点了开始。
  function seedRun(bw) {
    const list = bw && bw.samples;
    if (!Array.isArray(list) || list.length < 1 || pts.length) return;
    pts = list.map((s) => ({ t: s.t, mbps: s.mbps, bytes: s.bytes || 0 }));
    const tail = list[list.length - 1];
    last = { t: tail.t, mbps: tail.mbps, bytes: tail.bytes || 0 };
    recompute();
  }

  // 刷新页面后本地缓冲是空的，用后端结果里的窗口序列补上，
  // 否则"跑完了但图是白板"——数据其实一直在结果里。
  function seedFromResult(res) {
    if (!res || !Array.isArray(res.samples) || !res.samples.length) return;
    const list = res.samples;
    // 后端结果里的序列比流里收到的更长时，整段换过来。
    // 收尾那个不满整窗的尾窗是在采样 goroutine 退出时才产生的，
    // 它可能赶不上 SSE——少了它，图上最高的那一点就可能和结果里的
    // 「峰值」不是同一个点，而这两个数在同一屏上挨着放。
    if (pts.length < list.length) {
      pts = list.map((s) => ({ t: s.t, mbps: s.mbps, bytes: s.bytes || 0 }));
    }
    const tail = pts[pts.length - 1];
    // 收尾那一刻的读数取**最后一个窗口**的速率，不是 res.mbps（那是均值）。
    // 拿均值去填"最后一窗的速率"会让同一个格子里出现两个平均数的错觉。
    last = { t: tail.t, mbps: tail.mbps, bytes: res.bytes || tail.bytes || 0 };
    // 结论口径以后端为准：前端的 recompute 只是跑动中的近似，
    // 已经结算过的一跑不该再被前端算一遍。
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
    // 跑完以后总字节也认服务端结算值：最后一个窗口流到前端时，
    // 收尾那几秒还在路上，用流里的累计值会让「已下载」比结果卡少几 MB。
    const bytes = (done && done.bytes) || (last ? last.bytes : 0);

    nowNum.textContent = last ? last.mbps.toFixed(last.mbps >= 100 ? 0 : 1) : '—';
    nowAlt.textContent = last ? mbsText(last.mbps) : '';
    // 此刻恰好 0 的时候必须说清楚"是现在没数据"，而不是让一个 0.0
    // 孤零零站在平均 27 Mbps 旁边——看起来像页面算错了。
    const idle = !!last && last.mbps <= 0.05;
    nowNum.classList.toggle('is-zero', idle);
    nowSub.textContent = !last
      ? '尚未开始'
      : (done
        // 跑完了就没有"当前速率"这回事了，改说这是收尾那一刻的读数，
        // 免得它和下面结果里的「均值」被当成同一件事。
        ? `最后一窗的速率 · 全程 ${longDur(t)}`
        : (idle
          ? `最近 5 秒没有数据进来 · 已跑 ${longDur(t)}`
          : `最近 5 秒的平均速率 · 已跑 ${longDur(t)}${stalled ? ' · 正在断流' : ''}`));

    // 平均只认总字节 ÷ 总时长：这是唯一不受"窗口怎么切"影响的数。
    const avg = t > 0 && bytes > 0 ? (bytes * 8) / 1e6 / (t / 1000) : 0;
    // 峰值 / 有效时长 / 断流：跑完以后一律以后端结算值为准。
    // 跑动中用本地窗口算的近似值——两套算法只在最后一位上有差别，
    // 但让界面上的数在收尾瞬间往回跳一下，比差那一位更让人怀疑工具。
    const peak = done ? (done.peakMbps || 0) : (pts.length ? Math.max(...pts.map((p) => p.mbps)) : 0);
    const sus = done ? (done.sustainPct || 0) : sustain;
    const stall = done ? !!done.stalled : stalled;
    const settled = !!done || pts.length > 0;

    sAvg.v.textContent = avg > 0 ? fmt.mbps(avg) : '—';
    sAvg.alt.textContent = avg > 0 ? mbsText(avg) : '';
    sPeak.v.textContent = peak > 0 ? fmt.mbps(peak) : '—';
    sPeak.alt.textContent = peak > 0 ? mbsText(peak) : '';
    sBytes.v.textContent = bytes ? fmt.bytes(bytes) : '—';
    sSustain.v.textContent = settled ? fmt.pct(sus) : '—';
    sSustain.v.classList.toggle('is-warn', settled && sus < 90);
    sStall.v.textContent = settled ? (stall ? '有' : '无') : '—';
    sStall.v.classList.toggle('is-bad', settled && stall);

    const tPct = totalMs > 0 ? Math.min(100, (t / totalMs) * 100) : 0;
    timeBar.style.width = tPct.toFixed(1) + '%';
    timeVal.textContent = `${longDur(t)} / ${longDur(c.durationSec * 1000)}`;

    const bPct = budgetBytes > 0 ? Math.min(100, (bytes / budgetBytes) * 100) : 0;
    budBar.style.width = bPct.toFixed(1) + '%';
    budBar.classList.toggle('is-near', bPct >= 92);
    budVal.textContent = `${fmt.bytes(bytes)} / ${budgetLabel(c.budgetMB)}`;

    chart.setData(pts, { totalMs, running: !!(store.state.bandwidth && store.state.bandwidth.running) });

    // 预算提示跟着实测速率走：跑到 30 秒已经能算出这一跑会不会撞上流量上限，
    // 而用户此刻最想知道的正是「要不要现在加预算」。
    // 跑动中用全程平均（总字节÷总时长）外推最稳；跑完了用结算的窗口均值。
    // 0 的时候不动——那是「还没测过」的状态，交给 paintSetup 说。
    const measuredNow = done ? (done.mbps || 0) : avg;
    if (measuredNow > 0) paintBudget(measuredNow, !!done);
  }

  // ── 配置区状态 ──

  // 预算提示条单独拆出来，因为它要在跑动中途刷新。
  //
  // 之前它只活在 paintSetup 里，而 paintSetup 只由 render() 调用（切换页面、
  // 开跑、跑完）。结果是跑到第 2 分钟，提示条还停在点「开始」之前那句话上——
  // 一条永远不变的提示等于没有提示，用户会以为页面卡住了。
  // 段控那份（aria-pressed / disabled）仍然只在结构变化时刷，不跟着每 5 秒动。
  let budgetMbps = 0;

  function paintBudget(mbps, force) {
    const v0 = Math.max(0, Number(mbps) || 0);
    // 抑制末尾抖动：全程平均每 5 秒都会动一点点（36.0 → 36.1 → 35.9），
    // 把这三位数反复改写在同一个句子里，读者只会觉得这工具在发抖。
    // 变化不到 2% 就不重绘；换档位、开跑、跑完时 force，保证最后留在屏上的
    // 一定是结算值，而不是最后一次抖动前的值。
    if (!force && budgetMbps > 0 && v0 > 0 && Math.abs(v0 - budgetMbps) / budgetMbps < 0.02) return;
    budgetMbps = v0;
    // 时长和预算取「这一跑」的实际参数：跑动中被 activeCfg 锁住的是它，
    // 用页面草稿算会和进度条说的是两套话。
    const c = activeCfg();
    const v = budgetVerdict(v0, c.durationSec, c.budgetMB);
    if (budgetNote.textContent === v.text && budgetNote.dataset.tone === v.tone) return;
    budgetNote.textContent = v.text;
    budgetNote.dataset.tone = v.tone;
  }

  function paintSetup(running) {
    const c = cfg();
    const bw = store.state.bandwidth;
    const p1 = (store.state.results || []).find((r) => r.id === 'P1');
    const measured = (bw && bw.result && bw.result.mbps) || (p1 && p1.mbps) || 0;

    for (const [k, b] of durBtns) b.setAttribute('aria-pressed', String(c.durationSec === k));
    for (const [k, b] of budBtns) b.setAttribute('aria-pressed', String(c.budgetMB === k));
    for (const [k, b] of conBtns) b.setAttribute('aria-pressed', String(c.concurrency === k));
    for (const b of [...durBtns.values(), ...budBtns.values(), ...conBtns.values()]) b.disabled = running;

    paintBudget(measured, true);

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

    const fact = (k, v, tone, alt) => h('div', { class: 'bw-fact' },
      h('span', { class: 'bw-fact-k' }, k),
      h('span', { class: 'bw-fact-v' + (tone ? ' is-' + tone : '') }, v,
        alt ? h('i', { class: 'bw-fact-alt' }, alt) : null),
    );

    resultBox.appendChild(h('div', { class: 'bw-verdict', dataset: { tone: state } },
      h('div', { class: 'bw-verdict-main' },
        h('span', { class: 'bw-tag', dataset: { tone: state } }, r.stalled ? '中途断流' : '跑完了'),
        h('h3', {}, verdict),
        h('p', { class: 'lede' }, r.subline || ''),
      ),
      h('div', { class: 'bw-facts' },
        // 「均值」不是"总字节÷总时长"：长跑口径下它只用有数据的窗口算，
        // 断流的那段时间由「有效时长」单独表达。两个数字各说一件事，
        // 所以标签必须写清楚是哪一种均值，否则用户会拿它去对总下载量除时长。
        fact('窗口均值', fmt.mbps(r.mbps), '', mbsText(r.mbps)),
        fact('P50', fmt.mbps(r.p50Mbps), '', mbsText(r.p50Mbps)),
        fact('P90', fmt.mbps(r.p90Mbps), '', mbsText(r.p90Mbps)),
        fact('峰值', fmt.mbps(r.peakMbps), '', mbsText(r.peakMbps)),
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
