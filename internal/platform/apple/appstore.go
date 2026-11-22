//go:build darwin || ios

package apple

import (
	"context"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
)

// CheckAppStore is the shared Apple path for updater.Check: capture the
// installed bundle's identity, version and storefront country on the main
// thread, then query the store off it. The Mac's lookup selects Mac software.
func CheckAppStore(c updates.Config, mac bool, id uint32, dispatch func(driver.Event)) {
	bundle := objc.ID(Class("NSBundle")).Send(Sel("mainBundle"))
	if v := bundle.Send(Sel("bundleIdentifier")); v != 0 {
		c.ID = GoString(v)
	}
	if v := bundle.Send(Sel("objectForInfoDictionaryKey:"), NSString("CFBundleShortVersionString")); v != 0 {
		c.Version = GoString(v)
	}
	country := ""
	if v := objc.ID(Class("NSLocale")).Send(Sel("currentLocale")).Send(Sel("countryCode")); v != 0 {
		country = GoString(v)
	}
	updates.Answer(id, func(ctx context.Context) driver.Update {
		if mac {
			return updates.FindInMacAppStore(ctx, c, country)
		}
		return updates.FindInAppStore(ctx, c, country)
	}, dispatch)
}
