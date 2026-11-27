package android

import (
	"encoding/binary"
	"fmt"
	"image"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

type damageTestNode uint32

func (n damageTestNode) AnimatedNodeID() uint32 { return uint32(n) }

// encodedDrawingOps encodes d through writeDrawing and returns only the
// encoded operation byte stream that CanvasNode holds and patches.
func encodedDrawingOps(d *driver.Drawing) []byte {
	var w writer
	writeDrawing(&w, d)
	n := int(binary.BigEndian.Uint32(w.buf[:4]))
	out := make([]byte, n)
	copy(out, w.buf[4:4+n])
	return out
}

// applyDrawingChange returns a copy of d whose op index has arg set to num/col.
func applyDrawingChange(d *driver.Drawing, op int, arg driver.DrawArg, num float64, col driver.Color) *driver.Drawing {
	ops := append([]driver.DrawOp(nil), d.Ops...)
	ops[op] = driver.DrawSlot{Op: op, Arg: arg}.Set(ops[op], num, col)
	return &driver.Drawing{Ops: ops, Slots: d.Slots, Nodes: d.Nodes}
}

// TestAndroidCanvasDamageMatchesDriver compiles CanvasDamage.java and
// Protocol.java on the host JVM and verifies that CanvasDamage.scan matches
// driver.CanvasDamage across every drawing operation, state transition,
// clip/composite fallback, non-finite bound and split sample, and that
// CanvasDamage.pixelClip pads, rounds and clamps backing clip rects cleanly.
func TestAndroidCanvasDamageMatchesDriver(t *testing.T) {
	javac, java := androidJDK(t)
	tmp := t.TempDir()

	type damageCase struct {
		name   string
		before *driver.Drawing
		after  *driver.Drawing
		rawA   []byte
		rawB   []byte
		rawExp string // "FULL" for raw buffer error cases
	}

	var cases []damageCase

	// 1. canvastest.Split samples (ring moves X; path changes Color via color interpolation).
	for _, smp := range canvastest.Split(nil) {
		a := driver.NewAnimator(nil)
		if smp.Name == "path" {
			a.Apply([]driver.AnimationOp{
				{Kind: driver.AnimCreateValue, Node: 2, Value: 0},
				{Kind: driver.AnimCreateInterpolate, Node: 1, Input: 2, Interpolation: &driver.Interpolation{
					Input:  []float64{0, 1},
					Colors: []driver.Color{driver.RGB(0xff0000), driver.RGB(0x0000ff)},
				}},
			})
			a.BindCanvas(1, smp.Drawing.Slots)
			beforeOps := append([]driver.DrawOp(nil), a.CanvasOps(1, smp.Drawing)...)
			a.Apply([]driver.AnimationOp{{Kind: driver.AnimSet, Node: 2, Value: 1}})
			afterOps := append([]driver.DrawOp(nil), a.CanvasOps(1, smp.Drawing)...)
			cases = append(cases, damageCase{
				name:   "split_" + smp.Name,
				before: &driver.Drawing{Ops: beforeOps},
				after:  &driver.Drawing{Ops: afterOps},
			})
		} else {
			a.Apply([]driver.AnimationOp{{Kind: driver.AnimCreateValue, Node: 1, Value: 120}})
			a.BindCanvas(1, smp.Drawing.Slots)
			beforeOps := append([]driver.DrawOp(nil), a.CanvasOps(1, smp.Drawing)...)
			a.Apply([]driver.AnimationOp{{Kind: driver.AnimSet, Node: 1, Value: 60}})
			afterOps := append([]driver.DrawOp(nil), a.CanvasOps(1, smp.Drawing)...)
			cases = append(cases, damageCase{
				name:   "split_" + smp.Name,
				before: &driver.Drawing{Ops: beforeOps},
				after:  &driver.Drawing{Ops: afterOps},
			})
		}
	}

	// 2. Unchanged drawing -> empty damage.
	{
		d := driver.NewDrawing(nil).SetFillColor(driver.RGB(0x112233)).FillRect(0, 0, 200, 120)
		cases = append(cases, damageCase{name: "unchanged", before: d, after: d})
	}

	// 3. Moving arc over static background.
	{
		d := driver.NewDrawing(nil)
		d.SetFillColor(driver.RGB(0xff0000)).FillRect(0, 0, 500, 300)
		d.BeginPath().Arc(100, 50, 10, 0, 2*math.Pi, false)
		idx := len(d.Ops) - 1
		d.Fill()
		cases = append(cases, damageCase{
			name:   "moving_arc",
			before: d,
			after:  applyDrawingChange(d, idx, driver.ArgX, 140, 0),
		})
	}

	// 4. Transforms (Translate, Scale, Rotate, Transform, SetTransform) + Save/Restore.
	{
		d := driver.NewDrawing(nil)
		d.Save()
		d.Translate(10, 20)
		idx := len(d.Ops) - 1
		d.Scale(2, 1.5).Rotate(0.3).SetLineWidth(4).SetLineCap(driver.CapSquare).SetLineJoin(driver.JoinBevel)
		d.BeginPath().Rect(0, 0, 20, 15).Stroke()
		d.Restore()
		d.FillRect(300, 200, 10, 10) // unaffected after restore
		cases = append(cases, damageCase{
			name:   "transform_save_restore",
			before: d,
			after:  applyDrawingChange(d, idx, driver.ArgX, 40, 0),
		})
	}
	{
		d := driver.NewDrawing(nil)
		d.Transform(1, 0, 0, 1, 5, 5)
		d.SetTransform(1.5, 0.2, -0.1, 1.2, 10, 12)
		idx := len(d.Ops) - 1
		d.BeginPath().RoundRect(0, 0, 30, 20, 5).Fill()
		cases = append(cases, damageCase{
			name:   "set_transform_round_rect",
			before: d,
			after:  applyDrawingChange(d, idx, driver.ArgX, 35, 0),
		})
	}

	// 5. Curves, ellipses, miter limit, dash, dash offset.
	{
		d := driver.NewDrawing(nil)
		d.SetLineWidth(3).SetMiterLimit(5).SetLineJoin(driver.JoinMiter).SetLineDash(4, 2).SetLineDashOffset(1)
		d.BeginPath().MoveTo(10, 10).QuadTo(25, 5, 40, 20).CubicTo(50, 30, 60, 10, 70, 25).Ellipse(80, 30, 12, 6).ClosePath()
		idx := len(d.Ops) - 2 // Ellipse
		d.Stroke()
		cases = append(cases, damageCase{
			name:   "curves_ellipse_stroke",
			before: d,
			after:  applyDrawingChange(d, idx, driver.ArgX, 95, 0),
		})
	}

	// 6. Color, gradient, alpha, shadow.
	{
		d := driver.NewDrawing(nil)
		g := &driver.CanvasGradient{
			X1: 0, Y1: 0, X2: 50, Y2: 0,
			Stops: []driver.GradientStop{{Offset: 0, Color: driver.RGB(0xff0000)}, {Offset: 1, Color: driver.RGB(0x0000ff)}},
		}
		d.SetFillGradient(g).SetAlpha(0.5)
		idx := len(d.Ops) - 1
		d.SetShadow(driver.RGBA(0x000000, 0.6), 4, 6, 5)
		d.BeginPath().Rect(15, 20, 40, 25).Fill()
		cases = append(cases, damageCase{
			name:   "gradient_alpha_shadow",
			before: d,
			after:  applyDrawingChange(d, idx, driver.ArgA, 0.9, 0),
		})
	}
	{
		d := driver.NewDrawing(nil)
		d.SetFillColor(driver.RGB(0x112233)).Bind(driver.ArgColor, damageTestNode(1))
		idx := len(d.Ops) - 1
		d.FillRect(12, 18, 30, 22)
		cases = append(cases, damageCase{
			name:   "fill_color_change",
			before: d,
			after:  applyDrawingChange(d, idx, driver.ArgColor, 0, driver.RGB(0x445566)),
		})
	}

	// 7. FillText & StrokeText with multi-byte UTF-8 runes.
	{
		d := driver.NewDrawing(nil)
		d.FillText("Hi 🚀", 10, 25, driver.Font{Size: 12})
		cases = append(cases, damageCase{
			name:   "fill_text_utf8",
			before: d,
			after:  applyDrawingChange(d, 0, driver.ArgX, 30, 0),
		})
	}
	{
		d := driver.NewDrawing(nil)
		d.SetLineWidth(4).SetLineJoin(driver.JoinRound).StrokeText("OK", 20, 40, driver.Font{Size: 14})
		cases = append(cases, damageCase{
			name:   "stroke_text",
			before: d,
			after:  applyDrawingChange(d, 2, driver.ArgY, 55, 0),
		})
	}

	// 8. DrawImage: sized, tinted, and natural-size fallback.
	{
		src := driver.ImageSource{URI: "asset:tile.png"}
		d := driver.NewDrawing(nil).DrawImageIn(src, 10, 15, 40, 30)
		cases = append(cases, damageCase{
			name:   "image_sized",
			before: d,
			after:  applyDrawingChange(d, 0, driver.ArgX, 25, 0),
		})
	}
	{
		src := driver.ImageSource{URI: "asset:tile.png"}
		d := driver.NewDrawing(nil).DrawTintedImageIn(src, driver.RGB(0xff0000), 10, 15, 40, 30)
		cases = append(cases, damageCase{
			name:   "image_tinted",
			before: d,
			after:  applyDrawingChange(d, 0, driver.ArgColor, 0, driver.RGB(0x00ff00)),
		})
	}
	{
		src := driver.ImageSource{URI: "asset:tile.png"}
		d := driver.NewDrawing(nil).DrawImage(src, 10, 15)
		cases = append(cases, damageCase{
			name:   "image_natural_full",
			before: d,
			after:  applyDrawingChange(d, 0, driver.ArgX, 20, 0),
		})
	}

	// 9. DrawVertices and DrawAtlas under changing transform.
	{
		src := driver.ImageSource{Pixels: image.NewRGBA(image.Rect(0, 0, 2, 2))}
		verts := []driver.CanvasVertex{
			{Position: driver.Point{X: 2, Y: 4}, TexCoord: driver.Point{X: 0, Y: 0}},
			{Position: driver.Point{X: 8, Y: 4}, TexCoord: driver.Point{X: 2, Y: 0}},
			{Position: driver.Point{X: 8, Y: 12}, TexCoord: driver.Point{X: 2, Y: 2}},
		}
		d := driver.NewDrawing(nil).Translate(0, 0).DrawVertices(src, verts)
		cases = append(cases, damageCase{
			name:   "vertices_moved",
			before: d,
			after:  applyDrawingChange(d, 0, driver.ArgX, 10, 0),
		})
	}
	{
		src := driver.ImageSource{URI: "asset:atlas.png"}
		tile := driver.Bounds{Origin: driver.Point{X: 0, Y: 0}, Size: driver.Size{Width: 16, Height: 12}}
		d := driver.NewDrawing(nil).Translate(0, 0).DrawAtlas(src, []driver.Bounds{tile},
			[]driver.RSXform{{SCos: 1, SSin: 0, TX: 20, TY: 30}}, []float32{1})
		cases = append(cases, damageCase{
			name:   "atlas_moved",
			before: d,
			after:  applyDrawingChange(d, 0, driver.ArgY, 15, 0),
		})
	}

	// 10. DrawClear moving rect.
	{
		d := driver.NewDrawing(nil).Clear(10, 10, 20, 20)
		cases = append(cases, damageCase{
			name:   "clear_rect",
			before: d,
			after:  applyDrawingChange(d, 0, driver.ArgX, 40, 0),
		})
	}

	// 11. Fallbacks: changed clip, non-source-over composite, NaN.
	{
		d := driver.NewDrawing(nil)
		d.BeginPath().Rect(0, 0, 10, 10)
		idx := len(d.Ops) - 1
		d.Clip().FillRect(0, 0, 100, 100)
		cases = append(cases, damageCase{
			name:   "changed_clip_full",
			before: d,
			after:  applyDrawingChange(d, idx, driver.ArgW, 50, 0),
		})
	}
	{
		d := driver.NewDrawing(nil)
		d.SetComposite(driver.CompositeCopy).BeginPath().Rect(0, 0, 10, 10)
		idx := len(d.Ops) - 1
		d.Fill()
		cases = append(cases, damageCase{
			name:   "composite_copy_full",
			before: d,
			after:  applyDrawingChange(d, idx, driver.ArgX, 20, 0),
		})
	}
	{
		d := driver.NewDrawing(nil)
		d.BeginPath().Rect(0, 0, 10, 10)
		idx := len(d.Ops) - 1
		d.Fill()
		cases = append(cases, damageCase{
			name:   "nan_full",
			before: d,
			after:  applyDrawingChange(d, idx, driver.ArgX, math.NaN(), 0),
		})
	}
	{
		cases = append(cases, damageCase{
			name:   "mismatched_len_full",
			rawA:   []byte{byte(driver.DrawBeginPath)},
			rawB:   []byte{byte(driver.DrawBeginPath), byte(driver.DrawClosePath)},
			rawExp: "FULL",
		})
	}

	// Write case inputs into tmp/cases.bin for the JVM driver.
	var payload []byte
	putU32 := func(v uint32) {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], v)
		payload = append(payload, b[:]...)
	}
	putU32(uint32(len(cases)))
	for _, c := range cases {
		var ba, bb []byte
		if c.rawExp != "" {
			ba, bb = c.rawA, c.rawB
		} else {
			ba, bb = encodedDrawingOps(c.before), encodedDrawingOps(c.after)
		}
		putU32(uint32(len(ba)))
		payload = append(payload, ba...)
		putU32(uint32(len(bb)))
		payload = append(payload, bb...)
	}
	inPath := filepath.Join(tmp, "cases.bin")
	if err := os.WriteFile(inPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	driverSrc := filepath.Join(tmp, "CanvasDamageDriver.java")
	const src = `package dev.oneo.nsgo;

import java.io.DataInputStream;
import java.io.FileInputStream;

public class CanvasDamageDriver {
    public static void main(String[] args) throws Exception {
        try (DataInputStream in = new DataInputStream(new FileInputStream(args[0]))) {
            int n = in.readInt();
            for (int i = 0; i < n; i++) {
                byte[] a = new byte[in.readInt()];
                in.readFully(a);
                byte[] b = new byte[in.readInt()];
                in.readFully(b);
                CanvasDamage.Result r = CanvasDamage.scan(a, b);
                if (!r.bounded) {
                    System.out.println("CASE " + i + " FULL");
                } else if (r.empty) {
                    System.out.println("CASE " + i + " EMPTY");
                } else {
                    System.out.println("CASE " + i + " BOX " + r.x0 + " " + r.y0 + " " + r.x1 + " " + r.y1);
                }
            }
        }
        // pixelClip checks: 1x, 2x, low-res (0.5x min 2px pad), clamped, off-canvas, empty, full.
        CanvasDamage.Result box = new CanvasDamage.Result(true, false, 10.2, 20.4, 30.6, 40.8);
        printClip("CLIP1X", CanvasDamage.pixelClip(box, 1f, 1f, 200, 100));
        printClip("CLIP2X", CanvasDamage.pixelClip(box, 2f, 2f, 400, 200));
        printClip("CLIPLOW", CanvasDamage.pixelClip(new CanvasDamage.Result(true, false, 10, 10, 20, 20), 0.5f, 0.5f, 100, 100));
        printClip("CLIPCLAMP", CanvasDamage.pixelClip(new CanvasDamage.Result(true, false, -5, -5, 250, 120), 1f, 1f, 200, 100));
        printClip("CLIPOFF", CanvasDamage.pixelClip(new CanvasDamage.Result(true, false, 250, 10, 280, 30), 1f, 1f, 200, 100));
        printClip("CLIPEMPTY", CanvasDamage.pixelClip(CanvasDamage.Result.EMPTY, 1f, 1f, 200, 100));
        printClip("CLIPFULL", CanvasDamage.pixelClip(CanvasDamage.Result.FULL, 1f, 1f, 200, 100));
    }

    private static void printClip(String tag, CanvasDamage.PixelRect r) {
        if (r == null) {
            System.out.println(tag + " NULL");
        } else {
            System.out.println(tag + " " + r.left + " " + r.top + " " + r.right + " " + r.bottom);
        }
    }
}
`
	if err := os.WriteFile(driverSrc, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	protocolSrc := filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java")
	damageSrc := filepath.Join("java", "dev", "oneo", "nsgo", "CanvasDamage.java")
	out, err := exec.CommandContext(t.Context(), javac, "-d", tmp, protocolSrc, damageSrc, driverSrc).CombinedOutput()
	if err != nil {
		t.Fatalf("javac failed: %v\n%s", err, out)
	}
	out, err = exec.CommandContext(t.Context(), java, "-cp", tmp, "dev.oneo.nsgo.CanvasDamageDriver", inPath).CombinedOutput()
	if err != nil {
		t.Fatalf("java failed: %v\n%s", err, out)
	}

	clips := map[string]string{}
	checked := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if f[0] != "CASE" {
			clips[f[0]] = strings.Join(f[1:], " ")
			continue
		}
		idx, err := strconv.Atoi(f[1])
		if err != nil {
			t.Fatalf("bad case index in %q", line)
		}
		checked++
		c := cases[idx]
		if c.rawExp != "" {
			if f[2] != c.rawExp {
				t.Errorf("%s: got %s, want %s", c.name, f[2], c.rawExp)
			}
			continue
		}
		want, ok := driver.CanvasDamage(c.before.Ops, c.after.Ops)
		if !ok {
			if f[2] != "FULL" {
				t.Errorf("%s: Java returned %q, want FULL (matching driver.CanvasDamage)", c.name, line)
			}
			continue
		}
		if want == (driver.Bounds{}) {
			if f[2] != "EMPTY" {
				t.Errorf("%s: Java returned %q, want EMPTY", c.name, line)
			}
			continue
		}
		if f[2] != "BOX" || len(f) != 7 {
			t.Errorf("%s: Java returned %q, want BOX covering %+v", c.name, line, want)
			continue
		}
		x0, _ := strconv.ParseFloat(f[3], 64)
		y0, _ := strconv.ParseFloat(f[4], 64)
		x1, _ := strconv.ParseFloat(f[5], 64)
		y1, _ := strconv.ParseFloat(f[6], 64)
		wx0, wy0 := float64(want.Origin.X), float64(want.Origin.Y)
		wx1, wy1 := float64(want.Origin.X+want.Size.Width), float64(want.Origin.Y+want.Size.Height)
		const eps = 1e-3
		if math.Abs(x0-wx0) > eps || math.Abs(y0-wy0) > eps || math.Abs(x1-wx1) > eps || math.Abs(y1-wy1) > eps {
			t.Errorf("%s: Java box [%.4f, %.4f, %.4f, %.4f] != driver [%.4f, %.4f, %.4f, %.4f]",
				c.name, x0, y0, x1, y1, wx0, wy0, wx1, wy1)
		}
	}
	if checked != len(cases) {
		t.Fatalf("checked %d cases, want %d", checked, len(cases))
	}

	wantClips := map[string]string{
		"CLIP1X":    "8 18 33 43",
		"CLIP2X":    "16 36 66 86",
		"CLIPLOW":   "3 3 12 12",
		"CLIPCLAMP": "0 0 200 100",
		"CLIPOFF":   "NULL",
		"CLIPEMPTY": "NULL",
		"CLIPFULL":  "NULL",
	}
	for tag, want := range wantClips {
		if got := clips[tag]; got != want {
			t.Errorf("%s = %q, want %q", tag, got, want)
		}
	}
}

// TestAndroidCanvasPartialReplayMatchesFull verifies on the host JVM (using
// BufferedImage + Graphics2D) that clipping and clearing a retained canvas
// backing to CanvasDamage.pixelClip and replaying the encoded ops preserves
// pixels outside the damaged rectangle and matches a full redraw pixel-for-pixel
// on the split samples and on a drawing that relies on default paint state.
func TestAndroidCanvasPartialReplayMatchesFull(t *testing.T) {
	javac, java := androidJDK(t)
	tmp := t.TempDir()

	type replayCase struct {
		name          string
		width, height int
		before, after []byte
	}
	var cases []replayCase

	for _, smp := range canvastest.Split(nil) {
		a := driver.NewAnimator(nil)
		if smp.Name == "path" {
			a.Apply([]driver.AnimationOp{
				{Kind: driver.AnimCreateValue, Node: 2, Value: 0},
				{Kind: driver.AnimCreateInterpolate, Node: 1, Input: 2, Interpolation: &driver.Interpolation{
					Input:  []float64{0, 1},
					Colors: []driver.Color{driver.RGB(0xff0000), driver.RGB(0x0000ff)},
				}},
			})
			a.BindCanvas(1, smp.Drawing.Slots)
			before := encodedDrawingOps(&driver.Drawing{Ops: append([]driver.DrawOp(nil), a.CanvasOps(1, smp.Drawing)...)})
			a.Apply([]driver.AnimationOp{{Kind: driver.AnimSet, Node: 2, Value: 1}})
			after := encodedDrawingOps(&driver.Drawing{Ops: append([]driver.DrawOp(nil), a.CanvasOps(1, smp.Drawing)...)})
			cases = append(cases, replayCase{
				name:   smp.Name,
				width:  smp.Width,
				height: smp.Height,
				before: before,
				after:  after,
			})
		} else {
			a.Apply([]driver.AnimationOp{{Kind: driver.AnimCreateValue, Node: 1, Value: 120}})
			a.BindCanvas(1, smp.Drawing.Slots)
			before := encodedDrawingOps(&driver.Drawing{Ops: append([]driver.DrawOp(nil), a.CanvasOps(1, smp.Drawing)...)})
			a.Apply([]driver.AnimationOp{{Kind: driver.AnimSet, Node: 1, Value: 60}})
			after := encodedDrawingOps(&driver.Drawing{Ops: append([]driver.DrawOp(nil), a.CanvasOps(1, smp.Drawing)...)})
			cases = append(cases, replayCase{
				name:   smp.Name,
				width:  smp.Width,
				height: smp.Height,
				before: before,
				after:  after,
			})
		}
	}

	// Default-state drawing: fills/strokes with default black/lineWidth=1 before
	// changing styles for the animated dot.
	{
		d := driver.NewDrawing(nil)
		d.FillRect(10, 10, 140, 35)
		d.StrokeRect(20, 50, 120, 35)
		d.SetLineWidth(2).SetLineJoin(driver.JoinRound).SetStrokeColor(driver.RGB(0x00ff00)).SetFillColor(driver.RGB(0xff0000)).SetAlpha(0.5)
		d.BeginPath().Arc(120, 50, 10, 0, 2*math.Pi, false).Bind(driver.ArgX, damageTestNode(1))
		d.Fill().Stroke()
		a := driver.NewAnimator(nil)
		a.Apply([]driver.AnimationOp{{Kind: driver.AnimCreateValue, Node: 1, Value: 120}})
		a.BindCanvas(1, d.Slots)
		before := encodedDrawingOps(&driver.Drawing{Ops: append([]driver.DrawOp(nil), a.CanvasOps(1, d)...)})
		a.Apply([]driver.AnimationOp{{Kind: driver.AnimSet, Node: 1, Value: 60}})
		after := encodedDrawingOps(&driver.Drawing{Ops: append([]driver.DrawOp(nil), a.CanvasOps(1, d)...)})
		cases = append(cases, replayCase{
			name:   "default_state",
			width:  160,
			height: 100,
			before: before,
			after:  after,
		})
	}

	var payload []byte
	putU32 := func(v uint32) {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], v)
		payload = append(payload, b[:]...)
	}
	putU32(uint32(len(cases)))
	for _, c := range cases {
		putU32(uint32(c.width))
		putU32(uint32(c.height))
		putU32(uint32(len(c.before)))
		payload = append(payload, c.before...)
		putU32(uint32(len(c.after)))
		payload = append(payload, c.after...)
	}
	inPath := filepath.Join(tmp, "replay.bin")
	if err := os.WriteFile(inPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	driverSrc := filepath.Join(tmp, "PartialReplayDriver.java")
	const src = `package dev.oneo.nsgo;

import java.awt.AlphaComposite;
import java.awt.BasicStroke;
import java.awt.Color;
import java.awt.Graphics2D;
import java.awt.RenderingHints;
import java.awt.Shape;
import java.awt.geom.AffineTransform;
import java.awt.geom.Arc2D;
import java.awt.geom.Path2D;
import java.awt.geom.RoundRectangle2D;
import java.awt.image.BufferedImage;
import java.io.DataInputStream;
import java.io.FileInputStream;
import java.nio.ByteBuffer;
import java.util.ArrayDeque;
import java.util.ArrayList;

public class PartialReplayDriver {
    static final class St {
        Color fill = Color.BLACK, stroke = Color.BLACK;
        float width = 1f, alpha = 1f;
        AffineTransform tx = new AffineTransform();
        St copy() {
            St s = new St();
            s.fill = fill; s.stroke = stroke; s.width = width; s.alpha = alpha;
            s.tx = new AffineTransform(tx);
            return s;
        }
    }

    static Color rgba(int c, float a) {
        int r = (c >>> 24) & 0xff, g = (c >>> 16) & 0xff, b = (c >>> 8) & 0xff;
        int al = Math.round((c & 0xff) * a);
        return new Color(r, g, b, Math.max(0, Math.min(255, al)));
    }

    static void play(Graphics2D g, byte[] ops) {
        ByteBuffer b = ByteBuffer.wrap(ops);
        St st = new St();
        ArrayDeque<St> saved = new ArrayDeque<>();
        ArrayList<Shape> clips = new ArrayList<>();
        Path2D.Double path = new Path2D.Double();
        while (b.hasRemaining()) {
            int kind = b.get() & 0xff;
            switch (kind) {
                case Protocol.DRAW_SAVE:
                    saved.push(st.copy());
                    clips.add(g.getClip());
                    break;
                case Protocol.DRAW_RESTORE:
                    if (!saved.isEmpty()) {
                        st = saved.pop();
                        g.setClip(clips.remove(clips.size() - 1));
                    }
                    break;
                case Protocol.DRAW_BEGIN_PATH:
                    path = new Path2D.Double();
                    break;
                case Protocol.DRAW_MOVE_TO:
                    path.moveTo(b.getFloat(), b.getFloat());
                    break;
                case Protocol.DRAW_LINE_TO: {
                    float x = b.getFloat(), y = b.getFloat();
                    if (path.getCurrentPoint() == null) path.moveTo(x, y);
                    else path.lineTo(x, y);
                    break;
                }
                case Protocol.DRAW_RECT: {
                    float x = b.getFloat(), y = b.getFloat(), w = b.getFloat(), h = b.getFloat();
                    path.moveTo(x, y); path.lineTo(x + w, y); path.lineTo(x + w, y + h); path.lineTo(x, y + h); path.closePath();
                    break;
                }
                case Protocol.DRAW_ROUND_RECT: {
                    float x = b.getFloat(), y = b.getFloat(), w = b.getFloat(), h = b.getFloat(), r = b.getFloat();
                    path.append(new RoundRectangle2D.Float(x, y, w, h, r * 2, r * 2), false);
                    break;
                }
                case Protocol.DRAW_ARC: {
                    float x = b.getFloat(), y = b.getFloat(), r = b.getFloat(), a0 = b.getFloat(), a1 = b.getFloat();
                    b.get();
                    double startDeg = -Math.toDegrees(a0);
                    double extentDeg = -Math.toDegrees(a1 - a0);
                    path.append(new Arc2D.Double(x - r, y - r, r * 2, r * 2, startDeg, extentDeg, Arc2D.OPEN), false);
                    break;
                }
                case Protocol.DRAW_CLOSE_PATH:
                    path.closePath();
                    break;
                case Protocol.DRAW_FILL:
                    path.setWindingRule(b.get() != 0 ? Path2D.WIND_EVEN_ODD : Path2D.WIND_NON_ZERO);
                    g.setColor(rgba(st.fill.getRGB() << 8 | (st.fill.getRGB() >>> 24), st.alpha));
                    g.fill(path);
                    break;
                case Protocol.DRAW_STROKE:
                    g.setColor(rgba(st.stroke.getRGB() << 8 | (st.stroke.getRGB() >>> 24), st.alpha));
                    g.setStroke(new BasicStroke(st.width));
                    g.draw(path);
                    break;
                case Protocol.DRAW_CLIP:
                    path.setWindingRule(b.get() != 0 ? Path2D.WIND_EVEN_ODD : Path2D.WIND_NON_ZERO);
                    g.clip(path);
                    break;
                case Protocol.DRAW_FILL_COLOR: {
                    int c = b.getInt();
                    st.fill = new Color((c >>> 24) & 0xff, (c >>> 16) & 0xff, (c >>> 8) & 0xff, c & 0xff);
                    break;
                }
                case Protocol.DRAW_STROKE_COLOR: {
                    int c = b.getInt();
                    st.stroke = new Color((c >>> 24) & 0xff, (c >>> 16) & 0xff, (c >>> 8) & 0xff, c & 0xff);
                    break;
                }
                case Protocol.DRAW_LINE_WIDTH:
                    st.width = b.getFloat();
                    break;
                case Protocol.DRAW_LINE_JOIN:
                    b.get();
                    break;
                case Protocol.DRAW_ALPHA:
                    st.alpha = Math.max(0f, Math.min(1f, b.getFloat()));
                    break;
                case Protocol.DRAW_SHADOW:
                    b.getInt(); b.getFloat(); b.getFloat(); b.getFloat();
                    break;
                case Protocol.DRAW_FILL_TEXT:
                case Protocol.DRAW_STROKE_TEXT: {
                    int tl = b.getInt(); b.position(b.position() + tl);
                    b.getFloat(); b.getFloat();
                    int fl = b.getInt(); b.position(b.position() + fl);
                    b.getFloat(); b.getShort(); b.get(); b.get();
                    break;
                }
                default:
                    throw new IllegalStateException("unexpected op " + kind);
            }
        }
    }

    static BufferedImage renderFull(int w, int h, float scale, byte[] ops) {
        int bw = Math.round(w * scale), bh = Math.round(h * scale);
        BufferedImage img = new BufferedImage(bw, bh, BufferedImage.TYPE_INT_ARGB);
        Graphics2D g = img.createGraphics();
        g.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
        g.scale(scale, scale);
        play(g, ops);
        g.dispose();
        return img;
    }

    public static void main(String[] args) throws Exception {
        final float scale = 2f;
        try (DataInputStream in = new DataInputStream(new FileInputStream(args[0]))) {
            int n = in.readInt();
            for (int i = 0; i < n; i++) {
                int w = in.readInt(), h = in.readInt();
                byte[] before = new byte[in.readInt()]; in.readFully(before);
                byte[] after = new byte[in.readInt()]; in.readFully(after);

                BufferedImage partial = renderFull(w, h, scale, before);
                // Sentinel pixel outside any damaged area; a full redraw would clear it.
                final int sentinel = 0xffff00ff;
                partial.setRGB(0, 0, sentinel);

                CanvasDamage.Result dmg = CanvasDamage.scan(before, after);
                if (!dmg.bounded || dmg.empty) {
                    throw new AssertionError("case " + i + ": expected bounded non-empty damage");
                }
                int bw = partial.getWidth(), bh = partial.getHeight();
                CanvasDamage.PixelRect clip = CanvasDamage.pixelClip(dmg, scale, scale, bw, bh);
                if (clip == null) {
                    throw new AssertionError("case " + i + ": expected non-null clip");
                }
                if ((long) clip.width() * clip.height() * 2 > (long) bw * bh) {
                    throw new AssertionError("case " + i + ": clip too large: " + clip.width() + "x" + clip.height());
                }

                Graphics2D g = partial.createGraphics();
                g.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
                g.setClip(clip.left, clip.top, clip.width(), clip.height());
                g.setComposite(AlphaComposite.Clear);
                g.fillRect(clip.left, clip.top, clip.width(), clip.height());
                g.setComposite(AlphaComposite.SrcOver);
                g.scale(scale, scale);
                play(g, after);
                g.dispose();

                if (partial.getRGB(0, 0) != sentinel) {
                    throw new AssertionError("case " + i + ": undamaged sentinel pixel was overwritten");
                }

                BufferedImage full = renderFull(w, h, scale, after);
                int diff = 0;
                for (int y = 0; y < bh; y++) {
                    for (int x = 0; x < bw; x++) {
                        if (x == 0 && y == 0) continue;
                        int p = partial.getRGB(x, y), q = full.getRGB(x, y);
                        if (p != q) {
                            int dr = Math.abs(((p >>> 16) & 0xff) - ((q >>> 16) & 0xff));
                            int dg = Math.abs(((p >>> 8) & 0xff) - ((q >>> 8) & 0xff));
                            int db = Math.abs((p & 0xff) - (q & 0xff));
                            int da = Math.abs(((p >>> 24) & 0xff) - ((q >>> 24) & 0xff));
                            if (dr > 2 || dg > 2 || db > 2 || da > 2) diff++;
                        }
                    }
                }
                if (diff > 0) {
                    throw new AssertionError("case " + i + ": " + diff + " pixels differed from full redraw");
                }
                System.out.println("OK " + i + " " + clip.left + "," + clip.top + "," + clip.right + "," + clip.bottom);
            }
        }
    }
}
`
	if err := os.WriteFile(driverSrc, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	protocolSrc := filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java")
	damageSrc := filepath.Join("java", "dev", "oneo", "nsgo", "CanvasDamage.java")
	out, err := exec.CommandContext(t.Context(), javac, "-d", tmp, protocolSrc, damageSrc, driverSrc).CombinedOutput()
	if err != nil {
		t.Fatalf("javac failed: %v\n%s", err, out)
	}
	out, err = exec.CommandContext(t.Context(), java, "-Djava.awt.headless=true", "-cp", tmp, "dev.oneo.nsgo.PartialReplayDriver", inPath).CombinedOutput()
	if err != nil {
		t.Fatalf("java partial replay failed: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != len(cases) {
		t.Fatalf("got %d OK lines, want %d:\n%s", len(lines), len(cases), out)
	}
	for i, c := range cases {
		if !strings.HasPrefix(lines[i], fmt.Sprintf("OK %d ", i)) {
			t.Errorf("%s: unexpected output %q", c.name, lines[i])
		}
	}
}
