// Package imageload fetches image bytes for the backends that decode
// images themselves: https and http, file paths and file: URIs, data:
// URIs, "asset:" paths under the app's assets and bytes the app holds.
// Requests for the same URI share one fetch, and the bytes stay in a
// least-recently-used cache.
package imageload

import (
	"container/list"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/oneo-me/nsgo/core"
)

// AssetRoot is the directory "asset:" paths resolve against; next to
// the executable, in "assets", unless a backend sets it first.
var AssetRoot = func() string {
	exe, err := os.Executable()
	if err != nil {
		return "assets"
	}
	return filepath.Join(filepath.Dir(exe), "assets")
}()

// CacheBytes bounds the cache.
const CacheBytes = 32 << 20

// Progress receives bytes loaded and the total, 0 when unknown.
type Progress func(loaded, total int64)

var (
	mu       sync.Mutex
	inflight = map[string]*call{}
	cache    = newLRU(CacheBytes)
)

type call struct {
	done     chan struct{}
	data     []byte
	err      error
	waiters  int
	cancel   context.CancelFunc
	progress []Progress
}

// Load returns src's bytes. It blocks; call it on a goroutine of its own.
// Cancelling ctx stops waiting, and stops the fetch once nobody waits.
func Load(ctx context.Context, src core.ImageSource, progress Progress) ([]byte, error) {
	if len(src.Data) > 0 {
		return src.Data, nil
	}
	uri := src.URI
	if uri == "" {
		return nil, errors.New("imageload: empty source")
	}
	mu.Lock()
	if data, ok := cache.get(uri); ok {
		mu.Unlock()
		return data, nil
	}
	c := inflight[uri]
	if c == nil {
		fetchCtx, cancel := context.WithCancel(context.Background())
		c = &call{done: make(chan struct{}), cancel: cancel}
		inflight[uri] = c
		go c.fetch(fetchCtx, uri)
	}
	c.waiters++
	if progress != nil {
		c.progress = append(c.progress, progress)
	}
	mu.Unlock()

	select {
	case <-c.done:
		return c.data, c.err
	case <-ctx.Done():
		mu.Lock()
		if c.waiters--; c.waiters == 0 {
			c.cancel()
		}
		mu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *call) fetch(ctx context.Context, uri string) {
	data, err := fetch(ctx, uri, c.report)
	mu.Lock()
	delete(inflight, uri)
	if err == nil {
		cache.put(uri, data)
	}
	c.data, c.err = data, err
	mu.Unlock()
	close(c.done)
}

func (c *call) report(loaded, total int64) {
	mu.Lock()
	ps := append([]Progress(nil), c.progress...)
	mu.Unlock()
	for _, p := range ps {
		p(loaded, total)
	}
}

func fetch(ctx context.Context, uri string, progress Progress) ([]byte, error) {
	switch {
	case strings.HasPrefix(uri, "data:"):
		return decodeDataURI(uri)
	case strings.HasPrefix(uri, "asset:"):
		return os.ReadFile(filepath.Join(AssetRoot, filepath.FromSlash(strings.TrimLeft(uri[len("asset:"):], "/"))))
	case strings.HasPrefix(uri, "file:"):
		u, err := url.Parse(uri)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(filepath.FromSlash(u.Path))
	case strings.HasPrefix(uri, "http://"), strings.HasPrefix(uri, "https://"):
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("imageload: %s: %s", uri, resp.Status)
		}
		return readAll(resp.Body, resp.ContentLength, progress)
	}
	return os.ReadFile(uri)
}

func readAll(r io.Reader, total int64, progress Progress) ([]byte, error) {
	buf := make([]byte, 0, max(total, 32<<10))
	for {
		if len(buf) == cap(buf) {
			buf = append(buf, 0)[:len(buf)]
		}
		n, err := r.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]
		if n > 0 && progress != nil {
			progress(int64(len(buf)), max(total, 0))
		}
		if err == io.EOF {
			return buf, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func decodeDataURI(uri string) ([]byte, error) {
	meta, payload, ok := strings.Cut(uri[len("data:"):], ",")
	if !ok {
		return nil, errors.New("imageload: malformed data URI")
	}
	if strings.HasSuffix(meta, ";base64") {
		return base64.StdEncoding.DecodeString(payload)
	}
	s, err := url.PathUnescape(payload)
	return []byte(s), err
}

// lru is a byte-bounded least-recently-used cache.
type lru struct {
	limit, size int
	order       *list.List
	items       map[string]*list.Element
}

type entry struct {
	key  string
	data []byte
}

func newLRU(limit int) *lru {
	return &lru{limit: limit, order: list.New(), items: map[string]*list.Element{}}
}

func (c *lru) get(key string) ([]byte, bool) {
	if e, ok := c.items[key]; ok {
		c.order.MoveToFront(e)
		return e.Value.(*entry).data, true
	}
	return nil, false
}

func (c *lru) put(key string, data []byte) {
	if len(data) > c.limit {
		return
	}
	if e, ok := c.items[key]; ok {
		c.size -= len(e.Value.(*entry).data)
		c.order.Remove(e)
	}
	c.items[key] = c.order.PushFront(&entry{key, data})
	c.size += len(data)
	for c.size > c.limit {
		last := c.order.Back()
		ent := last.Value.(*entry)
		c.order.Remove(last)
		delete(c.items, ent.key)
		c.size -= len(ent.data)
	}
}
