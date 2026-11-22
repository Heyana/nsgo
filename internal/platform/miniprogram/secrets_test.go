package miniprogram

import (
	"strings"
	"testing"
)

func TestSecretStorageKey(t *testing.T) {
	k1 := secretStorageKey("com.example.app", "refresh-token")
	k2 := secretStorageKey("com.example.app", "refresh-token")
	if k1 != k2 {
		t.Fatalf("non-deterministic key: %q vs %q", k1, k2)
	}
	if !strings.HasPrefix(k1, secretStoragePrefix) || strings.HasPrefix(k1, "nsgo:") {
		t.Fatalf("unexpected key prefix %q", k1)
	}
	// Length-prefixed hashing prevents ambiguity between service and account boundaries.
	pairs := [][2]string{
		{"a", "bc"},
		{"ab", "c"},
		{"a:b", "c"},
		{"a", "b:c"},
		{"com.example.app", "token"},
		{"com.example.app#plugin", "token"},
	}
	seen := map[string][2]string{}
	for _, p := range pairs {
		k := secretStorageKey(p[0], p[1])
		if prev, dup := seen[k]; dup {
			t.Fatalf("collision between %v and %v: %s", prev, p, k)
		}
		seen[k] = p
	}
}

func TestIsStorageNotFound(t *testing.T) {
	for _, msg := range []string{
		"getStorage:fail data not found",
		"removeStorage:fail data not found",
		"getStorage:fail DATA NOT FOUND",
	} {
		if !isStorageNotFound(msg) {
			t.Errorf("isStorageNotFound(%q) = false, want true", msg)
		}
	}
	for _, msg := range []string{
		"",
		"setStorage:fail exceed max storage size",
		"getStorage:fail decrypt fail",
	} {
		if isStorageNotFound(msg) {
			t.Errorf("isStorageNotFound(%q) = true, want false", msg)
		}
	}
}
