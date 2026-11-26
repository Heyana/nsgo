package main

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// test.yml runs the suite as two jobs of package patterns. A package that
// neither lists is never tested, and one that both list runs twice, so this
// checks the lists against the module's own package list.
func TestSuiteShardsCoverEveryPackage(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/test.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Strategy struct {
				Matrix struct {
					Packages []string
				}
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	shards := workflow.Jobs["test-suite"].Strategy.Matrix.Packages
	if len(shards) < 2 {
		t.Fatalf("test-suite has %d package lists, want two or more", len(shards))
	}

	var seen []string
	for _, shard := range shards {
		list := strings.Fields(shard)
		if len(list) == 0 {
			t.Errorf("a shard lists no packages: %q", shard)
			continue
		}
		for _, pkg := range goList(t, list...) {
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
}

// goList expands the patterns as `go tool nsgo test` would.
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
