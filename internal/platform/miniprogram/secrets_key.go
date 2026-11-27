package miniprogram

import (
	"encoding/binary"
	"encoding/hex"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// secretStoragePrefix keeps secrets disjoint from plain storage's "nsgo:<id>:"
// keys so Storage.Load and Storage.Clear never read or delete them.
const secretStoragePrefix = "nsgo-secret:"

// secretStorageKey names (service, account) in encrypted mini program storage
// without collisions across delimiters or length boundaries.
func secretStorageKey(service, account string) string {
	buf := make([]byte, 4+len(service)+len(account))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(service)))
	copy(buf[4:], service)
	copy(buf[4+len(service):], account)
	sum := driver.Sum256(buf)
	return secretStoragePrefix + hex.EncodeToString(sum[:])
}

// isStorageNotFound reports whether a mini program storage failure means no
// entry exists for the key (e.g. WeChat's "getStorage:fail data not found").
func isStorageNotFound(errMsg string) bool {
	return strings.Contains(strings.ToLower(errMsg), "data not found")
}
