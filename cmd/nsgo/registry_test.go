package main

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The embedded registry.yaml satisfies the canonical-form, schema and typosquatting policy (#2461).
func TestBuiltinRegistryPolicy(t *testing.T) {
	if err := checkRegistryPolicy(builtinRegistryYAML); err != nil {
		t.Fatalf("embedded cmd/nsgo/registry.yaml failed policy check: %v", err)
	}
	if idx := builtinRegistry(); idx == nil || idx.Version != registryIndexVersion {
		t.Fatalf("builtinRegistry() = %#v, want version %d", idx, registryIndexVersion)
	}
}

func TestParseRegistryIndex(t *testing.T) {
	validExample := `version: 1
items:
  - name: chat
    module: github.com/nsgo-dev/nsgo
    component: chat
    description: A chat conversation with a composer, reasoning blocks and find.
    license: Apache-2.0
    tags: [ai, chat, messages]
  - name: flags
    module: github.com/nsgo-dev/nsgo-icons-flags
    version: v1.2.3
    description: Country and region flags as NSGo icons.
    license: MIT OR Apache-2.0
    homepage: https://github.com/nsgo-dev/nsgo-icons-flags
    tags: [flags, icons]
`
	idx, err := parseRegistryIndex([]byte(validExample))
	if err != nil {
		t.Fatalf("validExample: %v", err)
	}
	if len(idx.Items) != 2 || idx.Items[0].kind() != "component" || idx.Items[1].kind() != "library" {
		t.Fatalf("unexpected parsed items: %#v", idx.Items)
	}

	validCases := []struct {
		name string
		yaml string
	}{
		{
			name: "empty items",
			yaml: "version: 1\nitems: []\n",
		},
		{
			name: "1-char and 64-char names",
			yaml: "version: 1\nitems:\n" +
				"  - name: a\n    module: example.com/a\n    description: Single char.\n    license: MIT\n" +
				"  - name: " + strings.Repeat("a", 64) + "\n    module: example.com/b\n    description: Sixty-four chars.\n    license: MIT\n",
		},
		{
			name: "200 Unicode runes description",
			yaml: "version: 1\nitems:\n  - name: cjk\n    module: example.com/cjk\n    description: " + strings.Repeat("字", 200) + "\n    license: MIT\n",
		},
		{
			name: "major v2 module and prerelease tag",
			yaml: "version: 1\nitems:\n  - name: v2lib\n    module: example.com/v2lib/v2\n    version: v2.1.0-rc.1\n    description: Major v2 library.\n    license: BSD-3-Clause\n",
		},
		{
			name: "SPDX compound with exception and plus",
			yaml: "version: 1\nitems:\n  - name: spdx1\n    module: example.com/s1\n    description: Compound license.\n    license: (Apache-2.0 WITH LLVM-exception) AND GPL-2.0+\n",
		},
		{
			name: "SPDX LicenseRef and DocumentRef",
			yaml: "version: 1\nitems:\n  - name: spdx2\n    module: example.com/s2\n    description: Custom ref license.\n    license: DocumentRef-spdx-tool-1.2:LicenseRef-MIT-Style-2 OR LicenseRef-custom\n",
		},
		{
			name: "hyphenated and numeric tags",
			yaml: "version: 1\nitems:\n  - name: media\n    module: example.com/media\n    description: Video player.\n    license: LGPL-2.1-or-later\n    tags: [3d, h264, rich-text]\n",
		},
	}
	for _, tc := range validCases {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			if _, err := parseRegistryIndex([]byte(tc.yaml)); err != nil {
				t.Errorf("parseRegistryIndex failed: %v", err)
			}
		})
	}

	invalidCases := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "empty",
			data: []byte("   \n"),
			want: "empty",
		},
		{
			name: "exceeds 4 MiB",
			data: append([]byte("version: 1\nitems: []\n# "), bytes.Repeat([]byte("a"), maxRegistryIndexBytes)...),
			want: "exceeds",
		},
		{
			name: "invalid UTF-8",
			data: []byte("version: 1\nitems: []\n# \xff\xfe\n"),
			want: "UTF-8",
		},
		{
			name: "multiple documents",
			data: []byte("version: 1\nitems: []\n---\nversion: 1\nitems: []\n"),
			want: "multiple YAML documents",
		},
		{
			name: "root sequence not mapping",
			data: []byte("- a\n- b\n"),
			want: "mapping",
		},
		{
			name: "unknown top-level field",
			data: []byte("version: 1\nitems: []\nextra: true\n"),
			want: "not found",
		},
		{
			name: "unknown item field",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: MIT\n    unknown: 1\n"),
			want: "not found",
		},
		{
			name: "duplicate mapping key",
			data: []byte("version: 1\nversion: 1\nitems: []\n"),
			want: "duplicate YAML key",
		},
		{
			name: "YAML anchor",
			data: []byte("version: 1\nitems:\n  - &base\n    name: a\n    module: example.com/a\n    description: Desc.\n    license: MIT\n"),
			want: "anchors are not allowed",
		},
		{
			name: "YAML alias",
			data: []byte("version: 1\nitems:\n  - name: &n a\n    module: example.com/a\n    component: *n\n    description: Desc.\n    license: MIT\n"),
			want: "anchors are not allowed",
		},
		{
			name: "YAML merge key",
			data: []byte("version: 1\nitems:\n  - <<: {name: a}\n    module: example.com/a\n    description: Desc.\n    license: MIT\n"),
			want: "merge keys are not allowed",
		},
		{
			name: "unsupported version 0",
			data: []byte("items: []\n"),
			want: "unsupported registry index version 0",
		},
		{
			name: "unsupported version 2",
			data: []byte("version: 2\nitems: []\n"),
			want: "unsupported registry index version 2",
		},
		{
			name: "control character ESC in description",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: \"bad\\e[31m\"\n    license: MIT\n"),
			want: "control or format character",
		},
		{
			name: "control character newline in description",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: \"line1\\nline2\"\n    license: MIT\n"),
			want: "control or format character",
		},
		{
			name: "control character tab in description",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: \"a\\tb\"\n    license: MIT\n"),
			want: "control or format character",
		},
		{
			name: "bidi override U+202E in description",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: \"safe \u202e evil\"\n    license: MIT\n"),
			want: "U+202E",
		},
		{
			name: "bidi isolate U+2066 in description",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: \"safe \u2066 evil\"\n    license: MIT\n"),
			want: "U+2066",
		},
		{
			name: "zero-width space U+200B in description",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: \"safe\u200bevil\"\n    license: MIT\n"),
			want: "U+200B",
		},
		{
			name: "bidi override in comment",
			data: []byte("version: 1\n# \u202e comment\nitems: []\n"),
			want: "U+202E",
		},
		{
			name: "Unicode line separator U+2028",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: \"a\u2028b\"\n    license: MIT\n"),
			want: "U+2028",
		},
		{
			name: "empty name",
			data: []byte("version: 1\nitems:\n  - name: \"\"\n    module: example.com/a\n    description: Desc.\n    license: MIT\n"),
			want: "invalid name",
		},
		{
			name: "name too long",
			data: []byte("version: 1\nitems:\n  - name: " + strings.Repeat("a", 65) + "\n    module: example.com/a\n    description: Desc.\n    license: MIT\n"),
			want: "invalid name",
		},
		{
			name: "name uppercase",
			data: []byte("version: 1\nitems:\n  - name: Media\n    module: example.com/a\n    description: Desc.\n    license: MIT\n"),
			want: "invalid name",
		},
		{
			name: "name with dot",
			data: []byte("version: 1\nitems:\n  - name: a.b\n    module: example.com/a\n    description: Desc.\n    license: MIT\n"),
			want: "invalid name",
		},
		{
			name: "name leading hyphen",
			data: []byte("version: 1\nitems:\n  - name: -media\n    module: example.com/a\n    description: Desc.\n    license: MIT\n"),
			want: "invalid name",
		},
		{
			name: "name trailing hyphen",
			data: []byte("version: 1\nitems:\n  - name: media-\n    module: example.com/a\n    description: Desc.\n    license: MIT\n"),
			want: "invalid name",
		},
		{
			name: "name with underscore",
			data: []byte("version: 1\nitems:\n  - name: my_lib\n    module: example.com/a\n    description: Desc.\n    license: MIT\n"),
			want: "invalid name",
		},
		{
			name: "duplicate name",
			data: []byte("version: 1\nitems:\n  - name: media\n    module: example.com/a\n    description: First.\n    license: MIT\n  - name: media\n    module: example.com/b\n    description: Second.\n    license: MIT\n"),
			want: "duplicate item name",
		},
		{
			name: "invalid module without dot",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: nodot/pkg\n    description: Desc.\n    license: MIT\n"),
			want: "invalid module",
		},
		{
			name: "invalid module empty",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: \"\"\n    description: Desc.\n    license: MIT\n"),
			want: "invalid module",
		},
		{
			name: "invalid component name",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    component: Bad_Comp\n    description: Desc.\n    license: MIT\n"),
			want: "invalid component",
		},
		{
			name: "version pseudo-version",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    version: v0.0.0-20260101000000-abcdefabcdef\n    description: Desc.\n    license: MIT\n"),
			want: "pseudo-version",
		},
		{
			name: "version query latest",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    version: latest\n    description: Desc.\n    license: MIT\n"),
			want: "invalid version",
		},
		{
			name: "version query upgrade",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    version: upgrade\n    description: Desc.\n    license: MIT\n"),
			want: "invalid version",
		},
		{
			name: "version prefix query v1.2",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    version: v1.2\n    description: Desc.\n    license: MIT\n"),
			want: "invalid version",
		},
		{
			name: "version major mismatch",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    version: v2.0.0\n    description: Desc.\n    license: MIT\n"),
			want: "invalid version",
		},
		{
			name: "description empty",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: \"\"\n    license: MIT\n"),
			want: "description is required",
		},
		{
			name: "description whitespace only",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: \"   \"\n    license: MIT\n"),
			want: "description is required",
		},
		{
			name: "description leading space",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: \" leading\"\n    license: MIT\n"),
			want: "leading or trailing whitespace",
		},
		{
			name: "description > 200 runes",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: " + strings.Repeat("x", 201) + "\n    license: MIT\n"),
			want: "max 200",
		},
		{
			name: "license empty",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: \"\"\n"),
			want: "invalid license",
		},
		{
			name: "license slash",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: MIT/Apache-2.0\n"),
			want: "invalid license",
		},
		{
			name: "license lowercase operator",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: MIT or Apache-2.0\n"),
			want: "invalid license",
		},
		{
			name: "license space without operator",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: Apache 2.0\n"),
			want: "invalid license",
		},
		{
			name: "license trailing AND",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: MIT AND\n"),
			want: "invalid license",
		},
		{
			name: "license unbalanced paren",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: (MIT OR Apache-2.0\n"),
			want: "invalid license",
		},
		{
			name: "license empty parens",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: ()\n"),
			want: "invalid license",
		},
		{
			name: "license WITH on compound expr",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: (MIT OR Apache-2.0) WITH LLVM-exception\n"),
			want: "invalid license",
		},
		{
			name: "license LicenseRef with plus",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: LicenseRef-custom+\n"),
			want: "invalid license",
		},
		{
			name: "license NONE",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: NONE\n"),
			want: "invalid license",
		},
		{
			name: "license NOASSERTION",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: NOASSERTION\n"),
			want: "invalid license",
		},
		{
			name: "homepage http",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: MIT\n    homepage: http://example.com\n"),
			want: "https",
		},
		{
			name: "homepage no host",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: MIT\n    homepage: https://\n"),
			want: "host",
		},
		{
			name: "homepage userinfo",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: MIT\n    homepage: https://user@example.com\n"),
			want: "user info",
		},
		{
			name: "tag uppercase",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: MIT\n    tags: [Icons]\n"),
			want: "invalid tag",
		},
		{
			name: "tag with space",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: MIT\n    tags: [\"rich text\"]\n"),
			want: "invalid tag",
		},
		{
			name: "tag duplicate",
			data: []byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: Desc.\n    license: MIT\n    tags: [icons, icons]\n"),
			want: "duplicate tag",
		},
	}
	for _, tc := range invalidCases {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			_, err := parseRegistryIndex(tc.data)
			if err == nil {
				t.Fatalf("parseRegistryIndex succeeded, want error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestRegistryPolicy(t *testing.T) {
	canonicalExample := `version: 1
items:
  - name: chat
    module: github.com/nsgo-dev/nsgo
    component: chat
    description: A chat conversation with a composer, reasoning blocks and find.
    license: Apache-2.0
    tags: [ai, chat, messages]
  - name: flags
    module: github.com/nsgo-dev/nsgo-icons-flags
    description: Country and region flags as NSGo icons.
    license: MIT
    tags: [flags, icons]
`
	if err := checkRegistryPolicy([]byte(canonicalExample)); err != nil {
		t.Fatalf("canonicalExample failed policy: %v", err)
	}

	// Same module owner may have names within one edit of each other.
	sameOwnerClose := `version: 1
items:
  - name: flag
    module: github.com/nsgo-dev/nsgo-flag
    description: Single flag.
    license: MIT
  - name: flags
    module: github.com/nsgo-dev/nsgo-flags
    description: Many flags.
    license: MIT
`
	if err := checkRegistryPolicy([]byte(sameOwnerClose)); err != nil {
		t.Fatalf("same owner within 1 edit failed policy: %v", err)
	}

	// Different module owners at edit distance 2 are allowed.
	diffOwnerDist2 := `version: 1
items:
  - name: chart
    module: github.com/acme/chart
    description: Charts.
    license: MIT
  - name: chats
    module: github.com/other/chats
    description: Chats.
    license: MIT
`
	if err := checkRegistryPolicy([]byte(diffOwnerDist2)); err != nil {
		t.Fatalf("different owners at edit distance 2 failed policy: %v", err)
	}

	badPolicy := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "unsorted items",
			yaml: "version: 1\nitems:\n  - name: flags\n    module: github.com/nsgo-dev/nsgo-icons-flags\n    description: Flags.\n    license: MIT\n  - name: chat\n    module: github.com/nsgo-dev/nsgo\n    description: Chat.\n    license: Apache-2.0\n",
			want: "sorted by name",
		},
		{
			name: "fields out of schema order",
			yaml: "version: 1\nitems:\n  - module: github.com/nsgo-dev/nsgo-icons-flags\n    name: flags\n    description: Flags.\n    license: MIT\n",
			want: "canonical form",
		},
		{
			name: "four-space indent",
			yaml: "version: 1\nitems:\n    - name: flags\n      module: github.com/nsgo-dev/nsgo-icons-flags\n      description: Flags.\n      license: MIT\n",
			want: "canonical form",
		},
		{
			name: "block tags instead of flow",
			yaml: "version: 1\nitems:\n  - name: flags\n    module: github.com/nsgo-dev/nsgo-icons-flags\n    description: Flags.\n    license: MIT\n    tags:\n      - flags\n",
			want: "canonical form",
		},
		{
			name: "edit distance 1 insertion across owners",
			yaml: "version: 1\nitems:\n  - name: flag\n    module: github.com/acme/flag\n    description: Flag.\n    license: MIT\n  - name: flags\n    module: github.com/other/flags\n    description: Flags.\n    license: MIT\n",
			want: "within one edit",
		},
		{
			name: "edit distance 1 substitution across owners",
			yaml: "version: 1\nitems:\n  - name: char\n    module: github.com/acme/char\n    description: Char.\n    license: MIT\n  - name: chat\n    module: github.com/other/chat\n    description: Chat.\n    license: MIT\n",
			want: "within one edit",
		},
	}
	for _, tc := range badPolicy {
		t.Run(tc.name, func(t *testing.T) {
			err := checkRegistryPolicy([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("checkRegistryPolicy = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestLevenshteinDistance(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "abc", 3},
		{"media", "media", 0},
		{"flag", "flags", 1},
		{"flags", "flag", 1},
		{"chat", "char", 1},
		{"chart", "chats", 2},
		{"ab", "ba", 2},
	}
	for _, tc := range cases {
		if got := levenshteinDistance(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func FuzzParseIndex(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("version: 1\nitems: []\n"))
	f.Add([]byte("version: 1\nitems:\n  - name: flags\n    module: github.com/nsgo-dev/nsgo-icons-flags\n    description: Country flags.\n    license: MIT\n    tags: [flags, icons]\n"))
	f.Add([]byte("version: 1\nitems:\n  - &a\n    name: x\n    module: example.com/x\n    description: Desc.\n    license: MIT\n  - *a\n"))
	f.Add([]byte("version: 1\nitems:\n  - <<: {name: a}\n"))
	f.Add([]byte("version: 1\nitems: []\n---\nversion: 1\nitems: []\n"))
	f.Add([]byte("version: 1\nitems:\n  - name: a\n    module: example.com/a\n    description: \"bad\u202e\"\n    license: MIT\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		idx, err := parseRegistryIndex(data)
		if err != nil {
			return
		}
		encoded, err := encodeRegistryIndex(idx)
		if err != nil {
			t.Fatalf("encodeRegistryIndex failed: %v", err)
		}
		reparsed, err := parseRegistryIndex(encoded)
		if err != nil {
			t.Fatalf("reparsing canonical YAML failed: %v\nencoded:\n%s", err, encoded)
		}
		if !reflect.DeepEqual(idx.sortedCopy(), reparsed) {
			t.Fatalf("round-trip mismatch:\ngot:  %#v\nwant: %#v\nencoded:\n%s", reparsed, idx.sortedCopy(), encoded)
		}
	})
}

func testRegistryIndex(t *testing.T) *registryIndex {
	t.Helper()
	idx, err := parseRegistryIndex([]byte(`version: 1
items:
  - name: chart
    module: github.com/acme/chart
    description: Interactive charts for NSGo apps.
    license: MIT
    tags: [charts, plot]
  - name: chat
    module: github.com/nsgo-dev/nsgo
    component: chat
    description: A chat conversation with a composer, reasoning blocks and find.
    license: Apache-2.0
    tags: [ai, chat, messages]
  - name: edge
    module: github.com/acme/edge
    description: Unreleased library at a commit with no tag.
    license: MIT
    tags: [experimental]
  - name: oldpinned
    module: example.com/old
    version: v1.0.0
    description: Library with a pinned version in the index.
    license: MIT
    tags: [legacy]
`))
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

// With a test index, nsgo add <name> makes the exact same plan as nsgo add <module>, and uses an entry's pinned version when present (#2461).
func TestAddByNameMatchesModulePlan(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	idx := testRegistryIndex(t)
	dir := nsgoApp(t)

	byModule, err := planAdd(t, dir, "github.com/acme/chart")
	if err != nil {
		t.Fatalf("plan by module: %v\n%s", err, byModule)
	}
	var byName bytes.Buffer
	t.Chdir(dir)
	captureStderr(t, func() {
		err = addModulesWithIndex(t.Context(), addOptions{dryRun: true}, []string{"chart"}, idx, &byName, nil)
	})
	if err != nil {
		t.Fatalf("plan by name: %v\n%s", err, byName.String())
	}
	if byName.String() != byModule {
		t.Errorf("plan by name:\n%s\nwant same as by module:\n%s", byName.String(), byModule)
	}

	// Entry with pinned version uses that version; explicit @version overrides it.
	dir2 := t.TempDir()
	writeFiles(t, dir2, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n"})
	t.Chdir(dir2)
	var pinnedOut bytes.Buffer
	captureStderr(t, func() {
		err = addModulesWithIndex(t.Context(), addOptions{dryRun: true}, []string{"oldpinned"}, idx, &pinnedOut, nil)
	})
	if err != nil {
		t.Fatalf("plan pinned: %v\n%s", err, pinnedOut.String())
	}
	if !strings.Contains(pinnedOut.String(), "+ require example.com/old v1.0.0") {
		t.Errorf("pinned entry plan did not select v1.0.0:\n%s", pinnedOut.String())
	}

	var overrideOut bytes.Buffer
	captureStderr(t, func() {
		err = addModulesWithIndex(t.Context(), addOptions{dryRun: true}, []string{"oldpinned@v1.1.0"}, idx, &overrideOut, nil)
	})
	if err != nil {
		t.Fatalf("plan pinned override: %v\n%s", err, overrideOut.String())
	}
	if !strings.Contains(overrideOut.String(), "+ require example.com/old v1.1.0") {
		t.Errorf("override plan did not select v1.1.0:\n%s", overrideOut.String())
	}
}

// A bare name that resolves to an untagged commit is refused, -y or not, naming the path-with-version alternative (#2461).
func TestAddByNameRefusesUntaggedCommit(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	idx := testRegistryIndex(t)
	dir := t.TempDir()
	const gomod = "module example.com/tool\n\ngo 1.27\n"
	writeFiles(t, dir, map[string]string{"go.mod": gomod})
	t.Chdir(dir)

	for _, o := range []addOptions{{dryRun: true}, {yes: true}} {
		var out bytes.Buffer
		var err error
		captureStderr(t, func() {
			err = addModulesWithIndex(t.Context(), o, []string{"edge"}, idx, &out, nil)
		})
		const want = "edge has no release; add its module by path with a version"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v: err = %v, want substring %q", o, err, want)
		}
		if got, _ := moduleFiles(t, dir); string(got) != gomod {
			t.Fatalf("%+v: go.mod changed:\n%s", o, got)
		}
	}
}

// Unknown names fail suggesting -list, component items resolve as component copy requests, and duplicates are refused (#2461, #2467).
func TestAddByNameRefusesUnknownAndComponent(t *testing.T) {
	idx := testRegistryIndex(t)

	_, err := parseAddRequestsWithIndex([]string{"missing"}, idx)
	if err == nil || !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "-list") {
		t.Errorf("unknown name: err = %v, want name and -list suggestion", err)
	}

	got, err := parseAddRequestsWithIndex([]string{"chat"}, idx)
	if err != nil {
		t.Fatalf("component item: err = %v", err)
	}
	want := addRequest{path: "github.com/nsgo-dev/nsgo", component: "chat", fromName: "chat", registryLicense: "Apache-2.0"}
	if len(got) != 1 || got[0] != want {
		t.Errorf("component item = %+v, want [%+v]", got, want)
	}

	// Duplicate by bare name and module path together is caught.
	_, err = parseAddRequestsWithIndex([]string{"chart", "github.com/acme/chart"}, idx)
	if err == nil || !strings.Contains(err.Error(), "named twice") {
		t.Errorf("duplicate name and path: err = %v, want named twice", err)
	}
}

// -list prints names, kinds, modules and descriptions matching every word case-insensitively across name, description or tags, and works outside a module (#2461).
func TestAddList(t *testing.T) {
	idx := testRegistryIndex(t)

	// Works outside any Go module.
	outside := t.TempDir()
	t.Chdir(outside)

	var all bytes.Buffer
	if err := addModulesWithIndex(t.Context(), addOptions{list: true}, nil, idx, &all, nil); err != nil {
		t.Fatalf("-list outside module: %v", err)
	}
	const wantAll = `chart      library    github.com/acme/chart     Interactive charts for NSGo apps.
chat       component  github.com/nsgo-dev/nsgo  A chat conversation with a composer, reasoning blocks and find.
edge       library    github.com/acme/edge      Unreleased library at a commit with no tag.
oldpinned  library    example.com/old           Library with a pinned version in the index.
`
	if all.String() != wantAll {
		t.Errorf("-list output:\n%s\nwant:\n%s", all.String(), wantAll)
	}

	// Multi-word case-insensitive match across name, description, and tags.
	var filtered bytes.Buffer
	if err := addModulesWithIndex(t.Context(), addOptions{list: true}, []string{"AI", "COMPOSER"}, idx, &filtered, nil); err != nil {
		t.Fatalf("-list filtered: %v", err)
	}
	const wantFiltered = "chat  component  github.com/nsgo-dev/nsgo  A chat conversation with a composer, reasoning blocks and find.\n"
	if filtered.String() != wantFiltered {
		t.Errorf("-list AI COMPOSER:\n%s\nwant:\n%s", filtered.String(), wantFiltered)
	}

	// Word missing from all fields matches nothing.
	var empty bytes.Buffer
	if err := addModulesWithIndex(t.Context(), addOptions{list: true}, []string{"chat", "plot"}, idx, &empty, nil); err != nil {
		t.Fatalf("-list no match: %v", err)
	}
	if empty.Len() != 0 {
		t.Errorf("-list no match = %q, want empty", empty.String())
	}

	// -list refuses -dry-run and -y.
	for _, o := range []addOptions{{list: true, dryRun: true}, {list: true, yes: true}} {
		if err := addModulesWithIndex(t.Context(), o, nil, idx, &empty, nil); err == nil {
			t.Errorf("%+v succeeded, want refusal", o)
		}
	}

	// End-to-end CLI invocation of nsgo add -list outside a module exits 0.
	proxy := libraryProxy(t)
	path, env := goEnv(t, "file://"+filepath.ToSlash(proxy))
	if out, code := runNsgo(t, outside, path, env, "add", "-list"); code != 0 {
		t.Errorf("nsgo add -list outside module: exit %d\n%s", code, out)
	}
}
