//go:build js && wasm

package web

import (
	"strings"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Speech recognition through the browser's SpeechRecognition (or webkitSpeechRecognition), one session at a time for the page.

type webSpeechRun struct {
	id         uint32
	interim    bool
	rec        js.Value
	dispatch   func(driver.Event)
	onStart    js.Func
	onResult   js.Func
	onError    js.Func
	onEnd      js.Func
	text       string
	started    bool
	done       bool
	funcsBound bool
}

var recognizing *webSpeechRun

// speechConstructor returns window.SpeechRecognition or window.webkitSpeechRecognition when present.
func speechConstructor() js.Value {
	g := js.Global()
	if ctor := g.Get("SpeechRecognition"); ctor.Type() == js.TypeFunction {
		return ctor
	}
	if ctor := g.Get("webkitSpeechRecognition"); ctor.Type() == js.TypeFunction {
		return ctor
	}
	return js.Undefined()
}

// speechAvailable is Environment.Speech: the browser exposes SpeechRecognition or webkitSpeechRecognition.
func speechAvailable() bool {
	return speechConstructor().Truthy()
}

// speech carries out an ItemSpeech for session id, reporting EventSpeech through dispatch.
func speech(id uint32, r driver.SpeechRequest, dispatch func(driver.Event)) {
	switch r.Command {
	case driver.SpeechStart:
		if recognizing != nil {
			recognizing.abort()
		}
		ctor := speechConstructor()
		if !ctor.Truthy() {
			dispatch(driver.Event{
				Kind:  driver.EventSpeech,
				Count: id,
				Data:  &driver.SpeechEvent{Phase: driver.SpeechEnded, Error: driver.SpeechUnavailable},
			})
			return
		}
		var rec js.Value
		if errCode := callSpeechJS(func() { rec = ctor.New() }); errCode != "" || !rec.Truthy() {
			if errCode == "" {
				errCode = driver.SpeechUnavailable
			}
			dispatch(driver.Event{
				Kind:  driver.EventSpeech,
				Count: id,
				Data:  &driver.SpeechEvent{Phase: driver.SpeechEnded, Error: errCode},
			})
			return
		}
		run := &webSpeechRun{
			id:       id,
			interim:  r.Interim,
			rec:      rec,
			dispatch: dispatch,
		}
		recognizing = run
		rec.Set("continuous", false)
		rec.Set("interimResults", r.Interim)
		rec.Set("maxAlternatives", 1)
		if r.Lang != "" {
			rec.Set("lang", r.Lang)
		}
		run.onStart = js.FuncOf(func(js.Value, []js.Value) any {
			if run.done || recognizing != run || run.started {
				return nil
			}
			run.started = true
			run.report(driver.SpeechEvent{Phase: driver.SpeechStarted})
			return nil
		})
		run.onResult = js.FuncOf(func(_ js.Value, args []js.Value) any {
			if run.done || recognizing != run || len(args) == 0 {
				return nil
			}
			text, final, ok := webSpeechTranscript(args[0])
			if !ok || (!run.interim && !final) {
				return nil
			}
			run.text = text
			run.report(driver.SpeechEvent{Phase: driver.SpeechResult, Text: text, Final: final})
			return nil
		})
		run.onError = js.FuncOf(func(_ js.Value, args []js.Value) any {
			if run.done || recognizing != run {
				return nil
			}
			code := ""
			if len(args) > 0 && args[0].Truthy() {
				if errVal := args[0].Get("error"); errVal.Type() == js.TypeString {
					code = errVal.String()
				}
			}
			run.finish(webSpeechError(code))
			return nil
		})
		run.onEnd = js.FuncOf(func(js.Value, []js.Value) any {
			if run.done || recognizing != run {
				return nil
			}
			run.finish("")
			return nil
		})
		rec.Set("onstart", run.onStart)
		rec.Set("onresult", run.onResult)
		rec.Set("onerror", run.onError)
		rec.Set("onend", run.onEnd)
		run.funcsBound = true
		if errCode := callSpeechJS(func() { rec.Call("start") }); errCode != "" {
			run.finish(errCode)
		}
	case driver.SpeechStop:
		if run := recognizing; run != nil && run.id == id && !run.done {
			if errCode := callSpeechJS(func() { run.rec.Call("stop") }); errCode != "" {
				run.finish("")
			}
		}
	case driver.SpeechAbort:
		if run := recognizing; run != nil && run.id == id {
			run.abort()
		}
	}
}

func (run *webSpeechRun) report(e driver.SpeechEvent) {
	run.dispatch(driver.Event{Kind: driver.EventSpeech, Count: run.id, Data: &e})
}

func (run *webSpeechRun) finish(errCode string) {
	if run.done {
		return
	}
	run.done = true
	text := run.text
	run.release(false)
	run.report(driver.SpeechEvent{Phase: driver.SpeechEnded, Text: text, Error: errCode})
}

func (run *webSpeechRun) abort() {
	if run.done {
		return
	}
	run.done = true
	run.release(true)
}

func (run *webSpeechRun) release(abortRecognizer bool) {
	if recognizing == run {
		recognizing = nil
	}
	rec := run.rec
	run.rec = js.Undefined()
	if rec.Truthy() {
		rec.Set("onstart", js.Null())
		rec.Set("onresult", js.Null())
		rec.Set("onerror", js.Null())
		rec.Set("onend", js.Null())
		if abortRecognizer {
			_ = callSpeechJS(func() { rec.Call("abort") })
		}
	}
	if run.funcsBound {
		run.onStart.Release()
		run.onResult.Release()
		run.onError.Release()
		run.onEnd.Release()
		run.funcsBound = false
	}
}

// webSpeechTranscript reads the combined transcript and finality from a SpeechRecognitionEvent.
func webSpeechTranscript(ev js.Value) (text string, final bool, ok bool) {
	if !ev.Truthy() {
		return "", false, false
	}
	results := ev.Get("results")
	if !results.Truthy() {
		return "", false, false
	}
	n := results.Length()
	if n <= 0 {
		return "", false, false
	}
	var b strings.Builder
	allFinal := true
	hasAny := false
	for i := range n {
		res := results.Index(i)
		if !res.Truthy() || res.Length() <= 0 {
			continue
		}
		alt := res.Index(0)
		if !alt.Truthy() {
			continue
		}
		t := alt.Get("transcript")
		if t.Type() == js.TypeString {
			b.WriteString(t.String())
			hasAny = true
		}
		if !res.Get("isFinal").Bool() {
			allFinal = false
		}
	}
	if !hasAny {
		return "", false, false
	}
	return b.String(), allFinal, true
}

// webSpeechError maps a SpeechRecognitionErrorEvent.error string to driver's Speech errors.
func webSpeechError(code string) string {
	switch code {
	case "aborted":
		return ""
	case "not-allowed", "service-not-allowed":
		return driver.SpeechNotAllowed
	case "no-speech":
		return driver.SpeechNoSpeech
	case "audio-capture":
		return driver.SpeechAudioCapture
	case "network":
		return driver.SpeechNetwork
	case "language-not-supported":
		return driver.SpeechLanguageUnsupported
	default:
		return driver.SpeechFailed
	}
}

// callSpeechJS invokes fn, catching any JS exception and mapping it to a driver Speech error.
func callSpeechJS(fn func()) (errCode string) {
	defer func() {
		if r := recover(); r != nil {
			if je, ok := r.(js.Error); ok && je.Value.Truthy() {
				switch je.Value.Get("name").String() {
				case "NotAllowedError", "SecurityError":
					errCode = driver.SpeechNotAllowed
					return
				}
			}
			errCode = driver.SpeechFailed
		}
	}()
	fn()
	return ""
}
