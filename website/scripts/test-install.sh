#!/bin/sh
# Tests public/install.sh against fake releases served from a local HTTP server: a good install of the latest and of a pinned version, a checksum mismatch, a missing release and unsupported platforms. Needs python3 for the server.

set -eu

here=$(cd "$(dirname "$0")" && pwd)
script=$here/../public/install.sh
work=$(mktemp -d 2>/dev/null || mktemp -d -t nsgo-install-test)
server=
cleanup() {
	if [ -n "$server" ]; then
		kill "$server" 2>/dev/null || true
	fi
	rm -rf "$work"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

failures=0
pass() { printf 'ok   %s\n' "$1"; }
flunk() {
	printf 'FAIL %s\n' "$1"
	sed 's/^/     | /' "$work/out" || true
	failures=$((failures + 1))
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d ' ' -f 1
	else
		shasum -a 256 "$1" | cut -d ' ' -f 1
	fi
}

# release VERSION builds vVERSION's archives for every platform the script installs on, and its checksums.txt.
release() {
	dir=$work/srv/download/v$1
	mkdir -p "$dir" "$work/stage"
	printf '#!/bin/sh\necho v%s\n' "$1" >"$work/stage/nsgo"
	chmod 755 "$work/stage/nsgo"
	echo license >"$work/stage/LICENSE"
	echo readme >"$work/stage/README.md"
	: >"$dir/checksums.txt"
	for platform in darwin_arm64 darwin_amd64 linux_amd64 linux_arm64; do
		asset=nsgo_$1_$platform.tar.gz
		tar -czf "$dir/$asset" -C "$work/stage" nsgo LICENSE README.md
		printf '%s  %s\n' "$(sha256 "$dir/$asset")" "$asset" >>"$dir/checksums.txt"
	done
	rm -rf "$work/stage"
}

release 9.8.7
release 9.8.6
release 9.8.5
# v9.8.5's archives no longer match its checksums.
awk '{ c = substr($1, 1, 1); n = (c == "0") ? "1" : "0"; print n substr($1, 2) "  " $2 }' "$work/srv/download/v9.8.5/checksums.txt" >"$work/sums"
mv "$work/sums" "$work/srv/download/v9.8.5/checksums.txt"
cat >"$work/srv/latest.json" <<'EOF'
{
  "url": "https://api.github.com/repos/nsgo-dev/nsgo/releases/1",
  "tag_name": "v9.8.7",
  "name": "v9.8.7",
  "draft": false
}
EOF
cp "$script" "$work/srv/install.sh"

port=$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
python3 -m http.server "$port" --bind 127.0.0.1 --directory "$work/srv" >"$work/server.log" 2>&1 &
server=$!
base=http://127.0.0.1:$port
tries=0
until curl -fs -o /dev/null "$base/latest.json"; do
	tries=$((tries + 1))
	[ "$tries" -lt 50 ] || { cat "$work/server.log"; exit 1; }
	sleep 0.1
done

export NSGO_DOWNLOAD_BASE="$base/download"
export NSGO_RELEASE_API="$base/latest.json"
bin=$work/bin
unset NSGO_VERSION

# run COMMAND... runs a command with its output in $work/out, reporting its exit status in $status.
run() {
	set +e
	"$@" >"$work/out" 2>&1
	status=$?
	set -e
}

name="latest release through curl | sh"
run sh -c 'curl -fsSL "$1/install.sh" | NSGO_INSTALL_DIR="$2" sh' sh "$base" "$bin"
if [ "$status" = 0 ] && [ "$("$bin/nsgo")" = v9.8.7 ] && grep -q 'installed nsgo v9.8.7' "$work/out" && grep -q 'is not on your PATH' "$work/out"; then pass "$name"; else flunk "$name"; fi

name="pinned version without its v, over an existing install on PATH"
run env NSGO_VERSION=9.8.6 NSGO_INSTALL_DIR="$bin" PATH="$bin:$PATH" sh "$script"
if [ "$status" = 0 ] && [ "$("$bin/nsgo")" = v9.8.6 ] && ! grep -q 'PATH' "$work/out" && [ "$(ls -A "$bin")" = nsgo ]; then pass "$name"; else flunk "$name"; fi

name="checksum mismatch"
run env NSGO_VERSION=v9.8.5 NSGO_INSTALL_DIR="$bin" sh "$script"
if [ "$status" != 0 ] && grep -q 'checksum mismatch' "$work/out" && [ "$("$bin/nsgo")" = v9.8.6 ]; then pass "$name"; else flunk "$name"; fi

name="missing release"
run env NSGO_VERSION=v9.9.9 NSGO_INSTALL_DIR="$bin" sh "$script"
if [ "$status" != 0 ] && grep -q 'check that release v9.9.9 exists' "$work/out" && grep -q 'go install' "$work/out" && [ "$("$bin/nsgo")" = v9.8.6 ]; then pass "$name"; else flunk "$name"; fi

name="malformed version"
run env NSGO_VERSION='v1/../x' NSGO_INSTALL_DIR="$bin" sh "$script"
if [ "$status" != 0 ] && grep -q 'must look like vX.Y.Z' "$work/out"; then pass "$name"; else flunk "$name"; fi

# A fake uname reports the platform each unsupported case names.
fake_uname() {
	mkdir -p "$work/fake-$1-$2"
	# shellcheck disable=SC2016 # $1 is the fake's own argument.
	printf '#!/bin/sh\ncase $1 in -s) echo %s ;; -m) echo %s ;; esac\n' "$1" "$2" >"$work/fake-$1-$2/uname"
	chmod 755 "$work/fake-$1-$2/uname"
	echo "$work/fake-$1-$2"
}

for platform in FreeBSD:amd64 MINGW64_NT-10.0:x86_64 Linux:riscv64 Darwin:ppc; do
	os=${platform%%:*}
	arch=${platform#*:}
	name="unsupported platform $os $arch"
	run env PATH="$(fake_uname "$os" "$arch"):$PATH" NSGO_INSTALL_DIR="$work/none" sh "$script"
	if [ "$status" != 0 ] && grep -q 'go install github.com/nsgo-dev/nsgo/cmd/nsgo@latest' "$work/out" && [ ! -e "$work/none" ]; then pass "$name"; else flunk "$name"; fi
done

# wget alone, on a PATH holding every command but curl.
if command -v wget >/dev/null 2>&1; then
	mkdir "$work/nocurl"
	wget=$(command -v wget)
	for dir in "${wget%/*}" /usr/local/bin /usr/bin /bin; do
		for cmd in "$dir"/*; do
			name=${cmd##*/}
			[ "$name" = curl ] || [ -e "$work/nocurl/$name" ] || ln -s "$cmd" "$work/nocurl/$name"
		done
	done
	name="latest release through wget"
	run env PATH="$work/nocurl" NSGO_INSTALL_DIR="$work/wget" sh "$script"
	if [ "$status" = 0 ] && [ "$("$work/wget/nsgo")" = v9.8.7 ]; then pass "$name"; else flunk "$name"; fi
fi

if [ "$failures" != 0 ]; then
	echo "$failures failed"
	exit 1
fi
