package main

import (
	"bytes"
	"context"
	"debug/elf"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// appImageUpdate selects the external updater's locator independently of NSGo's signed appcast. A configured empty value leaves discovery disabled.
func (p *project) appImageUpdate(ctx context.Context, arch string) (string, error) {
	info := ""
	if p.Linux.AppImageUpdate != nil {
		info = *p.Linux.AppImageUpdate
	} else if repo := p.githubRepository(ctx); repo != "" {
		owner, name, _ := strings.Cut(repo, "/")
		pattern := p.packageSlug() + "-*-linux-" + goArchName(arch) + ".AppImage.zsync"
		info = strings.Join([]string{"gh-releases-zsync", owner, name, "latest", pattern}, "|")
	}
	if err := validateAppImageUpdate(info); err != nil {
		return "", fmt.Errorf("AppImage update information: %w", err)
	}
	return info, nil
}

func validateAppImageUpdate(info string) error {
	if info == "" {
		return nil
	}
	if strings.ContainsAny(info, "\x00\r\n") {
		return errors.New("contains a NUL or line break")
	}
	parts := strings.Split(info, "|")
	switch parts[0] {
	case "zsync":
		if len(parts) == 2 {
			u, err := url.Parse(parts[1])
			if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.Fragment == "" {
				return nil
			}
		}
		return errors.New("use zsync| followed by an absolute HTTP or HTTPS control-file URL without a fragment")
	case "gh-releases-zsync":
		if len(parts) == 5 && parts[1] != "" && parts[2] != "" && parts[3] != "" && parts[4] != "" && !strings.ContainsAny(parts[1]+parts[2], "/\\") {
			return nil
		}
		return errors.New("use gh-releases-zsync|owner|repository|tag|filename-pattern")
	default:
		return errors.New("transport must be zsync or gh-releases-zsync")
	}
}

// setAppImageUpdate fills the runtime's reserved section in memory. The cached, SHA-256-pinned runtime and all ELF offsets remain unchanged.
// https://github.com/AppImage/AppImageSpec/blob/master/draft.md#update-information
func setAppImageUpdate(runtime []byte, info string) error {
	f, err := elf.NewFile(bytes.NewReader(runtime))
	if err != nil {
		return fmt.Errorf("AppImage runtime ELF: %w", err)
	}
	defer f.Close()
	section := f.Section(".upd_info")
	if section == nil || section.Type != elf.SHT_PROGBITS || section.Offset > uint64(len(runtime)) || section.Size > uint64(len(runtime))-section.Offset {
		return errors.New("AppImage runtime has no reserved .upd_info section")
	}
	if uint64(len(info)) >= section.Size {
		return fmt.Errorf("AppImage update information needs %d bytes; runtime .upd_info holds %d", len(info)+1, section.Size)
	}
	data := runtime[section.Offset : section.Offset+section.Size]
	clear(data)
	copy(data, info)
	return nil
}
