import { h } from '../core/dom.js';
import { statusMeta } from './StatusBadge.js';

// 六项状态带：一次完整探测的时间轴上，每项一段。
// 这是这个产品的签名元素——一眼看出哪一项坏了，点一段直接跳到那一项。
const ORDER = ['P3', 'P4', 'P5', 'P6', 'P2', 'P1'];
const NAME = {
  P1: 'Google 大文件下载',
  P2: 'YouTube 链路',
  P3: 'ChatGPT 网页',
  P4: 'Gemini 网页',
  P5: 'OpenAI API',
  P6: '国际链路基线',
};

// 状态色只有三档：可用 / 受限 / 不可用；未测与探测中各自一档
function bandState(result) {
  if (!result) return 'pending';
  const tone = statusMeta(result.status).tone;
  if (tone === 'ok') return 'ok';
  if (tone === 'failed' || tone === 'hijacked') return 'failed';
  if (tone === 'running') return 'running';
  if (tone === 'question') return 'pending';
  return 'degraded';
}

export function ProbeBand({ onPick }) {
  const el = h('div', { class: 'band', 'aria-label': '六项探测状态' });
  const segs = new Map();

  for (const id of ORDER) {
    const seg = h('button', {
      class: 'band-seg',
      type: 'button',
      dataset: { state: 'pending' },
      title: `${id} ${NAME[id]} · 待测`,
      onclick: () => onPick(id),
    },
      h('span', { class: 'band-bar' }),
      h('span', { class: 'band-id' }, id),
    );
    segs.set(id, seg);
    el.appendChild(seg);
  }

  function render(state) {
    for (const id of ORDER) {
      const r = (state.results || []).find((x) => x.id === id);
      const seg = segs.get(id);
      seg.dataset.state = bandState(r);
      seg.title = `${id} ${NAME[id]} · ${r ? statusMeta(r.status).label : '待测'}`;
    }
  }

  return { el, render };
}
