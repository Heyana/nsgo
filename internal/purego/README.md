# purego

This is the `github.com/ebitengine/purego` v0.11.3 source used by NSGo, kept locally so Android arm64 can use the same call and callback machinery with `CGO_ENABLED=0`. The upstream Android dynamic-loader implementation imports cgo; the Android files here call bionic's `libdl.so` symbols through Go's dynamic imports and keep the desktop implementations unchanged. The source is distributed under the upstream Apache-2.0 license in [LICENSE](LICENSE).
