// Package cloud provides provider-neutral cloud key-value and file/snapshot storage across platforms: iCloud Key-Value Store and ubiquity containers on macOS and iOS, Google Play Games Saved Games on Android, and an isolated per-app in-memory store in nsgotest.
//
// All operations are asynchronous and deliver their callbacks on the engine goroutine through core.Scope.Post; callbacks whose scope has been disposed never run. WatchChanges and OnConflict automatically clean up when cx is disposed.
//
// Unavailable accounts, missing entitlements, or platforms without a cloud provider (such as Windows, Linux and the web) always fail with ErrUnavailable and never silently drop writes. On desktop stores such as Steam, Steam Auto-Cloud syncs core.Scope.DataDir directly without using this package.
package cloud

import (
	"bytes"
	"errors"
	"path"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/nsgo-dev/nsgo/core"
)

// Sentinel errors returned by package cloud operations.
var (
	// ErrUnavailable reports that cloud storage is unavailable on this platform, the user is not signed in, or the app lacks the required entitlements. Writes are never silently dropped.
	ErrUnavailable = core.ErrCloudUnavailable
	// ErrNotFound reports that a requested key or file does not exist.
	ErrNotFound = core.ErrCloudNotFound
	// ErrConflict reports that a cloud file or key has conflicting local and remote versions that must be resolved.
	ErrConflict = core.ErrCloudConflict
)

// ConflictError describes a conflict between local and remote versions of a cloud key or file. It unwraps to ErrConflict.
type ConflictError = core.CloudConflictError

// ChangeReason explains why an external cloud storage change arrived.
type ChangeReason = core.CloudChangeReason

const (
	// ChangeServer reports data updated from another device or the server.
	ChangeServer ChangeReason = core.CloudChangeServer
	// ChangeInitialSync reports values downloaded on initial sync after install or sign-in.
	ChangeInitialSync ChangeReason = core.CloudChangeInitialSync
	// ChangeQuotaViolation reports that the cloud key-value store exceeded its quota.
	ChangeQuotaViolation ChangeReason = core.CloudChangeQuotaViolation
	// ChangeAccount reports that the signed-in cloud account changed.
	ChangeAccount ChangeReason = core.CloudChangeAccount
)

// ChangeEvent describes keys and files changed externally in cloud storage.
type ChangeEvent = core.CloudChangeEvent

// Conflict describes a conflicting remote version of a cloud file or key along with a Resolve function that commits the winning bytes.
type Conflict = core.CloudConflict

var (
	errInvalidKey  = errors.New("cloud: key must not be empty")
	errInvalidName = errors.New("cloud: invalid file name")
)

func validateKey(key string) error {
	if key == "" {
		return errInvalidKey
	}
	return nil
}

// validateName checks that name is a clean relative path inside the app's cloud documents container without traversal segments or drive/backslash separators.
func validateName(name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00") {
		return "", errInvalidName
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", errInvalidName
	}
	return clean, nil
}

func requireScope(cx *core.Scope) {
	if cx == nil {
		panic("cloud: nil scope")
	}
}

// Get reads the value for key from the cloud key-value store and delivers the result to done on the engine goroutine. It returns ErrNotFound if key does not exist and ErrUnavailable if cloud storage is unavailable.
func Get(cx *core.Scope, key string, done func(value []byte, err error)) {
	requireScope(cx)
	if cx.Disposed() {
		return
	}
	if err := validateKey(key); err != nil {
		cx.Post(func() {
			if done != nil {
				done(nil, err)
			}
		})
		return
	}
	if store := cx.CloudStore(); store != nil {
		val, err := store.Get(key)
		cx.Post(func() {
			if done != nil {
				done(bytes.Clone(val), err)
			}
		})
		return
	}
	go func() {
		val, err := platformGet(key)
		cx.Post(func() {
			if done != nil {
				done(val, err)
			}
		})
	}()
}

// Set writes value for key in the cloud key-value store and delivers the result to done on the engine goroutine. It returns ErrUnavailable if cloud storage is unavailable and never silently drops writes.
func Set(cx *core.Scope, key string, value []byte, done func(err error)) {
	requireScope(cx)
	if cx.Disposed() {
		return
	}
	if err := validateKey(key); err != nil {
		cx.Post(func() {
			if done != nil {
				done(err)
			}
		})
		return
	}
	copied := bytes.Clone(value)
	if store := cx.CloudStore(); store != nil {
		err := store.Set(key, copied)
		cx.Post(func() {
			if done != nil {
				done(err)
			}
		})
		return
	}
	go func() {
		err := platformSet(key, copied)
		cx.Post(func() {
			if done != nil {
				done(err)
			}
		})
	}()
}

// Delete removes key from the cloud key-value store and delivers the result to done on the engine goroutine. Deleting a missing key succeeds when cloud storage is available; an unavailable store returns ErrUnavailable.
func Delete(cx *core.Scope, key string, done func(err error)) {
	requireScope(cx)
	if cx.Disposed() {
		return
	}
	if err := validateKey(key); err != nil {
		cx.Post(func() {
			if done != nil {
				done(err)
			}
		})
		return
	}
	if store := cx.CloudStore(); store != nil {
		err := store.Delete(key)
		cx.Post(func() {
			if done != nil {
				done(err)
			}
		})
		return
	}
	go func() {
		err := platformDelete(key)
		cx.Post(func() {
			if done != nil {
				done(err)
			}
		})
	}()
}

// Keys lists all keys currently present in the cloud key-value store, sorted, and delivers them to done on the engine goroutine.
func Keys(cx *core.Scope, done func(keys []string, err error)) {
	requireScope(cx)
	if cx.Disposed() {
		return
	}
	if store := cx.CloudStore(); store != nil {
		keys, err := store.Keys()
		cx.Post(func() {
			if done != nil {
				done(slices.Clone(keys), err)
			}
		})
		return
	}
	go func() {
		keys, err := platformKeys()
		cx.Post(func() {
			if done != nil {
				done(keys, err)
			}
		})
	}()
}

// ReadFile reads the cloud file or snapshot named name and delivers its bytes to done on the engine goroutine. It returns ErrNotFound if missing, *ConflictError (unwrapping to ErrConflict) if unresolved conflicts exist, or ErrUnavailable if cloud storage is unavailable.
func ReadFile(cx *core.Scope, name string, done func(data []byte, err error)) {
	requireScope(cx)
	if cx.Disposed() {
		return
	}
	clean, err := validateName(name)
	if err != nil {
		cx.Post(func() {
			if done != nil {
				done(nil, err)
			}
		})
		return
	}
	if store := cx.CloudStore(); store != nil {
		data, err := store.ReadFile(clean)
		cx.Post(func() {
			if done != nil {
				done(bytes.Clone(data), err)
			}
		})
		return
	}
	go func() {
		data, err := platformReadFile(clean)
		cx.Post(func() {
			if done != nil {
				done(data, err)
			}
		})
	}()
}

// WriteFile writes data to the cloud file or snapshot named name and delivers the result to done on the engine goroutine. It returns ErrUnavailable if cloud storage is unavailable and never silently drops writes.
func WriteFile(cx *core.Scope, name string, data []byte, done func(err error)) {
	requireScope(cx)
	if cx.Disposed() {
		return
	}
	clean, err := validateName(name)
	if err != nil {
		cx.Post(func() {
			if done != nil {
				done(err)
			}
		})
		return
	}
	copied := bytes.Clone(data)
	if store := cx.CloudStore(); store != nil {
		err := store.WriteFile(clean, copied)
		cx.Post(func() {
			if done != nil {
				done(err)
			}
		})
		return
	}
	go func() {
		err := platformWriteFile(clean, copied)
		cx.Post(func() {
			if done != nil {
				done(err)
			}
		})
	}()
}

// DeleteFile removes the cloud file or snapshot named name and delivers the result to done on the engine goroutine. Deleting a missing file succeeds when cloud storage is available; an unavailable store returns ErrUnavailable.
func DeleteFile(cx *core.Scope, name string, done func(err error)) {
	requireScope(cx)
	if cx.Disposed() {
		return
	}
	clean, err := validateName(name)
	if err != nil {
		cx.Post(func() {
			if done != nil {
				done(err)
			}
		})
		return
	}
	if store := cx.CloudStore(); store != nil {
		err := store.DeleteFile(clean)
		cx.Post(func() {
			if done != nil {
				done(err)
			}
		})
		return
	}
	go func() {
		err := platformDeleteFile(clean)
		cx.Post(func() {
			if done != nil {
				done(err)
			}
		})
	}()
}

// ListFiles lists all cloud files or snapshots currently stored, sorted, and delivers them to done on the engine goroutine.
func ListFiles(cx *core.Scope, done func(names []string, err error)) {
	requireScope(cx)
	if cx.Disposed() {
		return
	}
	if store := cx.CloudStore(); store != nil {
		names, err := store.ListFiles()
		cx.Post(func() {
			if done != nil {
				done(slices.Clone(names), err)
			}
		})
		return
	}
	go func() {
		names, err := platformListFiles()
		cx.Post(func() {
			if done != nil {
				done(names, err)
			}
		})
	}()
}

// WatchChanges subscribes onChange to external cloud key-value and file changes. Notifications are delivered on the engine goroutine and the subscription is automatically removed when cx is disposed or stop is called.
func WatchChanges(cx *core.Scope, onChange func(ChangeEvent)) (stop func()) {
	requireScope(cx)
	if onChange == nil || cx.Disposed() {
		return func() {}
	}
	var stopped atomic.Bool
	deliver := func(ev ChangeEvent) {
		if stopped.Load() {
			return
		}
		evCopy := ChangeEvent{
			Keys:   slices.Clone(ev.Keys),
			Files:  slices.Clone(ev.Files),
			Reason: ev.Reason,
		}
		cx.Post(func() {
			if !stopped.Load() {
				onChange(evCopy)
			}
		})
	}
	var cancel func()
	if store := cx.CloudStore(); store != nil {
		cancel = store.WatchChanges(deliver)
	} else {
		cancel = platformWatchChanges(deliver)
	}
	stop = func() {
		if stopped.Swap(true) {
			return
		}
		if cancel != nil {
			cancel()
		}
	}
	cx.Cleanup(stop)
	return stop
}

// OnConflict subscribes resolve to cloud version conflicts. Notifications are delivered on the engine goroutine and the subscription is automatically removed when cx is disposed or stop is called.
func OnConflict(cx *core.Scope, resolve func(Conflict)) (stop func()) {
	requireScope(cx)
	if resolve == nil || cx.Disposed() {
		return func() {}
	}
	var stopped atomic.Bool
	deliver := func(c Conflict) {
		if stopped.Load() {
			return
		}
		cCopy := Conflict{
			Name:       c.Name,
			Local:      bytes.Clone(c.Local),
			Remote:     bytes.Clone(c.Remote),
			LocalTime:  c.LocalTime,
			RemoteTime: c.RemoteTime,
			Resolve:    c.Resolve,
		}
		cx.Post(func() {
			if !stopped.Load() {
				resolve(cCopy)
			}
		})
	}
	var cancel func()
	if store := cx.CloudStore(); store != nil {
		cancel = store.OnConflict(deliver)
	} else {
		cancel = platformOnConflict(deliver)
	}
	stop = func() {
		if stopped.Swap(true) {
			return
		}
		if cancel != nil {
			cancel()
		}
	}
	cx.Cleanup(stop)
	return stop
}
