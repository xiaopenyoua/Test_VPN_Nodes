import { h, clear, fmt } from '../core/dom.js';
import { store } from '../core/store.js';
import { Toggle, setToggle } from './Toggle.js';
import { fieldsOf, supportsLongRun, LONG, longRunBudget, isOverridden, resolveProbe, NAME } from '../core/probes.js';

// 「这项设置」：只影响这一项的单测，不写回整体设置。
// 打开时留出的空位必须说清「现在这个数是从哪来的」——跟随整体还是这项自己改过，
// 否则用户改完再切到设置页看到另一个数，会以为改丢了。
export function ProbePanel({ id, onProbeOnly }) {
  const el = h('div', { class: 'pset' });
  const inputs = new Map();
  const flags = new Map();
  let head = null;
  let resetBtn = null;
  let runBtn = null;
  let longToggle = null;
  let longState = null;
  // 关掉长跑要把用户原来的短测参数还回去，而不是硬编码回 8 秒
  let preLong = null;

  function ov() {
    return (store.state.probeConfig || {})[id] || {};
  }

  function write(patch) {
    const all = store.state.probeConfig || {};
    store.patch({ probeConfig: { ...all, [id]: { ...(all[id] || {}), ...patch } } });
  }

  function drop(keys) {
    const all = store.state.probeConfig || {};
    const next = { ...(all[id] || {}) };
    for (const k of keys) delete next[k];
    store.patch({ probeConfig: { ...all, [id]: next } });
  }

  function measuredMbps() {
    const pick = (pid) => {
      const r = (store.state.results || []).find((x) => x.id === pid);
      return r && r.mbps > 0 ? r.mbps : 0;
    };
    return pick('P1') || pick('P6') || 0;
  }

  function build() {
    clear(el);
    inputs.clear();
    flags.clear();

    const name = NAME[id] || id;

    resetBtn = h('button', {
      class: 'link-btn',
      type: 'button',
      onclick: () => store.patch({ probeConfig: { ...store.state.probeConfig, [id]: {} } }),
    }, '恢复跟随整体');

    runBtn = h('button', {
      class: 'btn-sm',
      type: 'button',
      onclick: () => onProbeOnly(id),
    }, `只测 ${id}`);

    head = h('div', { class: 'pset-head' },
      h('b', {}, `${id} · ${name} · 这项设置`),
      resetBtn,
      runBtn,
    );

    const fields = h('div', { class: 'pset-fields' });

    for (const f of fieldsOf(id)) {
      const flag = h('span', { class: 'pf-flag' });
      flags.set(f.key, flag);
      const input = h('input', {
        class: 'num-input',
        type: 'number',
        min: f.min,
        max: f.max,
        inputmode: 'numeric',
        oninput: (e) => {
          const raw = parseInt(e.target.value, 10);
          if (Number.isNaN(raw)) return;
          const v = Math.max(f.min, Math.min(f.max, raw));
          write({ [f.key]: v });
        },
        onblur: (e) => {
          // 越界值被夹紧后要回写到框里，否则框里留着 9999、实际跑的是 1800
          const v = parseInt(e.target.value, 10);
          if (Number.isNaN(v)) { e.target.value = resolve()[f.key]; return; }
          const clamped = Math.max(f.min, Math.min(f.max, v));
          if (clamped !== v) e.target.value = clamped;
        },
      });
      inputs.set(f.key, input);
      fields.appendChild(h('div', { class: 'pf' },
        h('span', { class: 'pf-label' }, f.label, flag),
        h('span', { class: 'pf-ctl' }, input, h('span', { class: 'unit' }, f.unit)),
      ));
    }

    // 长时稳定性：只有 P1 跑下载，所以只有它有这个开关
    if (supportsLongRun(id)) {
      longState = h('div', { class: 'pf-state' });
      longToggle = Toggle({
        checked: false,
        label: '长时稳定性测试',
        onChange: (on) => toggleLong(on),
      });
      fields.appendChild(h('div', { class: 'pf pf-wide' },
        h('div', { class: 'pf' },
          h('span', { class: 'pf-label' }, '长时稳定性测试', h('span', { class: 'pf-flag' }, '30 分钟 · 单流')),
          h('span', { class: 'pf-ctl' }, longToggle),
        ),
        longState,
      ));
    }

    el.appendChild(head);
    el.appendChild(fields);
  }

  function resolve() {
    return resolveProbe(store.state.config, id, store.state.probeConfig);
  }

  // 打开长跑：时长/并发/预算必须一起改。只改时长会出现「跑 30 分钟但 20MB 就被截断」，
  // 图上后半段全 0，看起来像节点断流，实际是预算没了。
  function toggleLong(on) {
    if (on) {
      const cur = ov();
      preLong = {
        durationSec: cur.durationSec,
        budgetMb: cur.budgetMb,
        concurrency: cur.concurrency,
      };
      write({
        longRun: true,
        durationSec: LONG.durationSec,
        concurrency: 1,
        budgetMb: longRunBudget(measuredMbps()),
      });
      return;
    }
    const back = preLong || {};
    const next = { longRun: false };
    for (const k of ['durationSec', 'budgetMb', 'concurrency']) {
      if (back[k] !== undefined) next[k] = back[k];
    }
    const all = store.state.probeConfig || {};
    const merged = { ...(all[id] || {}), ...next };
    if (!preLong) {
      for (const k of ['durationSec', 'budgetMb', 'concurrency']) delete merged[k];
    }
    preLong = null;
    store.patch({ probeConfig: { ...all, [id]: merged } });
  }

  function render(state) {
    if (!head) build();
    const running = state.phase === 'running';
    const busy = state.onlyId === id;
    const g = state.config;
    const o = ov();
    const resolved = resolve();

    // 重绘时把焦点留在输入框里：每敲一个字符都 patch，不能把光标顶走
    for (const [key, input] of inputs) {
      input.disabled = running;
      if (document.activeElement !== input) input.value = resolved[key];
      const flag = flags.get(key);
      const own = isOverridden(state.probeConfig, id, key);
      flag.textContent = own ? '已覆盖' : '跟随整体';
      flag.classList.toggle('is-own', own);
      // 长跑生效时这三项由长跑口径决定，改单项没有意义
      const lockedByLong = !!resolved.longRun && ['durationSec', 'concurrency', 'budgetMb'].includes(key);
      input.disabled = running || lockedByLong;
      input.title = lockedByLong ? '长跑口径固定：30 分钟 / 单流' : `整体设置 ${g[key] ?? '—'}`;
    }

    const any = Object.keys(o).length > 0;
    resetBtn.disabled = running || !any;
    resetBtn.title = any ? '清掉这项的所有单独设置，回到整体设置' : '这项正在跟随整体设置';

    runBtn.disabled = running;
    runBtn.textContent = busy ? '测试中…' : `只测 ${id}`;

    if (longToggle) {
      const on = !!resolved.longRun;
      setToggle(longToggle, on, running);
      const mbps = measuredMbps();
      const budget = resolved.budgetMb;
      const need = (mbps > 0 ? mbps * LONG.durationSec : 0) / 8;
      if (!on) {
        longState.className = 'pf-state';
        longState.textContent = mbps > 0
          ? `按当前实测 ${fmt.mbps(mbps)} 推算，跑满 30 分钟约需 ${fmt.traffic(need)} 流量`
          : '开启后单流跑满 30 分钟，看的是持续性而不是峰值';
      } else if (mbps > 0 && need > budget) {
        // 预算不够：说出来，不要让用户等到 10 分钟后发现曲线全 0。
        // 而且不能一律劝加预算——按当前速度算需要 5.7 GB 时，上限只有 4 GB，
        // 那种建议等于让用户去填一个填不进去的数，必须改说「跑不满」。
        longState.className = 'pf-state is-warn';
        const lasts = fmt.dur((budget * 8 / mbps) * 1000);
        longState.textContent = need > LONG.maxBudgetMb
          ? `按当前速度跑满 30 分钟要 ${fmt.traffic(need)}，超过单次预算上限 ${fmt.traffic(LONG.maxBudgetMb)}，实际会在约 ${lasts} 处因预算耗尽提前结束`
          : `预算 ${fmt.traffic(budget)} 只够跑约 ${lasts}，想跑满 30 分钟请把流量预算提到 ${fmt.traffic(Math.ceil(need))} 以上`;
      } else {
        longState.className = 'pf-state';
        longState.textContent = `单流跑满 30 分钟 · 预算 ${fmt.traffic(budget)}${need > 0 ? `，预计消耗约 ${fmt.traffic(need)}` : ''}`;
      }
    }
  }

  build();
  return { el, render };
}
