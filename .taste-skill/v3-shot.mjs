import { Browser, sleep } from '/Users/wangq/.workbuddy/skills/design-taste-frontend/scripts/lib/cdp.mjs';
import { mkdirSync } from 'node:fs';

const URL = 'file:///Volumes/ssd/projects-github/Test_VPN_Nodes/design/v3/index.html';
const OUT = '/Volumes/ssd/projects-github/Test_VPN_Nodes/.taste-skill/v3/shots';
mkdirSync(OUT, { recursive: true });

const b = await Browser.launch({ width: 1440, height: 900 });
const shot = (p, n) => p.screenshot({ path: `${OUT}/${n}.jpg`, quality: 84 });
const click = (p, sel) => p.eval((s) => document.querySelector(s).click(), sel);

// 溢出体检：整页横向是否溢出、是否有元素超出视口
const probe = () => {
  const de = document.documentElement;
  const out = [];
  for (const el of document.querySelectorAll('.app *')) {
    const r = el.getBoundingClientRect();
    if (r.width && (r.right > window.innerWidth + 1 || r.left < -1)) {
      out.push(`${el.className || el.tagName} L${Math.round(r.left)} R${Math.round(r.right)}`);
    }
  }
  return { scrollW: de.scrollWidth, innerW: window.innerWidth, overflow: out.slice(0, 6) };
};

const errs = [];
const p = await b.newPage({ width: 1440, height: 900, dsf: 2 });
await p.goto(URL, { settleMs: 700 });
errs.push(...p.consoleErrors);

await click(p, '[data-tab="probes"]'); await sleep(350);
await shot(p, '01-probes-light');
console.log('1440 probes', JSON.stringify(await p.eval(probe)));

await click(p, '[data-setup="P1"]'); await sleep(350);
await shot(p, '02-probes-pset-p1');

await click(p, '[data-long]'); await sleep(300);
await shot(p, '03-probes-longrun-1800');

await click(p, '[data-tab="settings"]'); await sleep(250);
await click(p, '#modeSeg button:nth-child(2)'); await sleep(250);
await shot(p, '04-settings-custom');

await click(p, '[data-theme-opt="dark"]'); await sleep(250);
await click(p, '[data-tab="probes"]'); await sleep(300);
await shot(p, '05-probes-dark');
await click(p, '[data-tab="verdict"]'); await sleep(300);
await shot(p, '06-verdict-dark');

await click(p, '[data-theme-opt="light"]');
await click(p, '[data-tab="verdict"]'); await sleep(300);
await shot(p, '07-verdict-light');
await p.close();

const t = await b.newPage({ width: 768, height: 1024, dsf: 2 });
await t.goto(URL, { settleMs: 700 });
errs.push(...t.consoleErrors);
await click(t, '[data-tab="probes"]');
await click(t, '[data-setup="P1"]'); await sleep(400);
await shot(t, '08-tablet-probes');
console.log('768', JSON.stringify(await t.eval(probe)));
await t.close();

const m = await b.newPage({ width: 390, height: 844, dsf: 2, mobile: true });
await m.goto(URL, { settleMs: 700 });
errs.push(...m.consoleErrors);
await shot(m, '09-mobile-verdict');
console.log('390', JSON.stringify(await m.eval(probe)));
await click(m, '[data-tab="probes"]');
await click(m, '[data-setup="P1"]'); await sleep(400);
await shot(m, '10-mobile-pset');
await click(m, '[data-tab="settings"]'); await sleep(300);
await shot(m, '11-mobile-settings');
await m.close();

console.log('JS ERRORS:', errs.length, errs.slice(0, 5).join(' | '));
await b.close();
