package ui

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestNumberInput steps the value with its buttons up to the range's
// end, reads what is typed to the step, gives back text that is no
// number, and scrubs with a drag across its handle.
func TestNumberInput(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var value *core.Signal[float64]
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			value = core.NewSignal(cx, 1.5)
			return NumberInput{TestID: "number", Label: "Opacity", Value: value.Get, OnChange: value.Set, Min: 0, Max: 2, Step: 0.5,
				Leading: lucide.SlidersHorizontal, IncrementLabel: "Increase", DecrementLabel: "Decrease"}
		})
		if got := app.Find("label:Opacity").Text; got != "1.5" {
			t.Fatalf("the field shows %q, want 1.5", got)
		}
		app.Press("label:Increase")
		if value.Get() != 2 || !app.Find("label:Increase").Props.Disabled {
			t.Fatalf("value = %v; want 2 with the increase button disabled at the end", value.Get())
		}
		app.Clear("label:Opacity")
		app.Type("label:Opacity", "0.7")
		app.Submit("label:Opacity")
		if value.Get() != 0.5 {
			t.Fatalf("typed 0.7 gave %v, want 0.5 on the step", value.Get())
		}
		app.Clear("label:Opacity")
		app.Type("label:Opacity", "lots")
		app.Submit("label:Opacity")
		if got := app.Find("label:Opacity").Text; value.Get() != 0.5 || got != "0.5" {
			t.Fatalf("text that is no number left %v showing %q", value.Get(), got)
		}
		app.Pan("number-scrub", core.Point{X: 4}, core.Point{X: 9})
		if value.Get() != 1.5 {
			t.Fatalf("a scrub of two steps gave %v, want 1.5", value.Get())
		}
	})
}

// TestNumberInputKeys steps the focused field with the arrow keys.
func TestNumberInputKeys(t *testing.T) {
	var value *core.Signal[float64]
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		value = core.NewSignal(cx, 3.0)
		return NumberInput{Label: "Count", Value: value.Get, OnChange: value.Set}
	})
	app.Focus("label:Count")
	app.Key("up")
	app.Key("up")
	app.Key("down")
	if value.Get() != 4 || app.Find("label:Count").Text != "4" {
		t.Fatalf("value = %v, want 4", value.Get())
	}
}

func TestNumberInputPrecision(t *testing.T) {
	const field = "label:Gravity"
	var value *core.Signal[float64]
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		value = core.NewSignal(cx, 9.81)
		return NumberInput{TestID: "gravity", Label: "Gravity", Value: value.Get, OnChange: value.Set,
			Min: 0, Max: 100, Step: 0.1, Precision: 0.01, IncrementLabel: "Increase", LeadingText: "g"}
	})
	checkNumberInputValue(t, app, value, field, 9.81, "9.81")
	app.Post(func() { value.Set(9.8) })
	app.Settle()
	app.Clear(field)
	app.Type(field, "9.81")
	app.Submit(field)
	checkNumberInputValue(t, app, value, field, 9.81, "9.81")

	app.Press("label:Increase")
	checkNumberInputValue(t, app, value, field, 9.91, "9.91")
	app.Focus(field)
	app.Key("down")
	checkNumberInputValue(t, app, value, field, 9.81, "9.81")
	app.Pan("gravity-scrub", core.Point{X: 3}, core.Point{X: 4})
	checkNumberInputValue(t, app, value, field, 9.91, "9.91")
}

func TestNumberInputPrecisionFromMin(t *testing.T) {
	const field = "label:Offset"
	var value *core.Signal[float64]
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		value = core.NewSignal(cx, 0.005)
		return NumberInput{Label: "Offset", Value: value.Get, OnChange: value.Set,
			Min: 0.005, Max: 100, Step: 0.1, Precision: 0.01}
	})
	app.Clear(field)
	app.Type(field, "9.811")
	app.Submit(field)
	checkNumberInputValue(t, app, value, field, 9.815, "9.815")
}

func TestNumberInputNoSnap(t *testing.T) {
	const field = "label:Gravity"
	var value *core.Signal[float64]
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		value = core.NewSignal(cx, 9.8)
		return NumberInput{Label: "Gravity", Value: value.Get, OnChange: value.Set,
			Min: 0, Max: 100, Step: 0.1, NoSnap: true}
	})
	app.Clear(field)
	app.Type(field, "9.8123")
	app.Submit(field)
	checkNumberInputValue(t, app, value, field, 9.8123, "9.8123")

	// Stepping starts from the committed value, not the unsubmitted draft.
	app.Clear(field)
	app.Type(field, "15")
	app.Key("up")
	checkNumberInputValue(t, app, value, field, 9.9123, "9.9123")

	app.Clear(field)
	app.Type(field, "120")
	app.Submit(field)
	checkNumberInputValue(t, app, value, field, 100, "100")
}

func TestNumberInputInvalidPrecisionUsesStep(t *testing.T) {
	for name, precision := range map[string]float64{
		"negative": -0.01,
		"nan":      math.NaN(),
		"infinite": math.Inf(1),
	} {
		t.Run(name, func(t *testing.T) {
			const field = "label:Value"
			var value *core.Signal[float64]
			app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
				value = core.NewSignal(cx, 1.0)
				return NumberInput{Label: "Value", Value: value.Get, OnChange: value.Set,
					Min: 0, Max: 100, Step: 0.1, Precision: precision}
			})
			app.Clear(field)
			app.Type(field, "9.87")
			app.Submit(field)
			checkNumberInputValue(t, app, value, field, 9.9, "9.9")
		})
	}
}

func checkNumberInputValue(t *testing.T, app *nsgotest.App, value *core.Signal[float64], field string, want float64, wantText string) {
	t.Helper()
	var got float64
	app.Post(func() { got = value.Get() })
	app.Settle()
	if text := app.Find(field).Text; got != want || text != wantText {
		t.Fatalf("value = %v, shown %q; want %v, shown %q", got, text, want, wantText)
	}
}
