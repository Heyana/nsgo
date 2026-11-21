package base_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestColorEyeDropperDesktopPlatforms(t *testing.T) {
	cases := []struct {
		name string
		env  core.Environment
		want driver.Color
	}{
		{
			name: "windows",
			env:  core.Environment{OS: core.OSWindows, Idiom: core.IdiomDesktop, ScreenColor: true},
			want: core.SetColor(0x123456ff),
		},
		{
			name: "linux with screenshot portal",
			env:  core.Environment{OS: core.OSLinux, Idiom: core.IdiomDesktop, ScreenColor: true},
			want: core.SetColor(0x654321ff),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var picked, ended core.Color
			app := nsgotest.Start(t, func(*core.Scope) core.Node {
				return base.ColorPicker{
					DefaultValue: core.SetColor(0xff0000ff),
					OnChangeEnd:  func(c core.Color, _ string) { ended = c },
					Children: []core.Node{
						base.ColorEyeDropper{
							TestID: "dropper",
							Style:  core.Style{Width: core.Pt(40), Height: core.Pt(40)},
							OnPick: func(c core.Color) { picked = c },
						},
					},
				}
			}, nsgotest.Environment(tc.env))

			btn := app.FindMounted("id:dropper")
			if btn.Props.Disabled {
				t.Fatal("ColorEyeDropper is disabled when ScreenColor is true")
			}

			// A cancelled pick leaves the picker unchanged and allows picking again.
			app.PickScreenColor = func() (driver.Color, bool) { return 0, false }
			app.Press("id:dropper")
			if picked != 0 || ended != 0 {
				t.Fatalf("cancelled pick changed color: picked=%x ended=%x", picked, ended)
			}

			app.PickScreenColor = func() (driver.Color, bool) { return tc.want, true }
			app.Press("id:dropper")
			if picked != tc.want || ended != tc.want {
				t.Fatalf("picked=%x ended=%x, want %x", picked, ended, tc.want)
			}
		})
	}
}

func TestColorEyeDropperFollowsPortalCapability(t *testing.T) {
	env := core.Environment{OS: core.OSLinux, Idiom: core.IdiomDesktop, ScreenColor: false}
	var picked core.Color
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return base.ColorEyeDropper{
			TestID: "dropper",
			Style:  core.Style{Width: core.Pt(40), Height: core.Pt(40)},
			OnPick: func(c core.Color) { picked = c },
		}
	}, nsgotest.Environment(env))

	if !app.FindMounted("id:dropper").Props.Disabled {
		t.Fatal("ColorEyeDropper should be disabled when ScreenColor is false")
	}

	env.ScreenColor = true
	app.SetEnvironment(env)
	app.Settle()

	if app.FindMounted("id:dropper").Props.Disabled {
		t.Fatal("ColorEyeDropper should enable once ScreenColor becomes true")
	}
	app.PickScreenColor = func() (driver.Color, bool) { return core.SetColor(0x00abcdef), true }
	app.Press("id:dropper")
	if picked != core.SetColor(0x00abcdef) {
		t.Fatalf("picked = %x, want 00abcdef", picked)
	}
}
