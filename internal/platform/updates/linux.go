package updates

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// LinuxPackageKind identifies a self-updatable Linux package format.
type LinuxPackageKind uint8

const (
	LinuxPackageNone LinuxPackageKind = iota
	LinuxPackageDeb
	LinuxPackageArch
	LinuxPackageAppImage
)

// dpkgPattern escapes what dpkg-query --search reads as wildcards in a path.
var dpkgPattern = strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`)

// renameFile is replaced in tests to exercise rollback on rename failure.
var renameFile = os.Rename

// LinuxPackageFormat classifies a package file path or download URL by suffix.
func LinuxPackageFormat(nameOrURL string) LinuxPackageKind {
	name := nameOrURL
	if u, err := url.Parse(nameOrURL); err == nil && u.Scheme != "" {
		name = path.Base(u.Path)
	} else {
		name = filepath.Base(nameOrURL)
	}
	switch {
	case strings.HasSuffix(name, ".deb"):
		return LinuxPackageDeb
	case strings.HasSuffix(name, ".pkg.tar.zst"), strings.HasSuffix(name, ".pkg.tar.gz"):
		return LinuxPackageArch
	case strings.HasSuffix(name, ".AppImage"):
		return LinuxPackageAppImage
	default:
		return LinuxPackageNone
	}
}

// LinuxInstallCommand returns the privileged package-manager command for a
// Debian or Arch package staged at pkgPath.
func LinuxInstallCommand(pkgPath string) ([]string, error) {
	switch LinuxPackageFormat(pkgPath) {
	case LinuxPackageDeb:
		return []string{"pkexec", "dpkg", "--install", pkgPath}, nil
	case LinuxPackageArch:
		return []string{"pkexec", "pacman", "--upgrade", "--noconfirm", pkgPath}, nil
	default:
		return nil, fmt.Errorf("updater: unsupported Linux package %q", filepath.Base(pkgPath))
	}
}

// LinuxOwnershipCommand returns the package-manager query command that checks
// whether the package manager matching download owns exe.
func LinuxOwnershipCommand(download, exe string) ([]string, bool) {
	switch LinuxPackageFormat(download) {
	case LinuxPackageDeb:
		return []string{"dpkg-query", "--search", dpkgPattern.Replace(exe)}, true
	case LinuxPackageArch:
		return []string{"pacman", "--query", "--owns", exe}, true
	default:
		return nil, false
	}
}

// LinuxSelfUpdatable reports whether download can replace the running Linux
// app: an AppImage when appImage is non-empty, or a Debian/Arch package when
// its package manager owns exe and the app is not running from an AppImage.
func LinuxSelfUpdatable(ctx context.Context, exe, appImage, download string) bool {
	switch LinuxPackageFormat(download) {
	case LinuxPackageAppImage:
		return appImage != ""
	case LinuxPackageDeb, LinuxPackageArch:
		if appImage != "" || exe == "" {
			return false
		}
		args, ok := LinuxOwnershipCommand(download, exe)
		if !ok {
			return false
		}
		return exec.CommandContext(ctx, args[0], args[1:]...).Run() == nil
	default:
		return false
	}
}

// ReplaceAppImage atomically replaces the running AppImage at targetPath with
// the verified staged file at stagedPath, preserving permissions with
// executable bits set. If any step fails, the temporary file is removed and
// targetPath stays unchanged.
func ReplaceAppImage(stagedPath, targetPath string) error {
	if targetPath == "" {
		return errors.New("updater: app is not running from an AppImage")
	}
	realTarget, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		return fmt.Errorf("updater: resolve AppImage target: %w", err)
	}
	info, err := os.Stat(realTarget)
	if err != nil {
		return fmt.Errorf("updater: stat AppImage target: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("updater: AppImage target %q is not a regular file", realTarget)
	}
	mode := appImageMode(info.Mode().Perm())

	src, err := os.Open(stagedPath)
	if err != nil {
		return fmt.Errorf("updater: open staged AppImage: %w", err)
	}
	defer src.Close()

	dir := filepath.Dir(realTarget)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(realTarget)+".update-*")
	if err != nil {
		return fmt.Errorf("updater: stage AppImage replacement: %w", err)
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		if !keep {
			os.Remove(tmpName)
		}
	}()

	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return fmt.Errorf("updater: copy AppImage replacement: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("updater: set AppImage permissions: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("updater: sync AppImage replacement: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("updater: close AppImage replacement: %w", err)
	}
	if err := renameFile(tmpName, realTarget); err != nil {
		return fmt.Errorf("updater: replace AppImage: %w", err)
	}
	keep = true
	return nil
}

// appImageMode keeps existing permissions while ensuring the AppImage is
// readable/writable/executable by its owner and executable wherever readable.
func appImageMode(perm fs.FileMode) fs.FileMode {
	perm |= 0o700
	if perm&0o040 != 0 {
		perm |= 0o010
	}
	if perm&0o004 != 0 {
		perm |= 0o001
	}
	return perm
}
