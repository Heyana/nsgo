//go:build !gallery_group || gallery_navigation

package main

import (
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func init() {
	type m = locales.Messages
	registerGroup("components", 8, "Navigation", m.GroupNavigation, []demo{
		{"Tabs", m.DemoTabs, m.DemoTabsDetail, lucide.NotebookTabs, cardsDemo(tabsCard)},
		{"Breadcrumbs", m.DemoBreadcrumbs, m.DemoBreadcrumbsDetail, lucide.ChevronRight, cardsDemo(breadcrumbsCard, paginationCard, stepperPlayground)},
		{"Sidebar", m.DemoSidebar, m.DemoSidebarDetail, lucide.PanelLeft, cardsDemo(sidebarCard)},
		{"NavStack", m.DemoNavStack, m.DemoNavStackDetail, lucide.PanelsTopLeft, navigationDemo},
	})
}
