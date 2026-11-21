package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	steamLinuxRuntimeDir = "nsgo-gtk"
	steamLinuxLauncher   = "steam-launch.sh"
)

// Steam Linux Runtime 3 and 4 supply the display stack but lack GTK 4.
// This archive is built against the older runtime's SDK.
//
//go:embed steam-linux-runtime/amd64.tar.gz
var steamLinuxRuntime []byte

func (p *project) bundlesSteamLinuxRuntime(target string) bool {
	return p.channel() == channelSteam && target == "linux_x64"
}

// addSteamLinuxRuntime adds the GTK libraries and a launch option that
// makes them available to the dynamic loader before the app starts.
func addSteamLinuxRuntime(p *project, dir string) error {
	runtimeDir := filepath.Join(dir, steamLinuxRuntimeDir)
	if err := os.Mkdir(runtimeDir, 0o755); err != nil {
		return fmt.Errorf("Steam runtime directory: %w", err)
	}
	if err := unpackSteamLinuxRuntime(runtimeDir); err != nil {
		return fmt.Errorf("unpack Steam runtime: %w", err)
	}
	launcher := filepath.Join(dir, steamLinuxLauncher)
	file, err := os.OpenFile(launcher, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(file, `#!/bin/sh
set -eu
dir=$(CDPATH='' cd -P -- "$(dirname -- "$0")" && pwd)
runtime="$dir/%s"
export LD_LIBRARY_PATH="$runtime/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
export XDG_DATA_DIRS="$runtime/share:${XDG_DATA_DIRS:-/usr/local/share:/usr/share}"
exec "$dir/"%s "$@"
`, steamLinuxRuntimeDir, shQuote(p.Name))
	return errors.Join(err, file.Close())
}

func unpackSteamLinuxRuntime(dir string) error {
	stream, err := gzip.NewReader(bytes.NewReader(steamLinuxRuntime))
	if err != nil {
		return err
	}
	defer stream.Close()
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	archive := tar.NewReader(stream)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(header.Name, "/")
		if !fs.ValidPath(name) || name == "." {
			return fmt.Errorf("invalid Steam runtime path %q", name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				return err
			}
			_, err = io.Copy(file, archive)
			if err := errors.Join(err, file.Close()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected Steam runtime entry %q", name)
		}
	}
}
