package imageload

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestConcurrentLoadsShareOneFetch(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-release
		w.Write([]byte("png-bytes"))
	}))
	defer srv.Close()
	src := core.Src(srv.URL + "/a.png")
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if data, err := Load(context.Background(), src, nil); err != nil || string(data) != "png-bytes" {
				t.Errorf("Load = %q, %v", data, err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if hits.Load() != 1 {
		t.Fatalf("fetched %d times, want once", hits.Load())
	}
	if _, err := Load(context.Background(), src, nil); err != nil || hits.Load() != 1 {
		t.Fatalf("a cached load fetched again (%d) or failed: %v", hits.Load(), err)
	}
}

func TestDataURIAndCancel(t *testing.T) {
	data, err := Load(context.Background(), core.Src("data:image/png;base64,aGk="), nil)
	if err != nil || string(data) != "hi" {
		t.Fatalf("data URI = %q, %v", data, err)
	}
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(block)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := Load(ctx, core.Src(srv.URL+"/slow.png"), nil); err == nil {
		t.Fatal("a cancelled load succeeded")
	}
}

// A load right after the only waiter of a fetch cancelled it, as a canvas cache evicting and drawing a source again does, fetches anew instead of joining the cancelled fetch, and the cancelled fetch, returning later, leaves the new one in flight.
func TestLoadAfterCancelFetchesAnew(t *testing.T) {
	saved := HTTP
	defer func() { HTTP = saved }()
	var hits atomic.Int32
	first, second := make(chan struct{}), make(chan struct{})
	HTTP = func(ctx context.Context, url string, header Header, progress Progress) (*Response, error) {
		switch hits.Add(1) {
		case 1:
			<-first
			return nil, ctx.Err()
		default:
			<-second
			return &Response{Status: 200, Body: []byte("png-bytes")}, nil
		}
	}
	src := core.Src("https://example.invalid/load-after-cancel.png")
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatal(what)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancelled := make(chan error, 1)
	go func() {
		_, err := Load(ctx, src, nil)
		cancelled <- err
	}()
	waitFor("the first fetch did not start", func() bool { return hits.Load() == 1 })
	mu.Lock()
	old := inflight[src.URI]
	mu.Unlock()
	cancel()
	if err := <-cancelled; err != context.Canceled {
		t.Fatalf("cancelled Load = %v", err)
	}
	type result struct {
		data []byte
		err  error
	}
	results := make(chan result, 2)
	load := func() {
		data, err := Load(context.Background(), src, nil)
		results <- result{data, err}
	}
	go load()
	waitFor("a load after a cancel joined the cancelled fetch", func() bool { return hits.Load() == 2 })
	close(first)
	<-old.done
	mu.Lock()
	current := inflight[src.URI]
	mu.Unlock()
	if current == nil || current == old {
		t.Fatal("the cancelled fetch's end dropped the new fetch")
	}
	go load()
	time.Sleep(20 * time.Millisecond)
	close(second)
	for range 2 {
		if r := <-results; r.err != nil || string(r.data) != "png-bytes" {
			t.Fatalf("Load = %q, %v", r.data, r.err)
		}
	}
	if n := hits.Load(); n != 2 {
		t.Fatalf("fetched %d times, want 2", n)
	}
}

func TestInfoKeepsTheBytesForViews(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte("abc"))
	}))
	defer srv.Close()
	it := &driver.MountItem{Kind: driver.ItemImageInfo, Count: 7, Image: &driver.ImageConfig{Source: core.Src(srv.URL + "/b.png")}}
	ev := Info(it, func(data []byte) (driver.Size, error) {
		return driver.Size{Width: driver.Points(len(data)), Height: 1}, nil
	})
	if ev.Kind != driver.EventImageInfo || ev.Count != 7 || ev.Size != (driver.Size{Width: 3, Height: 1}) || ev.Text != "" {
		t.Fatalf("Info = %+v", ev)
	}
	if data, err := Load(context.Background(), it.Image.Source, nil); err != nil || string(data) != "abc" || hits.Load() != 1 {
		t.Fatalf("a view's load after Info = %q, %v, after %d fetches", data, err, hits.Load())
	}
	if ev := Info(&driver.MountItem{Count: 8, Image: &driver.ImageConfig{}}, nil); ev.Count != 8 || ev.Text == "" {
		t.Fatalf("Info of no source = %+v, want an error", ev)
	}
}

func isolatedCache(t *testing.T) {
	t.Helper()
	mu.Lock()
	saved := cache
	cache = newLRU(driver.DefaultImageCacheLimit)
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		cache = saved
		mu.Unlock()
	})
}

func TestCacheBudgetLRUKeepAndClamping(t *testing.T) {
	isolatedCache(t)
	savedHTTP := HTTP
	defer func() { HTTP = savedHTTP }()

	fetches := map[string]int{}
	var fetchMu sync.Mutex
	payloads := map[string][]byte{
		"https://example.invalid/a.png": make([]byte, 100),
		"https://example.invalid/b.png": make([]byte, 100),
		"https://example.invalid/c.png": make([]byte, 100),
		"https://example.invalid/big":   make([]byte, 300),
	}
	HTTP = func(ctx context.Context, url string, header Header, progress Progress) (*Response, error) {
		fetchMu.Lock()
		fetches[url]++
		fetchMu.Unlock()
		return &Response{Status: 200, Body: payloads[url]}, nil
	}
	load := func(u string) {
		t.Helper()
		if _, err := Load(context.Background(), core.Src(u), nil); err != nil {
			t.Fatalf("Load(%s): %v", u, err)
		}
	}

	if got := CacheLimit(); got != driver.DefaultImageCacheLimit {
		t.Fatalf("default CacheLimit = %d, want %d", got, driver.DefaultImageCacheLimit)
	}

	// Budget 250 bytes: holds 2 x 100B images, evicts LRU on the 3rd.
	SetCacheLimit(250)
	load("https://example.invalid/a.png")
	load("https://example.invalid/b.png")
	// Touch a.png so b.png becomes LRU.
	load("https://example.invalid/a.png")
	if fetches["https://example.invalid/a.png"] != 1 {
		t.Fatalf("a.png fetched %d times, want 1", fetches["https://example.invalid/a.png"])
	}
	load("https://example.invalid/c.png")
	if got := UnkeptCacheSize(); got != 200 {
		t.Fatalf("UnkeptCacheSize = %d, want 200", got)
	}
	// a.png and c.png should be cached; b.png should have been evicted.
	load("https://example.invalid/a.png")
	load("https://example.invalid/c.png")
	if fetches["https://example.invalid/a.png"] != 1 || fetches["https://example.invalid/c.png"] != 1 {
		t.Fatalf("MRU entries were evicted: fetches=%v", fetches)
	}
	load("https://example.invalid/b.png")
	if fetches["https://example.invalid/b.png"] != 2 {
		t.Fatalf("LRU entry b.png was not evicted: fetches=%v", fetches)
	}

	// Unkept entry larger than budget is not cached.
	load("https://example.invalid/big")
	load("https://example.invalid/big")
	if fetches["https://example.invalid/big"] != 2 {
		t.Fatalf("oversized unkept entry was cached: fetches=%v", fetches)
	}

	// Keep pins entries out of budget even when larger than the limit.
	ownerA, ownerB := new(int), new(int)
	Keep(ownerA, "https://example.invalid/big")
	Keep(ownerB, "https://example.invalid/big")
	load("https://example.invalid/big")
	load("https://example.invalid/big")
	if fetches["https://example.invalid/big"] != 3 {
		t.Fatalf("kept big entry was not cached: fetches=%v", fetches)
	}
	if CacheSize() != UnkeptCacheSize()+300 {
		t.Fatalf("CacheSize=%d UnkeptCacheSize=%d, want diff 300", CacheSize(), UnkeptCacheSize())
	}

	// Lowering the budget immediately evicts unkept entries down to the new budget while keeping pinned entries.
	SetCacheLimit(100)
	if UnkeptCacheSize() != 100 || CacheSize() != 400 {
		t.Fatalf("after shrinking to 100: unkept=%d total=%d", UnkeptCacheSize(), CacheSize())
	}

	// Negative budget clamps to 0, evicting all unkept entries while keeping pinned entries.
	SetCacheLimit(-42)
	if CacheLimit() != 0 {
		t.Fatalf("negative limit = %d, want 0", CacheLimit())
	}
	if UnkeptCacheSize() != 0 || CacheSize() != 300 {
		t.Fatalf("after zero limit: unkept=%d total=%d", UnkeptCacheSize(), CacheSize())
	}

	// Zero budget prevents caching new unkept entries.
	beforeA := fetches["https://example.invalid/a.png"]
	load("https://example.invalid/a.png")
	load("https://example.invalid/a.png")
	if fetches["https://example.invalid/a.png"] != beforeA+2 {
		t.Fatalf("zero budget cached unkept entry: fetches=%v", fetches)
	}

	// Releasing ownerA leaves ownerB's pin intact; releasing ownerB trims big immediately.
	ReleaseOwner(ownerA)
	if CacheSize() != 300 {
		t.Fatalf("big evicted while ownerB still kept it: total=%d", CacheSize())
	}
	Release(ownerB, "https://example.invalid/big")
	if CacheSize() != 0 || UnkeptCacheSize() != 0 {
		t.Fatalf("released big not trimmed at limit 0: total=%d unkept=%d", CacheSize(), UnkeptCacheSize())
	}
}
