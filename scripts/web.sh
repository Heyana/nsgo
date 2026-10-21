#!/bin/sh
# Builds a Go main package for the browser into a static site: index.html,
# Go's wasm_exec.js and app.wasm.
# Usage: scripts/web.sh <package> [title]
set -eu
pkg=$1
title=${2:-NSGo}
out=${OUT:-.nsgo/web}
mkdir -p "$out"
GOOS=js GOARCH=wasm go build -o "$out/app.wasm" "$pkg"
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$out/"
cat > "$out/index.html" <<HTML
<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>$title</title>
<style>html, body { margin: 0; height: 100%; }</style>
<script src="wasm_exec.js"></script>
</head>
<body>
<script>
const go = new Go();
WebAssembly.instantiateStreaming(fetch("app.wasm"), go.importObject).then((r) => go.run(r.instance));
</script>
</body>
</html>
HTML
echo "$out"
