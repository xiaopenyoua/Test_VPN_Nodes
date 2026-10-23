import { h, clear, NONE } from '../core/dom.js';
import { store } from '../core/store.js';
import { ORDER, NAME, hasOverride, resolveProbe } from '../core/probes.js';
import { StatusBadge } from './StatusBadge.js';
import { ProbePanel } from './ProbePanel.js';

// 分评：六行，一行一项。行内就能展开「这项设置」，改完直接只测这一项，
// 不用先去别处改参数再回来——这是「单项测试」这条需求的操作路径。
export function ProbeGrid({ onOpen, onProbeOnly }) {
  const el = h('div', { class: 'rows rows-probe' });
  const rows = new Map();
  const panels = new Map();

  for (const id of ORDER) {
    const tag = h('span', { class: 'tag tag-idle' }, '待测');
    const value = h('span', { class: 'row-value' }, NONE);
    const setBtn = h('button', {
      class: 'btn-sm',
      type: 'button',
      'aria-pressed': 'false',
      onclick: (e) => {
        e.stopPropagation();
        const cur = store.state.openPanel;
        store.patch({ openPanel: cur === id ? null : id, probeTarget: id });
      },
    }, '这项设置');
    const onlyBtn = h('button', {
      class: 'btn-sm',
      type: 'button',
      onclick: (e) => {
        e.stopPropagation();
        if (onlyBtn.disabled) return;
        onProbeOnly(id);
      },
    }, '只测这项');

    const row = h('div', {
      class: 'row row-probe',
      role: 'button',
      tabindex: '0',
      'aria-label': `${id} ${NAME[id]} 详情`,
      onclick: (e) => {
        if (e.target.closest('.row-act')) return;
        onOpen(id);
      },
      onkeydown: (e) => {
        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onOpen(id); }
      },
    },
      h('span', { class: 'row-id' }, id),
      h('span', { class: 'row-name', title: NAME[id] }, NAME[id]),
      tag,
      value,
      h('span', { class: 'row-act' }, setBtn, onlyBtn),
    );

    const panel = ProbePanel({ id, onProbeOnly });
    panel.el.hidden = true;
    // 面板紧跟在自己那一行之后：展开时读起来是「这一行 + 它的参数」
    panel.el.id = `pset-${id}`;

    rows.set(id, { row, tag, value, setBtn, onlyBtn });
    panels.set(id, panel);
    el.appendChild(row);
    el.appendChild(panel.el);
  }

  function render(state) {
    const running = state.phase === 'running';
    const results = state.results || [];

    for (const id of ORDER) {
      const r = results.find((x) => x.id === id);
      const ui = rows.get(id);
      const open = state.openPanel === id;

      clear(ui.tag);
      if (r) {
        const badge = StatusBadge(r.status);
        ui.tag.className = badge.className;
        ui.tag.textContent = badge.textContent;
      } else {
        ui.tag.className = 'tag tag-idle';
        ui.tag.textContent = running ? '排队中' : '待测';
      }

      clear(ui.value);
      ui.value.appendChild(document.createTextNode(r ? (r.headline || NONE) : NONE));
      // 首屏不出现裸状态码：状态码是证据，不是给用户看的语言，只在详情里呈现
      const sub = r ? (r.subline || '').replace(/\s*·\s*HTTP\s*\d{3}/gi, '') : '';
      if (sub) ui.value.appendChild(h('small', {}, sub));
      if (r && r.only) ui.value.appendChild(h('small', {}, '单项复测 · 不参与总评'));
      if (hasOverride(state.probeConfig, id)) ui.value.appendChild(h('small', {}, '这项参数已单独调整'));

      const busy = state.onlyId === id;
      ui.onlyBtn.disabled = running;
      ui.onlyBtn.textContent = busy ? '测试中…' : '只测这项';

      ui.setBtn.setAttribute('aria-pressed', String(open));
      ui.setBtn.textContent = open ? '收起' : '这项设置';

      const panel = panels.get(id);
      panel.el.hidden = !open;
      if (open) panel.render(state);
    }
  }

  // 供「总评 → 点某颗卵石」跳过来时定位到那一行
  function focusProbe(id) {
    const ui = rows.get(id);
    if (ui) ui.row.scrollIntoView({ block: 'center', behavior: 'smooth' });
  }

  return { el, render, focusProbe };
}
