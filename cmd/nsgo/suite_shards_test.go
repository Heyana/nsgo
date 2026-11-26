package main

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// test.yml runs the suite as shards of go package patterns, each one run of
// nsgo test. A package that no shard lists is never tested and one that two
// list runs twice; the dev client's nsgodev variant runs in the shard that
// lists its package, so this checks the lists against the module's own
// packages, expanding them as nsgo test does.
func TestSuiteShardsCoverEveryPackage(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/test.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Strategy struct {
				Matrix struct {
					Include []struct {
						Name     string
						Packages string
					}
				}
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	shards := workflow.Jobs["test-suite"].Strategy.Matrix.Include
	if len(shards) < 2 {
		t.Fatalf("test-suite has %d package lists, want two or more", len(shards))
	}

	dev := nsgoModule + "/internal/dev"
	var seen []string
	devShards := 0
	for _, shard := range shards {
		if shard.Name == "" {
			t.Errorf("a shard has no name to show as its job's")
		}
		list := strings.Fields(shard.Packages)
		if len(list) == 0 {
			t.Errorf("the %q shard lists no packages", shard.Name)
			continue
		}
		packages := goList(t, list...)
		if slices.Contains(packages, dev) {
			devShards++
		}
		for _, pkg := range packages {
			if slices.Contains(seen, pkg) {
				t.Errorf("%s is tested by more than one shard", pkg)
			}
			seen = append(seen, pkg)
		}
	}
	all := goList(t, "./...")
	slices.Sort(all)
	slices.Sort(seen)
	if !slices.Equal(all, seen) {
		missing := difference(all, seen)
		extra := difference(seen, all)
		t.Errorf("the shards test %d packages, the module has %d; missing %v, unknown %v", len(seen), len(all), missing, extra)
	}
	if devShards != 1 {
		t.Errorf("the dev client is tested with nsgodev in %d shards, want one: only a shard listing its package adds that run", devShards)
	}
}

// goList expands the patterns as `go tool nsgo test` does: from the module
// root, where a directory names its own package and /... its tree.
func goList(t *testing.T, patterns ...string) []string {
	t.Helper()
	cmd := exec.Command("go", append([]string{"list"}, patterns...)...)
	// The patterns are written from the module root, as test.yml has them.
	cmd.Dir = "../.."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list %v: %v: %s", patterns, err, out)
	}
	return strings.Fields(string(out))
}

func difference(a, b []string) []string {
	var out []string
	for _, v := range a {
		if !slices.Contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}
