#!/bin/sh
# Installs the nsgo command from NSGo's GitHub releases.
#
#   curl -fsSL https://nsgo.dev/install.sh | sh
#
# NSGO_VERSION=vX.Y.Z installs that release instead of the latest, and NSGO_INSTALL_DIR picks the directory, ~/.local/bin by default. It needs curl or wget, tar, and sha256sum or shasum, and never sudo.
#
# NSGO_DOWNLOAD_BASE and NSGO_RELEASE_API exist only for the script's tests, which serve a fake release from a local server.

set -eu

repo=nsgo-dev/nsgo
download_base=${NSGO_DOWNLOAD_BASE:-https://github.com/$repo/releases/download}
release_api=${NSGO_RELEASE_API:-https://api.github.com/repos/$repo/releases/latest}
go_install="go install github.com/$repo/cmd/nsgo@latest"

say() {
	printf 'nsgo: %s\n' "$*"
}

fail() {
	printf 'nsgo: error: %s\n' "$*" >&2
	exit 1
}

has() {
	command -v "$1" >/dev/null 2>&1
}

unsupported() {
	printf 'nsgo: error: %s\n' "$*" >&2
	printf 'nsgo: install it with Go instead: %s\n' "$go_install" >&2
	exit 1
}

# fetch URL FILE downloads URL into FILE, failing on an HTTP error.
fetch() {
	if has curl; then
		curl -fsSL --retry 3 -o "$2" "$1"
	elif has wget; then
		wget -q -O "$2" "$1"
	else
		fail "neither curl nor wget is installed"
	fi
}

sha256() {
	if has sha256sum; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif has shasum; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	else
		fail "neither sha256sum nor shasum is installed, so the download cannot be verified"
	fi
}

detect_os() {
	case $(uname -s) in
	Darwin) echo darwin ;;
	Linux) echo linux ;;
	MINGW* | MSYS* | CYGWIN* | Windows_NT)
		unsupported "this script installs on macOS and Linux; on Windows, download nsgo_<version>_windows_<arch>.zip from https://github.com/$repo/releases"
		;;
	*) unsupported "no prebuilt nsgo for $(uname -s)" ;;
	esac
}

detect_arch() {
	case $(uname -m) in
	x86_64 | amd64)
		# A shell translated by Rosetta reports x86_64 on an Apple silicon Mac, which runs the arm64 build natively.
		if [ "$1" = darwin ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || true)" = 1 ]; then
			echo arm64
		else
			echo amd64
		fi
		;;
	arm64 | aarch64) echo arm64 ;;
	*) unsupported "no prebuilt nsgo for $1 on $(uname -m)" ;;
	esac
}

latest_version() {
	fetch "$release_api" "$1/release.json" || fail "could not look up the latest release at $release_api; set NSGO_VERSION=vX.Y.Z to pick one"
	tag=$(tr ',' '\n' <"$1/release.json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
	[ -n "$tag" ] || fail "the latest release at $release_api has no tag; set NSGO_VERSION=vX.Y.Z to pick one"
	echo "$tag"
}

main() {
	os=$(detect_os)
	arch=$(detect_arch "$os")
	has tar || fail "tar is not installed"

	tmp=$(mktemp -d 2>/dev/null || mktemp -d -t nsgo)
	trap 'rm -rf "$tmp"' EXIT
	trap 'exit 1' HUP INT TERM

	tag=${NSGO_VERSION:-}
	if [ -z "$tag" ]; then
		tag=$(latest_version "$tmp")
	fi
	version=${tag#v}
	case $version in
	'' | *[!0-9A-Za-z.+-]*) fail "NSGO_VERSION must look like vX.Y.Z, not '$tag'" ;;
	esac
	tag=v$version

	asset=nsgo_${version}_${os}_${arch}.tar.gz
	say "downloading nsgo $tag for $os/$arch"
	fetch "$download_base/$tag/$asset" "$tmp/$asset" || unsupported "could not download $download_base/$tag/$asset; check that release $tag exists and has prebuilt binaries"
	fetch "$download_base/$tag/checksums.txt" "$tmp/checksums.txt" || fail "could not download $download_base/$tag/checksums.txt"

	want=$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1; exit }' "$tmp/checksums.txt")
	[ -n "$want" ] || fail "checksums.txt of $tag does not list $asset"
	got=$(sha256 "$tmp/$asset")
	[ "$got" = "$want" ] || fail "checksum mismatch for $asset: expected $want, got $got; nothing was installed"

	mkdir "$tmp/x"
	tar -xzf "$tmp/$asset" -C "$tmp/x"
	[ -f "$tmp/x/nsgo" ] || fail "$asset holds no nsgo binary"

	dir=${NSGO_INSTALL_DIR:-$HOME/.local/bin}
	mkdir -p "$dir" || fail "could not create $dir; set NSGO_INSTALL_DIR to a directory you can write"
	# Copy beside the old binary and rename over it, so a running nsgo or a failed copy never leaves a broken one.
	cp "$tmp/x/nsgo" "$dir/.nsgo.$$" || fail "could not write to $dir; set NSGO_INSTALL_DIR to a directory you can write"
	chmod 755 "$dir/.nsgo.$$"
	mv -f "$dir/.nsgo.$$" "$dir/nsgo"
	say "installed nsgo $tag to $dir/nsgo"

	case :${PATH}: in
	*:"$dir":*) ;;
	*)
		say "$dir is not on your PATH; add it in your shell's profile:"
		# shellcheck disable=SC2016 # The profile expands $PATH, not this script.
		printf '\n  export PATH="%s:$PATH"\n\n' "$dir"
		;;
	esac
}

main "$@"
