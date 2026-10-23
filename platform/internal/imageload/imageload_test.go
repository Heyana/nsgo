package imageload

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/core"
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

func TestInfoKeepsTheBytesForViews(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte("abc"))
	}))
	defer srv.Close()
	it := &core.MountItem{Kind: core.ItemImageInfo, Count: 7, Image: &core.ImageConfig{Source: core.Src(srv.URL + "/b.png")}}
	ev := Info(it, func(data []byte) (core.Size, error) { return core.Size{Width: core.Points(len(data)), Height: 1}, nil })
	if ev.Kind != core.EventImageInfo || ev.Count != 7 || ev.Size != (core.Size{Width: 3, Height: 1}) || ev.Text != "" {
		t.Fatalf("Info = %+v", ev)
	}
	if data, err := Load(context.Background(), it.Image.Source, nil); err != nil || string(data) != "abc" || hits.Load() != 1 {
		t.Fatalf("a view's load after Info = %q, %v, after %d fetches", data, err, hits.Load())
	}
	if ev := Info(&core.MountItem{Count: 8, Image: &core.ImageConfig{}}, nil); ev.Count != 8 || ev.Text == "" {
		t.Fatalf("Info of no source = %+v, want an error", ev)
	}
}
