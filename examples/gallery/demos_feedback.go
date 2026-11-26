//go:build !gallery_group || gallery_feedback

package main

import (
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func init() {
	type m = locales.Messages
	registerGroup("components", 6, "Feedback", m.GroupFeedback, []demo{
		{"Progress", m.DemoProgress, m.DemoProgressDetail, lucide.LoaderCircle, cardsDemo(progressCard, progressRingCard, spinnerPlayground, meterCard, skeletonCard)},
		{"Toast", m.DemoToast, m.DemoToastDetail, lucide.Bell, cardsDemo(toastPlayground)},
	})
}
