package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
)

const offlineWorkerName = "nsgo-sw.js"

//go:embed web/offline.js
var offlineWorker []byte

type offlineFile struct {
	URL       string `json:"url"`
	Integrity string `json:"integrity"`
}

// writeOfflineWorker names a cache by its complete contents and the worker
// code. The worker verifies each response before making that cache available.
func writeOfflineWorker(dir string, names []string) error {
	slices.Sort(names)
	names = slices.Compact(names)
	files := make([]offlineFile, 0, len(names))
	for _, name := range names {
		f, err := os.Open(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, err = io.Copy(hash, f)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return fmt.Errorf("hashing web file %s: %w", name, err)
		}
		u := url.URL{Path: "./" + name}
		files = append(files, offlineFile{
			URL:       u.EscapedPath(),
			Integrity: "sha256-" + base64.StdEncoding.EncodeToString(hash.Sum(nil)),
		})
	}
	data, err := json.Marshal(files)
	if err != nil {
		return err
	}
	hash := sha256.New()
	hash.Write(data)
	hash.Write(offlineWorker)
	source := fmt.Appendf(nil, "// Written by nsgo.\nconst version = %q;\nconst files = %s;\n", hex.EncodeToString(hash.Sum(nil)), data)
	source = append(source, offlineWorker...)
	return os.WriteFile(filepath.Join(dir, offlineWorkerName), source, 0o644)
}
