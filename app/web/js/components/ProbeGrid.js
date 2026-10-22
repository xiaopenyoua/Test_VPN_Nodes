import { h, clear } from '../core/dom.js';
import { ProbeCard } from './ProbeCard.js';

const ORDER = ['P3', 'P4', 'P5', 'P6', 'P2', 'P1'];

// 探测卡网格：按 ID 复用实例，只更新变化的卡片。
export function ProbeGrid({ onOpen }) {
  const el = h('div', { class: 'probe-grid' });
  const cards = new Map();

  function render(state) {
    const running = state.phase === 'running';
    const results = state.results || [];
    const ids = ORDER.filter((id) => results.some((r) => r.id === id) || running);

    // 初始态：给 6 张"预告卡"说明将测什么，而不是留一片空白。
    // 不用骨架屏——骨架的语义是"数据在路上"，此刻并没有数据在路上。
    if (results.length === 0 && !running) {
      if (el.dataset.mode === 'preview') return;
      clear(el); cards.clear();
      el.dataset.mode = 'preview';
      for (const id of ORDER) {
        el.appendChild(h('div', { class: 'probe-preview', 'aria-hidden': 'false' },
          h('span', { class: 'probe-id' }, id),
          h('span', {}, nameOf(id)),
        ));
      }
      return;
    }

    if (el.dataset.mode === 'preview') {
      clear(el);
      el.removeAttribute('data-mode');
    }

    if (results.length === 0 && running) {
      clear(el); cards.clear();
      for (const id of ORDER) {
        const card = ProbeCard({ onOpen });
        cards.set(id, card);
        card.update({ id, name: nameOf(id), status: 'pending', headline: '—', phases: {} });
        el.appendChild(card.el);
      }
      return;
    }

    for (const id of ids) {
      let card = cards.get(id);
      if (!card) {
        card = ProbeCard({ onOpen });
        cards.set(id, card);
        el.appendChild(card.el);
      }
      const r = results.find((x) => x.id === id);
      if (r) card.update(r);
    }
    // 移除不在当前结果里的卡
    for (const [id, card] of cards) {
      if (!ids.includes(id)) { card.el.remove(); cards.delete(id); }
    }
  }

  return { el, render };
}

function nameOf(id) {
  return { P1: 'Google 大文件下载', P2: 'YouTube 链路', P3: 'ChatGPT 网页', P4: 'Gemini 网页', P5: 'OpenAI API', P6: '国际链路基线' }[id] || id;
}
