# Performance

The performance monitor measures what a surface's frames and commits cost while it runs, for a panel over the app (`nsgo dev` or `nsgo.FPS`). It follows NSH's `nsh_engine::perf` and macOS HUD. Read this before changing `internal/engine/perf.go`, `internal/platform/macos/perf.go`, a backend's frame clock or anything that runs every frame.

## Measuring

`engine.PerfMonitor` is shared by the two threads of a surface:

- The engine goroutine records every wake-up that produced a commit with `RecordCommit`: the app's code and the core's work, and the mount items the commit emitted. `RecordBusy` records how long each wake-up was busy.
- `engine.Options.Perf` installs a monitor; nil measures nothing and costs one check per wake-up.
- The main thread records its work with `RecordWork`: applying batches, stepping animations, handling input; `dirty` says whether the work changes what the screen shows.
- It closes a frame at every display refresh with `Vsync`. A refresh in which something changed is a frame the screen showed, and its cost is the main-thread work since the previous refresh; refreshes that changed nothing are not frames.
- A batch carries `MountBatch.Sent` while a monitor measures, so the platform can report how long it waited between the engine goroutine and the main thread with `RecordLatency`.
- The process' CPU, memory and (on macOS) GPU arrive with `SetProcess` from the platform's sampler.

`Snapshot` turns what was recorded into `PerfSnapshot`: rates over the last second, distributions over the last 120 frames and commits (mean, p50, p95, max), the share of frames over one refresh, the engine's busy share and the process' resources. The first 4 frames and commits are cold and are not counted.

## The panel

`nsgo dev` passes `-X github.com/oneo-me/nsgo.fps=1` to every development build; `nsgo build` never passes it. `nsgo.Run` turns the flag into the backend's monitor, and `nsgo.FPS(true)` does the same from an app run with `go run`; embedded views have none. Only the native macOS target has a monitor; the others accept the option and ignore it.

The macOS panel sits at the top right of the surface, lets the mouse through, fades out while the pointer is over it, and is redrawn every 500 ms:

- `FPS` counts frames shown over the last second; `max` is what the main thread could keep up with, capped at the display's rate.
- `TICK` is the gaps between refreshes and the 95th-percentile gap between shown frames.
- `MAIN` is main-thread time per frame, its 95th percentile, and `drop`, the share of frames that took longer than one refresh.
- `APP` and `CORE` are the app's and the core's time per commit, commits per second and mount items per commit.
- `HOP` is how long batches waited between the threads, and how busy the engine goroutine was.
- `CPU` is percent of one core, `GPU` this process' share, `MEM` its physical footprint.
- The trace is the last 120 frames' main-thread cost against one refresh (the line): green within half of it, amber within it, red over.

While a monitor measures, macOS runs its display link on every refresh, so an idle app still reports the display's tick rate. Its `FPS` reads low when the app is idle; that is not slowness. Measure release builds: a debug build's text and layout costs are not the app's.

## Adding a backend

A backend calls `RecordWork` around its main-thread work, `Vsync` from its frame clock with the display link's period, `RecordLatency` on the main thread when a batch with `Sent` applies, and `SetProcess` from its process sampler. The panel draws `PerfSnapshot.Readings()`; its look is the platform's.

macOS reads memory with `proc_pid_rusage`'s physical footprint and GPU time from the `accumulatedGPUTime` of the process' accelerator clients in the IO registry (`internal/platform/apple/perf.go`), both through purego.

## Keeping frames cheap

Every frame runs the commit on the engine goroutine and, on the main thread, the batch, the tick and the drawing of what changed. `bench/quotes` measures these paths ([benchmarks](../../bench/README.md)); compare its `CPU per commit`, and the panel's `MAIN` and `CORE`, before and after changing them. The core's own benchmarks (`go test ./core -run '^$' -bench .`) time commits on the approximate text engine, as NSH's `bench_view_tick` and `bench_high_frequency` do:

| Benchmark | Per commit, M1 Max | Items |
| --- | --- | --- |
| `ViewTick`: 300 static rows and a counter | 4 µs (572 µs when commits walked the whole tree) | 1 |
| `Board/fixed`: 961 views, 40 values of fixed cells a tick | 52 µs (766 µs) | 40 |
| `Board/content`: the same in cells of their text's size | 51 µs (889 µs) | 40 |
| `Board/widening`: values that change width every tick | 486 µs (877 µs), nearly all of it flex layout | 280 |
| `ListScroll`: 1000 rows, 10 points a commit | 12 µs (121 µs) | 2.5 |

- A commit visits only what changed, lays out only what changes reach, and skips texts that measure as before ([commit](core.md#commit)). Text sizes are cached per engine ([text](text.md#text-and-drawing)) and lists pace their binds ([lists](scrolling.md#lists)).

- The engine's OS thread has the main thread's quality of service on Apple platforms (`engine.Options.Thread`, `apple.RaiseThread`), as NSH's does: at the default class macOS may run it on efficiency cores. Raised, `bench/quotes` commits about 15% faster, and steadily under load.
- The cores clock down between frames, so a commit in the app takes longer than the same work in a Go benchmark; measure a change both ways.
- A commit allocates only what it sends: compare a node's values on the stack and copy them into an item once they changed. Flex layout reuses its item buffers.
- macOS draws only content that changed and can be seen ([text and drawing](text.md#text-and-drawing)), and decides what can be seen from the batches' state rather than by asking AppKit.
- The main thread never waits for the GPU: Metal shader views take drawables and present on goroutines of their own ([shaders](shaders.md#platforms)).

## Frame clocks

macOS follows NSH's surface:

- While something animates on the frame clock (the animation graph, a layout animation, a scroll view moving or fading its indicator), batches wait for the start of the next frame. A frame the app asked for alone holds nothing back.
- A scroll frame moves only a layer's translation, and AppKit learns the offset once the scroll rests ([macOS scroll views](scrolling.md#macos-scroll-views)).
- A frame applies them for at most 4 ms (`frameBatches`), at least one whole batch, and carries the rest: a scroll's frames are not delayed by batches landing between them, and a burst spreads over frames.
- A resize applies what waited, then waits, at most 16 ms, for the commit at the new size (`engine.ResizeAndWait`) and applies it before AppKit draws the window: the one place the main thread waits for the engine.
- An engine that missed that deadline is not waited for again until its commit lands, so a busy app resizes as before.
- The display link does not fire while the window is occluded or minimized or the screen is locked: while frames are wanted and it has not ticked for 100 ms, a 60 Hz timer ticks in its place, so animations still end.

Windows follows the compositor: a goroutine waits for each composition with `DwmFlush` and posts `wmTick`, one at a time, while something animates, and parks while nothing does. It sleeps a frame's 16 ms where `DwmFlush` fails, and ticks at most every 3 ms should it return at once.

purego prices each foreign call by what its Go signature asks of reflection (Apple M1 Max):

| Call | Cost |
| --- | --- |
| `purego.SyscallN`, `apple.Msg` | Under 0.15 µs |
| A registered function or `ID.Send` with scalar arguments | 0.25–0.5 µs |
| `objc.Send[T]`, which registers a function on every call | 0.4 µs |
| A struct argument or result | About 2 µs |

On per-frame paths, send messages whose arguments and result are objects, integers or booleans with `apple.Msg`. Declare a 16-byte struct argument (`CFRange`, `CGSize`, `CGPoint`) as its two fields, which arm64 and x86-64 both pass in two registers; larger structs pass differently on the two, so they stay structs. Keep immutable CoreFoundation objects, such as fonts, attributes and frame paths, in bounded caches.

## Where Go departs from NSH

| NSH | NSGo | Why |
| --- | --- | --- |
| `JS` per commit | `APP` per commit | Apps are Go |
| `ops` and mount items in a commit | Mount items | NSGo's commit does not count ops |
| `--fps --trace` logs the readings once a second | Not ported | `nsgo dev` has no trace flag |
