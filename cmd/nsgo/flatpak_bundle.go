package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/flatpak"
)

// buildFlatpakBundle writes the already compiled app as a self-contained OSTree bundle. The separate archive/manifest remains available to flatpak-builder.
func buildFlatpakBundle(ctx context.Context, p *project, dir, arch string) (string, error) {
	stage(ctx, "writing the Flatpak bundle")
	entries, err := linuxDirEntries(p, dir)
	if err != nil {
		return "", err
	}
	files := []flatpak.Entry{{Name: "export", Dir: true, Mode: 0o755}}
	for _, e := range entries {
		files = append(files, flatpak.Entry{Name: "files/lib/" + p.packageSlug() + "/" + e.name, Data: e.data, Mode: uint32(e.mode), Link: e.link, Dir: e.dir})
		var dest string
		switch e.name {
		case p.ID + ".desktop":
			dest = "share/applications/" + p.ID + ".desktop"
		case p.ID + ".png":
			dest = "share/icons/hicolor/512x512/apps/" + p.ID + ".png"
		case p.ID + ".metainfo.xml":
			dest = "share/metainfo/" + p.ID + ".metainfo.xml"
		case p.ID + ".mime.xml":
			dest = "share/mime/packages/" + p.ID + ".xml"
		}
		if dest != "" {
			exported := e.data
			if e.name == p.ID+".desktop" {
				exported = flatpakExportedDesktop(e.data, p.ID, flatpakArch[goArchName(arch)], p.Name)
			}
			files = append(files, flatpak.Entry{Name: "files/" + dest, Data: e.data, Mode: 0o644})
			files = append(files, flatpak.Entry{Name: "export/" + dest, Data: exported, Mode: 0o644})
		}
	}
	files = append(files, flatpak.Entry{Name: "files/bin/" + p.Name, Link: "../lib/" + p.packageSlug() + "/" + p.Name, Mode: 0o777})
	runtime := p.Linux.FlatpakRuntime
	if runtime == "" {
		runtime = flatpakRuntime
	}
	// Keyfile lists are semicolon-terminated. The bundle grants the same display/audio/network permissions as the generated builder manifest.
	metadata := fmt.Sprintf("[Application]\nname=%s\nruntime=org.gnome.Platform/%s/%s\nsdk=org.gnome.Sdk/%s/%s\ncommand=%s\n\n[Context]\nshared=network;ipc;\nsockets=wayland;fallback-x11;pulseaudio;\ndevices=dri;\n", flatpakKeyfileValue(p.ID), flatpakArch[goArchName(arch)], flatpakKeyfileValue(runtime), flatpakArch[goArchName(arch)], flatpakKeyfileValue(runtime), flatpakKeyfileValue(p.Name))
	if strings.ContainsAny(p.ID+runtime+p.Name, "\x00\r\n") {
		return "", fmt.Errorf("Flatpak metadata contains a line break")
	}
	data, err := flatpak.Write(flatpak.Bundle{ID: p.ID, Arch: flatpakArch[goArchName(arch)], Branch: "stable", Metadata: metadata, Timestamp: uint64(time.Now().Unix())}, files)
	if err != nil {
		return "", err
	}
	file := filepath.Join(dir, p.releaseName("linux_"+goArchName(arch), ".flatpak"))
	return file, os.WriteFile(file, data, 0o644)
}

func flatpakKeyfileValue(s string) string {
	return strings.NewReplacer(`\`, `\\`, " ", `\s`, "\t", `\t`).Replace(s)
}

// flatpakExportedDesktop points the exported desktop entry's Exec at flatpak run, as flatpak build-finish rewrites it: the host has no command of the app's own, so the entry as written would launch nothing from the desktop's menus. Files and links the entry takes are forwarded into the sandbox.
func flatpakExportedDesktop(data []byte, id, arch, command string) []byte {
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	for i, line := range lines {
		rest, ok := strings.CutPrefix(line, "Exec=")
		if !ok {
			continue
		}
		args := " " + id
		switch {
		case strings.HasSuffix(rest, " %U"):
			args = " --file-forwarding " + id + " @@u %U @@"
		case strings.HasSuffix(rest, " %F"):
			args = " --file-forwarding " + id + " @@ %F @@"
		}
		quoted := desktopString(`"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`, `%`, `%%`).Replace("--command="+command) + `"`)
		lines[i] = "Exec=/usr/bin/flatpak run --branch=stable --arch=" + arch + " " + quoted + args
	}
	return []byte(strings.Join(append(lines, "X-Flatpak="+id), "\n") + "\n")
}
