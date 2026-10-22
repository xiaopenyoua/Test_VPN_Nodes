import { h, svg } from '../core/dom.js';

// 环形评分：纯内联 SVG，无图表库。
// score 传 null 表示"尚无分数"（未开始 / 探测中）。
// 关键：没有分数时不能画长度 0 的描边——stroke-linecap:round 会在 12 点方向留下一个孤立圆点，
// 看起来像"故障指示"而不是"尚未开始"。
export function ScoreRing(score, grade, state) {
  const R = 58;
  const C = 2 * Math.PI * R;
  const hasScore = score !== null && score !== undefined && !Number.isNaN(score);
  const running = state === 'running';
  const idle = !hasScore && !running;

  const pct = running ? 0.25 : (hasScore ? Math.max(0, Math.min(100, score)) / 100 : 0);
  const color = running ? 'var(--brand)'
    : idle ? 'var(--text-3)'
      : state === 'blocked' ? 'var(--danger)'
        : state === 'limited' ? 'var(--warn)' : 'var(--ok)';

  const cls = ['ring', running ? 'is-running' : '', idle ? 'is-idle' : ''].filter(Boolean).join(' ');

  return h('div', { class: cls },
    svg('svg', { width: 132, height: 132, viewBox: '0 0 132 132' },
      svg('circle', { cx: 66, cy: 66, r: R, fill: 'none', stroke: 'var(--border)', 'stroke-width': 8 }),
      // 未测态只留轨道，不画弧
      idle ? null : svg('circle', {
        cx: 66, cy: 66, r: R, fill: 'none', stroke: color, 'stroke-width': 8,
        'stroke-linecap': 'round',
        'stroke-dasharray': `${C * pct} ${C}`,
        style: running ? '' : 'transition: stroke-dasharray 600ms var(--ease)',
      }),
    ),
    h('div', { class: 'ring-value' },
      h('div', { style: 'text-align:center' },
        h('div', { class: 'ring-num', style: `color:${color}` }, hasScore ? String(Math.round(score)) : (running ? '' : '未测')),
        h('div', { class: 'ring-grade' }, grade || '等待检测'),
      ),
    ),
  );
}
