package android

import (
	"testing"
)

const (
	textOpPress      byte = 0
	textOpTouchless  byte = 1
	textOpCancel     byte = 2
	textOpNullLayout byte = 3
)

type textRunFixture struct {
	start, end int
	left       float32
	rtl        bool
}

type textLineFixture struct {
	start, end  int
	left        float32
	top, bottom int
	rtl         bool
	runs        []textRunFixture
}

type textIndexQuery struct {
	name string
	op   byte
	x, y float32
	want int
}

type textIndexCase struct {
	name      string
	text      string
	layoutX   int
	layoutY   int
	clipLines int
	width     int
	height    int
	lines     []textLineFixture
	queries   []textIndexQuery
}

// TestTextNodePressedIndex exercises TextNode.onTouchEvent and TextNode.pressedIndex on the JVM across LTR, multi-line, wrapped, clipped, RTL, mixed BiDi, emoji surrogate pairs, CJK UTF-8 offsets, layout offsets, misses, cancellations and touchless presses.
func TestTextNodePressedIndex(t *testing.T) {
	cases := []textIndexCase{
		{
			name:    "single-line LTR with layout offset",
			text:    "Read docs",
			layoutX: 12, layoutY: 8,
			width: 90, height: 20,
			lines: []textLineFixture{
				{start: 0, end: 9, left: 0, top: 0, bottom: 20, runs: []textRunFixture{{0, 9, 0, false}}},
			},
			queries: []textIndexQuery{
				{"touchless before any down", textOpTouchless, 0, 0, -1},
				{"left half of R", textOpPress, 12 + 2, 8 + 10, 0},
				{"right half of R", textOpPress, 12 + 8, 8 + 10, 0},
				{"second read consumes press", textOpTouchless, 0, 0, -1},
				{"left half of d in docs", textOpPress, 12 + 52, 8 + 10, 5},
				{"right half of s at end", textOpPress, 12 + 88, 8 + 10, 8},
				{"miss left of layout", textOpPress, 11, 8 + 10, -1},
				{"miss right of line", textOpPress, 12 + 90, 8 + 10, -1},
				{"miss above layout", textOpPress, 12 + 52, 7, -1},
				{"miss below layout", textOpPress, 12 + 52, 8 + 20, -1},
				{"cancelled touch clears press", textOpCancel, 12 + 52, 8 + 10, -1},
				{"null layout returns miss", textOpNullLayout, 12 + 52, 8 + 10, -1},
			},
		},
		{
			name:   "multi-line wrapped and newline-terminated LTR",
			text:   "hello world\n\nok",
			width:  60,
			height: 60,
			lines: []textLineFixture{
				// Wrapped line 0: "hello " (0..6)
				{start: 0, end: 6, left: 0, top: 0, bottom: 20, runs: []textRunFixture{{0, 6, 0, false}}},
				// Line 1: "world\n" (6..12, visible 6..11)
				{start: 6, end: 12, left: 0, top: 20, bottom: 40, runs: []textRunFixture{{6, 11, 0, false}}},
				// Empty line 2: "\n" (12..13, visible empty)
				{start: 12, end: 13, left: 0, top: 40, bottom: 50},
				// Line 3: "ok" (13..15)
				{start: 13, end: 15, left: 0, top: 50, bottom: 60, runs: []textRunFixture{{13, 15, 0, false}}},
			},
			queries: []textIndexQuery{
				{"right half of wrapped trailing space stays on line 0", textOpPress, 58, 10, 5},
				{"right half of o on line 0", textOpPress, 48, 10, 4},
				{"left half of w on line 1", textOpPress, 2, 30, 6},
				{"right half of d before newline", textOpPress, 48, 30, 10},
				{"past line 1 right", textOpPress, 55, 30, -1},
				{"empty line misses", textOpPress, 2, 45, -1},
				{"o on line 3", textOpPress, 5, 55, 13},
				{"k on line 3", textOpPress, 18, 55, 14},
			},
		},
		{
			name:      "clipped lines ignore hidden lines",
			text:      "aa\nbb",
			clipLines: 1,
			width:     20,
			height:    40,
			lines: []textLineFixture{
				{start: 0, end: 3, left: 0, top: 0, bottom: 20, runs: []textRunFixture{{0, 2, 0, false}}},
				{start: 3, end: 5, left: 0, top: 20, bottom: 40, runs: []textRunFixture{{3, 5, 0, false}}},
			},
			queries: []textIndexQuery{
				{"visible line 0 hits", textOpPress, 15, 10, 1},
				{"clipped line 1 misses", textOpPress, 5, 30, -1},
			},
		},
		{
			name:   "RTL Hebrew 2-byte UTF-8",
			text:   "שלום", // UTF-16 len 4; UTF-8 byte offsets: ש=0, ל=2, ו=4, ם=6
			width:  40,
			height: 20,
			lines: []textLineFixture{
				{start: 0, end: 4, left: 0, top: 0, bottom: 20, rtl: true, runs: []textRunFixture{{0, 4, 0, true}}},
			},
			queries: []textIndexQuery{
				{"right half of first char shin", textOpPress, 38, 10, 0},
				{"left half of first char shin", textOpPress, 32, 10, 0},
				{"right half of lamed", textOpPress, 28, 10, 2},
				{"left half of lamed", textOpPress, 22, 10, 2},
				{"right half of vav", textOpPress, 18, 10, 4},
				{"left half of final mem", textOpPress, 2, 10, 6},
				{"right half of final mem", textOpPress, 8, 10, 6},
				{"miss past right edge", textOpPress, 40, 10, -1},
				{"miss past left edge", textOpPress, -1, 10, -1},
			},
		},
		{
			name:   "mixed BiDi LTR paragraph with RTL span",
			text:   "ab של", // 'a'=0, 'b'=1, ' '=2, ש=3(byte 3), ל=4(byte 5)
			width:  50,
			height: 20,
			lines: []textLineFixture{
				{
					start: 0, end: 5, left: 0, top: 0, bottom: 20, rtl: false,
					runs: []textRunFixture{
						{0, 3, 0, false}, // "ab " at [0, 30)
						{3, 5, 30, true}, // "של" at [30, 50): ל at [30,40), ש at [40,50)
					},
				},
			},
			queries: []textIndexQuery{
				{"right half of space before RTL run", textOpPress, 28, 10, 2},
				{"left half of shin in RTL run", textOpPress, 42, 10, 3},
				{"right half of lamed in RTL run", textOpPress, 38, 10, 5},
				{"left half of lamed in RTL run", textOpPress, 32, 10, 5},
			},
		},
		{
			name:   "emoji surrogate pairs and CJK 3-byte UTF-8",
			text:   "hi 😀 你好", // h:0, i:1, ' ':2, 😀:3..4 (UTF-8 3..7), ' ':5 (UTF-8 7), 你:6 (UTF-8 8), 好:7 (UTF-8 11)
			width:  80,
			height: 20,
			lines: []textLineFixture{
				{start: 0, end: 8, left: 0, top: 0, bottom: 20, runs: []textRunFixture{{0, 8, 0, false}}},
			},
			queries: []textIndexQuery{
				{"left half of emoji", textOpPress, 34, 10, 3},
				{"right half of emoji", textOpPress, 46, 10, 3},
				{"space after emoji", textOpPress, 54, 10, 7},
				{"first CJK char", textOpPress, 64, 10, 8},
				{"second CJK char right half", textOpPress, 78, 10, 11},
			},
		},
		{
			name:   "emoji at end of line",
			text:   "a😀", // a:0, 😀:1..2 (UTF-8 byte 1)
			width:  30,
			height: 20,
			lines: []textLineFixture{
				{start: 0, end: 3, left: 0, top: 0, bottom: 20, runs: []textRunFixture{{0, 3, 0, false}}},
			},
			queries: []textIndexQuery{
				{"right half of trailing emoji", textOpPress, 28, 10, 1},
			},
		},
	}

	var input writer
	input.u32(uint32(len(cases)))
	for _, c := range cases {
		input.str(c.text)
		input.u32(uint32(c.layoutX))
		input.u32(uint32(c.layoutY))
		input.u32(uint32(c.clipLines))
		input.u32(uint32(c.width))
		input.u32(uint32(c.height))
		input.u32(uint32(len(c.lines)))
		for _, l := range c.lines {
			input.u32(uint32(l.start))
			input.u32(uint32(l.end))
			input.float(l.left)
			input.u32(uint32(l.top))
			input.u32(uint32(l.bottom))
			input.flags(l.rtl)
			input.u32(uint32(len(l.runs)))
			for _, run := range l.runs {
				input.u32(uint32(run.start))
				input.u32(uint32(run.end))
				input.float(run.left)
				input.flags(run.rtl)
			}
		}
		input.u32(uint32(len(c.queries)))
		for _, q := range c.queries {
			input.byte(q.op)
			input.floats(q.x, q.y)
		}
	}

	out := runJavaAlgorithm(t, "textindex", "TextIndexCheck", input.buf, "TextNode", "Protocol")
	r := reader{out}
	for _, c := range cases {
		for _, q := range c.queries {
			got := int(int32(r.u32()))
			if got != q.want {
				t.Errorf("%s / %s: pressedIndex() = %d, want %d", c.name, q.name, got, q.want)
			}
		}
	}
	if len(r.buf) != 0 {
		t.Errorf("Java wrote %d unexpected bytes", len(r.buf))
	}
}
