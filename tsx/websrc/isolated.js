// Isolated guest runtime for opaque-origin sandboxed TSX packages on the web.
// Runs inside <iframe sandbox="allow-scripts"> with a strict CSP and talks to
// the Go host solely over a private MessagePort transferred at bootstrap.
(function (initToken) {
  "use strict";

  // Remove the bootstrap <script> tag immediately so package code cannot read it.
  try {
    if (document.currentScript && document.currentScript.parentNode) {
      document.currentScript.parentNode.removeChild(document.currentScript);
    }
  } catch (_) {}

  // Capture private references to built-ins before hardening or package code runs.
  let _eval = eval;
  const _Object = Object;
  const _Array = Array;
  const _Map = Map;
  const _Set = Set;
  const _Promise = Promise;
  const _Error = Error;
  const _TypeError = TypeError;
  const _ReferenceError = ReferenceError;
  const _Uint8Array = Uint8Array;
  const _ArrayBuffer = ArrayBuffer;
  const _TextEncoder = TextEncoder;
  const _TextDecoder = TextDecoder;
  const _URL = URL;
  const _atob = atob.bind(globalThis);
  const _btoa = btoa.bind(globalThis);
  const _Reflect = Reflect;
  const _Symbol = Symbol;
  const _JSON = JSON;
  const _Math = Math;
  const _Number = Number;
  const _String = String;
  const _Boolean = Boolean;
  const _queueMicrotask = queueMicrotask.bind(globalThis);
  const _parent = window.parent;
  const _addEventListener = window.addEventListener.bind(window);
  const _removeEventListener = window.removeEventListener.bind(window);

  // Neutralize dangerous browser APIs (in addition to opaque origin + CSP).
  function lockdownGlobals() {
    const blocked = [
      "XMLHttpRequest",
      "WebSocket",
      "EventSource",
      "Worker",
      "SharedWorker",
      "BroadcastChannel",
      "RTCPeerConnection",
      "webkitRTCPeerConnection",
      "RTCDataChannel",
      "fetch",
      "open",
      "alert",
      "confirm",
      "prompt",
      "print",
      "showOpenFilePicker",
      "showSaveFilePicker",
      "showDirectoryPicker",
      "cookieStore",
      "navigation",
      "caches",
      "indexedDB",
    ];
    for (const name of blocked) {
      try {
        delete globalThis[name];
      } catch (_) {}
      try {
        if (name in globalThis) {
          _Object.defineProperty(globalThis, name, {
            value: undefined,
            writable: false,
            configurable: false,
          });
        }
      } catch (_) {}
    }
    try {
      if (navigator && "sendBeacon" in navigator) {
        _Object.defineProperty(navigator, "sendBeacon", {
          value: undefined,
          writable: false,
          configurable: false,
        });
      }
    } catch (_) {}
  }

  let port = null;
  let closed = false;
  let hostNowMS = 0;
  let hostWallMS = 0;
  let activeOwnerId = 0;
  let config = null;

  // Pending outbound ops batched for the host.
  let ops = [];
  let flushScheduled = false;

  function emit(op) {
    if (closed) return;
    ops.push(op);
    if (!flushScheduled) {
      flushScheduled = true;
      _queueMicrotask(() => {
        flushScheduled = false;
        if (!closed && ops.length > 0 && port) {
          const batch = ops;
          ops = [];
          port.postMessage({ type: "ops", ops: batch, ownerId: activeOwnerId });
        }
      });
    }
  }

  function takeOps() {
    if (ops.length === 0) return null;
    const batch = ops;
    ops = [];
    return batch;
  }

  function post(msg, transfer) {
    if (closed || !port) return;
    if (transfer && transfer.length > 0) {
      port.postMessage(msg, transfer);
    } else {
      port.postMessage(msg);
    }
  }

  function makeCodedError(name, message, code, permission, resource) {
    let err;
    if (name === "TypeError") err = new _TypeError(message);
    else if (name === "ReferenceError") err = new _ReferenceError(message);
    else err = new _Error(message);
    if (name && name !== "Error" && name !== "TypeError" && name !== "ReferenceError") {
      err.name = name;
    }
    if (code) err.code = code;
    if (permission) err.permission = permission;
    if (resource) err.resource = resource;
    return err;
  }

  function encodeError(err) {
    if (err && typeof err === "object") {
      const out = {
        name: typeof err.name === "string" ? err.name : "Error",
        message: typeof err.message === "string" ? err.message : _String(err),
      };
      if (typeof err.stack === "string") out.stack = err.stack;
      if (typeof err.code === "string") out.code = err.code;
      if (typeof err.permission === "string") out.permission = err.permission;
      if (typeof err.resource === "string") out.resource = err.resource;
      return out;
    }
    return { raw: true, message: _String(err) };
  }

  // ---- Reactive graph (Solid-style fine-grained signals) ----

  let nextScopeId = 1;
  let currentOwner = null;
  let currentListener = null;
  let batchDepth = 0;
  let pendingComputations = null;
  let pendingEffects = [];
  let flushingEffects = false;

  function createOwner(parent, eagerScope) {
    const owner = {
      parent: parent || null,
      children: [],
      cleanups: [],
      contexts: null,
      catchFn: null,
      disposed: false,
      scopeId: 0,
      hasHostScope: false,
    };
    if (parent) parent.children.push(owner);
    if (eagerScope) ensureHostScope(owner);
    return owner;
  }

  function ensureHostScope(owner) {
    if (!owner) return 0;
    if (owner.hasHostScope) return owner.scopeId;
    const parentScopeId = owner.parent ? ensureHostScope(owner.parent) : 0;
    const id = nextScopeId++;
    owner.scopeId = id;
    owner.hasHostScope = true;
    emit({ op: "childScope", id, parent: parentScopeId });
    return id;
  }

  function currentHostScopeId() {
    return currentOwner ? ensureHostScope(currentOwner) : 0;
  }

  function disposeOwner(owner) {
    if (!owner || owner.disposed) return;
    owner.disposed = true;
    const children = owner.children;
    owner.children = [];
    for (let i = children.length - 1; i >= 0; i--) {
      disposeOwner(children[i]);
    }
    const cleanups = owner.cleanups;
    owner.cleanups = [];
    for (let i = cleanups.length - 1; i >= 0; i--) {
      try {
        cleanups[i]();
      } catch (e) {
        reportErrorToHost(e);
      }
    }
    if (owner.hasHostScope) {
      owner.hasHostScope = false;
      emit({ op: "disposeScope", id: owner.scopeId });
    }
    if (owner.parent && !owner.parent.disposed) {
      const idx = owner.parent.children.indexOf(owner);
      if (idx >= 0) owner.parent.children.splice(idx, 1);
    }
  }

  function runWithOwner(owner, fn) {
    const prevOwner = currentOwner;
    const prevListener = currentListener;
    currentOwner = owner;
    currentListener = null;
    try {
      return fn();
    } finally {
      currentOwner = prevOwner;
      currentListener = prevListener;
    }
  }

  function handleCaughtError(owner, err) {
    let cur = owner;
    while (cur) {
      if (cur.catchFn) {
        cur.catchFn(err);
        return true;
      }
      cur = cur.parent;
    }
    return false;
  }

  function cleanupDeps(comp) {
    if (comp.deps.size > 0) {
      for (const sig of comp.deps) {
        sig.subs.delete(comp);
      }
      comp.deps.clear();
    }
  }

  function runComputation(comp) {
    if (comp.owner.disposed) return;
    // Dispose children and cleanups from the previous run.
    const children = comp.owner.children;
    comp.owner.children = [];
    for (let i = children.length - 1; i >= 0; i--) {
      disposeOwner(children[i]);
    }
    const cleanups = comp.owner.cleanups;
    comp.owner.cleanups = [];
    for (let i = cleanups.length - 1; i >= 0; i--) {
      try {
        cleanups[i]();
      } catch (e) {
        if (!handleCaughtError(comp.owner, e)) throw e;
      }
    }
    cleanupDeps(comp);

    const prevOwner = currentOwner;
    const prevListener = currentListener;
    currentOwner = comp.owner;
    currentListener = comp;
    let res;
    try {
      res = comp.fn(comp.value);
    } catch (e) {
      currentOwner = prevOwner;
      currentListener = prevListener;
      if (handleCaughtError(comp.owner, e)) return;
      throw e;
    } finally {
      currentOwner = prevOwner;
      currentListener = prevListener;
    }

    if (comp.isEffect) {
      if (typeof res === "function") {
        comp.owner.cleanups.push(res);
      }
    } else if (comp.isMemo) {
      const changed = comp.equals ? !comp.equals(comp.value, res) : true;
      if (!comp.initialized || changed) {
        comp.initialized = true;
        comp.value = res;
        notifySignal(comp.signal);
      }
    }
  }

  function flushPending() {
    while (pendingComputations && pendingComputations.size > 0) {
      const list = _Array.from(pendingComputations);
      pendingComputations.clear();
      for (const comp of list) {
        if (!comp.owner.disposed) {
          runComputation(comp);
        }
      }
    }
    if (!flushingEffects && pendingEffects.length > 0) {
      flushingEffects = true;
      try {
        while (pendingEffects.length > 0) {
          const eff = pendingEffects.shift();
          if (!eff.owner.disposed) {
            runComputation(eff);
          }
        }
      } finally {
        flushingEffects = false;
      }
    }
  }

  function notifySignal(sig) {
    if (sig.subs.size === 0) return;
    const subs = _Array.from(sig.subs);
    if (!pendingComputations) pendingComputations = new _Set();
    for (const comp of subs) {
      if (comp.owner.disposed) {
        sig.subs.delete(comp);
        continue;
      }
      if (comp.isEffect) {
        if (!pendingEffects.includes(comp)) pendingEffects.push(comp);
      } else {
        pendingComputations.add(comp);
      }
    }
    if (batchDepth === 0) {
      flushPending();
    }
  }

  function makeEquals(options) {
    if (options && typeof options === "object" && "equals" in options) {
      if (typeof options.equals === "function") return options.equals;
      if (!options.equals) return null;
    }
    return (a, b) => a === b;
  }

  function createSignal(initial, options) {
    const eq = makeEquals(options);
    const sig = { value: initial, subs: new _Set() };
    function read() {
      if (currentListener) {
        sig.subs.add(currentListener);
        currentListener.deps.add(sig);
      }
      return sig.value;
    }
    read.set = function (v) {
      if (eq && eq(sig.value, v)) return v;
      sig.value = v;
      notifySignal(sig);
      return v;
    };
    read.update = function (fn) {
      const v = fn(sig.value);
      return read.set(v);
    };
    return read;
  }

  function createComputed(fn, options) {
    const owner = createOwner(currentOwner, false);
    const comp = {
      owner,
      fn,
      deps: new _Set(),
      isMemo: true,
      isEffect: false,
      initialized: false,
      value: undefined,
      equals: makeEquals(options),
      signal: { subs: new _Set() },
    };
    owner.cleanups.push(() => cleanupDeps(comp));
    runComputation(comp);
    return function () {
      if (currentListener) {
        comp.signal.subs.add(currentListener);
        currentListener.deps.add(comp.signal);
      }
      return comp.value;
    };
  }

  function createRenderEffect(fn) {
    const owner = createOwner(currentOwner, false);
    const comp = {
      owner,
      fn,
      deps: new _Set(),
      isMemo: false,
      isEffect: false,
      value: undefined,
    };
    owner.cleanups.push(() => cleanupDeps(comp));
    runComputation(comp);
  }

  function createEffect(fn) {
    const owner = createOwner(currentOwner, false);
    const comp = {
      owner,
      fn,
      deps: new _Set(),
      isMemo: false,
      isEffect: true,
      value: undefined,
    };
    owner.cleanups.push(() => cleanupDeps(comp));
    pendingEffects.push(comp);
    if (batchDepth === 0) {
      flushPending();
    }
  }

  function runBatch(fn) {
    batchDepth++;
    try {
      return fn();
    } finally {
      batchDepth--;
      if (batchDepth === 0) flushPending();
    }
  }

  function runUntracked(fn) {
    const prev = currentListener;
    currentListener = null;
    try {
      return fn();
    } finally {
      currentListener = prev;
    }
  }

  function registerCleanup(fn) {
    if (currentOwner && typeof fn === "function") {
      currentOwner.cleanups.push(fn);
    }
  }

  function createContext(defaultValue) {
    const id = _Symbol("nsgo.ctx");
    function read() {
      let cur = currentOwner;
      while (cur) {
        if (cur.contexts && cur.contexts.has(id)) {
          return cur.contexts.get(id);
        }
        cur = cur.parent;
      }
      return defaultValue;
    }
    read.Provider = function (props) {
      const owner = createOwner(currentOwner, false);
      owner.contexts = new _Map();
      owner.contexts.set(id, props ? props.value : undefined);
      let out;
      runWithOwner(owner, () => {
        const m = createComputed(() => (props ? props.children : undefined), { equals: false });
        out = m;
      });
      return out;
    };
    return read;
  }

  // ---- Callback, Node & Handle Registries ----

  let nextNodeId = 1;
  let nextFnId = 1;
  let nextHandleId = 1;
  let nextRpcId = 1;
  let nextFetchId = 1;
  let nextTimerId = 1;
  let nextRafId = 1;

  const callbacks = new _Map();
  const pendingRpcs = new _Map();
  const pendingFetches = new _Map();
  const timerCallbacks = new _Map();
  const rafCallbacks = new _Map();
  let activeTimers = 0;
  let activeRafs = 0;

  const VNODE_BRAND = _Symbol("nsgo.vnode");
  const HANDLE_BRAND = _Symbol("nsgo.handle");

  function registerCallback(fn) {
    const id = nextFnId++;
    callbacks.set(id, fn);
    if (currentOwner && !currentOwner.disposed) {
      currentOwner.cleanups.push(() => callbacks.delete(id));
    }
    return id;
  }

  function rpcPromise(mod, path, args) {
    return new _Promise((resolve, reject) => {
      const id = nextRpcId++;
      pendingRpcs.set(id, { resolve, reject });
      const batch = takeOps();
      if (batch) {
        post({ type: "ops", ops: batch, ownerId: activeOwnerId });
      }
      post({
        type: "rpc",
        id,
        mod,
        path,
        args: encodeArgs(args),
        ownerId: activeOwnerId,
      });
    });
  }

  const nodeProto = {
    focus() {
      emit({ op: "nodeCall", id: this._id, method: "focus" });
    },
    blur() {
      emit({ op: "nodeCall", id: this._id, method: "blur" });
    },
    scrollTo(opts) {
      emit({ op: "nodeCall", id: this._id, method: "scrollTo", args: [encodeVal(opts)] });
    },
    scrollToIndex(opts) {
      emit({ op: "nodeCall", id: this._id, method: "scrollToIndex", args: [encodeVal(opts)] });
    },
    measure() {
      return rpcPromise("$node", _String(this._id), ["measure"]);
    },
  };

  function makeVNode(id, kind) {
    const n = _Object.create(nodeProto);
    n[VNODE_BRAND] = true;
    n._id = id;
    n._kind = kind; // "el", "raw", "face", "region"
    n._parent = null;
    n._children = [];
    n._text = "";
    return n;
  }

  function isVNode(v) {
    return v !== null && typeof v === "object" && v[VNODE_BRAND] === true;
  }

  function isHandle(v) {
    return v !== null && typeof v === "object" && v[HANDLE_BRAND] === true;
  }

  function makeHostHandle(handleId) {
    const h = { [HANDLE_BRAND]: true, _hid: handleId };
    return h;
  }

  function makeNavHandle(handleId, initialRoutes) {
    const routesSig = createSignal(_Array.isArray(initialRoutes) ? initialRoutes.slice() : []);
    const h = makeHostHandle(handleId);
    h.routes = routesSig;
    h.push = (r) => {
      routesSig.set([...routesSig(), r]);
      emit({ op: "callHandle", id: handleId, method: "push", args: [encodeVal(r)] });
    };
    h.pop = (n) => {
      const k = n === undefined ? 1 : _Number(n) | 0;
      const cur = routesSig();
      if (cur.length > 1) routesSig.set(cur.slice(0, _Math.max(1, cur.length - k)));
      emit({ op: "callHandle", id: handleId, method: "pop", args: n === undefined ? [] : [k] });
    };
    h.popToRoot = () => {
      const cur = routesSig();
      if (cur.length > 1) routesSig.set(cur.slice(0, 1));
      emit({ op: "callHandle", id: handleId, method: "popToRoot", args: [] });
    };
    h.replace = (r) => {
      const cur = routesSig();
      if (cur.length > 0) routesSig.set([...cur.slice(0, -1), r]);
      emit({ op: "callHandle", id: handleId, method: "replace", args: [encodeVal(r)] });
    };
    h.setRoutes = (list) => {
      routesSig.set(_Array.isArray(list) ? list.slice() : []);
      emit({ op: "callHandle", id: handleId, method: "setRoutes", args: [encodeVal(list)] });
    };
    h.remove = (r) => {
      const cur = routesSig();
      const idx = cur.lastIndexOf(r);
      if (idx >= 0) routesSig.set([...cur.slice(0, idx), ...cur.slice(idx + 1)]);
      emit({ op: "callHandle", id: handleId, method: "remove", args: [encodeVal(r)] });
    };
    return h;
  }

  function encodeVal(v, seen) {
    if (v === undefined) return { __nsgo_u: 1 };
    if (v === null || typeof v === "boolean" || typeof v === "number" || typeof v === "string") {
      return v;
    }
    if (typeof v === "function") {
      return { __nsgo_fn: registerCallback(v) };
    }
    if (isVNode(v)) {
      return { __nsgo_node: v._id };
    }
    if (isHandle(v)) {
      return { __nsgo_obj: v._hid };
    }
    if (v instanceof _Uint8Array) {
      return { __nsgo_bytes: v.slice() };
    }
    if (v instanceof _ArrayBuffer) {
      return { __nsgo_bytes: new _Uint8Array(v.slice(0)) };
    }
    if (v instanceof Date) {
      return { __nsgo_date: v.getTime() };
    }
    if (!seen) seen = new _Set();
    if (seen.has(v)) return null;
    seen.add(v);
    if (_Array.isArray(v)) {
      const out = new _Array(v.length);
      for (let i = 0; i < v.length; i++) out[i] = encodeVal(v[i], seen);
      seen.delete(v);
      return out;
    }
    const out = {};
    const keys = _Object.keys(v);
    for (let i = 0; i < keys.length; i++) {
      const k = keys[i];
      if (k === "__proto__" || k === "constructor" || k === "prototype") continue;
      out[k] = encodeVal(v[k], seen);
    }
    seen.delete(v);
    return out;
  }

  function encodeArgs(args) {
    const out = new _Array(args.length);
    for (let i = 0; i < args.length; i++) out[i] = encodeVal(args[i]);
    return out;
  }

  function decodeVal(v) {
    if (v === null || v === undefined || typeof v !== "object") return v;
    if (v.__nsgo_u === 1) return undefined;
    if (v.__nsgo_err) {
      const e = v.__nsgo_err;
      return makeCodedError(e.name, e.message, e.code, e.permission, e.resource);
    }
    if (v.__nsgo_bytes) {
      return v.__nsgo_bytes;
    }
    if (v.__nsgo_obj) {
      const hid = v.__nsgo_obj;
      const h = makeHostHandle(hid);
      if (_Array.isArray(v.methods)) {
        for (const m of v.methods) {
          h[m] = (...args) => rpcPromise("$handle", _String(hid), [m, ...args]);
        }
      }
      return h;
    }
    if (_Array.isArray(v)) {
      return v.map(decodeVal);
    }
    const out = {};
    for (const k of _Object.keys(v)) {
      out[k] = decodeVal(v[k]);
    }
    return out;
  }

  function cloneJSON(v) {
    if (v === undefined) return null;
    if (v === null || typeof v === "boolean" || typeof v === "number" || typeof v === "string") {
      return v;
    }
    try {
      return _JSON.parse(_JSON.stringify(v));
    } catch (_) {
      return null;
    }
  }

  // ---- Virtual DOM & Reconciliation (mirrors tsx/dom.go) ----

  const rootVNode = makeVNode(0, "region");

  function createElementNode(tag) {
    const id = nextNodeId++;
    emit({ op: "el", id, tag, scopeId: currentHostScopeId() });
    return makeVNode(id, "el");
  }

  function createRawNode(text) {
    const id = nextNodeId++;
    const s = _String(text);
    const n = makeVNode(id, "raw");
    n._text = s;
    emit({ op: "raw", id, text: s });
    return n;
  }

  function removeChildVNode(parent, child) {
    const idx = parent._children.indexOf(child);
    if (idx < 0) return;
    parent._children.splice(idx, 1);
    child._parent = null;
    emit({ op: "rem", p: parent._id, c: child._id });
  }

  function insertBeforeVNode(parent, child, marker) {
    if (child === parent) throw new _TypeError("nsgo: a node cannot go inside itself");
    if (child._parent) removeChildVNode(child._parent, child);
    let idx = parent._children.length;
    if (marker) {
      const m = parent._children.indexOf(marker);
      if (m >= 0) idx = m;
    }
    parent._children.splice(idx, 0, child);
    child._parent = parent;
    emit({ op: "ins", p: parent._id, c: child._id, before: marker ? marker._id : 0 });
  }

  function nextSiblingVNode(c) {
    const p = c._parent;
    if (!p) return null;
    const i = p._children.indexOf(c);
    if (i >= 0 && i + 1 < p._children.length) return p._children[i + 1];
    return null;
  }

  function resolveChildren(value, out, depth) {
    if (depth > 64) throw new _TypeError("nsgo: children nested too deeply");
    if (value === null || value === undefined || typeof value === "boolean") return out;
    if (isVNode(value)) {
      out.push({ node: value });
      return out;
    }
    if (typeof value === "function") {
      return resolveChildren(value(), out, depth + 1);
    }
    if (_Array.isArray(value)) {
      for (let i = 0; i < value.length; i++) {
        resolveChildren(value[i], out, depth + 1);
      }
      return out;
    }
    if (typeof value === "object") return out;
    out.push({ text: _String(value) });
    return out;
  }

  function materialize(items, reuse) {
    if (reuse && reuse._kind === "raw" && items.length === 1 && !items[0].node) {
      if (reuse._text !== items[0].text) {
        reuse._text = items[0].text;
        emit({ op: "setRaw", id: reuse._id, text: reuse._text });
      }
      return [reuse];
    }
    const nodes = new _Array(items.length);
    for (let i = 0; i < items.length; i++) {
      nodes[i] = items[i].node ? items[i].node : createRawNode(items[i].text);
    }
    return nodes;
  }

  function reconcile(parent, old, b, marker) {
    const a = old.slice();
    let aStart = 0,
      bStart = 0;
    let aEnd = a.length,
      bEnd = b.length;
    let after = marker;
    if (a.length > 0) {
      const s = nextSiblingVNode(a[a.length - 1]);
      if (s) after = s;
    }
    let index = null;
    while (aStart < aEnd || bStart < bEnd) {
      if (aStart < aEnd && bStart < bEnd && a[aStart] === b[bStart]) {
        aStart++;
        bStart++;
        continue;
      }
      while (aEnd > aStart && bEnd > bStart && a[aEnd - 1] === b[bEnd - 1]) {
        aEnd--;
        bEnd--;
      }
      if (aEnd === aStart) {
        const at = bEnd < b.length ? b[bEnd] : after;
        while (bStart < bEnd) insertBeforeVNode(parent, b[bStart++], at);
      } else if (bEnd === bStart) {
        while (aStart < aEnd) {
          if (!index || !index.has(a[aStart])) removeChildVNode(parent, a[aStart]);
          aStart++;
        }
      } else if (a[aStart] === b[bEnd - 1] && b[bStart] === a[aEnd - 1]) {
        aEnd--;
        const at = nextSiblingVNode(a[aEnd]);
        const firstNext = nextSiblingVNode(a[aStart]);
        insertBeforeVNode(parent, b[bStart++], firstNext);
        aStart++;
        bEnd--;
        insertBeforeVNode(parent, b[bEnd], at);
        a[aEnd] = b[bEnd];
      } else {
        if (!index) {
          index = new _Map();
          for (let i = bStart; i < bEnd; i++) index.set(b[i], i);
        }
        const i = index.get(a[aStart]);
        if (i !== undefined && bStart < i && i < bEnd) {
          let k = aStart + 1;
          let seq = 1;
          while (k < aEnd && k < bEnd) {
            if (index.get(a[k]) !== i + seq) break;
            seq++;
            k++;
          }
          if (seq > i - bStart) {
            const at = a[aStart];
            while (bStart < i) insertBeforeVNode(parent, b[bStart++], at);
          } else {
            insertBeforeVNode(parent, b[bStart++], a[aStart]);
            if (!index.has(a[aStart])) removeChildVNode(parent, a[aStart]);
            aStart++;
          }
        } else if (i !== undefined) {
          aStart++;
        } else {
          removeChildVNode(parent, a[aStart++]);
        }
      }
    }
  }

  function isDynamic(value, depth) {
    if (typeof value === "function") return true;
    if (depth > 64 || !_Array.isArray(value)) return false;
    for (let i = 0; i < value.length; i++) {
      if (isDynamic(value[i], depth + 1)) return true;
    }
    return false;
  }

  function insertValue(parent, value, marker) {
    const shown = { list: [] };
    function show(val) {
      const items = resolveChildren(val, [], 0);
      const reuse = shown.list.length === 1 && items.length === 1 && !items[0].node ? shown.list[0] : null;
      const nodes = materialize(items, reuse);
      reconcile(parent, shown.list, nodes, marker || null);
      shown.list = nodes;
    }
    if (typeof value === "function") {
      createRenderEffect(() => show(value()));
    } else if (_Array.isArray(value) && isDynamic(value, 0)) {
      createRenderEffect(() => show(value));
    } else {
      show(value);
    }
  }

  function applyRefValue(target, val, name) {
    if (typeof target === "function") {
      if (typeof target.set === "function") target.set(val);
      else target(val);
      return true;
    }
    if (name) {
      throw new _TypeError("nsgo: ref={" + name + "}: " + name + " holds no signal or function to give the node");
    }
    return false;
  }

  function setElementProp(node, name, val) {
    if (name === "ref") {
      applyRefValue(val, node, "");
      return;
    }
    emit({ op: "prop", id: node._id, key: name, val: encodeVal(val) });
  }

  function spreadProps(node, props, skip) {
    if (!props || typeof props !== "object") return;
    const prev = new _Map();
    createRenderEffect(() => {
      const keys = _Object.keys(props);
      for (let i = 0; i < keys.length; i++) {
        const k = keys[i];
        if (k === skip) continue;
        const v = props[k];
        if (prev.has(k) && prev.get(k) === v) continue;
        prev.set(k, v);
        setElementProp(node, k, v);
      }
    });
  }

  function hostComponent(tag) {
    return function (props) {
      const n = createElementNode(tag);
      spreadProps(n, props, "children");
      if (props && typeof props === "object" && "children" in props) {
        insertValue(n, () => props.children, null);
      }
      return n;
    };
  }

  function condHelper(when, thenFn, elseFn) {
    const truthy = createComputed(() => !!when());
    let branchOwner = null;
    const branch = createComputed(
      () => {
        if (branchOwner) disposeOwner(branchOwner);
        branchOwner = createOwner(currentOwner, true);
        const fn = truthy() ? thenFn : elseFn;
        if (typeof fn !== "function") return null;
        return runWithOwner(branchOwner, () => fn());
      },
      { equals: false }
    );
    return branch;
  }

  function eachHelper(listFn, mapFn) {
    let entries = [];
    const mapped = createComputed(
      () => {
        const list = listFn() || [];
        const next = new _Array(list.length);
        const oldByItem = new _Map();
        for (let i = 0; i < entries.length; i++) {
          const e = entries[i];
          if (!oldByItem.has(e.item)) oldByItem.set(e.item, []);
          oldByItem.get(e.item).push(e);
        }
        const reused = new _Set();
        for (let i = 0; i < list.length; i++) {
          const item = list[i];
          const bucket = oldByItem.get(item);
          if (bucket && bucket.length > 0) {
            const e = bucket.shift();
            reused.add(e);
            e.index.set(i);
            next[i] = e;
          } else {
            const owner = createOwner(currentOwner, true);
            const idxSig = createSignal(i);
            let node;
            runWithOwner(owner, () => {
              node = mapFn(item, idxSig);
            });
            next[i] = { item, owner, index: idxSig, node };
          }
        }
        for (let i = 0; i < entries.length; i++) {
          if (!reused.has(entries[i])) disposeOwner(entries[i].owner);
        }
        entries = next;
        return entries.map((e) => e.node);
      },
      { equals: false }
    );
    return mapped;
  }

  function ErrorBoundary(props) {
    const id = nextNodeId++;
    const region = makeVNode(id, "region");
    emit({ op: "region", id, scopeId: currentHostScopeId() });
    const errSig = createSignal(null);
    let tryOwner = null;
    function reset() {
      errSig.set(null);
    }
    insertValue(
      region,
      () => {
        const err = errSig();
        if (err !== null) {
          const fb = props ? props.fallback : undefined;
          return typeof fb === "function" ? fb(err, reset) : fb;
        }
        if (tryOwner) disposeOwner(tryOwner);
        tryOwner = createOwner(currentOwner, true);
        tryOwner.catchFn = (e) => errSig.set(e);
        return runWithOwner(tryOwner, () => {
          try {
            return props ? props.children : undefined;
          } catch (e) {
            errSig.set(e);
            return null;
          }
        });
      },
      null
    );
    return region;
  }

  function createResource(a, b, c) {
    let source = null,
      fetcher = a,
      options = b;
    if (typeof b === "function") {
      source = a;
      fetcher = b;
      options = c;
    }
    const initial = options && "initialValue" in options ? options.initialValue : undefined;
    const value = createSignal(initial);
    const state = createSignal(initial !== undefined ? "ready" : "unresolved");
    const errSig = createSignal(undefined);
    let run = 0;
    let abort = null;
    let current = undefined;

    function load(src, refetching) {
      const mine = ++run;
      if (abort) abort.abort();
      const ctl = new globalThis.AbortController();
      abort = ctl;
      const info = { signal: ctl.signal, refetching };
      state.set(value() !== undefined ? "refreshing" : "pending");
      function settle(v, failed) {
        if (mine !== run) return;
        if (abort === ctl) abort = null;
        runBatch(() => {
          if (failed) {
            errSig.set(v);
            state.set("errored");
          } else {
            errSig.set(undefined);
            value.set(v);
            state.set("ready");
          }
        });
      }
      let res;
      try {
        res = source === null ? fetcher(info) : fetcher(src, info);
      } catch (e) {
        settle(e, true);
        return;
      }
      if (res && typeof res.then === "function") {
        res.then(
          (v) => settle(v, false),
          (e) => settle(e, true)
        );
      } else {
        settle(res, false);
      }
    }

    if (source === null) {
      runUntracked(() => load(null, false));
    } else {
      createRenderEffect(() => {
        const src = typeof source === "function" ? source() : source;
        current = src;
        if (src === undefined || src === null || src === false) return;
        runUntracked(() => load(src, false));
      });
    }
    registerCleanup(() => {
      run++;
      if (abort) {
        const ctl = abort;
        abort = null;
        ctl.abort();
      }
    });
    const read = () => value();
    read.loading = () => {
      const s = state();
      return s === "pending" || s === "refreshing";
    };
    read.error = () => errSig();
    read.state = () => state();
    read.refetch = () => {
      if (source === null) load(null, true);
      else if (current !== undefined) load(current, true);
    };
    read.mutate = (v) => {
      run++;
      runBatch(() => {
        value.set(v);
        errSig.set(undefined);
        state.set("ready");
      });
      return v;
    };
    return read;
  }

  // ---- Environment Signals ----

  const envSignals = {};
  let envSnapshot = {};
  let themeTokens = {};

  function getEnvSignal(key, initial) {
    if (!envSignals[key]) envSignals[key] = createSignal(initial);
    return envSignals[key];
  }

  function getEnvAccessor(key, initial) {
    const sig = getEnvSignal(key, initial);
    return () => sig();
  }

  function applyEnvPatch(patch) {
    if (!patch || typeof patch !== "object") return;
    runBatch(() => {
      for (const k of _Object.keys(patch)) {
        if (k === "tokens") {
          themeTokens = patch.tokens || {};
          continue;
        }
        envSnapshot[k] = patch[k];
        if (envSignals[k]) envSignals[k].set(patch[k]);
      }
    });
  }

  // ---- Timers, RAF, Console, ReportError ----

  function reportErrorToHost(err) {
    post({ type: "reportError", err: encodeError(err), ownerId: activeOwnerId });
  }

  function installRuntimeGlobals() {
    function makeSetTimer(repeat) {
      return function (fn, ms, ...args) {
        const maxTimers = config.limits ? config.limits.timers : 0;
        if (maxTimers > 0 && activeTimers >= maxTimers) {
          throw makeCodedError(
            "Error",
            "tsx: the sandbox's quota of " + maxTimers + " timers is used up",
            "ERR_NSH_QUOTA_EXCEEDED",
            "",
            "timers"
          );
        }
        const id = nextTimerId++;
        activeTimers++;
        timerCallbacks.set(id, { fn, args, repeat });
        const delay = _Math.max(_Number(ms) || 0, 0);
        const batch = takeOps();
        if (batch) post({ type: "ops", ops: batch, ownerId: activeOwnerId });
        post({ type: "setTimer", id, delay, repeat, ownerId: activeOwnerId });
        return id;
      };
    }
    function clearTimer(id) {
      const n = _Number(id) | 0;
      if (timerCallbacks.delete(n)) {
        activeTimers = _Math.max(0, activeTimers - 1);
        post({ type: "clearTimer", id: n });
      }
    }
    globalThis.setTimeout = makeSetTimer(false);
    globalThis.setInterval = makeSetTimer(true);
    globalThis.clearTimeout = clearTimer;
    globalThis.clearInterval = clearTimer;

    globalThis.requestAnimationFrame = function (fn) {
      const maxTimers = config.limits ? config.limits.timers : 0;
      if (maxTimers > 0 && activeRafs >= maxTimers) {
        throw makeCodedError(
          "Error",
          "tsx: the sandbox's quota of " + maxTimers + " timers is used up",
          "ERR_NSH_QUOTA_EXCEEDED",
          "",
          "timers"
        );
      }
      const id = nextRafId++;
      activeRafs++;
      rafCallbacks.set(id, fn);
      const batch = takeOps();
      if (batch) post({ type: "ops", ops: batch, ownerId: activeOwnerId });
      post({ type: "raf", id, ownerId: activeOwnerId });
      return id;
    };
    globalThis.cancelAnimationFrame = function (id) {
      const n = _Number(id) | 0;
      if (rafCallbacks.delete(n)) {
        activeRafs = _Math.max(0, activeRafs - 1);
        post({ type: "cancelRaf", id: n });
      }
    };
    globalThis.queueMicrotask = function (fn) {
      _Promise.resolve().then(() => {
        try {
          fn();
        } catch (e) {
          reportErrorToHost(e);
        }
      });
    };
    globalThis.reportError = function (err) {
      reportErrorToHost(err);
    };

    const consoleObj = {};
    for (const level of ["log", "info", "debug", "warn", "error"]) {
      consoleObj[level] = function (...args) {
        const parts = args.map((a) => {
          if (a !== null && typeof a === "object" && !(a instanceof _Error)) {
            try {
              return _JSON.stringify(a);
            } catch (_) {}
          }
          return _String(a);
        });
        post({ type: "console", level, text: parts.join(" ") });
      };
    }
    globalThis.console = consoleObj;
  }

  // ---- Web APIs Adapter (for web.js inside the opaque iframe) ----

  function makeWebNatives() {
    const encoder = new _TextEncoder();
    const blobs = new _Map();
    let blobSeq = 0;

    function urlParts(u) {
      return {
        href: u.href,
        protocol: u.protocol,
        username: u.username,
        password: u.password,
        host: u.host,
        hostname: u.hostname,
        port: u.port,
        pathname: u.pathname,
        search: u.search,
        hash: u.hash,
        origin: u.origin,
      };
    }

    return {
      now: () => hostNowMS,
      utf8Encode: (s) => encoder.encode(_String(s)).buffer,
      utf8Decode: (buf, fatal) => {
        try {
          return new _TextDecoder("utf-8", { fatal: !!fatal }).decode(buf);
        } catch (_) {
          return null;
        }
      },
      atob: (s) => {
        try {
          const clean = _String(s).replace(/[\t\n\f\r ]+/g, "");
          return _atob(clean);
        } catch (_) {
          return null;
        }
      },
      btoa: (s) => {
        try {
          return _btoa(_String(s));
        } catch (_) {
          return null;
        }
      },
      formEncode: (s) =>
        encodeURIComponent(_String(s))
          .replace(/%20/g, "+")
          .replace(/[!'()~]/g, (c) => "%" + c.charCodeAt(0).toString(16).toUpperCase()),
      formDecode: (s) => {
        const str = _String(s).replace(/\+/g, " ");
        try {
          return decodeURIComponent(str);
        } catch (_) {
          return str;
        }
      },
      parseURL: (u, base) => {
        try {
          return urlParts(base !== undefined ? new _URL(_String(u), _String(base)) : new _URL(_String(u)));
        } catch (_) {
          return null;
        }
      },
      setURL: (href, key, val) => {
        try {
          const u = new _URL(_String(href));
          u[key] = _String(val);
          return urlParts(u);
        } catch (_) {
          return null;
        }
      },
      baseURL: () => "asset:/",
      blobURL: (bytes, typ) => {
        const url = "blob:nsgo/" + ++blobSeq;
        blobs.set(url, { bytes: new _Uint8Array(bytes), type: _String(typ || "") });
        return url;
      },
      revokeBlobURL: (url) => {
        blobs.delete(_String(url));
      },
      blobOf: (url) => {
        const b = blobs.get(_String(url));
        if (!b) return null;
        return { bytes: b.bytes.slice().buffer, type: b.type };
      },
      fetch: (target, method, headers, body, redirect, done, fail) => {
        const id = nextFetchId++;
        pendingFetches.set(id, { done, fail });
        let bodyBytes = null;
        if (body !== undefined && body !== null) {
          if (body instanceof _Uint8Array) bodyBytes = body.slice();
          else if (body instanceof _ArrayBuffer) bodyBytes = new _Uint8Array(body.slice(0));
          else if (body.buffer instanceof _ArrayBuffer) {
            bodyBytes = new _Uint8Array(body.buffer.slice(body.byteOffset, body.byteOffset + body.byteLength));
          }
        }
        const batch = takeOps();
        if (batch) post({ type: "ops", ops: batch, ownerId: activeOwnerId });
        post({
          type: "fetch",
          id,
          url: _String(target),
          method: _String(method || "GET"),
          headers: _Array.isArray(headers) ? headers.map((p) => [_String(p[0]), _String(p[1])]) : [],
          body: bodyBytes,
          redirect: _String(redirect || "follow"),
          ownerId: activeOwnerId,
        });
        return () => {
          pendingFetches.delete(id);
          post({ type: "fetchCancel", id });
        };
      },
    };
  }

  // ---- Module Construction from Host Manifest ----

  const moduleCache = new _Map();
  let splitFn = null;
  let mergeFn = null;

  const HOST_TAGS = {
    View: "view",
    Text: "text",
    Scroll: "scroll",
    Image: "image",
    Overlay: "overlay",
    WebView: "webview",
    Video: "video",
    Input: "input",
  };

  const SUBSCRIBE_METHODS = new _Set([
    "nsgo.host.on",
    "nsgo.keymap.bind",
    "nsgo/mobile.BackHandler.onBackPress",
    "nsgo.Notifications.on",
    "nsgo.Purchases.on",
    "nsgo.UrlScheme.on",
    "nsgo.Sensors.watchLocation",
    "nsgo.Sensors.watchMotion",
    "nsgo.Sensors.watchSteps",
    "nsgo.Sensors.watchPressure",
    "nsgo.Sensors.watchActivity",
    "nsgo.Sensors.watchGeofences",
    "nsgo.Sensors.watchBackgroundMotion",
  ]);

  const VOID_METHODS = new _Set([
    "nsgo.host.post",
    "nsgo.Keyboard.dismiss",
    "nsgo.announce",
    "nsgo.setAccessibilityFocus",
    "nsgo.setColorScheme",
    "nsgo.UrlScheme.setUrl",
    "nsgo/mobile.Haptics.impact",
    "nsgo/mobile.Haptics.notification",
    "nsgo/mobile.Haptics.selection",
    "nsgo/mobile.BackHandler.exitApp",
  ]);

  function makeFaceComponent(mod, name, statics) {
    function Comp(props) {
      const id = nextNodeId++;
      const vnode = makeVNode(id, "face");
      const keys = props && typeof props === "object" ? _Object.keys(props) : [];
      const getterKeys = [];
      const propKeys = [];
      for (let i = 0; i < keys.length; i++) {
        const k = keys[i];
        if (k === "ref") continue;
        propKeys.push(k);
        const d = _Object.getOwnPropertyDescriptor(props, k);
        if (d && typeof d.get === "function") getterKeys.push(k);
      }
      if (props && typeof props === "object" && "ref" in props) {
        applyRefValue(props.ref, vnode, "");
      }
      emit({
        op: "face",
        id,
        mod,
        name,
        keys: propKeys,
        getterKeys,
        scopeId: currentHostScopeId(),
      });
      for (let i = 0; i < propKeys.length; i++) {
        const k = propKeys[i];
        if (getterKeys.includes(k)) {
          createRenderEffect(() => {
            const v = props[k];
            emit({ op: "faceProp", id, key: k, val: encodeVal(v) });
          });
        } else {
          emit({ op: "faceProp", id, key: k, val: encodeVal(props[k]) });
        }
      }
      return vnode;
    }
    if (_Array.isArray(statics)) {
      for (const s of statics) {
        Comp[s] = (...args) => rpcPromise(mod, name + "." + s, args);
      }
    }
    return Comp;
  }

  function makeClassProxy(mod, name, statics) {
    function Ctor(...args) {
      if (!new.target) {
        throw new _TypeError("Class constructor " + name + " cannot be invoked without 'new'");
      }
      const handleId = nextHandleId++;
      emit({ op: "construct", id: handleId, mod, name, args: encodeArgs(args), scopeId: currentHostScopeId() });
      const inst = makeHostHandle(handleId);
      inst.dispose = () => emit({ op: "disposeHandle", id: handleId });
      if (_Symbol.dispose) inst[_Symbol.dispose] = inst.dispose;
      return inst;
    }
    if (_Array.isArray(statics)) {
      for (const s of statics) {
        Ctor[s] = (...args) => rpcPromise(mod, name + "." + s, args);
      }
    }
    return Ctor;
  }

  function makeMethod(mod, path) {
    const full = mod + "." + path;
    if (full === "nsgo.AuthSession.start") {
      return function (buildFn, opts) {
        if (typeof buildFn !== "function") {
          throw new _TypeError("nsgo: AuthSession.start takes a function building the authorization URL");
        }
        const handleId = nextHandleId++;
        const rpcId = nextRpcId++;
        const result = new _Promise((resolve, reject) => {
          pendingRpcs.set(rpcId, { resolve, reject });
        });
        const batch = takeOps();
        if (batch) post({ type: "ops", ops: batch, ownerId: activeOwnerId });
        post({
          type: "authStart",
          handleId,
          rpcId,
          args: encodeArgs([buildFn, opts]),
          ownerId: activeOwnerId,
        });
        return {
          result,
          cancel() {
            emit({ op: "callHandle", id: handleId, method: "cancel", args: [] });
          },
        };
      };
    }
    if (SUBSCRIBE_METHODS.has(full)) {
      return function (...args) {
        const handleId = nextHandleId++;
        emit({
          op: "subscribe",
          id: handleId,
          mod,
          path,
          args: encodeArgs(args),
          scopeId: currentHostScopeId(),
        });
        function unsub() {
          emit({ op: "disposeHandle", id: handleId });
        }
        unsub.remove = unsub;
        return unsub;
      };
    }
    if (VOID_METHODS.has(full)) {
      return function (...args) {
        emit({ op: "callVoid", mod, path, args: encodeArgs(args), scopeId: currentHostScopeId() });
      };
    }
    if (full === "nsgo.host.available") {
      return function () {
        return !!envSnapshot.hostAvailable;
      };
    }
    if (full === "nsgo.keymap.label") {
      return function (k) {
        return _String(k);
      };
    }
    return function (...args) {
      return rpcPromise(mod, path, args);
    };
  }

  function makeServiceObject(mod, name, entry) {
    const obj = {};
    if (entry.members) {
      for (const k of _Object.keys(entry.members)) {
        const m = entry.members[k];
        if (m.kind === "fn") {
          obj[k] = makeMethod(mod, name + "." + k);
        } else if (m.kind === "accessor") {
          if (mod === "nsgo" && name === "host" && k === "params") {
            _Object.defineProperty(obj, "params", {
              enumerable: true,
              configurable: false,
              get: () => envSnapshot.hostParams,
            });
          } else {
            obj[k] = m.value;
          }
        } else if (m.kind === "obj") {
          obj[k] = m.value !== undefined ? m.value : {};
        } else {
          obj[k] = m.value;
        }
      }
    }
    if (mod === "nsgo" && name === "UrlScheme" && "url" in obj) {
      obj.url = getEnvAccessor("urlSchemeUrl", envSnapshot.urlSchemeUrl ?? null);
    }
    return obj;
  }

  function buildNSGoRT() {
    return {
      list: makeFaceComponent("nsgo:rt", "list", null),
      setup: (opts) => emit({ op: "rt", method: "setup", args: [encodeVal(opts)] }),
      registerFont: (b64) => emit({ op: "rt", method: "registerFont", args: [_String(b64)] }),
      registerAsset: (u, d) => emit({ op: "rt", method: "registerAsset", args: [_String(u), _String(d)] }),
      element: (tag) => createElementNode(_String(tag)),
      text: (s) => createRawNode(s),
      append: (p, c) => {
        if (!isVNode(p)) throw new _TypeError("nsgo: not a node");
        if (isVNode(c)) insertBeforeVNode(p, c, null);
        else insertValue(p, c, null);
      },
      insert: (p, v, m) => {
        if (!isVNode(p)) throw new _TypeError("nsgo: not a node");
        insertValue(p, v, isVNode(m) ? m : null);
      },
      prop: (n, k, v) => {
        if (!isVNode(n)) throw new _TypeError("nsgo: not a node");
        setElementProp(n, _String(k), v);
      },
      bindProp: (n, k, fn) => {
        if (!isVNode(n)) throw new _TypeError("nsgo: not a node");
        createRenderEffect(() => setElementProp(n, _String(k), fn()));
      },
      component: (Comp, props) => runUntracked(() => Comp(props)),
      cond: condHelper,
      each: eachHelper,
      ref: (target, value, name) => applyRefValue(target, value, name ? _String(name) : ""),
      spread: (n, props) => {
        if (!isVNode(n)) throw new _TypeError("nsgo: not a node");
        spreadProps(n, props, "");
      },
      merge: mergeFn,
      keepContext: (kept) => kept,
      loadChunk: () => _Promise.reject(new _TypeError("nsgo: split chunks are not supported in sandboxed packages")),
    };
  }

  function buildNSGoBuiltin(name, entry) {
    switch (name) {
      case "signal":
        return createSignal;
      case "computed":
        return createComputed;
      case "effect":
        return createEffect;
      case "batch":
        return runBatch;
      case "untracked":
        return runUntracked;
      case "cleanup":
        return registerCleanup;
      case "context":
        return createContext;
      case "ErrorBoundary":
        return ErrorBoundary;
      case "split":
        return splitFn;
      case "resource":
        return createResource;
      case "mount":
        return function (fn) {
          if (config && config.exportView) return;
          const owner = createOwner(currentOwner, true);
          runWithOwner(owner, () => {
            insertValue(rootVNode, fn(), null);
          });
        };
      case "View":
      case "Text":
      case "Scroll":
      case "Overlay":
      case "WebView":
      case "Video":
        return hostComponent(HOST_TAGS[name]);
      case "Image": {
        const img = hostComponent("image");
        if (entry && _Array.isArray(entry.statics)) {
          for (const s of entry.statics) {
            img[s] = makeMethod("nsgo", "Image." + s);
          }
        }
        return img;
      }
      case "windowSize":
      case "colorScheme":
      case "fontScale":
      case "pixelRatio":
      case "locale":
      case "reduceMotion":
      case "increaseContrast":
      case "lifecycle":
      case "screenReaderEnabled":
      case "safeArea":
      case "keyboardFrame":
        return getEnvAccessor(name, envSnapshot[name]);
      case "safeAreaInsets":
        return () => getEnvAccessor("safeArea", envSnapshot.safeArea);
      case "platform": {
        const p = envSnapshot.platform || { os: "web", version: "", idiom: "desktop", catalyst: false };
        return {
          os: p.os,
          version: p.version,
          idiom: p.idiom,
          catalyst: !!p.catalyst,
          select(spec) {
            if (!spec || typeof spec !== "object") return undefined;
            const keys = [p.os];
            if (p.os === "ios" || p.os === "android") keys.push("native");
            if (p.idiom === "desktop") keys.push("desktop");
            keys.push("default");
            for (const k of keys) {
              if (k in spec && spec[k] !== undefined) return spec[k];
            }
            return undefined;
          },
        };
      }
      case "appInfo":
        return () => _Object.assign({}, envSnapshot.appInfo || {});
      case "token":
        return function (key) {
          getEnvSignal("colorScheme", envSnapshot.colorScheme)();
          const k = _String(key).replace(/^--/, "");
          return themeTokens[k];
        };
      case "nativeModule":
        return function (spec) {
          const s = _String(spec);
          if (!config.knownModules || !config.knownModules.includes(s)) {
            throw new _ReferenceError('nativeModule: no module "' + s + '" is registered');
          }
          return guestRequire(s);
        };
      case "nativeComponent":
        return function (compName, spec) {
          const cn = _String(compName);
          if (!config.allowedNatives || (!config.allowedNatives.includes("*") && !config.allowedNatives.includes(cn))) {
            throw new _TypeError('nsgo: native component "' + cn + '" is not given to this sandbox (Sandbox.Modules)');
          }
          return makeFaceComponent("native:" + cn, cn, null);
        };
      case "List":
      case "Canvas":
      case "Shader":
      case "ShaderEffect":
      case "ShaderTransition":
      case "ScreenEffect":
      case "ScreenTransition":
        return makeFaceComponent("nsgo", name, entry ? entry.statics : null);
      default:
        return undefined;
    }
  }

  function buildModule(modName) {
    if (modName === "nsgo:rt") return buildNSGoRT();
    const spec = config.manifest[modName];
    const mod = {};
    for (const k of _Object.keys(spec)) {
      const entry = spec[k];
      if (modName === "nsgo") {
        const builtin = buildNSGoBuiltin(k, entry);
        if (builtin !== undefined) {
          mod[k] = builtin;
          continue;
        }
      }
      if (modName === "nsgo/base" && k === "Input") {
        mod[k] = hostComponent("input");
        continue;
      }
      if (modName === "nsgo/base" && k === "createNavStack") {
        mod[k] = function (routes) {
          const handleId = nextHandleId++;
          emit({
            op: "createHandle",
            id: handleId,
            mod: "nsgo/base",
            name: "createNavStack",
            args: [encodeVal(routes)],
            scopeId: currentHostScopeId(),
          });
          return makeNavHandle(handleId, routes);
        };
        continue;
      }
      if (modName === "nsgo/base" && k === "NavPage") {
        mod[k] = function (props) {
          const handleId = nextHandleId++;
          const keys = props && typeof props === "object" ? _Object.keys(props) : [];
          const getterKeys = [];
          for (const pk of keys) {
            const d = _Object.getOwnPropertyDescriptor(props, pk);
            if (d && typeof d.get === "function") getterKeys.push(pk);
          }
          emit({
            op: "navPage",
            id: handleId,
            keys,
            getterKeys,
            scopeId: currentHostScopeId(),
          });
          for (const pk of keys) {
            if (getterKeys.includes(pk)) {
              createRenderEffect(() => {
                emit({ op: "handleProp", id: handleId, key: pk, val: encodeVal(props[pk]) });
              });
            } else {
              emit({ op: "handleProp", id: handleId, key: pk, val: encodeVal(props[pk]) });
            }
          }
          return makeHostHandle(handleId);
        };
        continue;
      }
      switch (entry.kind) {
        case "component":
          mod[k] = makeFaceComponent(modName, k, entry.statics);
          break;
        case "class":
          mod[k] = makeClassProxy(modName, k, entry.statics);
          break;
        case "service":
          mod[k] = makeServiceObject(modName, k, entry);
          break;
        case "fn":
          mod[k] = makeMethod(modName, k);
          break;
        case "val":
          mod[k] = entry.value;
          break;
        default:
          mod[k] = entry.value;
      }
    }
    return mod;
  }

  function guestRequire(name) {
    const spec = _String(name);
    if (moduleCache.has(spec)) return moduleCache.get(spec);
    if (!config.manifest || !(spec in config.manifest)) {
      if (config.knownModules && config.knownModules.includes(spec)) {
        throw new _TypeError('nsgo: module "' + spec + '" is not given to this sandbox (Sandbox.Modules)');
      }
      throw new _TypeError('nsgo: no module "' + spec + '"');
    }
    const mod = buildModule(spec);
    moduleCache.set(spec, mod);
    return mod;
  }

  // ---- Message Loop ----

  let rootOwner = null;

  function syncClocks(msg) {
    if (typeof msg.nowMS === "number") hostNowMS = msg.nowMS;
    if (typeof msg.wallMS === "number") hostWallMS = msg.wallMS;
    if (typeof msg.ownerId === "number") activeOwnerId = msg.ownerId;
  }

  function onHostMessage(e) {
    if (closed) return;
    const msg = e.data;
    if (!msg || typeof msg !== "object") return;
    syncClocks(msg);

    switch (msg.type) {
      case "init": {
        config = msg.config;
        envSnapshot = config.env || {};
        themeTokens = envSnapshot.tokens || {};
        hostNowMS = config.nowMS || 0;
        hostWallMS = config.wallMS || Date.now();
        activeOwnerId = msg.ownerId || 0;

        lockdownGlobals();
        installRuntimeGlobals();

        try {
          const patchTime = _eval(config.timeSource);
          patchTime(() => hostWallMS);
          const installWeb = _eval(config.webSource);
          installWeb(globalThis, makeWebNatives());
          splitFn = _eval(config.splitSource);
          mergeFn = _eval(config.mergeSource);
          globalThis.require = guestRequire;

          if (config.harden && config.hardenSource) {
            const harden = _eval(config.hardenSource);
            harden(globalThis);
          }
        } catch (err) {
          _eval = null;
          post({ type: "loadError", reqId: msg.reqId, error: encodeError(err) });
          return;
        }

        const runScript = _eval;
        _eval = null;

        rootOwner = createOwner(null, false);
        try {
          runWithOwner(rootOwner, () => {
            runScript(config.source + "\n//# sourceURL=" + config.name + "\n");
          });
        } catch (err) {
          post({ type: "loadError", reqId: msg.reqId, error: encodeError(err) });
          return;
        }

        const exportsObj = globalThis.__nsgo_exports;
        const exportNames = [];
        if (exportsObj && typeof exportsObj === "object") {
          for (const k of _Object.keys(exportsObj)) {
            if (typeof exportsObj[k] === "function") exportNames.push(k);
          }
        }

        if (config.exportName) {
          if (!exportsObj || !(config.exportName in exportsObj) || exportsObj[config.exportName] === undefined) {
            post({ type: "loadError", reqId: msg.reqId, kind: "noViewExport", name: config.exportName });
            return;
          }
          const comp = exportsObj[config.exportName];
          if (typeof comp !== "function") {
            post({ type: "loadError", reqId: msg.reqId, kind: "notComponent", name: config.exportName });
            return;
          }
          const exportOwner = createOwner(rootOwner, true);
          try {
            runWithOwner(exportOwner, () => {
              const res = comp(config.exportProps || {});
              insertValue(rootVNode, res, null);
            });
          } catch (err) {
            post({ type: "loadError", reqId: msg.reqId, error: encodeError(err) });
            return;
          }
        }

        post({
          type: "loaded",
          reqId: msg.reqId,
          exports: exportNames,
          ops: takeOps(),
        });
        break;
      }

      case "invoke": {
        const fn = callbacks.get(msg.fnId);
        if (!fn) {
          post({ type: "reply", reqId: msg.reqId, ops: takeOps() });
          return;
        }
        try {
          const args = _Array.isArray(msg.args) ? msg.args.map(decodeVal) : [];
          const res = runWithOwner(rootOwner, () => fn(...args));
          post({
            type: "reply",
            reqId: msg.reqId,
            value: encodeVal(res),
            ops: takeOps(),
          });
        } catch (err) {
          post({
            type: "reply",
            reqId: msg.reqId,
            error: encodeError(err),
            ops: takeOps(),
          });
        }
        break;
      }

      case "call": {
        const exportsObj = globalThis.__nsgo_exports;
        const fn = exportsObj && typeof exportsObj === "object" ? exportsObj[msg.name] : undefined;
        if (typeof fn !== "function") {
          post({ type: "reply", reqId: msg.reqId, settled: true, noExport: true, ops: takeOps() });
          return;
        }
        let res;
        try {
          const args = _Array.isArray(msg.args) ? msg.args : [];
          res = runWithOwner(rootOwner, () => fn(...args));
        } catch (err) {
          post({
            type: "reply",
            reqId: msg.reqId,
            settled: true,
            error: encodeError(err),
            ops: takeOps(),
          });
          return;
        }
        if (res && typeof res.then === "function") {
          const callId = msg.callId;
          const callOwnerId = activeOwnerId;
          post({ type: "reply", reqId: msg.reqId, settled: false, ops: takeOps() });
          res.then(
            (v) => {
              const prev = activeOwnerId;
              activeOwnerId = callOwnerId;
              post({ type: "callResult", callId, value: cloneJSON(v), ops: takeOps(), ownerId: callOwnerId });
              activeOwnerId = prev;
            },
            (err) => {
              const prev = activeOwnerId;
              activeOwnerId = callOwnerId;
              post({ type: "callResult", callId, error: encodeError(err), ops: takeOps(), ownerId: callOwnerId });
              activeOwnerId = prev;
            }
          );
        } else {
          post({
            type: "reply",
            reqId: msg.reqId,
            settled: true,
            value: cloneJSON(res),
            ops: takeOps(),
          });
        }
        break;
      }

      case "timer": {
        const entry = timerCallbacks.get(msg.id);
        if (!entry) {
          post({ type: "reply", reqId: msg.reqId, ops: takeOps() });
          return;
        }
        if (!entry.repeat) {
          timerCallbacks.delete(msg.id);
          activeTimers = _Math.max(0, activeTimers - 1);
        }
        try {
          runWithOwner(rootOwner, () => entry.fn(...entry.args));
          post({ type: "reply", reqId: msg.reqId, ops: takeOps() });
        } catch (err) {
          post({ type: "reply", reqId: msg.reqId, error: encodeError(err), ops: takeOps() });
        }
        break;
      }

      case "raf": {
        const fn = rafCallbacks.get(msg.id);
        rafCallbacks.delete(msg.id);
        if (!fn) {
          post({ type: "reply", reqId: msg.reqId, ops: takeOps() });
          return;
        }
        activeRafs = _Math.max(0, activeRafs - 1);
        try {
          runWithOwner(rootOwner, () => fn(msg.ts));
          post({ type: "reply", reqId: msg.reqId, ops: takeOps() });
        } catch (err) {
          post({ type: "reply", reqId: msg.reqId, error: encodeError(err), ops: takeOps() });
        }
        break;
      }

      case "fetchResolve": {
        const entry = pendingFetches.get(msg.id);
        pendingFetches.delete(msg.id);
        if (entry) {
          const bodyBuf = msg.body instanceof _Uint8Array ? msg.body.buffer.slice(msg.body.byteOffset, msg.body.byteOffset + msg.body.byteLength) : new _ArrayBuffer(0);
          entry.done({
            status: msg.status,
            statusText: msg.statusText,
            url: msg.url,
            redirected: !!msg.redirected,
            headers: msg.headers || [],
            body: bodyBuf,
          });
        }
        break;
      }

      case "fetchReject": {
        const entry = pendingFetches.get(msg.id);
        pendingFetches.delete(msg.id);
        if (entry) {
          const e = msg.error || {};
          entry.fail(makeCodedError("TypeError", e.message || "fetch failed", e.code, e.permission, e.resource));
        }
        break;
      }

      case "rpcResolve": {
        const entry = pendingRpcs.get(msg.id);
        pendingRpcs.delete(msg.id);
        if (entry) entry.resolve(decodeVal(msg.value));
        break;
      }

      case "rpcReject": {
        const entry = pendingRpcs.get(msg.id);
        pendingRpcs.delete(msg.id);
        if (entry) {
          const e = msg.error || {};
          entry.reject(makeCodedError(e.name || "Error", e.message || "rpc failed", e.code, e.permission, e.resource));
        }
        break;
      }

      case "env": {
        applyEnvPatch(msg.patch);
        break;
      }

      case "dispose": {
        if (rootOwner) {
          disposeOwner(rootOwner);
          rootOwner = null;
        }
        post({ type: "reply", reqId: msg.reqId, ops: takeOps() });
        closed = true;
        try {
          port.close();
        } catch (_) {}
        break;
      }
    }
  }

  function onHandshake(e) {
    if (e.source !== _parent) return;
    if (!e.data || e.data.token !== initToken) return;
    if (!e.ports || e.ports.length !== 1) return;
    _removeEventListener("message", onHandshake);
    port = e.ports[0];
    port.onmessage = onHostMessage;
    port.start();
    post({ type: "ready" });
  }

  _addEventListener("message", onHandshake);
  _parent.postMessage({ token: initToken, hello: true }, "*");
})
