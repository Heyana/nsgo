package ios

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestEditorHint(t *testing.T) {
	for _, c := range []struct {
		name        string
		props       *driver.ViewProps
		multiline   bool
		placeholder string
		text        string
		want        string
	}{
		{
			name:        "single-line leaves placeholder to UITextField",
			props:       &driver.ViewProps{},
			multiline:   false,
			placeholder: "Search messages",
			want:        "",
		},
		{
			name:        "single-line keeps explicit help text",
			props:       &driver.ViewProps{Hint: "Press Return to search"},
			multiline:   false,
			placeholder: "Search messages",
			want:        "Press Return to search",
		},
		{
			name:        "multiline empty without placeholder or hint",
			props:       &driver.ViewProps{},
			multiline:   true,
			placeholder: "",
			want:        "",
		},
		{
			name:        "multiline empty exposes placeholder",
			props:       &driver.ViewProps{},
			multiline:   true,
			placeholder: "Write a note",
			want:        "Write a note",
		},
		{
			name:        "multiline updated placeholder",
			props:       &driver.ViewProps{},
			multiline:   true,
			placeholder: "Add details",
			want:        "Add details",
		},
		{
			name:        "multiline with text hides placeholder",
			props:       &driver.ViewProps{},
			multiline:   true,
			placeholder: "Write a note",
			text:        "Hello",
			want:        "",
		},
		{
			name:        "multiline composes placeholder before explicit hint",
			props:       &driver.ViewProps{Hint: "Supports Markdown"},
			multiline:   true,
			placeholder: "Write a note",
			want:        "Write a note. Supports Markdown",
		},
		{
			name:        "multiline preserves trailing punctuation before hint",
			props:       &driver.ViewProps{Hint: "Supports Markdown"},
			multiline:   true,
			placeholder: "Write a note...",
			want:        "Write a note... Supports Markdown",
		},
		{
			name:        "multiline composes placeholder before description and hint",
			props:       &driver.ViewProps{Description: "Optional", Hint: "Supports Markdown"},
			multiline:   true,
			placeholder: "Write a note!",
			want:        "Write a note! Optional. Supports Markdown",
		},
		{
			name:        "multiline omits duplicate when placeholder matches help text",
			props:       &driver.ViewProps{Hint: "Write a note"},
			multiline:   true,
			placeholder: "Write a note",
			want:        "Write a note",
		},
		{
			name:        "multiline with text keeps explicit hint",
			props:       &driver.ViewProps{Hint: "Supports Markdown"},
			multiline:   true,
			placeholder: "Write a note",
			text:        "Hello",
			want:        "Supports Markdown",
		},
		{
			name:        "multiline cleared placeholder keeps explicit hint",
			props:       &driver.ViewProps{Hint: "Supports Markdown"},
			multiline:   true,
			placeholder: "",
			want:        "Supports Markdown",
		},
		{
			name:        "nil props with multiline placeholder",
			props:       nil,
			multiline:   true,
			placeholder: "Write a note",
			want:        "Write a note",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := editorHint(c.props, c.multiline, c.placeholder, c.text); got != c.want {
				t.Errorf("editorHint(%+v, multiline=%v, placeholder=%q, text=%q) = %q, want %q",
					c.props, c.multiline, c.placeholder, c.text, got, c.want)
			}
		})
	}
}
