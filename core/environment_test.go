package core

import (
	"fmt"
	"testing"
)

// A report that changes only fields read through Env signals commits without a full round, and a tree that reads none of them sends nothing.
func TestWindowEnvironmentReportsStayIncremental(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 100}, func(*Scope) Node {
		return View{TestID: "static"}
	})
	h.commit()
	h.commit()
	for _, env := range []Environment{
		{WindowFrame: WindowFrame{X: 20, Y: 30}},
		{Maximized: true},
		{Lifecycle: LifecycleBackground},
		{TitleBar: Edges{Top: 24}},
		{ThermalState: ThermalFair},
		{LowPowerMode: LowPowerOn},
	} {
		h.tree.SetEnvironment(env)
		if h.tree.full {
			t.Fatalf("environment %+v forced a full resolve", env)
		}
		if batch := h.commit(); batch != nil && len(batch.Items) > 0 {
			t.Fatalf("environment %+v emitted items for a tree that reads none of its signals: %+v", env, batch.Items)
		}
	}
}

// What reads the window's frame, maximized state, lifecycle or title bar through Env follows each report without a full round.
func TestWindowEnvironmentReportsReachTheirReaders(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		e := cx.Env()
		return View{Children: []Node{
			Text{TestID: "frame", ContentFunc: func() string { return fmt.Sprint(e.WindowFrame().X) }},
			Text{TestID: "maximized", ContentFunc: func() string { return fmt.Sprint(e.Maximized()) }},
			Text{TestID: "lifecycle", ContentFunc: func() string { return e.Lifecycle().String() }},
			Text{TestID: "titlebar", ContentFunc: func() string { return fmt.Sprint(e.TitleBar().Top) }},
		}}
	})
	h.commit()
	env := Environment{WindowFrame: WindowFrame{X: 20}, Maximized: true, Lifecycle: LifecycleBackground, TitleBar: Edges{Top: 24}}
	h.tree.SetEnvironment(env)
	if h.tree.full {
		t.Fatal("the window's report forced a full resolve")
	}
	h.commit()
	for id, want := range map[string]string{"frame": "20", "maximized": "true", "lifecycle": LifecycleBackground.String(), "titlebar": "24"} {
		if got := h.find(id).text.content; got != want {
			t.Errorf("%s shows %q, want %q", id, got, want)
		}
	}
}

func TestEnvironmentReportsThatAffectResolutionStayFull(t *testing.T) {
	if !environmentNeedsFullRound(Environment{}, Environment{Dark: true}) {
		t.Fatal("dark mode did not request a full resolve")
	}
	if environmentNeedsFullRound(Environment{}, Environment{WindowFrame: WindowFrame{X: 1}}) {
		t.Fatal("window frame requested a full resolve")
	}
}

// TestSecondaryTargetEnvCapabilities verifies how core.Env exposes the capability contracts reported by HarmonyOS, mini programs, watchOS and tvOS so higher-level game layers can adapt consistently (#1117).
func TestSecondaryTargetEnvCapabilities(t *testing.T) {
	h := newHost(t, Size{Width: 400, Height: 300}, func(*Scope) Node { return View{} })
	h.commit()
	env := h.root.Env()

	for _, tc := range []struct {
		name           string
		in             Environment
		wantAudio      bool
		wantSpeak      bool
		wantSpeech     bool
		wantWebView    bool
		wantCamera     bool
		wantCursor     bool
		wantAppleKeys  bool
		wantMobileBars bool
	}{
		{
			name:           "HarmonyOS phone",
			in:             Environment{OS: OSAndroid, Idiom: IdiomPhone, HasWebView: true, Camera: true},
			wantWebView:    true,
			wantCamera:     true,
			wantMobileBars: true,
		},
		{
			name:          "mini program on iOS",
			in:            Environment{OS: OSIOS, Idiom: IdiomPhone, Simulated: true, AppleKeyboard: true, Camera: true},
			wantCamera:    true,
			wantAppleKeys: true,
		},
		{
			name:          "mini program on macOS",
			in:            Environment{OS: OSMacOS, Idiom: IdiomPhone, Simulated: true, AppleKeyboard: true},
			wantAppleKeys: true,
		},
		{
			name:          "watchOS",
			in:            Environment{OS: OSIOS, Idiom: IdiomWatch, Speak: true},
			wantSpeak:     true,
			wantAppleKeys: true,
		},
		{
			name:           "tvOS",
			in:             Environment{OS: OSIOS, Idiom: IdiomTV, Speak: true, SignInWithApple: true, Purchases: true},
			wantSpeak:      true,
			wantAppleKeys:  true,
			wantMobileBars: true,
		},
	} {
		h.tree.SetEnvironment(tc.in)
		if env.CanPlayAudio() != tc.wantAudio {
			t.Errorf("%s: CanPlayAudio() = %v, want %v", tc.name, env.CanPlayAudio(), tc.wantAudio)
		}
		if env.CanSpeak() != tc.wantSpeak {
			t.Errorf("%s: CanSpeak() = %v, want %v", tc.name, env.CanSpeak(), tc.wantSpeak)
		}
		if env.CanRecognizeSpeech() != tc.wantSpeech {
			t.Errorf("%s: CanRecognizeSpeech() = %v, want %v", tc.name, env.CanRecognizeSpeech(), tc.wantSpeech)
		}
		if env.HasWebView() != tc.wantWebView {
			t.Errorf("%s: HasWebView() = %v, want %v", tc.name, env.HasWebView(), tc.wantWebView)
		}
		if env.CanTakePhoto() != tc.wantCamera {
			t.Errorf("%s: CanTakePhoto() = %v, want %v", tc.name, env.CanTakePhoto(), tc.wantCamera)
		}
		if env.SupportsCustomCursor() != tc.wantCursor {
			t.Errorf("%s: SupportsCustomCursor() = %v, want %v", tc.name, env.SupportsCustomCursor(), tc.wantCursor)
		}
		if env.AppleKeys() != tc.wantAppleKeys {
			t.Errorf("%s: AppleKeys() = %v, want %v", tc.name, env.AppleKeys(), tc.wantAppleKeys)
		}
		if env.mobileBars() != tc.wantMobileBars {
			t.Errorf("%s: mobileBars() = %v, want %v", tc.name, env.mobileBars(), tc.wantMobileBars)
		}
	}
}

func TestEnvCanUseCloudStorage(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		return Text{
			TestID: "cloud",
			ContentFunc: func() string {
				if cx.Env().CanUseCloudStorage() {
					return "yes"
				}
				return "no"
			},
		}
	})
	h.commit()
	if got := h.find("cloud").text.content; got != "no" {
		t.Fatalf("initial CanUseCloudStorage = %q, want no", got)
	}
	h.tree.SetEnvironment(Environment{CloudStorage: true})
	h.commit()
	if got := h.find("cloud").text.content; got != "yes" {
		t.Fatalf("updated CanUseCloudStorage = %q, want yes", got)
	}
}
