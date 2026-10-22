import { h, clear } from '../core/dom.js';
import { ScoreRing } from './ScoreRing.js';

const STATE_LABEL = {
  usable: { text: 'AI 可用', tone: 'ok' },
  limited: { text: 'AI 受限', tone: 'degraded' },
  blocked: { text: 'AI 不可用', tone: 'failed' },
};

// 分项条的色 = 该维度的档位色。三根条全用品牌色等于"把数字再画一遍"，
// 上色后一眼能看出哪一维拖后腿。
function barColor(score) {
  if (score >= 85) return 'var(--ok)';
  if (score >= 70) return 'var(--brand)';
  if (score >= 55) return 'var(--warn)';
  return 'var(--danger)';
}

// 结论区：三态裁定 + 总分 + 分项 + 下一步
export function VerdictPanelFactory({ onExport }) {
  const el = h('section', { class: 'verdict' });

  function render(state) {
    const s = state.summary;
    const running = state.phase === 'running';
    clear(el);

    if (!s) {
      // 三种"没有分数"必须分开说：取消是用户主动中止、未开始是还没测、
      // 测完但没有分数是这一轮一个响应都没拿到——三者原因完全不同，
      // 混成一句话会让用户误以为是自己操作出了问题。
      const cancelled = state.phase === 'cancelled';
      const invalid = state.phase === 'done' && !!state.error;

      let ringText = running ? '探测中' : '等待检测';
      let ringState = running ? 'running' : 'idle';
      let badgeTone = running ? 'running' : 'question';
      let badgeText = running ? '探测中' : '未开始';
      let title = running ? '正在探测真实链路…' : '还没有测过';
      let body = '点「开始探测」后，会真实访问 AI 站点并下载 Google 大文件，给出可用性裁定与下一步建议。';

      if (cancelled) {
        ringText = '已取消';
        badgeText = '已取消';
        title = '本次探测已取消';
        body = '探测没有跑完，半程数据不足以下结论，所以这里不给分数——重新开始就能拿到完整结果。';
      } else if (invalid) {
        ringText = '未测成';
        ringState = 'idle';
        badgeTone = 'degraded';
        badgeText = '无法测量';
        title = '本轮没有测成，所以不给分数';
        body = state.error;
      }

      el.appendChild(h('div', { class: 'verdict-left' },
        ScoreRing(null, ringText, ringState),
      ));
      el.appendChild(h('div', { class: 'verdict-right' },
        h('div', { class: 'verdict-head' },
          h('span', { class: 'badge', dataset: { status: badgeTone } },
            h('span', { class: 'dot' }), badgeText),
        ),
        h('div', { class: 'verdict-title' }, title),
        h('div', { class: 'verdict-text' }, body),
        invalid
          ? h('div', { class: 'verdict-actions' },
              h('button', { class: 'btn btn-sm', onclick: () => onExport('json') }, '导出 JSON'),
            )
          : null,
      ));
      return;
    }

    const st = STATE_LABEL[s.state] || STATE_LABEL.limited;
    el.appendChild(h('div', { class: 'verdict-left' },
      ScoreRing(s.total, s.grade, s.state),
      h('span', { class: 'badge', dataset: { status: st.tone } }, h('span', { class: 'dot' }), st.text),
    ));

    el.appendChild(h('div', { class: 'verdict-right' },
      h('div', { class: 'verdict-head' },
        h('span', { class: 'verdict-title' }, s.stateLabel || st.text),
        h('span', { class: 'badge', dataset: { status: 'question' } }, s.grade),
        h('span', { class: 'badge', dataset: { status: 'question' } }, `复测波动 ±15 分内属正常`),
      ),
      h('div', { class: 'verdict-text' }, s.verdict || ''),
      h('div', { class: 'dims' }, ...(s.dimensions || []).map((d) => {
        // 未测的维度显示「未测」而不是 0 分：0 分会让人以为测出来很差
        const skipped = d.measured === false;
        return h('div', { class: 'dim' + (skipped ? ' is-skipped' : '') },
          h('div', { class: 'dim-top' },
            h('span', { class: 'dim-label' }, skipped ? `${d.label} · 未测` : `${d.label} · ${Math.round(d.weight * 100)}%`),
            h('span', { class: 'dim-score' }, skipped ? '—' : String(Math.round(d.score))),
          ),
          h('div', { class: 'dim-bar' }, h('i', {
            style: skipped
              ? 'width:0%;background:transparent'
              : `width:${Math.max(2, d.score)}%;background:${barColor(d.score)}`,
          })),
          h('div', { class: 'dim-detail', title: d.detail }, d.detail),
        );
      })),
      (s.caps && s.caps.length) ? h('div', { class: 'degraded-flag' }, '⚠ ' + s.caps.join('；')) : null,
      (s.advice && s.advice.length) ? h('div', { class: 'advice' }, ...s.advice.map((a) => h('div', { class: 'advice-item' }, a))) : null,
      h('div', { class: 'verdict-actions' },
        h('button', { class: 'btn btn-sm', onclick: () => onExport('md') }, '导出 Markdown'),
        h('button', { class: 'btn btn-sm', onclick: () => onExport('json') }, '导出 JSON'),
      ),
    ));
  }

  return { el, render };
}
