package core

import "testing"

func TestParseYAML(t *testing.T) {
	t.Run("block_and_flow", func(t *testing.T) {
		doc := `---
# leading comment
radius: 12.5 # inline comment
sans: "Inter, 'Helvetica Neue'"
mono: 'JetBrains Mono' # single quoted
light:
  accent: "#112233"
  inset: '#445566'
  tag: foo#bar # trailing comment
  empty: ""
  nullVal: # comment means null
dark: { text: white, muted: 'rgb(0 0 0 / 0.5)', }
list:
  - 1
  - 2.5
  - [true, false, null]
...
`
		got, err := parseYAML([]byte(doc))
		if err != nil {
			t.Fatalf("parseYAML: %v", err)
		}
		m, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("root = %T, want map[string]any", got)
		}
		if m["radius"] != 12.5 {
			t.Errorf("radius = %#v, want 12.5", m["radius"])
		}
		if m["sans"] != "Inter, 'Helvetica Neue'" {
			t.Errorf("sans = %#v", m["sans"])
		}
		if m["mono"] != "JetBrains Mono" {
			t.Errorf("mono = %#v", m["mono"])
		}
		light := m["light"].(map[string]any)
		if light["accent"] != "#112233" || light["inset"] != "#445566" || light["tag"] != "foo#bar" || light["empty"] != "" || light["nullVal"] != nil {
			t.Errorf("light = %#v", light)
		}
		dark := m["dark"].(map[string]any)
		if dark["text"] != "white" || dark["muted"] != "rgb(0 0 0 / 0.5)" {
			t.Errorf("dark = %#v", dark)
		}
		list := m["list"].([]any)
		if len(list) != 3 || list[0] != 1 || list[1] != 2.5 {
			t.Errorf("list = %#v", list)
		}
		inner := list[2].([]any)
		if len(inner) != 3 || inner[0] != true || inner[1] != false || inner[2] != nil {
			t.Errorf("inner = %#v", inner)
		}
	})

	t.Run("json_escapes", func(t *testing.T) {
		got, err := parseYAML([]byte(`{"a": "hi\n\"there\"\u0021\uD83D\uDE00", "": "empty-key"}`))
		if err != nil {
			t.Fatalf("parseYAML: %v", err)
		}
		m := got.(map[string]any)
		if m["a"] != "hi\n\"there\"!😀" || m[""] != "empty-key" {
			t.Errorf("m = %#v", m)
		}
	})

	t.Run("errors", func(t *testing.T) {
		for _, bad := range []string{
			`{"a": `,
			`{"a": [1, 2}`,
			`"unterminated`,
			`'unterminated`,
			"a: 1\n  b: 2\n",
			"a: \"ok\" trailing",
		} {
			if _, err := parseYAML([]byte(bad)); err == nil {
				t.Errorf("parseYAML(%q): expected error", bad)
			}
		}
	})
}
