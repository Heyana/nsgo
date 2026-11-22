package main

import (
	"context"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const nativeManifestName = "nsgo-natives.yaml"
const maxNativeLibrary = 1 << 30

type nativeManifest struct {
	Version   int                        `yaml:"version"`
	Libraries map[string][]nativeLibrary `yaml:"libraries"`
}

type nativeLibrary struct {
	Name     string   `yaml:"name"`
	URL      string   `yaml:"url"`
	Path     string   `yaml:"path"`
	SHA256   string   `yaml:"sha256"`
	Licenses []string `yaml:"licenses"`

	packageDir, owner string
}

// nativeTarget names the desktop slice whose packages the build lists.
func nativeTarget(env []string) string {
	system, arch := runtime.GOOS, runtime.GOARCH
	for _, setting := range append(os.Environ(), env...) {
		key, value, _ := strings.Cut(setting, "=")
		switch key {
		case "GOOS":
			system = value
		case "GOARCH":
			arch = value
		}
	}
	if system == "darwin" {
		system = "macos"
	}
	if arch == "amd64" {
		arch = "x64"
	}
	if system != "macos" && system != "windows" && system != "linux" {
		return ""
	}
	return system + "_" + arch
}

func readNativeManifest(dir string) (*nativeManifest, error) {
	path, err := packageNativePath(dir, nativeManifestName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > 1<<20 {
		return nil, fmt.Errorf("native manifest exceeds 1 MiB")
	}
	decoder := yaml.NewDecoder(io.LimitReader(f, 1<<20))
	decoder.KnownFields(true)
	var manifest nativeManifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("one YAML document is required")
	}
	if manifest.Version != 1 || len(manifest.Libraries) == 0 {
		return nil, fmt.Errorf("version: 1 and a nonempty libraries target map are required")
	}
	for _, target := range slices.Sorted(maps.Keys(manifest.Libraries)) {
		libraries := manifest.Libraries[target]
		if !slices.Contains([]string{"macos_arm64", "macos_x64", "windows_arm64", "windows_x64", "linux_arm64", "linux_x64"}, target) {
			return nil, fmt.Errorf("unsupported native-library target %q; declare individual desktop architectures", target)
		}
		for i := range libraries {
			library := &libraries[i]
			library.SHA256 = strings.ToLower(library.SHA256)
			if err := library.validate(target); err != nil {
				return nil, fmt.Errorf("%s library %d: %w", target, i+1, err)
			}
		}
	}
	return &manifest, nil
}

func (library nativeLibrary) validate(target string) error {
	name := library.Name
	if name == "" || name == "." || name == ".." || strings.TrimSpace(name) != name ||
		strings.ContainsAny(name, "/\\:*?\"<>|\x00\r\n") || strings.HasSuffix(name, ".") {
		return fmt.Errorf("name %q must be a portable file name", name)
	}
	for _, c := range name {
		if c < 32 || c == 127 {
			return fmt.Errorf("native file name contains a control character")
		}
	}
	system, _, _ := strings.Cut(target, "_")
	valid := system == "macos" && strings.HasSuffix(name, ".dylib") ||
		system == "windows" && strings.HasSuffix(strings.ToLower(name), ".dll") ||
		system == "linux" && (strings.HasSuffix(name, ".so") || strings.Contains(name, ".so."))
	base, _, _ := strings.Cut(strings.ToUpper(name), ".")
	if system == "windows" && (slices.Contains([]string{"CON", "PRN", "AUX", "NUL"}, base) || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
		return fmt.Errorf("%q is a reserved Windows file name", name)
	}
	if !valid {
		return fmt.Errorf("%q is not a shared-library name for %s", name, target)
	}
	if (library.URL == "") == (library.Path == "") {
		return fmt.Errorf("%s needs exactly one url or path", name)
	}
	if library.URL != "" {
		u, err := url.Parse(library.URL)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.Scheme != "https" && u.Scheme != "http" {
			return fmt.Errorf("%s needs an HTTP(S) URL without user credentials or a fragment", name)
		}
	} else if !fs.ValidPath(library.Path) || strings.Contains(library.Path, "\\") {
		return fmt.Errorf("%s path must stay in its Go package directory", name)
	}
	if sum, err := hex.DecodeString(library.SHA256); err != nil || len(sum) != sha256.Size {
		return fmt.Errorf("%s needs its 64-digit SHA-256", name)
	}
	if len(library.Licenses) == 0 {
		return fmt.Errorf("%s needs license file paths", name)
	}
	for _, license := range library.Licenses {
		if !fs.ValidPath(license) || strings.Contains(license, "\\") {
			return fmt.Errorf("%s license paths must stay in its Go package directory", name)
		}
	}
	return nil
}

// packageNativePath follows links only within the declaring package.
func packageNativePath(root, name string) (string, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%q escapes its Go package directory", name)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%q must name a regular file", name)
	}
	return path, nil
}

// addPackageNatives runs for precisely the packages go list selected, with the same target, tags and overlay as the executable. License policy stays per module.
func (p *project) addPackageNatives(pkg licensePackage, target string, sources map[string]*licenseSource) error {
	if target == "" || pkg.Dir == "" {
		return nil
	}
	manifest, err := readNativeManifest(pkg.Dir)
	if err != nil {
		return fmt.Errorf("%s/%s: %w", pkg.ImportPath, nativeManifestName, err)
	}
	if manifest == nil {
		return nil
	}
	libraries, declared := manifest.Libraries[target]
	if !declared {
		return fmt.Errorf("%s/%s has no %s entry; declare [] if it needs no native libraries", pkg.ImportPath, nativeManifestName, target)
	}
	for _, library := range libraries {
		library.packageDir, library.owner = pkg.Dir, pkg.ImportPath
		for _, previous := range p.nativeLibraries[target] {
			if strings.EqualFold(previous.Name, library.Name) && (previous.SHA256 != library.SHA256 || previous.Name != library.Name) {
				return fmt.Errorf("native library %s conflicts between %s and %s", library.Name, previous.owner, library.owner)
			}
		}
		if !slices.ContainsFunc(p.nativeLibraries[target], func(n nativeLibrary) bool { return n.owner == library.owner && n.Name == library.Name }) {
			p.nativeLibraries[target] = append(p.nativeLibraries[target], library)
		}
		key := "native:" + pkg.ImportPath + "/" + library.Name
		source := sources[key]
		if source == nil {
			root, err := filepath.EvalSymlinks(pkg.Dir)
			if err != nil {
				return err
			}
			label := moduleLabel(pkg.Module)
			if replacement := pkg.Module.Replace; replacement != nil {
				if replacement.Version == "" {
					label += " (local replacement)"
				} else {
					label += " => " + moduleLabel(replacement)
				}
			}
			source = &licenseSource{name: pkg.Module.Path, label: label + " — native library " + library.Name + " (" + pkg.ImportPath + ")",
				root: root, files: map[string]bool{}}
			sources[key] = source
		}
		for _, license := range library.Licenses {
			path, err := packageNativePath(pkg.Dir, license)
			if err != nil {
				return fmt.Errorf("%s license %s: %w", library.owner, license, err)
			}
			primary, recognizedName := isLicenseDocument(filepath.Base(license))
			source.files[path] = primary || !recognizedName
		}
	}
	return nil
}

func (library nativeLibrary) source(ctx context.Context) (string, error) {
	if library.Path != "" {
		return packageNativePath(library.packageDir, library.Path)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return library.fetch(ctx, filepath.Join(cache, "nsgo", "natives"))
}

func (library nativeLibrary) fetch(ctx context.Context, dir string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, library.SHA256)
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	for {
		held, err := tryLockFile(lock)
		if err != nil {
			return "", err
		}
		if held {
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	defer unlockFile(lock)
	if err := verifyNativeChecksum(path, library.SHA256); err == nil {
		return path, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, library.URL, nil)
	if err != nil {
		return "", err
	}
	stage(ctx, "downloading native library %s", library.Name)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download native library %s: %w", library.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download native library %s: HTTP %d", library.Name, resp.StatusCode)
	}
	tmp, err := os.CreateTemp(dir, ".native-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, io.LimitReader(resp.Body, maxNativeLibrary+1)); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := verifyNativeChecksum(tmp.Name(), library.SHA256); err != nil {
		return "", fmt.Errorf("%s: %w", library.Name, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}

func verifyNativeChecksum(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxNativeLibrary {
		return fmt.Errorf("native library must be a regular file of at most 1 GiB")
	}
	sum := sha256.New()
	size, err := io.Copy(sum, io.LimitReader(f, maxNativeLibrary+1))
	if err != nil {
		return err
	}
	if size > maxNativeLibrary {
		return fmt.Errorf("native library exceeds 1 GiB")
	}
	if hex.EncodeToString(sum.Sum(nil)) != expected {
		return fmt.Errorf("native library SHA-256 mismatch")
	}
	return nil
}

// validateNativeBinary rejects a library for another OS or architecture before signing or making an installer. A universal Mach-O may supply either slice.
func validateNativeBinary(path, target string) error {
	system, arch, _ := strings.Cut(target, "_")
	switch system {
	case "macos":
		cpu := macho.CpuAmd64
		if arch == "arm64" {
			cpu = macho.CpuArm64
		}
		if fat, err := macho.OpenFat(path); err == nil {
			defer fat.Close()
			for _, a := range fat.Arches {
				if a.Cpu == cpu && a.Magic == macho.Magic64 && a.Type == macho.TypeDylib {
					return nil
				}
			}
		} else if f, err := macho.Open(path); err == nil {
			defer f.Close()
			if f.Cpu == cpu && f.Magic == macho.Magic64 && f.Type == macho.TypeDylib {
				return nil
			}
		}
	case "windows":
		machine := uint16(pe.IMAGE_FILE_MACHINE_AMD64)
		if arch == "arm64" {
			machine = pe.IMAGE_FILE_MACHINE_ARM64
		}
		if f, err := pe.Open(path); err == nil {
			defer f.Close()
			_, bits64 := f.OptionalHeader.(*pe.OptionalHeader64)
			if f.Machine == machine && bits64 && f.Characteristics&pe.IMAGE_FILE_DLL != 0 {
				return nil
			}
		}
	case "linux":
		machine := elf.EM_X86_64
		if arch == "arm64" {
			machine = elf.EM_AARCH64
		}
		if f, err := elf.Open(path); err == nil {
			defer f.Close()
			if f.Machine == machine && f.Class == elf.ELFCLASS64 && f.Data == elf.ELFDATA2LSB && (f.OSABI == elf.ELFOSABI_NONE || f.OSABI == elf.ELFOSABI_LINUX) && f.Type == elf.ET_DYN && !slices.ContainsFunc(f.Progs, func(p *elf.Prog) bool { return p.Type == elf.PT_INTERP }) {
				return nil
			}
		}
	}
	return fmt.Errorf("not a shared library for %s", target)
}

func (p *project) addNativeLibraries(ctx context.Context, target, dir string) error {
	done := map[string]string{}
	for _, library := range p.nativeLibraries[target] {
		if err := ctx.Err(); err != nil {
			return err
		}
		if previous := done[library.Name]; previous != "" {
			if previous != library.SHA256 {
				return fmt.Errorf("conflicting native library %s", library.Name)
			}
			continue
		}
		done[library.Name] = library.SHA256
		source, err := library.source(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", library.owner, err)
		}
		if err := verifyNativeChecksum(source, library.SHA256); err != nil {
			return fmt.Errorf("%s/%s: %w", library.owner, library.Name, err)
		}
		if err := validateNativeBinary(source, target); err != nil {
			return fmt.Errorf("native library %s from %s: %w", library.Name, library.owner, err)
		}
		dst := filepath.Join(dir, library.Name)
		if info, err := os.Lstat(dst); err == nil {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("native library %s collides with a non-regular packaged file", library.Name)
			}
			if err := verifyNativeChecksum(dst, library.SHA256); err != nil {
				return fmt.Errorf("native library %s collides with another packaged file", library.Name)
			}
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := copyNativeLibrary(source, dst, library.SHA256); err != nil {
			return err
		}
		if strings.HasPrefix(target, "macos_") {
			if fat, err := macho.OpenFat(dst); err == nil {
				multiple := len(fat.Arches) > 1
				fat.Close()
				if multiple {
					arch := "arm64"
					if target == "macos_x64" {
						arch = "x86_64"
					}
					if err := run(ctx, "lipo", "-thin", arch, dst, "-output", dst); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// copyNativeLibrary hashes exactly the bytes copied, committing a complete file only after its checksum matches. Large media libraries need no whole-file buffer.
func copyNativeLibrary(src, dst, expected string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(dst), ".native-*")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, sum), io.LimitReader(in, maxNativeLibrary+1))
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if n > maxNativeLibrary || hex.EncodeToString(sum.Sum(nil)) != expected {
		return fmt.Errorf("native library SHA-256 mismatch or size exceeds 1 GiB")
	}
	if err := os.Chmod(out.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(out.Name(), dst)
}

// matchMacLibraries prevents a universal package from retaining an arm-only library or silently dropping one supplied only for Intel.
func matchMacLibraries(arm, intel string) error {
	list := func(app string) ([]string, error) {
		paths, err := filepath.Glob(filepath.Join(app, "Contents", "Frameworks", "*.dylib"))
		for i := range paths {
			paths[i] = filepath.Base(paths[i])
		}
		return paths, err
	}
	a, err := list(arm)
	if err != nil {
		return err
	}
	b, err := list(intel)
	if err != nil {
		return err
	}
	if !slices.Equal(a, b) {
		return fmt.Errorf("universal macOS libraries differ: arm64 %v, x64 %v", a, b)
	}
	return nil
}
