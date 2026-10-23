import { h } from '../core/dom.js';

// 十二态是后端的结论枚举，措辞一个都不能省——「限流」和「不可达」是两件事。
// 但视觉上只收成四档：粘土拟态里十二种糖果色会让整页失去重点，
// 而用户真正要读的是「能用 / 要注意 / 不能用 / 没测」。
const MAP = {
  pending: { label: '待测', tone: 'idle' },
  skipped: { label: '未执行', tone: 'idle' },
  running: { label: '探测中', tone: 'run' },
  ok: { label: '可用', tone: 'ok' },
  auth_required: { label: '可达 · 需 Key', tone: 'warn' },
  degraded: { label: '受限', tone: 'warn' },
  inconclusive: { label: '无法确认', tone: 'warn' },
  rate_limited: { label: '限流 · 网络通', tone: 'warn' },
  challenge: { label: '被风控', tone: 'warn' },
  region_blocked: { label: '地区受限', tone: 'warn' },
  hijacked: { label: '疑似劫持', tone: 'bad' },
  failed: { label: '不可达', tone: 'bad' },
  cancelled: { label: '已取消', tone: 'idle' },
};

export function statusMeta(status) {
  return MAP[status] || { label: status || '未知', tone: 'idle' };
}

// 第二参数保留（旧调用点会传 'dot'），标签自带圆点后不再有变体差异
export function StatusBadge(status) {
  const m = statusMeta(status);
  return h('span', { class: 'tag tag-' + m.tone }, m.label);
}

// 卵石只有五档：可用 / 要注意 / 坏了 / 没测 / 正在跑
export function pebbleState(status) {
  return statusMeta(status).tone;
}
