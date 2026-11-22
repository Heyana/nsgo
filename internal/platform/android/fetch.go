//go:build android

package android

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

// Images download through the runtime's HttpURLConnection, with
// Android's certificates, proxies and network policy, so the Go process
// links no HTTP and TLS stack of its own: msgFetch starts a download or
// cancels it, and msgFetched reports its progress, then its response.

// What a msgFetch asks.
const (
	fetchStart  = 0
	fetchCancel = 1
	fetchToFile = 2
)

// The parts of a msgFetched.
const (
	fetchedProgress = 0
	fetchedDone     = 1
)

// downloads are the downloads Java has yet to finish, by request.
type downloads struct {
	mu      sync.Mutex
	last    uint32
	waiting map[uint32]*download
}

type download struct {
	progress imageload.Progress
	done     chan *reader
}

// httpGet is imageload.HTTP: a msgFetch, and its answer.
func (s *surface) httpGet(ctx context.Context, url string, header imageload.Header, progress imageload.Progress) (*imageload.Response, error) {
	return s.fetch(ctx, url, "", header, progress)
}

// httpGetToFile is imageload.HTTPToFile: streams the response body to dst in the Java runtime so large update packages do not enter the Java heap or pipe. A runtime without capFetchToFile, which would misread the request, sends the whole body, which this writes to dst.
func (s *surface) httpGetToFile(ctx context.Context, url string, header imageload.Header, dst string, progress imageload.Progress) (*imageload.Response, error) {
	if s.fetchToFile.Load() {
		return s.fetch(ctx, url, dst, header, progress)
	}
	resp, err := s.fetch(ctx, url, "", header, progress)
	if err != nil {
		return nil, err
	}
	if resp.Status == 200 {
		if err := os.WriteFile(dst, resp.Body, 0o600); err != nil {
			return nil, err
		}
	}
	resp.Body = nil
	return resp, nil
}

func (s *surface) fetch(ctx context.Context, url, dst string, header imageload.Header, progress imageload.Progress) (*imageload.Response, error) {
	f := &s.downloads
	d := &download{progress: progress, done: make(chan *reader, 1)}
	f.mu.Lock()
	f.last++
	n := f.last
	if f.waiting == nil {
		f.waiting = map[uint32]*download{}
	}
	f.waiting[n] = d
	f.mu.Unlock()
	s.send(msgFetch, func(w *writer) {
		w.u32(n)
		if dst != "" {
			w.byte(fetchToFile)
			w.str(url)
			w.str(dst)
		} else {
			w.byte(fetchStart)
			w.str(url)
		}
		count := 0
		for _, vs := range header {
			count += len(vs)
		}
		w.u32(uint32(count))
		for k, vs := range header {
			for _, v := range vs {
				w.str(k)
				w.str(v)
			}
		}
	})
	select {
	case r := <-d.done:
		if msg := r.str(); msg != "" {
			return nil, errors.New("android: " + msg)
		}
		resp := &imageload.Response{Status: int(r.u32()), Header: imageload.Header{}}
		for range r.u32() {
			k := r.str()
			resp.Header.Add(k, r.str())
		}
		resp.Body = r.bytes()
		if dst == "" && progress != nil {
			total, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
			progress(int64(len(resp.Body)), max(total, 0))
		}
		return resp, nil
	case <-ctx.Done():
		f.mu.Lock()
		delete(f.waiting, n)
		f.mu.Unlock()
		s.send(msgFetch, func(w *writer) {
			w.u32(n)
			w.byte(fetchCancel)
		})
		return nil, ctx.Err()
	}
}

// fetched hands a msgFetched to the download it answers.
func (s *surface) fetched(r *reader) {
	f := &s.downloads
	n, part := r.u32(), r.byte()
	f.mu.Lock()
	d := f.waiting[n]
	if part == fetchedDone {
		delete(f.waiting, n)
	}
	f.mu.Unlock()
	switch {
	case d == nil:
	case part == fetchedProgress:
		if d.progress != nil {
			loaded, total := r.u64(), r.u64()
			d.progress(int64(loaded), int64(total))
		}
	default:
		d.done <- &reader{buf: append([]byte(nil), r.buf...)}
	}
}
