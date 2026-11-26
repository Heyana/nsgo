//go:build !gallery_group || gallery_display

package main

import (
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func init() {
	type m = locales.Messages
	registerGroup("components", 3, "Display", m.GroupDisplay, []demo{
		{"Badge", m.DemoBadge, m.DemoBadgeDetail, lucide.Tag, cardsDemo(badgeCard)},
		{"Avatar", m.DemoAvatar, m.DemoAvatarDetail, lucide.User, cardsDemo(avatarCard, avatarGroupCard)},
		{"Image", m.DemoImage, m.DemoImageDetail, lucide.Image, cardsDemo(imagePlayground, imageCompareCard)},
		{"Carousel", m.DemoCarousel, m.DemoCarouselDetail, lucide.GalleryHorizontal, cardsDemo(carouselCard)},
	})
}
