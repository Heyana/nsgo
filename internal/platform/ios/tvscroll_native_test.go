//go:build ios && nsgotest

package ios_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	platformios "github.com/nsgo-dev/nsgo/internal/platform/ios"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestTVRemoteScrollConfiguration verifies that on tvOS a Scroll or List whose
// content has no focusable views enables UITouchTypeIndirect on its
// UIScrollView pan gesture recognizer and can become focused for Siri Remote
// swipes, while scroll views with focusable children preserve focus-driven
// scrolling and iOS leaves UIScrollView unchanged.
func TestTVRemoteScrollConfiguration(t *testing.T) {
	const wait = 5 * time.Second
	longText := strings.Repeat("Apple TV remote surface scrolling paragraph.\n", 80)
	var showButton *core.Signal[bool]

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		showButton = core.NewSignal(cx, false)
		return core.View{
			Style: core.Style{FlexGrow: 1, Direction: core.Column, Gap: 12, Padding: core.All(16)},
			Children: []core.Node{
				core.Scroll{
					TestID: "text-scroll",
					Style:  core.Style{Width: core.Pt(400), Height: core.Pt(180)},
					Children: []core.Node{
						core.Text{Content: longText},
						core.Show{
							When: showButton.Get,
							Then: func(cx *core.Scope) core.Node {
								return core.View{
									TestID:  "dynamic-btn",
									Style:   core.Style{Width: core.Pt(140), Height: core.Pt(44)},
									OnPress: func(core.PressEvent) {},
									Children: []core.Node{
										core.Text{Content: "Action"},
									},
								}
							},
						},
					},
				},
				core.Scroll{
					TestID:     "horiz-scroll",
					Horizontal: true,
					Style:      core.Style{Width: core.Pt(300), Height: core.Pt(80)},
					Children: []core.Node{
						core.View{
							Style:    core.Style{Width: core.Pt(900), Height: core.Pt(60)},
							Children: []core.Node{core.Text{Content: "Wide content"}},
						},
					},
				},
				core.List{
					TestID:        "text-list",
					Style:         core.Style{Width: core.Pt(400), Height: core.Pt(160)},
					EstimatedSize: 40,
					Count:         func() int { return 40 },
					Row: func(cx *core.Scope, row core.ListRow) core.Node {
						return core.View{
							Style: core.Style{Height: core.Pt(40)},
							Children: []core.Node{
								core.Text{ContentFunc: func() string { return fmt.Sprintf("Row %d", row.Index()) }},
							},
						}
					},
				},
				core.Scroll{
					TestID: "button-scroll",
					Style:  core.Style{Width: core.Pt(400), Height: core.Pt(140)},
					Children: []core.Node{
						core.View{
							TestID:   "item-1",
							Style:    core.Style{Width: core.Pt(200), Height: core.Pt(100)},
							OnPress:  func(core.PressEvent) {},
							Children: []core.Node{core.Text{Content: "Item 1"}},
						},
						core.View{
							TestID:   "item-2",
							Style:    core.Style{Width: core.Pt(200), Height: core.Pt(100)},
							OnPress:  func(core.PressEvent) {},
							Children: []core.Node{core.Text{Content: "Item 2"}},
						},
					},
				},
			},
		}
	})

	app.WaitFor("id:text-scroll", wait)
	app.WaitFor("id:horiz-scroll", wait)
	app.WaitFor("id:text-list", wait)
	app.WaitFor("id:button-scroll", wait)

	stateOf := func(id string) platformios.TVScrollState {
		t.Helper()
		var st platformios.TVScrollState
		app.WaitUntil(wait, func() bool {
			var ok bool
			st, ok = platformios.TVScrollStateForTest(id)
			return ok
		})
		return st
	}

	textScroll := stateOf("text-scroll")
	if !textScroll.OnTV {
		// On iOS / iPadOS, UIScrollView behavior must remain completely unchanged.
		if textScroll.IndirectPan || textScroll.CanBecomeFocused {
			t.Fatalf("iOS text-scroll unexpectedly enabled tvOS remote scroll: %+v", textScroll)
		}
		return
	}

	if !textScroll.IndirectPan || !textScroll.HasIndirectTouch || !textScroll.CanBecomeFocused {
		t.Fatalf("tvOS non-focusable Scroll = %+v, want IndirectPan, HasIndirectTouch and CanBecomeFocused", textScroll)
	}
	if horiz := stateOf("horiz-scroll"); !horiz.IndirectPan || !horiz.HasIndirectTouch || !horiz.CanBecomeFocused {
		t.Fatalf("tvOS horizontal non-focusable Scroll = %+v, want IndirectPan, HasIndirectTouch and CanBecomeFocused", horiz)
	}
	if list := stateOf("text-list"); !list.IndirectPan || !list.HasIndirectTouch || !list.CanBecomeFocused {
		t.Fatalf("tvOS non-focusable List = %+v, want IndirectPan, HasIndirectTouch and CanBecomeFocused", list)
	}
	if btnScroll := stateOf("button-scroll"); btnScroll.IndirectPan || btnScroll.HasIndirectTouch || btnScroll.CanBecomeFocused {
		t.Fatalf("tvOS Scroll with focusable buttons = %+v, want focus-driven scrolling without indirect pan", btnScroll)
	}

	// Scroll the non-focusable Scroll and List and confirm their offsets move.
	app.Scroll("id:text-scroll", 120)
	app.WaitUntil(wait, func() bool { return stateOf("text-scroll").Offset.Y > 0 })
	app.Scroll("id:text-list", 120)
	app.WaitUntil(wait, func() bool { return stateOf("text-list").Offset.Y > 0 })

	// Inserting a focusable button disables indirect pan; removing it restores remote-surface scrolling.
	app.Post(func() { showButton.Set(true) })
	app.WaitFor("id:dynamic-btn", wait)
	app.WaitUntil(wait, func() bool {
		st := stateOf("text-scroll")
		return !st.IndirectPan && !st.HasIndirectTouch && !st.CanBecomeFocused
	})
	app.Post(func() { showButton.Set(false) })
	app.WaitUntil(wait, func() bool {
		st := stateOf("text-scroll")
		return st.IndirectPan && st.HasIndirectTouch && st.CanBecomeFocused
	})
}
