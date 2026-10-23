#!/bin/sh
# Builds the quotes benchmark and reports its CPU time.
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

awk -v a="$t0" -v b="$t1" -v s="$seconds" 'BEGIN {
	printf "CPU: %.1f ms/s (%.1f%% of one core)\n", (b - a) / s * 1000, (b - a) / s * 100
}'

tail -n +"$((mark + 1))" "$tmp/out.log" |
	grep -o 'fps=[0-9.]*' |
	cut -d= -f2 |
	sort -n |
	awk '{ a[NR] = $1 }
		END {
			if (NR == 0) { print "fps: no samples"; exit }
			printf "fps: median %.1f (min %.1f, max %.1f, %d samples)\n", a[int((NR + 1) / 2)], a[1], a[NR], NR
		}'
