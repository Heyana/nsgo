package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProjectRegistriesValidation(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		dir := t.TempDir()
		content := `name: App
registries:
  "@acme": https://nsgo.acme.example/registry.yaml
  "@local-http": http://127.0.0.1:8080/registry.yaml
  "@localhost": http://localhost:8080/registry.yaml
  "@ipv6": http://[::1]:8080/registry.yaml
  "@team": tools/registry.yaml
  "@parent": ../shared/registry.yaml
`
		writeFiles(t, dir, map[string]string{"nsgo.yaml": content})
		p, err := load(dir)
		if err != nil {
			t.Fatalf("load valid registries: %v", err)
		}
		regs, err := p.ParseRegistries()
		if err != nil {
			t.Fatalf("ParseRegistries: %v", err)
		}
		if len(regs) != 6 {
			t.Fatalf("got %d registries, want 6: %+v", len(regs), regs)
		}
		byName := map[string]registryConfig{}
		for _, r := range regs {
			byName[r.Name] = r
		}
		if got := byName["@acme"].URL; got != "https://nsgo.acme.example/registry.yaml" {
			t.Errorf("@acme URL = %q", got)
		}
		if got, want := byName["@team"].Path, filepath.Join(dir, "tools", "registry.yaml"); got != want {
			t.Errorf("@team Path = %q, want %q", got, want)
		}
	})

	t.Run("null_or_empty", func(t *testing.T) {
		for _, body := range []string{
			"name: App\nregistries:\n",
			"name: App\nregistries: {}\n",
		} {
			dir := t.TempDir()
			writeFiles(t, dir, map[string]string{"nsgo.yaml": body})
			p, err := load(dir)
			if err != nil {
				t.Fatalf("%q: %v", body, err)
			}
			regs, err := p.ParseRegistries()
			if err != nil {
				t.Fatalf("%q ParseRegistries: %v", body, err)
			}
			if len(regs) != 0 {
				t.Errorf("%q: got %d registries, want 0", body, len(regs))
			}
		}
	})

	cases := []struct {
		name     string
		yaml     string
		wantLine string
		wantMsg  string
	}{
		{
			name:     "reserved_nsgo",
			yaml:     "name: App\nregistries:\n  \"@nsgo\": https://example.com/registry.yaml\n",
			wantLine: "line 3",
			wantMsg:  "reserved",
		},
		{
			name:     "missing_at_prefix",
			yaml:     "name: App\nregistries:\n  acme: https://example.com/registry.yaml\n",
			wantLine: "line 3",
			wantMsg:  "must start with @",
		},
		{
			name:     "uppercase_key",
			yaml:     "name: App\nregistries:\n  \"@Acme\": https://example.com/registry.yaml\n",
			wantLine: "line 3",
			wantMsg:  "invalid registry key",
		},
		{
			name:     "leading_hyphen_key",
			yaml:     "name: App\nregistries:\n  \"@-acme\": https://example.com/registry.yaml\n",
			wantLine: "line 3",
			wantMsg:  "invalid registry key",
		},
		{
			name:     "trailing_hyphen_key",
			yaml:     "name: App\nregistries:\n  \"@acme-\": https://example.com/registry.yaml\n",
			wantLine: "line 3",
			wantMsg:  "invalid registry key",
		},
		{
			name:     "empty_after_at",
			yaml:     "name: App\nregistries:\n  \"@\": https://example.com/registry.yaml\n",
			wantLine: "line 3",
			wantMsg:  "invalid registry key",
		},
		{
			name:     "too_long_key",
			yaml:     "name: App\nregistries:\n  \"@" + strings.Repeat("a", 65) + "\": https://example.com/registry.yaml\n",
			wantLine: "line 3",
			wantMsg:  "invalid registry key",
		},
		{
			name:     "duplicate_key",
			yaml:     "name: App\nregistries:\n  \"@acme\": https://example.com/a.yaml\n  \"@acme\": https://example.com/b.yaml\n",
			wantLine: "line 4",
			wantMsg:  "duplicate key",
		},
		{
			name:     "url_with_query",
			yaml:     "name: App\nregistries:\n  \"@acme\": https://example.com/registry.yaml?token=secret\n",
			wantLine: "line 3",
			wantMsg:  "query",
		},
		{
			name:     "url_with_empty_query",
			yaml:     "name: App\nregistries:\n  \"@acme\": https://example.com/registry.yaml?\n",
			wantLine: "line 3",
			wantMsg:  "query",
		},
		{
			name:     "url_with_fragment",
			yaml:     "name: App\nregistries:\n  \"@acme\": https://example.com/registry.yaml#v1\n",
			wantLine: "line 3",
			wantMsg:  "fragment",
		},
		{
			name:     "url_with_userinfo",
			yaml:     "name: App\nregistries:\n  \"@acme\": https://user:pass@example.com/registry.yaml\n",
			wantLine: "line 3",
			wantMsg:  "user credentials",
		},
		{
			name:     "non_loopback_http",
			yaml:     "name: App\nregistries:\n  \"@acme\": http://nsgo.acme.example/registry.yaml\n",
			wantLine: "line 3",
			wantMsg:  "https",
		},
		{
			name:     "unsupported_scheme",
			yaml:     "name: App\nregistries:\n  \"@acme\": file:///tmp/registry.yaml\n",
			wantLine: "line 3",
			wantMsg:  "unsupported URL scheme",
		},
		{
			name:     "absolute_local_path",
			yaml:     "name: App\nregistries:\n  \"@acme\": /etc/nsgo/registry.yaml\n",
			wantLine: "line 3",
			wantMsg:  "relative to nsgo.yaml",
		},
		{
			name:     "object_value",
			yaml:     "name: App\nregistries:\n  \"@acme\":\n    url: https://example.com/registry.yaml\n",
			wantLine: "line 4",
			wantMsg:  "must be an https URL or relative path",
		},
		{
			name:     "sequence_registries",
			yaml:     "name: App\nregistries:\n  - https://example.com/registry.yaml\n",
			wantLine: "line 3",
			wantMsg:  "must be a mapping",
		},
		{
			name:     "unknown_top_level_field",
			yaml:     "name: App\nregistry:\n  \"@acme\": https://example.com/registry.yaml\n",
			wantLine: "line 2",
			wantMsg:  "field registry not found",
		},
		{
			name:     "anchor_in_registries",
			yaml:     "name: App\nregistries:\n  \"@acme\": &u https://example.com/registry.yaml\n",
			wantLine: "line 3",
			wantMsg:  "anchors are not allowed",
		},
		{
			name:     "control_char_in_value",
			yaml:     "name: App\nregistries:\n  \"@acme\": \"tools/reg\u202e.yaml\"\n",
			wantLine: "line 3",
			wantMsg:  "control or Unicode format character",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, map[string]string{"nsgo.yaml": tc.yaml})
			_, err := load(dir)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.wantLine) {
				t.Errorf("error %q does not include %q", msg, tc.wantLine)
			}
			if !strings.Contains(msg, tc.wantMsg) {
				t.Errorf("error %q does not include %q", msg, tc.wantMsg)
			}
		})
	}
}

// Custom registries accept non-canonical key order, 4-space indentation, comments, and optional fields while sharing parseRegistryIndex's schema validation (#2464).
func TestParseCustomRegistryIndexNonCanonical(t *testing.T) {
	validYAML := `# Team registry
items:
    - tags: [charts, data-viz]
      license: (MIT OR Apache-2.0) AND BSD-3-Clause
      description: Team charts library.
      module: github.com/acme/chart
      version: v1.2.0
      homepage: https://acme.example/chart
      name: chart
    - name: chat
      module: github.com/acme/ui
      component: chat
      description: Chat view component.
      license: GPL-2.0-only WITH Classpath-exception-2.0
version: 1
`
	idx, err := parseRegistryIndex([]byte(validYAML))
	if err != nil {
		t.Fatalf("parseRegistryIndex valid: %v", err)
	}
	if idx.Version != 1 || len(idx.Items) != 2 {
		t.Fatalf("got %+v", idx)
	}
	if idx.Items[0].kind() != "library" || idx.Items[1].kind() != "component" {
		t.Errorf("kinds = %q, %q", idx.Items[0].kind(), idx.Items[1].kind())
	}
}

func isolatedRegistryCache(t *testing.T) string {
	t.Helper()
	cacheDir := t.TempDir()
	prevCache := registryUserCacheDir
	registryUserCacheDir = func() (string, error) { return cacheDir, nil }
	t.Cleanup(func() { registryUserCacheDir = prevCache })
	return cacheDir
}

const sampleAcmeRegistryYAML = `version: 1
items:
  - name: chart
    module: github.com/acme/chart
    description: Acme charts for NSGo.
    license: Apache-2.0
    tags: [charts, data]
  - name: dep
    module: example.com/dep
    version: v1.0.0
    description: Shared utility module.
    license: MIT
    tags: [util]
  - name: edge
    module: github.com/acme/edge
    description: Experimental untagged module.
    license: MIT
  - name: chat
    module: github.com/acme/chart
    component: chat
    description: A chat UI component.
    license: MIT
    tags: [ai, chat]
`

// Against httptest on loopback: fetch, 304 for ETag, cache path, offline fallback with age, and error naming URL without cache (#2464).
func TestRemoteRegistryFetchETagAndOfflineCache(t *testing.T) {
	cacheDir := isolatedRegistryCache(t)
	var requests atomic.Int32
	var gotIfNoneMatch atomic.Value
	const etag = `"v1-acme"`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		inm := r.Header.Get("If-None-Match")
		gotIfNoneMatch.Store(inm)
		if inm == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		_, _ = w.Write([]byte(sampleAcmeRegistryYAML))
	}))
	t.Cleanup(srv.Close)

	regURL := srv.URL + "/registry.yaml"
	rc := registryConfig{Name: "@acme", Raw: regURL, URL: regURL}

	// 1. Initial fetch populates cache at UserCacheDir/nsgo/registries/<sha256(url)>.yaml.
	lr, err := loadRegistry(t.Context(), rc)
	if err != nil {
		t.Fatalf("initial fetch: %v", err)
	}
	if lr.FromCache || len(lr.Index.Items) != 4 {
		t.Fatalf("initial fetch = %+v", lr)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}

	sum := sha256.Sum256([]byte(regURL))
	wantCacheFile := filepath.Join(cacheDir, "nsgo", "registries", hex.EncodeToString(sum[:])+".yaml")
	if data, err := os.ReadFile(wantCacheFile); err != nil || string(data) != sampleAcmeRegistryYAML {
		t.Fatalf("cache file %s: err=%v len=%d", wantCacheFile, err, len(data))
	}

	// 2. Second fetch sends If-None-Match and handles 304 Not Modified cleanly.
	lr2, err := loadRegistry(t.Context(), rc)
	if err != nil {
		t.Fatalf("304 fetch: %v", err)
	}
	if lr2.FromCache {
		t.Errorf("304 fetch should not report FromCache")
	}
	if got := gotIfNoneMatch.Load().(string); got != etag {
		t.Errorf("If-None-Match = %q, want %q", got, etag)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}

	// 3. Offline: server closed -> cached copy is used with its age shown.
	srv.Close()
	prevNow := registryNow
	registryNow = func() time.Time { return time.Now().Add(2 * time.Hour) }
	t.Cleanup(func() { registryNow = prevNow })

	lrOffline, err := loadRegistry(t.Context(), rc)
	if err != nil {
		t.Fatalf("offline cached load: %v", err)
	}
	if !lrOffline.FromCache {
		t.Fatalf("offline load did not set FromCache")
	}
	if got := lrOffline.headerLine(); !strings.Contains(got, regURL) || !strings.Contains(got, "cached 2h ago") {
		t.Errorf("offline headerLine = %q, want URL and 'cached 2h ago'", got)
	}

	// 4. Offline with no cached copy -> error names the URL.
	isolatedRegistryCache(t)
	_, err = loadRegistry(t.Context(), rc)
	if err == nil || !strings.Contains(err.Error(), regURL) {
		t.Fatalf("offline uncached load = %v, want error naming %q", err, regURL)
	}
}

// Redirect to http (both HTTPS->HTTP downgrade and non-loopback HTTP), too many redirects, and >4 MiB size limit are refused (#2464).
func TestRemoteRegistryRedirectsAndSizeLimit(t *testing.T) {
	t.Run("https_redirect_to_http_refused", func(t *testing.T) {
		isolatedRegistryCache(t)
		httpTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(sampleAcmeRegistryYAML))
		}))
		t.Cleanup(httpTarget.Close)

		tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, httpTarget.URL+"/registry.yaml", http.StatusFound)
		}))
		t.Cleanup(tlsSrv.Close)

		prevTransport := registryTransport
		registryTransport = tlsSrv.Client().Transport
		t.Cleanup(func() { registryTransport = prevTransport })

		rc := registryConfig{Name: "@acme", Raw: tlsSrv.URL + "/registry.yaml", URL: tlsSrv.URL + "/registry.yaml"}
		_, err := loadRegistry(t.Context(), rc)
		if err == nil || !strings.Contains(err.Error(), "redirected from https to http") {
			t.Fatalf("https->http redirect = %v, want refusal", err)
		}
	})

	t.Run("loopback_http_redirect_to_remote_http_refused", func(t *testing.T) {
		isolatedRegistryCache(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://nsgo.acme.example/registry.yaml", http.StatusFound)
		}))
		t.Cleanup(srv.Close)

		rc := registryConfig{Name: "@acme", Raw: srv.URL + "/registry.yaml", URL: srv.URL + "/registry.yaml"}
		_, err := loadRegistry(t.Context(), rc)
		if err == nil || !strings.Contains(err.Error(), "not https") {
			t.Fatalf("redirect to non-loopback http = %v, want refusal", err)
		}
	})

	t.Run("too_many_redirects_refused", func(t *testing.T) {
		isolatedRegistryCache(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/loop", http.StatusFound)
		}))
		t.Cleanup(srv.Close)

		rc := registryConfig{Name: "@acme", Raw: srv.URL + "/registry.yaml", URL: srv.URL + "/registry.yaml"}
		_, err := loadRegistry(t.Context(), rc)
		if err == nil || !strings.Contains(err.Error(), "10 redirects") {
			t.Fatalf("too many redirects = %v, want refusal", err)
		}
	})

	t.Run("size_limit_refused", func(t *testing.T) {
		isolatedRegistryCache(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("version: 1\nitems: []\n# "))
			_, _ = w.Write(bytes.Repeat([]byte("a"), maxRegistryIndexBytes))
		}))
		t.Cleanup(srv.Close)

		rc := registryConfig{Name: "@acme", Raw: srv.URL + "/registry.yaml", URL: srv.URL + "/registry.yaml"}
		_, err := loadRegistry(t.Context(), rc)
		if err == nil || !strings.Contains(err.Error(), "4 MiB") {
			t.Fatalf("oversized response = %v, want 4 MiB limit error", err)
		}
	})
}

// @acme/chart resolves through @acme (remote or local), plan's first line names the registry and URL, bare chart does not resolve through @acme, and unconfigured registries are never auto-added to nsgo.yaml (#2464).
func TestAddCustomRegistryResolutionAndPlan(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	isolatedRegistryCache(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(sampleAcmeRegistryYAML))
	}))
	t.Cleanup(srv.Close)

	dir := nsgoApp(t)
	regURL := srv.URL + "/registry.yaml"
	nsgoYAML := "name: App\nregistries:\n  \"@acme\": " + regURL + "\n  \"@team\": tools/registry.yaml\n"
	writeFiles(t, dir, map[string]string{
		"nsgo.yaml": nsgoYAML,
		"tools/registry.yaml": `version: 1
items:
  - name: dep
    module: example.com/dep
    description: Local team utility.
    license: MIT
    tags: [local, util]
`,
	})

	// 1. @acme/chart resolves through @acme, and plan's first line names @acme and its URL.
	plan, err := planAdd(t, dir, "@acme/chart")
	if err != nil {
		t.Fatalf("plan @acme/chart: %v\n%s", err, plan)
	}
	firstLine, _, _ := strings.Cut(plan, "\n")
	if want := "registry @acme (" + regURL + ")"; firstLine != want {
		t.Errorf("plan first line = %q, want %q\nfull plan:\n%s", firstLine, want, plan)
	}
	if !strings.Contains(plan, "+ require github.com/acme/chart v1.2.0 // indirect") {
		t.Errorf("plan missing github.com/acme/chart:\n%s", plan)
	}

	// Applying @acme/chart writes go.mod/go.sum identically to go get.
	wantMod, wantSum := goGetWrites(t, dir, "github.com/acme/chart@v1.2.0")
	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "@acme/chart"); err != nil {
		t.Fatalf("apply @acme/chart: %v", err)
	}
	if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, wantMod) || !bytes.Equal(gosum, wantSum) {
		t.Errorf("go.mod/go.sum mismatch after @acme/chart")
	}

	// 2. Local file registry @team/dep resolves and names its relative path on line 1.
	localPlan, err := planAdd(t, dir, "@team/dep")
	if err != nil {
		t.Fatalf("plan @team/dep: %v\n%s", err, localPlan)
	}
	localFirst, _, _ := strings.Cut(localPlan, "\n")
	if want := "registry @team (tools/registry.yaml)"; localFirst != want {
		t.Errorf("local plan first line = %q, want %q", localFirst, want)
	}

	// 3. Bare "chart" does NOT resolve through @acme.
	if _, err := planAdd(t, dir, "chart"); err == nil {
		t.Fatalf("bare 'chart' unexpectedly resolved")
	}

	// 4. Unconfigured @unknown/chart fails clearly and NEVER modifies nsgo.yaml.
	_, err = planAdd(t, dir, "@unknown/chart")
	if err == nil || !strings.Contains(err.Error(), "registry @unknown is not configured in nsgo.yaml") {
		t.Fatalf("@unknown/chart = %v, want unconfigured registry error", err)
	}
	if gotYAML, err := os.ReadFile(filepath.Join(dir, "nsgo.yaml")); err != nil || string(gotYAML) != nsgoYAML {
		t.Fatalf("nsgo.yaml changed after unconfigured registry error:\n%s", gotYAML)
	}

	// 5. Component item is refused naming its kind.
	if _, err := planAdd(t, dir, "@acme/chat"); err == nil || !strings.Contains(err.Error(), "is a component") {
		t.Fatalf("@acme/chat = %v, want component refusal", err)
	}

	// 6. Registry item resolving to an untagged commit is refused when no version was typed.
	if _, err := planAdd(t, dir, "@acme/edge"); err == nil || !strings.Contains(err.Error(), "@acme/edge has no release") {
		t.Fatalf("@acme/edge = %v, want no-release refusal", err)
	}

	// 7. Offline plan uses cached copy and shows its age on the plan's first line.
	srv.Close()
	prevNow := registryNow
	registryNow = func() time.Time { return time.Now().Add(45 * time.Minute) }
	t.Cleanup(func() { registryNow = prevNow })

	offlinePlan, err := planAdd(t, dir, "@acme/dep")
	if err != nil {
		t.Fatalf("offline plan @acme/dep: %v\n%s", err, offlinePlan)
	}
	offlineFirst, _, _ := strings.Cut(offlinePlan, "\n")
	if want := "registry @acme (" + regURL + ", cached 45m ago)"; offlineFirst != want {
		t.Errorf("offline plan first line = %q, want %q", offlineFirst, want)
	}
}

// -list lists every configured registry's items with their prefix, filters by @registry and words, and works outside a Go module (#2464).
func TestAddListCustomRegistries(t *testing.T) {
	isolatedRegistryCache(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleAcmeRegistryYAML))
	}))
	t.Cleanup(srv.Close)

	// Directory WITHOUT go.mod (outside a module) to verify -list works outside a module.
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"nsgo.yaml": "name: App\nregistries:\n  \"@acme\": " + srv.URL + "/registry.yaml\n  \"@acme-extra\": extra.yaml\n  \"@team\": tools/registry.yaml\n",
		"extra.yaml": `version: 1
items:
  - name: chart
    module: example.com/extra-chart
    description: Extra chart.
    license: MIT
`,
		"tools/registry.yaml": `version: 1
items:
  - name: dep
    module: example.com/dep
    description: Team helper library.
    license: MIT
    tags: [helper]
`,
	})

	runList := func(args ...string) (string, error) {
		t.Helper()
		t.Chdir(dir)
		var out bytes.Buffer
		var err error
		captureStderr(t, func() {
			err = addModules(context.Background(), addOptions{list: true}, args, &out, nil)
		})
		return out.String(), err
	}

	// 1. All configured registries listed with their @prefix.
	all, err := runList()
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	for _, want := range []string{
		"@acme/chart", "library", "github.com/acme/chart",
		"@acme/chat", "component",
		"@acme-extra/chart",
		"@team/dep", "Team helper library.",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("list all missing %q:\n%s", want, all)
		}
	}

	// 2. Filtering by @acme includes @acme/* and excludes @acme-extra/* and @team/*.
	acmeOnly, err := runList("@acme")
	if err != nil {
		t.Fatalf("list @acme: %v", err)
	}
	if !strings.Contains(acmeOnly, "@acme/chart") || strings.Contains(acmeOnly, "@acme-extra/chart") || strings.Contains(acmeOnly, "@team/dep") {
		t.Errorf("list @acme =\n%s", acmeOnly)
	}

	// 3. Filtering by @acme and word matches tag/name/description case-insensitively.
	filtered, err := runList("@acme", "AI")
	if err != nil {
		t.Fatalf("list @acme AI: %v", err)
	}
	if !strings.Contains(filtered, "@acme/chat") || strings.Contains(filtered, "@acme/chart") {
		t.Errorf("list @acme AI =\n%s", filtered)
	}

	// 4. Unconfigured registry in -list fails clearly.
	if _, err := runList("@missing"); err == nil || !strings.Contains(err.Error(), "registry @missing is not configured") {
		t.Errorf("list @missing = %v, want unconfigured error", err)
	}

	// 5. -list cannot combine with -dry-run or -y.
	t.Chdir(dir)
	if err := addModules(context.Background(), addOptions{list: true, yes: true}, nil, &bytes.Buffer{}, nil); err == nil {
		t.Errorf("-list -y should fail")
	}
}
