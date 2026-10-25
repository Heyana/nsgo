//go:build windows

package windows

import (
	"runtime"
	"unsafe"
)

// glyphOutlines turns a laid-out text's glyphs into path geometries, for
// canvases to stroke: DirectWrite draws the layout through outliner, an
// IDWriteTextRenderer of NSGo's, whose DrawGlyphRun asks each run's font
// face for its glyphs' outline.

var (
	iidDWriteTextRenderer = parseGUID("ef8a8135-5cc6-45fe-8825-c5a0724eb819")
	iidDWritePixelSnap    = parseGUID("eaf3a2da-ecf4-4d24-b644-b34f6842024b")
)

// outlineRun is a glyph run's outline, its origin x, y in the layout.
type outlineRun struct {
	geometry *comObject
	x, y     float32
}

// outliner is the renderer DirectWrite draws a layout through.
type outliner struct {
	vtbl     *[10]uintptr
	layout   *comObject
	factory  *comObject
	baseline float32
	runs     []outlineRun
}

// glyphRun is DWRITE_GLYPH_RUN.
type glyphRun struct {
	fontFace  *comObject
	emSize    float32
	count     uint32
	indices   uintptr
	advances  uintptr
	offsets   uintptr
	sideways  int32
	bidiLevel uint32
}

// glyphRunDescription is DWRITE_GLYPH_RUN_DESCRIPTION.
type glyphRunDescription struct {
	locale   uintptr
	text     uintptr
	length   uint32
	clusters uintptr
	position uint32
}

// glyphOutlines returns the outlines of layout's runs, on the line whose
// baseline is baseline below its top.
func glyphOutlines(factory, layout *comObject, baseline float32) []outlineRun {
	o := &outliner{vtbl: &outlinerVtbl, layout: layout, factory: factory, baseline: baseline}
	layout.callF(layoutDraw, 0b1100, 0, uintptr(unsafe.Pointer(o)), f32(0), f32(0))
	runtime.KeepAlive(o)
	return o.runs
}

func outlinerQueryInterface(o *outliner, iid *guid, out *unsafe.Pointer) uintptr {
	if *iid == iidUnknown || *iid == iidDWriteTextRenderer || *iid == iidDWritePixelSnap {
		*out = unsafe.Pointer(o)
		return sOK
	}
	*out = nil
	return eNoInterface
}

// The outliner lives as long as glyphOutlines, which outlasts the draw.
func outlinerAddRef(*outliner) uintptr  { return 1 }
func outlinerRelease(*outliner) uintptr { return 1 }

func outlinerSnapping(_ *outliner, _ uintptr, disabled *int32) uintptr {
	*disabled = 1
	return sOK
}

func outlinerTransform(_ *outliner, _ uintptr, m *matrix3x2) uintptr {
	*m = identity.d2d()
	return sOK
}

func outlinerPixelsPerDip(_ *outliner, _ uintptr, ppd *float32) uintptr {
	*ppd = 1
	return sOK
}

// glyphRun outlines a run. Callbacks cannot read the baseline origin,
// which DirectWrite passes as floats, so the run's x is where the layout
// places its first character and its y the line's baseline.
func (o *outliner) glyphRun(run *glyphRun, desc *glyphRunDescription) uintptr {
	if run == nil || run.count == 0 || run.fontFace == nil {
		return sOK
	}
	var x, y float32
	var metrics hitTestMetrics
	if desc != nil {
		o.layout.call(layoutHitTestTextPosition, uintptr(desc.position), 0, uintptr(unsafe.Pointer(&x)),
			uintptr(unsafe.Pointer(&y)), uintptr(unsafe.Pointer(&metrics)))
	}
	var geometry, sink *comObject
	if failed(o.factory.call(d2dFactoryCreatePathGeometry, uintptr(unsafe.Pointer(&geometry)))) {
		return sOK
	}
	if failed(geometry.call(pathOpen, uintptr(unsafe.Pointer(&sink)))) {
		geometry.release()
		return sOK
	}
	run.fontFace.callF(fontFaceGlyphOutline, 0b1, f32(run.emSize), run.indices, run.advances, run.offsets,
		uintptr(run.count), uintptr(run.sideways), uintptr(run.bidiLevel&1), sink.ptr())
	sink.call(sinkClose)
	sink.release()
	o.runs = append(o.runs, outlineRun{geometry: geometry, x: x, y: o.baseline})
	return sOK
}
