package webstyle

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestDefaultsWriteNothing(t *testing.T) {
	vs := driver.DefaultViewStyle
	if Fill(0) != "" || Opacity(1) != "" || Transform(driver.Transform{}) != "" || Transition(driver.Transition{}) != "" ||
		Radius(&vs) != "" || Shadow(&vs) != "" || Filter(nil) != "" {
		t.Fatal("a default value wrote CSS")
	}
}

func TestValues(t *testing.T) {
	if got := Color(driver.RGB(0xff8000)); got != "rgba(255,128,0,1)" {
		t.Errorf("Color = %q", got)
	}
	if got := Px(1.5); got != "1.5px" {
		t.Errorf("Px = %q", got)
	}
	vs := driver.ViewStyle{Corners: driver.Corners{TopLeft: 1, TopRight: 2, BottomRight: 3, BottomLeft: 4}}
	if got := Radius(&vs); got != "1px 2px 3px 4px" {
		t.Errorf("Radius = %q", got)
	}
	tr := driver.Transition{Duration: 200 * time.Millisecond, Properties: driver.TransitionOpacity}
	if got := Transition(tr); got != "opacity 200ms linear 0ms" {
		t.Errorf("Transition = %q", got)
	}
	if got := Decoration(driver.TextDecorationUnderline | driver.TextDecorationLineThrough); got != "underline line-through" {
		t.Errorf("Decoration = %q", got)
	}
	if got := Transform(driver.Transform{TranslateX: 2, Rotate: 90}); got != "translate(2px, 0px) rotate(90deg) scale(1, 1)" {
		t.Errorf("Transform = %q", got)
	}
}

func TestTransitionForAndAnimatedInline(t *testing.T) {
	tr := driver.Transition{Duration: 200 * time.Millisecond, Easing: driver.EaseOut}

	// TranslateX bound: only transform is written inline and excluded from transition.
	base := driver.ViewStyle{Opacity: 0.5, Background: driver.RGB(0x112233), BorderColor: driver.RGB(0x445566), Transition: tr}
	vals := driver.AnimatedValues{Set: 1 << driver.PropTranslateX, TranslateX: 12}
	got := AnimatedInline(base, vals)
	if got["transform"] == "" || got["will-change"] != "transform" {
		t.Errorf("bound transform inline = %q will-change=%q", got["transform"], got["will-change"])
	}
	for _, k := range []string{"opacity", "background-color", "border-color", "box-shadow", "-webkit-text-fill-color"} {
		if got[k] != "" {
			t.Errorf("unbound %q wrote inline %q, clobbering class transitions", k, got[k])
		}
	}
	if strings.Contains(got["transition"], "transform") {
		t.Errorf("transition still includes bound transform: %q", got["transition"])
	}
	for _, want := range []string{"opacity", "background-color", "border-width", "border-color", "border-radius", "box-shadow"} {
		if !strings.Contains(got["transition"], want) {
			t.Errorf("transition missing unbound %q: %q", want, got["transition"])
		}
	}

	// Only Opacity transitioned and Opacity is bound -> transition becomes "none".
	opOnly := base
	opOnly.Transition = driver.Transition{Duration: 200 * time.Millisecond, Properties: driver.TransitionOpacity}
	if g := AnimatedInline(opOnly, driver.AnimatedValues{Set: 1 << driver.PropOpacity, Opacity: 0.3}); g["transition"] != "none" || g["opacity"] != "0.3" {
		t.Errorf("opacity-only bound = %+v", g)
	}

	// Only Background transitioned and Opacity is bound -> inline transition stays "" so the class transition runs untouched.
	bgOnly := base
	bgOnly.Transition = driver.Transition{Duration: 200 * time.Millisecond, Properties: driver.TransitionBackground}
	if g := AnimatedInline(bgOnly, driver.AnimatedValues{Set: 1 << driver.PropOpacity, Opacity: 1}); g["transition"] != "" || g["opacity"] != "1" {
		t.Errorf("disjoint bound opacity = %+v", g)
	}

	// BorderColor bound with TransitionBorder -> keeps border-width, border-radius, outline-* and strips only border-color.
	borderTr := driver.Transition{Duration: 150 * time.Millisecond, Properties: driver.TransitionBorder}
	filtered := TransitionFor(borderTr, 1<<driver.PropBorderColor)
	if strings.Contains(filtered, "border-color") {
		t.Errorf("filtered border transition still has border-color: %q", filtered)
	}
	for _, want := range []string{"border-width", "border-radius", "outline-color", "outline-width", "outline-offset"} {
		if !strings.Contains(filtered, want) {
			t.Errorf("filtered border transition missing %q: %q", want, filtered)
		}
	}

	// ShadowRadius bound -> writes box-shadow inline and excludes box-shadow from transition while keeping opacity.
	shGot := AnimatedInline(base, driver.AnimatedValues{Set: 1 << driver.PropShadowRadius, ShadowRadius: 8})
	if shGot["box-shadow"] == "" || strings.Contains(shGot["transition"], "box-shadow") || !strings.Contains(shGot["transition"], "opacity") {
		t.Errorf("bound shadowRadius = %+v", shGot)
	}

	// TextColor bound -> writes -webkit-text-fill-color inline and leaves class transitions untouched.
	txtGot := AnimatedInline(base, driver.AnimatedValues{Set: 1 << driver.PropTextColor, TextColor: driver.RGB(0xff0000)})
	if txtGot["-webkit-text-fill-color"] == "" || txtGot["transition"] != "" {
		t.Errorf("bound textColor = %+v", txtGot)
	}

	// Unbinding clears every inline property.
	cleared := AnimatedInline(base, driver.AnimatedValues{})
	for k, v := range cleared {
		if v != "" {
			t.Errorf("unbound %q = %q, want empty", k, v)
		}
	}
}
