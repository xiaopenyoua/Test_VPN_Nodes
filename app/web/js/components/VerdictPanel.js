import { h, clear, NONE } from '../core/dom.js';
import { ORDER, NAME } from '../core/probes.js';
import { pebbleState } from './StatusBadge.js';

const STATE_LABEL = {
  usable: { text: 'AI 可用', tone: 'ok' },
  limited: { text: 'AI 受限', tone: 'warn' },
  blocked: { text: 'AI 不可用', tone: 'bad' },
};

// 分项条的色 = 该维度的档位色。三根条全用同一个色等于「把数字再画一遍」，
// 上色后一眼能看出哪一维拖后腿。
function barColor(score) {
  if (score >= 85) return 'var(--good)';
  if (score >= 70) return 'var(--pink)';
  if (score >= 55) return 'var(--warn)';
  return 'var(--bad)';
}

// 卵砂的宽度承载「这项花了多久」：0.7 最窄、3.6 最宽。
// 只比相对值不比绝对值——用户要的是哪一项拖慢了整轮，不是毫秒数。
function pebbleWidth(r, maxDur) {
  if (!r || !(r.durationMs > 0) || !(maxDur > 0)) return 1;
  return (0.7 + 2.9 * (r.durationMs / maxDur)).toFixed(2);
}

// 后端 verdict 是一整句：「短判断；原因一；原因二。」
// 标题只放判断句，原因落进段落。整句塞进 26px 的 h2，768 下会换五行，
// 首屏最该被读到的那句话反而读不完。措辞一字不改，只是拆行。
function splitVerdict(text) {
  const s = String(text || '').trim();
  if (!s) return { head: '', tail: '' };
  const parts = s.replace(/。$/, '').split('；').map((x) => x.trim()).filter(Boolean);
  if (!parts.length) return { head: s, tail: '' };
  return { head: parts[0], tail: parts.slice(1).join('；') };
}

// 结论区：总分币 + 裁定 + 六颗卵石 + 三个分项。
// 这一屏只回答一个问题——「这套代理整体能不能用」。
export function VerdictPanel({ onPickProbe }) {
  // 外框只负责把「结论卡」和「三个分项」上下排开；卡片本身是 .verdict
  const el = h('div', { class: 'verdict-wrap' });
  const coin = h('div', { class: 'score-coin' });
  const score = h('span', { class: 'score' }, NONE);
  coin.appendChild(score);
  const copy = h('div', { class: 'verdict-copy' });
  const pebbles = h('div', { class: 'pebbles', 'aria-label': '六项探测状态' });
  const pebbleBtns = new Map();

  for (const id of ORDER) {
    const b = h('button', {
      class: 'pebble',
      type: 'button',
      dataset: { state: 'idle' },
      title: `${id} ${NAME[id]} · 待测`,
      onclick: () => onPickProbe(id),
    }, h('i'), id);
    pebbleBtns.set(id, b);
    pebbles.appendChild(b);
  }

  const cap = h('div', { class: 'pebbles-cap' },
    h('span', {}, h('span', { class: 'cap-wide' }, '六项耗时 · 越宽越慢'), h('span', { class: 'cap-narrow' }, '六项状态')),
    h('span', {}, '点一颗看该项'),
  );

  const dims = h('div', { class: 'dims' });

  el.appendChild(h('div', { class: 'verdict' },
    h('div', { class: 'score-box' }, coin, h('span', { class: 'score-cap' }, '总分')),
    copy,
    h('div', { class: 'pebble-wrap' }, cap, pebbles),
  ));
  el.appendChild(dims);

  function render(state) {
    const s = state.summary;
    const running = state.phase === 'running';
    clear(copy);
    clear(dims);

    const results = state.results || [];

    // —— 没有分数：四种「没有」的原因必须分开说 ——
    if (!s) {
      const cancelled = state.phase === 'cancelled';
      const invalid = state.phase === 'done' && !!state.error;

      let text = running ? '…' : NONE;
      let tone = running ? 'run' : 'idle';
      let tagText = running ? '探测中' : '未开始';
      let title = running ? '正在探测真实链路…' : '还没有测过';
      let body = '点右上角「开始探测」，会真实访问 AI 站点并下载 Google 大文件，给出可用性裁定与下一步建议。';

      if (cancelled) {
        text = NONE;
        tone = 'idle';
        tagText = '已取消';
        title = '本次探测已取消';
        body = '半程数据不足以下结论，所以这里不给分数。重新开始就能拿到完整结果。';
      } else if (invalid) {
        text = NONE;
        tone = 'bad';
        tagText = '无法测量';
        title = '本轮没有测成，所以不给分数';
        body = state.error;
      }

      coin.dataset.tone = tone === 'run' ? 'idle' : tone;
      score.textContent = text;
      copy.appendChild(h('span', { class: 'tag tag-' + tone }, tagText));
      copy.appendChild(h('h2', {}, title));
      copy.appendChild(h('p', { class: 'lede' }, body));
      renderPebbles(results, running);
      return;
    }

    const st = STATE_LABEL[s.state] || STATE_LABEL.limited;
    coin.dataset.tone = st.tone;
    score.textContent = String(Math.round(s.total));

    copy.appendChild(h('span', { class: 'tag tag-' + st.tone }, s.stateLabel || st.text));
    const v = splitVerdict(s.verdict);
    copy.appendChild(h('h2', {}, v.head || st.text));
    if (v.tail) copy.appendChild(h('p', { class: 'lede' }, v.tail + '。'));

    // 三类文字必须分开站队：裁定（发生了什么）→ 封顶（分数为什么上不去）→ 下一步（怎么改）。
    // 封顶是评分口径，属于元信息；混进建议列表里，用户会把技术说明当成动作，
    // 真正该做的事反而被埋在两行小字中间。
    const caps = s.caps || [];
    if (caps.length) {
      copy.appendChild(h('div', { class: 'cap-note' },
        h('span', { class: 'cap-note-h' }, '分数封顶'),
        // 每条各占一行：合并成一句会被浏览器在任意汉字间断开，
        // 「总分封顶 69」被拆成「总」+「分封顶 69」就不好读了
        h('div', { class: 'cap-note-t' }, ...caps.map((c) => h('div', {}, c))),
      ));
    }

    const advice = s.advice || [];
    if (advice.length) {
      copy.appendChild(h('div', { class: 'next' },
        h('span', { class: 'next-h' }, '下一步'),
        h('ul', { class: 'tips' }, ...advice.map((t) => h('li', {}, t))),
      ));
    }

    for (const d of (s.dimensions || [])) {
      // 未测的维度显示「未测」而不是 0 分：0 分会让人以为测出来很差
      const skipped = d.measured === false;
      dims.appendChild(h('div', { class: 'dim' },
        h('div', { class: 'dim-top' },
          h('span', {}, d.label),
          h('span', { class: 'dim-w' }, skipped ? '未测' : `权重 ${Math.round(d.weight * 100)}%`),
        ),
        h('div', { class: 'dim-val' + (skipped ? ' is-skipped' : '') }, skipped ? NONE : String(Math.round(d.score))),
        h('div', { class: 'meter' }, h('i', {
          style: skipped
            ? '--p:0;--c:var(--ink-3)'
            : `--p:${Math.max(2, Math.round(d.score))};--c:${barColor(d.score)}`,
        })),
        d.detail ? h('p', { title: d.detail }, d.detail) : null,
      ));
    }

    renderPebbles(results, running);
  }

  function renderPebbles(results, running) {
    const maxDur = Math.max(0, ...results.map((r) => r.durationMs || 0));
    for (const id of ORDER) {
      const r = results.find((x) => x.id === id);
      const b = pebbleBtns.get(id);
      b.dataset.state = r ? pebbleState(r.status) : 'idle';
      b.style.setProperty('--w', pebbleWidth(r, maxDur));
      const label = r ? NAME[id] : (running ? `${NAME[id]} · 排队中` : `${NAME[id]} · 待测`);
      b.title = `${id} ${label}`;
    }
  }

  return { el, render };
}
