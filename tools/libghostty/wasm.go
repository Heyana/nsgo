package main

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// wasmSource prepares the pinned source in an independent temporary tree. The pinned release disables graphics on freestanding targets. The browser has the C API's PNG/entropy effects, but no files: enable graphics and reject file-based image media. Native builds and the checkout remain unchanged.
func wasmSource(src string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "libghostty-wasm-source-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	fail := func(err error) (string, func(), error) { cleanup(); return "", nil, err }
	cmd := exec.Command("git", "-C", src, "archive", "--format=tar", commit)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return fail(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fail(err)
	}
	r := tar.NewReader(pipe)
	var extractErr error
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			extractErr = err
			break
		}
		if !filepath.IsLocal(h.Name) {
			extractErr = fmt.Errorf("non-local source path %q", h.Name)
			break
		}
		path := filepath.Join(dir, filepath.FromSlash(h.Name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			extractErr = err
			break
		}
		switch h.Typeflag {
		case tar.TypeXGlobalHeader, tar.TypeXHeader:
			continue
		case tar.TypeDir:
			extractErr = os.MkdirAll(path, 0o755)
		case tar.TypeReg:
			var f *os.File
			f, extractErr = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, os.FileMode(h.Mode)&0o777)
			if extractErr == nil {
				_, extractErr = io.Copy(f, r)
				if err := f.Close(); extractErr == nil {
					extractErr = err
				}
			}
		case tar.TypeSymlink:
			if !filepath.IsLocal(filepath.Join(filepath.Dir(h.Name), h.Linkname)) {
				extractErr = fmt.Errorf("non-local source link %q", h.Linkname)
			} else {
				extractErr = os.Symlink(h.Linkname, path)
			}
		default:
			extractErr = fmt.Errorf("unsupported source archive entry %q", h.Name)
		}
		if extractErr != nil {
			break
		}
	}
	if extractErr != nil {
		cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if extractErr != nil {
		return fail(extractErr)
	}
	if waitErr != nil {
		return fail(waitErr)
	}
	patches := []struct{ path, before, after string }{
		{"src/terminal/build_options.zig", "if (target.os.tag == .freestanding) return false;", "if (target.os.tag == .freestanding and !target.cpu.arch.isWasm()) return false;"},
		{"src/terminal/kitty/graphics_image.zig", "else => @compileError(\"readFile only supports file and temporary_file\"),\n        }", "else => @compileError(\"readFile only supports file and temporary_file\"),\n        }\n        if (comptime builtin.os.tag == .freestanding) return error.UnsupportedMedium;"},
	}
	for _, patch := range patches {
		path := filepath.Join(dir, patch.path)
		data, err := os.ReadFile(path)
		if err != nil {
			return fail(err)
		}
		if strings.Count(string(data), patch.before) != 1 {
			return fail(fmt.Errorf("pinned browser patch no longer applies to %s", patch.path))
		}
		if err := os.WriteFile(path, []byte(strings.Replace(string(data), patch.before, patch.after, 1)), 0o644); err != nil {
			return fail(err)
		}
	}
	return dir, cleanup, nil
}

// writeWasmLicense retains the notices of code linked into the browser module.
func writeWasmLicense(src, zig string) error {
	wuffs, err := filepath.Glob(filepath.Join(src, "zig-pkg", "*", "release", "c", "wuffs-v0.4.c"))
	if err != nil || len(wuffs) != 1 {
		return fmt.Errorf("locate pinned Wuffs license: %v (%d matches)", err, len(wuffs))
	}
	zigPath, err := exec.LookPath(zig)
	if err != nil {
		return err
	}
	zigPath, err = filepath.EvalSymlinks(zigPath)
	if err != nil {
		return err
	}
	zigNotice := ""
	for dir, depth := filepath.Dir(zigPath), 0; depth < 3; dir, depth = filepath.Dir(dir), depth+1 {
		path := filepath.Join(dir, "LICENSE")
		if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), "Zig contributors") {
			zigNotice = path
			break
		}
	}
	if zigNotice == "" {
		return fmt.Errorf("cannot locate the Zig distribution's LICENSE beside %s", zigPath)
	}
	files := []struct{ name, path string }{
		{"Ghostty", filepath.Join(src, "LICENSE")},
		{"Zig standard library", zigNotice},
		{"Wuffs", filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(wuffs[0]))), "LICENSE-MIT")},
	}
	var out strings.Builder
	for _, file := range files {
		data, err := os.ReadFile(file.path)
		if err != nil {
			return err
		}
		fmt.Fprintf(&out, "%s\n\n%s\n\n", file.name, strings.TrimSpace(string(data)))
	}
	return os.WriteFile(filepath.Join(filepath.Dir(libDir), "LICENSE"), []byte(strings.TrimSpace(out.String())+"\n"), 0o644)
}
