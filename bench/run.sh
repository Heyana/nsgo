#!/bin/sh
# Builds the quotes benchmark and reports its CPU time, frame rate and
# commit times.
#
# Usage: bench/run.sh [seconds]
#
# The default is a 3 s warmup followed by 15 s of measurement. 1000 ms/s is
# one core; run one binary at a time, as squeezing two together makes their
# numbers converge.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
seconds=${1:-15}
warmup=3

case $seconds in
'' | *[!0-9]*)
	echo "usage: $0 [seconds]" >&2
	exit 2
	;;
esac
if [ "$seconds" -lt 1 ]; then
	echo "$0: seconds must be at least 1" >&2
	exit 2
fi

tmp=$(mktemp -d)
pid=
cleanup() {
	if [ -n "$pid" ]; then
		kill "$pid" 2>/dev/null || true
		wait "$pid" 2>/dev/null || true
	fi
	rm -rf "$tmp"
}
trap cleanup EXIT HUP INT TERM

# cputime prints a process' user+system CPU time in seconds.
cputime() {
	ps -p "$1" -o cputime= | awk -F'[:.]' '
		NF == 4 { printf "%.2f\n", $1 * 3600 + $2 * 60 + $3 + $4 / 100; next }
		NF == 3 { printf "%.2f\n", $1 * 60 + $2 + $3 / 100; next }
		NF == 2 { printf "%.2f\n", $1 + $2 / 100 }'
}

echo "building bench/quotes..." >&2
(
	cd "$root"
	go build -trimpath -buildvcs=false \
		-ldflags "-s -w -X 'github.com/oneo-me/nsgo.name=NSGo Quotes' -X 'github.com/oneo-me/nsgo.width=560' -X 'github.com/oneo-me/nsgo.height=1040'" \
		-o "$tmp/nsgo-quotes" ./bench/quotes
)

NSGO_BENCH_STATS=1 "$tmp/nsgo-quotes" >"$tmp/out.log" 2>&1 &
pid=$!

sleep "$warmup"
if ! kill -0 "$pid" 2>/dev/null; then
	echo "benchmark exited during warmup:" >&2
	cat "$tmp/out.log" >&2
	exit 1
fi

mark=$(wc -l <"$tmp/out.log" | tr -d ' ')
t0=$(cputime "$pid")
sleep "$seconds"
if ! kill -0 "$pid" 2>/dev/null; then
	echo "benchmark exited during measurement:" >&2
	cat "$tmp/out.log" >&2
	exit 1
fi
t1=$(cputime "$pid")

kill "$pid"
wait "$pid" 2>/dev/null || true
pid=

cpu=$(awk -v a="$t0" -v b="$t1" -v s="$seconds" 'BEGIN {
	printf "%.2f", (b - a) / s * 1000
}')
awk -v c="$cpu" 'BEGIN {
	printf "CPU: %.1f ms/s (%.1f%% of one core)\n", c, c / 10
}'

tail -n +"$((mark + 1))" "$tmp/out.log" |
	awk -v s="$seconds" -v cpu="$cpu" '
	function sort(a, n,   i, j, t) {
		for (i = 2; i <= n; i++) {
			t = a[i]
			for (j = i - 1; j > 0 && a[j] > t; j--) a[j + 1] = a[j]
			a[j + 1] = t
		}
	}
	function pct(a, n, p,   k) {
		if (n == 0) return 0
		k = int((n - 1) * p / 100) + 1
		return a[k]
	}
	/^nsgo 1s: frames=/ {
		split($3, kv, "=")
		frameSamples[++frameN] = kv[2] + 0
		next
	}
	/^nsgo commit:/ {
		for (i = 3; i <= NF; i++) {
			split($i, kv, "=")
			if (kv[1] == "frames") ft = kv[2] + 0
			else if (kv[1] == "interval_us") fi = kv[2] + 0
			else if (kv[1] == "commit_us") ff = kv[2] + 0
			else if (kv[1] == "latency_us") fl = kv[2] + 0
		}
		n++
		framesPerCommit += ft
		intervalTotal += fi
		interval[n] = fi
		commit[n] = ff
		latency[n] = fl
	}
	END {
		if (frameN == 0) {
			print "frames: no samples"
		} else {
			sort(frameSamples, frameN)
			printf "frames: median %.1f/s (%d samples)\n", pct(frameSamples, frameN, 50), frameN
		}
		if (n == 0) {
			print "commits: no samples"
			exit
		}
		# Commit rate comes from the intervals between reports, not the
		# shell window, so a partial first or last second does not change it.
		rate = (n > 1 && intervalTotal > 0) ? (n - 1) / (intervalTotal / 1000000) : n / s
		sort(interval, n)
		sort(commit, n)
		sort(latency, n)
		printf "commits: %.1f/s (%d samples, %.2f frames/commit)\n", rate, n, framesPerCommit / n
		printf "interval: p50 %.2f ms, p95 %.2f ms, p99 %.2f ms, max %.2f ms\n", pct(interval, n, 50) / 1000, pct(interval, n, 95) / 1000, pct(interval, n, 99) / 1000, interval[n] / 1000
		printf "commit: p50 %.2f ms, p95 %.2f ms, p99 %.2f ms, max %.2f ms\n", pct(commit, n, 50) / 1000, pct(commit, n, 95) / 1000, pct(commit, n, 99) / 1000, commit[n] / 1000
		printf "latency: p50 %.2f ms, p95 %.2f ms, p99 %.2f ms, max %.2f ms\n", pct(latency, n, 50) / 1000, pct(latency, n, 95) / 1000, pct(latency, n, 99) / 1000, latency[n] / 1000
		printf "CPU per commit: %.2f ms (%.2f ms per frame)\n", cpu / rate, cpu / (rate * (framesPerCommit / n))
	}'
