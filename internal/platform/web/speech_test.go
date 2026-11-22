//go:build js

package web

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestSpeech(t *testing.T) {
	g := js.Global()
	obj := g.Get("Object")
	origStd := obj.Call("getOwnPropertyDescriptor", g, "SpeechRecognition")
	origWebkit := obj.Call("getOwnPropertyDescriptor", g, "webkitSpeechRecognition")
	t.Cleanup(func() {
		if recognizing != nil {
			recognizing.abort()
		}
		restore := func(prop string, desc js.Value) {
			if desc.IsUndefined() {
				g.Delete(prop)
			} else {
				obj.Call("defineProperty", g, prop, desc)
			}
		}
		restore("SpeechRecognition", origStd)
		restore("webkitSpeechRecognition", origWebkit)
	})

	setGlobal := func(prop string, val any) {
		obj.Call("defineProperty", g, prop, map[string]any{
			"value":        val,
			"configurable": true,
			"writable":     true,
		})
	}

	events := make(chan driver.Event, 32)
	dispatch := func(ev driver.Event) { events <- ev }
	next := func() (uint32, driver.SpeechEvent) {
		t.Helper()
		select {
		case ev := <-events:
			if ev.Kind != driver.EventSpeech {
				t.Fatalf("unexpected event kind %v", ev.Kind)
			}
			return ev.Count, *ev.Data.(*driver.SpeechEvent)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for speech event")
			return 0, driver.SpeechEvent{}
		}
	}
	assertNoEvent := func() {
		t.Helper()
		select {
		case ev := <-events:
			t.Fatalf("unexpected speech event: %+v", ev)
		default:
		}
	}

	// 1. Absent in browser: speechAvailable is false and start reports unavailable.
	setGlobal("SpeechRecognition", js.Undefined())
	setGlobal("webkitSpeechRecognition", js.Undefined())
	if speechAvailable() {
		t.Fatal("speechAvailable() = true without constructor")
	}
	speech(1, driver.SpeechRequest{Command: driver.SpeechStart, Lang: "en-US"}, dispatch)
	if id, ev := next(); id != 1 || ev.Phase != driver.SpeechEnded || ev.Error != driver.SpeechUnavailable {
		t.Fatalf("absent start = %d %+v, want unavailable", id, ev)
	}

	// Install a fake constructor script that records created instances on globalThis.__nsgoSpeechFake.
	makeCtor := g.Get("Function").New(`
		const instances = [];
		function FakeSpeechRecognition() {
			this.lang = "";
			this.continuous = true;
			this.interimResults = false;
			this.maxAlternatives = 0;
			this.started = 0;
			this.stopped = 0;
			this.aborted = 0;
			this.onstart = null;
			this.onresult = null;
			this.onerror = null;
			this.onend = null;
			this.start = () => {
				this.started++;
				if (FakeSpeechRecognition.throwOnStart) {
					throw FakeSpeechRecognition.throwOnStart;
				}
			};
			this.stop = () => { this.stopped++; };
			this.abort = () => { this.aborted++; };
			instances.push(this);
		}
		FakeSpeechRecognition.instances = instances;
		FakeSpeechRecognition.throwOnStart = null;
		return FakeSpeechRecognition;
	`)

	// 2. webkitSpeechRecognition fallback is recognized.
	webkitCtor := makeCtor.Invoke()
	setGlobal("SpeechRecognition", js.Undefined())
	setGlobal("webkitSpeechRecognition", webkitCtor)
	if !speechAvailable() {
		t.Fatal("speechAvailable() = false with webkitSpeechRecognition")
	}

	// 3. Standard SpeechRecognition lifecycle: start -> interim -> stop -> final -> end.
	stdCtor := makeCtor.Invoke()
	setGlobal("SpeechRecognition", stdCtor)
	setGlobal("webkitSpeechRecognition", js.Undefined())
	if !speechAvailable() {
		t.Fatal("speechAvailable() = false with SpeechRecognition")
	}

	speech(2, driver.SpeechRequest{Command: driver.SpeechStart, Lang: "en-US", Interim: true}, dispatch)
	instances := stdCtor.Get("instances")
	if instances.Length() != 1 {
		t.Fatalf("instances = %d, want 1", instances.Length())
	}
	inst := instances.Index(0)
	if inst.Get("lang").String() != "en-US" || !inst.Get("interimResults").Bool() || inst.Get("continuous").Bool() || inst.Get("maxAlternatives").Int() != 1 || inst.Get("started").Int() != 1 {
		t.Fatalf("configured rec = lang:%q interim:%v continuous:%v maxAlt:%d started:%d",
			inst.Get("lang").String(), inst.Get("interimResults").Bool(), inst.Get("continuous").Bool(), inst.Get("maxAlternatives").Int(), inst.Get("started").Int())
	}

	inst.Call("onstart")
	if id, ev := next(); id != 2 || ev.Phase != driver.SpeechStarted {
		t.Fatalf("onstart = %d %+v", id, ev)
	}

	makeResultEvent := g.Get("Function").New("parts", `
		const results = parts.map(([text, isFinal]) => {
			const res = [{ transcript: text }];
			res.isFinal = isFinal;
			return res;
		});
		return { results };
	`)

	// Interim result.
	inst.Call("onresult", makeResultEvent.Invoke([]any{[]any{"hello", false}}))
	if id, ev := next(); id != 2 || ev.Phase != driver.SpeechResult || ev.Text != "hello" || ev.Final {
		t.Fatalf("interim = %d %+v", id, ev)
	}

	// Stop calls stop(), then final result + onend.
	speech(2, driver.SpeechRequest{Command: driver.SpeechStop}, dispatch)
	if inst.Get("stopped").Int() != 1 {
		t.Fatalf("stopped = %d, want 1", inst.Get("stopped").Int())
	}
	inst.Call("onresult", makeResultEvent.Invoke([]any{[]any{"hello ", true}, []any{"world", true}}))
	if id, ev := next(); id != 2 || ev.Phase != driver.SpeechResult || ev.Text != "hello world" || !ev.Final {
		t.Fatalf("final = %d %+v", id, ev)
	}
	inst.Call("onend")
	if id, ev := next(); id != 2 || ev.Phase != driver.SpeechEnded || ev.Text != "hello world" || ev.Error != "" {
		t.Fatalf("end = %d %+v", id, ev)
	}
	assertNoEvent()

	// 4. Interim: false suppresses non-final results.
	speech(3, driver.SpeechRequest{Command: driver.SpeechStart, Lang: "ja-JP", Interim: false}, dispatch)
	inst3 := instances.Index(1)
	inst3.Call("onstart")
	_, _ = next()
	inst3.Call("onresult", makeResultEvent.Invoke([]any{[]any{"konnichi", false}}))
	assertNoEvent()
	inst3.Call("onresult", makeResultEvent.Invoke([]any{[]any{"konnichiwa", true}}))
	if id, ev := next(); id != 3 || ev.Phase != driver.SpeechResult || ev.Text != "konnichiwa" || !ev.Final {
		t.Fatalf("final only = %d %+v", id, ev)
	}
	inst3.Call("onend")
	if id, ev := next(); id != 3 || ev.Phase != driver.SpeechEnded || ev.Text != "konnichiwa" || ev.Error != "" {
		t.Fatalf("end3 = %d %+v", id, ev)
	}

	// 5. onerror ends once and ignores trailing onend.
	speech(4, driver.SpeechRequest{Command: driver.SpeechStart, Interim: true}, dispatch)
	inst4 := instances.Index(2)
	inst4.Call("onerror", map[string]any{"error": "not-allowed"})
	if id, ev := next(); id != 4 || ev.Phase != driver.SpeechEnded || ev.Error != driver.SpeechNotAllowed {
		t.Fatalf("onerror = %d %+v", id, ev)
	}
	// Handlers were cleared on finish; simulate a trailing onend if called.
	if !inst4.Get("onend").IsNull() {
		t.Fatal("onend was not cleared on finish")
	}
	assertNoEvent()

	// 6. Abort calls abort() and emits nothing further.
	speech(5, driver.SpeechRequest{Command: driver.SpeechStart, Interim: true}, dispatch)
	inst5 := instances.Index(3)
	speech(5, driver.SpeechRequest{Command: driver.SpeechAbort}, dispatch)
	if inst5.Get("aborted").Int() != 1 {
		t.Fatalf("aborted = %d, want 1", inst5.Get("aborted").Int())
	}
	assertNoEvent()

	// 7. Starting a new session aborts the previous one.
	speech(6, driver.SpeechRequest{Command: driver.SpeechStart}, dispatch)
	inst6 := instances.Index(4)
	speech(7, driver.SpeechRequest{Command: driver.SpeechStart}, dispatch)
	if inst6.Get("aborted").Int() != 1 {
		t.Fatalf("previous session aborted = %d, want 1", inst6.Get("aborted").Int())
	}
	speech(7, driver.SpeechRequest{Command: driver.SpeechAbort}, dispatch)
	assertNoEvent()

	// 8. Exception thrown by start() maps NotAllowedError cleanly.
	stdCtor.Set("throwOnStart", g.Get("DOMException").New("blocked", "NotAllowedError"))
	speech(8, driver.SpeechRequest{Command: driver.SpeechStart}, dispatch)
	if id, ev := next(); id != 8 || ev.Phase != driver.SpeechEnded || ev.Error != driver.SpeechNotAllowed {
		t.Fatalf("throwOnStart = %d %+v, want not-allowed", id, ev)
	}
	stdCtor.Set("throwOnStart", js.Null())

	// 9. Error code mapping table.
	for code, want := range map[string]string{
		"aborted":                "",
		"not-allowed":            driver.SpeechNotAllowed,
		"service-not-allowed":    driver.SpeechNotAllowed,
		"no-speech":              driver.SpeechNoSpeech,
		"audio-capture":          driver.SpeechAudioCapture,
		"network":                driver.SpeechNetwork,
		"language-not-supported": driver.SpeechLanguageUnsupported,
		"bad-grammar":            driver.SpeechFailed,
		"":                       driver.SpeechFailed,
	} {
		if got := webSpeechError(code); got != want {
			t.Errorf("webSpeechError(%q) = %q, want %q", code, got, want)
		}
	}
}
