package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	committedDex = "dex\n035 committed"
	committedJar = "PK\x03\x04committed"
	builtDex     = "dex\n035 built"
	builtJar     = "PK\x03\x04built"
)

// runtimeCheckout makes a Git repository with a changelog ready to release and a runtime last rebuilt in its first commit, and returns it with a function that commits files in it.
func runtimeCheckout(t *testing.T) (root string, commit func(msg string, files map[string]string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root = repo(t, sample, map[string]string{"new.md": "### Added\n- Add new.\n"})
	gitT := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	commit = func(msg string, files map[string]string) {
		t.Helper()
		for name, data := range files {
			write(t, filepath.Join(root, name), data)
		}
		gitT("add", "-A")
		gitT("commit", "-q", "-m", msg)
	}
	gitT("init", "-q", "-b", "main")
	commit("rebuild the runtime", map[string]string{
		"internal/platform/android/java/dev/oneo/nsgo/Main.java": "class Main {}\n",
		runtimeDir + "/build.sh":                                 "#!/bin/sh\n",
		runtimeDir + "/play_review.py":                           "",
		runtimeDir + "/classes.dex":                              committedDex,
		runtimeDir + "/classes.jar":                              committedJar,
	})
	return root, commit
}

func head(t *testing.T, root string) string {
	t.Helper()
	out, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out)
}

// fakeActions is a GitHub whose runtime workflow, once dispatched, lists newRun after polls more looks at the runs, then reports it running for running more looks at it before it completes with conclusion.
type fakeActions struct {
	t          *testing.T
	tip        string
	before     []workflowRun
	newRun     workflowRun
	polls      int
	running    int
	conclusion string
	artifact   map[string]string

	dispatched bool
	downloaded int64
}

func (f *fakeActions) mainTip() (string, error) { return f.tip, nil }

func (f *fakeActions) dispatches() ([]workflowRun, error) {
	if !f.dispatched {
		return f.before, nil
	}
	if f.polls > 0 {
		f.polls--
		return f.before, nil
	}
	// An older run and a newer one at another commit are listed too, as when others dispatch the workflow at the same time.
	other := workflowRun{ID: f.newRun.ID + 1, HeadSHA: "0123456789abcdef", Status: "queued"}
	return append([]workflowRun{other, f.newRun}, f.before...), nil
}

func (f *fakeActions) dispatch() error {
	if f.dispatched {
		f.t.Error("dispatched twice")
	}
	f.dispatched = true
	return nil
}

func (f *fakeActions) run(id int64) (workflowRun, error) {
	if id != f.newRun.ID {
		f.t.Errorf("looked at run %d, want %d", id, f.newRun.ID)
	}
	r := f.newRun
	if f.running > 0 {
		f.running--
		r.Status = "in_progress"
		return r, nil
	}
	r.Status, r.Conclusion = "completed", f.conclusion
	return r, nil
}

func (f *fakeActions) download(id int64, dir string) error {
	f.downloaded = id
	for name, data := range f.artifact {
		write(f.t, filepath.Join(dir, name), data)
	}
	return nil
}

// noActions fails a test that reaches GitHub.
type noActions struct{ t *testing.T }

func (n noActions) mainTip() (string, error) {
	n.t.Error("asked for main's tip")
	return "", errors.New("unexpected")
}
func (n noActions) dispatches() ([]workflowRun, error) {
	n.t.Error("listed runs")
	return nil, errors.New("unexpected")
}
func (n noActions) dispatch() error { n.t.Error("dispatched"); return errors.New("unexpected") }
func (n noActions) run(int64) (workflowRun, error) {
	n.t.Error("viewed a run")
	return workflowRun{}, errors.New("unexpected")
}
func (n noActions) download(int64, string) error {
	n.t.Error("downloaded")
	return errors.New("unexpected")
}

func testRebuild(gh actions, slept *time.Duration) *rebuild {
	return &rebuild{gh: gh, sleep: func(d time.Duration) { *slept += d }, poll: time.Second, start: 5 * time.Second, finish: 20 * time.Second}
}

func assertRuntime(t *testing.T, root, dex, jar string) {
	t.Helper()
	if got := read(t, filepath.Join(root, runtimeDir, "classes.dex")); got != dex {
		t.Errorf("classes.dex is %q, want %q", got, dex)
	}
	if got := read(t, filepath.Join(root, runtimeDir, "classes.jar")); got != jar {
		t.Errorf("classes.jar is %q, want %q", got, jar)
	}
}

func assertUnreleased(t *testing.T, root string) {
	t.Helper()
	if got := read(t, filepath.Join(root, changelogFile)); got != sample {
		t.Errorf("rewrote the changelog:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(root, fragmentDir, "new.md")); err != nil {
		t.Errorf("removed the fragment: %v", err)
	}
}

func TestRuntimeStale(t *testing.T) {
	root, commit := runtimeCheckout(t)
	check := func(want bool, wantWhy string) {
		t.Helper()
		stale, why, err := runtimeStale(root)
		if err != nil {
			t.Fatal(err)
		}
		if stale != want || !strings.Contains(why, wantWhy) {
			t.Errorf("stale %v (%q), want %v containing %q", stale, why, want, wantWhy)
		}
	}
	check(false, "")
	// Neither the Go side of the runtime, its manifest nor anything outside it is built into the files.
	commit("touch what the runtime is not built from", map[string]string{
		runtimeDir + "/runtime.go":          "package runtime\n",
		runtimeDir + "/AndroidManifest.xml": "<manifest/>\n",
		"internal/platform/android/a.go":    "package android\n",
	})
	check(false, "")
	commit("change the Java", map[string]string{"internal/platform/android/java/dev/oneo/nsgo/Main.java": "class Main { int x; }\n"})
	check(true, "change the Java")
	commit("rebuild only the dex", map[string]string{runtimeDir + "/classes.dex": "dex\n035 rebuilt"})
	check(false, "")
	for _, input := range []string{runtimeDir + "/build.sh", runtimeDir + "/play_review.py", runtimeDir + "/play-review.pro"} {
		commit("change "+input, map[string]string{input: "changed\n"})
		check(true, "change "+input)
		commit("rebuild after "+input, map[string]string{runtimeDir + "/classes.jar": "PK\x03\x04" + input})
		check(false, "")
	}
}

func TestRuntimeNeverBuiltIsStale(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "empty"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if stale, why, err := runtimeStale(root); err != nil || !stale || !strings.Contains(why, "no commit") {
		t.Errorf("stale %v (%q), %v; want stale", stale, why, err)
	}
}

func TestReleaseLeavesCurrentRuntime(t *testing.T) {
	root, _ := runtimeCheckout(t)
	var slept time.Duration
	if err := release(root, "0.3.0", "2026-12-01", testRebuild(noActions{t}, &slept)); err != nil {
		t.Fatal(err)
	}
	assertRuntime(t, root, committedDex, committedJar)
	if got := read(t, filepath.Join(root, changelogFile)); !strings.Contains(got, "## [0.3.0] - 2026-12-01\n\n- Add new.\n") {
		t.Errorf("changelog:\n%s", got)
	}
}

func TestReleaseRebuildsStaleRuntime(t *testing.T) {
	root, commit := runtimeCheckout(t)
	commit("change the Java", map[string]string{"internal/platform/android/java/dev/oneo/nsgo/Main.java": "class Main { int x; }\n"})
	tip := head(t, root)
	gh := &fakeActions{
		t:          t,
		tip:        tip,
		before:     []workflowRun{{ID: 40, HeadSHA: tip, Status: "completed", Conclusion: "success"}, {ID: 30, HeadSHA: "fedcba9876543210", Status: "completed", Conclusion: "success"}},
		newRun:     workflowRun{ID: 41, HeadSHA: tip, Status: "queued", URL: "https://github.com/nsgo-dev/nsgo/actions/runs/41"},
		polls:      2,
		running:    3,
		conclusion: "success",
		artifact:   map[string]string{"classes.dex": builtDex, "classes.jar": builtJar},
	}
	var slept time.Duration
	if err := release(root, "0.3.0", "2026-12-01", testRebuild(gh, &slept)); err != nil {
		t.Fatal(err)
	}
	if gh.downloaded != 41 {
		t.Errorf("downloaded run %d, want the dispatched run 41, not the earlier run at the same commit or the later one at another", gh.downloaded)
	}
	if slept != 6*time.Second {
		t.Errorf("slept %v, want 6s: two polls for the run to appear, one while it is queued and three while it runs", slept)
	}
	assertRuntime(t, root, builtDex, builtJar)
	if got := read(t, filepath.Join(root, changelogFile)); !strings.Contains(got, "## [0.3.0] - 2026-12-01\n\n- Add new.\n") {
		t.Errorf("changelog:\n%s", got)
	}
}

func TestReleaseRefusesWhatItCannotRebuild(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(gh *fakeActions)
		want  string
	}{
		{"behind main", func(gh *fakeActions) { gh.tip = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" }, "main's tip on GitHub is aaaaaaaaaaaa"},
		{"failed run", func(gh *fakeActions) { gh.conclusion = "failure" }, "ended failure"},
		{"run never listed", func(gh *fakeActions) { gh.polls = 100 }, "appeared within 5s"},
		{"run never finished", func(gh *fakeActions) { gh.running = 100 }, "did not finish within 20s"},
		{"artifact without the jar", func(gh *fakeActions) { delete(gh.artifact, "classes.jar") }, "classes.jar"},
		{"artifact with something else", func(gh *fakeActions) { gh.artifact["classes.dex"] = "<html>" }, "holds a classes.dex that is not one"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, commit := runtimeCheckout(t)
			commit("change the Java", map[string]string{"internal/platform/android/java/dev/oneo/nsgo/Main.java": "class Main { int x; }\n"})
			tip := head(t, root)
			gh := &fakeActions{
				t:          t,
				tip:        tip,
				newRun:     workflowRun{ID: 1, HeadSHA: tip, Status: "queued", URL: "https://github.com/nsgo-dev/nsgo/actions/runs/1"},
				conclusion: "success",
				artifact:   map[string]string{"classes.dex": builtDex, "classes.jar": builtJar},
			}
			tc.setup(gh)
			var slept time.Duration
			err := release(root, "0.3.0", "2026-12-01", testRebuild(gh, &slept))
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "bot/android-runtime") {
				t.Fatalf("%v, want an error containing %q that names the manual rebuild", err, tc.want)
			}
			assertRuntime(t, root, committedDex, committedJar)
			assertUnreleased(t, root)
		})
	}
}

func TestReleaseWithoutGH(t *testing.T) {
	root, commit := runtimeCheckout(t)
	commit("change the Java", map[string]string{"internal/platform/android/java/dev/oneo/nsgo/Main.java": "class Main { int x; }\n"})
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	// A PATH holding git and nothing else, so gh is missing whatever the machine has.
	bin := t.TempDir()
	if err := os.Symlink(gitPath, filepath.Join(bin, "git")); err != nil {
		t.Skip(err)
	}
	t.Setenv("PATH", bin)
	err = run(root, []string{"release", "-date", "2026-12-01", "0.3.0"}, time.Time{})
	if err == nil || !strings.Contains(err.Error(), "gh command is not installed") || !strings.Contains(err.Error(), "-no-runtime") {
		t.Fatalf("%v, want an error naming the missing gh and the manual rebuild", err)
	}
	assertRuntime(t, root, committedDex, committedJar)
	assertUnreleased(t, root)
}
