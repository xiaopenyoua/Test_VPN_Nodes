import { h, clear } from '../core/dom.js';
import { store } from '../core/store.js';

const MODES = [
  { key: 'system', label: '系统代理' },
  { key: 'custom', label: '自定义代理' },
  { key: 'direct', label: '直连对照' },
];

const PRESETS = [
  { key: 'quick', label: '快速' },
  { key: 'standard', label: '标准' },
  { key: 'deep', label: '深度' },
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

export function ControlPanel({ onRun, onCancel }) {
  let segMode, segPreset, input, hint, runBtn, note, summaryEl, fieldsEl;
  // 结果出来后配置卡退回一行（P0-10）。用户点「修改配置」可临时展开，
  // 一旦再次运行或回到初始态就复位，避免状态残留。
  let userExpanded = false;
  let lastPhase = '';

  const modeSeg = h('div', { class: 'segmented', role: 'group', 'aria-label': '出口模式' });
  segMode = MODES.map((m) => {
    const b = h('button', {
      type: 'button',
      'aria-pressed': String(store.state.config.mode === m.key),
      onclick: () => {
        store.patch({ config: { ...store.state.config, mode: m.key } });
        render();
      },
    }, m.label);
    modeSeg.appendChild(b);
    return { key: m.key, el: b };
  });

  const presetSeg = h('div', { class: 'segmented', role: 'group', 'aria-label': '探测强度' });
  segPreset = PRESETS.map((p) => {
    const b = h('button', {
      type: 'button',
      'aria-pressed': String(store.state.config.preset === p.key),
      onclick: () => {
        store.patch({ config: { ...store.state.config, preset: p.key } });
        render();
      },
    }, p.label);
    presetSeg.appendChild(b);
    return { key: p.key, el: b };
  });

  input = h('input', {
    class: 'input',
    type: 'text',
    spellcheck: 'false',
    autocomplete: 'off',
    placeholder: 'socks5h://127.0.0.1:7890  或  http://user:pass@host:8080',
    value: store.state.config.proxyUrl,
    oninput: (e) => {
      const val = e.target.value;
      const cfg = { ...store.state.config, proxyUrl: val };
      // 输入框永不禁用：一旦输入内容即视为自定义代理；清空则回到系统代理语义
      if (val.trim() && cfg.mode !== 'custom') cfg.mode = 'custom';
      if (!val.trim() && cfg.mode === 'custom') cfg.mode = 'system';
      store.patch({ config: cfg });
      render();
    },
    onkeydown: (e) => {
      if (e.key === 'Enter') submit();
    },
  });

  hint = h('div', { class: 'field-hint' }, '留空即走系统代理；TUN 模式下无需填写');
  note = h('div', { class: 'exit-note' });
  summaryEl = h('div', { class: 'config-summary' });

  runBtn = h('button', { class: 'btn btn-primary', onclick: () => submit() }, '开始探测');

  const cancelBtn = h('button', {
    class: 'btn btn-danger',
    onclick: () => onCancel(),
  }, '取消');

  function submit() {
    const st = store.state;
    if (st.phase === 'running') return;
    const v = proxyValid(st.config.proxyUrl);
    if (st.config.mode === 'custom' && (!st.config.proxyUrl.trim() || !v.ok)) {
      render();
      input.focus();
      return;
    }
    onRun({ ...st.config });
  }

  function renderSummary() {
    clear(summaryEl);
    const st = store.state;
    const exit = st.exit;
    const modeText = st.config.mode === 'custom'
      ? '自定义代理 · ' + maskUrl(st.config.proxyUrl)
      : st.config.mode === 'direct'
        ? '直连对照 · 忽略所有代理设置'
        : '系统代理 · ' + ((exit && exit.detail) || '未识别出网路径');
    const presetText = (PRESETS.find((p) => p.key === st.config.preset) || PRESETS[1]).label + '档';

    summaryEl.appendChild(h('span', { class: 'exit-dot' }));
    summaryEl.appendChild(h('span', { class: 'cs-item', title: modeText }, modeText));
    summaryEl.appendChild(h('span', { class: 'cs-sep' }));
    summaryEl.appendChild(h('span', { class: 'cs-item' }, presetText));
    // 有提示时不展开整卡，改为折叠行上的一枚芯片，点开才看全文
    const warns = (exit && exit.warnings) || [];
    if (warns.length) {
      summaryEl.appendChild(h('button', {
        class: 'cs-warn',
        type: 'button',
        'aria-label': `展开查看 ${warns.length} 条提示`,
        onclick: () => { userExpanded = true; render(); },
      }, `⚠ ${warns.length} 条提示`));
    }
    summaryEl.appendChild(h('button', {
      class: 'btn btn-ghost btn-sm cs-edit',
      type: 'button',
      onclick: () => { userExpanded = true; render(); },
    }, '修改配置 ›'));
  }

  function render() {
    const st = store.state;
    const running = st.phase === 'running';

    // 运行中/初始态一律复位展开标志，只有「结果已出且用户没主动展开」才折叠
    if (st.phase !== lastPhase) {
      if (st.phase !== 'done') userExpanded = false;
      lastPhase = st.phase;
    }
    const collapsed = st.phase === 'done' && !userExpanded;

    const mode = st.config.mode;
    for (const m of segMode) {
      m.el.setAttribute('aria-pressed', String(mode === m.key));
      m.el.disabled = running;
    }
    for (const p of segPreset) {
      p.el.setAttribute('aria-pressed', String(st.config.preset === p.key));
      p.el.disabled = running;
    }
    const editable = !running;
    input.disabled = !editable;
    input.placeholder = mode === 'system'
      ? '可留空走系统代理；也可直接粘贴代理地址'
      : mode === 'direct'
        ? '直连模式：忽略所有代理设置'
        : 'socks5h://127.0.0.1:7890  或  http://user:pass@host:8080';

    const v = proxyValid(st.config.proxyUrl);
    const showErr = mode === 'custom' && !v.ok;
    input.classList.toggle('is-error', showErr);
    hint.className = 'field-hint' + (showErr ? ' is-error' : '');
    hint.textContent = showErr ? v.msg : (mode === 'custom' ? '支持 http / https / socks5 / socks5h，凭据不会落盘' : '留空即走系统代理；TUN 模式下无需填写');

    clear(runBtn);
    runBtn.appendChild(document.createTextNode(running ? '探测中…' : (st.phase === 'done' ? '重新探测' : '开始探测')));
    runBtn.disabled = running;
    cancelBtn.style.display = running ? '' : 'none';

    // 出口方式常驻可见，不随折叠隐藏
    clear(note);
    const exit = st.exit;
    if (exit) {
      note.appendChild(h('span', { class: 'exit-dot' }));
      note.appendChild(h('span', { class: 'exit-main' }, '出网方式：' + (exit.detail || '未知')));
      // warning 各自独占一行：拼在一行会变成一条读不完的长句
      for (const w of (exit.warnings || [])) {
        note.appendChild(h('span', { class: 'exit-warn' }, '⚠ ' + w));
      }
    } else {
      note.appendChild(h('span', { class: 'exit-dot' }));
      note.appendChild(h('span', { class: 'exit-main' }, '出网方式：启动探测后显示实际路径'));
    }

    el.classList.toggle('is-collapsed', collapsed);
    if (collapsed) renderSummary();
  }

  fieldsEl = h('div', { class: 'control-fields' },
    h('div', { class: 'field' }, h('span', { class: 'field-label' }, '出口模式'), modeSeg),
    h('div', { class: 'field grow' }, h('span', { class: 'field-label' }, '代理地址（可留空）'), input),
    h('div', { class: 'field' }, h('span', { class: 'field-label' }, '强度'), presetSeg),
    h('div', { class: 'actions' }, cancelBtn, runBtn),
    hint,
    note,
  );

  const el = h('section', { class: 'card control-panel' }, fieldsEl, summaryEl);

  // 让 hint 与 note 占满一行
  hint.style.width = '100%';

  render();
  return { el, render };
}
