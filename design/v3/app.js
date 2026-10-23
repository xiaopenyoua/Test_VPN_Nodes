/* ProxyLens v3 · 粘土拟态交互稿
   切页 / 主题 / 单项设置面板
   ——「整体设置」是六项共用默认值，「这项设置」只覆盖当前项 */
(() => {
  'use strict';

  /* ---------------- 数据 ---------------- */

  // 设置页那三项：六项共用的默认值
  const GLOBAL = { timeoutSec: 15, concurrency: 1, budgetMb: 20 };

  const FIELDS = {
    timeoutSec:  { label: '单请求超时', unit: '秒', min: 5, max: 60 },
    durationSec: { label: '单次时长',   unit: '秒', min: 5, max: 1800 },
    budgetMb:    { label: '流量预算',   unit: 'MB', min: 1, max: 1024 },
    concurrency: { label: '并发连接',   unit: '条', min: 1, max: 5 },
  };
  const LONG_RUN = { durationSec: 1800, budgetMb: 1024, concurrency: 1 };

  // own：有的键 = 这一项自己覆盖；没有 = 跟随整体设置
  const DATA = [
    { id:'P3', name:'ChatGPT 网页', tone:'bad', label:'受限', value:'403 · 0/3 次能打开',
      detail:'loc=SG · colo=SIN · warp=on', own:{}, fields:['timeoutSec'] },

    { id:'P4', name:'Gemini 网页', tone:'ok', label:'可用', value:'200',
      detail:'首页完整加载 · 1.2s', own:{}, fields:['timeoutSec'] },

    { id:'P5', name:'OpenAI API', tone:'idle', label:'可达 · 需 Key', value:'401',
      detail:'401 也代表网络与服务可达 · 0.42s', own:{}, fields:['timeoutSec'] },

    { id:'P6', name:'国际链路基线', tone:'ok', label:'可用', value:'成功率 100%',
      detail:'RTT 182 ms · 抖动 26 ms · 12 次采样', own:{}, fields:['timeoutSec'] },

    { id:'P2', name:'YouTube 链路', tone:'ok', label:'可看', value:'21.4 Mbps',
      detail:'分片实测 · readyState 4 · 时长 634.6s', own:{},
      fields:['timeoutSec','concurrency'] },

    { id:'P1', name:'Google 大文件下载', tone:'bad', label:'断流', value:'3.8 Mbps',
      detail:'第 42 秒起无数据 · 有效吞吐 6%',
      own:{ durationSec: 8, longRun: false },
      fields:['timeoutSec','durationSec','budgetMb','concurrency'], long:true },
  ];

  const PEBBLE_ORDER = ['P3','P4','P5','P6','P2','P1'];
  const TABS = {
    verdict:{ title:'总评', sub:'这套代理整体能不能用' },
    probes:{ title:'分评', sub:'六项逐一实测，可单跑' },
    settings:{ title:'设置', sub:'六项共用的整体参数' },
  };

  const $  = (s, r = document) => r.querySelector(s);
  const $$ = (s, r = document) => [...r.querySelectorAll(s)];
  const valOf = (p, k) => (p.own[k] !== undefined ? p.own[k] : GLOBAL[k]);

  /* ---------------- 主题 ---------------- */
  const themeSeg = $('#themeSeg');
  const mq = window.matchMedia('(prefers-color-scheme: dark)');
  let themeChoice = 'light';

  function applyTheme() {
    const dark = themeChoice === 'dark' || (themeChoice === 'auto' && mq.matches);
    document.documentElement.dataset.theme = dark ? 'dark' : 'light';
    $$('button', themeSeg).forEach((b) =>
      b.setAttribute('aria-pressed', String(b.dataset.themeOpt === themeChoice)));
  }

  themeSeg.addEventListener('click', (e) => {
    const b = e.target.closest('button[data-theme-opt]');
    if (!b) return;
    themeChoice = b.dataset.themeOpt;
    applyTheme();
  });
  mq.addEventListener('change', () => { if (themeChoice === 'auto') applyTheme(); });

  /* ---------------- 切页 ---------------- */
  const navBtns = $$('.nav button');
  const views = {
    verdict:  $('#view-verdict'),
    probes:   $('#view-probes'),
    settings: $('#view-settings'),
  };

  function goTab(tab) {
    Object.entries(views).forEach(([k, el]) => { el.hidden = k !== tab; });
    navBtns.forEach((b) => {
      const on = b.dataset.tab === tab;
      b.classList.toggle('is-current', on);
      b.setAttribute('aria-current', on ? 'page' : 'false');
    });
    $('#pageTitle').textContent = TABS[tab].title;
    $('#pageSub').textContent = TABS[tab].sub;
    $('#exportBtn').style.display = tab === 'settings' ? 'none' : '';
    $('.slab').scrollTop = 0;
  }

  navBtns.forEach((b) => b.addEventListener('click', () => goTab(b.dataset.tab)));
  $('#toSettings').addEventListener('click', () => goTab('settings'));

  /* ---------------- 行渲染 ---------------- */
  const tagHTML = (p) => `<span class="tag tag-${p.tone}">${p.label}</span>`;

  // 总评页的逐项证据：只读，不带操作
  function renderEvidence() {
    $('#evidenceRows').innerHTML = DATA.map((p) => `
      <div class="row">
        <span class="row-id">${p.id}</span>
        <span class="row-name">${p.name}</span>
        ${tagHTML(p)}
        <span class="row-value">${p.value}</span>
        <span class="row-detail">${p.detail}</span>
      </div>`).join('');
  }

  // 分评页：每行两个操作 —— 单独测 / 这项设置
  function renderProbes() {
    $('#probeRows').innerHTML = DATA.map((p) => `
      <div class="row row-probe" data-row="${p.id}">
        <span class="row-id">${p.id}</span>
        <span class="row-name">${p.name}</span>
        ${tagHTML(p)}
        <span class="row-value">${p.value}<small>${p.detail}</small></span>
        <span class="row-act">
          <button class="btn-sm" data-run="${p.id}">单独测</button>
          <button class="btn-sm" data-setup="${p.id}" aria-pressed="false">这项设置</button>
        </span>
      </div>
      <div class="pset" data-pset="${p.id}" hidden></div>`).join('');

    DATA.forEach(paint);
  }

  /* ---------------- 单项设置面板 ---------------- */

  // 一个字段：值 + 单位 + 来源标记（跟随整体 / 已覆盖）
  // 一个字段：标签行（含来源标记）+ 控件行
  function fieldHTML(p, k) {
    const f = FIELDS[k];
    const own = p.own[k] !== undefined;
    const locked = p.long && p.own.longRun && k !== 'timeoutSec';
    return `
      <label class="pf">
        <span class="pf-label">${f.label}<span class="pf-flag${own ? ' is-own' : ''}">${own ? '已覆盖' : '跟随整体'}</span></span>
        <span class="pf-ctl">
          <input class="num-input" type="number" data-k="${k}" value="${valOf(p, k)}"
                 min="${f.min}" max="${f.max}"${locked ? ' disabled' : ''} />
          <span class="unit">${f.unit}</span>
        </span>
      </label>`;
  }

  // P1 专属：长时稳定性开关（独占整行）
  function longHTML(p) {
    const on = !!p.own.longRun;
    return `
      <div class="pf pf-wide">
        <span class="pf-label">长时稳定性<span class="pf-flag${on ? ' is-own' : ''}">${on ? '已开启' : '未开启'}</span></span>
        <span class="pf-ctl">
          <button class="btn-sm" data-long="1" aria-pressed="${on}">
            ${on ? '30 分钟 · 已开' : '开 30 分钟'}
          </button>
          <span class="pf-state">${on
            ? '已锁 1800 秒 / 1024 MB / 单流，关掉才能改上面三项'
            : '规范口径：1800 秒 / 1024 MB / 强制单流'}</span>
        </span>
      </div>`;
  }

  function panelHTML(p) {
    const fields = p.fields.map((k) => fieldHTML(p, k)).join('');
    return `
      <div class="pset-head">
        <b>${p.id} 这项设置</b>
        <span class="pf-state">只影响 ${p.id}，不动整体设置</span>
        <button class="link-btn" data-reset="1">恢复默认</button>
      </div>
      <div class="pset-fields">${fields}${p.long ? longHTML(p) : ''}</div>`;
  }

  function paint(p) {
    const panel = $(`[data-pset="${p.id}"]`);
    panel.innerHTML = panelHTML(p);

    // 改数字：写进 own（就变成「已覆盖」），越界就地夹紧
    $$('input[data-k]', panel).forEach((inp) => {
      inp.addEventListener('change', () => {
        const k = inp.dataset.k;
        const f = FIELDS[k];
        let v = Number(inp.value);
        if (!Number.isFinite(v)) v = valOf(p, k);
        p.own[k] = Math.min(f.max, Math.max(f.min, Math.round(v)));
        paint(p);
      });
    });

    const longBtn = $('[data-long]', panel);
    if (longBtn) {
      longBtn.addEventListener('click', () => {
        if (p.own.longRun) {
          // 关掉：回到短跑默认，同时释放 budget / concurrency 的覆盖
          delete p.own.longRun;
          delete p.own.budgetMb;
          delete p.own.concurrency;
          p.own.durationSec = 8;
        } else {
          p.own.longRun = true;
          Object.assign(p.own, LONG_RUN);
        }
        paint(p);
      });
    }

    $('[data-reset]', panel).addEventListener('click', () => {
      p.own = p.long ? { durationSec: 8, longRun: false } : {};
      paint(p);
    });
  }

  function openPanel(id, open) {
    $(`[data-pset="${id}"]`).hidden = !open;
    $(`[data-setup="${id}"]`).setAttribute('aria-pressed', String(open));
  }

  $('#probeRows').addEventListener('click', (e) => {
    // 「这项设置」：一次只开一个，再点收起
    const setup = e.target.closest('[data-setup]');
    if (setup) {
      const id = setup.dataset.setup;
      const willOpen = $(`[data-pset="${id}"]`).hidden;
      DATA.forEach((p) => openPanel(p.id, false));
      if (willOpen) openPanel(id, true);
      return;
    }

    // 「单独测」：只跑这一项，其它五项结果保留
    const run = e.target.closest('[data-run]');
    if (!run) return;
    const id = run.dataset.run;
    const row = $(`[data-row="${id}"]`);
    if (row.dataset.running) return;

    row.dataset.running = '1';
    const tag = $('.tag', row);
    const val = $('.row-value', row);
    const keepTag = tag.outerHTML;
    const keepVal = val.innerHTML;
    tag.className = 'tag tag-idle';
    tag.textContent = '测试中';
    val.textContent = '正在跑这一项…';
    run.disabled = true;

    setTimeout(() => {
      tag.outerHTML = keepTag;
      val.innerHTML = keepVal;
      run.disabled = false;
      delete row.dataset.running;
    }, 1600);
  });

  /* ---------------- 卵石 → 分评对应行 ---------------- */
  $$('.pebble').forEach((peb, i) => {
    peb.addEventListener('click', () => {
      const id = PEBBLE_ORDER[i];
      goTab('probes');
      openPanel(id, true);
      $(`[data-row="${id}"]`).scrollIntoView({ block: 'center', behavior: 'smooth' });
    });
  });

  /* ---------------- 代理来源 ---------------- */
  const modeSeg = $('#modeSeg');
  const proxyUrl = $('#proxyUrl');
  const stripMain = $('#toSettings').firstElementChild;

  function syncMode() {
    const custom = $$('button', modeSeg)[1].getAttribute('aria-pressed') === 'true';
    proxyUrl.disabled = !custom;
    stripMain.textContent = custom
      ? '整体设置 · 自定义地址 · 快速档 · 六项一起跑'
      : '整体设置 · 系统代理 · 快速档 · 六项一起跑';
  }

  modeSeg.addEventListener('click', (e) => {
    const b = e.target.closest('button');
    if (!b) return;
    $$('button', modeSeg).forEach((x) => x.setAttribute('aria-pressed', String(x === b)));
    syncMode();
  });

  /* ---------------- 导出（演示态，不真的落盘） ---------------- */
  const exportBtn = $('#exportBtn');
  exportBtn.addEventListener('click', () => {
    const keep = exportBtn.textContent;
    exportBtn.textContent = '已生成';
    setTimeout(() => { exportBtn.textContent = keep; }, 1500);
  });

  /* ---------------- 启动 ---------------- */
  renderEvidence();
  renderProbes();
  applyTheme();
  syncMode();
  goTab('verdict');
})();
