package windows

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestToastActivationPreservesCompleteIDs(t *testing.T) {
	for _, id := range []string{"chat:42", " chat:42 \t\n", "\u3000", "a;b&n=2;id=c", "日本語 🦊 ", strings.Repeat("long-id:", 40)} {
		for _, launch := range []string{toastLaunch(id), "id=" + id} {
			got, press := parseToastActivationArgs(launch)
			if got != id || press != launch {
				t.Errorf("parse %q = (%q, %q), want (%q, %q)", launch, got, press, id, launch)
			}
		}
	}
}

func TestToastIdentityBoundsTagsWithoutCollidingWithShortIDs(t *testing.T) {
	ids := []string{"chat:42", strings.Repeat("x", 17), strings.Repeat("x", 64), strings.Repeat("x", 1000), strings.Repeat("🦊", 9)}
	seen := map[[2]string]string{}
	for _, id := range ids {
		tag, group := toastIdentity(id)
		if n := len(utf16.Encode([]rune(tag))); n == 0 || n > 16 {
			t.Fatalf("%q has a %d-unit tag", id, n)
		}
		if old, ok := seen[[2]string{tag, group}]; ok {
			t.Fatalf("%q and %q share an identity", id, old)
		}
		seen[[2]string{tag, group}] = id
		if tag2, group2 := toastIdentity(id); tag2 != tag || group2 != group {
			t.Fatal("identity changed across calls")
		}
		if !toastMatches(id, tag, group) || !toastMatches(id, id, toastGroup) {
			t.Fatal("close does not match current and legacy identities")
		}
		if group == toastLongGroup {
			shortTag, shortGroup := toastIdentity(tag)
			if shortTag != tag || shortGroup == group || toastMatches(tag, tag, group) {
				t.Fatal("a short app ID collided with a generated tag")
			}
		}
		if toastMatches(id, tag, "another-framework") {
			t.Fatal("close matched another framework's group")
		}
	}
	if !toastMatches("", "any", toastGroup) || !toastMatches("", "any", toastLongGroup) || toastMatches("", "any", "other") {
		t.Fatal("close-all must match both owned groups only")
	}
}
