package shareinbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestCommittedSharesSurviveRestart(t *testing.T) {
	group, cache := t.TempDir(), t.TempDir()
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("from another app"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := Begin(group)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Abort()
	if err := b.Text("https://example.com", "text/plain"); err != nil {
		t.Fatal(err)
	}
	if err := b.File(path, "note.txt", "text/plain"); err != nil {
		t.Fatal(err)
	}
	var got [][]driver.SharedItem
	deliver := func(items []driver.SharedItem, ack func() error) {
		got = append(got, items)
		if err := ack(); err != nil {
			t.Error(err)
		}
	}
	reader := NewReader(group, cache)
	if err := reader.Drain(deliver); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatal("unfinished batch was delivered")
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
	// Discard the writer and its original file, as when the extension exits.
	b = nil
	os.Remove(path)
	if err := reader.Drain(deliver); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("batches: %#v", got)
	}
	if got[0][0].Text != "https://example.com" || got[0][1].Name != "note.txt" {
		t.Fatalf("items: %#v", got[0])
	}
	data, err := os.ReadFile(got[0][1].Path)
	if err != nil || string(data) != "from another app" {
		t.Fatalf("shared file: %q, %v", data, err)
	}
	if err := reader.Drain(deliver); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatal("batch delivered twice")
	}
}

// An invalid batch is set aside without reading outside the inbox, so the next activation neither delivers it nor fails on it again, and it goes after a day.
func TestInvalidBatchIsSetAsideWithoutReadingOutsideInbox(t *testing.T) {
	for _, name := range []string{"../secret", "file-000"} {
		t.Run(name, func(t *testing.T) {
			group, cache := t.TempDir(), t.TempDir()
			dir := filepath.Join(group, Directory, "bad")
			os.MkdirAll(dir, 0o700)
			secret := filepath.Join(t.TempDir(), "secret")
			os.WriteFile(secret, []byte("private"), 0o600)
			os.Symlink(secret, filepath.Join(dir, "file-000"))
			os.WriteFile(filepath.Join(dir, "items.json"), []byte(`[{"Path":"`+name+`"}]`), 0o600)
			called := false
			if err := NewReader(group, cache).Drain(func([]driver.SharedItem, func() error) { called = true }); err == nil || called {
				t.Fatalf("drain: %v, delivered %v", err, called)
			}
			aside := filepath.Join(group, Directory, rejected+"bad")
			if _, err := os.Stat(aside); err != nil {
				t.Fatalf("failed batch was not set aside: %v", err)
			}
			files, _ := os.ReadDir(cache)
			if len(files) != 0 {
				t.Fatal("failed import left cache files")
			}
			if err := NewReader(group, cache).Drain(func([]driver.SharedItem, func() error) { called = true }); err != nil || called {
				t.Fatalf("second drain: %v, delivered %v", err, called)
			}
			old := time.Now().Add(-25 * time.Hour)
			os.Chtimes(aside, old, old)
			if err := NewReader(group, cache).Drain(func([]driver.SharedItem, func() error) {}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(aside); !os.IsNotExist(err) {
				t.Fatalf("set-aside batch survived a day: %v", err)
			}
		})
	}
}

func TestShareRemainsDurableUntilHandlerAcknowledges(t *testing.T) {
	group, cache := t.TempDir(), t.TempDir()
	batch, err := Begin(group)
	if err != nil {
		t.Fatal(err)
	}
	defer batch.Abort()
	if err := batch.Text("pending", "text/plain"); err != nil {
		t.Fatal(err)
	}
	if err := batch.Commit(); err != nil {
		t.Fatal(err)
	}
	reader := NewReader(group, cache)
	count := 0
	if err := reader.Drain(func([]driver.SharedItem, func() error) { count++ }); err != nil {
		t.Fatal(err)
	}
	if err := reader.Drain(func([]driver.SharedItem, func() error) { count++ }); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("pending batch delivered %d times", count)
	}
	// The process exits with no handler, so its in-memory claim disappears.
	restarted := NewReader(group, cache)
	if err := restarted.Drain(func(items []driver.SharedItem, ack func() error) {
		count++
		if len(items) != 1 || items[0].Text != "pending" {
			t.Fatal(items)
		}
		if err := ack(); err != nil {
			t.Fatal(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("unacknowledged batch lost on restart")
	}
	if err := NewReader(group, cache).Drain(func([]driver.SharedItem, func() error) { t.Fatal("acknowledged batch survived") }); err != nil {
		t.Fatal(err)
	}
}

func TestAbandonedImportIsRemovedAfterADay(t *testing.T) {
	group := t.TempDir()
	stale, err := Begin(group)
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Abort()
	recent, err := Begin(group)
	if err != nil {
		t.Fatal(err)
	}
	defer recent.Abort()
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stale.dir, old, old); err != nil {
		t.Fatal(err)
	}
	if err := NewReader(group, t.TempDir()).Drain(func([]driver.SharedItem, func() error) { t.Fatal("partial import delivered") }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale.dir); !os.IsNotExist(err) {
		t.Fatal("stale partial import remains")
	}
	if _, err := os.Stat(recent.dir); err != nil {
		t.Fatal("active import was removed")
	}
}

func TestShareLimitsRejectOversizedTextAndExtraItems(t *testing.T) {
	batch, err := Begin(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer batch.Abort()
	if err := batch.Text(strings.Repeat("x", MaxTextBytes+1), "text/plain"); err == nil {
		t.Fatal("oversized text was accepted")
	}
	for range maxItems {
		if err := batch.Text("item", "text/plain"); err != nil {
			t.Fatal(err)
		}
	}
	if err := batch.Text("one too many", "text/plain"); err == nil {
		t.Fatal("extra item was accepted")
	}
	if err := batch.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestOversizedManifestNeverBecomesVisible(t *testing.T) {
	group := t.TempDir()
	batch, err := Begin(group)
	if err != nil {
		t.Fatal(err)
	}
	defer batch.Abort()
	for range 2 {
		if err := batch.Text(strings.Repeat("x", MaxTextBytes), "text/plain"); err != nil {
			t.Fatal(err)
		}
	}
	if err := batch.Commit(); err == nil {
		t.Fatal("oversized manifest was committed")
	}
	if err := NewReader(group, t.TempDir()).Drain(func([]driver.SharedItem, func() error) { t.Fatal("invalid manifest was delivered") }); err != nil {
		t.Fatal(err)
	}
}
