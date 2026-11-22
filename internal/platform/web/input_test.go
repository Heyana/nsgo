//go:build js && wasm

package web

import (
	"math"
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// TestMultilineInputScrollsUnderPadding checks that a multiline input's
// textarea fills the padding box with its text inset by InputConfig.Padding,
// so its scroll bar runs along the edges and its text scrolls under the
// padding, while a single-line input stays at the content box (#1598).
func TestMultilineInputScrollsUnderPadding(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("run with nsgo test -target web")
	}
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-multiline-padding")
	setStyle(mount, map[string]string{"width": "320px", "height": "440px"})
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")

	pad := core.Edges{Top: 18, Right: 20, Bottom: 20, Left: 12}
	lines := strings.Repeat("line of text\n", 20)
	s := open(func(*core.Scope) core.Node {
		return core.View{
			Style: core.Style{Width: core.Pt(320), Height: core.Pt(440)},
			Children: []core.Node{
				core.Input{
					TestID:    "multi",
					Multiline: true,
					Value:     func() string { return lines },
					Style: core.Style{
						Width:       core.Pt(300),
						Height:      core.Pt(200),
						Padding:     pad,
						BorderWidth: 1,
						BorderColor: core.RGB(0xcccccc),
					},
				},
				core.Input{
					TestID:    "single",
					Multiline: false,
					Value:     func() string { return "hello" },
					Style: core.Style{
						Width:       core.Pt(300),
						Height:      core.Pt(200),
						Padding:     pad,
						BorderWidth: 1,
						BorderColor: core.RGB(0xcccccc),
					},
				},
			},
		}
	}, Options{Mount: "#nsgo-multiline-padding"}, nil)
	defer s.engine.Close()

	var multi, single js.Value
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		multi = mount.Call("querySelector", `[data-testid="multi"]`)
		single = mount.Call("querySelector", `[data-testid="single"]`)
		if multi.Truthy() && single.Truthy() {
			break
		}
	}
	if !multi.Truthy() || !single.Truthy() {
		t.Fatal("inputs did not mount")
	}
	// multi is the <textarea> (ARIA props sit on v.inner), its parent is the bordered view <div>.
	if tag := multi.Get("tagName").String(); tag != "TEXTAREA" {
		t.Fatalf("multiline tag = %q, want TEXTAREA", tag)
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 0.5 }
	mr := multi.Call("getBoundingClientRect")
	vr := multi.Get("parentElement").Call("getBoundingClientRect")
	if !near(mr.Get("left").Float()-vr.Get("left").Float(), 1) ||
		!near(mr.Get("top").Float()-vr.Get("top").Float(), 1) ||
		!near(mr.Get("width").Float(), 298) ||
		!near(mr.Get("height").Float(), 198) {
		t.Errorf("textarea box = (%v, %v, %v, %v) relative to view, want padding box (1, 1, 298, 198)",
			mr.Get("left").Float()-vr.Get("left").Float(),
			mr.Get("top").Float()-vr.Get("top").Float(),
			mr.Get("width").Float(), mr.Get("height").Float())
	}
	cs := js.Global().Call("getComputedStyle", multi)
	for prop, want := range map[string]string{
		"padding-top":        "18px",
		"padding-right":      "20px",
		"padding-bottom":     "20px",
		"padding-left":       "12px",
		"scroll-padding-top": "18px",
	} {
		if got := cs.Call("getPropertyValue", prop).String(); got != want {
			t.Errorf("textarea %s = %q, want %q", prop, got, want)
		}
	}
	if sh, ch := multi.Get("scrollHeight").Float(), multi.Get("clientHeight").Float(); sh <= ch {
		t.Fatalf("textarea did not overflow: scrollHeight=%v clientHeight=%v", sh, ch)
	}
	multi.Set("scrollTop", 40)
	if got := multi.Get("scrollTop").Float(); got < 30 {
		t.Errorf("textarea scrollTop = %v after scrolling past top padding, want >= 30", got)
	}

	// Single-line input stays in the content box with zero inner padding.
	if tag := single.Get("tagName").String(); tag != "INPUT" {
		t.Fatalf("single-line tag = %q, want INPUT", tag)
	}
	sr := single.Call("getBoundingClientRect")
	svr := single.Get("parentElement").Call("getBoundingClientRect")
	if !near(sr.Get("left").Float()-svr.Get("left").Float(), 13) ||
		!near(sr.Get("top").Float()-svr.Get("top").Float(), 19) ||
		!near(sr.Get("width").Float(), 266) ||
		!near(sr.Get("height").Float(), 160) {
		t.Errorf("single-line box = (%v, %v, %v, %v) relative to view, want content box (13, 19, 266, 160)",
			sr.Get("left").Float()-svr.Get("left").Float(),
			sr.Get("top").Float()-svr.Get("top").Float(),
			sr.Get("width").Float(), sr.Get("height").Float())
	}
	scs := js.Global().Call("getComputedStyle", single)
	if got := scs.Call("getPropertyValue", "padding-top").String(); got != "0px" {
		t.Errorf("single-line padding-top = %q, want 0px", got)
	}
}

// TestLanguageInInputsAndKoreanWordBreak checks that inputs carry lang from Style.Language and that Korean texts and inputs use word-break: keep-all.
func TestLanguageInInputsAndKoreanWordBreak(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("run with nsgo test -target web")
	}
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-input-language")
	setStyle(mount, map[string]string{"width": "320px", "height": "240px"})
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")

	s := open(func(*core.Scope) core.Node {
		return core.View{
			Style: core.Style{Width: core.Pt(320), Height: core.Pt(240)},
			Children: []core.Node{
				core.Input{TestID: "ja-in", Style: core.Style{Language: "ja", Width: core.Pt(200), Height: core.Pt(32)}},
				core.Input{TestID: "ko-in", Multiline: true, Style: core.Style{Language: "ko-KR", Width: core.Pt(200), Height: core.Pt(60)}},
				core.Text{TestID: "ko-txt", Content: "안녕하세요 반갑습니다", Style: core.Style{Language: "ko"}},
			},
		}
	}, Options{Mount: "#nsgo-input-language"}, nil)
	defer s.engine.Close()

	var jaIn, koIn, koTxt js.Value
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		jaIn = mount.Call("querySelector", `[data-testid="ja-in"]`)
		koIn = mount.Call("querySelector", `[data-testid="ko-in"]`)
		koTxt = mount.Call("querySelector", `[data-testid="ko-txt"] .nsgo-text`)
		if jaIn.Truthy() && koIn.Truthy() && koTxt.Truthy() {
			break
		}
	}
	if !jaIn.Truthy() || !koIn.Truthy() || !koTxt.Truthy() {
		t.Fatal("elements did not mount")
	}
	if got := jaIn.Call("getAttribute", "lang").String(); got != "ja" {
		t.Errorf("ja input lang = %q, want ja", got)
	}
	if got := koIn.Call("getAttribute", "lang").String(); got != "ko-KR" {
		t.Errorf("ko input lang = %q, want ko-KR", got)
	}
	cs := js.Global().Call("getComputedStyle", koIn)
	if got := cs.Call("getPropertyValue", "word-break").String(); got != "keep-all" {
		t.Errorf("ko input word-break = %q, want keep-all", got)
	}
	tcs := js.Global().Call("getComputedStyle", koTxt)
	if got := tcs.Call("getPropertyValue", "word-break").String(); got != "keep-all" {
		t.Errorf("ko text word-break = %q, want keep-all", got)
	}
}
