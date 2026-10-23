// 带宽长跑专用曲线。
//
// 与 SpeedChart 分开而不是加参数：这两张图回答的问题不同。
// SpeedChart 是"刚才那一小段速度抖不抖"（20 秒内的迷你折线）；
// 这张要回答"两个小时里它有没有掉下来过"——需要坐标刻度、断流定位、
// 以及跑得越久越压缩的时间轴。硬塞进一个函数只会让两边都变得难读。

import { h } from '../core/dom.js';

const FONT = '11.5px ui-rounded, "PingFang SC", system-ui, sans-serif';

// 把 Mbps 轴抬到一个"整"的刻度上：82.7 的峰值给 100 的轴，
// 直接按峰值画会让曲线长期贴着顶端，看不出还有多少余量。
function niceMax(v) {
  if (!(v > 0)) return 10;
  const pow = Math.pow(10, Math.floor(Math.log10(v)));
  for (const m of [1, 1.5, 2, 2.5, 3, 4, 5, 6, 8, 10]) {
    if (v <= m * pow) return m * pow;
  }
  return 10 * pow;
}

// 刻度标签必须是"能整除的整数"，不是"上限除以格数"。
//
// 峰值 33.7 时，如果只把上限抬到 40 再三等分，标签就变成 0 / 13 / 27——
// 读起来像随手写的数字，而不是一根量出来的坐标轴。
// 所以先反推一个干净的步长（1 / 2 / 2.5 / 5 × 10ⁿ），再把上限抬到步长的整数倍。
// 从大到小试，取第一个能给出至少 3 格的步长：宁可 3 格整数，不要 5 格小数。
function niceAxis(peak) {
  const p = peak > 0 ? peak * 1.06 : 10; // 顶端留一点余量，曲线不贴着上边框
  const pow = Math.pow(10, Math.floor(Math.log10(p)));
  const steps = [];
  for (const scale of [pow / 10, pow, pow * 10, pow * 100]) {
    for (const m of [1, 2, 2.5, 5]) steps.push(m * scale);
  }
  steps.sort((a, b) => b - a);
  for (const step of steps) {
    const n = Math.ceil(p / step);
    if (n >= 3) return { top: step * n, step };
  }
  const fallbackTop = niceMax(p);
  return { top: fallbackTop, step: fallbackTop / 4 };
}

function tickText(v) {
  if (v === 0) return '0';
  return v >= 10 ? String(Math.round(v)) : v.toFixed(1);
}

function fmtTick(ms) {
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  const r = s % 60;
  if (m < 60) return r ? `${m}m${String(r).padStart(2, '0')}s` : `${m}m`;
  const hr = Math.floor(m / 60);
  return `${hr}h${String(m % 60).padStart(2, '0')}m`;
}

// 时间轴的档位阶梯。
//
// 跑动中的横轴不能直接用"配置时长"：4 小时的长跑，前 10 分钟会全部挤在
// 最左边 4% 的一条缝里，看起来像一条竖线——用户会以为图坏了。
// 也不能每帧都按"已跑时长"自适应：轴标签会一直跳，读不出趋势。
// 折中是阶梯：跨过一档才跳一次，档内曲线自然增长。
const LADDER = [30000, 60000, 120000, 180000, 300000, 600000, 900000,
  1800000, 3600000, 7200000, 10800000, 14400000];

function domainMs(elapsed, running, configured) {
  if (!running) return Math.max(elapsed, 10000);
  const target = Math.max(elapsed * 1.12, 20000);
  for (const d of LADDER) if (d >= target) return d;
  return Math.max(configured, LADDER[LADDER.length - 1]);
}

function hexA(hex, alpha) {
  const h = String(hex).replace('#', '');
  const v = h.length === 3 ? h.split('').map((c) => c + c).join('') : h;
  return `rgba(${parseInt(v.slice(0, 2), 16)},${parseInt(v.slice(2, 4), 16)},${parseInt(v.slice(4, 6), 16)},${alpha})`;
}

export function createBandwidthChart() {
  const canvas = h('canvas', { class: 'bw-canvas', role: 'img', 'aria-label': '实时下载速率曲线' });
  const empty = h('div', { class: 'bw-empty' }, '点「开始长跑」后，这里会实时画出速率曲线');
  const wrap = h('div', { class: 'bw-chart' }, canvas, empty);

  let pts = [];
  let opts = { totalMs: 0, running: false };
  let raf = 0;

  function schedule() {
    if (raf) return;
    raf = requestAnimationFrame(() => {
      raf = 0;
      paint();
    });
  }

  function paint() {
    const dpr = window.devicePixelRatio || 1;
    const cssW = canvas.clientWidth || wrap.clientWidth || 600;
    const cssH = canvas.clientHeight || 220;
    if (!cssW || !cssH) return;
    canvas.width = Math.round(cssW * dpr);
    canvas.height = Math.round(cssH * dpr);
    const ctx = canvas.getContext('2d');
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, cssW, cssH);

    const cs = getComputedStyle(document.documentElement);
    const line = cs.getPropertyValue('--line').trim() || 'rgba(146,64,106,.13)';
    const ink3 = cs.getPropertyValue('--ink-3').trim() || '#9c4f70';
    const pink = cs.getPropertyValue('--pink').trim() || '#db2777';
    const bad = cs.getPropertyValue('--bad').trim() || '#a32a2a';
    const good = cs.getPropertyValue('--good').trim() || '#0b7a52';

    const live = pts.length > 0;
    empty.hidden = live;

    // 时间轴右端：跑动中按阶梯增长，结束后用实际跑完的时长。
    // 用"配置时长"会让 4 小时长跑的前 10 分钟挤在最左边一条缝里；
    // 用"已跑时长"会让轴标签每帧都跳。阶梯是这两者之间的折中。
    const elapsed = live ? pts[pts.length - 1].t : 0;
    const totalMs = domainMs(elapsed, !!opts.running && live, opts.totalMs || 0);

    // 留白：右边给 y 轴刻度文字，下边给时间标签
    const padL = 6;
    const padR = 52;
    const padT = 12;
    const padB = 20;
    const plotW = Math.max(10, cssW - padL - padR);
    const plotH = Math.max(10, cssH - padT - padB);

    const peak = live ? Math.max(...pts.map((p) => p.mbps), 0) : 0;
    const axis = niceAxis(peak);
    const top = axis.top;
    const x = (t) => padL + Math.min(1, Math.max(0, t / totalMs)) * plotW;
    const y = (v) => padT + plotH - Math.min(1, Math.max(0, v / top)) * plotH;

    // ── 网格与 y 轴刻度 ──
    ctx.font = FONT;
    ctx.textBaseline = 'middle';
    ctx.textAlign = 'left';
    const ticks = Math.max(1, Math.round(top / axis.step));
    for (let i = 0; i <= ticks; i++) {
      const v = axis.step * i;
      const yy = y(v);
      ctx.strokeStyle = line;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(padL, yy);
      ctx.lineTo(padL + plotW, yy);
      ctx.stroke();
      if (live) {
        ctx.fillStyle = ink3;
        ctx.fillText(tickText(v), padL + plotW + 8, yy);
      }
    }
    if (live) {
      ctx.fillStyle = ink3;
      ctx.fillText('Mbps', padL + plotW + 8, padT - 2);
    }

    // ── x 轴时间标签 ──
    ctx.textAlign = 'center';
    ctx.textBaseline = 'top';
    for (let i = 0; i <= 4; i++) {
      const t = (totalMs / 4) * i;
      const xx = padL + (plotW / 4) * i;
      ctx.fillStyle = ink3;
      ctx.fillText(fmtTick(t), Math.min(cssW - 16, Math.max(16, xx)), padT + plotH + 6);
    }

    if (!live) return;

    // ── 面积 ──
    ctx.beginPath();
    ctx.moveTo(x(pts[0].t), padT + plotH);
    for (const p of pts) ctx.lineTo(x(p.t), y(p.mbps));
    ctx.lineTo(x(pts[pts.length - 1].t), padT + plotH);
    ctx.closePath();
    ctx.fillStyle = hexA(pink, 0.12);
    ctx.fill();

    // ── 断流段：零吞吐单独染红 ──
    // 这是整张图最重要的信息。用同一条粉线画过去，两个小时的 0 只会
    // 变成一段贴着底边的直线，和"速度很慢"长得一模一样。
    let runStart = -1;
    const flushZero = (endIdx) => {
      if (runStart < 0) return;
      const a = pts[runStart];
      const b = pts[endIdx];
      ctx.beginPath();
      ctx.moveTo(x(a.t), padT + plotH);
      ctx.lineTo(x(b.t), padT + plotH);
      ctx.strokeStyle = bad;
      ctx.lineWidth = 3;
      ctx.lineCap = 'round';
      ctx.stroke();
      runStart = -1;
    };
    for (let i = 0; i < pts.length; i++) {
      const zero = pts[i].mbps <= 0.05;
      if (zero && runStart < 0) runStart = i;
      if (!zero && runStart >= 0) flushZero(i - 1);
    }
    if (runStart >= 0) flushZero(pts.length - 1);

    // ── 折线 ──
    ctx.beginPath();
    pts.forEach((p, i) => (i === 0 ? ctx.moveTo(x(p.t), y(p.mbps)) : ctx.lineTo(x(p.t), y(p.mbps))));
    ctx.strokeStyle = pink;
    ctx.lineWidth = 1.8;
    ctx.lineJoin = 'round';
    ctx.lineCap = 'round';
    ctx.stroke();

    // ── 当前游标 ──
    // 跑动中这一笔让"实时"看得见：曲线末端在动，数字也在跳。
    if (opts.running) {
      const last = pts[pts.length - 1];
      const lx = x(last.t);
      const ly = y(last.mbps);
      ctx.beginPath();
      ctx.moveTo(lx, padT);
      ctx.lineTo(lx, padT + plotH);
      ctx.strokeStyle = hexA(pink, 0.28);
      ctx.lineWidth = 1;
      ctx.stroke();
      ctx.beginPath();
      ctx.arc(lx, ly, 3.4, 0, Math.PI * 2);
      ctx.fillStyle = pink;
      ctx.fill();
      ctx.beginPath();
      ctx.arc(lx, ly, 7, 0, Math.PI * 2);
      ctx.fillStyle = hexA(pink, 0.18);
      ctx.fill();
    }

    // ── 峰值标注 ──
    if (peak > 0) {
      let best = pts[0];
      for (const p of pts) if (p.mbps > best.mbps) best = p;
      const bx = x(best.t);
      const by = y(best.mbps);
      ctx.strokeStyle = hexA(good, 0.5);
      ctx.lineWidth = 1;
      ctx.setLineDash([3, 3]);
      ctx.beginPath();
      ctx.moveTo(padL, by);
      ctx.lineTo(padL + plotW, by);
      ctx.stroke();
      ctx.setLineDash([]);
      ctx.beginPath();
      ctx.arc(bx, by, 2.6, 0, Math.PI * 2);
      ctx.fillStyle = good;
      ctx.fill();
    }
  }

  function setData(next, nextOpts) {
    pts = next || [];
    if (nextOpts) opts = { ...opts, ...nextOpts };
    schedule();
  }

  // 容器尺寸变了要重画：canvas 的位图尺寸绑定在绘制那一刻的 CSS 尺寸上，
  // 不重画的话窗口一缩放，曲线就会被拉伸成糊的。
  if (typeof ResizeObserver !== 'undefined') {
    const ro = new ResizeObserver(() => schedule());
    ro.observe(wrap);
  }

  return { el: wrap, setData, repaint: schedule };
}
