package main

import (
	"os"
	"strings"
	"testing"
)

// Every target the exported table names has a case in the build dispatch, so the two cannot drift (#2259).
func TestEveryTargetHasABuildCase(t *testing.T) {
	src, err := os.ReadFile("build.go")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(src), "\n")
	handled := map[string]bool{}
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "case ") {
			continue
		}
		label := strings.TrimSuffix(strings.TrimSpace(line), ":")
		for _, part := range strings.Split(label[len("case "):], ",") {
			handled[strings.Trim(strings.TrimSpace(part), `"`)] = true
		}
	}
	if len(handled) == 0 {
		t.Fatal("no build cases were read")
	}
	// A group of targets may be routed by a prefix before the switch, as the terminal targets are.
	var routed []string
	for _, line := range lines {
		const guard = `strings.HasPrefix(target, "`
		if i := strings.Index(line, guard); i >= 0 {
			if j := strings.Index(line[i+len(guard):], `")`); j > 0 {
				routed = append(routed, line[i+len(guard):i+len(guard)+j])
			}
		}
	}
	for _, name := range targets {
		if handled[name] {
			continue
		}
		if _, ok := miniProgramPlatformOf(name); ok {
			continue
		}
		ok := false
		for _, prefix := range routed {
			if strings.HasPrefix(name, prefix) {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("target %q has no case or prefix route in build.go", name)
		}
	}
}
