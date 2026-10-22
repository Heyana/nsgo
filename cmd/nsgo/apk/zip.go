package apk

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

// File is one entry of an APK.
type File struct {
	Name string
	Data []byte
	// Stored keeps Data uncompressed, for files the system maps instead
	// of reading whole.
	Stored bool
}

// The ZIP records packaging writes, from APPNOTE.TXT.
const (
	localHeader    = 0x04034b50
	centralHeader  = 0x02014b50
	endOfDirectory = 0x06054b50

	store   = 0
	deflate = 8

	// A fixed timestamp keeps builds reproducible. The DOS date is
	// 1980-01-01, the earliest it can hold.
	dosDate = 0x0021
	// zipalign's padding extra field; its ID is not reserved by any
	// reader, and Android ignores it.
	paddingID  = 0xd935
	alignment  = 4
	version    = 20 // 2.0, for deflate
	maxEntries = 0xffff
	maxSize    = 0xfffffffe
)

// archive writes the files as a ZIP archive, uncompressed entries aligned
// so the system can map them.
func archive(files []File) ([]byte, error) {
	if len(files) > maxEntries {
		return nil, errors.New("an APK holds at most 65535 files")
	}
	var body bytes.Buffer
	type entry struct {
		File
		method     uint16
		crc        uint32
		compressed []byte
		offset     uint32
		extra      []byte
	}
	entries := make([]*entry, 0, len(files))
	for _, f := range files {
		e := &entry{File: f, crc: crc32.ChecksumIEEE(f.Data)}
		if f.Stored {
			e.compressed = f.Data
			// Align the bytes that follow the local header and name.
			start := body.Len() + 30 + len(f.Name)
			if pad := (alignment - start%alignment) % alignment; pad > 0 {
				e.extra = make([]byte, 4+pad)
				binary.LittleEndian.PutUint16(e.extra, paddingID)
				binary.LittleEndian.PutUint16(e.extra[2:], uint16(pad))
			}
		} else {
			e.method = deflate
			var buf bytes.Buffer
			w, err := flate.NewWriter(&buf, flate.BestCompression)
			if err != nil {
				return nil, err
			}
			if _, err := w.Write(f.Data); err != nil {
				return nil, err
			}
			if err := w.Close(); err != nil {
				return nil, err
			}
			e.compressed = buf.Bytes()
		}
		if len(f.Data) > maxSize || len(e.compressed) > maxSize {
			return nil, fmt.Errorf("%s: an APK holds files below 4 GiB", f.Name)
		}
		e.offset = uint32(body.Len())
		put := func(v any) { binary.Write(&body, binary.LittleEndian, v) }
		put(uint32(localHeader))
		put(uint16(version))
		put(uint16(0)) // flags
		put(e.method)
		put(uint16(0)) // time
		put(uint16(dosDate))
		put(e.crc)
		put(uint32(len(e.compressed)))
		put(uint32(len(f.Data)))
		put(uint16(len(f.Name)))
		put(uint16(len(e.extra)))
		body.WriteString(f.Name)
		body.Write(e.extra)
		body.Write(e.compressed)
		entries = append(entries, e)
	}

	dir := body.Len()
	for _, e := range entries {
		put := func(v any) { binary.Write(&body, binary.LittleEndian, v) }
		put(uint32(centralHeader))
		put(uint16(version)) // version made by
		put(uint16(version))
		put(uint16(0)) // flags
		put(e.method)
		put(uint16(0))
		put(uint16(dosDate))
		put(e.crc)
		put(uint32(len(e.compressed)))
		put(uint32(len(e.Data)))
		put(uint16(len(e.Name)))
		put(uint16(0)) // extra
		put(uint16(0)) // comment
		put(uint16(0)) // disk
		put(uint16(0)) // internal attributes
		put(uint32(0)) // external attributes
		put(e.offset)
		body.WriteString(e.Name)
	}
	size := body.Len() - dir

	put := func(v any) { binary.Write(&body, binary.LittleEndian, v) }
	put(uint32(endOfDirectory))
	put(uint16(0)) // disk
	put(uint16(0)) // directory's disk
	put(uint16(len(entries)))
	put(uint16(len(entries)))
	put(uint32(size))
	put(uint32(dir))
	put(uint16(0)) // comment
	return body.Bytes(), nil
}
