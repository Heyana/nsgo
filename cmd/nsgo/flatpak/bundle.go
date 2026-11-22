// Package flatpak writes self-contained Flatpak bundles in Go. The format is an OSTree static delta from an empty base with inline parts and Flatpak metadata; neither GLib nor the Linux packaging tools run during a build.
package flatpak

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode/utf8"
)

// Entry is a regular file, directory or symbolic link relative to the commit root. Mode holds Unix permission bits; ownership is always root.
type Entry struct {
	Name string
	Data []byte
	Mode uint32
	Link string
	Dir  bool
}

// Bundle identifies an application and its runtime. Metadata is the Flatpak keyfile stored both in the commit and in the bundle's metadata header.
type Bundle struct {
	ID, Arch, Branch string
	Metadata         string
	Timestamp        uint64
}

type object struct {
	kind byte
	sum  [32]byte
	data []byte
	mode uint32
}
type directory struct {
	files map[string]Entry
	dirs  map[string]*directory
	mode  uint32
}

func newDirectory() *directory {
	return &directory{files: map[string]Entry{}, dirs: map[string]*directory{}, mode: 0o755}
}

// Write makes a complete offline bundle. Payload entries must include files/ and export/ as applicable; metadata is added by this function. It rejects ambiguous entry paths, absolute links and lexical link escapes, and leaves inputs untouched.
func Write(b Bundle, entries []Entry) ([]byte, error) {
	for _, s := range []string{b.ID, b.Arch, b.Branch} {
		if s == "" || strings.ContainsAny(s, "/\\\x00\r\n") || !utf8.ValidString(s) {
			return nil, fmt.Errorf("flatpak: invalid application reference")
		}
	}
	if b.Metadata == "" || strings.ContainsRune(b.Metadata, 0) || !utf8.ValidString(b.Metadata) {
		return nil, fmt.Errorf("flatpak: invalid metadata")
	}
	root := newDirectory()
	for _, e := range entries {
		if e.Name == "metadata" {
			return nil, fmt.Errorf("flatpak: reserved metadata entry")
		}
	}
	seen := map[string]bool{}
	entries = append(slices.Clone(entries), Entry{Name: "metadata", Data: []byte(b.Metadata), Mode: 0o644})
	for _, e := range entries {
		if seen[e.Name] {
			return nil, fmt.Errorf("flatpak: duplicate entry %q", e.Name)
		}
		if !validPath(e.Name) || e.Mode & ^uint32(0o777) != 0 || e.Dir && e.Link != "" {
			return nil, fmt.Errorf("flatpak: invalid entry %q", e.Name)
		}
		if e.Link != "" && (strings.HasPrefix(e.Link, "/") || strings.ContainsRune(e.Link, 0) || !utf8.ValidString(e.Link) || !validPath(path.Clean(path.Join(path.Dir(e.Name), e.Link)))) {
			return nil, fmt.Errorf("flatpak: link %q escapes the commit", e.Name)
		}
		seen[e.Name] = true
		d := root
		parts := strings.Split(e.Name, "/")
		for _, p := range parts[:len(parts)-1] {
			if _, ok := d.files[p]; ok {
				return nil, fmt.Errorf("flatpak: entry below a file")
			}
			if d.dirs[p] == nil {
				d.dirs[p] = newDirectory()
			}
			d = d.dirs[p]
		}
		name := parts[len(parts)-1]
		if e.Dir {
			if _, ok := d.files[name]; ok {
				return nil, fmt.Errorf("flatpak: file/directory collision")
			}
			if d.dirs[name] == nil {
				d.dirs[name] = newDirectory()
			}
			d.dirs[name].mode = e.Mode
		} else {
			if d.dirs[name] != nil {
				return nil, fmt.Errorf("flatpak: file/directory collision")
			}
			d.files[name] = e
		}
	}
	var objects []object
	tree, meta := root.objects(&objects)
	ref := "app/" + b.ID + "/" + b.Arch + "/" + b.Branch
	commitMetadata := dictionary(
		dictionaryEntry("ostree.ref-binding", array(stringValue(""), stringValue(ref))),
		dictionaryEntry("xa.ref", stringValue(ref)),
		dictionaryEntry("xa.metadata", stringValue(b.Metadata)),
	)
	commit := tuple(commitMetadata, bytesValue(nil), array(tuple(stringValue(""), bytesValue(nil))), stringValue(b.ID), stringValue(""), uint64Value(swap64(b.Timestamp)), bytesValue(tree[:]), bytesValue(meta[:]))
	commitSum := sha256.Sum256(commit.data)
	part, header := deltaPart(objects)
	encoded := strings.ReplaceAll(base64.RawStdEncoding.EncodeToString(commitSum[:]), "/", "_")
	partName := "deltas/" + encoded[:2] + "/" + encoded[2:] + "/0"
	metadata := dictionary(
		dictionaryEntry("flatpak", uint32Value(0xe5890001)),
		dictionaryEntry("ref", stringValue(ref)),
		dictionaryEntry("metadata", stringValue(b.Metadata)),
		dictionaryEntry("collection-id", stringValue("")),
		dictionaryEntry("ostree.endianness", byteValue('l')),
		dictionaryEntry(partName, part),
	)
	superblock := tuple(metadata, uint64Value(swap64(b.Timestamp)), bytesValue(nil), bytesValue(commitSum[:]), commit, bytesValue(nil), array(header, header), array(tuple(byteValue(0), bytesValue(nil), uint64Value(0), uint64Value(0))))
	return superblock.data, nil
}

func validPath(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.HasPrefix(s, "../") && !strings.HasPrefix(s, "/") && path.Clean(s) == s && !strings.ContainsAny(s, "\\\x00") && utf8.ValidString(s)
}
func swap32(v uint32) uint32 {
	return binary.BigEndian.Uint32(binary.LittleEndian.AppendUint32(nil, v))
}
func swap64(v uint64) uint64 {
	return binary.BigEndian.Uint64(binary.LittleEndian.AppendUint64(nil, v))
}
func emptyXattrs() value { return array(tuple(bytesValue(nil), bytesValue(nil))) }

func (d *directory) objects(objects *[]object) ([32]byte, [32]byte) {
	var files, dirs []value
	var names []string
	for n := range d.files {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		e := d.files[n]
		mode := e.Mode | 0o100000
		data := e.Data
		if e.Link != "" {
			mode = e.Mode | 0o120000
			data = []byte(e.Link)
		}
		header := tuple(uint32Value(0), uint32Value(0), uint32Value(swap32(mode)), uint32Value(0), stringValue(e.Link), emptyXattrs())
		h := sha256.New()
		h.Write(binary.BigEndian.AppendUint32(nil, uint32(len(header.data))))
		h.Write([]byte{0, 0, 0, 0})
		h.Write(header.data)
		if e.Link == "" {
			h.Write(data)
		}
		var sum [32]byte
		copy(sum[:], h.Sum(nil))
		*objects = append(*objects, object{1, sum, data, mode})
		files = append(files, tuple(stringValue(n), bytesValue(sum[:])))
	}
	names = nil
	for n := range d.dirs {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		tree, meta := d.dirs[n].objects(objects)
		dirs = append(dirs, tuple(stringValue(n), bytesValue(tree[:]), bytesValue(meta[:])))
	}
	tree := tuple(array(tuple(stringValue(""), bytesValue(nil)), files...), array(tuple(stringValue(""), bytesValue(nil), bytesValue(nil)), dirs...))
	meta := tuple(uint32Value(0), uint32Value(0), uint32Value(swap32(d.mode|0o40000)), emptyXattrs())
	ts, ms := sha256.Sum256(tree.data), sha256.Sum256(meta.data)
	*objects = append(*objects, object{2, ts, tree.data, 0}, object{3, ms, meta.data, 0})
	return ts, ms
}

func deltaPart(objects []object) (value, value) {
	var modes []value
	modeIDs := map[uint32]uint64{}
	var data, ops, sums []byte
	var size uint64
	seen := map[[33]byte]bool{}
	for _, o := range objects {
		var key [33]byte
		key[0] = o.kind
		copy(key[1:], o.sum[:])
		if seen[key] {
			continue
		}
		seen[key] = true
		sums = append(sums, key[:]...)
		ops = append(ops, 'S')
		if o.kind == 1 {
			id, ok := modeIDs[o.mode]
			if !ok {
				id = uint64(len(modes))
				modeIDs[o.mode] = id
				modes = append(modes, tuple(uint32Value(0), uint32Value(0), uint32Value(swap32(o.mode))))
			}
			ops = binary.AppendUvarint(ops, id)
			ops = binary.AppendUvarint(ops, 0) // empty xattrs
		}
		ops = binary.AppendUvarint(ops, uint64(len(o.data)))
		ops = binary.AppendUvarint(ops, uint64(len(data)))
		data = append(data, o.data...)
		size += uint64(len(o.data))
	}
	payload := tuple(array(tuple(uint32Value(0), uint32Value(0), uint32Value(0)), modes...), array(emptyXattrs(), emptyXattrs()), bytesValue(data), bytesValue(ops))
	part := tuple(byteValue(0), bytesValue(payload.data)) // uncompressed OSTree part
	sum := sha256.Sum256(part.data)
	header := tuple(uint32Value(0), bytesValue(sum[:]), uint64Value(uint64(len(part.data))), uint64Value(size), bytesValue(sums))
	return part, header
}
