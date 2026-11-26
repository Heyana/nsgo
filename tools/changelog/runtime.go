package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The Android runtime's prebuilt files, which pull requests leave alone (AGENTS.md), and what they are built from: build.sh compiles ../java with javac and d8, adds the Play Review bytecode play_review.py fetches, shrunk with play-review.pro.
const (
	runtimeDir      = "internal/platform/android/runtime"
	runtimeWorkflow = "android-runtime.yml"
	runtimeArtifact = "android-runtime"
	runtimeRepo     = "nsgo-dev/nsgo"
)

var (
	runtimeFiles  = []string{"classes.dex", "classes.jar"}
	runtimeInputs = []string{
		"internal/platform/android/java",
		runtimeDir + "/build.sh",
		runtimeDir + "/play_review.py",
		runtimeDir + "/play-review.pro",
	}
	// runtimeMagic is how each file starts: a dex's header, and a jar's first zip entry.
	runtimeMagic = map[string][]byte{"classes.dex": []byte("dex\n"), "classes.jar": []byte("PK\x03\x04")}
)

const manualRebuild = "rebuild it by hand instead: run " + runtimeDir + "/build.sh with a JDK 21 and the Android SDK on main's tip and merge classes.dex and classes.jar in a bot/android-runtime pull request, then release with -no-runtime"

// workflowRun is the part of a GitHub Actions run `gh run list` and `gh run view` report that the rebuild reads.
type workflowRun struct {
	ID         int64  `json:"databaseId"`
	HeadSHA    string `json:"headSha"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	URL        string `json:"url"`
}

// actions is the GitHub the rebuild talks to, through gh in a release and a fake in the tests.
type actions interface {
	// mainTip is the commit main's branch points at on GitHub.
	mainTip() (string, error)
	// dispatches lists the runtime workflow's recent workflow_dispatch runs.
	dispatches() ([]workflowRun, error)
	// dispatch starts the runtime workflow on main.
	dispatch() error
	run(id int64) (workflowRun, error)
	// download writes the run's runtime artifact's files into dir.
	download(id int64, dir string) error
}

// rebuild refreshes a stale committed Android runtime with one built by the runtime workflow on main.
type rebuild struct {
	gh    actions
	sleep func(time.Duration)
	// poll is the wait between two looks at the runs; start bounds the wait for the dispatched run to appear, and finish the wait for it to complete.
	poll, start, finish time.Duration
}

func newRebuild() *rebuild {
	return &rebuild{gh: ghCLI{}, sleep: time.Sleep, poll: 10 * time.Second, start: 2 * time.Minute, finish: 30 * time.Minute}
}

// runtimeStale reports the commits since the runtime was last rebuilt that change what it is built from, newest first, or why it is stale when it was never built. Every rebuild rewrites both files from the same inputs, so the newest commit changing either is the last rebuild.
//
// It decides from history rather than by comparing the files with a rebuild, which needs a JDK and the Android SDK; release.yml's check compares them exactly, so a rebuild made from an older main than the one it merged into still stops the release.
func runtimeStale(root string) (stale bool, why string, err error) {
	paths := make([]string, len(runtimeFiles))
	for i, f := range runtimeFiles {
		paths[i] = runtimeDir + "/" + f
	}
	last, err := git(root, append([]string{"log", "-1", "--format=%H", "HEAD", "--"}, paths...)...)
	if err != nil {
		return false, "", err
	}
	last = strings.TrimSpace(last)
	if last == "" {
		return true, "no commit has built it", nil
	}
	changes, err := git(root, append([]string{"log", "--format=%h %s", last + "..HEAD", "--"}, runtimeInputs...)...)
	if err != nil {
		return false, "", err
	}
	changes = strings.TrimSpace(changes)
	if changes == "" {
		return false, "", nil
	}
	return true, fmt.Sprintf("these commits changed its Java or build inputs after its last rebuild in %.12s:\n%s", last, changes), nil
}

// refresh leaves a runtime built from HEAD's Java in the working tree: it does nothing when no commit has changed the inputs since the last rebuild, and otherwise dispatches the runtime workflow on main, which must be HEAD, waits for it and copies the files it built over the committed ones.
func (r *rebuild) refresh(root string) error {
	stale, why, err := runtimeStale(root)
	if err != nil {
		return err
	}
	if !stale {
		fmt.Println("the Android runtime is current with its Java; it is left as it is")
		return nil
	}
	fmt.Printf("the committed Android runtime is stale: %s\n", why)
	if err := r.fetch(root); err != nil {
		return fmt.Errorf("rebuilding the Android runtime: %w; %s", err, manualRebuild)
	}
	return nil
}

func (r *rebuild) fetch(root string) error {
	head, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	head = strings.TrimSpace(head)
	tip, err := r.gh.mainTip()
	if err != nil {
		return err
	}
	// The workflow builds main's tip, so the files it gives are this release's only when the release is cut from that tip.
	if tip != head {
		return fmt.Errorf("main's tip on GitHub is %.12s but HEAD is %.12s; cut the release on an up-to-date main (git fetch, then a branch from origin/main)", tip, head)
	}
	// A run this dispatch starts is newer than every run listed before it, and run IDs grow, so a new ID at main's tip is ours whatever the clocks say.
	before, err := r.gh.dispatches()
	if err != nil {
		return err
	}
	var newest int64
	for _, run := range before {
		newest = max(newest, run.ID)
	}
	if err := r.gh.dispatch(); err != nil {
		return err
	}
	run, err := r.started(newest, tip)
	if err != nil {
		return err
	}
	fmt.Printf("building the Android runtime at %.12s in %s\n", tip, run.URL)
	if run, err = r.finished(run); err != nil {
		return err
	}
	if run.Conclusion != "success" {
		return fmt.Errorf("the run %s ended %s", run.URL, run.Conclusion)
	}
	return r.install(root, run)
}

// started waits for the run the dispatch started: the first one newer than after built at tip.
func (r *rebuild) started(after int64, tip string) (workflowRun, error) {
	for waited := time.Duration(0); ; waited += r.poll {
		runs, err := r.gh.dispatches()
		if err != nil {
			return workflowRun{}, err
		}
		var found *workflowRun
		for i := range runs {
			if runs[i].ID > after && runs[i].HeadSHA == tip && (found == nil || runs[i].ID < found.ID) {
				found = &runs[i]
			}
		}
		if found != nil {
			return *found, nil
		}
		if waited >= r.start {
			return workflowRun{}, fmt.Errorf("no %s run at %.12s appeared within %v of dispatching it", runtimeWorkflow, tip, r.start)
		}
		r.sleep(r.poll)
	}
}

func (r *rebuild) finished(run workflowRun) (workflowRun, error) {
	for waited := time.Duration(0); run.Status != "completed"; waited += r.poll {
		if waited >= r.finish {
			return run, fmt.Errorf("the run %s did not finish within %v", run.URL, r.finish)
		}
		r.sleep(r.poll)
		var err error
		if run, err = r.gh.run(run.ID); err != nil {
			return run, err
		}
	}
	return run, nil
}

// install downloads the run's files into a scratch directory, checks both are there and what they claim to be, and only then replaces the committed ones.
func (r *rebuild) install(root string, run workflowRun) error {
	tmp, err := os.MkdirTemp("", "nsgo-android-runtime-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := r.gh.download(run.ID, tmp); err != nil {
		return err
	}
	built := map[string][]byte{}
	for _, f := range runtimeFiles {
		data, err := os.ReadFile(filepath.Join(tmp, f))
		if err != nil {
			return fmt.Errorf("the %s artifact of %s: %w", runtimeArtifact, run.URL, err)
		}
		if !bytes.HasPrefix(data, runtimeMagic[f]) {
			return fmt.Errorf("the %s artifact of %s holds a %s that is not one", runtimeArtifact, run.URL, f)
		}
		built[f] = data
	}
	var changed []string
	for _, f := range runtimeFiles {
		path := filepath.Join(root, runtimeDir, f)
		if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, built[f]) {
			continue
		}
		if err := os.WriteFile(path, built[f], 0o644); err != nil {
			return err
		}
		changed = append(changed, f)
	}
	if len(changed) == 0 {
		fmt.Println("the rebuilt Android runtime is the committed one byte for byte; nothing changed")
		return nil
	}
	fmt.Printf("wrote the rebuilt %s to %s; commit them with the changelog\n", strings.Join(changed, " and "), runtimeDir)
	return nil
}

// ghCLI is actions through the gh command, which needs a gh logged in with the right to run workflows.
type ghCLI struct{}

func (ghCLI) gh(args ...string) ([]byte, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, errors.New("the gh command is not installed (https://cli.github.com)")
	}
	cmd := exec.Command("gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func (g ghCLI) mainTip() (string, error) {
	out, err := g.gh("api", "repos/"+runtimeRepo+"/commits/main", "--jq", ".sha")
	return strings.TrimSpace(string(out)), err
}

const runFields = "databaseId,headSha,status,conclusion,url"

func (g ghCLI) dispatches() ([]workflowRun, error) {
	out, err := g.gh("run", "list", "-R", runtimeRepo, "--workflow", runtimeWorkflow, "--event", "workflow_dispatch", "--limit", "20", "--json", runFields)
	if err != nil {
		return nil, err
	}
	var runs []workflowRun
	if err := json.Unmarshal(out, &runs); err != nil {
		return nil, fmt.Errorf("gh run list: %w", err)
	}
	return runs, nil
}

func (g ghCLI) dispatch() error {
	_, err := g.gh("workflow", "run", runtimeWorkflow, "-R", runtimeRepo, "--ref", "main")
	return err
}

func (g ghCLI) run(id int64) (workflowRun, error) {
	var run workflowRun
	out, err := g.gh("run", "view", strconv.FormatInt(id, 10), "-R", runtimeRepo, "--json", runFields)
	if err != nil {
		return run, err
	}
	if err := json.Unmarshal(out, &run); err != nil {
		return run, fmt.Errorf("gh run view: %w", err)
	}
	return run, nil
}

func (g ghCLI) download(id int64, dir string) error {
	_, err := g.gh("run", "download", strconv.FormatInt(id, 10), "-R", runtimeRepo, "-n", runtimeArtifact, "-D", dir)
	return err
}
