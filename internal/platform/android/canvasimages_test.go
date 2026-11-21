package android

import (
	"context"
	"fmt"
	"image"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestCanvasImagesJavaLifecycle(t *testing.T) {
	javac, java := androidJDK(t)
	dir := t.TempDir()
	if out, err := exec.CommandContext(t.Context(), javac, "-d", dir,
		filepath.Join("java", "dev", "oneo", "nsgo", "CanvasImages.java"),
		filepath.Join("testdata", "CanvasImagesCheck.java")).CombinedOutput(); err != nil {
		t.Fatalf("compile cache fixture: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.CanvasImagesCheck").CombinedOutput(); err != nil {
		t.Fatalf("cache lifecycle: %v\n%s", err, out)
	}
}

func canvasSourceBatch(id driver.NodeID, sources ...driver.ImageSource) *driver.MountBatch {
	d := &driver.Drawing{}
	for _, src := range sources {
		d.DrawImage(src, 0, 0)
	}
	return &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemCanvas, ID: id, Drawing: d}}}
}

func TestCanvasRequestsBoundedAndReloadable(t *testing.T) {
	var c canvasRequests
	defer c.closeSurface(0)
	key := func(i int) string { return driver.ImageSource{URI: fmt.Sprintf("image-%d", i)}.Key() }
	src := driver.ImageSource{URI: "image-0"}
	c.update(0, canvasSourceBatch(1, src))
	ctx, _, first, _ := c.start(0, src.Key(), 1)
	for i := 1; i < 1000; i++ {
		s := driver.ImageSource{URI: fmt.Sprintf("image-%d", i)}
		c.update(0, canvasSourceBatch(1, s))
		if _, got, load, ok := c.start(0, s.Key(), uint32(i+1)); !ok || load == nil || got.URI != s.URI {
			t.Fatalf("source %d not available", i)
		}
		if i >= maxCanvasImages {
			// Java cancels a request it no longer waits for, the oldest here.
			c.cancel(0, key(i-maxCanvasImages), uint32(i-maxCanvasImages+1))
		}
		if n := len(c.trees[0].loads); n > maxCanvasImages {
			t.Fatalf("%d loads", n)
		}
		if n := len(c.trees[0].sources); n != 1 {
			t.Fatalf("%d retained drawing sources", n)
		}
	}
	if ctx.Err() == nil || c.finish(0, src.Key(), first) {
		t.Fatal("evicted load survived")
	}
	c.update(0, canvasSourceBatch(1, src))
	ctx, got, reload, ok := c.start(0, src.Key(), 1001)
	if !ok || got.URI != src.URI || reload == first || ctx.Err() != nil {
		t.Fatal("evicted source did not reload")
	}
	if !c.finish(0, src.Key(), reload) {
		t.Fatal("reloaded source did not complete")
	}
}

// A drawing of more sources than Java's limit keeps them all pending in one pass: Go must not cancel any, or Java would wait for it forever.
func TestCanvasRequestsKeepAWideDrawingsLoads(t *testing.T) {
	var c canvasRequests
	defer c.closeSurface(0)
	sources := make([]driver.ImageSource, 2*maxCanvasImages)
	for i := range sources {
		sources[i] = driver.ImageSource{URI: fmt.Sprintf("image-%d", i)}
	}
	c.update(0, canvasSourceBatch(1, sources...))
	var ctxs []context.Context
	for i, src := range sources {
		ctx, _, load, ok := c.start(0, src.Key(), uint32(i+1))
		if !ok || load == nil {
			t.Fatalf("source %d not available", i)
		}
		ctxs = append(ctxs, ctx)
	}
	for i, ctx := range ctxs {
		if ctx.Err() != nil {
			t.Fatalf("load %d cancelled without Java asking", i)
		}
	}
}

func TestCanvasRequestsSurfaceAndGeneration(t *testing.T) {
	var c canvasRequests
	src := driver.ImageSource{Data: []byte("image")}
	for _, surface := range []uint32{0, 7} {
		c.update(surface, canvasSourceBatch(1, src))
	}
	oldCtx, _, old, _ := c.start(0, src.Key(), 1)
	ctx, _, current, _ := c.start(0, src.Key(), 2)
	otherCtx, _, other, _ := c.start(7, src.Key(), 1)
	if oldCtx.Err() == nil || c.finish(0, src.Key(), old) {
		t.Fatal("obsolete request survived replacement")
	}
	c.cancel(0, src.Key(), 1)
	if ctx.Err() != nil {
		t.Fatal("old cancellation cancelled a new request")
	}
	c.update(0, canvasSourceBatch(1, src))
	if ctx.Err() != nil {
		t.Fatal("unchanged drawing cancelled its load")
	}
	if _, _, duplicate, known := c.start(0, src.Key(), 2); !known || duplicate != nil {
		t.Fatal("duplicate load started")
	}
	c.closeSurface(0)
	if ctx.Err() == nil || c.finish(0, src.Key(), current) {
		t.Fatal("closed surface's load survived")
	}
	if otherCtx.Err() != nil || !c.finish(7, src.Key(), other) {
		t.Fatal("closing one surface affected another")
	}
	c.closeSurface(7)
	if _, _, l, known := c.start(7, src.Key(), 3); known || l != nil {
		t.Fatal("closed surface resurrected")
	}
}

func TestCanvasSourcesFollowNodesAndShaders(t *testing.T) {
	var c canvasRequests
	src := driver.ImageSource{URI: "shared"}
	c.update(0, canvasSourceBatch(1, src, src))
	c.update(0, &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemShader, ID: 2, Shader: &driver.Shader{Images: []driver.ImageSource{src}}}}})
	c.update(0, &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemDelete, ID: 1}}})
	if len(c.trees[0].sources) != 1 {
		t.Fatal("deleting canvas dropped shader's source")
	}
	c.update(0, &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemCreate, ID: 2}}})
	if len(c.trees[0].sources) != 0 || len(c.trees[0].nodes) != 0 {
		t.Fatal("reused node retained old drawing sources")
	}
	if _, _, l, known := c.start(0, src.Key(), 1); known || l != nil {
		t.Fatal("unmounted source started loading")
	}
	c.closeSurface(0)
}

func TestCanvasRequestsProcessCloseStopsEverySurface(t *testing.T) {
	var c canvasRequests
	src := driver.ImageSource{URI: "image"}
	c.update(0, canvasSourceBatch(1, src))
	c.update(7, canvasSourceBatch(1, src))
	main, _, _, _ := c.start(0, src.Key(), 1)
	overlay, _, _, _ := c.start(7, src.Key(), 1)
	c.close()
	if main.Err() == nil || overlay.Err() == nil || len(c.trees) != 0 {
		t.Fatal("process exit retained a surface's load")
	}
	c.update(7, canvasSourceBatch(1, src))
	if len(c.trees) != 0 {
		t.Fatal("a late mount revived a closed process")
	}
}

func TestCanvasRequestsLeavePixelsOut(t *testing.T) {
	var c canvasRequests
	defer c.closeSurface(0)
	px := driver.ImageSource{Pixels: image.NewRGBA(image.Rect(0, 0, 2, 2)), Version: 1}
	url := driver.ImageSource{URI: "https://x/a.png"}
	c.update(0, &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemShader, ID: 1,
		Shader: &driver.Shader{Images: []driver.ImageSource{px, url}}}}})
	c.update(0, canvasSourceBatch(2, px))
	if _, _, _, ok := c.start(0, px.Key(), 1); ok {
		t.Fatal("pixels are a source Java requests; they go with the mount")
	}
	if _, got, load, ok := c.start(0, url.Key(), 2); !ok || load == nil || got.URI != url.URI {
		t.Fatalf("the shader's URL is not available: %v %+v", ok, got)
	}
}
