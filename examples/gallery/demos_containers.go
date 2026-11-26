//go:build !gallery_group || gallery_containers

package main

import (
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func init() {
	type m = locales.Messages
	registerGroup("components", 5, "Containers", m.GroupContainers, []demo{
		{"Accordion", m.DemoAccordion, m.DemoAccordionDetail, lucide.Rows3, cardsDemo(accordionCard, collapsiblePlayground)},
		{"Resizable", m.DemoResizable, m.DemoResizableDetail, lucide.PanelLeft, resizableDemo},
		{"Dock", m.DemoDock, m.DemoDockDetail, lucide.AppWindow, dockDemo},
		{"Camera", m.DemoCamera, m.DemoCameraDetail, lucide.Move, cameraDemo},
	})
}
