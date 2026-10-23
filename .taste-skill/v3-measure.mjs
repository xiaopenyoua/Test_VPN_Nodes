import { Browser } from '/Users/wangq/.workbuddy/skills/design-taste-frontend/scripts/lib/cdp.mjs';
const b = await Browser.launch({ width: 1440, height: 900 });
const p = await b.newPage({ width: 1440, height: 900, dsf: 1 });
await p.goto('file:///Volumes/ssd/projects-github/Test_VPN_Nodes/design/v3/index.html', { settleMs: 600 });
await p.eval(() => document.querySelector('[data-tab="settings"]').click());
await new Promise(r => setTimeout(r, 300));
const r = await p.eval(() => {
  const i = document.querySelector('#proxyUrl');
  const cs = getComputedStyle(i);
  const rect = i.getBoundingClientRect();
  const f = i.closest('.field').getBoundingClientRect();
  return { w: Math.round(rect.width), left: Math.round(rect.left), fieldLeft: Math.round(f.left), fieldRight: Math.round(f.right),
           ta: cs.textAlign, dir: cs.direction, pad: cs.paddingLeft, flex: cs.flex };
});
console.log(JSON.stringify(r));
await b.close();
