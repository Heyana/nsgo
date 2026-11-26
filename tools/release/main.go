// Command release builds the prebuilt nsgo binaries a release attaches: an archive per platform and checksums.txt over them, from the checked-out commit, which must carry the tag vX.Y.Z so that Go stamps that version into each binary.
//
//	go run ./tools/release -out DIR X.Y.Z
//
// It cross-compiles ./cmd/nsgo with CGO_ENABLED=0 and -trimpath for every platform in targets, packs each binary with LICENSE and README.md into nsgo_<version>_<os>_<arch>.tar.gz (.zip for Windows), writes checksums.txt in sha256sum's format, then reads everything back: the checksums, each archive's entries, and each binary's build information, which must name the release's version, its platform, CGO_ENABLED=0 and -trimpath. The binary for the running platform also runs nsgo version. release.yml runs it before it tags anything, and release-assets.yml runs it on pull requests that change it, so a broken build fails there rather than in a release. The asset names and layout are a contract the install script and the Homebrew tap read.
package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
)

const (
	module    = "github.com/nsgo-dev/nsgo"
	command   = module + "/cmd/nsgo"
	checksums = "checksums.txt"
)

// targets are the platforms a release ships a binary for, each GOOS/GOARCH.
var targets = []target{
	{"darwin", "arm64"},
	{"darwin", "amd64"},
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"windows", "amd64"},
	{"windows", "arm64"},
}

// extras are the repository's files each archive holds beside the binary.
var extras = []string{"LICENSE", "README.md"}

var versionRE = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

type target struct{ goos, goarch string }

// binary is the name of nsgo's executable on the target.
func (t target) binary() string {
	if t.goos == "windows" {
		return "nsgo.exe"
	}
	return "nsgo"
}

// archive is the target's asset name for version, without the v.
func (t target) archive(version string) string {
	ext := ".tar.gz"
	if t.goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("nsgo_%s_%s_%s%s", version, t.goos, t.goarch, ext)
}

func main() {
	if err := run(".", os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root string, args []string) error {
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	out := fs.String("out", "", "the directory to write the archives and checksums.txt to, outside the repository")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return errors.New("usage: release -out DIR X.Y.Z")
	}
	version := fs.Arg(0)
	if !versionRE.MatchString(version) {
		return fmt.Errorf("%q is not a version: give X.Y.Z or X.Y.Z-suffix, without the v", version)
	}
	if err := build(root, *out, version); err != nil {
		return err
	}
	if err := verify(*out, version); err != nil {
		return err
	}
	fmt.Printf("built and verified %d archives and %s in %s\n", len(targets), checksums, *out)
	return nil
}

// build cross-compiles every target in the repository at root and writes the archives and checksums.txt to out.
func build(root, out, version string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	out, err = filepath.Abs(out)
	if err != nil {
		return err
	}
	// A file written inside the checkout would make Go stamp the next binary's version +dirty.
	if rel, err := filepath.Rel(root, out); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s is inside the repository %s; write the archives outside it", out, root)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	files := map[string][]byte{}
	for _, name := range extras {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		files[name] = data
	}
	work, err := os.MkdirTemp("", "nsgo-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	var sums strings.Builder
	for _, t := range targets {
		bin := filepath.Join(work, t.goos+"_"+t.goarch, t.binary())
		// -buildvcs=true overrides a GOFLAGS that turns stamping off, since the stamped version is what nsgo version prints. GOWORK=off builds the module as tagged, not with a workspace's replacements.
		cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=true", "-o", bin, "./cmd/nsgo")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+t.goos, "GOARCH="+t.goarch, "GOWORK=off")
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("building nsgo for %s/%s: %w", t.goos, t.goarch, err)
		}
		exe, err := os.ReadFile(bin)
		if err != nil {
			return err
		}
		info, err := buildinfo.ReadFile(bin)
		if err != nil {
			return fmt.Errorf("reading the build information of nsgo for %s/%s: %w", t.goos, t.goarch, err)
		}
		// The commit's time, so the same commit gives the same archive.
		modified := time.Unix(0, 0).UTC()
		if v, ok := setting(info, "vcs.time"); ok {
			if parsed, err := time.Parse(time.RFC3339, v); err == nil {
				modified = parsed
			}
		}
		entries := []entry{{t.binary(), 0o755, exe}}
		for _, name := range extras {
			entries = append(entries, entry{name, 0o644, files[name]})
		}
		var data []byte
		if t.goos == "windows" {
			data, err = zipArchive(entries, modified)
		} else {
			data, err = tarArchive(entries, modified)
		}
		if err != nil {
			return err
		}
		name := t.archive(version)
		if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	return os.WriteFile(filepath.Join(out, checksums), []byte(sums.String()), 0o644)
}

// entry is a file at an archive's root.
type entry struct {
	name string
	mode int64
	data []byte
}

func tarArchive(entries []entry, modified time.Time) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		header := &tar.Header{Name: e.name, Mode: e.mode, Size: int64(len(e.data)), ModTime: modified, Typeflag: tar.TypeReg, Format: tar.FormatPAX}
		if err := tw.WriteHeader(header); err != nil {
			return nil, err
		}
		if _, err := tw.Write(e.data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func zipArchive(entries []entry, modified time.Time) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		header := &zip.FileHeader{Name: e.name, Method: zip.Deflate, Modified: modified}
		header.SetMode(os.FileMode(e.mode))
		w, err := zw.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(e.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// verify reads back what build wrote to dir: checksums.txt lists exactly every target's archive with its SHA-256, each archive holds exactly the binary and extras at its root, and each binary's build information says it is nsgo at vversion for its platform, built with CGO_ENABLED=0 and -trimpath. The binary for this platform also has to print its version.
func verify(dir, version string) error {
	listed, err := readChecksums(filepath.Join(dir, checksums))
	if err != nil {
		return err
	}
	var want []string
	for _, t := range targets {
		want = append(want, t.archive(version))
	}
	got := make([]string, 0, len(listed))
	for name := range listed {
		got = append(got, name)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		return fmt.Errorf("%s lists %v, want %v", checksums, got, want)
	}
	for _, t := range targets {
		name := t.archive(version)
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != listed[name] {
			return fmt.Errorf("%s does not match its checksum in %s", name, checksums)
		}
		var files map[string]entry
		if t.goos == "windows" {
			files, err = readZip(data)
		} else {
			files, err = readTar(data)
		}
		if err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}
		names := make([]string, 0, len(files))
		for n := range files {
			names = append(names, n)
		}
		wantNames := append([]string{t.binary()}, extras...)
		slices.Sort(names)
		slices.Sort(wantNames)
		if !slices.Equal(names, wantNames) {
			return fmt.Errorf("%s holds %v, want %v", name, names, wantNames)
		}
		if files[t.binary()].mode&0o111 == 0 {
			return fmt.Errorf("%s's %s is not executable", name, t.binary())
		}
		if err := checkBinary(name, t, version, files[t.binary()].data); err != nil {
			return err
		}
	}
	return nil
}

// checkBinary checks the build information of the binary data, which archive name holds for t, and runs it when t is this platform.
func checkBinary(name string, t target, version string, data []byte) error {
	work, err := os.MkdirTemp("", "nsgo-release-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	bin := filepath.Join(work, t.binary())
	if err := os.WriteFile(bin, data, 0o755); err != nil {
		return err
	}
	info, err := buildinfo.ReadFile(bin)
	if err != nil {
		return fmt.Errorf("reading the build information of %s's %s: %w", name, t.binary(), err)
	}
	if info.Path != command || info.Main.Path != module {
		return fmt.Errorf("%s's %s is %s of %s, want %s of %s", name, t.binary(), info.Path, info.Main.Path, command, module)
	}
	if info.Main.Version != "v"+version {
		return fmt.Errorf("%s's %s has the version %q, want v%s: build from a clean checkout of the commit tagged v%s", name, t.binary(), info.Main.Version, version, version)
	}
	for key, want := range map[string]string{"GOOS": t.goos, "GOARCH": t.goarch, "CGO_ENABLED": "0", "-trimpath": "true"} {
		if v, _ := setting(info, key); v != want {
			return fmt.Errorf("%s's %s was built with %s=%q, want %q", name, t.binary(), key, v, want)
		}
	}
	if t.goos != runtime.GOOS || t.goarch != runtime.GOARCH {
		return nil
	}
	// Outside any module and with delegation off, so the binary reports itself rather than a module's nsgo.
	cmd := exec.Command(bin, "version")
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "NSGO_NO_DELEGATE=1")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("running %s's %s version: %w", name, t.binary(), err)
	}
	if got, want := strings.TrimSpace(string(output)), "nsgo v"+version; got != want {
		return fmt.Errorf("%s's %s version printed %q, want %q", name, t.binary(), got, want)
	}
	return nil
}

func setting(info *buildinfo.BuildInfo, key string) (string, bool) {
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value, true
		}
	}
	return "", false
}

// readChecksums parses sha256sum's output: a lowercase hex digest, two spaces and a file name per line.
func readChecksums(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sums := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		sum, name, ok := strings.Cut(scanner.Text(), "  ")
		if !ok || len(sum) != 2*sha256.Size || strings.ToLower(sum) != sum || name == "" {
			return nil, fmt.Errorf("%s has a malformed line %q", path, scanner.Text())
		}
		if _, err := hex.DecodeString(sum); err != nil {
			return nil, fmt.Errorf("%s has a malformed line %q", path, scanner.Text())
		}
		if _, dup := sums[name]; dup {
			return nil, fmt.Errorf("%s lists %s twice", path, name)
		}
		sums[name] = sum
	}
	return sums, scanner.Err()
}

func readTar(data []byte) (map[string]entry, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	files := map[string]entry{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return files, nil
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("%s is not a regular file", header.Name)
		}
		if _, dup := files[header.Name]; dup {
			return nil, fmt.Errorf("%s is in the archive twice", header.Name)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		files[header.Name] = entry{header.Name, header.Mode, body}
	}
}

func readZip(data []byte) (map[string]entry, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	files := map[string]entry{}
	for _, f := range zr.File {
		if !f.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file", f.Name)
		}
		if _, dup := files[f.Name]; dup {
			return nil, fmt.Errorf("%s is in the archive twice", f.Name)
		}
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			return nil, err
		}
		files[f.Name] = entry{f.Name, int64(f.Mode().Perm()), body}
	}
	return files, nil
}
