package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// galleryShards are the patterns the gallery job runs, one runner each; a test must match exactly one of them.
var galleryShards = []string{"^Test[A-M]", "^Test[N-Z]"}

// galleryWorkflow is the part of test.yml the shard checks read.
type galleryWorkflow struct {
	Jobs map[string]struct {
		Name     string
		Needs    any
		If       string
		Outputs  map[string]string
		Strategy struct {
			Matrix map[string]any
		}
		Steps []struct {
			Name string
			Run  string
			Env  map[string]string
		}
	}
}

func readGalleryWorkflow(t *testing.T) galleryWorkflow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "test.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow galleryWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	return workflow
}

// galleryTests returns every test function the gallery declares; TestMain sets the tests up and is none of them.
func galleryTests(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "examples", "gallery", "*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	fset := token.NewFileSet()
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Name.Name == "TestMain" {
				continue
			}
			names = append(names, fn.Name.Name)
		}
	}
	return names
}

// The gallery job's matrix has one shard per pattern, and its gallery step picks those patterns, in that order, by the shard number.
func TestGalleryShardsMatchTheWorkflow(t *testing.T) {
	workflow := readGalleryWorkflow(t)
	job, ok := workflow.Jobs["gallery"]
	if !ok {
		t.Fatal("test.yml has no gallery job")
	}
	var shards []int
	if list, ok := job.Strategy.Matrix["shard"].([]any); ok {
		for _, shard := range list {
			n, _ := shard.(int)
			shards = append(shards, n)
		}
	}
	var want []int
	for i := range galleryShards {
		want = append(want, i+1)
	}
	if !slices.Equal(shards, want) {
		t.Errorf("the gallery job's matrix has shards %v, want %v", shards, want)
	}
	var patterns []string
	for _, step := range job.Steps {
		if step.Name != "Test gallery on the target" {
			continue
		}
		if !strings.Contains(step.Run, `-run "$GALLERY_TESTS"`) {
			t.Errorf("the gallery step does not run $GALLERY_TESTS: %s", step.Run)
		}
		for _, quoted := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(step.Env["GALLERY_TESTS"], -1) {
			patterns = append(patterns, quoted[1])
		}
	}
	if !slices.Equal(patterns, galleryShards) {
		t.Errorf("the gallery step runs %q, want %q", patterns, galleryShards)
	}
}

// The required check Gallery (web) waits for the gallery job and reads the success of every web shard, so no shard runs outside what the merge queue gates on. It runs after a failed shard, as a skipped required check counts as passed, but not in a cancelled run, where it would fail a check nobody finished.
func TestGalleryWebCheckCoversEveryShard(t *testing.T) {
	workflow := readGalleryWorkflow(t)
	gallery := workflow.Jobs["gallery"]
	check, ok := workflow.Jobs["gallery-web"]
	if !ok {
		t.Fatal("test.yml has no gallery-web job")
	}
	if check.Name != "Gallery (web)" {
		t.Errorf("the gallery-web job is named %q, want the required check's name Gallery (web)", check.Name)
	}
	if needs, _ := check.Needs.(string); needs != "gallery" {
		t.Errorf("the gallery-web job needs %v, want gallery", check.Needs)
	}
	if check.If != "${{ !cancelled() }}" {
		t.Errorf("the gallery-web job runs if %q, want ${{ !cancelled() }}", check.If)
	}
	var env []string
	for _, step := range check.Steps {
		for _, value := range step.Env {
			env = append(env, value)
		}
	}
	for i := range galleryShards {
		output := "web_shard_" + strconv.Itoa(i+1)
		if gallery.Outputs[output] == "" {
			t.Errorf("the gallery job has no output %s", output)
		}
		if !slices.Contains(env, "${{ needs.gallery.outputs."+output+" }}") {
			t.Errorf("the gallery-web job does not read %s", output)
		}
	}
}

// Every gallery test runs in exactly one shard, so the two runners between them keep testing all of the gallery.
func TestGalleryShardsCoverEveryTest(t *testing.T) {
	tests := galleryTests(t)
	if len(tests) == 0 {
		t.Fatal("no gallery tests were read")
	}
	for _, name := range tests {
		matches := 0
		for _, pattern := range galleryShards {
			if regexp.MustCompile(pattern).MatchString(name) {
				matches++
			}
		}
		if matches != 1 {
			t.Errorf("%s runs in %d shards", name, matches)
		}
	}
}

// Neither shard is empty, which a change to the patterns could otherwise do.
func TestGalleryShardsAreBothUsed(t *testing.T) {
	counts := make([]int, len(galleryShards))
	for _, name := range galleryTests(t) {
		for i, pattern := range galleryShards {
			if regexp.MustCompile(pattern).MatchString(name) {
				counts[i]++
			}
		}
	}
	for i, count := range counts {
		if count == 0 {
			t.Errorf("shard %d runs no tests", i+1)
		}
	}
}
