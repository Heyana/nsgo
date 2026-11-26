//go:build !gallery_group || gallery_actions

package main

import (
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func init() {
	type m = locales.Messages
	registerGroup("components", 1, "Actions", m.GroupActions, []demo{
		{"Button", m.DemoButton, m.DemoButtonDetail, lucide.MousePointerClick, cardsDemo(buttonCard, iconButtonCard, toggleButtonCard, toggleGroupCard, holdPlayground)},
		{"Link", m.DemoLink, m.DemoLinkDetail, lucide.Link, cardsDemo(linkCard)},
		{"Toolbar", m.DemoToolbar, m.DemoToolbarDetail, lucide.PanelTop, cardsDemo(toolbarCard)},
	})
}
