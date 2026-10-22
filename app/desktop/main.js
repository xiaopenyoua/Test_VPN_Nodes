// ProxyLens 桌面外壳：启动 Go sidecar，解析 PROXYLENS_READY，再用原生窗口承载页面。
// 外壳只做三件事：拉起子进程、开窗、退出时收尸。探测逻辑全部留在 Go 里。
const { app, BrowserWindow, dialog, shell } = require('electron');
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');

const isMac = process.platform === 'darwin';
const READY_PREFIX = 'PROXYLENS_READY ';

let child = null;
let win = null;
let readyUrl = '';
let quitting = false;

// 单实例：第二次启动直接聚焦已有窗口，避免两个 sidecar 抢同一份状态
if (!app.requestSingleInstanceLock()) {
  app.quit();
}

function sidecarPath() {
  if (!app.isPackaged) {
    // 开发态用 build.sh local 的产物
    return path.join(__dirname, '..', 'build', process.platform === 'win32' ? 'proxylens.exe' : 'proxylens');
  }
  const dir = path.join(process.resourcesPath, 'bin');
  const suffix = process.platform === 'win32'
    ? `windows-${process.arch}.exe`
    : `darwin-${process.arch}`;
  try {
    const hit = fs.readdirSync(dir).find((f) => f.startsWith('proxylens-') && f.endsWith(suffix));
    if (hit) return path.join(dir, hit);
  } catch {
    // 目录不存在时落到下面的兜底路径，由启动失败提示用户
  }
  return path.join(dir, process.platform === 'win32' ? 'proxylens.exe' : 'proxylens');
}

function startSidecar() {
  return new Promise((resolve, reject) => {
    const bin = sidecarPath();
    if (!fs.existsSync(bin)) {
      reject(new Error(`找不到 sidecar：${bin}\n请先在 app/ 目录执行 ./build.sh local`));
      return;
    }

    child = spawn(bin, ['-no-open', '-emit-ready'], { stdio: ['ignore', 'pipe', 'pipe'] });
    let buf = '';
    let settled = false;

    const timer = setTimeout(() => {
      if (settled) return;
      settled = true;
      reject(new Error('sidecar 启动超时（20 秒）'));
    }, 20000);

    child.stdout.setEncoding('utf8');
    child.stdout.on('data', (chunk) => {
      buf += chunk;
      let idx;
      while ((idx = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, idx).trim();
        buf = buf.slice(idx + 1);
        if (!line.startsWith(READY_PREFIX) || settled) continue;
        settled = true;
        clearTimeout(timer);
        try {
          resolve(JSON.parse(line.slice(READY_PREFIX.length)));
        } catch (err) {
          reject(new Error('无法解析 PROXYLENS_READY：' + err.message));
        }
      }
    });

    child.stderr.setEncoding('utf8');
    child.stderr.on('data', (d) => console.error('[sidecar]', String(d).trim()));

    child.on('error', (err) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      reject(new Error('sidecar 启动失败：' + err.message));
    });

    child.on('exit', (code) => {
      child = null;
      if (!settled) {
        settled = true;
        clearTimeout(timer);
        reject(new Error(`sidecar 提前退出（code=${code}）`));
        return;
      }
      // 用户没点退出但 sidecar 没了：窗口留着也没用，直接收尾
      if (!quitting) app.quit();
    });
  });
}

function stopSidecar() {
  if (!child) return;
  const proc = child;
  child = null;
  try {
    proc.kill('SIGTERM');
  } catch {
    // 进程可能已经退出
  }
  // 3 秒还没退就强杀，避免残留进程占着端口
  setTimeout(() => {
    try {
      proc.kill('SIGKILL');
    } catch {
      // 已退出
    }
  }, 3000).unref();
}

function createWindow(url) {
  win = new BrowserWindow({
    width: 1280,
    height: 860,
    minWidth: 960,
    minHeight: 640,
    show: false,
    title: 'ProxyLens',
    // macOS：红绿灯嵌进内容区，配合 vibrancy 做半透明底
    titleBarStyle: isMac ? 'hiddenInset' : 'default',
    trafficLightPosition: isMac ? { x: 16, y: 14 } : undefined,
    vibrancy: isMac ? 'under-window' : undefined,
    visualEffectState: isMac ? 'active' : undefined,
    backgroundColor: isMac ? '#00000000' : '#0b0c0e',
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  win.once('ready-to-show', () => win.show());

  // 外链交给系统浏览器，不在应用内开新窗口
  win.webContents.setWindowOpenHandler(({ url: target }) => {
    shell.openExternal(target);
    return { action: 'deny' };
  });

  // 只允许停留在本地 sidecar 的同源页面
  const origin = new URL(url).origin;
  win.webContents.on('will-navigate', (e, target) => {
    if (new URL(target).origin !== origin) {
      e.preventDefault();
      shell.openExternal(target);
    }
  });

  win.loadURL(url + '&shell=electron');
  win.on('closed', () => {
    win = null;
  });
}

app.on('second-instance', () => {
  if (!win) return;
  if (win.isMinimized()) win.restore();
  win.focus();
});

app.on('before-quit', () => {
  quitting = true;
  stopSidecar();
});

app.on('window-all-closed', () => {
  if (!isMac) app.quit();
});

app.on('activate', () => {
  if (!win && readyUrl) createWindow(readyUrl);
});

app.whenReady().then(async () => {
  try {
    const ready = await startSidecar();
    readyUrl = ready.url;
    createWindow(readyUrl);
  } catch (err) {
    dialog.showErrorBox('ProxyLens 启动失败', err.message);
    quitting = true;
    stopSidecar();
    app.quit();
  }
});
