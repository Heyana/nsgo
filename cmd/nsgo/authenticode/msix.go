package authenticode

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net/url"
	"path"
	"strings"
)

const msixSignature = "AppxSignature.p7x"
const msixSignatureType = "application/vnd.ms-appx.signature"

// SignMSIX returns an MSIX package signed by key, replacing an existing signature. It preserves payload bytes and block-map bytes, registers the signature's content type and carries the APPX SIP digest in CMS. Windows still requires a trusted signing identity to install it.
func SignMSIX(data []byte, key *Key, stamp Stamper) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("MSIX archive: %w", err)
	}
	var files []*zip.File
	var contentTypes, blockMap, codeIntegrity []byte
	seen := map[string]bool{}
	for _, f := range r.File {
		name, err := url.PathUnescape(f.Name)
		if err != nil || name != path.Clean(name) || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00") {
			return nil, fmt.Errorf("invalid MSIX part %q", f.Name)
		}
		folded := strings.ToLower(name)
		if seen[folded] {
			return nil, fmt.Errorf("duplicate MSIX part %q", name)
		}
		seen[folded] = true
		if folded == strings.ToLower(msixSignature) {
			continue
		}
		files = append(files, f)
		switch name {
		case "[Content_Types].xml", "AppxBlockMap.xml", "AppxMetadata/CodeIntegrity.cat":
			b, err := msixRead(f)
			if err != nil {
				return nil, err
			}
			switch name {
			case "[Content_Types].xml":
				contentTypes = b
			case "AppxBlockMap.xml":
				blockMap = b
			default:
				codeIntegrity = b
			}
		}
	}
	if !seen[strings.ToLower("AppxManifest.xml")] || len(contentTypes) == 0 || len(blockMap) == 0 {
		return nil, errors.New("MSIX needs AppxManifest.xml, AppxBlockMap.xml and [Content_Types].xml")
	}
	contentTypes, err = msixRegisterSignature(contentTypes)
	if err != nil {
		return nil, err
	}
	unsigned, err := msixArchive(files, contentTypes, nil, r.Comment)
	if err != nil {
		return nil, err
	}
	directory, _, _, comment, err := msixDirectory(unsigned)
	if err != nil {
		return nil, err
	}
	// AppxSip hashes zero disk numbers in the classic EOCD even though AppxPackaging writes ZIP64 sentinels there in the actual package.
	directoryData := bytes.Clone(unsigned[directory:])
	end := len(directoryData) - 22 - len(comment)
	clear(directoryData[end+4 : end+8])
	blob := []byte("APPX")
	for _, part := range []struct {
		tag  string
		data []byte
	}{
		{"AXPC", unsigned[:directory]}, {"AXCD", directoryData},
		{"AXCT", contentTypes}, {"AXBM", blockMap},
	} {
		sum := sha256.Sum256(part.data)
		blob = append(blob, part.tag...)
		blob = append(blob, sum[:]...)
	}
	if codeIntegrity != nil {
		sum := sha256.Sum256(codeIntegrity)
		blob = append(blob, "AXCI"...)
		blob = append(blob, sum[:]...)
	}
	// The package SIP GUID and digest names are defined by Microsoft's MSIX signature verifier: https://github.com/microsoft/msix-packaging/blob/master/src/inc/internal/AppxSignature.hpp.
	guid := []byte{0x4b, 0xdf, 0xc5, 0x0a, 0x07, 0xce, 0xe2, 0x4d, 0xb7, 0x6e, 0x23, 0xc8, 0x39, 0xa0, 0x9f, 0xd1}
	sip := sequence(smallInt(0x01010000), octetString(guid), smallInt(0), smallInt(0), smallInt(0), smallInt(0), smallInt(0))
	content := sequence(sequence(oid(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 2, 1, 30}), sip), sequence(algorithmIdentifier(oidSHA256), octetString(blob)))
	signature, err := signedContent(content, key, stamp)
	if err != nil {
		return nil, err
	}
	return msixArchive(files, contentTypes, append([]byte("PKCX"), signature...), r.Comment)
}

func msixRead(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

type msixTypes struct {
	XMLName   xml.Name       `xml:"http://schemas.openxmlformats.org/package/2006/content-types Types"`
	Defaults  []msixDefault  `xml:"Default"`
	Overrides []msixOverride `xml:"Override"`
}
type msixDefault struct {
	Extension   string `xml:"Extension,attr"`
	ContentType string `xml:"ContentType,attr"`
}
type msixOverride struct {
	PartName    string `xml:"PartName,attr"`
	ContentType string `xml:"ContentType,attr"`
}

func msixRegisterSignature(data []byte) ([]byte, error) {
	var types msixTypes
	if err := xml.Unmarshal(data, &types); err != nil {
		return nil, fmt.Errorf("MSIX content types: %w", err)
	}
	found := false
	for i := range types.Overrides {
		o := &types.Overrides[i]
		if strings.EqualFold(o.PartName, "/"+msixSignature) {
			if found || o.ContentType != msixSignatureType {
				return nil, errors.New("invalid MSIX signature content type")
			}
			found = true
			o.PartName = "/" + msixSignature
		}
	}
	if !found {
		types.Overrides = append(types.Overrides, msixOverride{"/" + msixSignature, msixSignatureType})
	}
	out, err := xml.Marshal(types)
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

func msixArchive(files []*zip.File, contentTypes, signature []byte, comment string) ([]byte, error) {
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	if err := w.SetComment(comment); err != nil {
		return nil, err
	}
	add := func(name string, b []byte) error {
		h := &zip.FileHeader{Name: name, Method: zip.Store, CreatorVersion: 45, ReaderVersion: 20, CRC32: crc32.ChecksumIEEE(b), CompressedSize64: uint64(len(b)), UncompressedSize64: uint64(len(b)), ModifiedDate: 0x21}
		entry, err := w.CreateRaw(h)
		if err != nil {
			return err
		}
		_, err = entry.Write(b)
		return err
	}
	for _, f := range files {
		var err error
		if f.Name == "[Content_Types].xml" {
			err = add(f.Name, contentTypes)
		} else {
			var raw io.Reader
			raw, err = f.OpenRaw()
			if err == nil {
				header := f.FileHeader
				header.ReaderVersion = max(header.ReaderVersion, 20)
				// AppxSip rejects other creator versions or external attributes even when the archive and digest are otherwise consistent.
				header.CreatorVersion = 45
				header.ExternalAttrs = 0
				var entry io.Writer
				entry, err = w.CreateRaw(&header)
				if err == nil {
					_, err = io.Copy(entry, raw)
				}
			}
		}
		if err != nil {
			return nil, err
		}
	}
	if signature != nil {
		if err := add(msixSignature, signature); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return msixZIP64(out.Bytes())
}

// msixDirectory locates ZIP's central directory and end records. The returned offsets are bounded by the archive before conversion to int.
func msixDirectory(data []byte) (offset, size, count int, comment []byte, err error) {
	invalid := errors.New("invalid MSIX ZIP end records")
	end := -1
	for i := len(data) - 22; i >= max(0, len(data)-22-65535); i-- {
		if binary.LittleEndian.Uint32(data[i:]) == 0x06054b50 && i+22+int(binary.LittleEndian.Uint16(data[i+20:])) == len(data) {
			end = i
			break
		}
	}
	if end < 0 {
		return 0, 0, 0, nil, invalid
	}
	n := uint64(binary.LittleEndian.Uint16(data[end+10:]))
	s := uint64(binary.LittleEndian.Uint32(data[end+12:]))
	o := uint64(binary.LittleEndian.Uint32(data[end+16:]))
	if n == 0xffff || s == 0xffffffff || o == 0xffffffff {
		if end < 20 || binary.LittleEndian.Uint32(data[end-20:]) != 0x07064b50 {
			return 0, 0, 0, nil, invalid
		}
		z := binary.LittleEndian.Uint64(data[end-12:])
		if z > uint64(end-20) || uint64(end-20)-z < 56 || binary.LittleEndian.Uint32(data[int(z):]) != 0x06064b50 {
			return 0, 0, 0, nil, invalid
		}
		n = binary.LittleEndian.Uint64(data[int(z)+32:])
		s = binary.LittleEndian.Uint64(data[int(z)+40:])
		o = binary.LittleEndian.Uint64(data[int(z)+48:])
	}
	if o > uint64(len(data)) || s > uint64(len(data))-o || n > uint64(len(data)) {
		return 0, 0, 0, nil, invalid
	}
	return int(o), int(s), int(n), data[end+22:], nil
}

// Both signing passes use AppxPackaging's ZIP64 end records and classic sentinels. Windows AppxSip rejects a signature over a classic ZIP archive; keeping this form also avoids a size/count threshold changing the digest's footer format after appending the signature. Payload local headers stay unchanged.
func msixZIP64(data []byte) ([]byte, error) {
	offset, size, count, comment, err := msixDirectory(data)
	if err != nil {
		return nil, err
	}
	end := offset + size
	out := append([]byte(nil), data[:end]...)
	record := make([]byte, 56+20+22)
	put16 := func(i int, n uint16) { binary.LittleEndian.PutUint16(record[i:], n) }
	put32 := func(i int, n uint32) { binary.LittleEndian.PutUint32(record[i:], n) }
	put64 := func(i int, n uint64) { binary.LittleEndian.PutUint64(record[i:], n) }
	put32(0, 0x06064b50)
	put64(4, 44)
	put16(12, 45)
	put16(14, 45)
	put64(24, uint64(count))
	put64(32, uint64(count))
	put64(40, uint64(size))
	put64(48, uint64(offset))
	put32(56, 0x07064b50)
	put64(64, uint64(end))
	put32(72, 1)
	put32(76, 0x06054b50)
	put16(80, 0xffff)
	put16(82, 0xffff)
	put16(84, 0xffff)
	put16(86, 0xffff)
	put32(88, 0xffffffff)
	put32(92, 0xffffffff)
	put16(96, uint16(len(comment)))
	out = append(out, record...)
	return append(out, comment...), nil
}
