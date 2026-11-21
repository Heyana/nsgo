//go:build js && wasm

package web

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestBarcodeFormatsMapping(t *testing.T) {
	if got := barcodeFormats(driver.ScanRequest{}); !slices.Equal(got, []string{"qr_code"}) {
		t.Errorf("default formats = %v, want [qr_code]", got)
	}
	got := barcodeFormats(driver.ScanRequest{Kinds: []driver.CodeKind{driver.CodeEAN13, driver.CodeCode128, driver.CodePDF417}})
	want := []string{"pdf417", "ean_13", "upc_a", "code_128"}
	if !slices.Equal(got, want) {
		t.Errorf("formats = %v, want %v", got, want)
	}
	filtered := filterBarcodeFormats(want, []string{"qr_code", "ean_13", "code_128"})
	if !slices.Equal(filtered, []string{"ean_13", "code_128"}) {
		t.Errorf("filtered = %v, want [ean_13 code_128]", filtered)
	}
}

func TestBarcodeFromDetected(t *testing.T) {
	all := driver.ScanRequest{Kinds: []driver.CodeKind{
		driver.CodeQR, driver.CodeAztec, driver.CodeDataMatrix, driver.CodePDF417,
		driver.CodeEAN13, driver.CodeEAN8, driver.CodeUPCE, driver.CodeCode128,
		driver.CodeCode39, driver.CodeCode93, driver.CodeITF,
	}}
	cases := []struct {
		format, raw string
		wantKind    driver.CodeKind
		wantText    string
	}{
		{"qr_code", "https://nsgo.dev", driver.CodeQR, "https://nsgo.dev"},
		{"aztec", "AZ-1", driver.CodeAztec, "AZ-1"},
		{"data_matrix", "DM-2", driver.CodeDataMatrix, "DM-2"},
		{"pdf417", "PDF-3", driver.CodePDF417, "PDF-3"},
		{"ean_13", "4006381333931", driver.CodeEAN13, "4006381333931"},
		{"upc_a", "036000291452", driver.CodeEAN13, "0036000291452"},
		{"ean_8", "96385074", driver.CodeEAN8, "96385074"},
		{"upc_e", "01234565", driver.CodeUPCE, "01234565"},
		{"code_128", "ABC-123", driver.CodeCode128, "ABC-123"},
		{"code_39", "CODE39", driver.CodeCode39, "CODE39"},
		{"code_93", "CODE93", driver.CodeCode93, "CODE93"},
		{"itf", "12345678", driver.CodeITF, "12345678"},
	}
	for _, tc := range cases {
		got := barcodeFromDetected(tc.format, tc.raw, all)
		if got == nil || got.Kind != tc.wantKind || got.Text != tc.wantText {
			t.Errorf("barcodeFromDetected(%q, %q) = %+v, want {%v %q}", tc.format, tc.raw, got, tc.wantKind, tc.wantText)
		}
	}
	// Empty value, unknown format, or unwanted kind return nil.
	if got := barcodeFromDetected("qr_code", "", all); got != nil {
		t.Errorf("empty value returned %+v", got)
	}
	if got := barcodeFromDetected("codabar", "12345", all); got != nil {
		t.Errorf("unknown format returned %+v", got)
	}
	if got := barcodeFromDetected("ean_13", "4006381333931", driver.ScanRequest{}); got != nil {
		t.Errorf("unrequested EAN-13 returned %+v", got)
	}
}

func TestScanErrorMessage(t *testing.T) {
	if got := scanErrorMessage("NotAllowedError", "raw"); got != "camera access denied" {
		t.Errorf("NotAllowedError = %q", got)
	}
	if got := scanErrorMessage("NotFoundError", "raw"); got != "this device has no camera" {
		t.Errorf("NotFoundError = %q", got)
	}
	if got := scanErrorMessage("AbortError", "stream aborted"); got != "stream aborted" {
		t.Errorf("AbortError = %q", got)
	}
}
