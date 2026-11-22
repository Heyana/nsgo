---
title: TSX apps
description: Write an app in TSX with Tailwind classes, compiled ahead with no Node.
group: Core concepts
order: 63
---

## A TSX app

A TSX app is a Go package that embeds its sources and runs its entry:

```go
//go:embed *.tsx nsgo.yaml
var src embed.FS

var Root = compiler.App(src, "App.tsx")

func main() { nsgo.Run(Root) }
```

```tsx
import { mount, signal, Text, View } from "nsgo";

function App() {
  const count = signal(0);
  return (
    <View class="gap-3 p-6" onPress={() => count.update((n) => n + 1)}>
      <Text class="text-lg font-bold">Pressed {count()} times</Text>
    </View>
  );
}

mount(App);
```

`go tool nsgo init -template tsx` writes one, and `examples/tsx` is another. `nsgo dev` and `nsgo build` compile the TSX ahead of time, so packages ship no compiler; `go run` and `go test` compile it as the app starts. No Node is needed either way.

## Components and modules

TSX imports the primitives from `nsgo`, `base`'s components from `nsgo/base`, game controllers from `nsgo/gamepad` (under the `gamepad` capability in signed packages) and `ui`'s from `nsgo/ui`, once the app imports `tsx/ui`; `examples/gallery-tsx` shows them.

For Mermaid diagrams, import `github.com/nsgo-dev/nsgo/tsx/diagram` for its effect in the Go host, alongside `tsx/textview` or `tsx/ui`. This adds `Diagram` to `nsgo/base` without linking the diagram parser into apps that do not use it. It takes `source`, `fontSize`, a partial `theme`, `accessibilityLabel` and the usual styling props. Use it directly or supply a TextView renderer:

```tsx
import { Diagram, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderDiagram={(code) => <Diagram source={code.code} errorClass="font-mono" />} />
```

The same `renderDiagram` prop works on `nsgo/ui`'s TextView. A non-Mermaid code block keeps its usual renderer; an invalid diagram shows its source.

For typeset math, import `github.com/nsgo-dev/nsgo/tsx/formula` for its effect in the Go host, alongside `tsx/textview` or `tsx/ui`. This adds `Formula` to `nsgo/base`; apps that omit the package keep the math engine and font out of their binary. It takes `source`, `dialect` (`tex` by default, or `typst`), `display`, `accessibilityLabel` and styling props. Font size and color follow the surrounding text. Use it directly or provide a TextView renderer:

```tsx
import { Formula, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderMath={(math) => <Formula source={math.tex} display={math.display} />} />
```

The same `renderMath` works on `nsgo/ui`'s TextView. Formulas retain their TeX when copied. The Go `Renderer` remains a host configuration.

`UrlScheme.url()` reads the tracked application address; `UrlScheme.setUrl(url, { history: "push" })` adds an address entry, or replaces it when options are omitted. `UrlScheme.on("change", fn)` observes browser back, forward and hash changes and returns an unsubscribe function. See [Application addresses](addresses.md) for routing, same-origin validation and deep-link hosting.

## Go from TSX

Register Go for TSX to import:

```go
tsx.Register("app/native", map[string]core.Node{"Badge": Badge{}})
tsx.RegisterFunctions("app/native", map[string]any{
	"greet": func(name string) string { return "Hello, " + name },
})
```

`import { Badge, greet } from "app/native"` then takes `Badge`'s fields as props and calls `greet`. Functions taking a `context.Context` become promises, and `tsx.RegisterClass` makes Go types classes TSX creates with `new`.

An optional prop forwarded as `undefined` uses its default. When registering a Go component with a getter whose default differs from its type's zero, add an option such as `tsx.Default("Max", float64(100))`; the same default applies while a reactive prop returns undefined, without replacing the component.

Data fields with documented Go defaults can be omitted too: a `Flow` node's `width` and `height` measure its JSX content when absent, an edge's omitted `width` uses the default stroke, and a quota row's omitted `pace` leaves its pace mark hidden.

Registering Go components and classes finds their exported fields and methods by reflection as the app runs. That lookup keeps the Go linker from dropping exported methods nothing calls, in the whole app and not only in what TSX registers, so linking `tsx` adds about 20 MB to a stripped desktop binary, more the more packages the app links.

## Signed packages

An app can run TSX it downloads, such as its own screens delivered anew or plugins, once it verifies the package. Declare what the package's code may reach in its `nsgo.yaml`; it is refused everything else:

```yaml
capabilities:
  network:
    hosts: [api.example.com, "*.cdn.example.com"]
  open_url: true # or { schemes: [https] }
  host: [search, "player.*"] # the app's handlers it may call
  # webview: { hosts: [...], input: true, media: [microphone, camera] }, clipboard, picker, speech, text_to_speech, status_bar, notifications, biometrics, sign_in_with_apple, purchases, secrets, audio, gamepad, auth_session
```

Keep a root key offline and certify release keys with it, then sign the app in that directory. `go tool nsgo keygen` prints a key pair and writes no file; run it once for the root and once for each release key. Public keys go in flags and the app's code; secret keys stay out of the repository and reach the tool only through `NSGO_ROOT_KEY` and `NSGO_RELEASE_KEY`:

```sh
go tool nsgo keygen # once for the root, once for the release key
NSGO_ROOT_KEY=<root secret> go tool nsgo certify -key <release public> -apps 'com.example.*'
NSGO_RELEASE_KEY=<release secret> go tool nsgo publish -cert release.cert plugin
go tool nsgo verify -root <root public> plugin/dist/Plugin-1.0.0.nsgopkg
```

The app trusts the root's public key, written in its code, verifies the package off the engine's goroutine and shows it in `tsx.View`, sandboxed beside its own views. The package reaches the app only through the host channel the view gives it, and keeps its own part of the app's storage and secrets:

```go
trust, err := pkg.OpenTrust(filepath.Join(cx.DataDir(), "trust"))
trust.AddRoot(root)
p, err := trust.Verify(data) // refuses tampered, revoked and older builds
channel := core.NewHost()
channel.Handle("search", func(c *core.HostCall) { c.Resolve(results(c.Data)) })
view := tsx.View{Package: p, Sandbox: tsx.Sandbox{Host: channel}}
core.Catch{Try: view.Render, Fallback: failed}
trust.Accept(p) // once it ran, older builds are refused
```

`Sandbox.Grant` gives it less than it declares, `Sandbox.Limits` other time budgets and quotas, and `Sandbox.Modules` Go functions and components beyond NSGo's own. Its keyboard shortcuts, back handlers and navigation stacks take effect only while the keyboard focus is in its view, and it moves the focus itself (`focus()`, `autoFocus`, `Keyboard.dismiss`, `setAccessibilityFocus`) only while the focus is already there or in answer to the user's press in it. It shows in a view of its own that fills the room its parent gives it, no more, and clips it, menus, popovers and toasts included; in a scroll view, put it in a view to let it take its content's height. A dialog, an alert, a context menu or an announcement of its shows only in the same way, and back closes a dialog of its even if it does not remove it. `View.Export` names a component the package's entry module exports, shown in place of the root the entry mounts, whose `mount` then neither renders nor runs its component, and given `View.Props`, a JSON-encodable map copied for each view, so one package provides several views, each with state of its own, under the same capabilities and limits; a missing export panics with `tsx.ErrNoViewExport` and one that is not a function with `tsx.ErrViewExportNotComponent`. A package whose entry exports functions is a plugin of functions: `tsx.Load(cx, p, tsx.Sandbox{})` runs it unseen and synchronously on the caller's goroutine, under a `cx` that belongs to a view tree (without one it returns `tsx.ErrNoTree`), or returns what its entry module threw as it loaded, and `Plugin.Call` calls them. `go tool nsgo dev -limits` runs an app as such a view would, and `go tool nsgo revoke` signs a list of revoked release keys and packages for `Trust.UpdateRevocations`. `examples/plugins` asks the user to grant a package's capabilities, installs it and runs it. `Limits.Memory`, 64 MiB by default, budgets metered native storage for arrays, strings, buffers, and typed arrays. This includes `concat`, `flat`, `slice`, array spread, string concatenation, templates, `replace`, `replaceAll`, `TextEncoder`, and `Blob` copies. `memory=<bytes>` in `-limits` changes the budget, and `memory=0` turns it off. Checks run before allocation. A package exceeding the budget stops with `ERR_NSH_QUOTA_EXCEEDED` and resource `memory` after Go has checked for collectible storage. Ordinary objects, closures, stacks, small allocations, and unmetered built-ins and host APIs can also allocate. The browser keeps its own heap. An app that signs packages itself with `pkg.Build`, or `go tool nsgo publish -extra <file>` with that file's bytes, can put metadata of its own in `pkg.Contents.Extra`, such as what a plugin adds to its menus: it is signed with the package, and `Verify` returns it unchanged as `p.Extra` for the app to parse, so the app reads it without running the package's code.

An app can sign its own JSON documents, such as a catalog of its plugins, with `pkg.SignDocument` and verify them with `Signed.Open`, with the keys `nsgo keygen` prints. Give each kind of document an `app:` context of its own with a version, as `app:catalog-v1`: a document never passes for another kind, nor for a package, certificate or revocation list. `Open` trusts exactly the public keys it is given, with no certificate or revocation list, and refuses body fields the type it decodes into lacks. The envelope holds no time or sequence, so an older document verifies as well as a newer one: put a version or expiry in the body and check it.
In a native sandbox, `JSON.stringify`, `JSON.parse` with a reviver, `Array.prototype.join`, `toString`, `toLocaleString` and `flat`, and so `String()` of an array, throw a catchable `RangeError` once they go more than 1,000 levels deep, counting one inside another, as an array's `toString` and the `join` it calls each count; deeper data would hang the call or end the app. The web uses the browser's built-ins.

On native targets, `DefaultLimits.Stack` is 1,000 frames. Set `Limits.Stack` or `-limits stack=<frames>` to choose a different depth up to 10,000 frames. Go values at or below zero and inputs above the ceiling use 10,000. The web uses the browser's stack limit.

Native functions calling each other count against `Limits.Stack` as JS calls do: a getter, `toString` or `valueOf` that is a built-in, a regular expression whose `exec` is another of its methods, `Reflect.apply` and `Reflect.construct` within each other, and chains of bound functions or proxies throw a catchable `RangeError` past it rather than ending the app, and a plugin's value nested deeper than it fails the call that converts it to Go with a `RangeError`. This comes from the fork of goja NSGo requires, `github.com/nsgo-dev/goja`, which your app gets with NSGo without any change to its `go.mod`.

On native targets, `Limits.Call` bounds each slice of JavaScript, including timer callbacks and promise continuations after `await`. When a slice is cut, a pending `Plugin.Call` reports `tsx.ErrBudget` once, stops its remaining timers and ignores late host answers. Independent calls keep running, including promise reactions queued in the interrupted slice. A call that settles another's promise still reports its own result or error. Backtracking regular expressions use the shorter load or call budget as their match timeout; an unhandled timeout also becomes `tsx.ErrBudget`.

`Limits.CallTotal` adds a wall-clock deadline for each plugin call and sandboxed view event. It includes time spent waiting and holds its timers, intervals, frames, promise continuations and host answers even after the call settles. At the deadline the pending work is dropped and a still-waiting call fails with `tsx.ErrBudget`. The entry module's work is held to `Limits.Load`, using `CallTotal` if `Load` is zero. Standalone host events use `CallTotal` from delivery. Use `Plugin.CallWithBudget` to get a handle to a waiting call's deadline. In a host's `Scope.Post`, call `budget.Extend(duration)` before that deadline to give only this call more time.

`Plugin.Interrupt` stops the runtime from any goroutine, drops its pending timers and fails current and later calls. `Plugin.Stopped()` and `Plugin.Err()` can also be read from any goroutine. `Err()` is nil while the runtime runs and returns its first stop reason once stopped. Closing the plugin or disposing its owner normally runs its cleanups and stops it with `tsx.ErrPluginClosed`. Set `Sandbox.OnStop` when calling `tsx.Load` to receive that reason once on the engine's goroutine, after waiting calls fail.

Set `Sandbox.OnError` when loading a function plugin to receive uncaught timer, animation-frame, `queueMicrotask` and `reportError` failures outside a waiting `Plugin.Call`. Each plugin’s handler runs on the engine goroutine, so a host can attribute failures and call `Plugin.Interrupt` when its breaker opens. Native slice-budget cuts also report `tsx.ErrBudget`, including a promise job whose call already settled. Ordinary callback errors leave the plugin running. Load and call-result errors still return to their callers, and views still use `core.Catch`; without a handler, existing boundary and app-report behavior remains.

Set `Sandbox.OnViewRuntime` to receive each view's `*tsx.ViewRuntime` on the engine goroutine before its entry module runs. Call `Interrupt` from any goroutine to stop its work and leave the last rendered view in place. `Stopped` and `Err` report its terminal state, with `tsx.ErrViewClosed` on normal disposal. `Sandbox.OnStop` also reports a view's terminal reason once.

`Plugin.JSTime` and `ViewRuntime.JSTime` report the elapsed time spent in the native runtime's JavaScript entries, including the current entry. Waiting between entries and time inside registered synchronous Go functions are excluded. `ViewRuntime.EngineTime` also includes synchronous Go calls and owned reactive, mounting, commit and layout work, including work still running. It excludes waiting between engine entries, work belonging to the host or another view, shared commit bookkeeping and native drawing. Both counters are safe to read from any goroutine. The browser's JavaScript turns outside NSGo cannot be measured or preempted. There both counters report unavailable and `Interrupt` stops future NSGo callbacks when the host regains control.

The web cannot isolate a package from the page yet: its code would reach the page's own network, clipboard and storage, the trust store's included, past its capabilities. There `tsx.View` and `tsx.Load` refuse a package with `tsx.ErrUnisolated`, unless you set `Sandbox.WebUnisolated` to run it with the page's authority, which suits only packages you would trust with the app's own.

## Editors and tests

A `tsconfig.json` extending `.nsgo/tsconfig.json` gives editors the TSX's types, which `go tool nsgo dev` and `go tool nsgo check` write. `*.test.tsx` files beside the app, written with `nsgo/test`, run with its Go tests ([Testing](testing.md)).
