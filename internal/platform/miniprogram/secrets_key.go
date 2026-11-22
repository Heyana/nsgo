package miniprogram

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"
)

// secretStoragePrefix keeps secrets disjoint from plain storage's "nsgo:<id>:"
// keys so Storage.Load and Storage.Clear never read or delete them.
const secretStoragePrefix = "nsgo-secret:"

// secretStorageKey names (service, account) in encrypted mini program storage
// without collisions across delimiters or length boundaries.
func secretStorageKey(service, account string) string {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(service)))
	h := sha256.New()
	h.Write(n[:])
	h.Write([]byte(service))
	h.Write([]byte(account))
	return secretStoragePrefix + hex.EncodeToString(h.Sum(nil))
}

// isStorageNotFound reports whether a mini program storage failure means no
// entry exists for the key (e.g. WeChat's "getStorage:fail data not found").
func isStorageNotFound(errMsg string) bool {
	return strings.Contains(strings.ToLower(errMsg), "data not found")
}
