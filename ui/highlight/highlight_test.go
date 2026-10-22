package highlight

import "testing"

func TestTokens(t *testing.T) {
	code := "// hi\nfunc main() { x := \"s\" + 42 }"
	got := map[string]kind{}
	for _, tk := range tokens("go", code) {
		got[code[tk.start:tk.end]] = tk.kind
	}
	for text, want := range map[string]kind{"// hi": comment, "func": keyword, "main": title, `"s"`: str, "42": number} {
		if got[text] != want {
			t.Errorf("%q is %d, want %d (all: %v)", text, got[text], want, got)
		}
	}
	if tokens("no-such-language", code) != nil || tokens("", code) != nil {
		t.Error("an unknown language was colored")
	}
}
