// nsgo-worker.js boots a compiled Go WebAssembly module inside a dedicated
// Web Worker so CPU-heavy background work (such as formula font parsing) runs
// off the page's main thread without re-fetching or recompiling the module.
self.onmessage = async (e) => {
  const msg = e.data;
  if (!msg || msg.kind !== "init") return;
  try {
    importScripts(msg.exec);
    const go = new Go();
    go.env = msg.env || {};
    const instance = await WebAssembly.instantiate(msg.module, go.importObject);
    go.run(instance);
  } catch (err) {
    self.postMessage({ kind: "bootError", error: String(err?.message ?? err) });
  }
};
