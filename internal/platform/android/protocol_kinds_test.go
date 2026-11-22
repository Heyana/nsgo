package android

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func readJavaProtocol(t *testing.T) string {
	t.Helper()
	testhost.RequireSource(t)
	source, err := os.ReadFile("java/dev/oneo/nsgo/Protocol.java")
	if err != nil {
		t.Fatal(err)
	}
	return string(source)
}

func TestJavaProtocolDetectsEveryKindMutation(t *testing.T) {
	source := readJavaProtocol(t)
	pkg := driverProtocolConstants(t)
	// Mutate each shipped declaration independently, including aliases and the preflight constants previously checked by hand. No Java compiler is needed.
	declarations := regexp.MustCompile(`static final int ((?:ITEM|EVENT)_\w+) = (\d+);`).FindAllStringSubmatch(source, -1)
	if len(declarations) == 0 {
		t.Fatal("Protocol.java has no kind declarations to mutate")
	}
	for _, declaration := range declarations {
		t.Run(declaration[1], func(t *testing.T) {
			value, err := strconv.Atoi(declaration[2])
			if err != nil {
				t.Fatal(err)
			}
			mutated := strings.Replace(source, declaration[0], "static final int "+declaration[1]+" = "+strconv.Itoa(value+1)+";", 1)
			problems := javaProtocolKindErrors(mutated, pkg)
			if len(problems) != 1 || !strings.Contains(problems[0], declaration[1]+" is "+strconv.Itoa(value+1)+",") {
				t.Fatalf("mutation of %s: got %v, want its mismatched number", declaration[1], problems)
			}
		})
	}
}

func TestJavaProtocolKindValidation(t *testing.T) {
	source := readJavaProtocol(t)
	pkg := driverProtocolConstants(t)
	for _, prefix := range []string{"ITEM", "EVENT"} {
		t.Run("unknown_"+prefix, func(t *testing.T) {
			name := prefix + "_UNKNOWN_KIND"
			problems := javaProtocolKindErrors(source+"\nstatic final int "+name+" = 1;", pkg)
			if len(problems) != 1 || !strings.Contains(problems[0], name+" is no driver constant") {
				t.Fatalf("unknown kind: %v", problems)
			}
		})
	}
	t.Run("unimplemented_driver_kinds", func(t *testing.T) {
		if problems := javaProtocolKindErrors("static final int ITEM_OPEN_URL = 51;\nstatic final int EVENT_PRESS = 1;", pkg); len(problems) != 0 {
			t.Fatalf("Java may omit driver kinds: %v", problems)
		}
	})
	for _, value := range []string{"-1", "1 + 1", "9223372036854775808"} {
		t.Run("invalid_number/"+value, func(t *testing.T) {
			mutated := strings.Replace(source, "ITEM_CREATE = 1;", "ITEM_CREATE = "+value+";", 1)
			problems := javaProtocolKindErrors(mutated, pkg)
			if len(problems) != 1 || !strings.Contains(problems[0], "ITEM_CREATE") {
				t.Fatalf("invalid kind number %s: %v", value, problems)
			}
		})
	}
	t.Run("empty_source", func(t *testing.T) {
		if problems := javaProtocolKindErrors("", pkg); len(problems) != 1 {
			t.Fatalf("empty source: %v", problems)
		}
	})
}
