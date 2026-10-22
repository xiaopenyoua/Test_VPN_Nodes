import { h, clear } from '../core/dom.js';
import { store } from '../core/store.js';

const FIELDS = [
  { key: 'concurrency', label: '并发连接数', desc: '大文件下载使用的并发流数量，越高越吃带宽', min: 1, max: 16 },
  { key: 'budgetMb', label: '流量预算 (MB)', desc: '单次下载测试最多消耗的流量', min: 4, max: 512 },
  { key: 'durationSec', label: '下载时长 (秒)', desc: '大文件下载的最长持续时间', min: 3, max: 120 },
  { key: 'timeoutSec', label: '单请求超时 (秒)', desc: '任一 HTTP 请求的超时上限', min: 5, max: 60 },
];

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

  function render() {
    clear(body);
    for (const f of FIELDS) {
      const input = h('input', {
        class: 'input num-input',
        type: 'number',
        min: f.min,
        max: f.max,
        value: store.state.config[f.key],
        oninput: (e) => {
          let v = parseInt(e.target.value, 10);
          if (Number.isNaN(v)) return;
          v = Math.max(f.min, Math.min(f.max, v));
          store.patch({ config: { ...store.state.config, [f.key]: v } });
        },
      });
      body.appendChild(h('div', { class: 'settings-row' },
        h('div', {},
          h('div', { class: 'settings-label' }, f.label),
          h('div', { class: 'settings-desc' }, f.desc),
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
