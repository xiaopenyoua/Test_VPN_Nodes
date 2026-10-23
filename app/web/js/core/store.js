// 单一状态源：只做浅合并 + 订阅通知，避免引入框架。
const initial = {
  phase: 'idle', // idle | running | done | cancelled | error
  // 整体设置：六项共用，一次完整探测（P1–P6）就按这里的值跑。
  // 长跑不在其中——长跑只有 P1 一个观测对象，属于 P1 的单项设置。
  config: { mode: 'system', proxyUrl: '', preset: 'quick', concurrency: 1, budgetMb: 20, durationSec: 8, timeoutSec: 15 },
  exit: null,
  env: null,
  results: [],
  summary: null,
  runId: null,
  startedAt: 0,
  elapsedMs: 0,
  error: '',
  detailId: null,
  // 四个独立目的地：总评来自一次完整运行，分评展示每一项的当前结果，
  // 带宽是独立的大文件长跑，设置放完整探测的高级参数
  tab: 'verdict',
  // 正在单项复测的探测项 ID；null 表示没有单项复测在跑
  onlyId: null,
  // 分评页当前选中的探测项，以及它自己的参数「覆盖项」。
  // 只存被显式改过的字段：空对象（或字段缺失）= 跟随整体设置。
  // 这样「改整体」能自然扩散到没单独调过的项，不必逐个同步六份副本。
  probeTarget: 'P1',
  probeConfig: {},
  // 分评页当前展开「这项设置」的探测项；null 表示全部收起，一次只开一个
  openPanel: null,
  // 上一次完整探测的耗时；单项复测不会覆盖它
  summaryElapsedMs: 0,
  // 独立带宽长跑：状态与六项结果完全隔离，跑两小时也不会覆盖分评里的 P1
  bandwidth: null,
  // 带宽页自己的参数。和 config（六项共用）分开存：
  // 这边是"小时 / GB"，那边是"秒 / MB"，混成一份会让两边互相污染。
  bwConfig: { durationSec: 1800, budgetMB: 8192, concurrency: 1 },
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
    onlyId: null,
    summaryElapsedMs: 0,
    startedAt: Date.now(),
    elapsedMs: 0,
  });
}
