import { h, clear, fmt } from '../core/dom.js';
import { store } from '../core/store.js';
import { LONG } from '../core/probes.js';

const MODES = [
  { key: 'system', label: '系统代理' },
  { key: 'custom', label: '自定义' },
  { key: 'direct', label: '直连对照' },
];

// 档位不是另一个开关，它只是把下面三项一次性填成常用组合。
// 数值与后端 presetDefaults 一致，否则「深度」和「快速」跑出来一模一样。
const PRESETS = [
  { key: 'quick', label: '快速', defaults: { concurrency: 1, budgetMb: 20, durationSec: 8, timeoutSec: 15 } },
  { key: 'standard', label: '标准', defaults: { concurrency: 1, budgetMb: 64, durationSec: 15, timeoutSec: 20 } },
  { key: 'deep', label: '深度', defaults: { concurrency: 1, budgetMb: 160, durationSec: 30, timeoutSec: 25 } },
];

// 折叠行里不能出现明文凭据
function maskUrl(u) {
  return String(u || '').replace(/\/\/[^/@]+@/, '//***@');
}

function proxyValid(v) {
  if (!v.trim()) return { ok: true };
  if (!/^(https?|socks5h?):\/\//i.test(v.trim())) return { ok: false, msg: '需要以 http:// https:// socks5:// socks5h:// 开头' };
  if (!/:\d{1,5}\s*$/.test(v.trim())) return { ok: false, msg: '缺少端口，例如 127.0.0.1:7890' };
  return { ok: true };
}

const NUM_FIELDS = [
  { key: 'timeoutSec', label: '单请求超时', unit: '秒', min: 5, max: 60, desc: '任一 HTTP 请求的超时上限' },
  { key: 'concurrency', label: '并发连接数', unit: '条', min: 1, max: 16, desc: '默认 1 条流，等于你点下载时的真实速度；调高是多流叠加，数字好看但你实际拿不到' },
  { key: 'budgetMb', label: '流量预算', unit: 'MB', min: 4, max: 4096, desc: '一次下载最多消耗多少流量' },
];

// 设置页只放六项共用的整体参数。
// 单次时长与长跑属于 P1，不放这里——只有它跑下载，五个网页探测拿时长没有意义。
export function SettingsPanel() {
  const modeSeg = h('div', { class: 'seg seg-mode', role: 'group', 'aria-label': '代理来源' });
  const modeBtns = new Map();
  for (const m of MODES) {
    const b = h('button', {
      type: 'button',
      onclick: () => patchConfig({ mode: m.key }),
    }, m.label);
    modeBtns.set(m.key, b);
    modeSeg.appendChild(b);
  }

  const urlInput = h('input', {
    class: 'text-input',
    type: 'text',
    spellcheck: 'false',
    autocomplete: 'off',
    placeholder: 'http://127.0.0.1:7890',
    oninput: (e) => {
      const val = e.target.value;
      const cfg = { ...store.state.config, proxyUrl: val };
      // 输入框永不禁用：一旦输入内容即视为自定义代理；清空则回到系统代理语义
      if (val.trim() && cfg.mode !== 'custom') cfg.mode = 'custom';
      if (!val.trim() && cfg.mode === 'custom') cfg.mode = 'system';
      store.patch({ config: cfg });
    },
  });

  const urlDesc = h('span', { class: 'field-desc' }, 'http / https / socks5，例如 http://127.0.0.1:7890');
  const exitLine = h('div', { class: 'field-desc' });

  const presetSeg = h('div', { class: 'seg seg-preset', role: 'group', 'aria-label': '强度预设' });
  const presetBtns = new Map();
  for (const p of PRESETS) {
    const b = h('button', {
      type: 'button',
      title: `填入：超时 ${p.defaults.timeoutSec}s · 并发 ${p.defaults.concurrency} · 预算 ${p.defaults.budgetMb} MB`,
      onclick: () => patchConfig({ preset: p.key, ...p.defaults }),
    }, p.label);
    presetBtns.set(p.key, b);
    presetSeg.appendChild(b);
  }

  const inputs = new Map();
  const numRows = NUM_FIELDS.map((f) => {
    const input = h('input', {
      class: 'num-input',
      type: 'number',
      min: f.min,
      max: f.max,
      inputmode: 'numeric',
      oninput: (e) => {
        const raw = parseInt(e.target.value, 10);
        if (Number.isNaN(raw)) return;
        patchConfig({ [f.key]: Math.max(f.min, Math.min(f.max, raw)) });
      },
      onblur: (e) => {
        const v = parseInt(e.target.value, 10);
        if (Number.isNaN(v)) { e.target.value = store.state.config[f.key]; return; }
        const clamped = Math.max(f.min, Math.min(f.max, v));
        if (clamped !== v) e.target.value = clamped;
      },
    });
    inputs.set(f.key, input);
    return h('div', { class: 'field' },
      h('div', { class: 'field-copy' },
        h('span', { class: 'field-name' }, f.label),
        h('span', { class: 'field-desc' }, f.desc),
      ),
      input,
      h('span', { class: 'unit' }, f.unit),
    );
  });

  const el = h('div', { class: 'view' },
    h('div', { class: 'block' },
      h('h3', {}, '整体设置'),
      h('p', { class: 'hint' }, '这里是六项共用的设置，一次完整探测全按它跑。某一项要另调，去分评页那一行点「这项设置」——只覆盖那一项，不动这里。'),
      h('div', { class: 'list' },
        h('div', { class: 'field' },
          h('div', { class: 'field-copy' },
            h('span', { class: 'field-name' }, '代理来源'),
            h('span', { class: 'field-desc' }, '选系统代理时直接用操作系统的设置，不读下面的地址'),
          ),
          modeSeg,
        ),
        h('div', { class: 'field' },
          h('div', { class: 'field-copy' },
            h('span', { class: 'field-name' }, '代理地址'),
            urlDesc,
          ),
          urlInput,
        ),
      ),
      exitLine,
    ),

    h('div', { class: 'block' },
      h('h3', {}, '运行参数'),
      h('p', { class: 'hint' }, '这三项是六项共用的默认值。某一项要特殊对待，去分评页那一行的「这项设置」里改，改完标「已覆盖」。'),
      h('div', { class: 'list' },
        ...numRows,
        h('div', { class: 'field' },
          h('div', { class: 'field-copy' },
            h('span', { class: 'field-name' }, '强度预设'),
            h('span', { class: 'field-desc' }, '只是把上面三项一次填成常用组合，本身不参与探测'),
          ),
          presetSeg,
        ),
      ),
    ),

    h('div', { class: 'block' },
      h('h3', {}, '评分口径'),
      h('div', { class: 'list' },
        h('div', { class: 'field' },
          h('div', { class: 'field-copy' },
            h('span', { class: 'field-name' }, '速度 40% + 稳定性 30% + AI 可用性 30%'),
            h('span', { class: 'field-desc' }, '可用性是闸门：某一项不可用会直接封顶总分，而不是按权重扣几分。所以不会出现「85 分但 ChatGPT 打不开」。'),
          ),
        ),
      ),
    ),

    h('p', { class: 'foot' },
      `「单次时长」和「长时稳定性」不在这里，因为只有 P1 跑下载。它们属于 P1 自己——去「分评 · P1 · 这项设置」里开启 ${LONG.durationSec / 60} 分钟长跑。`),
  );

  function patchConfig(partial) {
    store.patch({ config: { ...store.state.config, ...partial } });
    render();
  }

  function render() {
    const st = store.state;
    const cfg = st.config;
    const running = st.phase === 'running';

    for (const [k, b] of modeBtns) {
      b.setAttribute('aria-pressed', String(cfg.mode === k));
      b.disabled = running;
    }

    urlInput.disabled = running || cfg.mode === 'direct';
    urlInput.value = document.activeElement === urlInput ? urlInput.value : cfg.proxyUrl;
    urlInput.placeholder = cfg.mode === 'direct'
      ? '直连模式：忽略所有代理设置'
      : cfg.mode === 'custom'
        ? 'socks5h://127.0.0.1:7890  或  http://user:pass@host:8080'
        : '可留空走系统代理；也可直接粘贴代理地址';

    const v = proxyValid(cfg.proxyUrl);
    const showErr = cfg.mode === 'custom' && !v.ok;
    urlInput.classList.toggle('is-error', showErr);
    urlDesc.className = 'field-desc' + (showErr ? ' is-error' : '');
    urlDesc.textContent = showErr ? v.msg : 'http / https / socks5，例如 http://127.0.0.1:7890';

    for (const f of NUM_FIELDS) {
      const input = inputs.get(f.key);
      input.disabled = running;
      if (document.activeElement !== input) input.value = cfg[f.key];
    }

    // 预设高亮只在数值真的等于那套组合时点亮，否则「快速」会替一组自定义数值背书
    for (const p of PRESETS) {
      const same = Object.entries(p.defaults).every(([k, val]) => cfg[k] === val);
      presetBtns.get(p.key).setAttribute('aria-pressed', String(same));
      presetBtns.get(p.key).disabled = running;
    }

    // 出网路径是「结果可解释」的前提，常驻显示
    const exit = st.exit;
    clear(exitLine);
    exitLine.className = 'field-desc';
    if (!exit) {
      exitLine.textContent = cfg.mode === 'custom'
        ? `当前：自定义代理 · ${maskUrl(cfg.proxyUrl) || '未填写地址'}`
        : cfg.mode === 'direct'
          ? '当前：直连对照 · 忽略所有代理设置'
          : '当前：系统代理 · 启动探测后显示实际出网路径';
      return;
    }
    const parts = ['出网路径 · ' + (exit.detail || '未知')];
    for (const w of (exit.warnings || [])) parts.push('注意 · ' + w);
    exitLine.textContent = parts.join('　|　');
    if ((exit.warnings || []).length) exitLine.style.color = 'var(--warn)';
    else exitLine.style.color = '';
  }

  render();
  return { el, render };
}
