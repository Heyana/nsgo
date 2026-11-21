package core

import (
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestScanRequestWantsAQRCodeByDefault(t *testing.T) {
	if !(driver.ScanRequest{}).Wants(CodeQR) || (driver.ScanRequest{}).Wants(CodeEAN13) {
		t.Error("an empty request does not want a QR code alone")
	}
	r := driver.ScanRequest{Kinds: []CodeKind{CodeEAN13, CodeCode128}}
	if r.Wants(CodeQR) || !r.Wants(CodeCode128) {
		t.Errorf("%+v wants the wrong kinds", r)
	}
}

func TestScanCode(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	h.commit()
	var got *ScannedCode
	var gotErr error
	answered := 0
	scan := func(c *ScannedCode, err error) { got, gotErr = c, err; answered++ }
	cx.ScanCode(ScanOptions{}, scan)
	if answered != 1 || !errors.Is(gotErr, ErrNoScanner) || command(h.commit(), driver.ItemScanCode) != nil {
		t.Fatalf("without a scanner: %+v, %v", got, gotErr)
	}
	h.tree.SetEnvironment(Environment{ScanCode: true})
	h.commit()
	kinds := []CodeKind{CodeQR, CodeEAN13}
	cx.ScanCode(ScanOptions{Kinds: kinds}, scan)
	kinds[1] = CodeAztec
	it := command(h.commit(), driver.ItemScanCode)
	if it == nil || len(it.Scan.Kinds) != 2 || it.Scan.Kinds[1] != CodeEAN13 {
		t.Fatalf("scan = %+v, the request must keep its own kinds", it)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventPick, Count: it.Count, Data: &driver.ScannedCode{Kind: CodeQR, Text: "myapp://pair?c=1"}})
	if answered != 2 || gotErr != nil || got == nil || got.Text != "myapp://pair?c=1" || got.Kind != CodeQR {
		t.Fatalf("code read = %+v, %v", got, gotErr)
	}
	cx.ScanCode(ScanOptions{}, scan)
	it = command(h.commit(), driver.ItemScanCode)
	h.tree.Dispatch(driver.Event{Kind: driver.EventPick, Count: it.Count})
	if answered != 3 || got != nil || gotErr != nil {
		t.Fatalf("a close answers %+v, %v", got, gotErr)
	}
	cx.ScanCode(ScanOptions{}, scan)
	it = command(h.commit(), driver.ItemScanCode)
	h.tree.Dispatch(driver.Event{Kind: driver.EventPick, Count: it.Count, Text: "camera access denied"})
	if answered != 4 || got != nil || gotErr == nil || gotErr.Error() != "camera access denied" {
		t.Fatalf("a failure answers %+v, %v", got, gotErr)
	}
	cx.ScanCode(ScanOptions{}, scan)
	it = command(h.commit(), driver.ItemScanCode)
	h.tree.Dispatch(driver.Event{Kind: driver.EventPick, Count: it.Count, Text: driver.NoScanCamera})
	if answered != 5 || got != nil || !errors.Is(gotErr, ErrNoScanner) {
		t.Fatalf("a disconnected camera answers %+v, %v", got, gotErr)
	}
}
