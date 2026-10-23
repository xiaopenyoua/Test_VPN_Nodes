// 设计稿交互：只做三件事 —— 切页、切主题、P1 展开长跑。真实实现会用现有的 store 驱动。

const DATA = [
  { id: 'P3', name: 'ChatGPT 网页', tone: 'bad', label: '受限', value: '403 · 0/3 次能打开', detail: 'loc=SG · colo=SIN · warp=on' },
  { id: 'P4', name: 'Gemini 网页', tone: 'ok', label: '可用', value: '200', detail: '首页完整加载 · 1.2s' },
  { id: 'P5', name: 'OpenAI API', tone: 'idle', label: '可达 · 需 Key', value: '401', detail: '401 也代表网络与服务可达 · 0.42s' },
  { id: 'P6', name: '国际链路基线', tone: 'ok', label: '可用', value: '成功率 100%', detail: 'RTT 182 ms · 抖动 26 ms · 12 次采样' },
  { id: 'P2', name: 'YouTube 链路', tone: 'ok', label: '可看', value: '21.4 Mbps', detail: '分片实测 · readyState 4 · 时长 634.6s' },
  { id: 'P1', name: 'Google 大文件下载', tone: 'bad', label: '断流', value: '3.8 Mbps', detail: '第 42 秒起无数据 · 有效吞吐 6%' },
];

const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];

function badge(d) {
  return `<span class="badge" data-tone="${d.tone}">${d.label}</span>`;
}

/* ---------- 总评：逐项证据 ---------- */
$('#evidenceRows').innerHTML = DATA.map((d) => `
  <div class="row">
    <span class="row-id">${d.id}</span>
    <span class="row-name">${d.name}</span>
    ${badge(d)}
    <span class="row-value">${d.value}</span>
    <span class="row-detail">${d.detail}</span>
  </div>`).join('');

/* ---------- 分评：可操作的六行 ---------- */
function probeRow(d) {
  const isP1 = d.id === 'P1';
  return `
  <div class="row row-probe" data-probe="${d.id}">
    <span class="row-id">${d.id}</span>
    <span class="row-name">${d.name}</span>
    ${badge(d)}
    <span class="row-value">${d.value}</span>
    <span class="row-detail">${d.detail}</span>
    <span class="row-act">
      ${isP1 ? `<button class="btn btn-ghost" data-longrun="toggle" aria-pressed="false">长跑 ⌄</button>` : ''}
      <button class="btn btn-ghost">只测这项</button>
      <button class="btn btn-ghost">详情 ›</button>
    </span>
  </div>
  ${isP1 ? `
  <div class="longrun" hidden>
    <div class="lr-controls">
      <span class="lr-field">时长 <b>1800</b> 秒</span>
      <span class="lr-field">预算 <b>1024</b> MB</span>
      <span class="lr-field">并发 <b>1</b> 条</span>
      <button class="btn" data-longrun="start">开始长跑</button>
      <span class="lr-note">单流跑满 30 分钟，专门看它什么时候断</span>
    </div>
    <div class="timeline">
      <div class="tl-plot">
        ${timelineSvg()}
        <span class="tl-mark">42s</span>
        <span class="tl-dead">29 分 18 秒 · 0 字节</span>
      </div>
      <div class="tl-legend">
        <span>上次长跑 · 满 30 分钟</span>
        <span>峰值 6.1 Mbps</span>
        <span>有效吞吐 <b>6%</b></span>
      </div>
    </div>
  </div>` : ''}`;
}

// 30 分钟刻度：42 秒就掉零，剩下 29 分 18 秒全是空白 —— 那空白就是结论，得标出来
function timelineSvg() {
  const W = 900, H = 64, N = 180;          // 每格 10 秒
  const peak = 6.1;
  const pts = [];
  for (let i = 0; i <= N; i++) {
    const sec = i * 10;
    let v = 0;
    if (sec <= 42) {
      // 前 42 秒：起速后缓慢衰减
      v = peak * (0.55 + 0.45 * Math.sin(Math.min(1, sec / 30) * Math.PI / 2)) * (1 - sec / 260);
    }
    const x = (sec / 1800) * W;
    const y = H - (v / peak) * (H - 8);
    pts.push(`${x.toFixed(1)},${y.toFixed(1)}`);
  }
  const stallX = (42 / 1800) * W;
  return `
  <svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" role="img"
       aria-label="30 分钟下载稳定性：第 42 秒后无任何数据">
    <rect x="${stallX}" y="0" width="${W - stallX}" height="${H}" fill="var(--bad-wash)"/>
    <line x1="0" y1="${H - 0.5}" x2="${W}" y2="${H - 0.5}" stroke="var(--separator)" stroke-width="1"
      vector-effect="non-scaling-stroke"/>
    <polyline points="${pts.join(' ')}" fill="none" stroke="var(--bad)" stroke-width="1.6"
      vector-effect="non-scaling-stroke" stroke-linejoin="round"/>
    <line x1="${stallX}" y1="0" x2="${stallX}" y2="${H}" stroke="var(--bad)" stroke-width="1"
      stroke-dasharray="2 3" vector-effect="non-scaling-stroke" opacity=".75"/>
  </svg>`;
}

$('#probeRows').innerHTML = DATA.map(probeRow).join('');

$$('[data-longrun="toggle"]').forEach((b) => {
  b.addEventListener('click', () => {
    const panel = $('.longrun');
    panel.hidden = !panel.hidden;
    b.setAttribute('aria-pressed', String(!panel.hidden));
    b.textContent = panel.hidden ? '长跑 ⌄' : '收起 ⌃';
  });
});

/* ---------- 切页 ---------- */
const COPY = {
  verdict: ['总评', '这套代理整体能不能用'],
  probes: ['分评', '六项真实探测各自的结果'],
  settings: ['设置', '一次完整探测的时长、流量与并发'],
};

function go(tab) {
  $$('.nav-item').forEach((b) => b.classList.toggle('is-current', b.dataset.tab === tab));
  $$('.view').forEach((v) => { v.hidden = v.id !== 'view-' + tab; });
  $('#pageTitle').textContent = COPY[tab][0];
  $('#pageSub').textContent = COPY[tab][1];
}
$$('.nav-item').forEach((b) => b.addEventListener('click', () => go(b.dataset.tab)));
go('verdict');

/* ---------- 主题三态 ---------- */
const mq = window.matchMedia('(prefers-color-scheme: dark)');
function applyTheme(pref) {
  const dark = pref === 'dark' || (pref === 'auto' && mq.matches);
  document.documentElement.dataset.theme = dark ? 'dark' : 'light';
  $$('#themeSeg button').forEach((b) => b.setAttribute('aria-pressed', String(b.dataset.themeOpt === pref)));
}
$$('#themeSeg button').forEach((b) => b.addEventListener('click', () => applyTheme(b.dataset.themeOpt)));
applyTheme('dark');

/* ---------- 代理来源：系统代理 ↔ 自定义地址 ---------- */
$$('#modeSeg button').forEach((b) => b.addEventListener('click', () => {
  const custom = b.textContent.trim() === '自定义地址';
  $$('#modeSeg button').forEach((x) => x.setAttribute('aria-pressed', String(x === b)));
  const url = $('#proxyUrl');
  url.disabled = !custom;
  if (custom) url.focus();
}));
