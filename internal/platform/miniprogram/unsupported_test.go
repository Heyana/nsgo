package miniprogram

import (
	"errors"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// mpHarness drives a real core.Tree against the mini-program mounter and
// delivers every event the mounter dispatches back into the tree.
type mpHarness struct {
	root *core.Scope
	tree *core.Tree
	m    *mounter
	h    *fakeHost
}

func newMPHarness(t *testing.T, build func(*core.Scope) core.Node) *mpHarness {
	t.Helper()
	h := &fakeHost{}
	m := newMounter(h)
	env := buildEnvironment(driver.Environment{}, deviceInfo{
		OS: "ios", OSVersion: "17.4", Language: "en_US", PixelRatio: 2,
	})
	root := core.NewRoot(core.RootOptions{})
	t.Cleanup(root.Dispose)
	tree := core.NewTree(root, NewTextEngine(&approx{}))
	tree.SetEnvironment(env)
	tree.Resize(driver.Size{Width: 375, Height: 667}, driver.Geometry{})
	tree.Mount(build)
	p := &mpHarness{root: root, tree: tree, m: m, h: h}
	p.flush()
	return p
}

func (p *mpHarness) flush() {
	for range 8 {
		batch := p.tree.Commit()
		if batch != nil && len(batch.Items) > 0 {
			p.m.mount(batch)
		}
		if len(p.h.events) == 0 {
			return
		}
		events := p.h.events
		p.h.events = nil
		for _, ev := range events {
			p.tree.Dispatch(ev)
		}
	}
}

func (p *mpHarness) assertNoUnexpectedServices(t *testing.T) {
	t.Helper()
	var unexpected []driver.ItemKind
	for _, kind := range p.h.services {
		if kind != driver.ItemDirection {
			unexpected = append(unexpected, kind)
		}
	}
	if len(unexpected) != 0 {
		t.Fatalf("leaked unsupported items to host service: %v", unexpected)
	}
}

func TestMiniProgramUnsupportedWebView(t *testing.T) {
	var (
		hasWebView bool
		ref        core.Ref
		src        *core.Signal[core.WebSource]
		webErrs    []error
		evalRes    string
		evalErr    error
		evalCalled bool
		scope      *core.Scope
	)
	p := newMPHarness(t, func(cx *core.Scope) core.Node {
		scope = cx
		hasWebView = cx.Env().HasWebView()
		src = core.NewSignal(cx, core.WebSrc("https://example.com/first"))
		return core.WebView{
			Ref:        &ref,
			SourceFunc: src.Get,
			OnError:    func(err error) { webErrs = append(webErrs, err) },
		}
	})
	if hasWebView {
		t.Fatal("Env.HasWebView() = true, want false on mini programs")
	}

	// Initial mount reports a *core.WebError wrapping errors.ErrUnsupported.
	if len(webErrs) != 1 {
		t.Fatalf("initial WebView errors = %v, want 1", webErrs)
	}
	checkWebErr := func(err error, wantURL string) {
		t.Helper()
		var we *core.WebError
		if !errors.As(err, &we) {
			t.Fatalf("error = %T (%v), want *core.WebError", err, err)
		}
		if !errors.Is(err, errors.ErrUnsupported) {
			t.Fatalf("WebError %v does not match errors.ErrUnsupported", err)
		}
		if we.URL != wantURL || we.Domain != "" || we.Code != 0 || !strings.Contains(we.Message, "<web-view>") {
			t.Fatalf("WebError = %#v, want URL %q, empty Domain/Code, and <web-view> explanation", we, wantURL)
		}
	}
	checkWebErr(webErrs[0], "https://example.com/first")

	// Updating SourceFunc reports again for the new URL.
	p.root.Run(func() { src.Set(core.WebSrc("https://example.com/second")) })
	p.flush()
	if len(webErrs) != 2 {
		t.Fatalf("WebView errors after source change = %v, want 2", webErrs)
	}
	checkWebErr(webErrs[1], "https://example.com/second")

	// Ref.Load also reports *core.WebError wrapping errors.ErrUnsupported.
	p.root.Run(func() { ref.Load(core.WebSrc("https://example.com/loaded")) })
	p.flush()
	if len(webErrs) != 3 {
		t.Fatalf("WebView errors after Ref.Load = %v, want 3", webErrs)
	}
	checkWebErr(webErrs[2], "https://example.com/loaded")

	// Ref.Evaluate completes immediately with errors.ErrUnsupported instead of hanging.
	p.root.Run(func() {
		ref.Evaluate("1 + 1", func(json string, err error) {
			evalCalled = true
			evalRes, evalErr = json, err
		})
	})
	p.flush()
	if !evalCalled || evalRes != "" || !errors.Is(evalErr, errors.ErrUnsupported) {
		t.Fatalf("Evaluate called=%v res=%q err=%v, want called with errors.ErrUnsupported", evalCalled, evalRes, evalErr)
	}

	// Navigation, messaging and input commands are clean no-ops and do not leak to service.
	p.root.Run(func() {
		ref.Reload()
		ref.GoBack()
		ref.GoForward()
		ref.StopLoading()
		ref.PostMessage("hello")
		ref.InjectJavaScript("console.log('noop')")
		ref.SendInput(core.WebClick(10, 10))
	})
	p.flush()
	p.assertNoUnexpectedServices(t)

	// Cookie and website-data calls report CodeUnavailable and errors.ErrUnsupported.
	var setErr, listErr, clearErr error
	var setDone, listDone, clearDone bool
	p.root.Run(func() {
		scope.SetWebCookies([]core.WebCookie{{Name: "s", Value: "1", Domain: "example.com", Secure: true}}, func(err error) {
			setDone, setErr = true, err
		})
		scope.WebCookies(core.WebCookieFilter{URL: "https://example.com/"}, func(_ []core.WebCookie, err error) {
			listDone, listErr = true, err
		})
		scope.ClearWebsiteData(func(err error) {
			clearDone, clearErr = true, err
		})
	})
	p.flush()
	for _, tc := range []struct {
		name string
		done bool
		err  error
	}{
		{"SetWebCookies", setDone, setErr},
		{"WebCookies", listDone, listErr},
		{"ClearWebsiteData", clearDone, clearErr},
	} {
		if !tc.done || core.ErrorCode(tc.err) != core.CodeUnavailable || !errors.Is(tc.err, errors.ErrUnsupported) {
			t.Errorf("%s: done=%v err=%v (code=%q), want done with CodeUnavailable and errors.ErrUnsupported",
				tc.name, tc.done, tc.err, core.ErrorCode(tc.err))
		}
	}
}

func TestMiniProgramUnsupportedNativeComponent(t *testing.T) {
	var (
		ref    core.Ref
		gotErr error
	)
	p := newMPHarness(t, func(*core.Scope) core.Node {
		return core.Native{
			Component: "custom/map",
			Props:     map[string]any{"zoom": 10},
			Ref:       &ref,
			OnError:   func(err error) { gotErr = err },
		}
	})
	if core.ErrorCode(gotErr) != core.CodeUnavailable || !errors.Is(gotErr, errors.ErrUnsupported) {
		t.Fatalf("Native OnError = %v (code=%q), want CodeUnavailable matching errors.ErrUnsupported",
			gotErr, core.ErrorCode(gotErr))
	}
	p.root.Run(func() { ref.Command("recenter", nil) })
	p.flush()
	p.assertNoUnexpectedServices(t)
}

func TestMiniProgramUnsupportedShadersAndGPU(t *testing.T) {
	var (
		supportsGPU bool
		gotErr      error
	)
	p := newMPHarness(t, func(cx *core.Scope) core.Node {
		supportsGPU = cx.Env().SupportsGPU()
		return core.Shader{
			TestID:  "sh",
			OnError: func(err error) { gotErr = err },
		}
	})
	if supportsGPU {
		t.Fatal("Env.SupportsGPU() = true, want false on mini programs")
	}

	// Even if a shader or GPU mount item reaches the mini-program mounter
	// directly (outside nsgo_noshader), ItemShader reports errors.ErrUnsupported
	// and transition/input/GPU items are clean no-ops that never leak to service.
	var shaderID driver.NodeID
	for _, v := range p.m.views {
		if v != nil && v.kind == driver.KindShader {
			shaderID = v.id
			break
		}
	}
	if shaderID == 0 {
		t.Fatal("KindShader view was not mounted")
	}
	p.m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemShader, ID: shaderID, Shader: &driver.Shader{Role: driver.ShaderFill}},
		{Kind: driver.ItemShaderTransition, ID: shaderID},
		{Kind: driver.ItemShaderInput, ID: shaderID},
		{Kind: driver.ItemCreate, ID: 99, View: driver.KindGPU},
		{Kind: driver.ItemGPU, ID: 99, GPU: &driver.GPUViewConfig{}},
	}})
	p.flush()
	if core.ErrorCode(gotErr) != core.CodeUnavailable || !errors.Is(gotErr, errors.ErrUnsupported) {
		t.Fatalf("Shader OnError = %v (code=%q), want CodeUnavailable matching errors.ErrUnsupported",
			gotErr, core.ErrorCode(gotErr))
	}
	p.assertNoUnexpectedServices(t)
}
