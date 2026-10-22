package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// The COFF and resource constants iconResource writes, from the PE format
// specification.
const (
	rtIcon      = 3
	rtGroupIcon = 14
	// iconGroupID is the group platform/windows loads the window's icon
	// from; Explorer shows the first group.
	iconGroupID = 1
	langEnUS    = 0x0409
)

// iconResource writes a COFF object with a .rsrc section holding one icon
// group of PNG images, which go build links into a Windows executable.
func iconResource(images [][]byte, sizes []int, goarch string) ([]byte, error) {
	machine, reloc := uint16(0), uint16(0)
	switch goarch {
	case "amd64":
		machine, reloc = 0x8664, 0x0003 // IMAGE_REL_AMD64_ADDR32NB
	case "arm64":
		machine, reloc = 0xaa64, 0x0002 // IMAGE_REL_ARM64_ADDR32NB
	case "386":
		machine, reloc = 0x014c, 0x0007 // IMAGE_REL_I386_DIR32NB
	default:
		return nil, fmt.Errorf("no Windows resources for %s", goarch)
	}

	// The group lists each image with the ID of its RT_ICON resource.
	var group bytes.Buffer
	put := func(b *bytes.Buffer, v any) { binary.Write(b, binary.LittleEndian, v) }
	put(&group, [3]uint16{0, 1, uint16(len(images))})
	for i, img := range images {
		side := uint8(sizes[i])
		if sizes[i] >= 256 {
			side = 0
		}
		put(&group, [4]uint8{side, side, 0, 0})
		put(&group, [2]uint16{1, 32})
		put(&group, uint32(len(img)))
		put(&group, uint16(i+1))
	}

	// The resource tree has three levels: type, ID and language. Every
	// directory comes first, then the data entries, then the data.
	type leaf struct {
		id   uint32
		data []byte
	}
	types := []struct {
		id     uint32
		leaves []leaf
	}{{rtIcon, nil}, {rtGroupIcon, []leaf{{iconGroupID, group.Bytes()}}}}
	for i, img := range images {
		types[0].leaves = append(types[0].leaves, leaf{uint32(i + 1), img})
	}
	const dirSize, entrySize, dataEntrySize = 16, 8, 16
	leaves := 0
	for _, t := range types {
		leaves += len(t.leaves)
	}
	idDirs := dirSize + entrySize*len(types)
	langDirs := idDirs
	for _, t := range types {
		langDirs += dirSize + entrySize*len(t.leaves)
	}
	dataEntries := langDirs + leaves*(dirSize+entrySize)
	data := dataEntries + leaves*dataEntrySize

	var sec bytes.Buffer
	dir := func(entries int) { put(&sec, [4]uint32{0, 0, 0, uint32(entries) << 16}) }
	const subdir = 0x80000000
	dir(len(types))
	at := idDirs
	for _, t := range types {
		put(&sec, [2]uint32{t.id, subdir | uint32(at)})
		at += dirSize + entrySize*len(t.leaves)
	}
	at = langDirs
	for _, t := range types {
		dir(len(t.leaves))
		for _, l := range t.leaves {
			put(&sec, [2]uint32{l.id, subdir | uint32(at)})
			at += dirSize + entrySize
		}
	}
	at = dataEntries
	for _, t := range types {
		for range t.leaves {
			dir(1)
			put(&sec, [2]uint32{langEnUS, uint32(at)})
			at += dataEntrySize
		}
	}
	// Each data entry's address is relative to the section; the linker
	// relocates it to an image address.
	var relocs []uint32
	at = data
	for _, t := range types {
		for _, l := range t.leaves {
			relocs = append(relocs, uint32(sec.Len()))
			put(&sec, [4]uint32{uint32(at), uint32(len(l.data)), 0, 0})
			at += align8(len(l.data))
		}
	}
	for _, t := range types {
		for _, l := range t.leaves {
			sec.Write(l.data)
			sec.Write(make([]byte, align8(len(l.data))-len(l.data)))
		}
	}

	const fileHeader, sectionHeader, relocSize = 20, 40, 10
	rawData := fileHeader + sectionHeader
	relocations := rawData + sec.Len()
	symbols := relocations + relocSize*len(relocs)
	var out bytes.Buffer
	put(&out, machine)
	put(&out, uint16(1))                                           // sections
	put(&out, [3]uint32{0, uint32(symbols), 1})                    // time, symbol table, symbols
	put(&out, [2]uint16{0, 0})                                     // optional header, characteristics
	out.WriteString(".rsrc\x00\x00\x00")                           // name
	put(&out, [4]uint32{0, 0, uint32(sec.Len()), uint32(rawData)}) // virtual size and address, raw size and offset
	put(&out, [2]uint32{uint32(relocations), 0})                   // relocations, line numbers
	put(&out, [2]uint16{uint16(len(relocs)), 0})
	put(&out, uint32(0x40000040)) // initialized data, readable
	out.Write(sec.Bytes())
	for _, r := range relocs {
		put(&out, [2]uint32{r, 0})
		put(&out, reloc)
	}
	// One symbol, the section, which every relocation refers to.
	out.WriteString(".rsrc\x00\x00\x00")
	put(&out, uint32(0))
	put(&out, int16(1))
	put(&out, uint16(0))
	put(&out, [2]uint8{3, 0}) // static, no auxiliary records
	put(&out, uint32(4))      // an empty string table
	return out.Bytes(), nil
}

func align8(n int) int { return (n + 7) &^ 7 }
