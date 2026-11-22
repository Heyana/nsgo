//go:build android

package android

import (
	"bufio"
	"image"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shader"
)

func TestMountFlushesDeferredShaderPixels(t *testing.T) {
	pipe, err := os.Create(filepath.Join(t.TempDir(), "batches"))
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()

	const root, first = driver.NodeID(1), driver.NodeID(2)
	var batch driver.MountBatch
	batch.Root = root
	for i := range 3 {
		batch.Items = append(batch.Items, driver.MountItem{Kind: driver.ItemShader, ID: first + driver.NodeID(i), Shader: &driver.Shader{
			Role: driver.ShaderFill, Images: []driver.ImageSource{{Pixels: image.NewRGBA(image.Rect(0, 0, 2048, 1536)), Version: 1}},
		}})
	}
	s := surface{out: pipe, main: treeView{defaultsSent: true}}
	s.mount(&s.main, &batch)
	if _, err := pipe.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	wire := bufio.NewReader(pipe)
	const bytesPerImage = 12 << 20
	for i, want := range []struct{ items, pixels int }{{3, 2}, {1, 1}} {
		kind, payload, err := readMessage(wire)
		if err != nil {
			t.Fatalf("batch %d: %v", i, err)
		}
		r := reader{buf: payload}
		if kind != msgBatch || r.u32() != 0 || r.u32() != uint32(root) || r.byte() != 0 || r.byte() != 0 {
			t.Fatalf("batch %d has an unexpected header", i)
		}
		if n := r.u32(); n != uint32(want.items) {
			t.Fatalf("batch %d has %d items, want %d", i, n, want.items)
		}
		if n := len(payload); n < want.pixels*bytesPerImage || n >= (want.pixels+1)*bytesPerImage || n > maxMessage {
			t.Fatalf("batch %d has %d bytes, want %d images plus metadata", i, n, want.pixels)
		}
		if i > 0 && (r.byte() != byte(driver.ItemShader) || r.u32() != uint32(first+2)) {
			t.Fatal("the follow-up did not carry just the last shader")
		}
	}
	if _, err := wire.Peek(1); err != io.EOF {
		t.Fatalf("unexpected third batch, read error %v", err)
	}
	for _, it := range batch.Items {
		if held := s.main.shaderPixels[it.ID]; len(held) != 1 || held[0] != it.Shader.Images[0].Pixels {
			t.Errorf("shader %d was not recorded as sent", it.ID)
		}
	}
}

// TestShaderProgramsReleaseWithTheirLastView counts the views drawing a program on every surface: Java hears of it once the last of them is gone, deleted, drawing another program or on an overlay that closed, and a view drawing it again later sends it anew.
func TestShaderProgramsReleaseWithTheirLastView(t *testing.T) {
	pipe, err := os.Create(filepath.Join(t.TempDir(), "messages"))
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	ir, err := shader.CompileFragment([]byte(`package main

func Fragment(dst vec4, pos vec2) vec4 {
	return vec4(1)
}
`))
	if err != nil {
		t.Fatal(err)
	}
	prog, other := &driver.ShaderProgram{IR: ir}, &driver.ShaderProgram{IR: ir}
	draw := func(id driver.NodeID, p *driver.ShaderProgram) driver.MountItem {
		return driver.MountItem{Kind: driver.ItemShader, ID: id, Shader: &driver.Shader{Role: driver.ShaderFill, Program: p}}
	}
	o := &overlay{view: treeView{id: 7, defaultsSent: true}}
	s := &surface{out: pipe, main: treeView{defaultsSent: true}, overlays: map[uint32]*overlay{7: o}}
	o.s = s
	wire := bufio.NewReader(pipe)
	// released mounts b on tv, or closes the overlay when tv is nil, and returns the programs Java was told to release.
	released := func(tv *treeView, items ...driver.MountItem) []uint32 {
		t.Helper()
		at, _ := pipe.Seek(0, io.SeekCurrent)
		if tv == nil {
			s.forget(o)
		} else {
			s.mount(tv, &driver.MountBatch{Root: 1, Items: items})
		}
		if _, err := pipe.Seek(at, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		wire.Reset(pipe)
		var ids []uint32
		for {
			kind, payload, err := readMessage(wire)
			if err == io.EOF {
				return ids
			} else if err != nil {
				t.Fatal(err)
			}
			if kind != msgShaderRelease {
				continue
			}
			r := reader{buf: payload}
			for range r.u32() {
				ids = append(ids, r.u32())
			}
		}
	}
	if ids := released(&s.main, draw(2, prog), draw(3, prog)); ids != nil {
		t.Fatalf("drawing released %v", ids)
	}
	if ids := released(&o.view, draw(2, prog)); ids != nil {
		t.Fatalf("drawing on an overlay released %v", ids)
	}
	if ids := released(&s.main, driver.MountItem{Kind: driver.ItemDelete, ID: 2}, draw(3, other)); ids != nil {
		t.Fatalf("the overlay's view still draws the program, but %v was released", ids)
	}
	// A program a later item of the same batch draws again stays.
	if ids := released(&s.main, driver.MountItem{Kind: driver.ItemDelete, ID: 3}, draw(4, other)); ids != nil {
		t.Fatalf("a program drawn again in its batch was released: %v", ids)
	}
	if ids := released(nil); len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("closing the overlay released %v, want program 1", ids)
	}
	if ids := released(&s.main, driver.MountItem{Kind: driver.ItemDelete, ID: 4}); len(ids) != 1 || ids[0] != 2 {
		t.Fatalf("deleting the last view released %v, want program 2", ids)
	}
	if len(s.programs) != 0 || len(s.main.shaders) != 0 {
		t.Fatalf("Go keeps %d programs and %d views", len(s.programs), len(s.main.shaders))
	}
	released(&s.main, draw(5, prog))
	if p := s.programs[prog]; p == nil || p.id != 3 || !p.sent {
		t.Fatalf("a program drawn again is %+v, want it sent anew as program 3", p)
	}
}
