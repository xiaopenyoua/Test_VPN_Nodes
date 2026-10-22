import { h, svg, fmt } from '../core/dom.js';

// 只给「真·长跑」渲染时间线：短测的 5 个采样点画出来是一段噪声，
// 用户会以为那是一条有意义的曲线。阈值与后端长跑口径对齐。
const MIN_SAMPLES = 10;
const MIN_DURATION_MS = 300000;
const VIEW_W = 100;
const VIEW_H = 40;
const PAD_TOP = 4;
const PAD_BOTTOM = 3;

export function isLongRun(result) {
  if (!result || !Array.isArray(result.samples)) return false;
  return result.samples.length > MIN_SAMPLES && (result.durationMs || 0) > MIN_DURATION_MS;
}

// 断流次数按「连续的零吞吐窗口」计数：底部一连串 0 是一次断流，不是十次。
function countStalls(samples) {
  let n = 0;
  let inStall = false;
  for (const s of samples) {
    const dead = !(s.mbps > 0.01);
    if (dead && !inStall) n++;
    inStall = dead;
  }
  return n;
}

// 长跑时间线：极简 SVG 折线 + 断流竖线，手写 path，不引图表库。
// props 进（result）、纯展示、无回调。
export function SustainTimeline(result) {
  if (!isLongRun(result)) return null;

  const pts = result.samples.filter((s) => typeof s.mbps === 'number');
  const span = result.durationMs || pts[pts.length - 1]?.t || 1;
  const maxV = Math.max(...pts.map((p) => p.mbps), 1);
  const y = (v) => VIEW_H - PAD_BOTTOM - (v / maxV) * (VIEW_H - PAD_TOP - PAD_BOTTOM);
  const x = (t) => Math.max(0, Math.min(VIEW_W, (t / span) * VIEW_W));

  const polyline = pts.map((p) => `${x(p.t).toFixed(2)},${y(p.mbps).toFixed(2)}`).join(' ');

  const stalled = !!result.stalled;
  // stallAtMs 为 0 说明后端没定位到时刻（比如整段都没出数据），此时只报次数、不画线
  const stallX = stalled && result.stallAtMs > 0 ? x(result.stallAtMs) : null;
  const stallCount = stalled ? Math.max(1, countStalls(pts)) : 0;
  const sustainPct = Math.round(result.sustainPct || 0);

  const chart = svg('svg', {
    class: 'sustain-svg',
    viewBox: `0 0 ${VIEW_W} ${VIEW_H}`,
    preserveAspectRatio: 'none',
    role: 'img',
    'aria-label': stalled
      ? `长跑吞吐曲线，${fmt.dur(result.stallAtMs)}处断流，有效吞吐占 ${sustainPct}% 时长`
      : `长跑吞吐曲线，有效吞吐占 ${sustainPct}% 时长`,
  },
    svg('line', { class: 'sustain-axis', x1: 0, y1: VIEW_H - 1, x2: VIEW_W, y2: VIEW_H - 1 }),
    svg('polyline', { class: 'sustain-line', points: polyline }),
    stallX !== null ? svg('line', {
      class: 'sustain-stall-line',
      x1: stallX, y1: 0, x2: stallX, y2: VIEW_H,
    }) : null,
  );

  const wrap = h('div', { class: 'sustain' }, h('div', { class: 'sustain-canvas' }, chart));

  if (stallX !== null) {
    // 标注跟着竖线走，但要避开两端：贴边时文字会被卡片裁掉
    const labelLeft = Math.max(12, Math.min(88, stallX));
    wrap.firstChild.appendChild(h('span', {
      class: 'sustain-stall-label',
      style: `left:${labelLeft.toFixed(1)}%`,
    }, `${fmt.dur(result.stallAtMs)} 断流`));
  }

  wrap.appendChild(h('div', { class: 'sustain-note' },
    `有效吞吐占 ${sustainPct}% 时长 · 断流 ${stallCount} 次`,
  ));

  return wrap;
}
