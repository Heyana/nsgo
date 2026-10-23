# Roadmap

This plan lists what NSGo still lacks compared with NSH's `main` at `b2af2f21`, in the order to build it. Each item names its NSH reference and the result that finishes it. What exists today is in [ARCHITECTURE.md](../ARCHITECTURE.md); design questions that block an item are in its [Open questions](../ARCHITECTURE.md#open-questions).

References are files in `nsh/docs/dev/` unless they name a crate, with the bullet or section to read in brackets. To refresh the plan, read `git log b2af2f21..` in `nsh/` and the documents it changed.

## Order

Sections run in build order: what apps build on first, then components, services, the tool, embedding and performance. The last section waits for community contributions, and nothing above it depends on it.

## 1. Input and interaction

| Item | NSH reference | Done when |
| --- | --- | --- |
| Pointer events | `elements.md` (interaction and the environment) | `OnHoverMove` and `OnMouseMove` with positions and modifiers; presses and context menus report their point |
| Touch handling | `elements.md`, `components.md` (touches and the responder) | `PointerEvents`, `HitSlop`, `DelayLongPress`, raw touches and the responder, touches on children outside their parent |
| Gesture values | `animation.md` | Pinch and rotation drive animated values as `PanValues` does; pan start and end report the bound values |
| Keyboard focus | `macos.md` (keyboard focus for pressable views) | Pressables are Tab stops pressed by Return and Space, opt out with `Focusable` and show `Style.Focus`; focus returns when a modal closes |
| Back and keys | `components.md`, `crates/nsh-core/src/keymap.rs` | Any owner can take back before stacks; key bindings take `when` and `repeat` and give labels such as ⇧⌘L |

## 2. Styles and layout

| Item | NSH reference | Done when |
| --- | --- | --- |
| Shapes and borders | `styles.md` (view styles) | Per-corner and start/end radii, continuous corners, dashed, dotted, none and hidden borders |
| Shadows and outlines | `styles.md` (effects, `currentColor`) | Several and inset shadows, rings, outlines, borders in the text color |
| Backgrounds | `styles.md` (effects) | Linear, radial, conic and repeating gradients on views, mixed in Oklab, replaced by a state's background color |
| Filters and materials | `styles.md` (effects), `elements.md` | Filters, backdrop blur with iOS materials, Liquid Glass on iOS 26, `MaskImage` gradients, blend modes |
| Transforms | `styles.md` (view styles, `perspective-*`) | rotateX and rotateY, skew, perspective, transform origins, backface visibility, translation by a fraction of the size |
| Positioning | `styles.md` (`position`, `position: "sticky"`) | `static`, `fixed` in its root, and `sticky` pinned in its scroll view or list |
| Sizing and inheritance | `styles.md` (layout values, line heights) | Min-, max- and fit-content sizes, line heights as multipliers and em, inherited white space and selectability, `zIndex`, visibility, cursors |
| Direction | `elements.md` (right to left) | A subtree overrides the direction; start and end style fields; writing direction |
| Group and keyframe styles | `components.md` (styles), `styles.md` (effects) | Children style by an ancestor's state; spin, ping, pulse and bounce keyframes |

## 3. Text and canvas

| Item | NSH reference | Done when |
| --- | --- | --- |
| Fonts | `components.md` (assets) | Apps ship font files that every backend registers before the first view |
| Ellipses | `text.md` | Head and middle ellipses on several lines, and beside attachments on Windows and the web |
| Canvas drawing | `canvas.md` | Matrices, composite modes, dashes, miter limits, shadows, text alignment and stroking, `arcTo`, rotated ellipses, per-corner rectangles, conic gradients, SVG paths, images |
| Canvas queries | `canvas.md` | `MeasureText`, text metrics, point-in-path and point-in-stroke tests |
| Canvas nodes | `canvas.md` (nodes) | Named parts of a drawing are accessibility and test elements with a role, label and selected state |
| Animated drawings | `canvas.md`, `animation.md` | Animated values as drawing arguments, redrawn on the frame clock without commits |
| Animation API | `animation.md` | Composite `Reset`, spring and decay samplers, shadow opacity and text color bindings |

## 4. Base components and popups

| Item | NSH reference | Done when |
| --- | --- | --- |
| Theme tokens | `components.md` (the theme) | `ui.Theme` names shadcn/ui's tokens as NSH's does and holds the app's own by name, in NSGo's own colors |
| Collapsible and Progress | `crates/nsh-base/src/collapsible.rs`, `progress.rs` | `base.Collapsible` keeps its content while closed; `base.Progress` reports its role and value, and `ui.Progress` builds on it |
| Control details | `crates/nsh-base/src/button.rs`, `radio.rs`, `keyboard.rs`, `alert_dialog.rs` | Buttons take selected, role and press down and up; radios report selected; `KeyboardAvoidingView` animates; alerts present as action sheets |
| Resizable panels | `dock.md` (layers) | `ResizablePanelGroup`, `ResizablePanel` and `ResizeHandle` with sizes, limits, handle parts and screen-reader steps |
| Dock area | `dock.md` | `DockArea` reads and writes gpui-base's layout format, drags tabs between groups and docks, and shows one group at a time below 600 pt |
| Touch-screen popups | `components.md` (popups) | Phones give popovers a backdrop and menus a bottom sheet with submenu levels; context menus open at the pointer or after a 400 ms hold |
| System headers | `elements.md` (`<NavStack>`) | Large titles collapse on scroll, transparent headers inset scroll views, platforms measure each screen's header, and `ui.Dialog` takes the modal props |
| Sheets and tooltips | `crates/nsh-base/src/sheet.rs`, `tooltip.rs` | Sheets slide from their side over a fading backdrop and report after the motion; any element takes a tooltip that never takes the pointer |
| Text view | `text-view.md` | A streaming caret, table and code block actions, scroll bar switches, Markdown options, extensions for a subtree, highlighting off the engine goroutine |

`DockArea` needs the resizable panels and the touch responder from section 1.

## 5. Services and windows

| Item | NSH reference | Done when |
| --- | --- | --- |
| Clipboard | `crates/nsh-core/src/runtime/clipboard.rs` | Images and files besides text |
| Environment | `crates/nsh-core/src/platform.rs` | The OS version and the mac and TV idioms |
| System toast | `crates/nsh-core/src/runtime/android.rs` | Android shows the platform's toast |
| Window commands | `crates/nsh-core/src/runtime/desktop.rs` | Animated, top-anchored `SetFrame`, window levels, `OrderFront`, an auto-hiding menu bar, native popup menus |
| App lifecycle | `macos.md` (standalone shutdown) | `nsgo.Run` returns for cleanup; apps quit themselves or outlive their last window; a forced appearance reaches native chrome |

## 6. Testing and the tool

| Item | NSH reference | Done when |
| --- | --- | --- |
| Test steps | `tooling.md` (headless runs) | Press, type, clear, submit, scroll, drag, hover and long press send what real input sends, move the clock and fail on disabled views |
| Test queries | `crates/nsh-testing/src/selector.rs` | Bare and regex selectors, Playwright's visibility, more waits, `FastForward`, the environment from the first frame, default dialog answers |
| `nsgo test` | `tooling.md` (`nsh test`) | Runs an app's tests with a filter, JUnit output and per-commit traces |
| Tests on real targets | `tooling.md` (on a real platform) | `nsgo test -target` drives the steps on each backend with native input and takes screenshots |
| Dev loop | `tooling.md` (dev server), `macos.md` (measuring) | Release dev builds, a free port, LAN hosts, an error overlay, a reload key and simulators by name |
| Packages | `embedding.md`, `web.md` | Build numbers, the app's appearance, Linux and Windows archives, IPAs, a cacheable web site with a manifest, `-run` |
| Store signing | `embedding.md` | `nsgo build` signs an IPA with a provisioning profile and an APK with a keystore |

## 7. Embedding

| Item | NSH reference | Done when |
| --- | --- | --- |
| Embedding SDKs | `embedding.md` | iOS, Android and desktop host apps show an NSGo view run by their own loop |
| Several engines | `architecture.md` | Backends keep no singletons, so one process runs several surfaces |
| Host settings | `embedding.md` | A host registers components per view, sets locale, text size and appearance, receives app errors and sets the data directory |
| Component lifetime | `embedding.md` (native components) | Native components are destroyed when their app stops, on Android too |

## 8. Performance

| Item | NSH reference | Done when |
| --- | --- | --- |
| Incremental layout | `pitfalls.md` (relayout boundaries), `performance.md` | Changes stop at fixed-size flex items, and a text that measures as before lays nothing out |
| Measure cache | `architecture.md` (data flow and threads) | Text measurements are cached by content and width per engine, so identical texts and list rows measure once |
| Paced list binds | `scrolling-and-lists.md` (paced binding) | A list binds its viewport first and its overscan over later frames |
| macOS frames | `macos.md` (architecture) | Scrolls move the container layer and commit at rest; batches take at most 4 ms a frame; resizes wait for layout; a timer covers a stalled display link |
| macOS drawing | `macos.md` (architecture), `canvas.md` | Content layers replace `drawRect:`; canvases draw off the main thread and redraw only what changed |
| Windows frames | `windows.md` (frames) | The frame clock follows the compositor with `DwmFlush` |

## 9. Finish the existing platforms

These items wait for community contributions.

| Item | NSH reference | Done when |
| --- | --- | --- |
| iOS touch | `crates/nsh-platform-ios` | Gestures, text input and selection are checked in the simulator; selections have touch handles; hover, swipe back and touches outside the parent work |
| iOS system UI | `elements.md` (overlays), `pitfalls.md` | VoiceOver's escape gesture goes back and closes popups; inputs in scroll views stay clear of the keyboard |
| Text selection | `text.md` (selection areas) | Selectable text and selection areas work on Windows, Linux and Android |
| Desktop accessibility | `windows.md`, `linux.md` | Windows exposes UI Automation and Linux GTK's accessibility, and both announce |
| Desktop chrome | `windows.md` (alerts), `linux.md` | Windows draws alerts in the surface; Linux draws window controls under a hidden title bar and animates modals |
| Android | `elements.md`, `styles.md` (Android differences) | Ripples, predictive back on screens, and blurred images before Android 12 |
| Android frames | `elements.md` (interaction and the environment) | `RequestAnimationFrame` runs on `Choreographer` frames |
| Web | `web.md` | Browser history and back keys, a focus state for pressables, local time with daylight saving |
| Real file drags | `file-drop.md` | A real drag from Finder, Explorer and a Linux file manager is checked to reach a view |

## Not ported

Go apps do not need these NSH parts:

- The JS runtime, bundler, TSX compiler, `nsh check`, compatibility baselines and hot module replacement, since apps are Go and `go vet` checks them.
- Tailwind classes, since styles are Go values.
- `fetch`, WebSockets and `resource`, since apps use `net/http` or Go libraries with `Scope.Post`.
- Component refs, slots, render props and typed rows, since Go fields and functions carry them.
- `EngineContext`, since a host gives each engine's app function its values directly.
- The GPUI backend and the LayerFS trial.
