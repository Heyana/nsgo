package windowsinstaller

import (
	"archive/zip"
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

var payloadMagic = [16]byte{'N', 'S', 'G', 'o', 'I', 'n', 's', 't', 'a', 'l', 'l', 'e', 'r', 0, 0, 1}

// AppendPayload adds a ZIP and a fixed footer after the Go PE image. Signing happens afterward: Authenticode covers this overlay as well as the image.
func AppendPayload(executable string, payload io.Reader) error {
	file, err := os.OpenFile(executable, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	offset, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	size, err := io.Copy(file, payload)
	if err != nil {
		return err
	}
	var footer [32]byte
	copy(footer[:], payloadMagic[:])
	binary.LittleEndian.PutUint64(footer[16:], uint64(offset))
	binary.LittleEndian.PutUint64(footer[24:], uint64(size))
	_, err = file.Write(footer[:])
	return err
}

// OpenPayload reads the archive in place instead of loading the app into RAM. A signature's certificate table and its alignment padding follow the footer.
func OpenPayload(executable string) (*zip.Reader, io.Closer, error) {
	file, err := os.Open(executable)
	if err != nil {
		return nil, nil, err
	}
	archive, err := payloadReader(file)
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return archive, file, nil
}
func payloadReader(file *os.File) (*zip.Reader, error) {
	image, err := pe.NewFile(file)
	if err != nil {
		return nil, err
	}
	defer image.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	end := info.Size()
	var certificate pe.DataDirectory
	switch h := image.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		certificate = h.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_SECURITY]
	case *pe.OptionalHeader64:
		certificate = h.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_SECURITY]
	}
	if certificate.VirtualAddress != 0 {
		end = int64(certificate.VirtualAddress)
	}
	if end < 32 || end > info.Size() {
		return nil, errors.New("invalid installer payload boundary")
	}
	size := int64(64)
	if end < size {
		size = end
	}
	tail := make([]byte, size)
	if _, err := file.ReadAt(tail, end-size); err != nil {
		return nil, err
	}
	i := bytes.LastIndex(tail, payloadMagic[:])
	if i < 0 || i+32 > len(tail) {
		return nil, errors.New("installer has no payload footer")
	}
	offset := binary.LittleEndian.Uint64(tail[i+16:])
	length := binary.LittleEndian.Uint64(tail[i+24:])
	footer := uint64(end - size + int64(i))
	if offset > footer || length != footer-offset {
		return nil, errors.New("invalid installer payload size")
	}
	return zip.NewReader(io.NewSectionReader(file, int64(offset), int64(length)), int64(length))
}
