package windowsinstaller

import (
	"archive/zip"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"
)

// Extract validates the entire archive before touching destination, then writes only the manifest's files. Root confines junctions and symlinks as well as ..; an unrelated file in a user-selected installation directory stays untouched.
func Extract(destination string, m Manifest, archive *zip.Reader, progress func(uint64, uint64)) error {
	if err := m.Validate(); err != nil {
		return err
	}
	expected := map[string]bool{}
	for _, file := range m.Files {
		expected[file] = true
	}
	expected["Uninstall.exe"] = true
	var total uint64
	for _, file := range archive.File {
		if !ValidPath(file.Name) || !file.Mode().IsRegular() || !expected[file.Name] {
			return fmt.Errorf("unexpected installer payload %q", file.Name)
		}
		delete(expected, file.Name)
		total += file.UncompressedSize64
	}
	if len(expected) != 0 {
		return fmt.Errorf("installer payload is missing %d files", len(expected))
	}
	if err := os.MkdirAll(destination, 0755); err != nil {
		return err
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	var copied uint64
	for _, file := range archive.File {
		if dir := path.Dir(file.Name); dir != "." {
			if err := root.MkdirAll(dir, 0755); err != nil {
				return err
			}
		}
		name := file.Name + ".nsgo-" + rand.Text()
		out, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		in, err := file.Open()
		if err == nil {
			_, err = io.Copy(out, in)
			closeErr := in.Close()
			if err == nil {
				err = closeErr
			}
		}
		closeErr := out.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = root.Rename(name, file.Name)
		}
		if err != nil {
			root.Remove(name)
			return err
		}
		copied += file.UncompressedSize64
		if progress != nil {
			progress(copied, total)
		}
	}
	return nil
}

// RemoveFiles removes manifest files and empty directories, never recursively. The uninstaller runs from a temporary copy, so its installed image can go too.
func RemoveFiles(destination string, m Manifest, keepUninstaller bool) error {
	if err := m.Validate(); err != nil {
		return err
	}
	root, err := os.OpenRoot(destination)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	var first error
	files := slices.Clone(m.Files)
	if !keepUninstaller {
		files = append(files, "Uninstall.exe")
	}
	for _, name := range files {
		if err := root.Remove(name); err != nil && !os.IsNotExist(err) && first == nil {
			first = err
		}
	}
	dirs := slices.Clone(m.Dirs)
	slices.SortFunc(dirs, func(a, b string) int { return strings.Count(b, "/") - strings.Count(a, "/") })
	for _, name := range dirs {
		_ = root.Remove(name)
	}
	if first != nil {
		return first
	}
	_ = os.Remove(destination) // succeeds only when the installation root is empty
	return nil
}
