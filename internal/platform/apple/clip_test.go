//go:build darwin

package apple

import (
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestNeedsClipHolder(t *testing.T) {
	plain := driver.ViewStyle{Clip: true}
	if NeedsClipHolder(&plain) {
		t.Fatal("plain clipping should stay on the styled layer")
	}
	if NeedsClipHolder(&driver.ViewStyle{ShadowColor: driver.RGB(0x112233)}) {
		t.Fatal("a shadow without clipping needs no holder")
	}
	if !NeedsClipHolder(&driver.ViewStyle{Clip: true, ShadowColor: driver.RGB(0x112233)}) {
		t.Fatal("a clipped native shadow needs a holder")
	}
	if !NeedsClipHolder(&driver.ViewStyle{Clip: true, Outline: driver.Outline{Width: 1, Color: driver.RGB(0x112233)}}) {
		t.Fatal("a clipped outline needs a holder")
	}
	if NeedsClipHolder(&driver.ViewStyle{Clip: true, Effects: &driver.Effects{Shadows: []driver.BoxShadow{{Inset: true, Color: driver.RGB(0x112233)}}}}) {
		t.Fatal("an inset shadow stays inside the clip")
	}
	if !NeedsClipHolder(&driver.ViewStyle{Clip: true, Effects: &driver.Effects{Shadows: []driver.BoxShadow{{Color: driver.RGB(0x112233)}}}}) {
		t.Fatal("an outer box shadow needs a holder")
	}
}

func TestShapedCorners(t *testing.T) {
	for _, c := range []struct {
		name string
		vs   driver.ViewStyle
		want bool
	}{
		{"square", driver.ViewStyle{}, false},
		{"one radius", driver.ViewStyle{CornerRadius: 12}, false},
		{"the top corners only", driver.ViewStyle{Corners: driver.Corners{TopLeft: 12, TopRight: 12}}, false},
		{"radii that differ", driver.ViewStyle{Corners: driver.Corners{TopLeft: 16, TopRight: 4, BottomRight: 4, BottomLeft: 4}}, true},
	} {
		if got := ShapedCorners(&c.vs); got != c.want {
			t.Errorf("%s: ShapedCorners = %v, want %v", c.name, got, c.want)
		}
	}
}

// A holder clips to the view's corners: its own corner radius for one radius, a shape mask for radii that differ, which gives way to an effect's mask and comes back with HolderMask, and nothing once the view stops clipping in it.
func TestClipHolderClipsToTheCorners(t *testing.T) {
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer.Send(selRelease)
	defer ForgetHolder(layer)
	layer.Send(sel("setBounds:"), CGRect{Size: CGSize{Width: 200, Height: 120}})
	mask := func() objc.ID { return layer.Send(sel("mask")) }
	clips := func() bool { return objc.Send[bool](layer, sel("masksToBounds")) }
	radius := func() float64 { return objc.Send[float64](layer, sel("cornerRadius")) }

	ClipHolder(layer, &driver.ViewStyle{Clip: true, CornerRadius: 16}, false)
	if !clips() || radius() != 16 || mask() != 0 {
		t.Fatalf("one radius: masksToBounds %v, cornerRadius %v, mask %v; want true, 16, none", clips(), radius(), mask())
	}

	shaped := &driver.ViewStyle{Clip: true, Corners: driver.Corners{TopLeft: 16, TopRight: 4, BottomRight: 4, BottomLeft: 4}}
	ClipHolder(layer, shaped, false)
	m := HolderMask(layer)
	if m == 0 || mask() != m || clips() || radius() != 0 {
		t.Fatalf("radii that differ: mask %v (kept %v), masksToBounds %v, cornerRadius %v; want the kept mask, false, 0", mask(), m, clips(), radius())
	}
	if f := objc.Send[CGRect](m, sel("frame")); f.Size != (CGSize{Width: 200, Height: 120}) {
		t.Fatalf("mask frame %v, want the holder's bounds", f)
	}

	// An effect hides the children with a mask of its own; laying the holder out again leaves it.
	hide := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer hide.Send(selRelease)
	layer.Send(selSetMask, hide)
	ClipHolder(layer, shaped, false)
	if mask() != hide {
		t.Fatal("the holder's clip replaced an effect's mask")
	}
	layer.Send(selSetMask, HolderMask(layer))
	if mask() != m {
		t.Fatal("the holder's clip did not come back")
	}

	ClipHolder(layer, nil, false)
	if mask() != 0 || HolderMask(layer) != 0 || clips() || radius() != 0 {
		t.Fatalf("unclipped: mask %v, masksToBounds %v, cornerRadius %v; want none, false, 0", mask(), clips(), radius())
	}
	ClipHolder(layer, nil, true)
	if !clips() {
		t.Fatal("a scroll view's container stopped clipping to its bounds")
	}
}

// A holder gives the children the view's perspective, which the view's own sublayer transform gives only the flat holder, and leaves a scroll view's container's sublayer transform, which scrolls its children, alone.
func TestClipHolderTakesThePerspective(t *testing.T) {
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer.Send(selRelease)
	defer ForgetHolder(layer)
	layer.Send(sel("setBounds:"), CGRect{Size: CGSize{Width: 200, Height: 120}})
	depth := func() float64 { return objc.Send[transform3D](layer, sel("sublayerTransform")).M34 }

	ClipHolder(layer, &driver.ViewStyle{Clip: true, CornerRadius: 16, ChildPerspective: 500}, false)
	if got := depth(); got != -1.0/500 {
		t.Fatalf("held children's perspective m34 = %v, want %v", got, -1.0/500)
	}
	// A holder that stopped clipping, as its view lost its shadow, still gives the children the view's perspective.
	ClipHolder(layer, &driver.ViewStyle{ChildPerspective: 500}, false)
	if got := depth(); got != -1.0/500 {
		t.Fatalf("unclipped children's perspective m34 = %v, want %v", got, -1.0/500)
	}
	if objc.Send[bool](layer, sel("masksToBounds")) {
		t.Fatal("a holder that does not clip still masks to its bounds")
	}
	ClipHolder(layer, nil, false)
	if got := depth(); got != 0 {
		t.Fatalf("children's perspective with no style m34 = %v, want none", got)
	}
	SetSublayerTranslation(layer, 0, -40)
	ClipHolder(layer, &driver.ViewStyle{Clip: true, CornerRadius: 16}, true)
	if ty := objc.Send[transform3D](layer, sel("sublayerTransform")).M42; ty != -40 {
		t.Fatalf("a scroll view's container's translation became %v, want -40", ty)
	}
}

// An image clips its own layer to uniform or differing corner radii, keeps the mask sized after a resize, and releases it when the image closes.
func TestImageClipsToItsRoundedCorners(t *testing.T) {
	host := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer host.Send(selRelease)
	iv := NewImageView(host)
	defer iv.Close()
	radius := func() float64 { return objc.Send[float64](iv.layer, sel("cornerRadius")) }
	corners := func() uint { return objc.Send[uint](iv.layer, sel("maskedCorners")) }

	iv.SetStyle(&driver.ViewStyle{Corners: driver.Corners{TopLeft: 12, TopRight: 12}})
	iv.Layout(CGSize{Width: 200, Height: 120})
	if radius() != 12 || corners() != 1|2 {
		t.Fatalf("the top corners: cornerRadius %v, maskedCorners %b; want 12 on the top two", radius(), corners())
	}
	iv.SetStyle(&driver.ViewStyle{CornerRadius: 8})
	if radius() != 8 || corners() != 1|2|4|8 {
		t.Fatalf("one radius: cornerRadius %v, maskedCorners %b; want 8 on all four", radius(), corners())
	}
	iv.SetStyle(&driver.ViewStyle{Corners: driver.Corners{TopLeft: 16, TopRight: 4, BottomRight: 4, BottomLeft: 4}})
	if radius() != 0 || iv.layer.Send(sel("mask")) == 0 {
		t.Fatalf("radii that differ: cornerRadius %v, mask %v; want a shaped image-layer clip", radius(), iv.layer.Send(sel("mask")))
	}
	iv.Layout(CGSize{Width: 80, Height: 60})
	m := iv.layer.Send(sel("mask"))
	if got := objc.Send[CGRect](m, sel("frame")); got != Rect(0, 0, 80, 60) {
		t.Errorf("resized image mask: %v, want current image bounds", got)
	}
	iv.SetStyle(&driver.ViewStyle{CornerRadius: 8})
	if iv.layer.Send(sel("mask")) != 0 || radius() != 8 {
		t.Error("a uniform radius did not replace the shape mask")
	}
	iv.SetStyle(&driver.ViewStyle{Corners: driver.Corners{TopLeft: 16, BottomRight: 8}})
	iv.Close()
	if HolderMask(iv.layer) != 0 {
		t.Error("closing the image retained its corner mask")
	}
}
