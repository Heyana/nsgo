//go:build android || nsgoandroidhost

package android

import (
	"errors"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The Android Keystore is the runtime's: its keys never leave the system's keystore, so the Go process asks Java (Secrets.java) to encrypt, decrypt and delete: msgSecret asks, msgSecretReply answers. Java encrypts each value with AES-GCM under one key of the app's in AndroidKeyStore and keeps the ciphertext in the app's no-backup files, where neither other apps nor backups reach it. Android has no access groups or accessibility, which it ignores.

// The requests a msgSecret makes, as Secrets.java has them.
const (
	secretRead   = 0
	secretWrite  = 1
	secretDelete = 2
)

// secretWaits are the requests waiting for Java's answers, by number.
type secretWaits struct {
	mu      sync.Mutex
	last    uint32
	waiting map[uint32]chan secretAnswer
}

// secretAnswer is Java's answer: the value, whether there was one, and its failure, "" for none.
type secretAnswer struct {
	value []byte
	found bool
	err   string
}

// keystore is the surface's driver.SecretStore.
type keystore struct{ s *surface }

// askSecret sends a msgSecret of op for item, with value for a write, and waits for Java's answer.
func (k keystore) askSecret(op byte, item driver.SecretItem, value string) (secretAnswer, error) {
	w := &k.s.secrets
	ch := make(chan secretAnswer, 1)
	w.mu.Lock()
	w.last++
	n := w.last
	if w.waiting == nil {
		w.waiting = map[uint32]chan secretAnswer{}
	}
	w.waiting[n] = ch
	w.mu.Unlock()
	k.s.send(msgSecret, func(w *writer) {
		w.u32(n)
		w.byte(op)
		w.str(item.Service)
		w.str(item.Account)
		w.str(value)
	})
	a := <-ch
	if a.err != "" {
		return a, errors.New("android keystore: " + a.err)
	}
	return a, nil
}

// secretReply hands Java's answer to the request waiting for it.
func (s *surface) secretReply(r *reader) {
	n := r.u32()
	a := secretAnswer{err: r.str()}
	a.found = r.byte() != 0
	a.value = append([]byte(nil), r.bytes()...)
	w := &s.secrets
	w.mu.Lock()
	ch := w.waiting[n]
	delete(w.waiting, n)
	w.mu.Unlock()
	if ch != nil {
		ch <- a
	}
}

func (k keystore) ReadSecret(item driver.SecretItem) (string, bool, error) {
	a, err := k.askSecret(secretRead, item, "")
	if err != nil || !a.found {
		return "", false, err
	}
	return string(a.value), true, nil
}

func (k keystore) WriteSecret(item driver.SecretItem, value string) error {
	_, err := k.askSecret(secretWrite, item, value)
	return err
}

func (k keystore) DeleteSecret(item driver.SecretItem) error {
	_, err := k.askSecret(secretDelete, item, "")
	return err
}
