package main

import (
	"bytes"
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFailuresWithoutPackageEvents(t *testing.T) {
	const diagnostic = "bad_test.go:4: undefined: nsgoCompileOnlyMissing"
	const build = `{"ImportPath":"example.com/bad [example.com/bad.test]","Action":"build-output","Output":"bad_test.go:4: undefined: nsgoCompileOnlyMissing\n"}` + "\n" +
		`{"ImportPath":"example.com/bad [example.com/bad.test]","Action":"build-fail"}` + "\n"
	for _, tc := range []struct {
		name, events, label, wantPackage string
		failed                           bool
	}{
		{"compile only", build, "", "example.com/bad [example.com/bad.test]", true},
		{"ordinary package failure", build + `{"Action":"fail","Package":"example.com/bad","FailedBuild":"example.com/bad [example.com/bad.test]"}` + "\n", "", "example.com/bad", true},
		{"labelled compile only", build, devClientLabel, "example.com/bad [example.com/bad.test]" + devClientLabel, true},
		{"successful build output", `{"ImportPath":"example.com/bad","Action":"build-output","Output":"a compiler warning\n"}` + "\n", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			r := &testRun{running: map[string]*testResult{}, packages: map[string]*testResult{}, label: tc.label, out: &out}
			r.read(strings.NewReader(tc.events))
			report := filepath.Join(t.TempDir(), "report.xml")
			err := r.finish(testOptions{junit: report}, nil)
			if tc.failed {
				if err == nil || err.Error() != "tests did not build" {
					t.Errorf("finish = %v, want build failure", err)
				}
				if n := strings.Count(out.String(), diagnostic); n != 1 {
					t.Errorf("compiler diagnostic printed %d times:\n%s", n, out.String())
				}
			} else if err != nil {
				t.Errorf("successful build output caused %v", err)
			}
			data, err := os.ReadFile(report)
			if err != nil {
				t.Fatal(err)
			}
			var suites junitSuites
			if err := xml.Unmarshal(data, &suites); err != nil {
				t.Fatal(err)
			}
			if tc.failed {
				if suites.Tests != 1 || suites.Failures != 1 || len(suites.Suites) != 1 {
					t.Fatalf("JUnit does not contain exactly one build failure: %s", data)
				}
				suite := suites.Suites[0]
				if suite.Name != tc.wantPackage || len(suite.Cases) != 1 || suite.Cases[0].Failure == nil || !strings.Contains(suite.Cases[0].Failure.Body, diagnostic) {
					t.Errorf("JUnit lost the package or compiler diagnostic: %s", data)
				}
			} else if suites.Tests != 0 || suites.Failures != 0 {
				t.Errorf("successful build output produced failed tests: %s", data)
			}
		})
	}
}

// A separate test process captures the tool's console while it consumes the actual Go compile-only event stream, which ends without package fail events.
func TestCompileOnlyFailureIncludesDiagnostics(t *testing.T) {
	const marker = "nsgoCompileOnlyMissing"
	const helper = "NSGO_TEST_COMPILE_FAILURE_DIR"
	if dir := os.Getenv(helper); dir != "" {
		p, err := loadForTest(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		err = runTests(t.Context(), p, testOptions{junit: filepath.Join(dir, "report.xml"), args: []string{"-c", "-o", filepath.Join(dir, "fixture.test")}})
		if err == nil || err.Error() != "tests did not build" {
			t.Fatalf("runTests = %v, want compiler failure", err)
		}
		return
	}
	if testing.Short() {
		t.Skip("runs a real compiler failure")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":      "module example.com/compileonly\n\ngo 1.27\n",
		"bad_test.go": "package compileonly\nimport \"testing\"\nfunc TestBroken(t *testing.T) { " + marker + "() }\n",
	})
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+t.Name()+"$")
	cmd.Env = append(os.Environ(), helper+"="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compile-only helper: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "undefined: "+marker) {
		t.Errorf("console lost the compiler diagnostic:\n%s", out)
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var suites junitSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatal(err)
	}
	if suites.Failures != 1 || !strings.Contains(string(data), "undefined: "+marker) {
		t.Errorf("JUnit lost the compile-only failure: %s", data)
	}
}
