package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type devProfileOptions struct {
	kind, output string
	duration     time.Duration
}

func (o devProfileOptions) forProject(p *project) (devProfileOptions, error) {
	names := map[string]string{"cpu": "cpu.pprof", "heap": "heap.pprof", "trace": "execution.trace"}
	if o.kind == "" {
		if o.output != "" {
			return o, errors.New("-profile-output needs -profile")
		}
		return o, nil
	}
	name, ok := names[o.kind]
	if !ok {
		return o, fmt.Errorf("-profile %q is invalid, use cpu, heap or trace", o.kind)
	}
	if o.kind == "heap" {
		o.duration = 0
	} else if o.duration <= 0 {
		return o, errors.New("-profile-duration must be positive")
	}
	if o.kind == "cpu" && p.Dev.Target == "web" {
		return o, errors.New("Go's browser runtime has no CPU sampling, use -profile heap or trace")
	}
	if o.output == "" {
		o.output = p.cache("profiles", name)
	} else {
		o.output = p.resolve(o.output)
	}
	return o, nil
}

type devProfile struct {
	opts devProfileOptions
	done atomic.Bool
	mu   sync.Mutex // only one upload publishes a completed capture
}

func (s *devServer) serveProfile(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	p := s.profile
	s.mu.Unlock()
	if p == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		if p.done.Load() {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if p.opts.duration > 0 {
			event("capturing a %s profile for %s", p.opts.kind, p.opts.duration)
		} else {
			event("capturing a heap profile")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Kind     string        `json:"kind"`
			Duration time.Duration `json:"duration"`
		}{p.opts.kind, p.opts.duration})
	case http.MethodPost:
		p.accept(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "GET a profile request or POST its result", http.StatusMethodNotAllowed)
	}
}

func (p *devProfile) accept(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done.Load() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch r.Header.Get("Content-Type") {
	case "text/plain":
		message, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
		if err != nil {
			http.Error(w, "invalid profile error", http.StatusBadRequest)
			return
		}
		warn("runtime %s profile: %s", p.opts.kind, strings.TrimSpace(string(message)))
	case "application/octet-stream":
		if err := writeRuntimeProfile(p.opts.output, r.Body); err != nil {
			warn("writing runtime %s profile: %v", p.opts.kind, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		p.done.Store(true)
		event("wrote %s profile to %s", p.opts.kind, rel(p.opts.output))
	default:
		http.Error(w, "POST a runtime profile", http.StatusUnsupportedMediaType)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeRuntimeProfile only replaces path after a complete upload.
func writeRuntimeProfile(path string, body io.Reader) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".nsgo-profile-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	n, copyErr := io.Copy(f, body)
	if err := errors.Join(copyErr, f.Close()); err != nil {
		return err
	}
	if n == 0 {
		return errors.New("empty runtime profile")
	}
	return os.Rename(f.Name(), path)
}
