package testhost

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestClicksRestartWhenTheViewChanges(t *testing.T) {
	var c Clicks
	start := time.Unix(0, 0)
	if got := c.Press(driver.Point{}, "first", start, time.Second); got != 1 {
		t.Fatalf("first click = %d, want 1", got)
	}
	if got := c.Press(driver.Point{}, "second", start.Add(10*time.Millisecond), time.Second); got != 1 {
		t.Fatalf("click on a new view = %d, want 1", got)
	}
	if got := c.Press(driver.Point{}, "second", start.Add(20*time.Millisecond), time.Second); got != 2 {
		t.Fatalf("second click on the new view = %d, want 2", got)
	}
}
