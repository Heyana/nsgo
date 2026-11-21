// The generated declarations above describe one complete web release.
const scope = new URL(self.registration.scope);
const namespace = "nsgo-offline:" + encodeURIComponent(scope.href) + ":";
const cacheName = namespace + version;
const index = new URL("index.html", scope).href;
const assets = new Map(files.map(({ url, integrity }) => [new URL(url, scope).href, integrity]));

self.addEventListener("install", (event) => {
  event.waitUntil((async () => {
    try {
      const cache = await caches.open(cacheName);
      await cache.addAll(files.map(({ url, integrity }) => {
        const href = new URL(url, scope).href;
        return new Request(href, { integrity, cache: href === index ? "reload" : "default" });
      }));
    } catch (error) {
      await caches.delete(cacheName);
      throw error;
    }
  })());
});

self.addEventListener("activate", (event) => {
  // The browser activates this worker after the previous one has no
  // clients, so live pages can still load assets from their own release.
  event.waitUntil((async () => {
    const names = await caches.keys();
    await Promise.all(names.filter((name) => name.startsWith(namespace) && name !== cacheName)
      .map((name) => caches.delete(name)));
    await self.clients.claim();
  })());
});

self.addEventListener("fetch", (event) => {
  const request = event.request;
  if (request.method !== "GET") return;
  const url = new URL(request.url);
  if (request.mode === "navigate" && url.origin === scope.origin &&
      (url.pathname === scope.pathname || url.pathname === new URL(index).pathname)) {
    // Online visits get the current page, which names its immutable files.
    // A disconnected visit gets the complete release of the active worker.
    event.respondWith(fetch(request, { cache: "no-cache" }).catch(offlinePage));
  } else if (url.href !== index && assets.has(url.href)) {
    event.respondWith((async () => {
      const response = await cached(url.href);
      if (!response) return fetch(new Request(request, { integrity: assets.get(url.href) }));
      return request.headers.has("Range") ? singleRange(request, response) : response;
    })());
  }
});

async function cached(url) {
  const cache = await caches.open(cacheName);
  return cache.match(url, { ignoreVary: true });
}

async function offlinePage() {
  const page = await cached(index);
  if (!page) return Response.error();
  if (!page.redirected) return page;
  // Static hosts may redirect index.html to the directory. Navigation
  // requests use manual redirect handling, so they need a local response.
  const headers = new Headers(page.headers);
  headers.delete("Content-Encoding");
  headers.delete("Content-Length");
  return new Response(page.body, { status: page.status, headers });
}

// Media can request a byte interval of an asset even on a disconnected page.
async function singleRange(request, response) {
  const match = /^bytes=(\d*)-(\d*)$/.exec(request.headers.get("Range"));
  if (!match || (!match[1] && !match[2])) return fetch(request);
  const body = await response.blob();
  const start = match[1] ? Number(match[1]) : Math.max(0, body.size - Number(match[2]));
  const end = match[1] && match[2] ? Math.min(body.size, Number(match[2]) + 1) : body.size;
  if (start >= end) {
    return new Response(null, { status: 416, headers: { "Content-Range": `bytes */${body.size}` } });
  }
  const headers = new Headers(response.headers);
  headers.delete("Content-Encoding");
  headers.set("Content-Length", String(end - start));
  headers.set("Content-Range", `bytes ${start}-${end - 1}/${body.size}`);
  headers.set("Accept-Ranges", "bytes");
  return new Response(body.slice(start, end), { status: 206, headers });
}
