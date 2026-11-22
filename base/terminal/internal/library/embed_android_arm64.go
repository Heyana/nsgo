//go:build android && arm64

package library

// Android packages place this library beside libnsgo.so in the APK's native
// library directory. It is intentionally not embedded in the Go executable.
const name = "libghostty-vt.so"

var embedded []byte
