// 单一状态源：只做浅合并 + 订阅通知，避免引入框架。
const initial = {
  phase: 'idle', // idle | running | done | cancelled | error
  // longRun 打开后由后端（config.Normalize）把时长/流量/并发收敛成长跑口径；
  // 前端也会同步写入，保证「关掉开关能回到短测参数」有据可依。
  config: { mode: 'system', proxyUrl: '', preset: 'quick', concurrency: 1, budgetMb: 20, durationSec: 8, timeoutSec: 15, longRun: false },
  exit: null,
  env: null,
  results: [],
  summary: null,
  runId: null,
  startedAt: 0,
  elapsedMs: 0,
  error: '',
  detailId: null,
  // 总评与分评是两个独立功能页：总评来自一次完整运行，分评展示每一项的当前结果
  tab: 'verdict',
  // 正在单项复测的探测项 ID；null 表示没有单项复测在跑
  onlyId: null,
  theme: localStorage.getItem('proxylens.theme') || 'auto',
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
    onlyId: null,
    startedAt: Date.now(),
    elapsedMs: 0,
  });
}
