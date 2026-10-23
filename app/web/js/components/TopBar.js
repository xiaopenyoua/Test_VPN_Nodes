import { h } from '../core/dom.js';
import { store } from '../core/store.js';

// 顶栏是页面级元素：左边是这一页的名字与它回答的问题，右边是全局动作。
// 四页共用同一个顶栏，只有标题与动作的可用状态会变。
const COPY = {
  verdict: { title: '总评', sub: '这套代理整体能不能用' },
  probes: { title: '分评', sub: '六项真实探测各自的结果，每项都能单独调' },
  bandwidth: { title: '带宽', sub: '连续跑下去，这条链路会不会自己掉下来' },
  settings: { title: '设置', sub: '六项共用的整体参数' },
};

export function TopBar({ onRun, onCancel, onExport }) {
  const title = h('h1', {}, COPY.verdict.title);
  const sub = h('p', {}, COPY.verdict.sub);

  const exportBtn = h('button', {
    class: 'btn',
    type: 'button',
    onclick: () => onExport('md'),
  }, '导出报告');

  const cancelBtn = h('button', {
    class: 'btn',
    type: 'button',
    onclick: () => onCancel(),
    hidden: true,
  }, '取消');

  const runBtn = h('button', {
    class: 'btn btn-go',
    type: 'button',
    onclick: () => onRun(),
  }, '开始探测');

  const el = h('header', { class: 'top' },
    h('div', { class: 'top-copy' }, title, sub),
    h('div', { class: 'top-acts' }, exportBtn, cancelBtn, runBtn),
  );

  // 没有结果时导出按钮不可点，避免「点了才知道没东西」的空点击
  function render() {
    const st = store.state;
    const c = COPY[st.tab] || COPY.verdict;
    title.textContent = c.title;
    sub.textContent = c.sub;

    const running = st.phase === 'running';
    const has = !!st.summary;
    // 带宽页有自己的起停：那一档一跑就是半小时起，按钮必须贴着参数放，
    // 不该让用户先改完参数再回顶栏找「开始」。
    const own = st.tab === 'bandwidth';
    exportBtn.hidden = running || own;
    cancelBtn.hidden = !running || own;
    runBtn.hidden = own;
    exportBtn.disabled = !has;
    exportBtn.title = has ? '导出本次探测报告（Markdown）' : '先完成一次探测才能导出';

    runBtn.disabled = running;
    runBtn.textContent = running ? '探测中…' : (st.phase === 'done' ? '重新探测' : '开始探测');
    runBtn.title = running ? '探测进行中，可点「取消」中止' : '按当前整体设置跑完六项';
  }

  render();
  return { el, render };
}
