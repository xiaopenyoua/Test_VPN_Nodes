// 主题解析：auto 跟随系统，light / dark 是用户的显式选择。
// 显式选择优先于系统偏好，否则用户点了「亮色」又被系统改回去，会以为按钮坏了。
const KEY = 'proxylens.theme';
const mq = window.matchMedia('(prefers-color-scheme: dark)');

export function resolveTheme(pref) {
  return pref === 'auto' ? (mq.matches ? 'dark' : 'light') : pref;
}

export function applyTheme(pref) {
  document.documentElement.dataset.theme = resolveTheme(pref);
  document.documentElement.dataset.themePref = pref;
}

export function saveTheme(pref) {
  localStorage.setItem(KEY, pref);
}

// 系统主题变化时只在 auto 模式下跟随；显式选择不动。
export function watchSystemTheme(onChange) {
  const fn = () => onChange();
  mq.addEventListener('change', fn);
  return () => mq.removeEventListener('change', fn);
}
