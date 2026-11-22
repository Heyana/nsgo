package miniprogram

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// approx measures every character 0.5 of the font size wide, CJK ones
// the whole size, from the size in the canvas font.
type approx struct{ calls int }

func (a *approx) Width(font, text string) float64 {
	a.calls++
	size := 14.0
	for _, f := range strings.Fields(font) {
		if v, ok := strings.CutSuffix(f, "px"); ok {
			size = 0
			for _, c := range v {
				size = size*10 + float64(c-'0')
			}
		}
	}
	w := 0.0
	for _, r := range text {
		if cjk(r) {
			w += size
		} else {
			w += size / 2
		}
	}
	return w
}

func plain(s string) *driver.AttributedText {
	return &driver.AttributedText{Text: s, Style: driver.TextStyle{Font: driver.Font{Size: 10, Weight: 400}, Color: driver.RGB(0),
		Align: driver.TextAlignStart}}
}

func runs(l driver.TextLayout) []string {
	var out []string
	for _, r := range l.(*TextLayout).runs {
		out = append(out, r.text)
	}
	return out
}

func TestBreaksAfterSpacesAndAroundCJK(t *testing.T) {
	got := func(s string) []string {
		var out []string
		for _, b := range breaks(s) {
			out = append(out, s[b[0]:b[1]])
		}
		return out
	}
	if g := got("hello  world"); strings.Join(g, "|") != "hello  |world" {
		t.Errorf("breaks = %q", g)
	}
	if g := got("你好abc"); strings.Join(g, "|") != "你|好|abc" {
		t.Errorf("breaks = %q", g)
	}
}

func TestLinesWrapGreedilyAndMeasureTheirWidest(t *testing.T) {
	e := NewTextEngine(&approx{})
	p := e.Prepare(plain("hello world"))
	// 5 points a character: "hello" is 25, "hello world" 55.
	if s := p.Measure(driver.Auto); s.Width != 55 || s.Height != 12 {
		t.Fatalf("one line = %+v", s)
	}
	if s := p.Measure(40); s.Width != 25 || s.Height != 24 {
		t.Fatalf("wrapped = %+v", s)
	}
	l := p.Layout(driver.Size{Width: 40, Height: 24})
	if g := runs(l); strings.Join(g, "|") != "hello|world" {
		t.Fatalf("runs = %q", g)
	}
	lines := l.(*TextLayout).TextLines()
	if len(lines) != 2 || lines[1].Bounds.Origin.Y != 12 || lines[1].Start != 6 {
		t.Fatalf("lines = %+v", lines)
	}
}

func TestLongWordsAndCJKBreakBetweenCharacters(t *testing.T) {
	e := NewTextEngine(&approx{})
	l := e.Prepare(plain("你好世界")).Layout(driver.Size{Width: 25, Height: 100})
	if g := runs(l); strings.Join(g, "|") != "你好|世界" {
		t.Fatalf("CJK runs = %q", g)
	}
	l = e.Prepare(plain("abcdefgh")).Layout(driver.Size{Width: 20, Height: 100})
	if g := runs(l); strings.Join(g, "|") != "abcd|efgh" {
		t.Fatalf("long word runs = %q", g)
	}
}

func TestLineLimitEllipsizes(t *testing.T) {
	e := NewTextEngine(&approx{})
	at := plain("one two three four")
	at.MaxLines, at.Ellipsize = 1, driver.EllipsizeTail
	p := e.Prepare(at)
	if s := p.Measure(40); s.Height != 12 {
		t.Fatalf("limited = %+v", s)
	}
	g := strings.Join(runs(p.Layout(driver.Size{Width: 40, Height: 12})), "")
	if !strings.HasSuffix(g, "…") || utf8.RuneCountInString(g)*5 > 40 {
		t.Fatalf("ellipsized = %q", g)
	}
	at.Ellipsize = driver.EllipsizeMiddle
	g = strings.Join(runs(e.Prepare(at).Layout(driver.Size{Width: 40, Height: 12})), "")
	if !strings.Contains(g, "…") || strings.HasSuffix(g, "…") || strings.HasPrefix(g, "…") {
		t.Fatalf("middle = %q", g)
	}
}

func TestSpansKeepTheirStylesOnOneBaseline(t *testing.T) {
	e := NewTextEngine(&approx{})
	at := plain("Hi, ￼Yo")
	big := at.Style
	big.Font.Size, big.Color = 20, driver.RGB(0xff0000)
	at.Spans = []driver.TextSpan{{Start: 7, End: 9, Style: big}}
	at.Attachments = []driver.TextAttachment{{Offset: 4, Size: driver.Size{Width: 10, Height: 10}}}
	l := e.Prepare(at).Layout(driver.Size{Width: 500, Height: 100}).(*TextLayout)
	if len(l.runs) != 2 || l.runs[0].text != "Hi, " || l.runs[1].text != "Yo" || l.runs[1].style != 1 {
		t.Fatalf("runs = %+v", l.runs)
	}
	if !strings.Contains(l.styles[1], "font-size:20px") || !strings.Contains(l.styles[1], "rgba(255,0,0,1)") {
		t.Fatalf("span style = %q", l.styles[1])
	}
	// Each baseline is 0.3 em below the middle of its run's line box.
	b0 := l.runs[0].y + l.runs[0].height/2 + 3
	b1 := l.runs[1].y + l.runs[1].height/2 + 6
	if d := b0 - b1; d > 0.01 || d < -0.01 {
		t.Fatalf("baselines %v and %v", b0, b1)
	}
	box, ok := l.Attachment(0)
	if !ok || box.Origin.X != 20 || box.Origin.Y+10 != b0 {
		t.Fatalf("attachment at %+v (baseline %v)", box, b0)
	}
}

func TestWidthsAreMeasuredOnce(t *testing.T) {
	m := &approx{}
	e := NewTextEngine(m)
	e.Prepare(plain("same words same words"))
	n := m.calls
	e.Prepare(plain("same words same words"))
	if m.calls != n {
		t.Fatalf("measured %d times again", m.calls-n)
	}
}

func TestAlignment(t *testing.T) {
	e := NewTextEngine(&approx{})
	at := plain("ab")
	at.Style.Align = driver.TextAlignCenter
	l := e.Prepare(at).Layout(driver.Size{Width: 30, Height: 12}).(*TextLayout)
	if l.runs[0].x != 10 {
		t.Fatalf("centered at %v", l.runs[0].x)
	}
	at.Style.Align, at.Style.RTL = driver.TextAlignStart, true
	l = e.Prepare(at).Layout(driver.Size{Width: 30, Height: 12}).(*TextLayout)
	if l.runs[0].x != 20 {
		t.Fatalf("right to left at %v", l.runs[0].x)
	}
}

func TestCaretsHitTheCharacters(t *testing.T) {
	e := NewTextEngine(&approx{})
	l := e.Prepare(plain("ab cd")).Layout(driver.Size{Width: 15, Height: 24}).(*TextLayout)
	if c := l.HitTest(driver.Point{X: 6, Y: 18}); c.Offset != 4 {
		t.Fatalf("hit = %+v", c)
	}
}

func TestLimitedTextTruncatesRemainingTextAndMatchesMeasure(t *testing.T) {
	e := NewTextEngine(&approx{})
	// 5 pts per character at size 10, line height 12.
	for _, tc := range []struct {
		name      string
		text      string
		maxLines  int
		width     driver.Points
		wantRuns  string
		wantWidth driver.Points
	}{
		{
			name:      "single line pulls remaining word before ellipsizing",
			text:      "Earth Supercalifragilistic",
			maxLines:  1,
			width:     72,
			wantRuns:  "Earth Superca…",
			wantWidth: 70,
		},
		{
			name:      "two lines pulls remaining word onto second line",
			text:      "Short line\nA Supercalifragilistic",
			maxLines:  2,
			width:     72,
			wantRuns:  "Short line|A Supercalifr…",
			wantWidth: 70,
		},
		{
			name:      "two lines wraps words then truncates remaining text",
			text:      "one two three four five",
			maxLines:  2,
			width:     40,
			wantRuns:  "one two|three f…",
			wantWidth: 40,
		},
		{
			name:      "trailing spaces before truncation are trimmed",
			text:      "a   bcdefgh",
			maxLines:  1,
			width:     20,
			wantRuns:  "a…",
			wantWidth: 10,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := plain(tc.text)
			at.MaxLines, at.Ellipsize = tc.maxLines, driver.EllipsizeTail
			p := e.Prepare(at)
			size := p.Measure(tc.width)
			if size.Width != tc.wantWidth || size.Height != driver.Points(tc.maxLines*12) {
				t.Fatalf("Measure(%v) = %+v, want width %v height %d", tc.width, size, tc.wantWidth, tc.maxLines*12)
			}
			fitted := p.Layout(size).(*TextLayout)
			fixed := p.Layout(driver.Size{Width: tc.width, Height: size.Height}).(*TextLayout)
			if got := strings.Join(runs(fitted), "|"); got != tc.wantRuns {
				t.Errorf("fitted runs = %q, want %q", got, tc.wantRuns)
			}
			if got := strings.Join(runs(fixed), "|"); got != tc.wantRuns {
				t.Errorf("fixed runs = %q, want %q", got, tc.wantRuns)
			}
			gl, wl := fitted.TextLines(), fixed.TextLines()
			if len(gl) != tc.maxLines || len(wl) != tc.maxLines {
				t.Fatalf("line counts %d / %d, want %d", len(gl), len(wl), tc.maxLines)
			}
			if gl[tc.maxLines-1].Bounds.Size.Width != wl[tc.maxLines-1].Bounds.Size.Width {
				t.Errorf("last line width at measured box = %v, at offered width = %v",
					gl[tc.maxLines-1].Bounds.Size.Width, wl[tc.maxLines-1].Bounds.Size.Width)
			}
		})
	}
}

func TestLimitedTextKeepsWidthAtHardBreak(t *testing.T) {
	e := NewTextEngine(&approx{})
	for _, tc := range []struct {
		name      string
		text      string
		maxLines  int
		mode      driver.EllipsizeMode
		wantRuns  string
		wantWidth driver.Points
	}{
		{"tail 1 line", "Short\nline", 1, driver.EllipsizeTail, "Short…", 30},
		{"clip 1 line", "Short\nline", 1, driver.EllipsizeClip, "Short", 25},
		{"head 1 line", "Short\nline", 1, driver.EllipsizeHead, "…line", 25},
		{"tail 2 lines", "Short\nSecond\nThird", 2, driver.EllipsizeTail, "Short|Second…", 35},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := plain(tc.text)
			at.MaxLines, at.Ellipsize = tc.maxLines, tc.mode
			p := e.Prepare(at)
			bounded, intrinsic := p.Measure(200), p.Measure(driver.Auto)
			if bounded != intrinsic {
				t.Fatalf("Measure(200) = %+v != Measure(Auto) = %+v", bounded, intrinsic)
			}
			if bounded.Width != tc.wantWidth || bounded.Height != driver.Points(tc.maxLines*12) {
				t.Fatalf("Measure = %+v, want width %v height %d", bounded, tc.wantWidth, tc.maxLines*12)
			}
			if got := strings.Join(runs(p.Layout(bounded)), "|"); got != tc.wantRuns {
				t.Errorf("runs = %q, want %q", got, tc.wantRuns)
			}
		})
	}
}

func TestLimitedTextHeadAndMiddleMultiLine(t *testing.T) {
	e := NewTextEngine(&approx{})
	for _, tc := range []struct {
		name     string
		mode     driver.EllipsizeMode
		maxLines int
		text     string
		width    driver.Points
		wantRuns string
	}{
		{"1 line head", driver.EllipsizeHead, 1, "Earth Supercalifragilistic", 72, "…ifragilistic"},
		{"1 line middle", driver.EllipsizeMiddle, 1, "Earth Science Weekly", 72, "Earth S…Weekly"},
		{"2 lines middle", driver.EllipsizeMiddle, 2, "Short line\nA Supercalifragilistic", 72, "Short line|A Super…listic"},
		{"2 lines head", driver.EllipsizeHead, 2, "Short line\nA Supercalifragilistic", 72, "Short line|…ragilistic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := plain(tc.text)
			at.MaxLines, at.Ellipsize = tc.maxLines, tc.mode
			p := e.Prepare(at)
			size := p.Measure(tc.width)
			if size.Width > tc.width || size.Height != driver.Points(tc.maxLines*12) {
				t.Fatalf("Measure(%v) = %+v", tc.width, size)
			}
			fitted := p.Layout(size).(*TextLayout)
			fixed := p.Layout(driver.Size{Width: tc.width, Height: size.Height}).(*TextLayout)
			if got := strings.Join(runs(fitted), "|"); got != tc.wantRuns {
				t.Errorf("fitted runs = %q, want %q", got, tc.wantRuns)
			}
			if got := strings.Join(runs(fixed), "|"); got != tc.wantRuns {
				t.Errorf("fixed runs = %q, want %q", got, tc.wantRuns)
			}
		})
	}

	// Attachments kept after middle truncation keep their original indices.
	att := string(driver.AttachmentRune)
	at := plain(att + " middletext " + att)
	at.MaxLines, at.Ellipsize = 1, driver.EllipsizeMiddle
	at.Attachments = []driver.TextAttachment{
		{Offset: 0, Size: driver.Size{Width: 10, Height: 10}},
		{Offset: len(at.Text) - len(att), Size: driver.Size{Width: 10, Height: 10}},
	}
	p := e.Prepare(at)
	size := p.Measure(40)
	l := p.Layout(size).(*TextLayout)
	if _, ok := l.Attachment(0); !ok {
		t.Error("first attachment was dropped")
	}
	if _, ok := l.Attachment(1); !ok {
		t.Error("second attachment lost its original index after middle cut")
	}
}

func TestLimitedTextAlignsEllipsis(t *testing.T) {
	e := NewTextEngine(&approx{})
	const width driver.Points = 72
	for _, tc := range []struct {
		name  string
		align driver.TextAlign
		rtl   bool
		wantX driver.Points
	}{
		{"center", driver.TextAlignCenter, false, 1},
		{"end", driver.TextAlignEnd, false, 2},
		{"RTL start", driver.TextAlignStart, true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := plain("Earth Supercalifragilistic")
			at.MaxLines, at.Ellipsize = 1, driver.EllipsizeTail
			at.Style.Align, at.Style.RTL = tc.align, tc.rtl
			l := e.Prepare(at).Layout(driver.Size{Width: width, Height: 12}).(*TextLayout)
			if len(l.lines) != 1 {
				t.Fatalf("got %d lines, want 1", len(l.lines))
			}
			// "Earth Superca…" is 70 pts wide inside 72 pts -> center x=1, end x=2.
			if got := l.lines[0].Bounds.Origin.X; got != tc.wantX {
				t.Errorf("x = %v, want %v (line width %v)", got, tc.wantX, l.lines[0].Bounds.Size.Width)
			}
		})
	}
}

func TestLimitedTextSpansAndAttachments(t *testing.T) {
	e := NewTextEngine(&approx{})
	att := string(driver.AttachmentRune)
	// "Hi " (15) + att0 (10) + " " (5) + "Super" (25) + att1 (10) = 65 pts.
	at := plain("Hi " + att + " Super" + att)
	at.MaxLines, at.Ellipsize = 1, driver.EllipsizeTail
	red := at.Style
	red.Color = driver.RGB(0xff0000)
	superStart := len("Hi " + att + " ")
	at.Spans = []driver.TextSpan{{Start: superStart, End: superStart + len("Super"), Style: red}}
	at.Attachments = []driver.TextAttachment{
		{Offset: len("Hi "), Size: driver.Size{Width: 10, Height: 10}},
		{Offset: superStart + len("Super"), Size: driver.Size{Width: 10, Height: 10}},
	}
	p := e.Prepare(at)
	// Width 45 fits "Hi "(15) + att0(10) + " "(5) + "Su"(10) + "…"(5) = 45;
	// height is 13 because the 10pt baseline-aligned attachment reaches 1pt above the 9pt ascent.
	size := p.Measure(45)
	if size.Width != 45 || size.Height != 13 {
		t.Fatalf("Measure(45) = %+v, want {Width:45 Height:13}", size)
	}
	l := p.Layout(size).(*TextLayout)
	if _, ok := l.Attachment(0); !ok {
		t.Error("first attachment should stay visible")
	}
	if _, ok := l.Attachment(1); ok {
		t.Error("second attachment past the truncation point should be hidden")
	}
	if got := strings.Join(runs(l), "|"); got != "Hi | |Su…" {
		t.Errorf("runs = %q, want %q", got, "Hi | |Su…")
	}
}
