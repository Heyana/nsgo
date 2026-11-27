// Package imageload fetches image bytes for the native backends: https
// and http, file paths and file: URIs, data: URIs, "asset:" paths under
// the app's resources and bytes the app holds. Requests for the same URI
// share one fetch, the bytes stay in a least-recently-used cache in
// memory, and downloads in a disk cache under HTTP's caching rules
// (UseDiskCache).
package imageload

import (
	"container/list"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ResourceRoot is the directory "asset:" paths resolve against: the
// package's resource root, Contents/Resources in a macOS bundle and the
// executable's directory elsewhere, where Windows and Linux packages put
// their resources. Android reads them from its APK instead.
var ResourceRoot = func() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(filepath.Dir(exe), "..", "Resources")
	}
	return filepath.Dir(exe)
}()

// ReadResource reads one of the app's resources by its slash-separated
// path, as "asset:" sources do.
func ReadResource(path string) ([]byte, error) { return readResource(path) }

// ReadAsset reads the app's bundled file from its native resource directory.
func ReadAsset(ctx context.Context, path string) ([]byte, error) {
	return driver.ReadAssetFile(ctx, ResourceRoot, path)
}

// readResource reads one of the app's resources by its slash-separated
// path.
func readResource(path string) ([]byte, error) {
	if runtime.GOOS == "android" {
		return readAPKAsset(path)
	}
	return os.ReadFile(filepath.Join(ResourceRoot, filepath.FromSlash(path)))
}

// CacheBytes is the default byte budget of the in-memory encoded image cache.
const CacheBytes = int(driver.DefaultImageCacheLimit)

// Progress receives bytes loaded and the total, 0 when unknown.
type Progress func(loaded, total int64)

var (
	mu       sync.Mutex
	inflight = map[string]*call{}
	cache    = newLRU(driver.DefaultImageCacheLimit)
)

// SetCacheLimit sets the maximum number of unkept encoded image bytes kept in memory. Negative values are clamped to zero. Excess unkept entries are evicted immediately in least-recently-used order while kept entries remain pinned until released.
func SetCacheLimit(maxBytes int64) {
	mu.Lock()
	cache.setLimit(maxBytes)
	mu.Unlock()
}

// CacheLimit returns the current in-memory encoded image byte budget.
func CacheLimit() int64 {
	mu.Lock()
	defer mu.Unlock()
	return cache.limit
}

// CacheSize returns the total encoded image bytes currently cached in memory, including kept entries.
func CacheSize() int64 {
	mu.Lock()
	defer mu.Unlock()
	return cache.totalBytes
}

// UnkeptCacheSize returns the unkept encoded image bytes currently cached in memory.
func UnkeptCacheSize() int64 {
	mu.Lock()
	defer mu.Unlock()
	return cache.unkeptBytes
}

// Keep pins key in the in-memory encoded image cache on behalf of owner so it is not evicted while kept. Passing nil owner tracks an unowned pin.
func Keep(owner any, key string) {
	if key == "" {
		return
	}
	mu.Lock()
	cache.keep(owner, key)
	mu.Unlock()
}

// Release releases one pin of key held by owner, immediately trimming excess unkept entries once key is no longer pinned.
func Release(owner any, key string) {
	if key == "" {
		return
	}
	mu.Lock()
	cache.release(owner, key)
	mu.Unlock()
}

// ReleaseOwner releases every pin held by owner (for example when a surface closes) and trims any newly unkept excess entries.
func ReleaseOwner(owner any) {
	if owner == nil {
		return
	}
	mu.Lock()
	cache.releaseOwner(owner)
	mu.Unlock()
}

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
// When core attached a source check to src (for a restricted scope), every
// HTTP redirect hop is validated before it is followed, cached entries are
// served only after every recorded hop passes the check, and local files are
// re-verified right before reading.
func Load(ctx context.Context, src driver.ImageSource, progress Progress) ([]byte, error) {
	if len(src.Data) > 0 {
		return src.Data, nil
	}
	uri := src.URI
	if uri == "" {
		return nil, errors.New("imageload: empty source")
	}
	if check := driver.ImageSourceCheck(src); check != nil {
		return loadChecked(ctx, uri, check, progress)
	}
	mu.Lock()
	if ent, ok := cache.get(uri); ok {
		mu.Unlock()
		return ent.data, nil
	}
	c := inflight[uri]
	if c == nil || c.waiters == 0 {
		// A call nobody waits for is cancelled, its fetch only not yet returned: joining it would fail with its cancellation.
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

const maxCheckedRedirects = 10

func loadChecked(ctx context.Context, uri string, check func(string) error, progress Progress) ([]byte, error) {
	switch {
	case strings.HasPrefix(uri, "data:"):
		return decodeDataURI(uri)
	case strings.HasPrefix(uri, "asset:"):
		return readResource(strings.TrimLeft(uri[len("asset:"):], "/"))
	case driver.HasSchemePrefix(uri, "http"), driver.HasSchemePrefix(uri, "https"):
		canon, err := driver.CanonicalHTTPURL(uri)
		if err != nil {
			return nil, err
		}
		if err := check(canon); err != nil {
			return nil, err
		}
		mu.Lock()
		ent, ok := cache.get(canon)
		mu.Unlock()
		if ok && ent.hopsVerified {
			for _, hop := range ent.hops {
				if err := check(hop); err != nil {
					return nil, err
				}
			}
			return ent.data, nil
		}
		data, hops, err := fetchCheckedHTTP(ctx, canon, check, progress)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		cache.put(canon, data, hops, true)
		mu.Unlock()
		return data, nil
	default:
		// Local file (file: URL or bare path): re-resolve symlinks and verify
		// against fs.read right before reading to prevent symlink-swap TOCTOU.
		win := runtime.GOOS == "windows"
		canonPath, err := driver.ResolveLocalMediaPath(uri, win)
		if err != nil {
			return nil, check(uri)
		}
		canonURL := driver.CanonicalFileURL(canonPath, win)
		if err := check(canonURL); err != nil {
			return nil, err
		}
		return os.ReadFile(canonPath)
	}
}

func fetchCheckedHTTP(ctx context.Context, startURL string, check func(string) error, progress Progress) ([]byte, []string, error) {
	cur := startURL
	hops := []string{cur}
	for range maxCheckedRedirects + 1 {
		resp, err := getNoRedirect(ctx, cur, Header{}, progress)
		if err != nil {
			return nil, nil, err
		}
		switch resp.Status {
		case 200, 203:
			return resp.Body, hops, nil
		case 301, 302, 303, 307, 308:
			if len(hops) > maxCheckedRedirects {
				return nil, nil, fmt.Errorf("imageload: %s: too many redirects", startURL)
			}
			loc := resp.Header.Get("Location")
			if loc == "" {
				return nil, nil, fmt.Errorf("imageload: %s: redirect without Location", cur)
			}
			next, err := resolveCheckedRedirect(cur, loc)
			if err != nil {
				if checkErr := check(loc); checkErr != nil {
					return nil, nil, checkErr
				}
				return nil, nil, err
			}
			if err := check(next); err != nil {
				return nil, nil, err
			}
			hops = append(hops, next)
			cur = next
		default:
			return nil, nil, fmt.Errorf("imageload: %s: status %d", cur, resp.Status)
		}
	}
	return nil, nil, fmt.Errorf("imageload: %s: too many redirects", startURL)
}

func resolveCheckedRedirect(baseRaw, loc string) (string, error) {
	if strings.ContainsAny(loc, "\r\n\x00") {
		return "", errors.New("imageload: redirect Location contains control characters")
	}
	beforeQuery := loc
	if i := strings.IndexAny(beforeQuery, "?#"); i >= 0 {
		beforeQuery = beforeQuery[:i]
	}
	if strings.ContainsRune(beforeQuery, '\\') {
		return "", errors.New("imageload: redirect Location contains backslash")
	}
	base, err := url.Parse(baseRaw)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(loc)
	if err != nil {
		return "", err
	}
	resolved := base.ResolveReference(ref)
	return driver.CanonicalHTTPURL(resolved.String())
}

// Info answers an ItemImageInfo: it loads the item's source, which keeps
// its bytes in the cache for the views that show it next, and returns the
// EventImageInfo with the size read by size from the bytes, or the error.
// It blocks; call it on a goroutine of its own.
func Info(it *driver.MountItem, size func(data []byte) (driver.Size, error)) driver.Event {
	ev := driver.Event{Kind: driver.EventImageInfo, Count: it.Count}
	var src driver.ImageSource
	if it.Image != nil {
		src = it.Image.Source
	}
	data, err := Load(context.Background(), src, nil)
	if err == nil {
		ev.Size, err = size(data)
	}
	if err != nil {
		ev.Text = err.Error()
	}
	return ev
}

func (c *call) fetch(ctx context.Context, uri string) {
	data, err := fetch(ctx, uri, c.report)
	mu.Lock()
	if inflight[uri] == c {
		delete(inflight, uri)
	}
	if err == nil {
		cache.put(uri, data, nil, false)
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
		return readResource(strings.TrimLeft(uri[len("asset:"):], "/"))
	case strings.HasPrefix(uri, "file:"):
		u, err := url.Parse(uri)
		if err != nil {
			return nil, err
		}
		path, err := driver.FileURLPath(u, runtime.GOOS == "windows")
		if err != nil {
			return nil, fmt.Errorf("imageload: %s: %w", uri, err)
		}
		return os.ReadFile(path)
	case strings.HasPrefix(uri, "http://"), strings.HasPrefix(uri, "https://"):
		if c := disk.Load(); c != nil {
			return c.fetch(ctx, uri, progress)
		}
		resp, err := get(ctx, uri, Header{}, progress)
		if err != nil {
			return nil, err
		}
		if resp.Status != 200 {
			return nil, fmt.Errorf("imageload: %s: status %d", uri, resp.Status)
		}
		return resp.Body, nil
	}
	return os.ReadFile(uri)
}

// ReadAll reads r to its end, total bytes long or 0 when unknown,
// calling progress as it goes: what an HTTP client reads a body with.
func ReadAll(r io.Reader, total int64, progress Progress) ([]byte, error) {
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

// Copy streams r into w to its end, total bytes long or 0 when unknown,
// calling progress as bytes are written: what a streaming HTTP client writes a
// body to disk with.
func Copy(w io.Writer, r io.Reader, total int64, progress Progress) (int64, error) {
	buf := make([]byte, 32<<10)
	var loaded int64
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return loaded, werr
			}
			loaded += int64(n)
			if progress != nil {
				progress(loaded, max(total, 0))
			}
		}
		if err == io.EOF {
			return loaded, nil
		}
		if err != nil {
			return loaded, err
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

// lru is a byte-bounded least-recently-used cache whose pinned (kept) entries stay out of eviction until released.
type lru struct {
	limit       int64
	totalBytes  int64
	unkeptBytes int64
	order       *list.List // front = most recently used, back = least recently used
	items       map[string]*list.Element
	kept        map[string]int
	keptByOwner map[any]map[string]int
}

type entry struct {
	key          string
	data         []byte
	hops         []string
	hopsVerified bool
}

func newLRU(limit int64) *lru {
	if limit < 0 {
		limit = 0
	}
	return &lru{
		limit:       limit,
		order:       list.New(),
		items:       map[string]*list.Element{},
		kept:        map[string]int{},
		keptByOwner: map[any]map[string]int{},
	}
}

func (c *lru) get(key string) (*entry, bool) {
	if e, ok := c.items[key]; ok {
		c.order.MoveToFront(e)
		return e.Value.(*entry), true
	}
	return nil, false
}

func (c *lru) put(key string, data []byte, hops []string, hopsVerified bool) {
	n := int64(len(data))
	isKept := c.kept[key] > 0
	if e, ok := c.items[key]; ok {
		old := int64(len(e.Value.(*entry).data))
		c.totalBytes -= old
		if !isKept {
			c.unkeptBytes -= old
		}
		c.order.Remove(e)
		delete(c.items, key)
	}
	if !isKept && (c.limit == 0 || n > c.limit) {
		c.evict()
		return
	}
	c.items[key] = c.order.PushFront(&entry{key: key, data: data, hops: hops, hopsVerified: hopsVerified})
	c.totalBytes += n
	if !isKept {
		c.unkeptBytes += n
	}
	c.evict()
}

func (c *lru) setLimit(maxBytes int64) {
	if maxBytes < 0 {
		maxBytes = 0
	}
	c.limit = maxBytes
	c.evict()
}

func (c *lru) keep(owner any, key string) {
	wasKept := c.kept[key] > 0
	c.kept[key]++
	if owner != nil {
		m := c.keptByOwner[owner]
		if m == nil {
			m = map[string]int{}
			c.keptByOwner[owner] = m
		}
		m[key]++
	}
	if !wasKept {
		if e, ok := c.items[key]; ok {
			c.unkeptBytes -= int64(len(e.Value.(*entry).data))
		}
	}
}

func (c *lru) release(owner any, key string) {
	if owner != nil {
		m := c.keptByOwner[owner]
		if m == nil || m[key] == 0 {
			return
		}
		if m[key]--; m[key] == 0 {
			delete(m, key)
			if len(m) == 0 {
				delete(c.keptByOwner, owner)
			}
		}
	} else if c.kept[key] == 0 {
		return
	}
	if c.kept[key] <= 1 {
		delete(c.kept, key)
		if e, ok := c.items[key]; ok {
			c.unkeptBytes += int64(len(e.Value.(*entry).data))
		}
		c.evict()
	} else {
		c.kept[key]--
	}
}

func (c *lru) releaseOwner(owner any) {
	m := c.keptByOwner[owner]
	if len(m) == 0 {
		return
	}
	delete(c.keptByOwner, owner)
	for key, n := range m {
		if c.kept[key] <= n {
			delete(c.kept, key)
			if e, ok := c.items[key]; ok {
				c.unkeptBytes += int64(len(e.Value.(*entry).data))
			}
		} else {
			c.kept[key] -= n
		}
	}
	c.evict()
}

func (c *lru) evict() {
	for e := c.order.Back(); e != nil && (c.unkeptBytes > c.limit || c.limit == 0); {
		prev := e.Prev()
		ent := e.Value.(*entry)
		if c.kept[ent.key] == 0 {
			n := int64(len(ent.data))
			c.order.Remove(e)
			delete(c.items, ent.key)
			c.totalBytes -= n
			c.unkeptBytes -= n
		}
		e = prev
	}
}
