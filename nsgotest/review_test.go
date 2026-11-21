package nsgotest

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

func TestRequestReviewSupportedTargets(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  core.Environment
	}{
		{"ios", core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone, CanRequestReview: true}},
		{"macos", core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, CanRequestReview: true}},
		{"android", core.Environment{OS: core.OSAndroid, Idiom: core.IdiomPhone, CanRequestReview: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := Start(t, func(cx *core.Scope) core.Node {
				return core.View{Children: []core.Node{
					core.Show{
						When: cx.Env().CanRequestReview,
						Then: func(*core.Scope) core.Node { return core.Text{Content: "review-supported"} },
						Else: func(*core.Scope) core.Node { return core.Text{Content: "review-unsupported"} },
					},
					core.View{
						TestID: "rate",
						Style:  core.Style{Width: core.Pt(80), Height: core.Pt(30)},
						OnPress: func(core.PressEvent) {
							cx.RequestReview()
						},
					},
				}}
			}, Environment(tc.env))

			app.Find("text:review-supported")
			if app.ReviewRequests != 0 {
				t.Fatalf("initial ReviewRequests = %d, want 0", app.ReviewRequests)
			}
			app.Press("id:rate")
			if app.ReviewRequests != 1 {
				t.Fatalf("ReviewRequests after first press = %d, want 1", app.ReviewRequests)
			}
			app.Press("id:rate")
			if app.ReviewRequests != 2 {
				t.Fatalf("ReviewRequests after second press = %d, want 2", app.ReviewRequests)
			}
		})
	}
}

func TestRequestReviewUnsupportedTargets(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  core.Environment
	}{
		{"windows", core.Environment{OS: core.OSWindows, Idiom: core.IdiomDesktop}},
		{"linux", core.Environment{OS: core.OSLinux, Idiom: core.IdiomDesktop}},
		{"web", core.Environment{OS: core.OSWeb, Idiom: core.IdiomDesktop}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pressed := false
			app := Start(t, func(cx *core.Scope) core.Node {
				return core.View{Children: []core.Node{
					core.Show{
						When: cx.Env().CanRequestReview,
						Then: func(*core.Scope) core.Node { return core.Text{Content: "review-supported"} },
						Else: func(*core.Scope) core.Node { return core.Text{Content: "review-unsupported"} },
					},
					core.View{
						TestID: "rate",
						Style:  core.Style{Width: core.Pt(80), Height: core.Pt(30)},
						OnPress: func(core.PressEvent) {
							pressed = true
							cx.RequestReview()
						},
					},
				}}
			}, Environment(tc.env))

			app.Find("text:review-unsupported")
			app.Press("id:rate")
			if !pressed {
				t.Fatal("press handler did not run")
			}
			if app.ReviewRequests != 0 {
				t.Fatalf("unsupported target recorded %d review requests, want 0", app.ReviewRequests)
			}
		})
	}
}

func TestRequestReviewEnvironmentToggle(t *testing.T) {
	app := Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.Show{
				When: cx.Env().CanRequestReview,
				Then: func(*core.Scope) core.Node { return core.Text{Content: "can-review"} },
				Else: func(*core.Scope) core.Node { return core.Text{Content: "no-review"} },
			},
			core.View{
				TestID:  "rate",
				Style:   core.Style{Width: core.Pt(80), Height: core.Pt(30)},
				OnPress: func(core.PressEvent) { cx.RequestReview() },
			},
		}}
	}, Environment(core.Environment{OS: core.OSWindows, Idiom: core.IdiomDesktop}))

	app.Find("text:no-review")
	app.Press("id:rate")
	if app.ReviewRequests != 0 {
		t.Fatalf("before toggle: ReviewRequests = %d, want 0", app.ReviewRequests)
	}

	app.SetEnvironment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, CanRequestReview: true})
	app.Find("text:can-review")
	app.Press("id:rate")
	if app.ReviewRequests != 1 {
		t.Fatalf("after toggle: ReviewRequests = %d, want 1", app.ReviewRequests)
	}
}
