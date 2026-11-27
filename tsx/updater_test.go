package tsx_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/bundler"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
	"github.com/nsgo-dev/nsgo/updater"
)

func TestUpdaterDeclarationsIncluded(t *testing.T) {
	decl := tsx.Declarations()["types/nsgo/updater.d.ts"]
	for _, want := range []string{
		`export type UpdaterManaged = "unavailable" | "self" | "page" | "development" | "channel" | "store";`,
		`export type UpdatePhase = "idle" | "downloading" | "installing";`,
		"export interface UpdaterCapability",
		"export interface UpdaterState",
		"export declare function checkForUpdate(): void;",
		"export declare function checkInBackground(): void;",
		"export declare function checkDaily(): void;",
		"export declare function capability(): UpdaterCapability;",
		"export declare function status(): UpdaterState;",
		"export declare function setAutomaticChecks(on: boolean): void;",
		"export declare function setAutomaticDownloads(on: boolean): void;",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("types/nsgo/updater.d.ts missing %q", want)
		}
	}
}

func TestUpdaterCheckForUpdateFlows(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("synthetic update checks run headless only")
	}
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(`
import { mount, View, Text } from "nsgo";
import { checkForUpdate, checkInBackground, checkDaily } from "nsgo/updater";

function App() {
  return <View>
    <View testID="check" style={{ width: 80, height: 30 }} onPress={() => checkForUpdate()}><Text>Check</Text></View>
    <View testID="bg" style={{ width: 80, height: 30 }} onPress={() => checkInBackground()}><Text>Quietly</Text></View>
    <View testID="daily" style={{ width: 80, height: 30 }} onPress={() => checkDaily()}><Text>Daily</Text></View>
  </View>;
}
mount(App);
`)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("available_opens_url", func(t *testing.T) {
		app := nsgotest.Start(t, tsx.App("App.tsx", code), nsgotest.Size(300, 120))
		app.Update = func(bool) core.UpdateCheck {
			return core.UpdateCheck{
				App:     "Hello",
				Current: "1.0",
				Version: "1.1",
				URL:     "https://example.com/hello/1.1",
				Notes:   "• Faster startup",
			}
		}
		app.Press("id:check")
		alerts := app.Alerts()
		if len(alerts) != 1 || alerts[0].Title != "A new version of Hello is available" ||
			alerts[0].Message != "Hello 1.1 is available. You have 1.0.\n\n• Faster startup" {
			t.Fatalf("alerts = %+v", alerts)
		}
		app.Answer(0)
		if !slices.Equal(app.OpenedURLs, []string{"https://example.com/hello/1.1"}) {
			t.Fatalf("opened URLs = %v", app.OpenedURLs)
		}
		app.Press("id:check")
		app.Answer(1)
		if len(app.OpenedURLs) != 1 {
			t.Fatalf("Later opened %v", app.OpenedURLs[1:])
		}
		if !slices.Equal(app.UpdateChecks, []bool{false, false}) {
			t.Fatalf("checks = %v", app.UpdateChecks)
		}
	})

	t.Run("prepared_restart", func(t *testing.T) {
		app := nsgotest.Start(t, tsx.App("App.tsx", code), nsgotest.Size(300, 120))
		app.Update = func(bool) core.UpdateCheck {
			return core.UpdateCheck{App: "Hello", Current: "1.0", Restart: true}
		}
		app.Press("id:check")
		alerts := app.Alerts()
		if len(alerts) != 1 || alerts[0].Message != "A new build of Hello is ready. Restart to use it." {
			t.Fatalf("alerts = %+v", alerts)
		}
		app.Answer(0)
		if app.Restarts != 1 || len(app.OpenedURLs) != 0 {
			t.Fatalf("restarts = %d, urls = %v", app.Restarts, app.OpenedURLs)
		}
	})

	t.Run("up_to_date_and_error_and_managed", func(t *testing.T) {
		for _, tc := range []struct {
			name        string
			found       core.UpdateCheck
			wantTitle   string
			wantContain string
		}{
			{"up_to_date", core.UpdateCheck{App: "Hello", Current: "1.0"}, "You’re up to date", "Hello 1.0 is the newest version available."},
			{"error", core.UpdateCheck{Err: errors.New("offline")}, "Can’t check for updates", "Try again later"},
			{"development", core.UpdateCheck{Managed: updater.ManagedDevelopment}, "Can’t check for updates", "development build"},
			{"channel", core.UpdateCheck{Managed: updater.ManagedChannel}, "Can’t check for updates", "distribution channel"},
			{"store", core.UpdateCheck{Managed: updater.ManagedStore, Store: "Flatpak"}, "Can’t check for updates", "Flatpak"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				app := nsgotest.Start(t, tsx.App("App.tsx", code), nsgotest.Size(300, 120))
				app.Update = func(bool) core.UpdateCheck { return tc.found }
				app.Press("id:check")
				alerts := app.Alerts()
				if len(alerts) != 1 || alerts[0].Title != tc.wantTitle || !strings.Contains(alerts[0].Message, tc.wantContain) {
					t.Fatalf("alerts = %+v", alerts)
				}
			})
		}
	})

	t.Run("native_platform_window_shows_no_alert", func(t *testing.T) {
		app := nsgotest.Start(t, tsx.App("App.tsx", code), nsgotest.Size(300, 120))
		app.Update = func(bool) core.UpdateCheck { return core.UpdateCheck{Shown: true} }
		app.Press("id:check")
		if alerts := app.Alerts(); len(alerts) != 0 {
			t.Fatalf("alerts = %+v over native updater window", alerts)
		}
		if !slices.Equal(app.UpdateChecks, []bool{false}) {
			t.Fatalf("checks = %v", app.UpdateChecks)
		}
	})

	t.Run("background_and_daily_schedule", func(t *testing.T) {
		app := nsgotest.Start(t, tsx.App("App.tsx", code), nsgotest.Size(300, 120))
		app.Update = func(bool) core.UpdateCheck { return core.UpdateCheck{App: "Hello", Current: "1.0"} }
		app.Press("id:bg")
		if len(app.Alerts()) != 0 || !slices.Equal(app.UpdateChecks, []bool{true}) {
			t.Fatalf("bg up-to-date: alerts=%+v checks=%v", app.Alerts(), app.UpdateChecks)
		}

		app.Update = func(bool) core.UpdateCheck { return core.UpdateCheck{Err: errors.New("offline")} }
		app.Press("id:daily")
		app.Press("id:daily")
		if len(app.Alerts()) != 0 || !slices.Equal(app.UpdateChecks, []bool{true, true}) {
			t.Fatalf("daily retry throttle: alerts=%+v checks=%v", app.Alerts(), app.UpdateChecks)
		}

		app.Clock.SetWallTime(app.Clock.WallTime().Add(time.Hour))
		app.Update = func(bool) core.UpdateCheck {
			return core.UpdateCheck{App: "Hello", Current: "1.0", Version: "1.2", URL: "https://example.com/1.2"}
		}
		app.Press("id:daily")
		if len(app.Alerts()) != 1 || len(app.UpdateChecks) != 3 {
			t.Fatalf("daily after retry: alerts=%+v checks=%v", app.Alerts(), app.UpdateChecks)
		}
		app.Answer(1)

		// Within 24 hours, checkDaily skips checking.
		app.Clock.SetWallTime(app.Clock.WallTime().Add(12 * time.Hour))
		app.Press("id:daily")
		if len(app.UpdateChecks) != 3 {
			t.Fatalf("daily within 24h checked anyway: %v", app.UpdateChecks)
		}
		// After 24 hours, checkDaily checks again.
		app.Clock.SetWallTime(app.Clock.WallTime().Add(13 * time.Hour))
		app.Press("id:daily")
		if len(app.UpdateChecks) != 4 || len(app.Alerts()) != 1 {
			t.Fatalf("daily after 24h: alerts=%+v checks=%v", app.Alerts(), app.UpdateChecks)
		}
	})
}

func TestUpdaterCapabilityStatusAndPreferences(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("synthetic updater state runs headless only")
	}
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(`
import { mount, View, Text } from "nsgo";
import { capability, status, setAutomaticChecks, setAutomaticDownloads, checkDaily } from "nsgo/updater";

function App() {
  return <View>
    <Text testID="cap">{() => {
      const c = capability();
      return c.managed + "|" + String(c.store);
    }}</Text>
    <Text testID="state">{() => {
      const s = status();
      return [
        s.capability.managed,
        String(s.capability.store),
        s.automaticChecks,
        s.automaticDownloads,
        String(s.available),
        String(s.notes),
        String(s.lastCheck),
        s.phase,
        s.loaded,
        s.total,
        s.fraction,
      ].join("|");
    }}</Text>
    <View testID="checks-off" style={{ width: 80, height: 30 }} onPress={() => setAutomaticChecks(false)}><Text>Off</Text></View>
    <View testID="checks-on" style={{ width: 80, height: 30 }} onPress={() => setAutomaticChecks(true)}><Text>On</Text></View>
    <View testID="dl-on" style={{ width: 80, height: 30 }} onPress={() => setAutomaticDownloads(true)}><Text>DL</Text></View>
    <View testID="daily" style={{ width: 80, height: 30 }} onPress={() => checkDaily()}><Text>Daily</Text></View>
  </View>;
}
mount(App);
`)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("all_managed_capabilities", func(t *testing.T) {
		for _, tc := range []struct {
			managed core.UpdaterManaged
			store   string
			want    string
		}{
			{core.UpdaterUnavailable, "", "unavailable|null"},
			{core.UpdaterSelf, "", "self|null"},
			{core.UpdaterPage, "", "page|null"},
			{core.UpdaterDevelopment, "", "development|null"},
			{core.UpdaterChannel, "", "channel|null"},
			{core.UpdaterStore, "Flatpak", "store|Flatpak"},
		} {
			app := nsgotest.Start(t, tsx.App("App.tsx", code),
				nsgotest.UpdaterState(core.UpdaterState{Managed: tc.managed, Store: tc.store}))
			if got := app.Find("id:cap").Text; got != tc.want {
				t.Errorf("managed=%v store=%q: got %q, want %q", tc.managed, tc.store, got, tc.want)
			}
		}
	})

	t.Run("reactive_preferences_and_metadata", func(t *testing.T) {
		app := nsgotest.Start(t, tsx.App("App.tsx", code),
			nsgotest.UpdaterState(core.UpdaterState{Managed: core.UpdaterSelf, AutomaticChecks: true}))
		if got := app.Find("id:state").Text; got != "self|null|true|false|null|null|null|idle|0|0|0" {
			t.Fatalf("initial state = %q", got)
		}
		app.Press("id:checks-off")
		if got := app.Find("id:state").Text; got != "self|null|false|false|null|null|null|idle|0|0|0" {
			t.Fatalf("after off = %q", got)
		}
		app.Press("id:daily")
		if len(app.UpdateChecks) != 0 {
			t.Fatalf("disabled daily check ran: %v", app.UpdateChecks)
		}
		app.Press("id:checks-on")
		app.Update = func(bool) core.UpdateCheck {
			return core.UpdateCheck{Managed: core.UpdaterSelf, Version: "2.0", Notes: "shiny"}
		}
		app.Press("id:daily")
		wantTS := app.Clock.WallTime().Truncate(time.Second).UnixMilli()
		want := "self|null|true|false|2.0|shiny|" + formatInt64(wantTS) + "|idle|0|0|0"
		if got := app.Find("id:state").Text; got != want {
			t.Fatalf("after daily = %q, want %q", got, want)
		}
	})

	t.Run("native_preferences_macos_and_non_macos", func(t *testing.T) {
		app := nsgotest.Start(t, tsx.App("App.tsx", code),
			nsgotest.UpdaterState(core.UpdaterState{
				Managed:           core.UpdaterSelf,
				NativePreferences: true,
				AutomaticChecks:   true,
				Phase:             core.UpdateDownloading,
				Loaded:            50,
				Total:             200,
			}),
			nsgotest.Environment(core.Environment{OS: core.OSMacOS}))
		if got := app.Find("id:state").Text; got != "self|null|true|false|null|null|null|downloading|50|200|0.25" {
			t.Fatalf("macos initial = %q", got)
		}
		app.Press("id:checks-off")
		app.Press("id:dl-on")
		if got := app.Find("id:state").Text; got != "self|null|false|true|null|null|null|downloading|50|200|0.25" {
			t.Fatalf("macos updated = %q", got)
		}

		// On non-macOS, setAutomaticDownloads is a no-op; also verify installing phase.
		win := nsgotest.Start(t, tsx.App("App.tsx", code),
			nsgotest.UpdaterState(core.UpdaterState{
				Managed:           core.UpdaterSelf,
				NativePreferences: true,
				AutomaticChecks:   true,
				Phase:             core.UpdateInstalling,
				Loaded:            200,
				Total:             200,
			}),
			nsgotest.Environment(core.Environment{OS: core.OSWindows}))
		win.Press("id:dl-on")
		if got := win.Find("id:state").Text; got != "self|null|true|false|null|null|null|installing|200|200|1" {
			t.Fatalf("windows dl-on = %q", got)
		}
	})
}

func TestUpdaterSandboxGating(t *testing.T) {
	const src = `
import { mount, View, Text } from "nsgo";
import { capability, checkForUpdate } from "nsgo/updater";

function App() {
  const c = capability();
  return <View testID="btn" style={{ width: 80, height: 30 }} onPress={() => checkForUpdate()}>
    <Text testID="out">{"managed:" + c.managed}</Text>
  </View>;
}
mount(App);
`
	t.Run("refused_by_default_sandbox", func(t *testing.T) {
		p := verified(t, src, pkg.Capabilities{})
		var caught string
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return core.Catch{
				Try: func(cx *core.Scope) core.Node {
					return tsx.View{Package: p, Sandbox: tsx.Sandbox{WebUnisolated: true}}.Render(cx)
				},
				Fallback: func(_ *core.Scope, err error, _ func()) core.Node {
					caught = strings.TrimSpace(err.Error())
					return core.Text{Content: "caught"}
				},
			}
		})
		app.Find("text:caught")
		if !strings.Contains(caught, `module "nsgo/updater" is not given to this sandbox`) {
			t.Fatalf("unexpected sandbox error: %q", caught)
		}
	})

	t.Run("allowed_when_host_grants_module", func(t *testing.T) {
		if nsgotest.Target() != "" {
			t.Skip("synthetic update checks run headless only")
		}
		p := verified(t, src, pkg.Capabilities{})
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return tsx.View{Package: p, Sandbox: tsx.Sandbox{
				Modules:       []string{"nsgo/updater"},
				WebUnisolated: true,
			}}
		}, nsgotest.UpdaterState(core.UpdaterState{Managed: core.UpdaterSelf}))
		app.Update = func(bool) core.UpdateCheck { return core.UpdateCheck{Shown: true} }
		app.Find("text:managed:self")
		app.Press("id:btn")
		if !slices.Equal(app.UpdateChecks, []bool{false}) {
			t.Fatalf("sandbox checkForUpdate checks = %v", app.UpdateChecks)
		}
	})
}

func formatInt64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
