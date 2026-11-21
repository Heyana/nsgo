package driver

import (
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
