//go:build !gallery_group || gallery_interaction

package main

import (
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func init() {
	type m = locales.Messages
	registerGroup("basics", 2, "Interaction", m.GroupInteraction, []demo{
		{"Animations", m.DemoAnimations, m.DemoAnimationsDetail, lucide.Zap, animationsDemo},
		{"Gestures", m.DemoGestures, m.DemoGesturesDetail, lucide.Hand, gesturesDemo},
		{"Physics", m.DemoPhysics, m.DemoPhysicsDetail, lucide.Atom, physicsDemo},
		{"Collapsing header", m.DemoCollapsingHeader, m.DemoCollapsingHeaderDetail, lucide.PanelTop, collapsingDemo},
		{"Patterns", m.DemoPatterns, m.DemoPatternsDetail, lucide.Component, componentsDemo},
	})
}
