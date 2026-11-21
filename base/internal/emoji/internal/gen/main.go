// Command gen writes emoji.bin, the Unicode emoji catalog and multi-language
// CLDR names and keywords for package base/internal/emoji, from a pinned
// release of emojibase-data: go generate ./base/internal/emoji runs it.
//
// The tarball is checked against its SHA-256. Each locale's table is deflated
// independently so only the locales an app searches are inflated at runtime,
// and child locales (en-gb, es-mx) store only entries that differ from their
// parent.
package main

import (
	"archive/tar"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	version    = "18.0.0"
	tarballURL = "https://registry.npmjs.org/emojibase-data/-/emojibase-data-" + version + ".tgz"
	tarballSHA = "ed207c64f31baab5db419686135be3696bbd196a88228a3564383d7918db8f34"
)

type rawEmoji struct {
	Hexcode string     `json:"hexcode"`
	Label   string     `json:"label"`
	Unicode string     `json:"unicode"`
	Group   *int       `json:"group"`
	Order   *int       `json:"order"`
	Tags    []string   `json:"tags"`
	Skins   []rawEmoji `json:"skins"`
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("emoji-gen: ")
	out := flag.String("o", ".", "output directory for emoji.bin")
	cache := flag.String("cache", "", "optional directory holding emojibase-data-<version>.tgz")
	flag.Parse()

	files, err := loadTarball(*cache)
	if err != nil {
		log.Fatal(err)
	}

	var enAll []rawEmoji
	if err := json.Unmarshal(files["en/compact.json"], &enAll); err != nil {
		log.Fatalf("decode en/compact.json: %v", err)
	}
	var valid []rawEmoji
	for _, e := range enAll {
		if e.Group == nil || *e.Group == 2 || e.Order == nil {
			continue
		}
		valid = append(valid, e)
	}
	slices.SortFunc(valid, func(a, b rawEmoji) int {
		return *a.Order - *b.Order
	})

	// Merge English shortcodes (emojibase + github) into English tags so
	// queries like "thumbsup", "+1", "tada", "rofl", "joy" match naturally.
	extraEN := map[string][]string{}
	for _, scFile := range []string{"en/shortcodes/emojibase.json", "en/shortcodes/github.json"} {
		if raw, ok := files[scFile]; ok {
			mergeShortcodes(raw, extraEN)
		}
	}

	// Build base table: emoji\tgroup\tskins (comma-separated).
	var baseBuf bytes.Buffer
	for i, e := range valid {
		if i > 0 {
			baseBuf.WriteByte('\n')
		}
		g := *e.Group
		if g > 2 {
			g-- // collapse Unicode group 2 (component) so categories are 0..8
		}
		baseBuf.WriteString(e.Unicode)
		baseBuf.WriteByte('\t')
		baseBuf.WriteString(strconv.Itoa(g))
		baseBuf.WriteByte('\t')
		for j, s := range e.Skins {
			if j > 0 {
				baseBuf.WriteByte(',')
			}
			baseBuf.WriteString(s.Unicode)
		}
	}
	baseDeflated := deflateBytes(baseBuf.Bytes())

	// Discover locales present in the tarball.
	var locales []string
	for name := range files {
		if loc, ok := strings.CutSuffix(name, "/compact.json"); ok && !strings.Contains(loc, "/") {
			locales = append(locales, loc)
		}
	}
	slices.Sort(locales)

	parents := map[string]string{
		"en-gb": "en",
		"es-mx": "es",
	}

	// Decode every locale aligned to valid's order.
	type locEntry struct {
		label string
		tags  string
	}
	localeEntries := map[string][]locEntry{}
	for _, loc := range locales {
		var arr []rawEmoji
		if err := json.Unmarshal(files[loc+"/compact.json"], &arr); err != nil {
			log.Fatalf("decode %s/compact.json: %v", loc, err)
		}
		byHex := make(map[string]rawEmoji, len(arr))
		for _, e := range arr {
			byHex[e.Hexcode] = e
		}
		entries := make([]locEntry, len(valid))
		for i, e := range valid {
			le := byHex[e.Hexcode]
			seen := map[string]bool{}
			lowerLabel := strings.ToLower(le.Label)
			if lowerLabel != "" {
				seen[lowerLabel] = true
			}
			var tags []string
			addTag := func(t string) {
				t = strings.TrimSpace(t)
				if t == "" {
					return
				}
				lt := strings.ToLower(t)
				if seen[lt] {
					return
				}
				seen[lt] = true
				tags = append(tags, t)
			}
			for _, t := range le.Tags {
				addTag(t)
			}
			if loc == "en" {
				for _, sc := range extraEN[e.Hexcode] {
					addTag(sc)
				}
			}
			entries[i] = locEntry{
				label: cleanField(le.Label),
				tags:  cleanField(strings.Join(tags, "|")),
			}
		}
		localeEntries[loc] = entries
	}

	type localeChunk struct {
		name    string
		parent  string
		payload []byte
	}
	var chunks []localeChunk
	for _, loc := range locales {
		parent := parents[loc]
		entries := localeEntries[loc]
		var buf bytes.Buffer
		if parent != "" {
			pEntries := localeEntries[parent]
			first := true
			for i, e := range entries {
				if e == pEntries[i] {
					continue
				}
				if !first {
					buf.WriteByte('\n')
				}
				first = false
				buf.WriteString(strconv.Itoa(i))
				buf.WriteByte('\t')
				buf.WriteString(e.label)
				buf.WriteByte('\t')
				buf.WriteString(e.tags)
			}
		} else {
			for i, e := range entries {
				if i > 0 {
					buf.WriteByte('\n')
				}
				buf.WriteString(e.label)
				buf.WriteByte('\t')
				buf.WriteString(e.tags)
			}
		}
		chunks = append(chunks, localeChunk{
			name:    loc,
			parent:  parent,
			payload: deflateBytes(buf.Bytes()),
		})
	}

	// Pack emoji.bin:
	// [4]byte magic "NSEM"
	// byte version (1)
	// uint32 baseOff, uint32 baseLen
	// uint16 localeCount
	// for each locale:
	//   byte nameLen, [nameLen]byte name
	//   byte parentLen, [parentLen]byte parent
	//   uint32 off, uint32 len
	// payloads...
	headerSize := 4 + 1 + 4 + 4 + 2
	for _, c := range chunks {
		headerSize += 1 + len(c.name) + 1 + len(c.parent) + 4 + 4
	}
	var bin bytes.Buffer
	bin.WriteString("NSEM")
	bin.WriteByte(1)
	off := uint32(headerSize)
	_ = binary.Write(&bin, binary.LittleEndian, off)
	_ = binary.Write(&bin, binary.LittleEndian, uint32(len(baseDeflated)))
	off += uint32(len(baseDeflated))
	_ = binary.Write(&bin, binary.LittleEndian, uint16(len(chunks)))
	for _, c := range chunks {
		bin.WriteByte(byte(len(c.name)))
		bin.WriteString(c.name)
		bin.WriteByte(byte(len(c.parent)))
		bin.WriteString(c.parent)
		_ = binary.Write(&bin, binary.LittleEndian, off)
		_ = binary.Write(&bin, binary.LittleEndian, uint32(len(c.payload)))
		off += uint32(len(c.payload))
	}
	bin.Write(baseDeflated)
	for _, c := range chunks {
		bin.Write(c.payload)
	}

	dest := filepath.Join(*out, "emoji.bin")
	if err := os.WriteFile(dest, bin.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%d emojis, %d locales, %d bytes)", dest, len(valid), len(chunks), bin.Len())
}

func mergeShortcodes(raw []byte, into map[string][]string) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return
	}
	for hex, val := range m {
		var single string
		if json.Unmarshal(val, &single) == nil && single != "" {
			into[hex] = append(into[hex], single)
			continue
		}
		var multi []string
		if json.Unmarshal(val, &multi) == nil {
			into[hex] = append(into[hex], multi...)
		}
	}
}

func cleanField(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "\r", "")
}

func deflateBytes(raw []byte) []byte {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := w.Write(raw); err != nil {
		log.Fatal(err)
	}
	if err := w.Close(); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}

func loadTarball(cache string) (map[string][]byte, error) {
	var data []byte
	var err error
	if cache != "" {
		data, err = os.ReadFile(filepath.Join(cache, "emojibase-data-"+version+".tgz"))
		if err != nil {
			return nil, err
		}
	} else {
		resp, err := http.Get(tarballURL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("download %s: %s", tarballURL, resp.Status)
		}
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != tarballSHA {
		return nil, fmt.Errorf("sha256 mismatch: got %s, want %s", got, tarballSHA)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name, ok := strings.CutPrefix(hdr.Name, "package/")
		if !ok {
			continue
		}
		if strings.HasSuffix(name, "/compact.json") || name == "en/shortcodes/emojibase.json" || name == "en/shortcodes/github.json" {
			b, err := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			files[name] = b
		}
	}
	return files, nil
}
