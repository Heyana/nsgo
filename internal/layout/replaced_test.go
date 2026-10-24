package layout

import "testing"

// Taffy's hand-written tests of replaced and compressible replaced boxes and
// of percentage grid tracks (tests/hand_written).

func allInsets(v float32) Rect[Length] { return Rect[Length]{Px(v), Px(v), Px(v), Px(v)} }

func layoutChild(container, child Style, content func(Size[Opt], Size[AvailableSpace]) Size[float32]) Layout {
	c := leafWith(child, content)
	root := newNode(container, c)
	computeLayout(root, maxContentSpaces)
	return c.layout()
}

func TestAbsoluteReplaced(t *testing.T) {
	// A 100x50 image.
	image := aspectContent(100, 0.5)
	container := Style{Display: DisplayBlock, Position: PositionRelative, Size: pxSize(400, 300)}

	l := layoutChild(container, Style{Position: PositionAbsolute, ItemIsReplaced: true, Inset: allInsets(10)}, image)
	if l.Size != (Size[float32]{100, 50}) || l.Location != (Point[float32]{10, 10}) {
		t.Errorf("auto size despite insets: size %v location %v", l.Size, l.Location)
	}
	l = layoutChild(container, Style{
		Position:       PositionAbsolute,
		ItemIsReplaced: true,
		Inset:          allInsets(10),
		Size:           Size[Length]{Width: Px(200)},
	}, image)
	if l.Size != (Size[float32]{200, 100}) {
		t.Errorf("auto height from aspect ratio: size %v", l.Size)
	}
	l = layoutChild(container, Style{Position: PositionAbsolute, Inset: allInsets(10)}, image)
	if l.Size != (Size[float32]{380, 280}) {
		t.Errorf("non-replaced stretched between insets: size %v", l.Size)
	}
}

func TestBlockReplaced(t *testing.T) {
	// A 142x20 badge.
	badge := aspectContent(142, 20.0/142.0)
	container := func(w float32) Style { return Style{Display: DisplayBlock, Size: Size[Length]{Width: Px(w)}} }

	l := layoutChild(container(600), Style{Display: DisplayBlock, ItemIsReplaced: true}, badge)
	if l.Size != (Size[float32]{142, 20}) {
		t.Errorf("auto width uses intrinsic size: size %v", l.Size)
	}
	l = layoutChild(container(600), Style{Display: DisplayBlock}, badge)
	if l.Size.Width != 600 {
		t.Errorf("non-replaced child is stretched: width %v", l.Size.Width)
	}
	l = layoutChild(container(600), Style{Display: DisplayBlock, ItemIsReplaced: true, Size: pxSize(300, 50)}, badge)
	if l.Size != (Size[float32]{300, 50}) {
		t.Errorf("explicit width: size %v", l.Size)
	}
	l = layoutChild(container(100), Style{
		Display:        DisplayBlock,
		ItemIsReplaced: true,
		MaxSize:        Size[Length]{Width: Percent(1)},
	}, badge)
	if l.Size.Width != 100 {
		t.Errorf("max-width clamps intrinsic size: width %v", l.Size.Width)
	}
	l = layoutChild(container(600), Style{
		Display:        DisplayBlock,
		ItemIsReplaced: true,
		Margin:         Rect[Length]{Left: Auto, Right: Auto},
	}, badge)
	if l.Size.Width != 142 || l.Location.X != (600-142)/2 {
		t.Errorf("auto margins center: width %v x %v", l.Size.Width, l.Location.X)
	}
}

func TestCompressibleReplaced(t *testing.T) {
	// A 150x20 text input.
	input := aspectContent(150, 20.0/150.0)
	block := Style{Display: DisplayBlock, Size: Size[Length]{Width: Px(400)}}
	positioned := Style{Display: DisplayBlock, Position: PositionRelative, Size: pxSize(400, 300)}

	l := layoutChild(block, Style{Display: DisplayBlock, ItemIsCompressibleReplaced: true}, input)
	if l.Size != (Size[float32]{150, 20}) {
		t.Errorf("block-level auto width uses intrinsic size: size %v", l.Size)
	}
	l = layoutChild(block, Style{
		Display:                    DisplayBlock,
		ItemIsCompressibleReplaced: true,
		JustifySelf:                alignOf(AlignStretch),
	}, input)
	if l.Size.Width != 400 {
		t.Errorf("justify-self stretch stretches: width %v", l.Size.Width)
	}
	l = layoutChild(positioned, Style{
		Position:                   PositionAbsolute,
		ItemIsCompressibleReplaced: true,
		Inset:                      allInsets(10),
	}, input)
	if l.Size != (Size[float32]{380, 280}) || l.Location != (Point[float32]{10, 10}) {
		t.Errorf("absolute auto size stretched between insets: size %v location %v", l.Size, l.Location)
	}

	s := Style{ItemIsReplaced: true}
	if !s.isCompressibleReplaced() || !s.isReplaced() {
		t.Error("replaced must imply compressible replaced")
	}
	s = Style{ItemIsCompressibleReplaced: true}
	if !s.isCompressibleReplaced() || s.isReplaced() {
		t.Error("compressible replaced must not imply replaced")
	}
}

// TestGridPercentageRerun checks that percentage tracks of a grid container
// with an indefinite size are treated as auto while sizing the container,
// then resolved against its size, whatever the available space.
func TestGridPercentageRerun(t *testing.T) {
	for _, available := range []AvailableSpace{MaxContentSpace, Definite(200)} {
		item := leafWith(Style{}, textContent("HH​HH​HH"))
		root := newNode(Style{
			Display:             DisplayGrid,
			GridTemplateColumns: []GridTemplateComponent{{Track: Track(FitContentPercent(0.5))}},
			GridTemplateRows:    []GridTemplateComponent{{Track: Track(Px(40))}},
		}, item)
		computeLayout(root, Size[AvailableSpace]{available, MaxContentSpace})
		if rw, iw := root.layout().Size.Width, item.layout().Size.Width; rw != 60 || iw != 30 {
			t.Errorf("fit-content(50%%) in %v: container %v item %v, want 60 and 30", available, rw, iw)
		}
	}

	// Mirrors WPT css/css-grid/layout-algorithm/grid-percent-rows-filled-shrinkwrap-001.html:
	// percentage rows are re-resolved against the container's first-pass
	// height even when columns are not re-sized, and the auto rows stretch.
	for _, available := range []AvailableSpace{MaxContentSpace, Definite(500)} {
		a := newLeaf(Style{Size: pxSize(20, 20)})
		b := newLeaf(Style{Size: pxSize(20, 60)})
		c := newLeaf(Style{Size: pxSize(20, 20)})
		root := newNode(Style{
			Display:             DisplayGrid,
			GridTemplateColumns: []GridTemplateComponent{{Track: Track(Auto)}},
			GridTemplateRows: []GridTemplateComponent{
				{Track: Track(Auto)}, {Track: Track(Percent(0.2))}, {Track: Track(Auto)},
			},
		}, a, b, c)
		computeLayout(root, Size[AvailableSpace]{Definite(200), available})
		total := root.layout().Size.Height
		yb, yc := b.layout().Location.Y, c.layout().Location.Y
		got := [4]float32{total, yb, yc - yb, total - yc}
		if got != [4]float32{100, 40, 20, 40} {
			t.Errorf("percentage rows in %v: %v, want [100 40 20 40]", available, got)
		}
	}
}
