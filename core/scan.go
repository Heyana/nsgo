package core

import (
	"errors"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// CodeKind is a kind of code ScanCode reads.
type CodeKind = driver.CodeKind

// The kinds of code ScanCode reads.
const (
	CodeQR         = driver.CodeQR
	CodeAztec      = driver.CodeAztec
	CodeDataMatrix = driver.CodeDataMatrix
	CodePDF417     = driver.CodePDF417
	// CodeEAN13 is also a UPC-A code, which reads as an EAN-13 with a leading 0.
	CodeEAN13   = driver.CodeEAN13
	CodeEAN8    = driver.CodeEAN8
	CodeUPCE    = driver.CodeUPCE
	CodeCode128 = driver.CodeCode128
	CodeCode39  = driver.CodeCode39
	CodeCode93  = driver.CodeCode93
	CodeITF     = driver.CodeITF
)

// ScannedCode is a code ScanCode read: its kind and the text it holds.
type ScannedCode = driver.ScannedCode

// ScanOptions say which codes ScanCode reads.
type ScanOptions struct {
	// Kinds are the kinds of code to read; a QR code alone when empty.
	Kinds []CodeKind
}

// ErrNoScanner is ScanCode's failure where Env.CanScanCode is false.
var ErrNoScanner = errors.New("core: no camera to scan a code with")

// ScanCode shows the camera for the user to point at a code, and calls done with the first one it reads of the kinds o asks for, as the camera sees it, or with nil when the user closes it; err says what failed, such as a refused permission or ErrNoScanner. macOS and iOS read codes with AVFoundation's metadata output and need NSCameraUsageDescription in the app's Info.plist. On macOS the preview appears in an AppKit sheet when the app has a visible window. On iOS it fills the screen. Android shows Camera2's preview and reads its frames in Go, every kind but PDF417, and needs CAMERA in the app's manifest (nsgo.yaml's android.permissions). A code holds whatever whoever printed it chose, so treat its text as untrusted input.
func (cx *Scope) ScanCode(o ScanOptions, done func(code *ScannedCode, err error)) {
	t := cx.tree()
	if t == nil || cx.denied(servicePicker, "") {
		return
	}
	if !Untracked(cx, cx.Env().CanScanCode) {
		if done != nil {
			done(nil, ErrNoScanner)
		}
		return
	}
	r := driver.ScanRequest{Kinds: append([]CodeKind(nil), o.Kinds...)}
	t.request(cx, driver.MountItem{Kind: driver.ItemScanCode, Scan: &r}, func(ev driver.Event) {
		if done == nil {
			return
		}
		if ev.Text == driver.NoScanCamera {
			done(nil, ErrNoScanner)
			return
		}
		if ev.Text != "" {
			done(nil, errors.New(ev.Text))
			return
		}
		code, _ := ev.Data.(*driver.ScannedCode)
		done(code, nil)
	})
}
