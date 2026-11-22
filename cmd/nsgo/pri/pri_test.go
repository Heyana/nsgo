package pri

import (
	"bytes"
	"encoding/binary"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func fixtureResources() []Resource {
	return []Resource{
		{Name: "Resources/AppDisplayName", Candidates: []Candidate{{Value: "Fixture App", Language: "en-US"}, {Value: "测试应用 🧪", Language: "zh-CN"}}},
		{Name: "Files/Assets/Logo.png", Path: true, Candidates: []Candidate{{Value: `Assets\Logo.scale-100.png`, Scale: 100}, {Value: `Assets\Logo.scale-200.png`, Scale: 200}, {Value: `Assets\Logo.scale-400.png`, Scale: 400}}},
	}
}

func TestWriteDeterministicCompleteIndex(t *testing.T) {
	input := fixtureResources()
	before := fixtureResources()
	first, err := Write("NSGoFixture", "en-US", input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("writer changed caller's resources")
	}
	slices.Reverse(input)
	second, err := Write("NSGoFixture", "en-US", input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("resource order changed index bytes")
	}
	b := first
	if string(b[:8]) != "mrm_pri2" || binary.LittleEndian.Uint32(b[12:]) != uint32(len(b)) || binary.LittleEndian.Uint32(b[len(b)-16:]) != 0xdefffade || !bytes.Equal(b[:8], b[len(b)-8:]) {
		t.Fatal("invalid file framing")
	}
	toc := int(binary.LittleEndian.Uint32(b[16:]))
	data := int(binary.LittleEndian.Uint32(b[20:]))
	count := int(binary.LittleEndian.Uint16(b[24:]))
	if count != 4 {
		t.Fatalf("sections=%d, want self-contained decisions/descriptor/schema/map", count)
	}
	seen := map[string]bool{}
	for i := 0; i < count; i++ {
		entry := b[toc+32*i : toc+32*(i+1)]
		offset := int(binary.LittleEndian.Uint32(entry[24:]))
		size := int(binary.LittleEndian.Uint32(entry[28:]))
		section := b[data+offset : data+offset+size]
		if !bytes.Equal(entry[:16], section[:16]) || size%8 != 0 || binary.LittleEndian.Uint32(section[24:]) != uint32(size) || binary.LittleEndian.Uint32(section[size-8:]) != 0xdef5fade || binary.LittleEndian.Uint32(section[size-4:]) != uint32(size) {
			t.Fatal("section header/TOC/trailer disagree")
		}
		seen[strings.TrimRight(string(section[:16]), "\x00 ")] = true
	}
	for _, name := range []string{"[mrm_decn_info]", "[mrm_pridescex]", "[mrm_hschema]", "[mrm_res_map2_]"} {
		if !seen[name] {
			t.Errorf("missing %s", name)
		}
	}
	for _, s := range []string{"Fixture App", "测试应用 🧪", `Assets\Logo.scale-100.png`, `Assets\Logo.scale-200.png`, `Assets\Logo.scale-400.png`} {
		if !bytes.Contains(b, wide(s)) {
			t.Errorf("missing complete UTF-16 value %q", s)
		}
	}
	// Optional owned native-fixture export; normal tests write no artifact.
	if path := os.Getenv("NSGO_PRI_FIXTURE_OUT"); path != "" {
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSchemaChecksumMatchesMicrosoftMRTControl(t *testing.T) {
	// Windows SDK 10.0.26100 makepri control: simple ID NSGoFixture, English/Chinese strings, two scaled paths and the indexed config file.
	checksum := schemaChecksum("ms-appx://NSGoFixture/", "NSGoFixture", []string{"", "Files", "Files/Assets", "Resources"}, []string{"Files/Assets/Logo.png", "Resources/AppDisplayName", "Files/priconfig.xml"})
	if checksum != 0xc9b8b270 {
		t.Fatalf("schema checksum=%08x, native MRT control=c9b8b270", checksum)
	}
}

func TestWriteRejectsAmbiguousOrUnrepresentableValues(t *testing.T) {
	for i, rs := range [][]Resource{
		nil,
		{{Name: "Resources/A", Candidates: []Candidate{{Value: "x"}}}, {Name: "resources/a", Candidates: []Candidate{{Value: "y"}}}},
		{{Name: "Resources/A", Candidates: []Candidate{{Value: "x"}}}, {Name: "Resources/A/B", Candidates: []Candidate{{Value: "y"}}}},
		{{Name: "../A", Candidates: []Candidate{{Value: "x"}}}},
		{{Name: "A", Candidates: []Candidate{{Value: "x", Language: "en-US", Scale: 100}}}},
		{{Name: "A", Candidates: []Candidate{{Value: "x", Language: "en-US"}, {Value: "y", Language: "EN-us"}}}},
		{{Name: "A", Candidates: []Candidate{{Value: "x", Scale: 10}}}},
		{{Name: "A", Candidates: []Candidate{{Value: "x\x00y"}}}},
		{{Name: "A", Candidates: []Candidate{{Value: strings.Repeat("界", 40000)}}}},
		{{Name: "A", Path: true, Candidates: []Candidate{{Value: `..\outside.png`}}}},
	} {
		if _, err := Write("NSGoFixture", "en-US", rs); err == nil {
			t.Errorf("invalid resource case %d accepted", i)
		}
	}
}
