package android

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/core"
)

// animatorBatch is ops applied after the animator steps to at.
type animatorBatch struct {
	at  time.Duration
	ops []core.AnimationOp
}

// animatorScenario exercises every node kind, easing family, driver and
// composite, an interruption and a read.
func animatorScenario() []animatorBatch {
	const (
		a, b, c, d, e, f = 1, 2, 3, 4, 5, 6
	)
	val := func(id uint32, v float64) core.AnimationOp {
		return core.AnimationOp{Kind: core.AnimCreateValue, Node: id, Value: v}
	}
	interp := func(id, in uint32, r core.Interpolation) core.AnimationOp {
		return core.AnimationOp{Kind: core.AnimCreateInterpolate, Node: id, Input: in, Interpolation: &r}
	}
	arith := func(id uint32, op core.MathOp, x uint32, k float64) core.AnimationOp {
		return core.AnimationOp{Kind: core.AnimCreateMath, Node: id, Math: op, A: core.Operand{Node: x}, B: core.Operand{Const: k}}
	}
	bind := func(view core.NodeID, p core.AnimatedProp, node uint32) core.AnimationOp {
		return core.AnimationOp{Kind: core.AnimBind, View: view, Prop: p, Node: node}
	}
	timing := func(v uint32, to float64, ms int, e core.Easing) *core.Animation {
		return &core.Animation{Kind: core.AnimationTiming, Value: v, To: to,
			Timing: core.TimingConfig{Duration: time.Duration(ms) * time.Millisecond, Easing: e}}
	}
	start := func(id uint32, an *core.Animation) core.AnimationOp {
		return core.AnimationOp{Kind: core.AnimStart, ID: id, Animation: an}
	}
	red, green, blue := core.Color(0xff0000ff), core.Color(0x00ff0080), core.Color(0x0000ffff)
	first := []core.AnimationOp{
		val(a, 0), val(b, 0), val(c, 0), val(d, 0), val(e, 0), val(f, 0),
		interp(10, a, core.Interpolation{Input: []float64{0, 50, 100}, Colors: []core.Color{red, green, blue}}),
		interp(11, b, core.Interpolation{Input: []float64{0, 1}, Output: []float64{0, 300},
			ExtrapolateRight: core.ExtrapolateExtend, Easing: core.Easing{Kind: core.EasingBack, Mode: core.EaseModeOut}}),
		arith(12, core.MathMul, a, 0.01),
		arith(13, core.MathMod, c, 37),
		{Kind: core.AnimCreateDiffClamp, Node: 14, Input: c, Min: 0, Max: 60},
		interp(15, d, core.Interpolation{Input: []float64{0.2, 0.8}, Output: []float64{0, 1},
			Extrapolate: core.ExtrapolateIdentity, ExtrapolateRight: core.ExtrapolateClamp,
			Easing: core.Easing{Kind: core.EasingCircle, Mode: core.EaseModeInOut}}),
		interp(16, e, core.Interpolation{Input: []float64{0, 10}, Output: []float64{1, 0.5},
			Easing: core.Easing{Kind: core.EasingSin, Mode: core.EaseModeInOut}}),
		arith(17, core.MathDiv, f, -4),
		bind(1, core.PropOpacity, 12), bind(1, core.PropTranslateX, a), bind(1, core.PropBackground, 10),
		bind(2, core.PropScale, 11), bind(2, core.PropRotate, 13), bind(2, core.PropTranslateY, 14),
		bind(3, core.PropTranslateX, d), bind(3, core.PropScaleX, 16), bind(3, core.PropScaleY, 17),
		bind(3, core.PropShadowRadius, c), bind(3, core.PropBorderColor, 10), bind(4, core.PropOpacity, 15),
		start(1, &core.Animation{Kind: core.AnimationParallel, Steps: []*core.Animation{
			timing(a, 100, 1000, core.BezierEasing(core.EaseInOut)),
			{Kind: core.AnimationSpring, Value: b, To: 1, Spring: core.SpringConfig{Stiffness: 100, Damping: 10, Mass: 1}},
		}}),
		start(2, &core.Animation{Kind: core.AnimationDecay, Value: c,
			Decay: core.DecayConfig{Velocity: 1000, Deceleration: 0.998, Clamp: true, Min: -50, Max: 200, Bounce: 0.5}}),
		start(3, &core.Animation{Kind: core.AnimationLoop, Iterations: 3, Steps: []*core.Animation{
			{Kind: core.AnimationSequence, Steps: []*core.Animation{
				timing(d, 1, 300, core.Easing{Kind: core.EasingBounce, Mode: core.EaseModeOut}),
				{Kind: core.AnimationDelay, Delay: 50 * time.Millisecond},
				timing(d, 0, 200, core.Easing{Kind: core.EasingElastic, Mode: core.EaseModeInOut, Param: 2}),
			}},
		}}),
		start(4, &core.Animation{Kind: core.AnimationStagger, Delay: 100 * time.Millisecond, Steps: []*core.Animation{
			timing(e, 10, 250, core.Easing{Kind: core.EasingPoly, Param: 3}),
			timing(f, -10, 250, core.Easing{Kind: core.EasingExp, Mode: core.EaseModeOut}),
			{Kind: core.AnimationSpring, Value: e, To: 4, Spring: core.SpringConfig{Stiffness: 300, Damping: 30, Mass: 2, Velocity: 5}},
		}}),
	}
	return []animatorBatch{
		{at: time.Second, ops: first},
		// A Set interrupts run 1's timing, ending the run unfinished.
		{at: 1600 * time.Millisecond, ops: []core.AnimationOp{
			{Kind: core.AnimSet, Node: a, Value: 30},
			{Kind: core.AnimSetOffset, Node: a, Value: 5},
			{Kind: core.AnimRead, Node: a, ID: 9},
			start(5, timing(a, 80, 400, core.Easing{Kind: core.EasingCubic, Mode: core.EaseModeOut})),
		}},
		{at: 2300 * time.Millisecond, ops: []core.AnimationOp{
			{Kind: core.AnimFlattenOffset, Node: a},
			{Kind: core.AnimStop, ID: 3},
			{Kind: core.AnimRelease, Node: 13},
		}},
	}
}

// animatorSteps are the frame times, at an uneven rate.
func animatorSteps() []time.Duration {
	var out []time.Duration
	t := time.Second
	for i := 0; t < 4*time.Second; i++ {
		out = append(out, t)
		t += time.Duration(12+i*7%11) * time.Millisecond
	}
	return out
}

// runCoreAnimator samples core.Animator as the Java check does.
func runCoreAnimator(batches []animatorBatch, steps []time.Duration) []string {
	var out, reports []string
	a := core.NewAnimator(func(ev core.Event) {
		reports = append(reports, fmt.Sprintf("report %d %v %v", ev.Count, ev.Value, ev.Finished))
	})
	next := 0
	for _, t := range steps {
		a.Step(t)
		for next < len(batches) && batches[next].at <= t {
			a.Apply(batches[next].ops)
			next++
		}
		sort.Strings(reports)
		out = append(out, fmt.Sprintf("step %d", t))
		out = append(out, reports...)
		reports = reports[:0]
		for view := core.NodeID(1); view <= 4; view++ {
			v, _ := a.Values(view)
			out = append(out, fmt.Sprintf("view %d %d %v %v %v %v %v %v %v %v %s %s %s", view, v.Set, v.Opacity,
				v.TranslateX, v.TranslateY, v.Scale, v.ScaleX, v.ScaleY, v.Rotate, v.ShadowRadius,
				channels(v.Background), channels(v.BorderColor), channels(v.ShadowColor)))
		}
	}
	return out
}

func channels(c core.Color) string {
	return fmt.Sprintf("%d %d %d %d", c>>24, c>>16&0xff, c>>8&0xff, c&0xff)
}

// TestAnimatorMatchesJava runs the Java runtime's Animator on the JVM
// with the ops android.go encodes and compares what it samples with
// core.Animator's. It needs a JDK on PATH.
func TestAnimatorMatchesJava(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil || exec.Command(javac, "-version").Run() != nil {
		t.Skip("no JDK")
	}
	batches, steps := animatorScenario(), animatorSteps()
	w := &writer{}
	w.u32(uint32(len(batches)))
	for _, b := range batches {
		w.u64(uint64(b.at))
		writeAnimation(w, b.ops)
	}
	w.u32(uint32(len(steps)))
	for _, s := range steps {
		w.u64(uint64(s))
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "ops")
	if err := os.WriteFile(input, w.buf, 0o644); err != nil {
		t.Fatal(err)
	}
	build := exec.Command(javac, "-d", dir, "java/dev/oneo/nsgo/Animator.java", "testdata/AnimatorCheck.java")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	out, err := exec.Command("java", "-cp", dir, "dev.oneo.nsgo.AnimatorCheck", input).Output()
	if err != nil {
		t.Fatalf("java: %v", err)
	}
	want := runCoreAnimator(batches, steps)
	got := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(got) != len(want) {
		t.Fatalf("Java printed %d lines, core %d", len(got), len(want))
	}
	moving := 0
	for i := range want {
		if !sameLine(got[i], want[i]) {
			t.Fatalf("line %d differs:\n java %s\n core %s", i, got[i], want[i])
		}
		if i > 0 && want[i] != want[max(i-5, 0)] {
			moving++
		}
	}
	if moving < len(want)/4 {
		t.Fatalf("the scenario hardly moves: %d of %d lines change", moving, len(want))
	}
	t.Logf("%d lines over %d steps match", len(want), len(steps))
}

// sameLine compares two lines word by word, numbers within float32
// rounding and color channels within 1.
func sameLine(a, b string) bool {
	x, y := strings.Fields(a), strings.Fields(b)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] == y[i] {
			continue
		}
		p, err1 := strconv.ParseFloat(x[i], 64)
		q, err2 := strconv.ParseFloat(y[i], 64)
		if err1 != nil || err2 != nil || math.Abs(p-q) > max(1e-3, 1e-4*math.Abs(q)) && math.Abs(p-q) > 1 {
			return false
		}
		if math.Abs(p-q) > max(1e-3, 1e-4*math.Abs(q)) && !isChannel(i, x) {
			return false
		}
	}
	return true
}

// isChannel reports whether word i of a view line is a color channel.
func isChannel(i int, words []string) bool { return words[0] == "view" && i >= 11 }
