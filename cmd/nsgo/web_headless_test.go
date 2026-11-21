package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

// Library tests run the headless model inside the browser, without the platform host. Exercise both the model and a control through that path.
func TestWebHeadlessLibrary(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs library tests in a browser")
	}
	if _, err := findBrowser(); err != nil {
		t.Skip(err)
	}
	for _, c := range []struct {
		dir, run string
		want     []string
		skipped  string
	}{
		{"nsgotest", "^(TestFramesFollowTheClock|TestAppDirectoriesAreTemporary|TestMicrophoneInput)$", []string{"TestAppDirectoriesAreTemporary", "TestFramesFollowTheClock", "TestMicrophoneInput"}, "TestMicrophoneInput"},
		{"ui", "^TestInput$", []string{"TestInput"}, ""},
		{"tsx", "^(TestAlertDialogNeedsActions|TestAnimatedCompositesRunInOrder)$", []string{"TestAlertDialogNeedsActions", "TestAnimatedCompositesRunInOrder"}, ""},
	} {
		t.Run(c.dir, func(t *testing.T) {
			p, err := loadForTest(filepath.Join("..", "..", c.dir), nil)
			if err != nil {
				t.Fatal(err)
			}
			report := filepath.Join(t.TempDir(), "tests.xml")
			if err := runTests(t.Context(), p, testOptions{target: "web", run: c.run, junit: report}); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(report)
			if err != nil {
				t.Fatal(err)
			}
			var suites junitSuites
			if err := xml.Unmarshal(data, &suites); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, suite := range suites.Suites {
				for _, result := range suite.Cases {
					if result.Failure != nil || (result.Skipped != nil) != (result.Name == c.skipped) {
						t.Fatalf("unexpected result for %s: %+v", result.Name, result)
					}
					got = append(got, result.Name)
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, c.want) {
				t.Fatalf("reported tests = %v, want %v", got, c.want)
			}
		})
	}
}

// Rebuilding a package must not replace the files a live browser runner serves. Each runner keeps its own build until it closes.
func TestWebTestBuildsAreIsolated(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs two web test binaries")
	}
	if _, err := findBrowser(); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":    "module example.com/fixture\n\ngo 1.26\n",
		"first.go":  "//go:build !second\n\npackage fixture\nconst revision = \"first\"\n",
		"second.go": "//go:build second\n\npackage fixture\nconst revision = \"second\"\n",
		"fixture_test.go": `package fixture
import ("os"; "testing")
func TestRevision(t *testing.T) {
	if want := os.Getenv("NSGO_EXPECTED"); revision != want { t.Fatalf("revision = %s, want %s", revision, want) }
}
`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	build := func(tags ...string) targetRunner {
		t.Helper()
		p, err := loadForTest(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		p.testBinary, p.Build.Tags = true, tags
		runner, err := targetFor(t.Context(), p, testOptions{target: "web"})
		if err != nil {
			t.Fatal(err)
		}
		runner.close = sync.OnceFunc(runner.close)
		t.Cleanup(runner.close)
		return runner
	}
	first, second := build(), build("second")
	checkBuilds := func(want int) {
		t.Helper()
		entries, err := os.ReadDir(filepath.Join(dir, ".nsgo", "test", "web"))
		if err != nil || len(entries) != want {
			t.Fatalf("live builds = %d, error %v; want %d", len(entries), err, want)
		}
	}
	checkBuilds(2)
	run := func(runner targetRunner, want string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		var out bytes.Buffer
		code, err := runner.run(ctx, targetTestArgs("TestRevision", "", "web", 20*time.Second), []string{"NSGO_EXPECTED=" + want}, &out)
		if err != nil || code != 0 {
			t.Fatalf("%s run: exit %d, error %v\n%s", want, code, err, out.String())
		}
	}
	run(first, "first")
	first.close()
	checkBuilds(1)
	run(second, "second")
	second.close()
	checkBuilds(0)
}

// A run sweeps the builds of runs that never closed their runner once they are stale, and what earlier versions left, but never a live run's.
func TestSweepTargetBuilds(t *testing.T) {
	base := t.TempDir()
	now := time.Now()
	for _, name := range []string{"run-live", "run-crashed", "run-old-live"} {
		if err := os.Mkdir(filepath.Join(base, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(base, "app.wasm"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "Fixture.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-staleTargetBuild - time.Minute)
	if err := os.Chtimes(filepath.Join(base, "run-crashed"), old, old); err != nil {
		t.Fatal(err)
	}
	recent := now.Add(-staleTargetBuild + time.Hour)
	if err := os.Chtimes(filepath.Join(base, "run-old-live"), recent, recent); err != nil {
		t.Fatal(err)
	}
	sweepTargetBuilds(base, now)
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if want := []string{"run-live", "run-old-live"}; !slices.Equal(got, want) {
		t.Fatalf("after the sweep = %v, want %v", got, want)
	}
}
