package android

import (
	"context"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// maxCanvasImages is the bound of Java's cache (CanvasImages.LIMIT), which loads under way and a pass of drawing more sources exceed until later passes draw others.
const maxCanvasImages = 64

// canvasRequests keeps sources only while a mounted drawing uses them. Java owns the bounded decoded cache and asks again on a miss, never evicts a pending entry and cancels the loads of a cache it closes, so the loads under way are the pending entries of Java's cache; Go retains no sent-key set. Mounts and requests arrive on different goroutines.
type canvasRequests struct {
	mu     sync.Mutex
	trees  map[uint32]*canvasSources
	closed bool
}

type canvasSources struct {
	nodes   map[driver.NodeID][]string
	sources map[string]*canvasSource
	loads   map[string]*canvasLoad
}

type canvasSource struct {
	source driver.ImageSource
	refs   int
}

type canvasLoad struct {
	request uint32
	cancel  context.CancelFunc
}

func (c *canvasRequests) update(surface uint32, b *driver.MountBatch) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if c.trees == nil {
		c.trees = map[uint32]*canvasSources{}
	}
	t := c.trees[surface]
	if t == nil {
		t = &canvasSources{nodes: map[driver.NodeID][]string{}, sources: map[string]*canvasSource{}, loads: map[string]*canvasLoad{}}
		c.trees[surface] = t
	}
	for _, it := range b.Items {
		switch it.Kind {
		case driver.ItemCreate, driver.ItemDelete, driver.ItemCanvas, driver.ItemShader:
		default:
			continue
		}
		var sources []driver.ImageSource
		if it.Kind == driver.ItemCanvas && it.Drawing != nil {
			for _, op := range it.Drawing.Ops {
				if (op.Kind == driver.DrawImage || op.Kind == driver.DrawVertices) && op.Image != nil && op.Image.Pixels == nil {
					sources = append(sources, *op.Image)
				}
			}
		}
		if it.Kind == driver.ItemShader && it.Shader != nil {
			// Pixels go with the item, never through Java's requests.
			for _, src := range it.Shader.Images {
				if src.Pixels == nil {
					sources = append(sources, src)
				}
			}
		}
		// Add before removing: unchanged sources keep their in-flight load.
		keys := make([]string, 0, len(sources))
		for _, src := range sources {
			key := src.Key()
			s := t.sources[key]
			if s == nil {
				s = &canvasSource{source: src}
				t.sources[key] = s
			}
			s.refs++
			keys = append(keys, key)
		}
		for _, key := range t.nodes[it.ID] {
			s := t.sources[key]
			s.refs--
			if s.refs == 0 {
				delete(t.sources, key)
			}
		}
		delete(t.nodes, it.ID)
		if len(keys) != 0 {
			t.nodes[it.ID] = keys
		}
	}
}

func (c *canvasRequests) start(surface uint32, key string, request uint32) (context.Context, driver.ImageSource, *canvasLoad, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.trees[surface]
	if t == nil || t.sources[key] == nil {
		return nil, driver.ImageSource{}, nil, false
	}
	if old := t.loads[key]; old != nil {
		if old.request == request {
			return nil, driver.ImageSource{}, nil, true
		}
		old.cancel()
		delete(t.loads, key)
	}
	// No bound of Go's own: cancelling a load Java still waits for would leave its entry pending, never drawn.
	ctx, cancel := context.WithCancel(context.Background())
	l := &canvasLoad{request: request, cancel: cancel}
	t.loads[key] = l
	return ctx, t.sources[key].source, l, true
}

func (c *canvasRequests) cancel(surface uint32, key string, request uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.trees[surface]; t != nil {
		if l := t.loads[key]; l != nil && l.request == request {
			l.cancel()
			delete(t.loads, key)
		}
	}
}

func (c *canvasRequests) finish(surface uint32, key string, l *canvasLoad) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.trees[surface]; t != nil && t.loads[key] == l {
		delete(t.loads, key)
		l.cancel()
		return true
	}
	return false
}

func (c *canvasRequests) closeSurface(surface uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.trees[surface]; t != nil {
		for _, l := range t.loads {
			l.cancel()
		}
		delete(c.trees, surface)
	}
}

func (c *canvasRequests) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for _, t := range c.trees {
		for _, l := range t.loads {
			l.cancel()
		}
	}
	clear(c.trees)
}
