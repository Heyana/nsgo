package review_test

import (
	"os"
	"testing"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// Run against a Google Play emulator or device with NSGO_TEST_PLAY_REVIEW=1. A locally installed test app cannot prove the store displays its rating card.
func TestStandaloneStoreReviewService(t *testing.T) {
	if os.Getenv("NSGO_TEST_PLAY_REVIEW") != "1" {
		t.Skip("needs an enabled Google Play Store")
	}
	var available bool
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.Effect(func() { available = cx.Env().CanRequestReview() })
		return core.View{TestID: "review", Style: core.Style{Width: core.Pt(100), Height: core.Pt(50)},
			OnPress: func(core.PressEvent) { cx.RequestReview() }}
	})
	if !available {
		t.Fatal("standalone package cannot load Play Review with an enabled Play Store")
	}
	app.Press("id:review")
	if app.ReviewRequests != 1 {
		t.Fatalf("review requests = %d, want 1", app.ReviewRequests)
	}
}
