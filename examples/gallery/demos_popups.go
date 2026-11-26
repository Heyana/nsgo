//go:build !gallery_group || gallery_popups

package main

import (
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func init() {
	type m = locales.Messages
	registerGroup("components", 7, "Popups", m.GroupPopups, []demo{
		{"Dialog", m.DemoDialog, m.DemoDialogDetail, lucide.AppWindow, cardsDemo(dialogCard, alertDialogCard, sheetCard)},
		{"Popover", m.DemoPopover, m.DemoPopoverDetail, lucide.MessageSquare, cardsDemo(popoverPlayground, tooltipPlayground, hoverCardPlayground)},
		{"Command", m.DemoCommand, m.DemoCommandDetail, lucide.Search, cardsDemo(commandPlayground)},
		{"Menu", m.DemoMenu, m.DemoMenuDetail, lucide.Menu, menusDemo},
	})
}
