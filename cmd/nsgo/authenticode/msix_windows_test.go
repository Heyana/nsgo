//go:build windows

package authenticode

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

type msixTrustGUID struct {
	A    uint32
	B, C uint16
	D    [8]byte
}

type msixTrustFile struct {
	Size uint32
	Name *uint16
	File uintptr
	Type *msixTrustGUID
}

type msixTrustData struct {
	Size                   uint32
	Callback, Client       uintptr
	UI, Revocation, Choice uint32
	File                   *msixTrustFile
	Action                 uint32
	State                  uintptr
	URL                    *uint16
	Flags, Context         uint32
	Settings               uintptr
}

// TestMSIXVerifiesWithWindows consumes the public fixture exported by TestSignsMSIXWithCertificate on a build host. It changes no certificate stores, policies or installed packages. The synthetic identity is expected to fail trust after its digest verifies; a changed payload must fail earlier.
func TestMSIXVerifiesWithWindows(t *testing.T) {
	root := os.Getenv("NSGO_MSIX_NATIVE_FIXTURE")
	if root == "" {
		t.Skip("set NSGO_MSIX_NATIVE_FIXTURE to the certificate signing test's public fixture directory")
	}
	verify := syscall.NewLazyDLL("wintrust.dll").NewProc("WinVerifyTrust")
	for name, want := range map[string]uint32{
		"signed.msix":   0x800b0109, // CERT_E_UNTRUSTEDROOT
		"tampered.msix": 0x80096010, // TRUST_E_BAD_DIGEST
		"unsigned.msix": 0x800b0100, // TRUST_E_NOSIGNATURE
	} {
		t.Run(name, func(t *testing.T) {
			path, err := syscall.UTF16PtrFromString(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			action := msixTrustGUID{0x00aac56b, 0xcd44, 0x11d0, [8]byte{0x8c, 0xc2, 0, 0xc0, 0x4f, 0xc2, 0x95, 0xee}}
			file := msixTrustFile{Name: path}
			file.Size = uint32(unsafe.Sizeof(file))
			data := msixTrustData{UI: 2, Choice: 1, File: &file, Action: 1, Flags: 0x1010}
			data.Size = uint32(unsafe.Sizeof(data))
			var pins runtime.Pinner
			pins.Pin(path)
			pins.Pin(&action)
			pins.Pin(&file)
			pins.Pin(&data)
			defer pins.Unpin()
			got, _, _ := verify.Call(^uintptr(0), uintptr(unsafe.Pointer(&action)), uintptr(unsafe.Pointer(&data)))
			data.Action = 2 // WTD_STATEACTION_CLOSE releases the verification state.
			verify.Call(^uintptr(0), uintptr(unsafe.Pointer(&action)), uintptr(unsafe.Pointer(&data)))
			if uint32(got) != want {
				t.Fatalf("WinVerifyTrust = %#08x, want %#08x", uint32(got), want)
			}
		})
	}
}
