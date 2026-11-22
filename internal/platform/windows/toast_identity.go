package windows

import (
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"
)

// Short IDs keep their existing WinRT identity. Longer IDs use a separate
// group, so an app's short ID cannot collide with a generated tag. Tags stay
// within the original 16 UTF-16-unit limit, also on older Windows 10 builds.
const (
	toastGroup     = "nsgo"
	toastLongGroup = "nsgo.long"
)

func toastIdentity(id string) (tag, group string) {
	if len(utf16.Encode([]rune(id))) <= 16 {
		return id, toastGroup
	}
	sum := sha256.Sum256([]byte(id))
	return base64.RawURLEncoding.EncodeToString(sum[:12]), toastLongGroup
}

// toastMatches includes the raw tags older versions used for long IDs.
func toastMatches(id, tag, group string) bool {
	if id == "" {
		return group == toastGroup || group == toastLongGroup
	}
	wantTag, wantGroup := toastIdentity(id)
	return tag == wantTag && group == wantGroup || tag == id && group == toastGroup
}

// toastNonce counts the toasts shown, from the time the process first showed one, so a toast of an earlier run of the app, pressed after it ended, does not share its nonce with one this run shows.
var toastNonce struct {
	sync.Once
	n atomic.Uint64
}

// toastLaunch is the launch arguments of a toast shown with id: a nonce new for each toast, then the ID, which runs to the end and may hold any character.
func toastLaunch(id string) string {
	toastNonce.Do(func() { toastNonce.n.Store(uint64(time.Now().UnixNano())) })
	return "n=" + strconv.FormatUint(toastNonce.n.Add(1), 36) + ";id=" + id
}

// parseToastActivationArgs returns the ID of the toast launch arguments args name and the arguments the press is known by. It also takes the bare id=<ID> of a toast an earlier version of the app showed.
func parseToastActivationArgs(args string) (id, press string) {
	press = args
	rest := press
	if nonce, after, ok := strings.Cut(rest, ";"); ok && strings.HasPrefix(nonce, "n=") && strings.HasPrefix(after, "id=") {
		rest = after
	}
	if !strings.HasPrefix(rest, "id=") {
		return "", ""
	}
	return strings.TrimPrefix(rest, "id="), press
}
