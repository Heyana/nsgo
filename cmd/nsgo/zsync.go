package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/md4"
)

// zsyncControl writes the legacy format AppImageUpdate consumes: full 32-bit rolling sums and MD4 block hashes, plus the whole-file SHA-1. These hashes describe delta blocks; releaseAssets still writes SHA-256, and NSGo's appcast signatures remain separate.
// https://github.com/cph6/zsync/blob/master/SPEC.md#checksum-section
func zsyncControl(data []byte, name string, modified time.Time) ([]byte, error) {
	if name == "" || strings.ContainsAny(name, "/\\\x00\r\n") {
		return nil, errors.New("zsync filename must be a single line and path component")
	}
	const blockSize = 2048
	var out bytes.Buffer
	fmt.Fprintf(&out, "zsync: 0.6.2\nFilename: %s\nMTime: %s\nBlocksize: %d\nLength: %d\nHash-Lengths: 1,4,16\nURL: %s\nSHA-1: %x\n\n",
		name, modified.UTC().Format(time.RFC1123Z), blockSize, len(data), url.PathEscape(name), sha1.Sum(data))
	strong := md4.New()
	for offset := 0; offset < len(data); offset += blockSize {
		var block [blockSize]byte
		copy(block[:], data[offset:min(offset+blockSize, len(data))])
		var a, b uint16
		for i, value := range block {
			a += uint16(value)
			b += uint16(blockSize-i) * uint16(value)
		}
		var weak [4]byte
		binary.BigEndian.PutUint16(weak[:2], a)
		binary.BigEndian.PutUint16(weak[2:], b)
		out.Write(weak[:])
		strong.Reset()
		strong.Write(block[:])
		out.Write(strong.Sum(nil))
	}
	return out.Bytes(), nil
}
