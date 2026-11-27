package keys_test

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func nativeSelectionTarget(t *testing.T) {
	t.Helper()
	if !slices.Contains([]string{"ios", "ios_simulator", "android", "harmonyos"}, nsgotest.Target()) {
		t.Skip("checks phone targets' native UTF-16 selection")
	}
}

func checkNativeUTF16Selection(t *testing.T, multiline bool) {
	t.Helper()
	nativeSelectionTarget(t)
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.Input{TestID: "field", DefaultValue: "A😀e\u0301Z", Multiline: multiline,
			Style: core.Style{Width: core.Pt(240), Height: core.Pt(80)}}
	}, nsgotest.Size(300, 160))
	// UTF-16 offset 3 is after the supplementary emoji; e and its combining
	// accent occupy two units. Reversed ranges normalize before reaching native.
	app.Select("id:field", 5, 3)
	if got := app.Find("id:field").Selection; got != (core.TextRange{Start: 3, End: 5}) {
		t.Fatalf("native selection without a callback = %v", got)
	}
	app.Type("id:field", "X")
	if got := app.Find("id:field").Text; got != "A😀XZ" {
		t.Fatalf("typing over a UTF-16 range = %q", got)
	}
	app.Select("id:field", -3, 999)
	app.Type("id:field", "Q")
	app.Select("id:field", 0, 0)
	app.Type("id:field", "!")
	if got := app.Find("id:field").Text; got != "!Q" {
		t.Fatalf("clamped range and collapsed caret = %q", got)
	}
}

func TestNativeSelectUTF16SingleLine(t *testing.T) { checkNativeUTF16Selection(t, false) }
func TestNativeSelectUTF16Multiline(t *testing.T)  { checkNativeUTF16Selection(t, true) }

func TestNativeSelectThenCopyAndPaste(t *testing.T) {
	if !slices.Contains([]string{"ios_simulator", "android", "harmonyos"}, nsgotest.Target()) {
		t.Skip("requires native clipboard editing keys")
	}
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.Input{TestID: "field", DefaultValue: "hello world",
			Style: core.Style{Width: core.Pt(280), Height: core.Pt(40)}}
	}, nsgotest.Size(320, 140))
	app.Select("id:field", 0, 5)
	app.Key("mod+c")
	app.Select("id:field", 6, 11)
	app.Key("mod+v")
	if got := app.Find("id:field").Text; got != "hello hello" {
		t.Fatalf("native copy/paste acted on a different selection: %q", got)
	}
}
