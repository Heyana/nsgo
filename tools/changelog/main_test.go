package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sample = `# Changelog

## [Unreleased]

- Keep an older line.

## [0.2.0] - 2026-11-19

- Add something released.
`

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// repo makes a changelog and fragments in a temporary directory.
func repo(t *testing.T, changelog string, frags map[string]string) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, changelogFile), changelog)
	write(t, filepath.Join(root, fragmentDir, readme), "# Changelog fragments\n\n### Not a fragment\n")
	for name, data := range frags {
		write(t, filepath.Join(root, fragmentDir, name), data)
	}
	return root
}

func TestParseFragment(t *testing.T) {
	f, err := parseFragment("x.md", []byte("### Fixed\r\n\r\n- Fix one.\n- Fix two.  \n\n### added\n- Add one.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.bullets[kindIndex("Fixed")], "|"); got != "- Fix one.|- Fix two." {
		t.Errorf("fixed bullets %q", got)
	}
	if got := strings.Join(f.bullets[kindIndex("Added")], "|"); got != "- Add one." {
		t.Errorf("added bullets %q", got)
	}
	for data, want := range map[string]string{
		"- No heading.\n":                      "under a ### heading",
		"### Improved\n- Something.\n":         "one of Added",
		"## Fixed\n- Something.\n":             "one of Added",
		"### Fixed\n- One line\n  continued\n": "only ### headings",
		"### Fixed\n":                          "no bullets",
		"### Fixed\n-\n":                       "only ### headings",
		"":                                     "no bullets",
	} {
		if _, err := parseFragment("x.md", []byte(data)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want an error containing %q", data, err, want)
		}
	}
}

func TestReadFragmentsReportsEveryBadFile(t *testing.T) {
	root := repo(t, sample, map[string]string{
		"good.md":      "### Fixed\n- Fix.\n",
		"Bad_Name.md":  "### Fixed\n- Fix.\n",
		"notes.txt":    "### Fixed\n- Fix.\n",
		"no-kind.md":   "- Fix.\n",
		"also-good.md": "### Added\n- Add.\n",
	})
	frags, err := readFragments(root)
	if err == nil {
		t.Fatal("accepted invalid fragments")
	}
	for _, want := range []string{"Bad_Name.md", "notes.txt", "no-kind.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}
	if len(frags) != 2 {
		t.Errorf("read %d valid fragments, want 2", len(frags))
	}
	if frags, err := readFragments(t.TempDir()); err != nil || frags != nil {
		t.Errorf("a missing directory: %v, %v", frags, err)
	}
}

func TestCollectOrdersByKindThenName(t *testing.T) {
	root := repo(t, sample, map[string]string{
		"b-fix.md":    "### Fixed\n- Fix b.\n\n### Added\n- Add b.\n",
		"a-fix.md":    "### Fixed\n- Fix a.\n",
		"c-remove.md": "### Removed\n- Remove c.\n",
	})
	n, err := collect(root)
	if err != nil || n != 3 {
		t.Fatalf("collect: %d, %v", n, err)
	}
	want := `# Changelog

## [Unreleased]

- Add b.
- Remove c.
- Fix a.
- Fix b.
- Keep an older line.

## [0.2.0] - 2026-11-19

- Add something released.
`
	if got := read(t, filepath.Join(root, changelogFile)); got != want {
		t.Errorf("changelog:\n%s\nwant:\n%s", got, want)
	}
	entries, err := os.ReadDir(filepath.Join(root, fragmentDir))
	if err != nil || len(entries) != 1 || entries[0].Name() != readme {
		t.Errorf("left %v, %v; want only the README", entries, err)
	}
}

func TestCollectIntoEmptyUnreleased(t *testing.T) {
	root := repo(t, "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-11-19\n\n- Add x.\n", map[string]string{"y.md": "### Fixed\n- Fix y.\n"})
	if _, err := collect(root); err != nil {
		t.Fatal(err)
	}
	want := "# Changelog\n\n## [Unreleased]\n\n- Fix y.\n\n## [0.2.0] - 2026-11-19\n\n- Add x.\n"
	if got := read(t, filepath.Join(root, changelogFile)); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCollectLeavesFragmentsOnError(t *testing.T) {
	root := repo(t, "# Changelog\n", map[string]string{"y.md": "### Fixed\n- Fix y.\n"})
	if _, err := collect(root); err == nil || !strings.Contains(err.Error(), "Unreleased") {
		t.Fatalf("collect without an Unreleased heading: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, fragmentDir, "y.md")); err != nil {
		t.Errorf("fragment removed: %v", err)
	}
}

func TestRelease(t *testing.T) {
	root := repo(t, sample, map[string]string{"new.md": "### Added\n- Add new.\n"})
	if err := run(root, []string{"release", "-no-runtime", "-date", "2026-12-01", "0.3.0"}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	want := `# Changelog

## [Unreleased]

## [0.3.0] - 2026-12-01

- Add new.
- Keep an older line.

## [0.2.0] - 2026-11-19

- Add something released.
`
	if got := read(t, filepath.Join(root, changelogFile)); got != want {
		t.Errorf("changelog:\n%s\nwant:\n%s", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, fragmentDir, "new.md")); !os.IsNotExist(err) {
		t.Errorf("fragment kept: %v", err)
	}
}

func TestReleaseDefaultsToToday(t *testing.T) {
	root := repo(t, sample, nil)
	if err := run(root, []string{"release", "-no-runtime", "0.3.0-rc.1"}, time.Date(2026, 12, 2, 23, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(root, changelogFile)); !strings.Contains(got, "\n## [0.3.0-rc.1] - 2026-12-02\n\n- Keep an older line.\n") {
		t.Errorf("changelog:\n%s", got)
	}
}

func TestReleaseRefusesBeforeTouchingFragments(t *testing.T) {
	for _, tc := range []struct {
		changelog string
		args      []string
		want      string
	}{
		{sample, []string{"release", "-no-runtime", "0.2.0"}, "already has"},
		{sample, []string{"release", "-no-runtime", "v0.3.0"}, "is not X.Y.Z"},
		{sample, []string{"release", "-no-runtime", "-date", "12/01/2026", "0.3.0"}, "is not YYYY-MM-DD"},
	} {
		root := repo(t, tc.changelog, map[string]string{"new.md": "### Added\n- Add new.\n"})
		if err := run(root, tc.args, time.Now()); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: %v, want %q", tc.args, err, tc.want)
		}
		if got := read(t, filepath.Join(root, changelogFile)); got != tc.changelog {
			t.Errorf("%v rewrote the changelog", tc.args)
		}
		if _, err := os.Stat(filepath.Join(root, fragmentDir, "new.md")); err != nil {
			t.Errorf("%v removed the fragment: %v", tc.args, err)
		}
	}
	empty := repo(t, "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-11-19\n\n- Add x.\n", nil)
	if err := run(empty, []string{"release", "-no-runtime", "0.3.0"}, time.Now()); err == nil || !strings.Contains(err.Error(), "nothing to release") {
		t.Errorf("empty release: %v", err)
	}
}

func TestBranchBullets(t *testing.T) {
	work := strings.Replace(sample, "- Keep an older line.", "- Fix the branch's change.\n- Keep an older line.\n- Add another.", 1)
	added, restored, err := branchBullets(sample, work)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(added, "|"); got != "- Fix the branch's change.|- Add another." {
		t.Errorf("added %q", got)
	}
	if restored != sample {
		t.Errorf("restored %q", restored)
	}
	for work, want := range map[string]string{
		sample: "adds no bullet",
		strings.Replace(sample, "- Keep an older line.", "- Keep an edited line.", 1):                  "beyond adding",
		strings.Replace(sample, "- Add something released.", "- Add something released.\n- Sneak.", 1): "beyond adding",
		strings.Replace(sample, "- Keep an older line.", "Prose.\n- Keep an older line.", 1):           "not a bullet",
	} {
		if _, _, err := branchBullets(sample, work); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", work, err, want)
		}
	}
}

func TestAdopt(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := repo(t, sample, nil)
	gitT := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitT("init", "-q", "-b", "main")
	gitT("add", ".")
	gitT("commit", "-q", "-m", "base")
	gitT("checkout", "-q", "-b", "feature")
	work := strings.Replace(sample, "## [Unreleased]\n\n", "## [Unreleased]\n\n- Fix the feature.\n", 1)
	write(t, filepath.Join(root, changelogFile), work)
	gitT("commit", "-q", "-am", "feature")

	if err := run(root, []string{"adopt", "-base", "main", "-kind", "fixed", "fix-feature"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(root, changelogFile)); got != sample {
		t.Errorf("changelog not restored:\n%s", got)
	}
	if got, want := read(t, filepath.Join(root, fragmentDir, "fix-feature.md")), "### Fixed\n\n- Fix the feature.\n"; got != want {
		t.Errorf("fragment %q, want %q", got, want)
	}
	if err := run(root, []string{"adopt", "-base", "main", "-kind", "Fixed", "fix-feature"}, time.Now()); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second adopt: %v", err)
	}
	for _, args := range [][]string{
		{"adopt", "-base", "main", "-kind", "Improved", "x"},
		{"adopt", "-base", "main", "-kind", "Fixed", "Bad_Name"},
		{"adopt", "-base", "main", "-kind", "Fixed", "readme"},
		{"adopt", "-kind", "Fixed", "x"},
	} {
		if err := run(root, args, time.Now()); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
}

// TestRepositoryFragments keeps the repository's own fragments valid, so a malformed one fails the pull request that adds it rather than the release.
func TestRepositoryFragments(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, changelogFile)); err != nil {
		t.Skipf("no source checkout: %v", err)
	}
	if _, err := readFragments(root); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(root, fragmentDir, readme)); err != nil {
		t.Error(err)
	}
	if _, _, _, err := unreleased(read(t, filepath.Join(root, changelogFile))); err != nil {
		t.Error(err)
	}
}
