//go:build ios && !tvos && nsgotest

package ios

import (
	"math"
	"strings"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestNativeHeaderTitleAttributes(t *testing.T) {
	onMainSync(func() {
		if attrs := headerTitleAttributes(0, driver.Font{}, false); attrs != 0 {
			t.Error("unset title style replaced UIKit defaults")
		}
		for _, large := range []bool{false, true} {
			attrs := headerTitleAttributes(driver.Color(0x224466ff), driver.Font{
				Family: "Georgia", Size: 24, Weight: driver.FontWeightBold, Italic: true,
			}, large)
			font := attrs.Send(sel("objectForKey:"), apple.Global(uikitPath, "NSFontAttributeName"))
			color := attrs.Send(sel("objectForKey:"), apple.Global(uikitPath, "NSForegroundColorAttributeName"))
			if font == 0 || color == 0 {
				t.Fatalf("large=%t: font %v, color %v", large, font, color)
			}
			if family := apple.GoString(font.Send(sel("familyName"))); family != "Georgia" {
				t.Errorf("large=%t family %q", large, family)
			}
			name := apple.GoString(font.Send(sel("fontName")))
			if !strings.Contains(name, "Bold") || !strings.Contains(name, "Italic") {
				t.Errorf("large=%t lost weight/slant: %q", large, name)
			}
			style, base := "UIFontTextStyleHeadline", float64(17)
			if large {
				style, base = "UIFontTextStyleLargeTitle", 34
			}
			preferred := objc.ID(apple.Class("UIFont")).Send(sel("preferredFontForTextStyle:"), apple.Global(uikitPath, style))
			want := 24 * objc.Send[float64](preferred, sel("pointSize")) / base
			if size := objc.Send[float64](font, sel("pointSize")); math.Abs(size-want) > .01 {
				t.Errorf("large=%t size %.2f, want %.2f", large, size, want)
			}
		}
	})
}
