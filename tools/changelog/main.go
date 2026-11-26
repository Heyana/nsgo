// Command changelog keeps CHANGELOG.md's `Unreleased` section in fragments: each pull request adds a file to changelog.d instead of a line at the top of the section, so pull requests do not conflict with each other there, and a release folds the fragments into CHANGELOG.md.
//
//	go run ./tools/changelog check
//	go run ./tools/changelog collect
//	go run ./tools/changelog release [-date YYYY-MM-DD] [-no-runtime] X.Y.Z
//	go run ./tools/changelog adopt -base origin/main -kind Fixed <name>
//
// check validates the fragments; collect moves them into `Unreleased` and deletes them; release collects them and turns `Unreleased` into the version's section above a fresh one, once every release nsgo names for a command (commandReleases in cmd/nsgo/delegate.go) is a tag or this release, and after replacing a stale Android runtime with one the android-runtime.yml workflow builds on main (runtime.go), unless -no-runtime; adopt moves the `Unreleased` lines a branch added to CHANGELOG.md into a fragment, for branches started before fragments.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	changelogFile = "CHANGELOG.md"
	fragmentDir   = "changelog.d"
	// readme explains the directory to contributors; it is not a fragment and keeps the directory in Git when no fragment is left.
	readme            = "README.md"
	unreleasedHeading = "## [Unreleased]"
)

// kinds are Keep a Changelog's types, in the order a release lists their bullets. CHANGELOG.md has no heading per type: its sections are bullets ordered by type, so the type only orders them.
var kinds = []string{"Added", "Changed", "Deprecated", "Removed", "Fixed", "Security"}

var (
	fragmentName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*\.md$`)
	versionRE    = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)
)

// fragment is one file of changelog.d: its bullets by kind, each a whole line starting with "- ".
type fragment struct {
	name    string
	bullets [][]string
}

func main() {
	if err := run(".", os.Args[1:], time.Now()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() error {
	return errors.New("usage: changelog check | collect | release [-date YYYY-MM-DD] [-no-runtime] X.Y.Z | adopt -base REV -kind KIND NAME")
}

func run(root string, args []string, now time.Time) error {
	if len(args) == 0 {
		return usage()
	}
	switch cmd, rest := args[0], args[1:]; cmd {
	case "check":
		if len(rest) != 0 {
			return usage()
		}
		frags, err := readFragments(root)
		if err != nil {
			return err
		}
		fmt.Printf("%d changelog fragments\n", len(frags))
		return nil
	case "collect":
		if len(rest) != 0 {
			return usage()
		}
		n, err := collect(root)
		if err != nil {
			return err
		}
		fmt.Printf("collected %d fragments into %s\n", n, changelogFile)
		return nil
	case "release":
		fs := flag.NewFlagSet("release", flag.ContinueOnError)
		date := fs.String("date", now.UTC().Format(time.DateOnly), "the release's date")
		noRuntime := fs.Bool("no-runtime", false, "leave the Android runtime as committed rather than rebuilding it when its Java changed")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return usage()
		}
		var rt *rebuild
		if !*noRuntime {
			rt = newRebuild()
		}
		return release(root, fs.Arg(0), *date, rt)
	case "adopt":
		fs := flag.NewFlagSet("adopt", flag.ContinueOnError)
		base := fs.String("base", "", "the branch the change merges into, such as origin/main")
		kind := fs.String("kind", "", "the bullets' type: "+strings.Join(kinds, ", "))
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if fs.NArg() != 1 || *base == "" || *kind == "" {
			return usage()
		}
		return adopt(root, *base, *kind, fs.Arg(0))
	}
	return usage()
}

func kindIndex(name string) int {
	return slices.IndexFunc(kinds, func(k string) bool { return strings.EqualFold(k, name) })
}

// parseFragment reads a fragment: "### Kind" headings, each followed by one-line bullets.
func parseFragment(name string, data []byte) (fragment, error) {
	f := fragment{name: name, bullets: make([][]string, len(kinds))}
	kind, count := -1, 0
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, " \t\r")
		switch {
		case line == "":
		case strings.HasPrefix(line, "#"):
			title, ok := strings.CutPrefix(line, "### ")
			kind = kindIndex(strings.TrimSpace(title))
			if !ok || kind < 0 {
				return f, fmt.Errorf("%s:%d: a heading is ### and one of %s", name, i+1, strings.Join(kinds, ", "))
			}
		case strings.HasPrefix(line, "- ") && strings.TrimSpace(line[2:]) != "":
			if kind < 0 {
				return f, fmt.Errorf("%s:%d: a bullet goes under a ### heading naming its type", name, i+1)
			}
			f.bullets[kind] = append(f.bullets[kind], line)
			count++
		default:
			return f, fmt.Errorf("%s:%d: a fragment holds only ### headings and one-line \"- \" bullets", name, i+1)
		}
	}
	if count == 0 {
		return f, fmt.Errorf("%s: no bullets", name)
	}
	return f, nil
}

// readFragments reads changelog.d in name order, reporting every invalid file at once.
func readFragments(root string) ([]fragment, error) {
	entries, err := os.ReadDir(filepath.Join(root, fragmentDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var frags []fragment
	var errs []error
	for _, e := range entries {
		name := e.Name()
		if name == readme {
			continue
		}
		if e.IsDir() || !fragmentName.MatchString(name) {
			errs = append(errs, fmt.Errorf("%s/%s: a fragment is a file named in kebab case ending in .md, such as keep-caret-after-paste.md", fragmentDir, name))
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, fragmentDir, name))
		if err != nil {
			return nil, err
		}
		f, err := parseFragment(fragmentDir+"/"+name, data)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		frags = append(frags, f)
	}
	return frags, errors.Join(errs...)
}

// unreleased splits a changelog around its `Unreleased` section's body: what comes before and including the heading, the body's lines without the blank lines around them, and the next section on.
func unreleased(text string) (head string, body []string, tail string, err error) {
	lines := strings.SplitAfter(text, "\n")
	start := slices.IndexFunc(lines, func(l string) bool { return strings.TrimRight(l, " \t\r\n") == unreleasedHeading })
	if start < 0 {
		return "", nil, "", fmt.Errorf("%s has no %q heading", changelogFile, unreleasedHeading)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	for _, l := range lines[start+1 : end] {
		body = append(body, strings.TrimRight(l, "\r\n"))
	}
	for len(body) > 0 && strings.TrimSpace(body[0]) == "" {
		body = body[1:]
	}
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	return strings.Join(lines[:start+1], ""), body, strings.Join(lines[end:], ""), nil
}

// section writes a heading's body followed by the rest of the file, with one blank line between them.
func section(head string, body []string, tail string) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(head, "\r\n"))
	b.WriteString("\n\n")
	if len(body) > 0 {
		b.WriteString(strings.Join(body, "\n"))
		b.WriteString("\n")
		if tail != "" {
			b.WriteString("\n")
		}
	}
	b.WriteString(tail)
	return b.String()
}

// fold puts the fragments' bullets at the top of `Unreleased`, by kind and then by fragment name, above the lines already there.
func fold(text string, frags []fragment) (string, error) {
	head, body, tail, err := unreleased(text)
	if err != nil {
		return "", err
	}
	var added []string
	for k := range kinds {
		for _, f := range frags {
			added = append(added, f.bullets[k]...)
		}
	}
	return section(head, append(added, body...), tail), nil
}

// collect folds changelog.d into CHANGELOG.md, writing the changelog before deleting any fragment.
func collect(root string) (int, error) {
	frags, err := readFragments(root)
	if err != nil {
		return 0, err
	}
	path := filepath.Join(root, changelogFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	text, err := fold(string(data), frags)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return 0, err
	}
	for _, f := range frags {
		if err := os.Remove(filepath.Join(root, f.name)); err != nil {
			return 0, err
		}
	}
	return len(frags), nil
}

// cut turns `Unreleased` into the version's section, above a fresh, empty `Unreleased`.
func cut(text, version, date string) (string, error) {
	if !versionRE.MatchString(version) {
		return "", fmt.Errorf("version %q is not X.Y.Z or X.Y.Z-suffix", version)
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return "", fmt.Errorf("date %q is not YYYY-MM-DD", date)
	}
	heading := "## [" + version + "]"
	for _, l := range strings.Split(text, "\n") {
		if l == heading || strings.HasPrefix(l, heading+" ") {
			return "", fmt.Errorf("%s already has a %s section", changelogFile, heading)
		}
	}
	head, body, tail, err := unreleased(text)
	if err != nil {
		return "", err
	}
	if len(body) == 0 {
		return "", fmt.Errorf("%s's Unreleased section is empty: nothing to release", changelogFile)
	}
	return section(head, nil, section(heading+" - "+date, body, tail)), nil
}

// release cuts the version's section; with rt, it first brings the Android runtime up to date with its Java, so the release's pull request carries the files release.yml checks.
func release(root, version, date string, rt *rebuild) error {
	path := filepath.Join(root, changelogFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	// Cut a copy first, so a bad version or an existing section leaves the fragments in place.
	frags, err := readFragments(root)
	if err != nil {
		return err
	}
	folded, err := fold(string(data), frags)
	if err != nil {
		return err
	}
	if _, err := cut(folded, version, date); err != nil {
		return err
	}
	if err := checkCommandReleases(root, version); err != nil {
		return err
	}
	if rt != nil {
		if err := rt.refresh(root); err != nil {
			return err
		}
	}
	n, err := collect(root)
	if err != nil {
		return err
	}
	data, err = os.ReadFile(path)
	if err != nil {
		return err
	}
	text, err := cut(string(data), version, date)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return err
	}
	fmt.Printf("collected %d fragments and wrote the %s section of %s; review it, then merge it and tag v%s\n", n, version, changelogFile, version)
	return nil
}

// delegateFile holds commandReleases, the release in which each of nsgo's newer commands first ships, which an installed nsgo names to a module that pins an older one.
const delegateFile = "cmd/nsgo/delegate.go"

// checkCommandReleases refuses a release while commandReleases names a release that neither is tagged nor is this one, X.Y.Z or the X.Y.Z a pre-release leads to: a command added for one release and shipped in another would send users to a version without it.
func checkCommandReleases(root, version string) error {
	path := filepath.Join(root, filepath.FromSlash(delegateFile))
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return err
	}
	var releases map[string]string
	ast.Inspect(f, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != "commandReleases" || len(spec.Values) != 1 {
			return true
		}
		if lit, ok := spec.Values[0].(*ast.CompositeLit); ok {
			releases = map[string]string{}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				k, kerr := stringLiteral(kv.Key)
				v, verr := stringLiteral(kv.Value)
				if kerr == nil && verr == nil {
					releases[k] = v
				}
			}
		}
		return false
	})
	if releases == nil {
		return fmt.Errorf("%s has no commandReleases map literal", delegateFile)
	}
	this, _, _ := strings.Cut(version, "-")
	var stale []string
	for cmd, release := range releases {
		if release == "v"+version || release == "v"+this {
			continue
		}
		if _, err := git(root, "rev-parse", "-q", "--verify", "refs/tags/"+release+"^{commit}"); err == nil {
			continue
		}
		stale = append(stale, fmt.Sprintf("%s: %s", cmd, release))
	}
	if len(stale) == 0 {
		return nil
	}
	slices.Sort(stale)
	return fmt.Errorf("%s's commandReleases names releases that are neither tagged nor v%s (%s): set them to v%s, or fetch the tags with git fetch --tags", delegateFile, this, strings.Join(stale, ", "), this)
}

// stringLiteral is the value of the Go string literal e.
func stringLiteral(e ast.Expr) (string, error) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", errors.New("not a string literal")
	}
	return strconv.Unquote(lit.Value)
}

// branchBullets returns the `Unreleased` bullets the working copy's changelog adds to the base's and the changelog to restore, the base's. Without those bullets the working copy must be the base: a branch that also changed other lines needs a person to move them.
func branchBullets(base, work string) (added []string, restored string, err error) {
	_, baseBody, _, err := unreleased(base)
	if err != nil {
		return nil, "", err
	}
	head, body, tail, err := unreleased(work)
	if err != nil {
		return nil, "", err
	}
	remaining := map[string]int{}
	for _, l := range baseBody {
		remaining[l]++
	}
	var kept []string
	for _, l := range body {
		if remaining[l] > 0 {
			remaining[l]--
			kept = append(kept, l)
			continue
		}
		if strings.TrimSpace(l) == "" {
			kept = append(kept, l)
			continue
		}
		if !strings.HasPrefix(l, "- ") {
			return nil, "", fmt.Errorf("the branch adds %q to Unreleased, which is not a bullet; move it by hand", l)
		}
		added = append(added, l)
	}
	if normalize(section(head, kept, tail)) != normalize(base) {
		return nil, "", fmt.Errorf("the branch changes %s beyond adding Unreleased bullets, such as editing an existing line; move those changes by hand", changelogFile)
	}
	if len(added) == 0 {
		return nil, "", fmt.Errorf("the branch adds no bullet to %s's Unreleased section", changelogFile)
	}
	return added, base, nil
}

func normalize(s string) string { return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n")) }

func adopt(root, base, kind, name string) error {
	k := kindIndex(kind)
	if k < 0 {
		return fmt.Errorf("kind %q is not one of %s", kind, strings.Join(kinds, ", "))
	}
	file := strings.TrimSuffix(name, ".md") + ".md"
	if strings.EqualFold(file, readme) || !fragmentName.MatchString(file) {
		return fmt.Errorf("name %q is not kebab case, such as keep-caret-after-paste", name)
	}
	out := filepath.Join(root, fragmentDir, file)
	if _, err := os.Stat(out); err == nil {
		return fmt.Errorf("%s/%s already exists", fragmentDir, file)
	}
	mergeBase, err := git(root, "merge-base", base, "HEAD")
	if err != nil {
		return err
	}
	baseText, err := git(root, "show", strings.TrimSpace(mergeBase)+":"+changelogFile)
	if err != nil {
		return err
	}
	path := filepath.Join(root, changelogFile)
	work, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	added, rest, err := branchBullets(baseText, string(work))
	if err != nil {
		return err
	}
	frag := "### " + kinds[k] + "\n\n" + strings.Join(added, "\n") + "\n"
	if _, err := parseFragment(file, []byte(frag)); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, []byte(frag), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(rest), 0o644); err != nil {
		return err
	}
	fmt.Printf("moved %d bullets to %s/%s and restored %s to %s's; check the type, then fold both into the commit that added the bullets (git commit --amend on a one-commit branch) so a rebase stops replaying that edit\n", len(added), fragmentDir, file, changelogFile, base)
	return nil
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}
