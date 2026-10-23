# Performance

The performance monitor measures what a surface's frames and commits cost while it runs, for a panel over the app (`nsgo dev -fps` or `nsgo.FPS`). It follows NSH's `nsh_engine::perf` and macOS HUD; read this before changing `engine/perf.go`, `platform/macos/perf.go` or a backend's frame clock.

## Measuring

`engine.PerfMonitor` is shared by the two threads of a surface:

- The engine goroutine records every wake-up that produced a commit with `RecordCommit`: the app's code and the core's work, and the mount items the commit emitted. It records how long each wake-up was busy with `RecordBusy`. `engine.Options.Perf` installs a monitor; nil measures nothing and costs one check per wake-up.
- The main thread records its work with `RecordWork`: applying batches, stepping animations, handling input; `dirty` says whether the work changes what the screen shows. It closes a frame at every display refresh with `Vsync`: a refresh in which something changed is a frame the screen showed, and its cost is the main-thread work since the previous refresh. Refreshes that changed nothing are not frames.
- A batch carries `MountBatch.Sent` while a monitor measures, so the platform can report how long it waited between the engine goroutine and the main thread with `RecordLatency`.
- The process' CPU, memory and (on macOS) GPU arrive with `SetProcess` from the platform's sampler.

`Snapshot` turns what was recorded into `PerfSnapshot`: rates over the last second, distributions over the last 120 frames and commits (mean, p50, p95, max), the share of frames over one refresh, the engine's busy share and the process' resources. The first 4 frames and commits are cold and are not counted.

## The panel

`nsgo dev -fps` passes `-X github.com/oneo-me/nsgo.fps=1` to the app's build; `nsgo.Run` turns that into the backend's monitor. `nsgo.FPS(true)` does the same from an app, for `go run` or embedding. The flag exists only for the native macOS target: other targets have no monitor yet and the tool refuses `-fps` for them.

The macOS panel sits at the top right of the surface, lets the mouse through, and is redrawn every 500 ms:

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

macOS reads memory with `proc_pid_rusage`'s physical footprint and GPU time from the `accumulatedGPUTime` of the process' accelerator clients in the IO registry (`platform/internal/apple/perf.go`), both through purego.

## Where Go departs from NSH

| NSH | NSGo | Why |
| --- | --- | --- |
| `JS` per commit | `APP` per commit | Apps are Go |
| `ops` and mount items in a commit | Mount items | NSGo's commit does not count ops |
| `--fps --trace` logs the readings once a second | Not ported | `nsgo dev` has no trace flag |
