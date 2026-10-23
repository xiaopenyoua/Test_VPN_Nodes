import { h } from '../core/dom.js';

const MAP = {
  pending: { label: '待测', tone: 'question' },
  skipped: { label: '未执行', tone: 'question' },
  running: { label: '探测中', tone: 'running' },
  ok: { label: '可用', tone: 'ok' },
  auth_required: { label: '可达 · 需 Key', tone: 'auth_required' },
  degraded: { label: '受限', tone: 'degraded' },
  inconclusive: { label: '无法确认', tone: 'degraded' },
  rate_limited: { label: '限流 · 网络通', tone: 'degraded' },
  challenge: { label: '被风控', tone: 'challenge' },
  region_blocked: { label: '地区受限', tone: 'region_blocked' },
  hijacked: { label: '疑似劫持', tone: 'hijacked' },
  failed: { label: '不可达', tone: 'failed' },
  cancelled: { label: '已取消', tone: 'question' },
};

export function statusMeta(status) {
  return MAP[status] || { label: status || '未知', tone: 'question' };
}

export function StatusBadge(status, variant) {
  const m = statusMeta(status);
  const ds = { status: m.tone };
  if (variant === 'dot') ds.variant = 'dot';
  return h('span', { class: 'badge', dataset: ds },
    h('span', { class: 'dot' }),
    m.label,
  );
}
