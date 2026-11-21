package linux

import "testing"

func TestEditorPlaceholderState(t *testing.T) {
	if placeholderLabelRole != "presentation" {
		t.Fatalf("placeholderLabelRole = %q, want presentation", placeholderLabelRole)
	}
	if editorPlaceholderProperty != "placeholder" {
		t.Fatalf("editorPlaceholderProperty = %q, want placeholder", editorPlaceholderProperty)
	}

	var s editorPlaceholderState
	steps := []struct {
		name        string
		placeholder string
		resetFirst  bool
		want        editorPlaceholderAction
	}{
		{"initial empty is noop", "", false, editorPlaceholderNoop},
		{"initial non-empty sets property", "Write a note", false, editorPlaceholderSet},
		{"same value is noop", "Write a note", false, editorPlaceholderNoop},
		{"updated value sets property", "Add details", false, editorPlaceholderSet},
		{"cleared value resets property", "", false, editorPlaceholderReset},
		{"repeated clear is noop", "", false, editorPlaceholderNoop},
		{"setting after clear sets property", "New hint", false, editorPlaceholderSet},
		{"after editor reset same placeholder sets again", "New hint", true, editorPlaceholderSet},
		{"after editor reset empty is noop", "", true, editorPlaceholderNoop},
	}
	for _, step := range steps {
		if step.resetFirst {
			s.reset()
		}
		if got := s.update(step.placeholder); got != step.want {
			t.Errorf("%s: update(%q) = %v, want %v", step.name, step.placeholder, got, step.want)
		}
	}
}
