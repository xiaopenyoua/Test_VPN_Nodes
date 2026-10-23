// 纯 Canvas 折线图：速度曲线 / 迷你 sparkline 共用。
export function drawSpeed(canvas, samples, opts = {}) {
  const dpr = window.devicePixelRatio || 1;
  const cssW = canvas.clientWidth || 320;
  const cssH = canvas.clientHeight || 140;
  canvas.width = cssW * dpr;
  canvas.height = cssH * dpr;
  const ctx = canvas.getContext('2d');
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, cssW, cssH);

  const pts = (samples || []).filter((s) => typeof s.mbps === 'number');
  const cs = getComputedStyle(document.documentElement);
  const grid = cs.getPropertyValue('--line').trim() || 'rgba(146,64,106,.13)';
  const line = opts.color || cs.getPropertyValue('--pink').trim() || '#db2777';
  const text = cs.getPropertyValue('--ink-3').trim() || '#9c4f70';

  if (pts.length < 2) {
    ctx.fillStyle = text;
    ctx.font = '12px ui-rounded, "PingFang SC", system-ui, sans-serif';
    ctx.fillText(opts.empty || '等待采样…', 8, cssH / 2);
    return;
  }

  const padL = 4, padR = 4, padT = 8, padB = opts.labels ? 16 : 4;
  const maxV = Math.max(...pts.map((p) => p.mbps), opts.minMax || 1);
  const maxT = pts[pts.length - 1].t || 1;

  const x = (t) => padL + (t / maxT) * (cssW - padL - padR);
  const y = (v) => cssH - padB - (v / maxV) * (cssH - padT - padB);

  // 网格：三条水平参考线，不画竖线
  ctx.strokeStyle = grid;
  ctx.lineWidth = 1;
  for (let i = 0; i <= 3; i++) {
    const yy = padT + ((cssH - padT - padB) / 3) * i;
    ctx.beginPath();
    ctx.moveTo(padL, yy);
    ctx.lineTo(cssW - padR, yy);
    ctx.stroke();
  }

  // 面积：一层很淡的平涂，不用渐变
  ctx.beginPath();
  ctx.moveTo(x(pts[0].t), cssH - padB);
  for (const p of pts) ctx.lineTo(x(p.t), y(p.mbps));
  ctx.lineTo(x(pts[pts.length - 1].t), cssH - padB);
  ctx.closePath();
  ctx.fillStyle = hexA(line, 0.1);
  ctx.fill();

  // 折线
  ctx.beginPath();
  pts.forEach((p, i) => (i === 0 ? ctx.moveTo(x(p.t), y(p.mbps)) : ctx.lineTo(x(p.t), y(p.mbps))));
  ctx.strokeStyle = line;
  ctx.lineWidth = 1.8;
  ctx.lineJoin = 'round';
  ctx.stroke();

  if (opts.labels) {
    ctx.fillStyle = text;
    ctx.font = '12px ui-rounded, "PingFang SC", system-ui, sans-serif';
    ctx.fillText(`${maxV.toFixed(0)} Mbps`, padL, 10);
    ctx.fillText('0', padL, cssH - 3);
  }
}

function hexA(hex, alpha) {
  const h = hex.replace('#', '');
  const v = h.length === 3 ? h.split('').map((c) => c + c).join('') : h;
  const r = parseInt(v.slice(0, 2), 16);
  const g = parseInt(v.slice(2, 4), 16);
  const b = parseInt(v.slice(4, 6), 16);
  return `rgba(${r},${g},${b},${alpha})`;
}
