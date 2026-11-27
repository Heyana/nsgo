package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

const (
	spriteAtlasPath = "asset:perf/sprite-atlas.png"
	spriteCanvasID  = "perf-sprite-canvas"
	spriteTile      = 32
	spriteTiles     = 1024 / spriteTile
	spriteSize      = 16
)

type spriteFrames struct {
	mu   sync.Mutex
	last int
	at   time.Time

	// Assigned once on mount and used through App.Post.
	pause  func()
	single func()
}

func (f *spriteFrames) record(frame int) {
	f.mu.Lock()
	f.last, f.at = frame, time.Now()
	f.mu.Unlock()
}

func (f *spriteFrames) latest() (int, time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last, f.at
}

// animate counts animation callbacks in the signal it returns, recording each in frames when they are measured.
func animate(cx *core.Scope, frames *spriteFrames) *core.Signal[int] {
	frame := core.NewSignal(cx, 0)
	drawFrame := func(time.Time) {
		next := frame.Get() + 1
		if frames != nil {
			frames.record(next)
		}
		frame.Set(next)
	}
	var step func(time.Time)
	var stop func()
	step = func(at time.Time) {
		drawFrame(at)
		stop = cx.RequestAnimationFrame(step)
	}
	stop = cx.RequestAnimationFrame(step)
	if frames != nil {
		frames.pause = func() { stop() }
		frames.single = func() { cx.RequestAnimationFrame(drawFrame) }
	}
	return frame
}

func spriteCanvas(cx *core.Scope, count int, frames *spriteFrames) core.Node {
	frame := animate(cx, frames)
	atlas := core.Src(spriteAtlasPath)
	return core.Canvas{
		Style:  core.Style{Width: core.Pct(100), Height: core.Pct(100), Background: core.RGB(0x152b37)},
		TestID: spriteCanvasID,
		Draw: func(d *core.Drawing, size core.Size) {
			version := frame.Get()
			width, height := max(1, int(size.Width)-spriteSize), max(1, int(size.Height)-spriteSize)
			for i := range count {
				tile := i % (spriteTiles * spriteTiles)
				sx, sy := float32(tile%spriteTiles*spriteTile), float32(tile/spriteTiles*spriteTile)
				x := float32((i*137 + version*(1+i%4)) % width)
				y := float32((i*97 + version*(1+i%3)) % height)
				d.DrawImageFrom(atlas, sx, sy, spriteTile, spriteTile, x, y, spriteSize, spriteSize)
			}
			if frames != nil {
				markSpriteFrame(d, size, byte(version))
			}
		},
	}
}

const (
	meshColumns, meshRows = 25, 20
	meshTriangles         = meshColumns * meshRows * 2
)

// meshCanvas draws the atlas through one DrawVertices of 1,000 triangles covering the canvas, every vertex moving each frame, as #1211's throughput criterion asks.
func meshCanvas(cx *core.Scope, frames *spriteFrames) core.Node {
	frame := animate(cx, frames)
	atlas := core.Src(spriteAtlasPath)
	var vertices []core.CanvasVertex
	return core.Canvas{
		Style:  core.Style{Width: core.Pct(100), Height: core.Pct(100), Background: core.RGB(0x152b37)},
		TestID: spriteCanvasID,
		Draw: func(d *core.Drawing, size core.Size) {
			version := frame.Get()
			phase := float64(version) / 20
			w, h := float64(size.Width), float64(size.Height)
			at := func(column, row int) core.CanvasVertex {
				x, y := w*float64(column)/meshColumns, h*float64(row)/meshRows
				// The edges stay put so the mesh keeps covering the canvas.
				if column > 0 && column < meshColumns {
					x += 8 * math.Sin(phase+y/60)
				}
				if row > 0 && row < meshRows {
					y += 8 * math.Cos(phase+x/80)
				}
				return core.CanvasVertex{Position: core.Point{X: core.Points(x), Y: core.Points(y)},
					TexCoord: core.Point{X: core.Points(1024 * column / meshColumns), Y: core.Points(1024 * row / meshRows)}}
			}
			vertices = vertices[:0]
			for row := range meshRows {
				for column := range meshColumns {
					a, c := at(column, row), at(column+1, row+1)
					vertices = append(vertices, a, at(column+1, row), c, a, c, at(column, row+1))
				}
			}
			d.DrawVertices(atlas, vertices)
			if frames != nil {
				markSpriteFrame(d, size, byte(version))
			}
		},
	}
}

// A ten-column marker lets a capture identify the version it actually
// reads. The outer white and black columns distinguish it from sprites.
func markSpriteFrame(d *core.Drawing, size core.Size, frame byte) {
	w, h := float32(size.Width), float32(size.Height)
	for column := range 10 {
		ink := core.RGB(0)
		if column == 0 || column < 9 && frame&(1<<(column-1)) != 0 {
			ink = core.RGB(0xffffff)
		}
		d.SetFillColor(ink).FillRect(w*(0.25+0.05*float32(column)), h*0.4, w*0.05, h*0.2)
	}
}

func capturedSpriteFrame(img image.Image) (byte, bool) {
	bounds := img.Bounds()
	var frame byte
	for column := range 10 {
		x := bounds.Min.X + int(float64(bounds.Dx())*(0.275+0.05*float64(column)))
		r, g, b, _ := img.At(x, bounds.Min.Y+bounds.Dy()/2).RGBA()
		white := min(r, g, b) >= 0xe000
		black := max(r, g, b) <= 0x1fff
		switch {
		case column == 0:
			if !white {
				return 0, false
			}
		case column == 9:
			if !black {
				return 0, false
			}
		case white:
			frame |= 1 << (column - 1)
		case !black:
			return 0, false
		}
	}
	return frame, true
}

func TestCanvasSpriteWorkload(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("headless checks the recorded operations")
	}
	for _, count := range []int{1000, 5000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node { return spriteCanvas(cx, count, nil) }, perfOptions()...)
			check := func() []core.Point {
				t.Helper()
				v := app.Find("id:" + spriteCanvasID)
				ops := v.Ops
				points := make([]core.Point, 0, count)
				for i := range ops {
					op := &ops[i]
					if op.Kind != core.DrawImage {
						continue
					}
					img := v.Drawing.Image(op)
					if img == nil || img.URI != spriteAtlasPath ||
						op.X2 != spriteTile || op.Y2 != spriteTile ||
						op.X1 < 0 || op.Y1 < 0 || op.X1+op.X2 > 1024 || op.Y1+op.Y2 > 1024 {
						t.Fatalf("sprite has invalid atlas crop: %+v", *op)
					}
					points = append(points, core.Point{X: core.Points(op.X), Y: core.Points(op.Y)})
				}
				if len(points) != count {
					t.Fatalf("drew %d sprites, want %d", len(points), count)
				}
				return points
			}
			before := check()
			app.Advance(17 * time.Millisecond)
			for i, after := range check() {
				if after == before[i] {
					t.Fatalf("sprite %d did not move from %v", i, after)
				}
			}
		})
	}
}

func TestCanvasMeshWorkload(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("headless checks the recorded operations")
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node { return meshCanvas(cx, nil) }, perfOptions()...)
	check := func() []core.CanvasVertex {
		t.Helper()
		v := app.Find("id:" + spriteCanvasID)
		var meshes []core.DrawOp
		for _, op := range v.Ops {
			if op.Kind == core.DrawVertices {
				meshes = append(meshes, op)
			}
		}
		if len(meshes) != 1 || v.Drawing.Image(&meshes[0]) == nil || v.Drawing.Image(&meshes[0]).URI != spriteAtlasPath {
			t.Fatalf("drew %d meshes, want one of the atlas", len(meshes))
		}
		if n := len(meshes[0].Mesh.Vertices); n != meshTriangles*3 {
			t.Fatalf("mesh has %d vertices, want %d", n, meshTriangles*3)
		}
		return meshes[0].Mesh.Vertices
	}
	before := check()
	app.Advance(17 * time.Millisecond)
	if slices.Equal(check(), before) {
		t.Fatal("the mesh did not deform")
	}
}

func TestPerfCanvasSprites1000(t *testing.T) { perfCanvasSprites(t, 1000) }

func TestPerfCanvasSprites5000(t *testing.T) { perfCanvasSprites(t, 5000) }

func perfCanvasSprites(t *testing.T, count int) {
	perfCanvas(t, fmt.Sprintf("sprites=%d", count), fmt.Sprintf("Canvas %d sprites", count),
		func(cx *core.Scope, frames *spriteFrames) core.Node { return spriteCanvas(cx, count, frames) })
}

func TestPerfCanvasMesh1000(t *testing.T) {
	perfCanvas(t, fmt.Sprintf("mesh=%d", meshTriangles), fmt.Sprintf("Canvas %d mesh triangles", meshTriangles), meshCanvas)
}

// perfCanvas measures a canvas that marks its frames, named load in its log lines and mark in the monitor's.
func perfCanvas(t *testing.T, load, mark string, canvas func(*core.Scope, *spriteFrames) core.Node) {
	perfScenarios(t)
	if nsgotest.Target() == "" {
		t.Skip("run on a target with -fps")
	}
	frames := new(spriteFrames)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node { return canvas(cx, frames) }, perfOptions()...)
	app.Advance(2 * time.Second)
	app.Mark(mark)
	first, started := frames.latest()
	app.Advance(perfSeconds(5))
	last, ended := frames.latest()
	app.Mark("-")
	if first == 0 || last <= first {
		t.Fatal("the canvas did not receive display frames")
	}
	fmt.Printf("nsgo: perf %s callbacks=%d callback_fps=%.2f\n", load, last-first, float64(last-first)/ended.Sub(started).Seconds())
	// Capture a held one-shot frame after the throughput run. Continuously
	// invalidating a GTK widget can leave its offscreen capture empty between
	// paints. Capture age includes the test host's PNG readback and settling.
	app.Post(frames.pause)
	app.Settle()
	var ages []time.Duration
	var sawSprites bool
	for range 7 {
		previous, _ := frames.latest()
		app.Post(frames.single)
		app.WaitUntil(2*time.Second, func() bool {
			current, _ := frames.latest()
			return current > previous
		})
		want, at := frames.latest()
		data := app.CapturePNG()
		received := time.Now()
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		sawSprites = sawSprites || capturedSprites(img)
		if frame, ok := capturedSpriteFrame(img); ok && frame == byte(want) {
			ages = append(ages, received.Sub(at))
		}
	}
	if !sawSprites {
		app.Screenshot("perf-" + strings.NewReplacer("=", "-").Replace(load))
		t.Fatal("the atlas was not drawn")
	}
	if len(ages) == 0 {
		t.Fatal("no capture contained a recorded frame marker")
	}
	slices.Sort(ages)
	fmt.Printf("nsgo: perf %s captures=%d callback_to_capture_p50=%v max=%v\n", load, len(ages), ages[len(ages)/2], ages[len(ages)-1])
}

// Samples the area above the marker for the atlas's saturated pixels.
func capturedSprites(img image.Image) bool {
	b := img.Bounds()
	for row := 2; row <= 6; row++ {
		for column := 2; column < 18; column++ {
			r, g, blue, _ := img.At(b.Min.X+b.Dx()*column/20, b.Min.Y+b.Dy()*row/20).RGBA()
			if max(r, g, blue) >= 0x8800 && min(r, g, blue) < 0x6000 && max(r, g, blue)-min(r, g, blue) > 0x3400 {
				return true
			}
		}
	}
	return false
}
