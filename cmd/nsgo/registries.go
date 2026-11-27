package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	registryConnectTimeout = 30 * time.Second
	registryFetchTimeout   = 5 * time.Minute
	registryMaxRedirects   = 10
)

// errRegistryPolicy marks a fetch failure caused by a policy violation (such as
// a redirect to plain HTTP or too many redirects), which must never fall back
// to a cached copy.
var errRegistryPolicy = errors.New("registry fetch policy violation")

// registryTransport carries custom registry fetches; tests replace it to reach
// local TLS servers.
var registryTransport http.RoundTripper = defaultRegistryTransport()

// registryUserCacheDir returns the OS user cache directory; tests may replace
// it for isolated caching.
var registryUserCacheDir = os.UserCacheDir

// registryNow returns the current time; tests may replace it to verify offline
// cache age formatting.
var registryNow = time.Now

func defaultRegistryTransport() *http.Transport {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.DialContext = (&net.Dialer{
		Timeout:   registryConnectTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	base.TLSHandshakeTimeout = registryConnectTimeout
	return base
}

// loadedRegistry is a custom registry index fetched from its URL (or cached
// copy offline) or read from a local relative file.
type loadedRegistry struct {
	Config    registryConfig
	Index     *registryIndex
	FromCache bool
	CacheAge  time.Duration
}

// headerLine formats the plan header naming the registry, its source, and cache
// age when loaded offline.
func (lr *loadedRegistry) headerLine() string {
	if lr.FromCache {
		return fmt.Sprintf("registry %s (%s, cached %s ago)", lr.Config.Name, lr.Config.Raw, formatCacheAge(lr.CacheAge))
	}
	return fmt.Sprintf("registry %s (%s)", lr.Config.Name, lr.Config.Raw)
}

// isLoopbackHost reports whether host is localhost or a loopback IP address.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// loadProjectRegistries searches dirs in order for an nsgo.yaml and returns its
// configured registries. If no directory holds an nsgo.yaml, it returns an
// empty map and no error.
func loadProjectRegistries(dirs ...string) (map[string]registryConfig, []registryConfig, error) {
	seenDirs := map[string]bool{}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, nil, err
		}
		if seenDirs[abs] {
			continue
		}
		seenDirs[abs] = true
		p, err := load(abs)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		regs, err := p.ParseRegistries()
		if err != nil {
			return nil, nil, err
		}
		byName := make(map[string]registryConfig, len(regs))
		for _, rc := range regs {
			byName[rc.Name] = rc
		}
		return byName, regs, nil
	}
	return map[string]registryConfig{}, nil, nil
}

// loadRegistry reads and parses a custom registry index from its local file or
// remote URL.
func loadRegistry(ctx context.Context, rc registryConfig) (*loadedRegistry, error) {
	if rc.Path != "" {
		return loadLocalRegistry(rc)
	}
	return fetchRemoteRegistry(ctx, rc)
}

func loadLocalRegistry(rc registryConfig) (*loadedRegistry, error) {
	info, err := os.Stat(rc.Path)
	if err != nil {
		return nil, fmt.Errorf("registry %s (%s): %w", rc.Name, rc.Raw, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("registry %s (%s): must be a regular file", rc.Name, rc.Raw)
	}
	if info.Size() > maxRegistryIndexBytes {
		return nil, fmt.Errorf("registry %s (%s): index exceeds %d MiB limit", rc.Name, rc.Raw, maxRegistryIndexBytes>>20)
	}
	f, err := os.Open(rc.Path)
	if err != nil {
		return nil, fmt.Errorf("registry %s (%s): %w", rc.Name, rc.Raw, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxRegistryIndexBytes+1))
	if err != nil {
		return nil, fmt.Errorf("registry %s (%s): %w", rc.Name, rc.Raw, err)
	}
	idx, err := parseRegistryIndex(data)
	if err != nil {
		return nil, fmt.Errorf("registry %s (%s): %w", rc.Name, rc.Raw, err)
	}
	return &loadedRegistry{Config: rc, Index: idx}, nil
}

func registryCachePath(cacheRoot, rawURL string) (yamlPath, etagPath, lockPath string) {
	sum := sha256.Sum256([]byte(rawURL))
	key := hex.EncodeToString(sum[:])
	dir := filepath.Join(cacheRoot, "nsgo", "registries")
	return filepath.Join(dir, key+".yaml"), filepath.Join(dir, key+".etag"), filepath.Join(dir, key+".lock")
}

func fetchRemoteRegistry(ctx context.Context, rc registryConfig) (*loadedRegistry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cacheRoot, err := registryUserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("registry %s (%s): cache directory: %w", rc.Name, rc.URL, err)
	}
	yamlPath, etagPath, lockPath := registryCachePath(cacheRoot, rc.URL)
	cacheDir := filepath.Dir(yamlPath)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("registry %s (%s): %w", rc.Name, rc.URL, err)
	}

	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("registry %s (%s): %w", rc.Name, rc.URL, err)
	}
	defer lock.Close()
	for {
		held, err := tryLockFile(lock)
		if err != nil {
			return nil, fmt.Errorf("registry %s (%s): %w", rc.Name, rc.URL, err)
		}
		if held {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	defer unlockFile(lock)

	var cachedIndex *registryIndex
	var cachedModTime time.Time
	var cachedETag string
	if info, err := os.Stat(yamlPath); err == nil && info.Mode().IsRegular() && info.Size() <= maxRegistryIndexBytes {
		if data, err := os.ReadFile(yamlPath); err == nil {
			if idx, err := parseRegistryIndex(data); err == nil {
				cachedIndex = idx
				cachedModTime = info.ModTime()
				if etagBytes, err := os.ReadFile(etagPath); err == nil {
					cachedETag = strings.TrimSpace(string(etagBytes))
				}
			}
		}
	}

	fetchCtx, cancel := context.WithTimeout(ctx, registryFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, rc.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("registry %s (%s): %w", rc.Name, rc.URL, err)
	}
	if cachedIndex != nil && cachedETag != "" {
		req.Header.Set("If-None-Match", cachedETag)
	}

	stage(ctx, "fetching registry %s", rc.Name)
	client := &http.Client{
		Transport:     registryTransport,
		CheckRedirect: checkRegistryRedirect,
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, errRegistryPolicy) {
			return nil, fmt.Errorf("registry %s (%s): %w", rc.Name, rc.URL, err)
		}
		if cachedIndex != nil {
			age := max(registryNow().Sub(cachedModTime), 0)
			return &loadedRegistry{
				Config:    rc,
				Index:     cachedIndex,
				FromCache: true,
				CacheAge:  age,
			}, nil
		}
		return nil, fmt.Errorf("registry %s (%s): %w", rc.Name, rc.URL, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		if cachedIndex == nil {
			return nil, fmt.Errorf("registry %s (%s): server returned HTTP 304 without a cached copy", rc.Name, rc.URL)
		}
		now := registryNow()
		_ = os.Chtimes(yamlPath, now, now)
		if etag := strings.TrimSpace(resp.Header.Get("ETag")); etag != "" && etag != cachedETag {
			_ = writeAtomicFile(etagPath, []byte(etag+"\n"), 0o644)
		}
		return &loadedRegistry{Config: rc, Index: cachedIndex}, nil

	case http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxRegistryIndexBytes+1))
		if err != nil {
			return nil, fmt.Errorf("registry %s (%s): %w", rc.Name, rc.URL, err)
		}
		if len(body) > maxRegistryIndexBytes {
			return nil, fmt.Errorf("registry %s (%s): index exceeds %d MiB limit", rc.Name, rc.URL, maxRegistryIndexBytes>>20)
		}
		idx, err := parseRegistryIndex(body)
		if err != nil {
			return nil, fmt.Errorf("registry %s (%s): %w", rc.Name, rc.URL, err)
		}
		if err := writeAtomicFile(yamlPath, body, 0o644); err != nil {
			return nil, fmt.Errorf("registry %s (%s): cache write: %w", rc.Name, rc.URL, err)
		}
		if etag := strings.TrimSpace(resp.Header.Get("ETag")); etag != "" {
			if err := writeAtomicFile(etagPath, []byte(etag+"\n"), 0o644); err != nil {
				return nil, fmt.Errorf("registry %s (%s): cache etag write: %w", rc.Name, rc.URL, err)
			}
		} else {
			_ = os.Remove(etagPath)
		}
		return &loadedRegistry{Config: rc, Index: idx}, nil

	default:
		return nil, fmt.Errorf("registry %s (%s): HTTP %d", rc.Name, rc.URL, resp.StatusCode)
	}
}

func checkRegistryRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= registryMaxRedirects {
		return fmt.Errorf("%w: stopped after %d redirects", errRegistryPolicy, registryMaxRedirects)
	}
	if req.URL.Hostname() == "" || req.URL.User != nil {
		return fmt.Errorf("%w: redirected to invalid URL %s", errRegistryPolicy, req.URL.Redacted())
	}
	switch req.URL.Scheme {
	case "https":
		return nil
	case "http":
		for _, prev := range via {
			if prev.URL.Scheme == "https" {
				return fmt.Errorf("%w: redirected from https to http (%s)", errRegistryPolicy, req.URL.Redacted())
			}
		}
		if !isLoopbackHost(req.URL.Hostname()) {
			return fmt.Errorf("%w: redirected to %s, which is not https", errRegistryPolicy, req.URL.Redacted())
		}
		return nil
	default:
		return fmt.Errorf("%w: redirected to %s, which is not https", errRegistryPolicy, req.URL.Redacted())
	}
}

func writeAtomicFile(path string, data []byte, perm fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(data)
	err = errors.Join(writeErr, tmp.Chmod(perm), tmp.Close())
	if err == nil {
		err = os.Rename(tmpName, path)
	}
	if err != nil {
		_ = os.Remove(tmpName)
	}
	return err
}

func formatCacheAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
