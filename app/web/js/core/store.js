// 单一状态源：只做浅合并 + 订阅通知，避免引入框架。
const initial = {
  phase: 'idle', // idle | running | done | cancelled | error
  config: { mode: 'system', proxyUrl: '', preset: 'quick', concurrency: 4, budgetMb: 24, durationSec: 8, timeoutSec: 15 },
  exit: null,
  env: null,
  results: [],
  summary: null,
  runId: null,
  startedAt: 0,
  elapsedMs: 0,
  error: '',
  detailId: null,
  theme: localStorage.getItem('proxylens.theme') || 'dark',
  streamOk: true,
};

export const store = {
  state: { ...initial },
  _subs: new Set(),

  patch(partial) {
    Object.assign(this.state, partial);
    this._emit();
  },

  updateResult(id, fn) {
    const list = this.state.results.slice();
    const i = list.findIndex((r) => r.id === id);
    if (i >= 0) list[i] = fn(list[i]);
    this.state.results = list;
    this._emit();
  },

  upsertResult(result) {
    const list = this.state.results.slice();
    const i = list.findIndex((r) => r.id === result.id);
    if (i >= 0) list[i] = result; else list.push(result);
    this.state.results = list;
    this._emit();
  },

  subscribe(fn) {
    this._subs.add(fn);
    return () => this._subs.delete(fn);
  },

  _emit() {
    for (const fn of this._subs) fn(this.state);
  },
};

export function resetForRun(cfg, exit) {
  store.patch({
    phase: 'running',
    config: cfg,
    exit,
    results: [],
    summary: null,
    error: '',
    detailId: null,
    startedAt: Date.now(),
    elapsedMs: 0,
  });
}
