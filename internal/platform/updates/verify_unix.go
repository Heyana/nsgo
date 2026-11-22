//go:build unix

package updates

import (
	"crypto/ed25519"
	"io"
	"os"
	"syscall"
)

// verifyEd25519File verifies sig against f's bytes without copying the file
// onto the Go heap by mapping it read-only. If mmap is unavailable on the
// backing filesystem, it falls back to reading f.
func verifyEd25519File(f *os.File, size int64, key ed25519.PublicKey, sig []byte) (bool, error) {
	if size == 0 {
		return ed25519.Verify(key, nil, sig), nil
	}
	if int64(int(size)) == size {
		data, err := syscall.Mmap(int(f.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
		if err == nil {
			defer syscall.Munmap(data)
			return ed25519.Verify(key, data, sig), nil
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return false, err
	}
	return ed25519.Verify(key, data, sig), nil
}
