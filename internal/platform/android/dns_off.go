//go:build (android || nsgoandroidhost) && !nsgo_android_dns && !nsgodev && !nsgotest

package android

// useDNS is a no-op when the app imports no Go networking of its own, keeping net and its DNS resolver out of minimal Android binaries.
func useDNS() {}
