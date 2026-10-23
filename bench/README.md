# Benchmarks

`bench/quotes` reproduces the cost of text shaping in a moving screen: a
40-row quote table where every 16 ms tick changes 10 rows, four text cells
each, while the other 30 rows stay untouched. Its values, PRNG (mulberry32,
seed 42), column widths and window size match the mygo, gogui and nsh ports
of the same case.

The benchmark writes `nsgo 1s: fps=N` to stderr with `NSGO_BENCH_STATS=1`,
and reports nothing otherwise. It uses `time.Ticker(16ms)` where nsh uses
`requestAnimationFrame`, which is how a Go app drives a frame without a
display link; gogui's built-in ticker does the same.

## Running it

```sh
bench/run.sh          # 3 s warmup, then 15 s measured
bench/run.sh 30       # 3 s warmup, then 30 s measured
```

The script builds a release binary, runs it on its own, and prints the
process' CPU time from `ps -o cputime` as milliseconds per second of wall
time. 1000 ms/s is one full core. It also prints the median FPS over the
measured window.

Run one binary at a time. Two benchmarks competing for the machine squeeze
each other and their numbers converge, which hides both improvements and
regressions. Compare runs on the same machine, back to back and repeated,
rather than absolute numbers from another machine.

The macOS window must be on screen: a hidden or fully occluded window can
draw less, which lowers CPU without making the app cheaper. The window is
1040 points tall, so on a shorter laptop display (982 points at the default
scale on a 14-inch MacBook Pro) macOS constrains it and clips the last
rows; use a taller display or a scaled mode for a like-for-like run.

## The case

Each row is one signal, so a tick settles four text nodes per touched row.
The title's tick count changes every frame too. The result is the per-frame
cost of preparing and measuring that text, on top of layout and drawing.
