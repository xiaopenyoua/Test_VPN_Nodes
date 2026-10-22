import { h, clear, fmt } from '../core/dom.js';
import { StatusBadge } from './StatusBadge.js';
import { drawSpeed } from './SpeedChart.js';

const ATTR = {
  local: '本地网络',
  proxy: '代理链路',
  target: '目标服务',
  undetermined: '待定（需直连对照）',
};

export function DetailDrawer({ onClose }) {
  const body = h('div', { class: 'drawer-body' });
  const title = h('div', { class: 'drawer-title' }, '详情');
  const el = h('div', { class: 'drawer-mask', onclick: (e) => { if (e.target === el) onClose(); } },
    h('aside', { class: 'drawer', role: 'dialog', 'aria-modal': 'true' },
      h('div', { class: 'drawer-head' },
        title,
        h('button', { class: 'btn btn-ghost btn-sm drawer-close', onclick: onClose }, '关闭 ✕'),
      ),
      body,
    ),
  );

  function render(result) {
    if (!result) return;
    clear(body);
    title.textContent = `${result.id} · ${result.name}`;

    body.appendChild(h('div', { style: 'display:flex;align-items:center;gap:12px;flex-wrap:wrap' },
      StatusBadge(result.status),
      h('span', { style: 'font-family:var(--font-mono);font-size:20px;font-weight:600' }, result.headline || '—'),
      result.attribution ? h('span', { class: 'badge', dataset: { status: 'question' } }, '归因：' + (ATTR[result.attribution] || result.attribution)) : null,
    ));

    if (result.nextStep) {
      body.appendChild(h('div', { class: 'dim', style: 'border-color:var(--info-border);background:var(--info-bg)' },
        h('div', { class: 'dim-label', style: 'color:var(--info)' }, '下一步'),
        h('div', { style: 'font-size:13px;margin-top:4px' }, result.nextStep),
      ));
    }

    // 阶段瀑布
    const p = result.phases || {};
    const rows = [
      ['DNS', p.dnsMs, 'dns'],
      ['TCP 建连', p.tcpMs, 'tcp'],
      ['TLS 握手', p.tlsMs, 'tls'],
      ['TTFB', p.ttfbMs || result.ttfbMs, 'ttfb'],
    ].filter(([, v]) => v > 0);
    const maxV = Math.max(...rows.map(([, v]) => v), 1);
    if (rows.length) {
      body.appendChild(h('div', {},
        h('div', { class: 'dim-label', style: 'margin-bottom:8px' }, '阶段耗时'),
        h('div', { class: 'waterfall' }, ...rows.map(([label, v, cls]) => h('div', { class: 'wf-row' },
          h('span', { class: 'wf-label' }, label),
          h('div', { class: 'wf-track' }, h('i', { class: `wf-bar ${cls}`, style: `left:0;width:${Math.max(2, (v / maxV) * 100)}%` })),
          h('span', { class: 'wf-val' }, `${Math.round(v)}ms`),
        ))),
      ));
    }

    // 速度曲线
    if (result.samples && result.samples.filter((s) => s.mbps > 0).length > 1) {
      const cv = h('canvas');
      body.appendChild(h('div', {},
        h('div', { class: 'dim-label', style: 'margin-bottom:8px' }, '吞吐曲线'),
        h('div', { class: 'chart-wrap' },
          cv,
          h('div', { class: 'chart-legend' },
            h('span', {}, `均值 ${fmt.mbps(result.mbps)}`),
            h('span', {}, `P50 ${fmt.mbps(result.p50Mbps)}`),
            h('span', {}, `峰值 ${fmt.mbps(result.peakMbps)}`),
            h('span', {}, `CV ${result.cv ?? '—'}`),
          ),
        ),
      ));
      requestAnimationFrame(() => drawSpeed(cv, result.samples, { labels: true }));
    }

    // 关键指标
    const kv = h('dl', { class: 'kv' },
      kvRow('HTTP 状态', String(result.httpStatus || '—')),
      kvRow('最终 URL', result.finalUrl || '—'),
      kvRow('TLS', result.tlsVersion || '—'),
      kvRow('对端地址', result.remoteAddr || '—'),
      kvRow('下载字节', fmt.bytes(result.bytes)),
      kvRow('成功率', result.successPct ? `${result.successPct}%` : '—'),
      kvRow('抖动', result.jitterMs ? `${Math.round(result.jitterMs)} ms` : '—'),
      kvRow('耗时', result.durationMs ? `${Math.round(result.durationMs)} ms` : '—'),
    );
    body.appendChild(h('div', {}, h('div', { class: 'dim-label', style: 'margin-bottom:8px' }, '关键指标'), kv));

    // 证据链
    if (result.evidence && result.evidence.length) {
      body.appendChild(h('div', {},
        h('div', { class: 'dim-label', style: 'margin-bottom:8px' }, '证据链'),
        h('ul', { class: 'evidence' }, ...result.evidence.map((e) => h('li', {}, e))),
      ));
    }
    if (result.error) {
      body.appendChild(h('div', { class: 'dim', style: 'border-color:var(--danger-border);background:var(--danger-bg)' },
        h('div', { class: 'dim-label', style: 'color:var(--danger)' }, '错误'),
        h('div', { style: 'font-family:var(--font-mono);font-size:12px;margin-top:4px;word-break:break-all' }, result.error),
      ));
    }

    body.appendChild(h('details', {},
      h('summary', {}, '原始 JSON'),
      h('pre', { class: 'json' }, JSON.stringify(result, null, 2)),
    ));
  }

  return { el, render };
}

function kvRow(k, v) {
  return [h('dt', {}, k), h('dd', {}, v)];
}
