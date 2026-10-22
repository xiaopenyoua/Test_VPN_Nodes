import { h } from '../core/dom.js';

const MAP = {
  pending: { label: '待测', tone: 'question', icon: '○' },
  skipped: { label: '未执行', tone: 'question', icon: '○' },
  running: { label: '探测中', tone: 'running', icon: '◍' },
  ok: { label: '可用', tone: 'ok', icon: '✓' },
  auth_required: { label: '可达 · 需 Key', tone: 'auth_required', icon: '◐' },
  degraded: { label: '受限', tone: 'degraded', icon: '!' },
  inconclusive: { label: '无法确认', tone: 'degraded', icon: '?' },
  rate_limited: { label: '限流 · 网络通', tone: 'degraded', icon: '~' },
  challenge: { label: '被风控', tone: 'challenge', icon: '⚠' },
  region_blocked: { label: '地区受限', tone: 'region_blocked', icon: '⊘' },
  hijacked: { label: '疑似劫持', tone: 'hijacked', icon: '✕' },
  failed: { label: '不可达', tone: 'failed', icon: '✕' },
  cancelled: { label: '已取消', tone: 'question', icon: '–' },
};

export function statusMeta(status) {
  return MAP[status] || { label: status || '未知', tone: 'question', icon: '·' };
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
