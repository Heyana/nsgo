// nsgo.js, the loader, runs an app `nsgo build -target web` built, in a
// page of its own or in an element of another page:
//
//   import { mount } from "https://example.com/app/nsgo.0123456789.js";
//   const player = await mount(element, "https://example.com/app/nsgo.json", {
//     params: { user: "Ada" },
//     appearance: "dark",
//     host: { call(name, args) { ... } },
//   });
//   player.hostEvent("message", { text: "Order filled" });
//   player.setAppearance("light");
//
// nsgo.json names the app's files relative to itself. The host answers
// the app's calls: call(name, args) returns the result or a promise of
// it, and a throw or a rejection fails the call with its message; posts
// go to post(name, args) when the host has one, else to call, whose
// answer nobody hears. openURL(url), when the host has it, sees the URLs
// the app opens and returns, or resolves with, whether it took one.
// Without a host option the host is nsgo.json's host module's, whose
// createHost(player) returns it, or a promise of it.
//
// appearance ("light", "dark" or "system"), locale (a BCP 47 tag) and
// fontScale stand in for the system's, as a native host's settings do;
// the player's setAppearance, setLocale and setFontScale change them.
// module, a compiled WebAssembly.Module or a promise of one, spares
// fetching and compiling the wasm, so a page showing the app many times,
// or the wasm compressed, compiles it once.
// onProgress({ loaded, total }) replaces the download bar. total is
// null when the size of the decoded module is unknown. onProgress(null)
// clears the indication when startup ends. The app's host module can
// export onProgress to replace the bar in the app's own page.
//
// A release's nsgo.json names the scripts of the app's realm: mount loads
// them into an iframe of the page's own before the app starts, so a
// Content-Security-Policy that allows no 'unsafe-eval' runs the app
// (docs/architecture/tsx.md). The iframe is the page's global named
// NSGO_REALM, which the runtime adopts.

let apps = 0;

// mount runs the app nsgo.json at url describes in element, or the whole
// page when element is null; config, when given, is that file's content,
// which spares fetching it, and url (the page's when null) still what its
// names are relative to. It resolves with the player once the app runs.
export async function mount(element, url, options = {}) {
  const base = new URL(url ?? ".", document.baseURI);
  const config = options.config ?? (await (await fetch(base)).json());
  const at = (name) => new URL(name, base).href;
  if (typeof globalThis.Go !== "function") await loadScript(at(config.exec));
  const hostModule = options.host === undefined && config.host ? await import(at(config.host)) : null;
  const progress = options.module ? null : loadProgress(element, options.onProgress ?? hostModule?.onProgress);
  try {
    return await runApp(element, at, config, options, hostModule, progress?.report);
  } finally {
    progress?.close();
  }
}

async function runApp(element, at, config, options, hostModule, report) {
  const key = `nsgoHost${++apps}`;
  const environment = {
    appearance: options.appearance ?? "system",
    locale: options.locale ?? "",
    fontScale: options.fontScale ?? 0,
  };
  const bridge = {
    params: JSON.stringify(options.params ?? {}),
    environment: JSON.stringify(environment),
    events: [],
  };
  const setting = (name) => (value) => {
    environment[name] = value;
    bridge.environment = JSON.stringify(environment);
    bridge.setEnvironment?.(bridge.environment);
  };
  const player = {
    // hostEvent sends the app an event; a payload that is not JSON
    // throws here.
    hostEvent(name, payload) {
      const data = JSON.stringify(payload ?? null);
      if (data === undefined) throw new TypeError(`the event ${name}'s payload is not JSON`);
      if (bridge.deliver) bridge.deliver(String(name), data);
      else bridge.events.push([String(name), data]);
    },
    // setAppearance shows the app "light" or "dark", or "system" to
    // follow the system again; an appearance the app forces still wins.
    setAppearance: setting("appearance"),
    // setLocale has the app format for a BCP 47 tag, or "" for the
    // browser's language.
    setLocale: setting("locale"),
    // setFontScale sets the text size multiplier, or 0 for the browser's.
    setFontScale: setting("fontScale"),
  };
  let host = options.host;
  if (hostModule && (hostModule.createHost || typeof hostModule.onProgress !== "function")) {
    host = await hostModule.createHost?.(player);
    if (typeof host?.call !== "function") throw new Error("the app's host could not be loaded");
  }
  // An app in an element or with a host has the bridge; one filling the
  // page alone runs with no host.
  const bridged = element != null || host != null;
  if (bridged) globalThis[key] = bridge;
  if (host != null) {
    if (typeof host.call !== "function") throw new TypeError("a host has a call function");
    bridge.call = (id, name, data, post) => {
      const args = JSON.parse(data);
      const fn = post && typeof host.post === "function" ? host.post : host.call;
      const answer = Promise.resolve().then(() => fn.call(host, name, args));
      if (post) {
        answer.catch((e) => console.error(`nsgo: the host's post of ${name}:`, e));
        return;
      }
      answer.then(
        (value) => {
          let text;
          try {
            text = JSON.stringify(value ?? null);
          } catch (e) {
            bridge.reply(id, false, `the host's answer is not JSON: ${e.message}`);
            return;
          }
          bridge.reply(id, true, text);
        },
        (e) => bridge.reply(id, false, String(e?.message ?? e)),
      );
    };
    if (typeof host.openURL === "function") {
      bridge.openURL = (id, url) =>
        Promise.resolve()
          .then(() => host.openURL(url))
          .then(
            (handled) => bridge.opened(id, handled === true),
            () => bridge.opened(id, false),
          );
    }
  }

  const go = new Go();
  // The page's nsgo- parameters are the engine's flags: ?nsgo-fps&nsgo-lang=de.
  for (const [k, v] of new URLSearchParams(location.search)) {
    if (k.startsWith("nsgo-")) go.argv.push(v === "" ? `--${k}` : `--${k}=${v}`);
  }
  go.env = { NSGO_BASE: at(".") };
  if (config.resources) go.env.NSGO_RESOURCES = JSON.stringify(config.resources);
  if (bridged) go.env.NSGO_HOST = key;
  if (element) {
    element.dataset.nsgoMount = key;
    go.env.NSGO_MOUNT = `[data-nsgo-mount="${key}"]`;
  }
  const [instance, realm] = await Promise.all([
    options.module
      ? Promise.resolve(options.module).then((module) => WebAssembly.instantiate(module, go.importObject))
      : instantiateWasm(at(config.wasm), config.wasmSize, go.importObject, report),
    config.runtime && openRealm([config.runtime, ...(config.scripts ?? [])].map(at), config.realm && at(config.realm)),
  ]);
  // The realm's scripts load before instantiation, but the global is set
  // after, as the app reads it from its first instruction.
  if (realm) {
    const name = `nsgoRealm${++apps}`;
    globalThis[name] = realm;
    go.env.NSGO_REALM = name;
  }
  go.run(instance);
  return player;
}

function loadProgress(element, report) {
  if (report) return { report, close: () => report(null) };
  const bar = document.createElement("progress");
  bar.className = "nsgo-load-progress";
  bar.setAttribute("aria-label", "Loading application");
  (element ?? document.body).append(bar);
  return {
    close: () => bar.remove(),
    report({ loaded, total }) {
      if (total !== null) {
        bar.max = total;
        bar.value = loaded;
      }
    },
  };
}

async function instantiateWasm(url, size, imports, report) {
  const response = await fetch(url);
  if (!response.ok) throw new Error(`${url} could not be loaded (HTTP ${response.status})`);
  // The stream is decoded by fetch. A compressed response's Content-Length
  // counts the encoded bytes, so use the module's size from its build.
  const encoding = response.headers.get("Content-Encoding");
  const length = Number(response.headers.get("Content-Length"));
  const validSize = (n) => Number.isSafeInteger(n) && n > 0;
  const total = validSize(size) ? size : (!encoding || encoding === "identity") && validSize(length) ? length : null;
  let loaded = 0;
  report({ loaded, total });
  const body = response.body?.pipeThrough(new TransformStream({
    transform(chunk, controller) {
      loaded += chunk.byteLength;
      report({ loaded, total });
      controller.enqueue(chunk);
    },
  }));
  const type = response.headers.get("Content-Type")?.split(";")[0].trim().toLowerCase();
  if (body && type === "application/wasm" && typeof WebAssembly.instantiateStreaming === "function") {
    return (await WebAssembly.instantiateStreaming(
      new Response(body, { headers: { "Content-Type": type } }), imports,
    )).instance;
  }
  const bytes = await (body ? new Response(body) : response).arrayBuffer();
  return (await WebAssembly.instantiate(bytes, imports)).instance;
}

// openRealm makes the hidden iframe an app runs in, and loads into it the
// scripts its realm runs: each is the page's own file, which a policy
// allows where it allows any script.
//
// An offline release names page, an empty document of its own for the frame: a service worker serves the scripts of a frame showing it, and not those of an about:blank frame, which Chrome leaves uncontrolled.
async function openRealm(urls, page) {
  let frame = hiddenFrame();
  if (page) {
    const loaded = new Promise((resolve) => frame.addEventListener("load", resolve, { once: true }));
    frame.src = page;
    document.documentElement.append(frame);
    await loaded;
    // A host that refuses to be framed leaves the frame an error page, so the realm makes do with an about:blank one, as a release without web.offline does.
    if (frame.contentDocument?.URL !== page) {
      frame.remove();
      frame = hiddenFrame();
      page = null;
    }
  }
  if (!page) document.documentElement.append(frame);
  for (const url of urls) await loadScript(url, frame.contentWindow);
  return frame;
}

function hiddenFrame() {
  const frame = document.createElement("iframe");
  frame.setAttribute("aria-hidden", "true");
  frame.setAttribute("tabindex", "-1");
  frame.style.display = "none";
  return frame;
}

const scripts = new Map();

// loadScript runs the classic script at url once in the page's document,
// or in win's: a realm runs each script of its own, as its own realm.
function loadScript(url, win) {
  if (!win && scripts.has(url)) return scripts.get(url);
  const doc = (win ?? globalThis).document;
  const loading = new Promise((resolve, reject) => {
    const script = doc.createElement("script");
    script.src = url;
    script.onload = resolve;
    script.onerror = () => reject(new Error(`${url} could not be loaded`));
    doc.head.append(script);
  });
  if (!win) scripts.set(url, loading);
  return loading;
}
