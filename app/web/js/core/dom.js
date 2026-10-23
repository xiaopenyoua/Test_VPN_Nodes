// 极简 DOM 构建工具：无框架、无构建链，组件全部是返回 DOM 的函数。
export function h(tag, attrs = {}, ...children) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === null || v === undefined || v === false) continue;
    if (k === 'class') el.className = v;
    else if (k === 'dataset') Object.assign(el.dataset, v);
    else if (k === 'style' && typeof v === 'object') Object.assign(el.style, v);
    else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2).toLowerCase(), v);
    else if (k === 'html') el.innerHTML = v;
    else el.setAttribute(k, v === true ? '' : String(v));
  }
  append(el, children);
  return el;
}

export function svg(tag, attrs = {}, ...children) {
  const el = document.createElementNS('http://www.w3.org/2000/svg', tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === null || v === undefined) continue;
    el.setAttribute(k, String(v));
  }
  append(el, children);
  return el;
}

function append(el, children) {
  for (const c of children.flat(3)) {
    if (c === null || c === undefined || c === false) continue;
    el.appendChild(c instanceof Node ? c : document.createTextNode(String(c)));
  }
}

export function clear(node) {
  while (node.firstChild) node.removeChild(node.firstChild);
  return node;
}

// 缺失值统一印一根横杠，不印 0：0 会被读成"测出来很差"
export const NONE = '—';

export const fmt = {
  mbps: (v) => (v > 0 ? `${v.toFixed(v >= 100 ? 0 : 1)} Mbps` : NONE),
  ms: (v) => (v > 0 ? `${Math.round(v)} ms` : NONE),
  pct: (v) => `${Math.round(v)}%`,
  bytes: (v) => {
    if (!v) return '0 B';
    const u = ['B', 'KB', 'MB', 'GB'];
    let i = 0, n = v;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return `${n.toFixed(n >= 10 || i === 0 ? 0 : 1)} ${u[i]}`;
  },
  clock: (ms) => {
    const s = Math.floor(ms / 1000);
    return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
  },
  // 长跑场景说的是人话时长（"3分12秒"），clock 的 00:03:12 是仪表盘语言，读起来要先换算
  dur: (ms) => {
    const total = Math.max(0, Math.floor((ms || 0) / 1000));
    const m = Math.floor(total / 60);
    const s = total % 60;
    if (!m) return `${s}秒`;
    if (!s) return `${m}分`;
    return `${m}分${s}秒`;
  },
  // 流量预估按 MB 报数：超过 1 GB 才进位，避免"1024 MB"和"1 GB"两套说法在界面上打架
  traffic: (mb) => (mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${Math.round(mb)} MB`),
};

export function debounce(fn, wait = 200) {
  let t;
  return (...args) => {
    clearTimeout(t);
    t = setTimeout(() => fn(...args), wait);
  };
}
