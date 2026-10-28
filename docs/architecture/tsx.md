# TSX and dynamic delivery

NSGo runs TSX apps on NSH's design: they import `nsgo`, `nsgo/base`, `nsgo/mobile` and `nsgo/desktop`, have NSH's props, events, style keys, Tailwind classes and behavior, and compile with `tsx/compiler`. Each part is a Go port of its NSH counterpart, so what NSH adds ports the same way (decided by the user, 2026-10-28); an NSH app runs once its imports name `nsgo` in place of `nsh`. The [roadmap](../roadmap.md#tsx-and-dynamic-delivery) orders what is left.

An app embeds its sources and compiles them as it starts, so `go run`, `go test` and `nsgo dev`, which watches embedded files, need nothing else:

```go
//go:embed *.tsx *.ts
var src embed.FS

func main() { nsgo.Run(compiler.App(src, "App.tsx")) }
```

`examples/tsx` is such an app: its modules, components, signals, inputs, lists and timers, styled with classes in light and dark, one column on phones and two from `md:`.

## The ports

| NSH | NSGo | Part |
| --- | --- | --- |
| `nsh-bundler`: Oxc, `jsx.rs`, rolldown | `tsx/compiler`: esbuild and `jsx.go` | TSX to one script of the app's code |
| `nsh-js` `reactive.rs` | `tsx/reactive.go`, on `core`'s graph | `signal`, `computed`, `effect`, `batch`, `untracked`, `cleanup` |
| `nsh-js` `dom.rs` | `tsx/dom.go`, `tsx/components.go` | `nsgo:rt`, what compiled JSX calls; `mount`, `context`, `ErrorBoundary`, the primitives as components |
| `nsh-js` `protocol.rs` | `tsx/protocol.go` | Props and style objects decoded into `core`'s values |
| `nsh-js` `env.rs` | `tsx/env.go` | The environment's accessors and `platform` |
| `nsh-js` `modules.rs` | `tsx/modules.go` | The modules and their exports |
| `nsh-testing` | `nsgotest` | Headless and platform tests of a TSX app, as of a Go one |
| `nsh-js` `components.rs` | `tsx` (planned) | The JS face of Go components |
| `nsh-core`'s `Op` | `core.Element` | Elements built one change at a time |
| `nsh-core` `tailwind/`, `core/classes.rs` | `core/tailwind*.go`, `core/classes.go` | Tailwind classes ([styles](styles.md#classes)) |
| `nsh-package`, `sandbox.rs` | `tsx` (planned) | Signed packages and limits |

Read the NSH file before changing its port, and keep the port's names and order where Go allows, so NSH's next change maps onto it.

## The engine

goja runs the JS: pure Go, so it builds for every target, `js/wasm` included. The alternatives fail a [principle](../../AGENTS.md#principles) or a target:

| Engine | Verdict |
| --- | --- |
| QuickJS through cgo, NSH's | cgo and C on every target |
| `modernc.org/quickjs` (QuickJS translated to Go) | Linux only |
| QuickJS in WebAssembly on wazero | iOS forbids generated code, so wazero interprets an interpreter there |
| The system's (JavaScriptCore, the browser's) | A bridge per platform; Android's runs only out of process |

What goja lacks, and the answer:

- **ES modules**: the compiler bundles one script, and imports of `nsgo` modules become `require` of the modules the runtime makes.
- **Bytecode**: none to save, so bundles and packages carry source, compiled as they load.
- **A heap limit**: JS allocates from Go's heap. Time (`Runtime.Interrupt`), stack (`SetMaxCallStackSize`) and NSH's quotas are enforceable; memory is not ([sandbox](#sandbox)).
- **Speed**: slower than QuickJS on pure JS, so NSH's first performance rule matters more: only the app's code is JS.

## Compiling

`tsx/compiler` compiles with esbuild's Go API, so no Node is needed:

- Each module goes through esbuild's transform: TypeScript stripped, syntax lowered to ES2020, JSX in its classic form (`__jsx(tag, props, ...children)`, with React's whitespace rules, NSH's), as CommonJS, since goja's parser reads no ES modules.
- `jsx.go`, a port of `jsx.rs`, reads that with goja's parser and writes each call as NSH's compiler does: host elements made in place (`element`, `prop`, `bindProp`, `append`, `insert`), components called with getters for what changes, `If` and `Each` as `cond` and `each`. The expressions are copied as they are; the chunks keep where they came from, and its source map is composed with esbuild's.
- Tags and refs resolve through the module's scopes (`scope.go`), TypeScript's `declare`d names among them, as Oxc's scopes resolve them for NSH. The names NSH's source writes directly are module members in CommonJS (`import_nsh.View`); they count as names, not member reads.
- esbuild then bundles the modules from an `fs.FS` into one script with its source map inline, which goja reads, so stacks point at `.tsx` lines. Imports of `nsgo`, `nsgo/*` and `nsgo:rt` stay `require` calls the runtime answers; an app imports its own files and JSON, not npm packages yet.
- NSH's compiler tests are its tests, on this output (`jsx_test.go`), with its errors: lowercase tags, unknown tags with the import to write, text outside `<Text>`.

## The runtime

`tsx.App(name, bundle)` returns the app function `nsgo.Run` and `nsgotest.Start` take: it runs the bundle in a goja VM on the engine goroutine, and what `mount` renders is the app. JS reaches the graph through the root scope; `core` gives what JS creates to the innermost running owner, as NSH's graph does.

It runs `View`, `Text`, `Scroll`, `Image` and `nsgo/base`'s `Input` with their common props, `class`, `classList`, `style` and the state styles (`activeStyle`, ...), and `nsgo`'s reactive API, `mount`, `context`, `split`, `ErrorBoundary`, the environment and the primitives as components. Its tests and `examples/tsx`'s run apps headless; `TestControlFlow` expects the texts NSH showed for the same app.

- A node is a JS object over a `core.Element`. Host elements are made detached, their props set one at a time, and dynamic children kept by a render effect reconciling the nodes it inserted with Solid's `reconcileArrays`, as `dom.rs` does.
- Props decode into `core`'s element structs (`protocol.go`, NSH's `decode_prop`): an NSH prop sets the field of the same meaning. A prop `core` lacks is added to `core`, so Go apps gain it too; one not decoded yet is logged once.
- Style objects decode into `core.Style`: NSH's keys, colors through `core.ParseColor`, numbers as points. Classes go to `core` as they are, which resolves them.
- `core.Text` takes spans, not children, so the runtime merges a `<Text>`'s raw text and nested `<Text>`s, with their styles and classes, into its spans, and makes other elements attachments, as NSH's core merges text.
- A node is released with the owner it was made under. NSH releases one when QuickJS collects it; Go's collector cannot see that only the tree's handler closures still hold it.
- An exception is a panic `core` recovers: an `ErrorBoundary` above shows its fallback, which gets what was thrown, else the host reports it with the stack mapped to `.tsx` lines.
- goja runs promise jobs as the outermost call returns; `setTimeout`, `setInterval` and `queueMicrotask` run on the engine goroutine through `Scope.AfterFunc`, and `console` writes to the log (`timers.go`).
- `host` reaches the embedding app through the [host channel](embedding.md#the-host-channel); `fetch` runs on `net/http`, the network capability checked first (planned).

## Core's part

`core.Element` is nsh-core's `Op` entry on `core`'s tree, the part of `core` only `tsx` uses:

- `NewElement` makes one detached from an element value (`View`, `Text`, `Scroll`, `Input`, `Image`), its props, whose functions it reads; `Set` replaces them, `Insert` and `Remove` place children, and `Release` frees it.
- An element is a `Node`: mounted where Go code puts a node, it shows there, so Go components take TSX children.
- A region element (`NewElement` with no value) lays its children out as its parent's, and `Mount` shows Go nodes in it, so TSX shows Go components and `ErrorBoundary` is a `Catch`.
- `Scope.RenderEffect` runs bindings as `core`'s own do, and `Scope.Run` runs code under an owner, as Solid's `runWithOwner`.

## Types and checking (planned)

`nsgo` writes `.nsgo/types` from the runtime's tables, as NSH's `declarations.rs` does, with a `tsconfig.json` mapping the modules there. `nsgo check` runs TypeScript 7's checker built from its Go sources: `go run github.com/microsoft/typescript-go/cmd/tsgo@<pinned>` builds once (about 70 s) and is cached, so checking needs only Go.

## Developing

`nsgo dev` watches the files a package embeds, so a save rebuilds and restarts a TSX app as it does a Go one. Rebuilding only the bundle (planned): the Go host built once, then the bundle on each save, which esbuild does in milliseconds. The dev server sends it over `/_nsgo/events`, and the app starts a new VM with it; failed builds use the existing overlay. Keeping state across an edit, as NSH's hot runtime does (`nsh/crates/nsh-bundler/src/hot.rs`), is a later step.

## Delivering (planned)

- A host places `tsx.View` where a feature goes, given a verified package or a bundle of its own. The iOS and Android faces gain the same loading call.
- Packages follow `.nshpkg`: a header (app ID, version, build, capabilities, minimum runtime, signing time, certificate, file table with SHA-256s), the files (source, source map, assets) and an Ed25519 signature over both.
- Keys, certificates, revocation lists with sequence numbers, the trust store and rollback protection port as NSH has them, on `crypto/ed25519` and `crypto/sha256`. A host verifies off the main thread and accepts a build once it ran healthy.
- `nsgo keygen`, `certify`, `publish`, `verify` and `revoke` are NSH's commands. Where packages come from is the host's business, as in NSH.
- A package runs on every later runtime, so once TSX ships nothing its modules expose is removed or renamed, as NSH's `compatibility.md` rules.

## Sandbox (planned)

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
| Oxc, `jsx.rs`, rolldown | esbuild and a port of `jsx.rs` on goja's parser, over CommonJS | Go libraries, no Node; goja's parser reads no ES modules |
| `nsh build` writes the bundle | The app compiles its embedded sources as it starts | Plain `go run` and `go test`; `nsgo build` compiling ahead is planned |
| Bytecode in packages | Source only | goja saves no bytecode |
| A heap limit | Time, stack and quotas | goja allocates from Go's heap |
| Nodes released when collected | Released with their owner | Go's collector cannot tell the tree's handler closures from the app's references |
| The core merges a text's children | The runtime merges them into `core.Text`'s spans | `core.Text` takes spans |
| `nsh.json` | `nsgo.yaml` | Configuration is YAML |
| `nsh check` downloads `tsc` from npm | `go run` builds it | Only Go is assumed |
| The modules `nsh`, `nsh/base`, `nsh:rt` | `nsgo`, `nsgo/base`, `nsgo:rt` | The product's name (decided by the user, 2026-10-28) |
