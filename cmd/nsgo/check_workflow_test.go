package main

import (
	"os"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// check.yml splits -target all between a Linux job and a Mac one, as Apple's targets need Xcode, so a target added to targets must join one of its lists or no workflow vets it. The Linux job also names the HarmonyOS preview, which all leaves out, so its nsgoharmony backend is vetted too, and the tui group, which stands for every terminal target.
func TestCheckWorkflowVetsEveryTargetOnce(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/check.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Run string
				Env map[string]string
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			// A step with its own GOFLAGS vets extra build tags, such as Android host development, on top of the default lists.
			if step.Env["GOFLAGS"] != "" {
				continue
			}
			if list, ok := strings.CutPrefix(step.Run, "go tool nsgo check -target "); ok {
				// A flag after the list, such as -jobs, is no target.
				list, _, _ = strings.Cut(list, " ")
				// A group such as tui stands for its targets, as nsgo check expands it; each name expands on its own so a target listed twice still shows twice.
				for _, name := range strings.Split(strings.TrimSpace(list), ",") {
					expanded, err := expandTargets([]string{name})
					if err != nil {
						t.Fatal(err)
					}
					got = append(got, expanded...)
				}
			}
		}
	}
	slices.Sort(got)
	want := append(goAndXcodeTargets(), "harmonyos")
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("check.yml vets %v, want each required target %v once", got, want)
	}
}
