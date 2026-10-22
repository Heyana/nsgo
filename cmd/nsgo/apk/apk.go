// Package apk writes Android packages: the ZIP archive, the binary
// manifest and the v2 signature. Every NSGo build needs only Go, so the
// Android SDK's aapt2, d8 and apksigner stay out of it; the Java runtime
// comes prebuilt from platform/android/runtime.
package apk

import (
	"io"
	"strconv"
	"strings"
)

// Write writes a signed APK to w: template's manifest with m filled in,
// then files, and the empty resource table the system expects.
func Write(w io.Writer, template []byte, m Manifest, files []File, key *Key) error {
	manifest, err := EncodeManifest(template, m)
	if err != nil {
		return err
	}
	all := []File{{Name: "AndroidManifest.xml", Data: manifest, Stored: true}}
	all = append(all, files...)
	all = append(all, File{Name: "resources.arsc", Data: emptyResources, Stored: true})
	zip, err := archive(all)
	if err != nil {
		return err
	}
	signed, err := sign(zip, key)
	if err != nil {
		return err
	}
	_, err = w.Write(signed)
	return err
}

// emptyResources is an empty resource table: an NSGo app defines no
// resources of its own yet.
var emptyResources = []byte{
	0x02, 0x00, 0x0c, 0x00, 0x28, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x1c, 0x00, 0x1c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x1c, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
}

// VersionCode turns a version like 1.2.3 into Android's integer code. A
// version that is not dotted numbers codes as 1.
func VersionCode(version string) int {
	parts := strings.Split(version, ".")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	code := 0
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return 1
		}
		code = code*100 + n
	}
	for i := len(parts); i < 3; i++ {
		code *= 100
	}
	if code > 2100000000 {
		return 2100000000
	}
	if code < 1 {
		return 1
	}
	return code
}
