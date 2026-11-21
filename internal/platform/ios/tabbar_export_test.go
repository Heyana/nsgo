//go:build ios && nsgotest

package ios

import "github.com/nsgo-dev/nsgo/internal/platform/apple"

// TabBadgeForTest reads the tab's identity, badge and assigned accessibility value from UIKit.
func TabBadgeForTest(index int) (item uintptr, badge, value string) {
	onMainSync(func() {
		for _, bar := range tabBars {
			if index >= len(bar.items) {
				continue
			}
			it := bar.items[index]
			item = uintptr(it)
			badge = apple.GoString(it.Send(sel("badgeValue")))
			value = apple.GoString(it.Send(sel("accessibilityValue")))
			return
		}
	})
	return
}
