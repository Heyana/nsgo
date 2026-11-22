//go:build darwin && !ios

package macos

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestPastePreservesGIF(t *testing.T) {
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 1, 1), palette)
	second := image.NewPaletted(first.Bounds(), palette)
	second.SetColorIndex(0, 0, 1)
	var animated, still bytes.Buffer
	if err := gif.EncodeAll(&animated, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{10, 10}}); err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(&still, first); err != nil {
		t.Fatal(err)
	}
	onMain(func() {
		loadAppKit()
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))
		pb := objc.ID(apple.Class("NSPasteboard")).Send(sel("pasteboardWithUniqueName"))
		if pb == 0 {
			t.Fatal("could not open a unique pasteboard")
		}
		defer pb.Send(sel("releaseGlobally"))
		writeType := func(uti string, data []byte) {
			t.Helper()
			if !objc.Send[bool](pb, sel("setData:forType:"), apple.NSData(data), apple.NSString(uti)) {
				t.Fatalf("could not write %s to the pasteboard", uti)
			}
		}
		checkPaste := func(mime string, data []byte) {
			t.Helper()
			files := pasteFrom(pb).Files
			if len(files) != 1 || files[0].MIME != mime || !bytes.Equal(files[0].Data, data) {
				t.Fatalf("paste did not report the original %s", mime)
			}
		}
		pb.Send(sel("clearContents"))
		writeType("public.png", still.Bytes())
		checkPaste("image/png", still.Bytes())
		writeType("com.compuserve.gif", animated.Bytes())
		checkPaste("image/gif", animated.Bytes())
	})
}
