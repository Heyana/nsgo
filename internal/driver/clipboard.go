package driver

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf16"
)

// ClipboardItem is one native clipboard item, in the order its formats are offered.
type ClipboardItem struct {
	Representations []ClipboardRepresentation
}

// ClipboardRepresentation is the bytes supplied for a platform's native format.
type ClipboardRepresentation struct {
	Type string
	Data []byte
}

// ClipboardResult is an EventClipboard for the item, write or change-count request.
type ClipboardResult struct {
	Items       []ClipboardItem
	Formats     []string
	ChangeCount int64
	Err         error
}

var ErrClipboardQuota = errors.New("clipboard representations exceed 48 MiB")

// ValidateClipboardItems checks the total byte quota and each item's format names.
func ValidateClipboardItems(items []ClipboardItem) error {
	remaining := MaxClipboardImage
	for i, item := range items {
		if len(item.Representations) == 0 {
			return fmt.Errorf("clipboard item %d has no representations", i+1)
		}
		types := make(map[string]bool, len(item.Representations))
		for _, rep := range item.Representations {
			if rep.Type == "" || strings.ContainsRune(rep.Type, 0) {
				return fmt.Errorf("clipboard item %d has an invalid format name", i+1)
			}
			if types[rep.Type] {
				return fmt.Errorf("clipboard item %d repeats format %q", i+1, rep.Type)
			}
			types[rep.Type] = true
			if len(rep.Data) > remaining {
				return ErrClipboardQuota
			}
			remaining -= len(rep.Data)
		}
	}
	return nil
}

func CloneClipboardItems(items []ClipboardItem) []ClipboardItem {
	out := slices.Clone(items)
	for i := range out {
		reps := slices.Clone(out[i].Representations)
		for j := range reps {
			reps[j].Data = slices.Clone(reps[j].Data)
		}
		out[i].Representations = reps
	}
	return out
}

// CloneClipboardItem returns a deep copy of item, or nil when item is nil.
func CloneClipboardItem(item *ClipboardItem) *ClipboardItem {
	if item == nil {
		return nil
	}
	reps := slices.Clone(item.Representations)
	for i := range reps {
		reps[i].Data = slices.Clone(reps[i].Data)
	}
	return &ClipboardItem{Representations: reps}
}

// ClipboardItemEqual reports whether a and b hold the same ordered representations and bytes.
func ClipboardItemEqual(a, b *ClipboardItem) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil || len(a.Representations) != len(b.Representations) {
		return false
	}
	for i := range a.Representations {
		ra, rb := a.Representations[i], b.Representations[i]
		if ra.Type != rb.Type || !bytes.Equal(ra.Data, rb.Data) {
			return false
		}
	}
	return true
}

// ClipboardItemWithPlain returns a copy of item whose text/plain representation is plain when plain is non-empty.
func ClipboardItemWithPlain(item *ClipboardItem, plain string) ClipboardItem {
	cloned := CloneClipboardItem(item)
	if cloned == nil {
		if plain == "" {
			return ClipboardItem{}
		}
		return ClipboardItem{Representations: []ClipboardRepresentation{{Type: "text/plain", Data: []byte(plain)}}}
	}
	if plain == "" {
		return *cloned
	}
	for i := range cloned.Representations {
		if cloned.Representations[i].Type == "text/plain" {
			cloned.Representations[i].Data = []byte(plain)
			return *cloned
		}
	}
	reps := make([]ClipboardRepresentation, 0, len(cloned.Representations)+1)
	reps = append(reps, ClipboardRepresentation{Type: "text/plain", Data: []byte(plain)})
	reps = append(reps, cloned.Representations...)
	return ClipboardItem{Representations: reps}
}

func (c *MemoryClipboard) setItems(items []ClipboardItem) {
	c.Items = CloneClipboardItems(items)
	c.Text, c.Image, c.Files = "", nil, nil
	for _, item := range c.Items {
		for _, rep := range item.Representations {
			switch rep.Type {
			case "public.utf8-plain-text", "text/plain", "text/plain;charset=utf-8":
				c.Text = string(rep.Data)
			case "CF_UNICODETEXT":
				c.Text = windowsClipboardText(rep.Data)
			case "public.png", "image/png", "PNG":
				c.Image = rep.Data
			case "public.file-url":
				c.Files = append(c.Files, string(rep.Data))
			}
		}
	}
	c.ChangeCount++
}

// windowsClipboardText decodes the nul-terminated UTF-16LE format Win32 uses.
func windowsClipboardText(data []byte) string {
	var units []uint16
	for len(data) >= 2 {
		unit := binary.LittleEndian.Uint16(data)
		if unit == 0 {
			break
		}
		units = append(units, unit)
		data = data[2:]
	}
	return string(utf16.Decode(units))
}

func (c *MemoryClipboard) snapshot() []ClipboardItem {
	if c.Items != nil {
		return c.Items
	}
	var reps []ClipboardRepresentation
	if c.Text != "" {
		reps = append(reps, ClipboardRepresentation{Type: "public.utf8-plain-text", Data: []byte(c.Text)})
	}
	if len(c.Image) > 0 {
		reps = append(reps, ClipboardRepresentation{Type: "public.png", Data: c.Image})
	}
	var items []ClipboardItem
	if len(reps) > 0 {
		items = append(items, ClipboardItem{Representations: reps})
	}
	for _, file := range c.Files {
		items = append(items, ClipboardItem{Representations: []ClipboardRepresentation{{Type: "public.file-url", Data: []byte(file)}}})
	}
	return items
}
