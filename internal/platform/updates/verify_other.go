//go:build !unix

package updates

import (
	"crypto/ed25519"
	"io"
	"os"
)

func verifyEd25519File(f *os.File, size int64, key ed25519.PublicKey, sig []byte) (bool, error) {
	if size == 0 {
		return ed25519.Verify(key, nil, sig), nil
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
