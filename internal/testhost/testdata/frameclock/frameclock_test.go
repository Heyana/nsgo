package frameclock_test

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

type reading struct {
	stamp, arrival time.Time
}

func TestDisplayFrameTimes(t *testing.T) {
	target := nsgotest.Target()
	if target == "" {
		t.Skip("requires a target's display clock")
	}
	var period time.Duration
	if value := os.Getenv("NSGO_FRAME_TEST_HZ"); value != "" {
		hz, err := strconv.ParseFloat(value, 64)
		ns := math.Round(float64(time.Second) / hz)
		if err != nil || !(ns >= 1 && ns < float64(math.MaxInt64)) {
			t.Fatalf("NSGO_FRAME_TEST_HZ = %q, want a positive refresh rate with a representable period", value)
		}
		period = time.Duration(ns)
	}

	const warmup, intervals = 60, 180
	const frames = warmup + intervals + 1
	draw := os.Getenv("NSGO_FRAME_TEST_DRAW") == "1"
	result := make(chan []reading, 1)
	nsgotest.Start(t, func(cx *core.Scope) core.Node {
		samples := make([]reading, 0, frames)
		ink := core.NewSignal(cx, core.RGB(0x123456))
		var frame func(time.Time)
		frame = func(at time.Time) {
			samples = append(samples, reading{stamp: at, arrival: time.Now()})
			if draw {
				ink.Set(core.RGB(0x123456 + uint32(len(samples)%2)*0x00000f))
			}
			if len(samples) == frames {
				result <- samples
				return
			}
			cx.RequestAnimationFrame(frame)
		}
		cx.RequestAnimationFrame(frame)
		return core.View{StyleFunc: func() core.Style {
			return core.Style{Width: core.Pct(100), Height: core.Pct(100), Background: ink.Get()}
		}}
	}, nsgotest.Size(280, 180), nsgotest.Measure())

	var samples []reading
	select {
	case samples = <-result:
	case <-time.After(25 * time.Second):
		t.Fatal("the frame clock did not deliver the sample")
	}

	gaps := make([]time.Duration, len(samples)-1)
	var latestLead time.Duration
	var backward, repeated, startupRepeated int
	for i, s := range samples {
		if i >= warmup {
			latestLead = max(latestLead, s.stamp.Sub(s.arrival))
		}
		if i == 0 {
			continue
		}
		gaps[i-1] = s.stamp.Sub(samples[i-1].stamp)
		switch {
		case gaps[i-1] < 0:
			backward++
		case gaps[i-1] == 0 && i <= warmup:
			startupRepeated++
		case gaps[i-1] == 0:
			repeated++
		}
	}

	startup := analyzeCadence(gaps[:warmup], period)
	gaps = gaps[warmup:]
	steady := analyzeCadence(gaps, period)
	slices.Sort(gaps)
	fmt.Printf("nsgo: perf mark frame clock target=%s intervals=%d nominal=%v min=%v p50=%v p95=%v max=%v max_lead=%v single=%d missed=%d off_cadence=%d max_error=%v startup_repeated=%d startup_off_cadence=%d startup_max_error=%v draw=%t\n",
		target, len(gaps), period, gaps[0], gaps[len(gaps)/2], gaps[len(gaps)*95/100], gaps[len(gaps)-1],
		latestLead, steady.single, steady.missed, steady.off, steady.maxError, startupRepeated, startup.off, startup.maxError, draw)
	if backward != 0 || repeated != 0 {
		t.Errorf("got %d backward timestamps and %d repeated timestamps after startup", backward, repeated)
	}
	if period == 0 {
		return
	}
	if steady.off != 0 || steady.single*10 < len(gaps)*9 {
		t.Errorf("want at least 90%% single-refresh intervals and every gap within %v of a whole number of refreshes", cadenceTolerance)
	}
	if (target == "macos" || target == "ios" || target == "ios_simulator") && latestLead < time.Millisecond {
		t.Error("no frame timestamp was at least 1 ms ahead of its callback, want a display link's target timestamp")
	}
}

const cadenceTolerance = 500 * time.Microsecond

type cadence struct {
	single, missed, off int
	maxError            time.Duration
}

func analyzeCadence(gaps []time.Duration, period time.Duration) (c cadence) {
	if period == 0 {
		return c
	}
	for _, gap := range gaps {
		ticks := max(1, int(math.Round(float64(gap)/float64(period))))
		deviation := (gap - time.Duration(ticks)*period).Abs()
		c.maxError = max(c.maxError, deviation)
		switch {
		case deviation > cadenceTolerance:
			c.off++
		case ticks == 1:
			c.single++
		default:
			c.missed += ticks - 1
		}
	}
	return c
}
