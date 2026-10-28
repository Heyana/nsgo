# TSX and dynamic delivery

NSGo apps are Go today. This subject plans NSH's main feature for NSGo: screens written in TSX, and TSX modules a host app downloads, verifies and runs in a sandbox it configures. Nothing here exists yet; the [roadmap](../roadmap.md#tsx-and-dynamic-delivery) orders the steps, and the [open questions](../../ARCHITECTURE.md#open-questions) list what waits for a decision.

## How NSH does it

| Part | NSH | Reference |
| --- | --- | --- |
| Compile | Oxc strips types, NSH's JSX compiler turns JSX into calls to a native renderer, rolldown bundles one ES module holding only the app's code; imports of NSH's modules stay imports | `native-pipeline.md`, `nsh/crates/nsh-bundler/src/jsx.rs` |
| Run | One QuickJS VM per surface. The reactive graph, the renderer, every module and the Rust components' JS face are native; JSX pushes ops into a queue the core commits once per turn | `architecture.md`, `js-modules.md`, `nsh/crates/nsh-js/src/dom.rs` |
| Types | `.nsh/types` generated from the Rust specs; `nsh check` runs TypeScript 7's native `tsc` | `tooling.md` (TypeScript) |
| Develop | The dev server sends a patch of the changed modules; the runtime runs them again, keeping signals and mounts | `tooling.md` (Dev server) |
| Deliver | `.nshpkg`: a JSON header, the source, bytecode, a source map and assets, signed with Ed25519 by a release key a root key certified; revocation lists; rollback protection | `signed-packages.md`, `nsh/crates/nsh-package/src/lib.rs` |
| Sandbox | Heap, stack and time limits per VM; no `eval` or `Function`; capabilities the app declares and the host narrows, checked in Rust | `sandbox.md` |
| Compatibility | A package is never rebuilt, so nothing a release shipped is removed or renamed | `compatibility.md` |

References are in `nsh/docs/dev/` unless they start with `nsh/`.

## Design

TSX builds the element values a Go app writes, so `core` runs a TSX screen as it runs a Go one: components run once, signals drive bindings, regions rebuild their span.

```text
App.tsx ─ nsgo build ─▶ esbuild: types stripped, JSX → __jsx(tag, props, ...children)
                        lowering: dynamic props → getters, dynamic children → thunks
                        esbuild: one script; imports of nsgo modules → require
        ─▶ bundle.js and its source map ─ nsgo publish ─▶ app.nsgopkg, signed

host app ─ tsx: a goja VM per engine ─ __jsx builds core and base values ─▶ core, as for Go
```

- A package `tsx` holds the runtime and imports `core`, `base`, `mobile` and `desktop`, so only apps that run TSX link goja. Its API names no goja type, so the engine can change.
- One VM per engine, on the engine goroutine. JS reaches the graph through the root scope; `core` already gives what JS creates to the innermost running owner.
- Everything is Go memory, which Go's collector sees whole. NSH makes nodes and signals QuickJS objects of native classes so QuickJS's collector sees their cycles; here nothing is needed.

## The engine

goja runs the JS. The alternatives fail a [principle](../../AGENTS.md#principles) or a target:

| Engine | Verdict |
| --- | --- |
| goja (pure Go) | Builds for every target, `js/wasm` included: ES2022 classes and private fields, async/await, generators, `for await`, Proxy, BigInt. On an M1 Max a Go function called from JS costs about 110 ns, a JS loop step 7 ns, compiling 100 KB of JS 2.6 ms |
| QuickJS through cgo | cgo and hand-written C on every target |
| `modernc.org/quickjs` (QuickJS translated to Go) | Linux only |
| QuickJS in WebAssembly on wazero (`fastschema/qjs`) | iOS forbids generated code, so wazero interprets an interpreter there |
| QuickJS through `wasm2go` | Young (v0.4), and QuickJS's API needs a C shim |
| The system's (JavaScriptCore, the browser's) | A bridge per platform, and Android's runs only out of process |

What goja lacks, and the answer:

- **ES modules**: esbuild bundles to one script, and native imports become `require` of modules the runtime makes.
- **Bytecode**: none to save, so bundles and packages carry source, compiled as they load. NSH's rule that bytecode is trusted input has nothing to guard.
- **A heap limit**: JS allocates from the process's heap. Time (`Runtime.Interrupt`), stack (`SetMaxCallStackSize`) and NSGo's quotas are enforceable; memory is not ([sandbox](#sandbox)).
- **Speed**: slower than QuickJS on pure JS. NSH's first performance rule holds: only the app's code is JS; the graph, layout, text and components are Go.

## Compiling TSX

`nsgo` compiles with esbuild's Go API (`github.com/evanw/esbuild/pkg/api`), so no Node is needed. esbuild strips TypeScript, applies React's JSX whitespace rules (NSH's), lowers syntax to what goja runs, bundles, minifies and writes source maps. Its classic JSX transform makes `<View a={x()}>{y.z}</View>` into `__jsx(View, { a: x() }, y.z)`, evaluated at once, which loses reactivity.

A lowering pass between esbuild's transform and its bundling restores NSH's semantics. It parses the transform's output with goja's own parser, so it reads exactly the syntax the runtime runs, and inserts text at the nodes' offsets:

- A part is dynamic when it calls, constructs or reads a member outside nested functions, as NSH's `is_dynamic`. A nested `__jsx` call and a member of a module namespace (`import_nsgo.View`) do not count.
- A dynamic prop becomes a getter (`get a() { return x(); }`) and a dynamic child a thunk (`() => y.z`). Static parts stay values.
- A spread becomes `merge(...)`, which keeps getters as Solid's `mergeProps` does; an object spread would read them once.
- A conditional child (`c() ? <A/> : <B/>`, `c() && <A/>`) becomes a region whose condition is a memo, so the branch is built again only when its truth changes.
- `ref` takes a signal or a function. NSH also assigns a `let` variable, which needs scope analysis the pass does not have.

The pass only inserts text, so it shifts columns on the lines it touches; it updates the transform's source map to match, keeping stacks pointed at `.tsx` lines.

## The runtime

`__jsx(tag, props, ...children)` returns a `core.Node`:

| Tag | Becomes |
| --- | --- |
| A host element or Go component (`View`, `Text`, `base.Button`, the app's) | Its struct, filled by reflection: prop `label` sets `Label`; a getter sets `LabelFunc`, or a field typed `func() T` such as `Disabled` |
| A JS function | `core.Func` calling it untracked with the props object, `children` a getter |
| `If`, `Each`, `ErrorBoundary`, a fragment | `core.Show`, `core.Each`, `core.Catch`, `core.Group` |
| A thunk child | `core.Dynamic`; inside a `Text`, its `ContentFunc` or `SpansFunc` |

- Fields map by name, lowerCamelCase in JS. Event fields wrap the JS function and convert the event struct to an object. `core.Node` fields take elements, and `func(T) core.Node` fields render functions.
- A getter for a field with no function form builds the element again in a `core.Dynamic` when it changes, losing native state; a dev build warns once, naming the field to give a `Func` form.
- `Each` keys rows by the `key` prop or the item, and compares items with `===`. `core.Each` notifies rows of non-plain items on every read, so it needs an equality, as `NewSignalFunc` takes.
- One registration per Go type makes its JS face, so `base`, `mobile`, `desktop` and an app's components need no code each. Modules: `nsgo`, `nsgo/base`, `nsgo/mobile`, `nsgo/desktop`, and the app's (`app/native`).
- `signal` is a callable over `core.Signal[goja.Value]` comparing with `===`, with `set` and `update`; `computed`, `effect`, `batch`, `untracked`, `cleanup` and `context` map to `core`'s memos, effects, `Untrack`, `Cleanup` and `Key`.
- The entry calls `mount(() => <App />)`, which gives `tsx.Run` or `tsx.View` the root.
- `style` objects decode into `core.Style`: lowerCamelCase fields, colors through `core.ParseColor`, numbers as points, strings such as `"50%"` as lengths. Tailwind classes come later, as a port of `nsh/crates/nsh-core/src/tailwind/` (about 6,900 lines of Rust) to `core.Style`.
- An exception is a panic `core` recovers: a `Catch` above shows its fallback, else the host reports it with the stack mapped to `.tsx` lines.
- goja runs promise jobs as the outermost call returns; timers go through `Scope.AfterFunc`.
- `host` reaches the embedding app through the [host channel](embedding.md#the-host-channel), so a delivered screen gets the app's data from functions the app names. `fetch` runs on `net/http`, the network capability checked first.

## Types and checking

`nsgo` writes `.nsgo/types` from the runtime's reflection, with doc comments `go/doc` reads from NSGo's sources in the module cache, and a `tsconfig.json` mapping the modules there. `nsgo check` runs TypeScript 7's checker built from its Go sources: `go run github.com/microsoft/typescript-go/cmd/tsgo@<pinned>` builds once (about 70 s) and is cached, so checking needs only Go.

## Developing

A TSX entry in `nsgo.yaml` makes `nsgo dev` build the Go host once, with a generated `main` calling `tsx.Run`, then only the bundle on each save, which esbuild does in milliseconds. The dev server sends it over `/_nsgo/events`, and the app starts a new VM with it; failed builds use the existing overlay. Keeping state across an edit, as NSH's hot runtime does (`nsh/crates/nsh-bundler/src/hot.rs`), is a later step.

## Delivering

- A host places `tsx.View` where a feature goes, given a verified package or a bundle of its own; `tsx.Run` is an app made only of TSX. The iOS and Android faces gain the same loading call.
- `.nsgopkg` follows `.nshpkg`: a header (app ID, version, build, capabilities, minimum runtime, signing time, certificate, file table with SHA-256s), the files (source, source map, assets) and an Ed25519 signature over both.
- Keys, certificates, revocation lists with sequence numbers, the trust store and rollback protection port as NSH has them, on `crypto/ed25519` and `crypto/sha256`. A host verifies off the main thread and accepts a build once it ran healthy.
- `nsgo keygen`, `certify`, `publish`, `verify` and `revoke` are NSH's commands. Where packages come from is the host's business, as in NSH.
- A package runs on every later runtime, so once TSX ships every name its face exposes is frozen. The face comes from Go's field names, so renaming a field of `core` or `base` then needs an alias kept for good.

## Sandbox

Limits and checks are Go's, never JS's, since a bundle holds only the app's code.

- A Go app and its Go components are native code and trusted; only a TSX module is sandboxed.
- Limits per VM, which only an embedding host sets, as in NSH: a time budget per call into JS (10 s to load, 2 s per call) cut with `Runtime.Interrupt`, a stack depth, and quotas on nodes, timers, host calls and requests.
- Without a heap limit a module can still exhaust memory. That suits the app's own screens; a store of untrusted modules needs an engine with one, which the `tsx` API leaves room for.
- Outside development, `eval` is deleted, the `Function` constructors throw and the shared prototypes are frozen.
- Capabilities are declared under `capabilities:` in `nsgo.yaml` and carried in the package, with NSH's keys: `network`, `webview`, `openUrl`, `clipboard`, `picker`, `host`, `statusBar`.
- The host may grant less, and a request needs both. Go checks them where each request is made: `fetch`, remote images, web views, `OpenURL`, host calls.

## Store policies

- Apple's guideline 2.5.2 forbids downloaded code that changes an app's features, while its developer agreement allows interpreted code that keeps the app's purpose.
- Apple's guideline 4.7 allows JavaScript mini apps under its rules: an index of them, and no native API exposed to them without Apple's permission.
- Google Play forbids downloaded executable code, except code run in a virtual machine or interpreter with indirect access to Android's APIs.
- Updating an app's own screens fits both stores. A store of third-party modules owes 4.7's rules, and capabilities keep its modules from what the host did not grant.

## Where this departs from NSH

| NSH | NSGo | Why |
| --- | --- | --- |
| QuickJS | goja | No C and no cgo, on every target |
| Oxc, NSH's JSX compiler, rolldown | esbuild and a lowering pass on goja's parser | Go libraries, no Node |
| A renderer of ops | `__jsx` builds `core`'s element values | Go and JS call each other as Go calls, and `core` already binds what changes |
| Bytecode in packages | Source only | goja saves no bytecode |
| A heap limit | Time, stack and quotas | goja allocates from Go's heap |
| `ref` assigns variables | Signals and functions | No scope analysis |
| `nsh check` downloads `tsc` from npm | `go run` builds it | Only Go is assumed |
| Tailwind classes first | `style` objects first | The Tailwind port is large |
| NSH's TSX names | Names from NSGo's Go fields | One API, generated rather than mapped; an open question |
