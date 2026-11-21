//go:build darwin

package apple

import (
	"slices"
	"sync"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMetadataTypesOfAScan(t *testing.T) {
	if got := MetadataTypes(driver.ScanRequest{}); !slices.Equal(got, []string{"org.iso.QRCode"}) {
		t.Errorf("a default scan reads %v, want a QR code alone", got)
	}
	got := MetadataTypes(driver.ScanRequest{Kinds: []driver.CodeKind{driver.CodeEAN13, driver.CodeITF}})
	if !slices.Equal(got, []string{"org.gs1.EAN-13", "org.ansi.Interleaved2of5"}) {
		t.Errorf("EAN-13 and ITF read %v", got)
	}
	for k := driver.CodeQR; k <= driver.CodeITF; k++ {
		if back, ok := MetadataKind(metadataTypes[k]); !ok || back != k {
			t.Errorf("kind %d does not read back from %q", k, metadataTypes[k])
		}
	}
	if _, ok := MetadataKind("face"); ok {
		t.Error("a face reads as a code")
	}
}

type metadataSample struct {
	kind driver.CodeKind
	text string
}

var metadataSamples = map[objc.ID]metadataSample{}

var metadataSampleClass = sync.OnceValue(func() objc.Class {
	return RegisterClass("NsgoScanMetadataSample", Class("NSObject"), nil, []objc.MethodDef{{
		Cmd: sel("type"),
		Fn:  func(self objc.ID, _ objc.SEL) objc.ID { return NSString(metadataTypes[metadataSamples[self].kind]) },
	}, {
		Cmd: sel("stringValue"),
		Fn: func(self objc.ID, _ objc.SEL) objc.ID {
			if text := metadataSamples[self].text; text != "" {
				return NSString(text)
			}
			return 0
		},
	}})
})

func TestCodeFromMetadataReadsTheFirstRequestedText(t *testing.T) {
	withPool(func() {
		objects := objc.ID(Class("NSMutableArray")).Send(sel("array"))
		for _, sample := range []metadataSample{
			{kind: driver.CodeCode128, text: "other format"},
			{kind: driver.CodeQR}, // binary code has no string value
			{kind: driver.CodeQR, text: "first"},
			{kind: driver.CodeQR, text: "second"},
		} {
			o := objc.ID(metadataSampleClass()).Send(sel("new"))
			metadataSamples[o] = sample
			defer delete(metadataSamples, o)
			objects.Send(sel("addObject:"), o)
			o.Send(sel("release"))
		}
		code := CodeFromMetadata(objects, driver.ScanRequest{})
		if code == nil || *code != (driver.ScannedCode{Kind: driver.CodeQR, Text: "first"}) {
			t.Fatalf("code = %+v, want the first text QR code", code)
		}
		if code := CodeFromMetadata(objects, driver.ScanRequest{Kinds: []driver.CodeKind{driver.CodeEAN8}}); code != nil {
			t.Fatalf("EAN-8 scan found %+v", code)
		}
	})
}
