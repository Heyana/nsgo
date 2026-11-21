package driver

// CodeKind is a kind of code ItemScanCode reads: a QR code by default.
type CodeKind uint8

const (
	CodeQR CodeKind = iota
	CodeAztec
	CodeDataMatrix
	CodePDF417
	// CodeEAN13 is also a UPC-A code, which is an EAN-13 with a leading 0.
	CodeEAN13
	CodeEAN8
	CodeUPCE
	CodeCode128
	CodeCode39
	CodeCode93
	CodeITF
)

// NoScanCamera says that the camera became unavailable before scanning.
const NoScanCamera = "this device has no camera to scan a code with"

// ScanRequest is ItemScanCode's request: the kinds of code to read, a QR code alone when empty.
type ScanRequest struct {
	Kinds []CodeKind
}

// Wants reports whether r reads codes of kind k.
func (r ScanRequest) Wants(k CodeKind) bool {
	if len(r.Kinds) == 0 {
		return k == CodeQR
	}
	for _, w := range r.Kinds {
		if w == k {
			return true
		}
	}
	return false
}

// ScannedCode is a code ItemScanCode read: its kind and the text it holds.
type ScannedCode struct {
	Kind CodeKind
	Text string
}
