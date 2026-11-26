package textview

import (
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/nsgo-dev/nsgo/core"
)

// The characters a document gains fade in one after another, a step
// apart, each at most a lag behind its arrival: a burst of tokens spreads
// out over the lag rather than showing at once, and a slow stream shows
// each character as it comes.
const (
	fadeStep   = 14 * time.Millisecond
	fadeLag    = 160 * time.Millisecond
	fadeLevels = 32
	// fadeReduced is the longest fade under reduced motion, which shows
	// a chunk's characters together.
	fadeReduced = 200 * time.Millisecond
)

// fader fades in the characters a view's texts gain after it shows, by
// each text's place, so a text made again as its block's frame changes
// keeps the fades of the one before.
type fader struct {
	cx       *core.Scope
	duration time.Duration
	step     time.Duration
	// settled is set once the view's first texts are made: characters
	// shown then do not fade.
	settled bool
	texts   map[string]*fadeText
	// now is the time of the frame the fades show, which the texts in
	// flight follow.
	now *core.Signal[time.Time]
	// last is when the latest character starts to fade in, and until when
	// the last fade ends.
	last, until time.Time
	running     bool
}

// fadeText is what a text's characters fade: those before done show
// whole, then each run's from its start, step apart.
type fadeText struct {
	done  int
	runs  []fadeRun
	units int
}

type fadeRun struct {
	end   int
	start time.Time
	step  time.Duration
}

func newFader(cx *core.Scope, d time.Duration) *fader {
	f := &fader{cx: cx, duration: d, step: fadeStep, texts: map[string]*fadeText{}, now: core.NewSignal(cx, cx.Now())}
	if core.Untracked(cx, cx.Env().ReduceMotion) {
		f.duration, f.step = min(d, fadeReduced), 0
	}
	cx.AfterFunc(0, func() { f.settled = true })
	return f
}

// text is the fade of the text at place.
func (f *fader) text(place TextRef) *fadeText {
	key := place.key()
	t := f.texts[key]
	if t == nil {
		t = &fadeText{}
		f.texts[key] = t
	}
	return t
}

// grow records that t shows n units now, starting the fades of those it
// gains.
func (f *fader) grow(t *fadeText, n int) {
	if n < t.units {
		// The text lost characters, as a held line was made again.
		t.done = min(t.done, n)
		for i, r := range t.runs {
			if r.end > n {
				t.runs = t.runs[:i]
				if i == 0 || t.runs[i-1].end < n {
					t.runs = append(t.runs, fadeRun{end: n, start: r.start, step: r.step})
				}
				break
			}
		}
		t.units = n
		return
	}
	if n == t.units {
		return
	}
	added := n - t.units
	t.units = n
	if !f.settled {
		t.done = n
		return
	}
	now := f.cx.Now()
	start := now
	if next := f.last.Add(f.step); next.After(start) {
		start = next
	}
	if latest := now.Add(fadeLag); start.After(latest) {
		start = latest
	}
	step := f.step
	if added > 1 && start.Add(step*time.Duration(added-1)).After(now.Add(fadeLag)) {
		step = now.Add(fadeLag).Sub(start) / time.Duration(added-1)
	}
	t.runs = append(t.runs, fadeRun{end: n, start: start, step: step})
	f.last = start.Add(step * time.Duration(added-1))
	if end := f.last.Add(f.duration); end.After(f.until) {
		f.until = end
	}
	f.run()
}

// run moves the frames on while a fade is in flight.
func (f *fader) run() {
	if f.running {
		return
	}
	f.running = true
	var frame func(at time.Time)
	frame = func(at time.Time) {
		f.now.Set(at)
		if at.Before(f.until) {
			f.cx.RequestAnimationFrame(frame)
			return
		}
		f.running = false
	}
	// Under the view's scope, as the text growing may be made again first.
	f.cx.Post(func() { f.cx.RequestAnimationFrame(frame) })
}

// alpha is unit i's opacity at now, in fadeLevels steps.
func (f *fader) alpha(t *fadeText, i int, now time.Time) float32 {
	if i < t.done {
		return 1
	}
	from := t.done
	for _, r := range t.runs {
		if i < r.end {
			start := r.start.Add(r.step * time.Duration(i-from))
			p := float64(now.Sub(start)) / float64(f.duration)
			if p <= 0 {
				return 0
			}
			if p >= 1 {
				return 1
			}
			// Ease out: quick to show, slow to settle.
			e := 1 - (1-p)*(1-p)*(1-p)
			return float32(math.Round(e*fadeLevels) / fadeLevels)
		}
		from = r.end
	}
	return 1
}

// spans fades spans, the text t shows: its characters split into spans
// of one opacity each, those yet to fade in transparent. It reads the
// frame's time while a fade in it is in flight.
func (f *fader) spans(t *fadeText, spans []core.Span) []core.Span {
	f.grow(t, spanUnits(spans))
	if t.done == t.units {
		return spans
	}
	now := f.now.Get()
	// Runs whose fades are over show whole.
	from := t.done
	for len(t.runs) > 0 {
		r := t.runs[0]
		lastStart := r.start.Add(r.step * time.Duration(r.end-from-1))
		if now.Before(lastStart.Add(f.duration)) {
			break
		}
		t.done, from, t.runs = r.end, r.end, t.runs[1:]
	}
	if t.done == t.units {
		return spans
	}
	out := make([]core.Span, 0, len(spans)+4)
	i := 0
	for _, sp := range spans {
		if sp.Attachment != nil || sp.Content == "" {
			out = append(out, sp)
			continue
		}
		var b strings.Builder
		level := float32(-1)
		flush := func() {
			if b.Len() == 0 {
				return
			}
			s := sp
			s.Content = b.String()
			if level < 1 {
				s.Style.Opacity = core.Alpha(level)
			}
			out = append(out, s)
			b.Reset()
		}
		var prev rune
		for _, r := range sp.Content {
			if !joins(prev, r) {
				if a := f.alpha(t, i, now); a != level {
					flush()
					level = a
				}
				i++
			}
			b.WriteRune(r)
			prev = r
		}
		flush()
	}
	return out
}

// faded is t fading in the characters it gains, when the view fades.
func (r *renderer) faded(place TextRef, t core.Text) core.Text {
	f := r.fade
	if f == nil {
		return t
	}
	ft := f.text(place)
	var spans func() []core.Span
	switch {
	case t.SpansFunc != nil:
		spans = t.SpansFunc
	case t.Spans != nil:
		static := pinned(t.Spans)
		spans = func() []core.Span { return resolvedSpans(static) }
	default:
		static := []core.Span{{Content: t.Content}}
		spans = func() []core.Span { return static }
	}
	t.Content, t.Spans = "", nil
	t.SpansFunc = func() []core.Span { return f.spans(ft, spans()) }
	return t
}

// spanUnits counts the characters of spans' content that fade alone.
func spanUnits(spans []core.Span) int {
	n := 0
	for _, sp := range spans {
		if sp.Attachment != nil {
			continue
		}
		var prev rune
		for _, r := range sp.Content {
			if !joins(prev, r) {
				n++
			}
			prev = r
		}
	}
	return n
}

// joins reports whether r belongs to the character before it, prev, as
// marks, joiners, variation selectors and emoji modifiers do, so the two
// fade as one.
func joins(prev, r rune) bool {
	switch {
	case prev == 0:
		return false
	case prev == '‍', r == '‍':
		return true
	case unicode.Is(unicode.M, r):
		return true
	case r >= 0xfe00 && r <= 0xfe0f, r >= 0xe0100 && r <= 0xe01ef, r >= 0xe0020 && r <= 0xe007f:
		return true
	case r >= 0x1f3fb && r <= 0x1f3ff:
		return true
	case isRegional(prev) && isRegional(r):
		return true
	}
	return false
}

func isRegional(r rune) bool { return r >= 0x1f1e6 && r <= 0x1f1ff }
