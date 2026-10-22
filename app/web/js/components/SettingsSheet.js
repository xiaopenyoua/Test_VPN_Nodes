import { h, clear, fmt } from '../core/dom.js';
import { store } from '../core/store.js';
import { Toggle } from './Toggle.js';

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

export function SettingsSheet({ onClose }) {
  const body = h('div', { class: 'drawer-body' });
  const el = h('div', { class: 'drawer-mask', onclick: (e) => { if (e.target === el) onClose(); } },
    h('aside', { class: 'drawer', role: 'dialog', 'aria-modal': 'true' },
      h('div', { class: 'drawer-head' },
        h('div', { class: 'drawer-title' }, '探测设置'),
        h('button', { class: 'btn btn-ghost btn-sm', onclick: onClose }, '关闭 ✕'),
      ),
      body,
    ),
  );

  // 打开长跑前先记住用户当时的短测参数，关闭时原样还回去（默认 quick 即 8 秒 / 20MB）。
  // 不做"关闭就变回 quick"的硬编码——那会把用户选的「深度」档静默降级。
  let shortSnapshot = null;

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
    render();
  }

  function estimateLine() {
    const mbps = measureMbps();
    if (!mbps) {
      return h('div', { class: 'settings-estimate' }, '流量预估：未知（首次运行后可见）');
    }
    const mb = (mbps * store.state.config.durationSec) / 8;
    if (mb > 1024) {
      return h('div', { class: 'settings-estimate is-warn' },
        `本次将消耗约 ${fmt.traffic(mb)} 流量，注意流量套餐`);
    }
    return h('div', { class: 'settings-estimate' },
      `按当前实测带宽约消耗 ${fmt.traffic(mb)}`);
  }

  function render() {
    clear(body);
    const longRun = !!store.state.config.longRun;

    // —— 长时稳定性测试 ——
    body.appendChild(h('div', { class: 'settings-row settings-row--longrun' },
      h('div', { class: 'settings-text' },
        h('div', { class: 'settings-label' }, '长时稳定性测试'),
        h('div', { class: 'settings-desc' },
          '专门抓"能连上但下载几十秒就断"的节点；单流跑满时长，看的是持续性而不是峰值'),
      ),
      Toggle({
        checked: longRun,
        label: '长时稳定性测试',
        onChange: toggleLongRun,
      }),
    ));
    if (longRun) body.appendChild(estimateLine());

    // —— 数值参数 ——
    for (const f of FIELDS) {
      let value = store.state.config[f.key];
      let desc = f.desc;
      let max = f.max;
      let disabled = false;

      if (longRun && f.key === 'durationSec') {
        value = LONG_DURATION_SEC;
        max = LONG_DURATION_SEC;
        disabled = true;
        desc = '长跑固定 30 分钟：短于此暴露不出"几十秒后断流"';
      } else if (longRun && f.key === 'concurrency') {
        value = 1;
        disabled = true;
        desc = '单流才等于你点「下载」时的真实体感';
      } else if (longRun && f.key === 'budgetMb') {
        desc = '长跑需要跑满 30 分钟，预算太少会被提前截断';
      }

      const input = h('input', {
        class: 'input num-input',
        type: 'number',
        min: f.min,
        max,
        value,
        disabled,
        oninput: (e) => {
          let v = parseInt(e.target.value, 10);
          if (Number.isNaN(v)) return;
          v = Math.max(f.min, Math.min(max, v));
          patchConfig({ [f.key]: v });
        },
      });
      body.appendChild(h('div', { class: 'settings-row' },
        h('div', { class: 'settings-text' },
          h('div', { class: 'settings-label' }, f.label),
          h('div', { class: 'settings-desc' }, desc),
        ),
        input,
      ));
    }

    body.appendChild(h('div', { class: 'settings-desc', style: 'margin-top:16px' },
      '评分口径：速度 40% + 稳定性 30% + AI 可用性 30%。探测目标固定为官方站点，不支持自定义 URL（避免被当成内网探测跳板）。'));
  }

  render();
  return { el };
}
