# Benchmarks against other frameworks

The same three apps written for NSGo, [MyGo](https://github.com/egoist/mygo), [Fyne](https://github.com/fyne-io/fyne), [Wails](https://github.com/wailsapp/wails), [NSH](https://github.com/longbridge/nsh), [GPUI Kit](https://github.com/longbridge/gpui-kit) and [GPUI Fast](https://github.com/longbridge/gpui-fast), and the runner that measures them on macOS, Windows and Linux. The website's Benchmarks page shows `results/`.

| Case | What it does | Measured |
| --- | --- | --- |
| `counter` | A count and two buttons, left idle (360×260) | Startup, memory, binary size |
| `quotes` | 40 quotes, ten changed every display frame (560×860) | CPU, the compositor's CPU, memory, frames |
| `list` | 10,000 rows, scrolled a row further every display frame (480×800) | The same |
| `grid` | 600 numbers, all changed every display frame (1232×432), NSGo's alone and measured only when named | The same |

Each framework writes them its own way: NSGo's and NSH's rows are signals that a tick sets, MyGo builds its view every frame, GPUI renders its view again when the frame asks. Fyne sets the changed texts from an animation, which it ticks before drawing each frame. Wails' page is HTML in the system webview: on each animation frame the quotes page asks Go to tick the market and sets the rows it returns, and the list page is a virtual list that scrolls itself and reports its frames to Go once a second. The quotes' market is mulberry32 from seed 42 in both `internal/bench` and `cases/`, whose tests pin the same rows.

Fyne draws from a 60 Hz ticker rather than the display's refresh, and the system webview gives Wails' pages animation frames at 60 Hz on macOS, so on a faster display both change half as many frames as the others: compare their CPU with their frames.

| Folder | What |
| --- | --- |
| `nsgo/`, `mygo/`, `fyne/`, `wails/` | The Go programs, a package a case, in this module; Wails' with its page |
| `nsh/` | NSH's, in Rust on the OS's own backend, a binary a case |
| `gpui/` | GPUI Kit's, a binary a case; the feature `fast` builds them on GPUI Fast |
| `cases/`, `internal/bench/` | What the Rust and the Go programs share |
| `runner/` | Builds, runs and measures them |
| `results/` | `darwin.json`, `windows.json` and `linux.json` |

## Running it

Go, Rust and, on Linux, GTK 4 and a desktop session. From this folder, with
nothing else running and the display awake:

```sh
go run ./runner                                   # every framework and case, 3 runs of 10 s
go run ./runner -frameworks nsgo,nsh -cases list  # some of them
go run ./runner -frameworks nsgo -cases grid      # where NSGo's frames run out
go run ./runner -frameworks nsgo -cases grid -grid-cols 20 -grid-rows 15 -out /tmp/grid.json
go run ./runner -runs 1 -seconds 3 -out /tmp/x.json
```

The Rust programs can be built on another Mac and copied into their target folders (`nsh/target/release/`, `gpui/target/release/` and `gpui/target/fast/release/`) with the same cargo commands; `-prebuilt` then measures them as they are, so the measured machine needs no Rust toolchain or room for their builds.

It builds release binaries (`go build -ldflags "-s -w"`, `cargo build
--release` with symbols stripped, GPUI Fast in `gpui/target/fast`), then runs
each case's programs one after another, as many rounds as `-runs`, and keeps
each value's median:

- A program prints `bench ready` on its first frame, then every second
  `bench frames N`, the frames it updated. Startup is from launching it to
  the first line.
- After 3 s to settle, the runner reads the process's CPU time over `-seconds`, and its memory at the end: the physical footprint on macOS, the proportional set size on Linux, the working set on Windows. On macOS the XPC services the program started count with it: a system webview (WKWebView, which Wails draws in) lays out, runs and paints its page in WebKit's GPU, WebContent and Networking services rather than in the app's process. The runner takes those started since the program that macOS holds on the same process' account (the app for an app bundle, the terminal for a program a terminal starts).
- It reads the compositor's CPU over the same time, less what it spent at
  rest, measured before each round with none of the programs open:
  WindowServer on macOS, `dwm.exe` on Windows, the Wayland compositor and
  Xwayland on Linux. Native views (NSGo, NSH) leave drawing to it, where GPU
  renderers (MyGo, GPUI) draw in their process, so compare the two added.
  Reading `dwm.exe` takes an elevated runner.

Updates follow the display: frames stay at its refresh rate while an app keeps
up, and a 120 Hz display does twice a 60 Hz one's work. Compare results of one
machine, not across machines.

The grid is configurable for capacity runs without rebuilding the benchmark. `-grid-cols` and `-grid-rows` set the dimensions for the NSGo `grid` process, with the published 30 by 20 case as the default, and the results name the size they used in `grid`. Run one dimension at a time and compare the resulting FPS, CPU and memory on the same machine, writing them with `-out` elsewhere than the published `results/`. The flags only affect `grid`; other cases keep their normal process environment.

The compositor's rest follows everything else on screen, so close other
apps, or at least keep them still. On Windows, run it from the desktop's own
session, as an elevated terminal there: a program started from an SSH session
has no desktop to draw on.
