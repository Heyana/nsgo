package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSimulatorProcessResults(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("NSGO_SIMULATOR_E2E") != "1" {
		t.Skip("set NSGO_SIMULATOR_E2E=1 to run tests in an iPhone simulator")
	}
	if runtime.GOARCH != "arm64" {
		t.Fatal("NSGo's simulator binaries are arm64; run this check on an Apple silicon Mac")
	}
	for _, fixture := range []string{"simulator", "simulatorplain", "simulatornohost"} {
		t.Run(fixture, func(t *testing.T) {
			p, err := loadForTest(filepath.Join("..", "..", "testdata", fixture), nil)
			if err != nil {
				t.Fatal(err)
			}
			report := filepath.Join(t.TempDir(), "junit.xml")
			output := captureStderr(t, func() {
				err = runTests(t.Context(), p, testOptions{target: "ios_simulator", device: os.Getenv("NSGO_SIMULATOR_DEVICE"),
					run: "^Test(Fast|Crash)$", timeout: 30 * time.Second, junit: report})
			})
			if err == nil {
				t.Fatal("the panic was reported as a success")
			}
			runErr := err
			data, err := os.ReadFile(report)
			if err != nil {
				t.Fatalf("report: %v; run: %v\n%s", err, runErr, output)
			}
			var suites junitSuites
			if err := xml.Unmarshal(data, &suites); err != nil {
				t.Fatal(err)
			}
			if suites.Tests != 2 || suites.Failures != 1 || len(suites.Suites) != 1 {
				t.Fatalf("test results: %s\n%s", data, output)
			}
			for _, result := range suites.Suites[0].Cases {
				switch result.Name {
				case "TestFast":
					if result.Failure != nil {
						t.Fatalf("fast test failed: %s", data)
					}
				case "TestCrash":
					if result.Failure == nil || !strings.Contains(result.Failure.Body, "panic: nsgo simulator crash fixture") || strings.Contains(result.Failure.Body, "exited with 0") {
						t.Fatalf("crash report: %s", data)
					}
				default:
					t.Fatalf("unexpected test result: %s", data)
				}
			}
		})
	}
}

// TestIOSSimulatorDevEndToEnd launches both versions of a signed simulator
// app and checks that a Go-only rebuild leaves the installed bundle in place.
func TestIOSSimulatorDevEndToEnd(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("NSGO_SIMULATOR_E2E") != "1" {
		t.Skip("set NSGO_SIMULATOR_E2E=1 to run tests in an iPhone simulator")
	}
	ctx := t.Context()
	udid, err := bootSimulator(ctx, iOS, os.Getenv("NSGO_SIMULATOR_DEVICE"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := load(filepath.Join("..", "..", "testdata", "simulatordev"))
	if err != nil {
		t.Fatal(err)
	}
	p.ID += fmt.Sprintf(".test%d", os.Getpid())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		run(ctx, "xcrun", "simctl", "uninstall", udid, p.ID)
	})
	var installation iosSimulatorInstall
	var plist os.FileInfo
	inputs := iosSimulatorInputs(p)
	for _, marker := range []string{"first", "second"} {
		p.Build.LDFlags = []string{"-X main.marker=" + marker}
		bundle, err := buildIOSApp(ctx, p, iOS, t.TempDir(), true)
		if err != nil {
			t.Fatal(err)
		}
		if err := installation.update(ctx, p, udid, bundle, inputs); err != nil {
			t.Fatal(err)
		}
		installed, err := output(ctx, "xcrun", "simctl", "get_app_container", udid, p.ID, "app")
		if err != nil {
			t.Fatal(err)
		}
		current, err := os.Stat(filepath.Join(installed, "Info.plist"))
		if err != nil {
			t.Fatal(err)
		}
		if plist != nil && !os.SameFile(plist, current) {
			t.Fatal("a Go-only rebuild replaced the installed bundle")
		}
		plist = current
		data, err := output(ctx, "xcrun", "simctl", "get_app_container", udid, p.ID, "data")
		if err != nil {
			t.Fatal(err)
		}
		markerPath := filepath.Join(data, "Documents", "marker")
		if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := run(ctx, "env", "SIMCTL_CHILD_NSGO_TEST_MARKER="+markerPath, "xcrun", "simctl", "launch", udid, p.ID); err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(15 * time.Second); ; {
			if got, _ := os.ReadFile(markerPath); string(got) == marker {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("the simulator did not run the %s executable", marker)
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err := run(ctx, "xcrun", "simctl", "terminate", udid, p.ID); err != nil {
			t.Fatal(err)
		}
	}
}
