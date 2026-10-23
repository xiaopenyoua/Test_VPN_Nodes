// 与本地后端通信：所有请求带启动令牌；事件走 SSE。
const TOKEN_KEY = 'proxylens.token';

function readToken() {
  const fromUrl = new URLSearchParams(location.search).get('t');
  if (fromUrl) {
    sessionStorage.setItem(TOKEN_KEY, fromUrl);
    return fromUrl;
  }
  return sessionStorage.getItem(TOKEN_KEY) || '';
}

const token = readToken();

async function request(path, opts = {}) {
  const res = await fetch(path, {
    ...opts,
    headers: {
      'Content-Type': 'application/json',
      'X-ProxyLens-Token': token,
      ...(opts.headers || {}),
    },
  });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { raw: text }; }
  if (!res.ok) {
    throw new Error((data && data.error) || `请求失败（HTTP ${res.status}）`);
  }
  return data;
}

export const api = {
  token,
  state: () => request('/api/state'),
  run: (cfg) => request('/api/run', { method: 'POST', body: JSON.stringify(cfg) }),
  cancel: () => request('/api/cancel', { method: 'POST' }),
  reportUrl: (format) => `/api/report?format=${format}&t=${token}`,

  subscribe(handlers) {
    const es = new EventSource(`/api/events?t=${token}`);
    // bw_* 是独立带宽长跑的专用通道。刻意不复用 sample / probe_result：
    // 那两个名字的事件会被当成六项里 P1 的数据写进分评页。
    const types = [
      'snapshot', 'run_start', 'probe_start', 'sample', 'probe_result', 'score', 'run_end', 'error',
      'bw_start', 'bw_window', 'bw_result', 'bw_end',
    ];
    for (const t of types) {
      es.addEventListener(t, (ev) => {
        let payload = null;
        try { payload = JSON.parse(ev.data); } catch { /* ignore */ }
        const fn = handlers[t];
        if (fn) fn(payload);
      });
    }
    es.onerror = () => handlers.stream_error?.();
    return es;
  },
};
