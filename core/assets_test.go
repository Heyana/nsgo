package core

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"time"
)

func TestAssetPath(t *testing.T) {
	for uri, want := range map[string]string{
		"asset:maps/level.bin":      "maps/level.bin",
		"asset:/data/file%20a.txt":  "data/file a.txt",
		"ASSET:/data/en.json?v=1#x": "data/en.json",
		"asset:data/literal%25.txt": "data/literal%.txt",
	} {
		if got, err := assetPath(uri); err != nil || got != want {
			t.Errorf("assetPath(%q) = %q, %v; want %q", uri, got, err, want)
		}
	}
	for _, uri := range []string{
		"", "asset:", "asset:/", "file:/data.txt", "asset://host/data.txt",
		"asset:../data.txt", "asset:/maps/%2e%2e/data.txt", "asset:maps//data.txt",
		"asset:C:%5Cdata.txt", "asset:folder/%00name", "asset:folder/%xx",
	} {
		if _, err := assetPath(uri); err == nil {
			t.Errorf("assetPath(%q) succeeded", uri)
		}
	}
}

func TestReadAssetPostsBytesAndMissingErrors(t *testing.T) {
	posts := make(chan func(), 1)
	release := make(chan struct{}, 1)
	root := testRoot(RootOptions{
		Post: func(fn func()) { posts <- fn },
		ReadAsset: func(ctx context.Context, path string) ([]byte, error) {
			if path != "book.txt" {
				return nil, fs.ErrNotExist
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-release:
				return []byte("the book"), nil
			}
		},
	})
	defer root.Dispose()
	wait := func() func() {
		t.Helper()
		select {
		case fn := <-posts:
			return fn
		case <-time.After(3 * time.Second):
			t.Fatal("the asset read did not post a result")
			return nil
		}
	}
	called := false
	root.ReadAsset("asset:book.txt", func(data []byte, err error) {
		called = true
		if string(data) != "the book" || err != nil {
			t.Errorf("book = %q, %v", data, err)
		}
	})
	if called {
		t.Fatal("the asset callback ran inline")
	}
	release <- struct{}{}
	wait()()
	if !called {
		t.Fatal("the asset callback did not run on the graph")
	}

	owner := root.Child()
	owner.ReadAsset("asset:missing", func([]byte, error) { t.Error("a disposed owner heard an asset result") })
	post := wait()
	owner.Dispose()
	post()

	root.ReadAsset("asset:missing", func(data []byte, err error) {
		if data != nil || !errors.Is(err, ErrAssetNotFound) {
			t.Errorf("missing asset = %q, %v", data, err)
		}
	})
	wait()()
}
