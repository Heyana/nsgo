//go:build android

package cloud

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type androidCommand struct {
	Op         string `json:"op"`
	ID         uint64 `json:"id"`
	Key        string `json:"key,omitempty"`
	Name       string `json:"name,omitempty"`
	Data       string `json:"data,omitempty"`
	ConflictID string `json:"conflictId,omitempty"`
}

type androidReport struct {
	Type         string   `json:"type"` // state, reply, change, conflict
	ID           uint64   `json:"id"`
	Available    bool     `json:"available"`
	Data         string   `json:"data"`
	Keys         []string `json:"keys"`
	Files        []string `json:"files"`
	Reason       string   `json:"reason"`
	Error        string   `json:"error"`
	Detail       string   `json:"detail"`
	ConflictID   string   `json:"conflictId"`
	Name         string   `json:"name"`
	Local        string   `json:"local"`
	Remote       string   `json:"remote"`
	LocalTimeMs  int64    `json:"localTimeMs"`
	RemoteTimeMs int64    `json:"remoteTimeMs"`
}

var androidCloud struct {
	once      sync.Once
	mu        sync.Mutex
	nextID    uint64
	available bool
	pending   map[uint64]chan androidReport
	watchers  map[uint64]func(ChangeEvent)
	conflicts map[uint64]func(Conflict)
}

func listenAndroidCloud() {
	androidCloud.once.Do(func() {
		androidCloud.mu.Lock()
		androidCloud.pending = map[uint64]chan androidReport{}
		androidCloud.watchers = map[uint64]func(ChangeEvent){}
		androidCloud.conflicts = map[uint64]func(Conflict){}
		androidCloud.mu.Unlock()
		driver.HearCloudStorage(hearAndroidCloud)
	})
}

func hearAndroidCloud(raw []byte) {
	var r androidReport
	if json.Unmarshal(raw, &r) != nil {
		return
	}
	switch r.Type {
	case "state":
		androidCloud.mu.Lock()
		androidCloud.available = r.Available
		androidCloud.mu.Unlock()
	case "reply":
		androidCloud.mu.Lock()
		ch := androidCloud.pending[r.ID]
		delete(androidCloud.pending, r.ID)
		androidCloud.mu.Unlock()
		if ch != nil {
			ch <- r
		}
	case "change":
		reason := ChangeServer
		switch r.Reason {
		case "initial_sync":
			reason = ChangeInitialSync
		case "quota_violation":
			reason = ChangeQuotaViolation
		case "account":
			reason = ChangeAccount
		}
		ev := ChangeEvent{
			Keys:   slices.Clone(r.Keys),
			Files:  slices.Clone(r.Files),
			Reason: reason,
		}
		androidCloud.mu.Lock()
		list := make([]func(ChangeEvent), 0, len(androidCloud.watchers))
		for _, fn := range androidCloud.watchers {
			list = append(list, fn)
		}
		androidCloud.mu.Unlock()
		for _, fn := range list {
			fn(ev)
		}
	case "conflict":
		localBytes, _ := base64.StdEncoding.DecodeString(r.Local)
		remoteBytes, _ := base64.StdEncoding.DecodeString(r.Remote)
		conflictID := r.ConflictID
		c := Conflict{
			Name:       r.Name,
			Local:      localBytes,
			Remote:     remoteBytes,
			LocalTime:  time.UnixMilli(r.LocalTimeMs).UTC(),
			RemoteTime: time.UnixMilli(r.RemoteTimeMs).UTC(),
			Resolve: func(chosen []byte) error {
				_, err := callAndroidCloud(androidCommand{
					Op:         "resolve",
					ConflictID: conflictID,
					Data:       base64.StdEncoding.EncodeToString(chosen),
				})
				return err
			},
		}
		androidCloud.mu.Lock()
		list := make([]func(Conflict), 0, len(androidCloud.conflicts))
		for _, fn := range androidCloud.conflicts {
			list = append(list, fn)
		}
		androidCloud.mu.Unlock()
		for _, fn := range list {
			fn(c)
		}
	}
}

func callAndroidCloud(cmd androidCommand) (androidReport, error) {
	listenAndroidCloud()
	androidCloud.mu.Lock()
	if !androidCloud.available || !driver.CloudStorageConnected() {
		androidCloud.mu.Unlock()
		return androidReport{}, ErrUnavailable
	}
	androidCloud.nextID++
	cmd.ID = androidCloud.nextID
	ch := make(chan androidReport, 1)
	androidCloud.pending[cmd.ID] = ch
	androidCloud.mu.Unlock()

	msg, err := json.Marshal(cmd)
	if err != nil {
		androidCloud.mu.Lock()
		delete(androidCloud.pending, cmd.ID)
		androidCloud.mu.Unlock()
		return androidReport{}, err
	}
	driver.SendCloudStorage(msg)
	r := <-ch
	if r.Error != "" {
		return r, androidCloudError(r)
	}
	return r, nil
}

func androidCloudError(r androidReport) error {
	switch r.Error {
	case "unavailable":
		return ErrUnavailable
	case "not_found":
		return ErrNotFound
	case "conflict":
		localBytes, _ := base64.StdEncoding.DecodeString(r.Local)
		remoteBytes, _ := base64.StdEncoding.DecodeString(r.Remote)
		return &ConflictError{
			Key:        r.Name,
			Local:      localBytes,
			Remote:     remoteBytes,
			LocalTime:  time.UnixMilli(r.LocalTimeMs).UTC(),
			RemoteTime: time.UnixMilli(r.RemoteTimeMs).UTC(),
		}
	default:
		if r.Detail != "" {
			return errors.New("cloud: " + r.Detail)
		}
		return errors.New("cloud: " + r.Error)
	}
}

func platformGet(key string) ([]byte, error) {
	r, err := callAndroidCloud(androidCommand{Op: "get", Key: key})
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(r.Data)
}

func platformSet(key string, value []byte) error {
	_, err := callAndroidCloud(androidCommand{
		Op:   "set",
		Key:  key,
		Data: base64.StdEncoding.EncodeToString(value),
	})
	return err
}

func platformDelete(key string) error {
	_, err := callAndroidCloud(androidCommand{Op: "delete", Key: key})
	return err
}

func platformKeys() ([]string, error) {
	r, err := callAndroidCloud(androidCommand{Op: "keys"})
	if err != nil {
		return nil, err
	}
	keys := slices.Clone(r.Keys)
	slices.Sort(keys)
	return keys, nil
}

func platformReadFile(name string) ([]byte, error) {
	r, err := callAndroidCloud(androidCommand{Op: "read_file", Name: name})
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(r.Data)
}

func platformWriteFile(name string, data []byte) error {
	_, err := callAndroidCloud(androidCommand{
		Op:   "write_file",
		Name: name,
		Data: base64.StdEncoding.EncodeToString(data),
	})
	return err
}

func platformDeleteFile(name string) error {
	_, err := callAndroidCloud(androidCommand{Op: "delete_file", Name: name})
	return err
}

func platformListFiles() ([]string, error) {
	r, err := callAndroidCloud(androidCommand{Op: "list_files"})
	if err != nil {
		return nil, err
	}
	files := slices.Clone(r.Files)
	slices.Sort(files)
	return files, nil
}

func platformWatchChanges(onChange func(ChangeEvent)) func() {
	listenAndroidCloud()
	androidCloud.mu.Lock()
	androidCloud.nextID++
	id := androidCloud.nextID
	androidCloud.watchers[id] = onChange
	androidCloud.mu.Unlock()
	return func() {
		androidCloud.mu.Lock()
		delete(androidCloud.watchers, id)
		androidCloud.mu.Unlock()
	}
}

func platformOnConflict(resolve func(Conflict)) func() {
	listenAndroidCloud()
	androidCloud.mu.Lock()
	androidCloud.nextID++
	id := androidCloud.nextID
	androidCloud.conflicts[id] = resolve
	androidCloud.mu.Unlock()
	return func() {
		androidCloud.mu.Lock()
		delete(androidCloud.conflicts, id)
		androidCloud.mu.Unlock()
	}
}
