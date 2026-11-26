//go:build !gallery_group || gallery_styling

package main

import (
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func init() {
	type m = locales.Messages
	registerGroup("basics", 1, "Styling", m.GroupStyling, []demo{
		{"Styles", m.DemoStyles, m.DemoStylesDetail, lucide.Palette, stylesDemo},
		{"Layout", m.DemoLayout, m.DemoLayoutDetail, lucide.Columns2, layoutDemo},
	})
}
