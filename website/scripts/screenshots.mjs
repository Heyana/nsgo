// Takes the home page's screenshots of the gallery from its web build:
// the desktop layout and the phone layout, in every language of the site
// and in light and dark, into src/assets/screens/ as
// <layout>-<lang>-<theme>.jpg. Run `bun run gallery` first; it needs Google Chrome
// and macOS's sips.
//
// The gallery is served from public/gallery/ and driven through Chrome's DevTools protocol, which waits for the wasm to start before capturing. It is a group's app.html — the apps group — the gallery without the platform frames of index.html and without the shell's toolbar.
import { execFileSync, spawn } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// Languages the gallery app itself ships catalogs for (examples/gallery/locales/).
const langs = ['en', 'zh'];

const here = path.dirname(fileURLToPath(import.meta.url));
const website = path.resolve(here, '..');
const publicDir = path.join(website, 'public');
const outDir = path.join(website, 'src', 'assets', 'screens');
const chrome = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
// Long enough to download, compile and start the gallery from disk.
const startWait = 30000;

// The group whose demos the home page's screenshots show.
const group = 'apps';

if (!existsSync(path.join(publicDir, 'gallery', group, 'app.html'))) {
  throw new Error('public/gallery/ is missing: run `bun run gallery` first');
}

const server = Bun.serve({
  port: 0,
  async fetch(request) {
    const { pathname } = new URL(request.url);
    const name = decodeURIComponent(pathname.replace(/^\//, '')).replace(/(^|\/)$/, '$1index.html');
    const file = Bun.file(path.join(publicDir, name));
    return (await file.exists()) ? new Response(file) : new Response('Not found', { status: 404 });
  },
});

const shots = [
  { name: 'desktop', width: 1440, height: 900, mobile: false, resize: ['-Z', '1800'], quality: '82' },
  { name: 'phone', width: 390, height: 844, mobile: true, resize: [], quality: '85' },
];
// Each site language's browser locale and the gallery's own (--nsgo-lang).
const locales = { en: { browser: 'en-US', app: 'en' }, zh: { browser: 'zh-CN', app: 'zh-Hans' } };
const themes = ['light', 'dark'];
const missing = langs.filter((lang) => !locales[lang]);
if (missing.length) throw new Error(`no locale for ${missing.join(', ')}: add it to scripts/screenshots.mjs`);
// Captures at once, each with its own Chrome; more starve the wasm of CPU.
const parallel = 4;

async function capture(url, shot, locale, out, port) {
  const profile = mkdtempSync(path.join(tmpdir(), 'nsgo-shot-'));
  const browser = spawn(
    chrome,
    ['--headless=new', `--lang=${locale}`, `--accept-lang=${locale}`, `--remote-debugging-port=${port}`, `--user-data-dir=${profile}`, 'about:blank'],
    { stdio: 'ignore' },
  );
  try {
    let page;
    for (let i = 0; i < 50 && !page; i++) {
      try {
        const targets = await (await fetch(`http://127.0.0.1:${port}/json`)).json();
        page = targets.find((target) => target.type === 'page');
      } catch {
        await Bun.sleep(200);
      }
    }
    const socket = new WebSocket(page.webSocketDebuggerUrl);
    await new Promise((resolve) => (socket.onopen = resolve));
    let id = 0;
    const pending = new Map();
    socket.onmessage = (event) => {
      const message = JSON.parse(String(event.data));
      const resolve = pending.get(message.id);
      if (typeof resolve === 'function') {
        pending.delete(message.id);
        resolve(message.result);
      }
    };
    const send = (method, params = {}) =>
      new Promise((resolve) => {
        pending.set(++id, resolve);
        socket.send(JSON.stringify({ id, method, params }));
      });

    await send('Emulation.setLocaleOverride', { locale });
    await send('Emulation.setDeviceMetricsOverride', {
      width: shot.width,
      height: shot.height,
      deviceScaleFactor: 2,
      mobile: shot.mobile,
    });
    if (shot.mobile) await send('Emulation.setTouchEmulationEnabled', { enabled: true });
    await send('Page.navigate', { url });
    await Bun.sleep(startWait);
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    const png = `${out}.png`;
    await Bun.write(png, Buffer.from(data, 'base64'));
    execFileSync('sips', ['-s', 'format', 'jpeg', '-s', 'formatOptions', shot.quality, ...shot.resize, png, '--out', out], {
      stdio: 'ignore',
    });
    rmSync(png);
    socket.close();
  } finally {
    // Free the debugging port for the slot's next capture.
    const exited = new Promise((resolve) => browser.once('exit', resolve));
    browser.kill();
    await exited;
    rmSync(profile, { recursive: true, force: true });
  }
}

try {
  mkdirSync(outDir, { recursive: true });
  for (const file of readdirSync(outDir)) if (file.endsWith('.jpg')) rmSync(path.join(outDir, file));
  const jobs = langs.flatMap((lang) =>
    themes.flatMap((theme) =>
      shots.map((shot) => {
        const query = new URLSearchParams({ theme, 'nsgo-lang': locales[lang].app, 'nsgo-appearance': theme });
        return {
          url: `http://localhost:${server.port}/gallery/${group}/app.html?${query}`,
          shot,
          locale: locales[lang].browser,
          out: path.join(outDir, `${shot.name}-${lang}-${theme}.jpg`),
        };
      }),
    ),
  );
  // Each capture runs its own Chrome on its own debugging port.
  let next = 0;
  const worker = async (slot) => {
    while (next < jobs.length) {
      const job = jobs[next++];
      await capture(job.url, job.shot, job.locale, job.out, 9320 + slot);
      console.log(path.relative(website, job.out));
    }
  };
  await Promise.all(Array.from({ length: parallel }, (_, slot) => worker(slot)));
  console.log(`Screenshots written to ${path.relative(website, outDir)}/`);
} finally {
  server.stop(true);
}
