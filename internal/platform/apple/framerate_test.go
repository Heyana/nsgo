//go:build darwin

package apple

import (
	"testing"

	"github.com/ebitengine/purego/objc"
)

func TestSetAnimationFrameRate(t *testing.T) {
	anim := objc.ID(Class("CABasicAnimation")).Send(sel("animationWithKeyPath:"), NSString("transform.translation.x"))
	if anim == 0 {
		t.Fatal("failed to allocate CABasicAnimation")
	}
	if !objc.Send[bool](anim, sel("respondsToSelector:"), selSetPreferredFrameRateRange) {
		t.Skip("preferredFrameRateRange unavailable on this OS")
	}

	SetAnimationFrameRate(anim, 120)
	got := objc.Send[CAFrameRateRange](anim, sel("preferredFrameRateRange"))
	want := CAFrameRateRange{Minimum: 80, Maximum: 120, Preferred: 120}
	if got != want {
		t.Fatalf("preferredFrameRateRange(120) = %+v, want %+v", got, want)
	}

	SetAnimationFrameRate(anim, 60)
	got60 := objc.Send[CAFrameRateRange](anim, sel("preferredFrameRateRange"))
	want60 := CAFrameRateRange{Minimum: 60, Maximum: 60, Preferred: 60}
	if got60 != want60 {
		t.Fatalf("preferredFrameRateRange(60) = %+v, want %+v", got60, want60)
	}
}
