package nsgotest

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ScreenConfig records the header and navigation settings of a mounted screen.
type ScreenConfig struct {
	Title string
	// HeaderHidden hides the header and sets HeaderHeight to zero.
	HeaderHidden      bool
	HeaderTransparent bool
	LargeTitle        bool
	// CanGoBack reports a preceding screen. BackHidden and BackTitle
	// configure its back button.
	CanGoBack  bool
	BackHidden bool
	BackTitle  string
	// OwnBackTitle is the screen's own HeaderBackTitle: the label of the
	// back button to it on the screen above, which that screen's
	// BackTitle shows.
	OwnBackTitle string
	// NoPopOnSwipe disables swipe navigation. NoPopOnBack lets the app
	// handle a back press through its callback.
	NoPopOnSwipe bool
	NoPopOnBack  bool
	// Background, TitleColor and TintColor select the header colors.
	// Zero uses the platform defaults.
	Background, TitleColor, TintColor core.Color
	// Left, Right and TitleView are the header views, each a layout root
	// at its content's size that no view holds, or 0.
	Left, Right, TitleView core.NodeID
	// Header identifies the screen's custom header view.
	Header core.NodeID
	// RightSystemItem, with a Right view, is the UIKit button iOS shows
	// in Right's place. A press on it presses Right's first view that
	// takes presses.
	RightSystemItem core.HeaderSystemItem
	// RightSymbols, with a Right view, are the UIKit buttons iOS shows in
	// its place, a line each of an SF Symbol's name, a tab and its label. A
	// press on one presses that of Right's views that take presses.
	RightSymbols string
	// HeaderHeight is the area the header covers from the screen's top:
	// the bar and the part of the status bar the screen reaches under.
	HeaderHeight core.Points
	// Animation is how the screen enters and leaves.
	Animation core.ScreenAnimation
	// TitleFont and LargeTitleFont override the native title fonts; zero fields keep their platform defaults.
	TitleFont, LargeTitleFont core.Font
	// RightWithLargeTitle moves Android's trailing view beside its large title, returning to the toolbar as the header collapses.
	RightWithLargeTitle bool
	// TitleFills lets TitleView expand to fill the room between the bar's ends.
	TitleFills bool
}

// Equal reports whether a and b describe the same mounted screen configuration.
func (a ScreenConfig) Equal(b ScreenConfig) bool {
	return a.Title == b.Title && a.HeaderHidden == b.HeaderHidden &&
		a.HeaderTransparent == b.HeaderTransparent && a.LargeTitle == b.LargeTitle &&
		a.CanGoBack == b.CanGoBack && a.BackHidden == b.BackHidden &&
		a.BackTitle == b.BackTitle && a.OwnBackTitle == b.OwnBackTitle &&
		a.NoPopOnSwipe == b.NoPopOnSwipe && a.NoPopOnBack == b.NoPopOnBack &&
		a.Background == b.Background && a.TitleColor == b.TitleColor &&
		a.TintColor == b.TintColor && a.Left == b.Left && a.Right == b.Right &&
		a.TitleView == b.TitleView && a.Header == b.Header &&
		a.RightSystemItem == b.RightSystemItem && a.RightSymbols == b.RightSymbols &&
		a.HeaderHeight == b.HeaderHeight && a.Animation == b.Animation &&
		a.TitleFont.Equal(b.TitleFont) && a.LargeTitleFont.Equal(b.LargeTitleFont) &&
		a.RightWithLargeTitle == b.RightWithLargeTitle && a.TitleFills == b.TitleFills
}

func screenConfig(c driver.ScreenConfig) ScreenConfig {
	return ScreenConfig{
		Title:               c.Title,
		HeaderHidden:        c.HeaderHidden,
		HeaderTransparent:   c.HeaderTransparent,
		LargeTitle:          c.LargeTitle,
		CanGoBack:           c.CanGoBack,
		BackHidden:          c.BackHidden,
		BackTitle:           c.BackTitle,
		OwnBackTitle:        c.OwnBackTitle,
		NoPopOnSwipe:        c.NoPopOnSwipe,
		NoPopOnBack:         c.NoPopOnBack,
		Background:          c.Background,
		TitleColor:          c.TitleColor,
		TintColor:           c.TintColor,
		Left:                c.Left,
		Right:               c.Right,
		TitleView:           c.TitleView,
		Header:              c.Header,
		RightSystemItem:     c.RightSystemItem,
		RightSymbols:        c.RightSymbols,
		HeaderHeight:        c.HeaderHeight,
		Animation:           c.Animation,
		TitleFont:           c.TitleFont,
		LargeTitleFont:      c.LargeTitleFont,
		RightWithLargeTitle: c.RightWithLargeTitle,
		TitleFills:          c.TitleFills,
	}
}
