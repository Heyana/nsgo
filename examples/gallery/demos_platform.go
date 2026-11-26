//go:build !gallery_group || gallery_platform

package main

import (
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func init() {
	type m = locales.Messages
	registerGroup("basics", 3, "Platform", m.GroupPlatform, []demo{
		{"Foldable display", m.DemoFold, m.DemoFoldDetail, lucide.Columns2, foldDemo},
		{"Mobile keyboard", m.DemoMobileKeyboard, m.DemoMobileKeyboardDetail, lucide.Keyboard, keyboardDemo},
		{"System", m.DemoSystem, m.DemoSystemDetail, lucide.Cpu, systemDemo},
		{"Storage", m.DemoStorage, m.DemoStorageDetail, lucide.Database, storageDemo},
		{"Widgets", m.DemoWidgets, m.DemoWidgetsDetail, lucide.LayoutDashboard, widgetsDemo},
		{"Accessibility", m.DemoAccessibility, m.DemoAccessibilityDetail, lucide.Accessibility, accessibilityDemo},
	})
}
