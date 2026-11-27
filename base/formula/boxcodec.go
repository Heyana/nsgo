package formula

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/caelyreth/untie"
	"github.com/caelyreth/untie/display"
	"github.com/caelyreth/untie/geometry"
	"github.com/caelyreth/untie/identity"
	"github.com/caelyreth/untie/source"
)

// textMeasure is one measured system text run's geometry without importing core.
type textMeasure struct {
	width   float32
	ascent  float32
	descent float32
}

// Binary codecs for transferring Untie layout requests and display boxes across
// a browser Web Worker boundary without reflection or loss of geometry, hit
// testing, caret stops, or font fallback metadata.

type binWriter struct {
	b []byte
}

func (w *binWriter) u8(v uint8) { w.b = append(w.b, v) }

func (w *binWriter) bool(v bool) {
	if v {
		w.b = append(w.b, 1)
	} else {
		w.b = append(w.b, 0)
	}
}

func (w *binWriter) u16(v uint16) { w.b = binary.LittleEndian.AppendUint16(w.b, v) }
func (w *binWriter) u32(v uint32) { w.b = binary.LittleEndian.AppendUint32(w.b, v) }
func (w *binWriter) i32(v int)    { w.u32(uint32(int32(v))) }
func (w *binWriter) u64(v uint64) { w.b = binary.LittleEndian.AppendUint64(w.b, v) }
func (w *binWriter) f32(v float32) {
	w.u32(math.Float32bits(v))
}

func (w *binWriter) str(s string) {
	w.u32(uint32(len(s)))
	w.b = append(w.b, s...)
}

func (w *binWriter) point(p geometry.Point) {
	w.f32(p.X)
	w.f32(p.Y)
}

func (w *binWriter) rect(r geometry.Rect) {
	w.f32(r.X)
	w.f32(r.Y)
	w.f32(r.Width)
	w.f32(r.Height)
}

func (w *binWriter) srcRange(r source.Range) {
	w.i32(r.Start)
	w.i32(r.End)
}

type binReader struct {
	b   []byte
	pos int
	err error
}

func (r *binReader) need(n int) bool {
	if r.err != nil {
		return false
	}
	if n < 0 || n > len(r.b)-r.pos {
		r.err = errors.New("formula: truncated wire message")
		return false
	}
	return true
}

func (r *binReader) u8() uint8 {
	if !r.need(1) {
		return 0
	}
	v := r.b[r.pos]
	r.pos++
	return v
}

func (r *binReader) bool() bool { return r.u8() != 0 }

func (r *binReader) u16() uint16 {
	if !r.need(2) {
		return 0
	}
	v := binary.LittleEndian.Uint16(r.b[r.pos : r.pos+2])
	r.pos += 2
	return v
}

func (r *binReader) u32() uint32 {
	if !r.need(4) {
		return 0
	}
	v := binary.LittleEndian.Uint32(r.b[r.pos : r.pos+4])
	r.pos += 4
	return v
}

func (r *binReader) i32() int { return int(int32(r.u32())) }

func (r *binReader) u64() uint64 {
	if !r.need(8) {
		return 0
	}
	v := binary.LittleEndian.Uint64(r.b[r.pos : r.pos+8])
	r.pos += 8
	return v
}

func (r *binReader) f32() float32 { return math.Float32frombits(r.u32()) }

func (r *binReader) str() string {
	n := int(r.u32())
	if !r.need(n) {
		return ""
	}
	s := string(r.b[r.pos : r.pos+n])
	r.pos += n
	return s
}

func (r *binReader) point() geometry.Point {
	return geometry.Point{X: r.f32(), Y: r.f32()}
}

func (r *binReader) rect() geometry.Rect {
	return geometry.Rect{X: r.f32(), Y: r.f32(), Width: r.f32(), Height: r.f32()}
}

func (r *binReader) srcRange() source.Range {
	return source.Range{Start: r.i32(), End: r.i32()}
}

type layoutRequest struct {
	source  string
	dialect untie.Dialect
	mode    untie.Mode
	style   untie.Style
	opts    untie.LayoutOptions
}

func encodeLayoutRequest(req layoutRequest) []byte {
	var w binWriter
	w.str(req.source)
	w.u8(uint8(req.dialect))
	w.u8(uint8(req.mode))
	w.f32(req.style.Size)
	w.f32(req.style.MaxWidth)
	w.u8(uint8(req.style.Align))
	w.i32(req.style.ScriptLevel)
	w.u32(uint32(len(req.style.Features)))
	for _, f := range req.style.Features {
		w.str(f)
	}
	w.bool(req.style.GlyphRefs)
	w.bool(req.style.GenerateAlt)
	w.u8(uint8(req.opts.Recovery))
	return w.b
}

func decodeLayoutRequest(data []byte) (layoutRequest, error) {
	r := binReader{b: data}
	req := layoutRequest{
		source:  r.str(),
		dialect: untie.Dialect(r.u8()),
		mode:    untie.Mode(r.u8()),
		style: untie.Style{
			Size:        r.f32(),
			MaxWidth:    r.f32(),
			Align:       untie.Align(r.u8()),
			ScriptLevel: r.i32(),
		},
	}
	if n := int(r.u32()); n > 0 {
		if !r.need(n * 4) {
			return layoutRequest{}, r.err
		}
		req.style.Features = make([]string, n)
		for i := range req.style.Features {
			req.style.Features[i] = r.str()
		}
	}
	req.style.GlyphRefs = r.bool()
	req.style.GenerateAlt = r.bool()
	req.opts.Recovery = untie.RecoveryMode(r.u8())
	return req, r.err
}

type systemTextRequest struct {
	key      layoutKey
	language string
	missing  []untie.MissingFallback
	measured []textMeasure
}

func encodeSystemTextRequest(req systemTextRequest) []byte {
	var w binWriter
	w.str(req.key.source)
	w.u8(uint8(req.key.dialect))
	w.bool(req.key.display)
	w.f32(req.key.maxWidth)
	w.str(req.language)
	w.u32(uint32(len(req.missing)))
	for _, m := range req.missing {
		w.srcRange(m.Source)
		w.str(m.Text)
		w.u32(uint32(len(m.TriedFaces)))
		for _, f := range m.TriedFaces {
			w.str(f)
		}
	}
	w.u32(uint32(len(req.measured)))
	for _, m := range req.measured {
		w.f32(m.width)
		w.f32(m.ascent)
		w.f32(m.descent)
	}
	return w.b
}

func decodeSystemTextRequest(data []byte) (systemTextRequest, error) {
	r := binReader{b: data}
	req := systemTextRequest{
		key: layoutKey{
			source:   r.str(),
			dialect:  Dialect(r.u8()),
			display:  r.bool(),
			maxWidth: r.f32(),
		},
		language: r.str(),
	}
	if n := int(r.u32()); n > 0 {
		if !r.need(n * 12) {
			return systemTextRequest{}, r.err
		}
		req.missing = make([]untie.MissingFallback, n)
		for i := range req.missing {
			req.missing[i].Source = r.srcRange()
			req.missing[i].Text = r.str()
			if k := int(r.u32()); k > 0 {
				if !r.need(k * 4) {
					return systemTextRequest{}, r.err
				}
				tried := make([]string, k)
				for j := range tried {
					tried[j] = r.str()
				}
				req.missing[i].TriedFaces = tried
			}
		}
	}
	if n := int(r.u32()); n > 0 {
		if !r.need(n * 12) {
			return systemTextRequest{}, r.err
		}
		req.measured = make([]textMeasure, n)
		for i := range req.measured {
			req.measured[i] = textMeasure{width: r.f32(), ascent: r.f32(), descent: r.f32()}
		}
	}
	return req, r.err
}

func encodeBox(box *untie.Box) []byte {
	if box == nil {
		return nil
	}
	var w binWriter
	w.b = make([]byte, 0, 256+len(box.Items)*64+len(box.Paths)*128)
	w.f32(box.Width)
	w.f32(box.Ascent)
	w.f32(box.Descent)
	w.f32(box.Italic)
	w.f32(box.AxisHeight)
	w.f32(box.Overflow)

	w.u32(uint32(len(box.Items)))
	for _, it := range box.Items {
		w.u8(uint8(it.Kind))
		w.u8(uint8(it.Paint.Role))
		w.bool(it.Paint.HasExplicit)
		if it.Paint.HasExplicit {
			w.u8(it.Paint.Explicit.R)
			w.u8(it.Paint.Explicit.G)
			w.u8(it.Paint.Explicit.B)
			w.u8(it.Paint.Explicit.A)
		}
		w.rect(it.Bound)
		w.u64(uint64(it.NodeID))
		w.i32(it.PathIndex)
		w.rect(it.Rect)
		w.u8(uint8(it.Snap))
		w.str(it.FontID)
		w.u16(it.Glyph)
		w.point(it.At)
		w.f32(it.Em)
	}

	w.u32(uint32(len(box.Paths)))
	for _, path := range box.Paths {
		w.rect(path.Bound)
		w.u32(uint32(len(path.Segments)))
		for _, seg := range path.Segments {
			w.u8(uint8(seg.Op))
			switch seg.Op {
			case geometry.SegMoveTo, geometry.SegLineTo:
				w.point(seg.Points[0])
			case geometry.SegQuadTo:
				w.point(seg.Points[0])
				w.point(seg.Points[1])
			case geometry.SegCubeTo:
				w.point(seg.Points[0])
				w.point(seg.Points[1])
				w.point(seg.Points[2])
			}
		}
	}

	w.u32(uint32(len(box.Hits)))
	for _, hit := range box.Hits {
		w.srcRange(hit.Source)
		w.rect(hit.Area)
		w.i32(hit.Depth)
		w.u8(uint8(hit.Kind))
		w.u64(uint64(hit.NodeID))
	}

	w.u32(uint32(len(box.CaretStops)))
	for _, c := range box.CaretStops {
		w.i32(c.Source)
		w.f32(c.X)
		w.f32(c.Top)
		w.f32(c.Bottom)
		w.u64(uint64(c.NodeID))
	}

	w.u32(uint32(len(box.FontUsage.Faces)))
	for _, f := range box.FontUsage.Faces {
		w.srcRange(f.Source)
		w.str(f.FaceID)
		w.bool(f.Fallback)
		w.bool(f.Missing)
	}

	w.u32(uint32(len(box.FontUsage.Missing)))
	for _, m := range box.FontUsage.Missing {
		w.srcRange(m.Source)
		w.str(m.Text)
		w.u32(uint32(len(m.TriedFaces)))
		for _, f := range m.TriedFaces {
			w.str(f)
		}
	}

	return w.b
}

func decodeBox(data []byte) (*untie.Box, error) {
	if len(data) == 0 {
		return nil, nil
	}
	r := binReader{b: data}
	box := &untie.Box{
		Width:      r.f32(),
		Ascent:     r.f32(),
		Descent:    r.f32(),
		Italic:     r.f32(),
		AxisHeight: r.f32(),
		Overflow:   r.f32(),
	}

	if n := int(r.u32()); n > 0 {
		if !r.need(n * 16) {
			return nil, r.err
		}
		box.Items = make([]display.Item, n)
		for i := range box.Items {
			it := &box.Items[i]
			it.Kind = display.ItemKind(r.u8())
			it.Paint.Role = display.Role(r.u8())
			it.Paint.HasExplicit = r.bool()
			if it.Paint.HasExplicit {
				it.Paint.Explicit = display.RGBA{R: r.u8(), G: r.u8(), B: r.u8(), A: r.u8()}
			}
			it.Bound = r.rect()
			it.NodeID = identity.NodeID(r.u64())
			it.PathIndex = r.i32()
			it.Rect = r.rect()
			it.Snap = display.SnapAxis(r.u8())
			it.FontID = r.str()
			it.Glyph = r.u16()
			it.At = r.point()
			it.Em = r.f32()
		}
	}

	if n := int(r.u32()); n > 0 {
		if !r.need(n * 20) {
			return nil, r.err
		}
		box.Paths = make([]display.PathTemplate, n)
		for i := range box.Paths {
			box.Paths[i].Bound = r.rect()
			if k := int(r.u32()); k > 0 {
				if !r.need(k) {
					return nil, r.err
				}
				segs := make([]geometry.Segment, k)
				for j := range segs {
					op := geometry.SegOp(r.u8())
					segs[j].Op = op
					switch op {
					case geometry.SegMoveTo, geometry.SegLineTo:
						segs[j].Points[0] = r.point()
					case geometry.SegQuadTo:
						segs[j].Points[0] = r.point()
						segs[j].Points[1] = r.point()
					case geometry.SegCubeTo:
						segs[j].Points[0] = r.point()
						segs[j].Points[1] = r.point()
						segs[j].Points[2] = r.point()
					}
				}
				box.Paths[i].Segments = segs
			}
		}
	}

	if n := int(r.u32()); n > 0 {
		if !r.need(n * 37) {
			return nil, r.err
		}
		box.Hits = make([]display.Hit, n)
		for i := range box.Hits {
			box.Hits[i] = display.Hit{
				Source: r.srcRange(),
				Area:   r.rect(),
				Depth:  r.i32(),
				Kind:   display.HitKind(r.u8()),
				NodeID: identity.NodeID(r.u64()),
			}
		}
	}

	if n := int(r.u32()); n > 0 {
		if !r.need(n * 24) {
			return nil, r.err
		}
		box.CaretStops = make([]display.CaretStop, n)
		for i := range box.CaretStops {
			box.CaretStops[i] = display.CaretStop{
				Source: r.i32(),
				X:      r.f32(),
				Top:    r.f32(),
				Bottom: r.f32(),
				NodeID: identity.NodeID(r.u64()),
			}
		}
	}

	if n := int(r.u32()); n > 0 {
		if !r.need(n * 14) {
			return nil, r.err
		}
		box.FontUsage.Faces = make([]display.FaceUse, n)
		for i := range box.FontUsage.Faces {
			box.FontUsage.Faces[i] = display.FaceUse{
				Source:   r.srcRange(),
				FaceID:   r.str(),
				Fallback: r.bool(),
				Missing:  r.bool(),
			}
		}
	}

	if n := int(r.u32()); n > 0 {
		if !r.need(n * 16) {
			return nil, r.err
		}
		box.FontUsage.Missing = make([]display.MissingFallback, n)
		for i := range box.FontUsage.Missing {
			box.FontUsage.Missing[i].Source = r.srcRange()
			box.FontUsage.Missing[i].Text = r.str()
			if k := int(r.u32()); k > 0 {
				if !r.need(k * 4) {
					return nil, r.err
				}
				tried := make([]string, k)
				for j := range tried {
					tried[j] = r.str()
				}
				box.FontUsage.Missing[i].TriedFaces = tried
			}
		}
	}

	if r.err != nil {
		return nil, r.err
	}
	return box, nil
}
