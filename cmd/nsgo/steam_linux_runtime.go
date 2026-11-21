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

// The notice and build script go into the bundle's licenses, so that each depot names the LGPL libraries' corresponding source and how it was built.
var (
	//go:embed steam-linux-runtime/NOTICE
	steamLinuxRuntimeNotice []byte
	//go:embed steam-linux-runtime/build.sh
	steamLinuxRuntimeBuild []byte
)

// steamLinuxLaunch starts the app with the bundle only where the system lacks GTK 4.14 or newer, as in Steam Linux Runtime 3.0 and 4.0. Elsewhere the bundle would break the app and what it starts: its GTK needs the runtime's libjpeg.so.62, and its GLib would replace a newer system one.
const steamLinuxLaunch = `#!/bin/sh
set -eu
dir=$(CDPATH='' cd -P -- "$(dirname -- "$0")" && pwd)
gtk=$(PATH="$PATH:/sbin:/usr/sbin" ldconfig -p 2>/dev/null | sed -n 's/^[[:space:]]*libgtk-4\.so\.1 (libc6,x86-64[^)]*) => //p' | sed -n 1p) || gtk=
version=
if [ -n "$gtk" ]; then
    version=$(readlink -f -- "$gtk" 2>/dev/null) || version=
fi
version=${version##*/libgtk-4.so.1.}
version=${version%%.*}
case $version in
'' | *[!0-9]*) version=0 ;;
esac
if [ "$version" -lt 1400 ]; then
    runtime="$dir/@RUNTIME@"
    export LD_LIBRARY_PATH="$runtime/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
    export XDG_DATA_DIRS="$runtime/share:${XDG_DATA_DIRS:-/usr/local/share:/usr/share}"
fi
exec "$dir/"@APP@ "$@"
`

func (p *project) bundlesSteamLinuxRuntime(target string) bool {
	return p.channel() == channelSteam && target == "linux_x64"
}

// addSteamLinuxRuntime adds the GTK libraries and a launch option that makes them available to the dynamic loader before the app starts, where the system has no GTK of its own.
func addSteamLinuxRuntime(p *project, dir string) error {
	runtimeDir := filepath.Join(dir, steamLinuxRuntimeDir)
	if err := os.Mkdir(runtimeDir, 0o755); err != nil {
		return fmt.Errorf("Steam runtime directory: %w", err)
	}
	if err := unpackSteamLinuxRuntime(runtimeDir); err != nil {
		return fmt.Errorf("unpack Steam runtime: %w", err)
	}
	for name, data := range map[string][]byte{"NOTICE": steamLinuxRuntimeNotice, "build.sh": steamLinuxRuntimeBuild} {
		if err := writeNewFile(filepath.Join(runtimeDir, "licenses", name), data, 0o644); err != nil {
			return err
		}
	}
	launch := strings.NewReplacer("@RUNTIME@", steamLinuxRuntimeDir, "@APP@", shQuote(p.Name)).Replace(steamLinuxLaunch)
	return writeNewFile(filepath.Join(dir, steamLinuxLauncher), []byte(launch), 0o755)
}

// writeNewFile writes a file that must not exist yet.
func writeNewFile(name string, data []byte, perm os.FileMode) error {
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
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
