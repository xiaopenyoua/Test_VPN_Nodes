import { h, fmt } from '../core/dom.js';
import { store } from '../core/store.js';
import { Toggle, setToggle } from './Toggle.js';

const FIELDS = [
  { key: 'concurrency', label: '并发连接数', desc: '默认 1 条流，等于你点「下载」时的真实速度；调高会把多流带宽叠加，数字更好看但你实际拿不到', min: 1, max: 16 },
  { key: 'budgetMb', label: '流量预算 (MB)', desc: '单次下载测试最多消耗的流量', min: 4, max: 4096 },
  { key: 'durationSec', label: '下载时长 (秒)', desc: '大文件下载的最长持续时间', min: 3, max: 120 },
  { key: 'timeoutSec', label: '单请求超时 (秒)', desc: '任一 HTTP 请求的超时上限', min: 5, max: 60 },
];

// 长跑是「另一个量纲」的测试：固定 30 分钟、固定单流。
// 这三个值必须一起改动，否则会出现「勾了长跑但 8 秒就结束」或
// 「跑了 20 分钟却因为 20MB 预算被提前截断」这类看起来像节点断流的假象。
const LONG_DURATION_SEC = 1800;
const LONG_MIN_BUDGET_MB = 1024;
const LONG_MAX_BUDGET_MB = 4096;
const SHORT_FALLBACK = { preset: 'quick', concurrency: 1, budgetMb: 20, durationSec: 8 };

// 设置页：只放完整探测（P1–P6）的高级参数。
// 单项复测的时长/流量/并发在「分评」页单独设置，两边互不影响。
export function SettingsPanel() {
  let shortSnapshot = null;
  const inputs = new Map();
  const descs = new Map();
  const estimate = h('div', { class: 'settings-estimate' });

  const toggle = Toggle({
    checked: false,
    label: '长时稳定性测试',
    onChange: (on) => toggleLongRun(on),
  });

  function measureMbps() {
    // 预估只用真实测出来的数字：P1 是同一个下载口径，没有就退到 P6 的链路吞吐。
    const pick = (id) => {
      const r = (store.state.results || []).find((x) => x.id === id);
      return r && r.mbps > 0 ? r.mbps : 0;
    };
    return pick('P1') || pick('P6') || 0;
  }

  function patchConfig(partial) {
    store.patch({ config: { ...store.state.config, ...partial } });
  }

  // 打开长跑前先记住用户当时的短测参数，关闭时原样还回去（默认 quick 即 8 秒 / 20MB）。
  // 不做"关闭就变回 quick"的硬编码——那会把用户选的「深度」档静默降级。
  function toggleLongRun(on) {
    const cfg = store.state.config;
    if (on) {
      shortSnapshot = {
        preset: cfg.preset,
        concurrency: cfg.concurrency,
        budgetMb: cfg.budgetMb,
        durationSec: cfg.durationSec,
      };
      const mbps = measureMbps();
      // 预算要盖住预估流量，否则 30 分钟的测试会先被预算截断，指标上全是 0 窗口。
      const need = mbps > 0
        ? Math.min(LONG_MAX_BUDGET_MB, Math.max(LONG_MIN_BUDGET_MB, Math.ceil((mbps * LONG_DURATION_SEC) / 8 * 1.05)))
        : LONG_MIN_BUDGET_MB;
      patchConfig({ longRun: true, durationSec: LONG_DURATION_SEC, concurrency: 1, budgetMb: need });
    } else {
      const back = shortSnapshot || SHORT_FALLBACK;
      shortSnapshot = null;
      patchConfig({ longRun: false, ...back });
    }
  }

  const rows = FIELDS.map((f) => {
    const input = h('input', {
      class: 'input num-input',
      type: 'number',
      min: f.min,
      max: f.max,
      oninput: (e) => {
        let v = parseInt(e.target.value, 10);
        if (Number.isNaN(v)) return;
        v = Math.max(f.min, Math.min(parseInt(input.max, 10) || f.max, v));
        patchConfig({ [f.key]: v });
      },
    });
    const desc = h('div', { class: 'settings-desc' }, f.desc);
    inputs.set(f.key, input);
    descs.set(f.key, desc);
    return h('div', { class: 'settings-row' },
      h('div', { class: 'settings-text' },
        h('div', { class: 'settings-label' }, f.label),
        desc,
      ),
      input,
    );
  });

  const el = h('div', { class: 'view' },
    h('section', { class: 'setting-row' },
      h('div', { class: 'setting-text' },
        h('h2', { class: 'setting-label' }, '长时稳定性测试'),
        h('p', { class: 'setting-desc' }, '专门抓「能连上但下载几十秒就断」的节点；单流跑满时长，看的是持续性而不是峰值。'),
      ),
      h('div', { class: 'setting-panel' },
        h('div', { class: 'settings-row settings-row--longrun' },
          h('div', { class: 'settings-text' },
            h('div', { class: 'settings-label' }, '开启 30 分钟长跑'),
            h('div', { class: 'settings-desc' }, '固定 30 分钟、固定单流；关闭后回到你原来的短测参数。'),
          ),
          toggle,
        ),
        estimate,
      ),
    ),
    h('section', { class: 'setting-row' },
      h('div', { class: 'setting-text' },
        h('h2', { class: 'setting-label' }, '数值参数'),
        h('p', { class: 'setting-desc' }, '这些值决定一次完整探测跑多久、花多少流量。单项复测在「分评」页有自己的参数。'),
      ),
      h('div', { class: 'setting-panel' }, ...rows),
    ),
    h('section', { class: 'setting-row' },
      h('div', { class: 'setting-text' },
        h('h2', { class: 'setting-label' }, '评分口径'),
        h('p', { class: 'setting-desc' }, '可用性是闸门：某一项不可用会直接封顶总分，而不是按权重扣几分。'),
      ),
      h('div', { class: 'setting-panel' },
        h('div', { class: 'settings-row' },
          h('div', { class: 'settings-text' },
            h('div', { class: 'settings-label' }, '速度 40% + 稳定性 30% + AI 可用性 30%'),
            h('div', { class: 'settings-desc' }, '探测目标固定为官方站点，不支持自定义 URL（避免被当成内网探测跳板）。'),
          ),
        ),
      ),
    ),
  );

  function sync() {
    const cfg = store.state.config;
    const longRun = !!cfg.longRun;
    setToggle(toggle, longRun);

    for (const f of FIELDS) {
      const input = inputs.get(f.key);
      const desc = descs.get(f.key);
      let value = cfg[f.key];
      let text = f.desc;
      let max = f.max;
      let disabled = false;

      if (longRun && f.key === 'durationSec') {
        value = LONG_DURATION_SEC;
        max = LONG_DURATION_SEC;
        disabled = true;
        text = '长跑固定 30 分钟：短于此暴露不出「几十秒后断流」';
      } else if (longRun && f.key === 'concurrency') {
        value = 1;
        disabled = true;
        text = '单流才等于你点「下载」时的真实体感';
      } else if (longRun && f.key === 'budgetMb') {
        text = '长跑需要跑满 30 分钟，预算太少会被提前截断';
      }

      input.max = max;
      input.disabled = disabled;
      // 正在输入的框不回写，否则光标会被推到末尾
      if (document.activeElement !== input) input.value = value;
      desc.textContent = text;
    }

    estimate.hidden = !longRun;
    if (!longRun) return;
    const mbps = measureMbps();
    if (!mbps) {
      estimate.textContent = '流量预估：未知（首次运行后可见）';
      estimate.classList.remove('is-warn');
      return;
    }
    const mb = (mbps * cfg.durationSec) / 8;
    const heavy = mb > 1024;
    estimate.textContent = heavy
      ? `本次将消耗约 ${fmt.traffic(mb)} 流量，注意流量套餐`
      : `按当前实测带宽约消耗 ${fmt.traffic(mb)}`;
    estimate.classList.toggle('is-warn', heavy);
  }

  sync();
  return { el, render: sync };
}
