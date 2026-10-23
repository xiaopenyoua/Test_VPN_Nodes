// 探测项的静态元信息与「整体设置 ⊕ 单项覆盖」的解析。
// 单一真源：顺序、名称、字段定义、长跑口径都只在这里写一次。

// 展示顺序：先 AI 可用性（这是用户最关心的），最后才是大文件下载（最慢的一项）
export const ORDER = ['P3', 'P4', 'P5', 'P6', 'P2', 'P1'];

export const NAME = {
  P1: 'Google 大文件下载',
  P2: 'YouTube 链路',
  P3: 'ChatGPT 网页',
  P4: 'Gemini 网页',
  P5: 'OpenAI API',
  P6: '国际链路基线',
};

// 长跑是「另一个量纲」的测试：固定 30 分钟、固定单流。
// 后端 config.Normalize 只在这些值为空时补默认，前端给了短测值就会照着跑，
// 所以时长/并发/预算必须由前端一起改，否则会出现「勾了长跑但 8 秒就结束」。
export const LONG = { durationSec: 1800, budgetMb: 1024, maxBudgetMb: 4096 };

// 每项能单独调什么。字段少是刻意的：能调的都必须真的影响该项结果。
export const FIELDS = {
  P1: [
    { key: 'durationSec', label: '下载时长', unit: '秒', min: 3, max: 1800 },
    { key: 'budgetMb', label: '流量预算', unit: 'MB', min: 4, max: 4096 },
    { key: 'concurrency', label: '并发连接', unit: '条', min: 1, max: 16 },
  ],
  default: [
    { key: 'timeoutSec', label: '单请求超时', unit: '秒', min: 5, max: 60 },
  ],
};

export function fieldsOf(id) {
  return FIELDS[id] || FIELDS.default;
}

// 只有 P1 跑下载，所以长跑开关只属于 P1
export function supportsLongRun(id) {
  return id === 'P1';
}

// 解析某项的最终参数：整体设置打底，单项覆盖压上去。
export function resolveProbe(globalCfg, id, overrides) {
  const ov = (overrides && overrides[id]) || {};
  const cfg = { ...globalCfg, ...ov };
  if (cfg.longRun) {
    cfg.durationSec = LONG.durationSec;
    cfg.concurrency = 1;
    if (!(cfg.budgetMb >= LONG.budgetMb)) cfg.budgetMb = LONG.budgetMb;
  }
  return cfg;
}

// 30 分钟跑满需要多少流量。不知道实测带宽时给下限，宁可变慢也不提前截断——
// 被预算截断的长跑在图上表现为「后半段全 0」，会被误读成节点断流。
export function longRunBudget(measuredMbps) {
  if (!(measuredMbps > 0)) return LONG.budgetMb;
  const need = Math.ceil((measuredMbps * LONG.durationSec) / 8 * 1.05);
  return Math.min(LONG.maxBudgetMb, Math.max(LONG.budgetMb, need));
}

// 该字段是否是「这项」自己改过的（而不是跟随整体）
export function isOverridden(overrides, id, key) {
  const ov = overrides && overrides[id];
  return !!ov && ov[key] !== undefined;
}

export function hasOverride(overrides, id) {
  const ov = overrides && overrides[id];
  return !!ov && Object.keys(ov).length > 0;
}
