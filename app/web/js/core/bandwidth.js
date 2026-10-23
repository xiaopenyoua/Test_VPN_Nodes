// 独立带宽长跑的参数真源。
//
// 和六项综合测试不是一套量纲：那边是"顺手测一下"（秒 / MB），
// 这边是"挂两小时看它会不会掉下来"（小时 / GB）。
// 上限必须与后端 config.MaxBandwidthDurationSec / MaxBandwidthBudgetMB 一致，
// 否则前端允许填的值会被后端静默夹掉，界面上显示 4 小时、实际跑 30 分钟。

export const BW = {
  maxDurationSec: 14400, // 4 小时
  maxBudgetMB: 102400, // 100 GB
  minDurationSec: 60,
  minBudgetMB: 256,

  // 档位而不是自由输入：长跑是"离开电脑前点一下"的操作，
  // 让用户输 7320 秒既没有意义，也容易输错一个数量级。
  durations: [
    { sec: 900, label: '15 分' },
    { sec: 1800, label: '30 分' },
    { sec: 3600, label: '1 小时' },
    { sec: 7200, label: '2 小时' },
    { sec: 14400, label: '4 小时' },
  ],
  budgets: [
    { mb: 2048, label: '2 GB' },
    { mb: 8192, label: '8 GB' },
    { mb: 32768, label: '32 GB' },
    { mb: 65536, label: '64 GB' },
    { mb: 102400, label: '100 GB' },
  ],
  concurrencies: [1, 2, 4],

  default: { durationSec: 1800, budgetMB: 8192, concurrency: 1 },
};

// 时长说人话：「1 小时」而不是「3600 秒」。
// 4 小时档位上写「240 分钟」要靠心算才知道是一下午，这层换算不该留给用户。
export function durationLabel(sec) {
  const s = Number(sec) || 0;
  if (s < 60) return `${Math.round(s)} 秒`;
  if (s < 3600) {
    const m = s / 60;
    return `${Number.isInteger(m) ? m : m.toFixed(1)} 分钟`;
  }
  const hr = s / 3600;
  return `${Number.isInteger(hr) ? hr : hr.toFixed(1)} 小时`;
}

export function budgetLabel(mb) {
  const v = Number(mb) || 0;
  return v >= 1024 ? `${(v / 1024).toFixed(v % 1024 === 0 ? 0 : 1)} GB` : `${Math.round(v)} MB`;
}

// 跑满目标时长需要多少流量（MB）。
// 1.05 是给 TCP 重传和 Range 收尾留的余量：卡在理论值上，
// 最后一个窗口会因为差几 MB 预算而变空，图上看起来就像断流。
export function needMB(mbps, sec) {
  if (!(mbps > 0) || !(sec > 0)) return 0;
  return Math.ceil(((mbps * sec) / 8) * 1.05);
}

// 给定预算在该速度下最多能撑多久（秒）。
export function lastsSec(mbps, budgetMB) {
  if (!(mbps > 0) || !(budgetMB > 0)) return 0;
  return Math.floor((budgetMB * 8) / mbps);
}

// 预算与时长是否互相矛盾。返回一句直白的话，而不是让用户自己推。
// 之前综合测试里就犯过这个错：提示「请把预算提到 5.7 GB」，而上限只有 4 GB——
// 建议了一个填不进去的数，等于没建议。
export function budgetVerdict(mbps, sec, budgetMB) {
  if (!(mbps > 0)) {
    return { tone: 'idle', text: '测过一次之后，这里会按实测速度推算跑满全程需要多少流量' };
  }
  const need = needMB(mbps, sec);
  const lasts = lastsSec(mbps, budgetMB);
  if (need <= budgetMB) {
    return {
      tone: 'ok',
      text: `按实测 ${mbps.toFixed(1)} Mbps 跑满 ${durationLabel(sec)} 约需 ${budgetLabel(need)}，预算够用`,
    };
  }
  if (need > BW.maxBudgetMB) {
    return {
      tone: 'warn',
      text: `按实测 ${mbps.toFixed(1)} Mbps 跑满 ${durationLabel(sec)} 要 ${budgetLabel(need)}，` +
        `超过单次上限 ${budgetLabel(BW.maxBudgetMB)}，实际会在约 ${durationLabel(lasts)} 处因预算耗尽提前结束`,
    };
  }
  return {
    tone: 'warn',
    text: `预算 ${budgetLabel(budgetMB)} 在当前速度下只够跑约 ${durationLabel(lasts)}，` +
      `想跑满 ${durationLabel(sec)} 请把预算提到 ${budgetLabel(need)} 以上`,
  };
}
