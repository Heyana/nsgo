//go:build darwin && !ios

package macos

import (
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestScrollIndicatorPointer(t *testing.T) {
	for _, tc := range []struct {
		name, preference string
		x                driver.Points
		reveal           bool
	}{
		{"content", "Automatic", 50, false},
		{"track", "Automatic", 96, true},
		{"when-scrolling", "WhenScrolling", 96, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			onMain(func() {
				loadAppKit()
				// Override the preference for this process, without writing to disk.
				defaults := objc.ID(apple.Class("NSUserDefaults")).Send(sel("standardUserDefaults"))
				domain := apple.NSString("NSArgumentDomain")
				old := defaults.Send(sel("volatileDomainForName:"), domain).Send(sel("retain"))
				args := old.Send(sel("mutableCopy"))
				args.Send(sel("setObject:forKey:"), apple.NSString(tc.preference), apple.NSString("AppleShowScrollBars"))
				defaults.Send(sel("removeVolatileDomainForName:"), domain)
				defaults.Send(sel("setVolatileDomain:forName:"), args, domain)
				args.Send(selRelease)
				cached := whenScrolling
				whenScrolling.read = time.Time{}
				defer func() {
					defaults.Send(sel("removeVolatileDomainForName:"), domain)
					defaults.Send(sel("setVolatileDomain:forName:"), old, domain)
					old.Send(selRelease)
					whenScrolling = cached
				}()

				s := &surface{native: objc.ID(classView).Send(selNew), animator: driver.NewAnimator(nil), inside: map[*view]struct{}{}}
				defer s.native.Send(selRelease)
				defer func() { s.closed = true }()
				v := s.create(driver.KindScroll, 1)
				defer v.native.Send(selRelease)
				defer delete(views, v.native)
				s.views = []*view{nil, v}
				v.frame = driver.Bounds{Size: driver.Size{Width: 100, Height: 200}}
				v.native.Send(sel("setFrame:"), apple.Rect(10, 20, 100, 200))
				v.sc.content = driver.Size{Width: 100, Height: 1000}
				v.sc.ticking = true

				loc := objc.Send[apple.CGPoint](v.native, sel("convertPoint:toView:"), apple.CGPoint{X: float64(tc.x), Y: 160}, objc.ID(0))
				event := objc.ID(apple.Class("NSEvent")).Send(
					sel("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
					uint(evMoved), loc, uint(0), uptime(), 0, objc.ID(0), 0, 0, float32(0))
				if event == 0 {
					t.Fatal("could not make a mouse event")
				}
				check := func(what string, want bool) {
					t.Helper()
					if shown := v.sc.thumb != 0 && !v.sc.drawn.hidden; shown != want {
						t.Errorf("%s: indicator visible = %v, want %v", what, shown, want)
					}
				}
				fade := func() { v.tickScroll(mediaTime() + time.Second) }
				mouseEntered(v.native, 0, event)
				check("pointer entered", tc.reveal)
				fade()
				mouseMoved(v.native, 0, event)
				check("pointer moved", tc.reveal)
				if tc.name == "content" {
					return
				}
				if tc.preference == "WhenScrolling" {
					v.wheelBy(0, 20, mediaTime())
					check("wheel scrolled", true)
				}
				fade()
				check("idle", false)
				thumb, _, ok := v.thumbRect()
				if !ok || !v.grabsThumb(thumb.Origin) {
					t.Fatal("could not grab the faded thumb")
				}
				if v.grabsThumb(driver.Point{X: tc.x, Y: 160}) {
					t.Fatal("press on the track away from the thumb grabbed it")
				}
				v.thumbBegin()
				check("thumb grabbed", true)
				before := v.pos()
				v.thumbMove(driver.Point{Y: 20})
				if v.pos() <= before {
					t.Error("dragging the thumb did not scroll")
				}
				v.thumbEnd()
			})
		})
	}
}
