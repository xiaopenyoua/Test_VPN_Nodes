import { h } from '../core/dom.js';
import { store } from '../core/store.js';

// 顶栏是窗口级元素：产品标识在左，唯一的全局动作在右。
// 页面自己的名字与它回答的问题放在纸里的标题行（.page-head），那是全文档唯一的 h1。
const COPY = {
  verdict: { title: '总评', sub: '这套代理整体能不能用' },
  probes: { title: '分评', sub: '六项真实探测各自的结果' },
  settings: { title: '设置', sub: '一次完整探测的时长、流量与并发' },
};

// 标识：一枚镜头 —— 外环 + 斜向通光口，颜色取系统蓝
const MARK = '<svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg">' +
  '<circle cx="10" cy="10" r="8.1" stroke="currentColor" stroke-width="1.6"/>' +
  '<path d="M10 1.9A8.1 8.1 0 0 1 10 18.1Z" fill="currentColor" fill-opacity="0.18"/>' +
  '<path d="M4.9 4.9 15.1 15.1" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/>' +
  '</svg>';

export function TopBar({ onExport }) {
  const exportBtn = h('button', {
    class: 'btn btn-sm',
    type: 'button',
    onclick: () => onExport('md'),
  }, '导出报告');

  const el = h('header', { class: 'topbar' },
    h('div', { class: 'topbar-lead' },
      h('span', { class: 'mark', html: MARK, 'aria-hidden': 'true' }),
      h('div', { class: 'brand' },
        h('span', { class: 'brand-name' }, 'ProxyLens'),
        h('span', { class: 'brand-sub' }, '代理能力探测'),
      ),
    ),
    h('div', { class: 'topbar-right' }, exportBtn),
  );

  const title = h('h1', { class: 'page-title' }, COPY.verdict.title);
  const sub = h('div', { class: 'page-sub' }, COPY.verdict.sub);
  const head = h('div', { class: 'page-head' }, h('div', {}, title, sub));

  // 没有结果时导出按钮不可点，避免"点了才知道没东西"的空点击
  function render() {
    const c = COPY[store.state.tab] || COPY.verdict;
    title.textContent = c.title;
    sub.textContent = c.sub;
    const has = !!store.state.summary;
    exportBtn.disabled = !has;
    exportBtn.title = has ? '导出本次探测报告（Markdown）' : '先完成一次探测才能导出';
  }

  render();
  return { el, head, render };
}
