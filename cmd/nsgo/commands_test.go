package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/authenticode"
)

func TestCommandsConfig(t *testing.T) {
	for _, c := range []struct {
		yaml, err string
	}{
		{"commands: [{name: tool, package: ./cmd/tool}]", ""},
		{"commands: [{name: tool-admin_2, package: example.com/notes/cmd/admin}, {name: tool, package: ./cmd/tool}]", ""},
		{"commands: [{name: '', package: ./cmd/tool}]", "commands entry 1: name"},
		{"commands: [{name: tool.exe, package: ./cmd/tool}]", "letters, digits"},
		{"commands: [{name: -tool, package: ./cmd/tool}]", "letters, digits"},
		{"commands: [{name: ../tool, package: ./cmd/tool}]", "letters, digits"},
		{"commands: [{name: nul, package: ./cmd/tool}]", "reserved Windows file name"},
		{"commands: [{name: com1, package: ./cmd/tool}]", "reserved Windows file name"},
		{"commands: [{name: notes, package: ./cmd/tool}]", "the app's own executable"},
		{"name: My Notes\ncommands: [{name: my-notes, package: ./cmd/tool}]", "the app's own command in /usr/bin"},
		{"commands: [{name: uninstall, package: ./cmd/tool}]", "the app's own Windows uninstaller"},
		{"commands: [{name: apprun, package: ./cmd/tool}]", "the app's own AppImage entry point"},
		{"commands: [{name: tool, package: ./cmd/tool}, {name: TOOL, package: ./cmd/tool2}]", "tool and TOOL name the same file"},
		{"commands: [{name: tool}]", "is not one Go package's path"},
		{"commands: [{name: tool, package: ./cmd/...}]", "is not one Go package's path"},
		{"commands: [{name: tool, package: -toolexec=x}]", "is not one Go package's path"},
		{"commands: [{name: tool, package: ./cmd/tool, ldflags: [-X main.channel=beta]}]", ""},
	} {
		dir := t.TempDir()
		manifest := "name: Notes\nid: dev.example.notes\n" + c.yaml + "\n"
		if strings.HasPrefix(c.yaml, "name:") {
			manifest = "id: dev.example.notes\n" + c.yaml + "\n"
		}
		writeFiles(t, dir, map[string]string{"nsgo.yaml": manifest})
		_, err := load(dir)
		if c.err == "" && err != nil {
			t.Errorf("%s: %v", c.yaml, err)
		}
		if c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%s: error %v, want %q", c.yaml, err, c.err)
		}
	}
}

// commandFixture is an app's module with a command's main package, cmd/tool, and a package that is not one, lib.
func commandFixture(t *testing.T) *project {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":           "module example.com/fixture\n\ngo 1.26\n",
		"cmd/tool/main.go": "package main\n\nimport \"fmt\"\n\nvar version, channel = \"dev\", \"stable\"\n\nfunc main() { fmt.Println(version, channel) }\n",
		"lib/lib.go":       "package lib\n",
	})
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	return &project{Dir: dir, Name: "App", ID: "dev.example.app", Version: "1.0.0", Release: true}
}

// A command built from a Go package lands beside the app as an executable for the target, named for its system.
func TestAddCommands(t *testing.T) {
	p := commandFixture(t)
	p.Commands = []command{
		{Name: "tool", Package: "./cmd/tool"},
	}
	linux := t.TempDir()
	if err := p.addCommands(t.Context(), "linux_x64", linux); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tool"} {
		path := filepath.Join(linux, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&0o111 == 0 {
			t.Errorf("%s's mode is %v", name, info.Mode())
		}
		f, err := elf.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if f.Machine != elf.EM_X86_64 {
			t.Errorf("%s is for %v", name, f.Machine)
		}
		f.Close()
	}
	if got := p.commandFiles(linux, "linux_x64"); len(got) != 1 {
		t.Errorf("commandFiles = %q", got)
	}

	windows := t.TempDir()
	if err := p.addCommands(t.Context(), "windows_x64", windows); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tool.exe"} {
		f, err := pe.Open(filepath.Join(windows, name))
		if err != nil {
			t.Fatal(err)
		}
		if f.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
			t.Errorf("%s is for machine %#x", name, f.Machine)
		}
		f.Close()
	}

	mac := t.TempDir()
	if err := p.addCommands(t.Context(), "macos_arm64", mac); err != nil {
		t.Fatal(err)
	}
	f, err := macho.Open(filepath.Join(mac, "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Cpu != macho.CpuArm64 || f.Type != macho.TypeExec {
		t.Errorf("tool is a %v %v", f.Cpu, f.Type)
	}
	f.Close()
}

// A command links with the app's linker flags, then its own, whose -X values win, and the identity core.App reads, which neither replaces.
func TestCommandLdflags(t *testing.T) {
	p := commandFixture(t)
	p.Build.LDFlags = []string{"-X main.version=1.0.0", "-X main.channel=app"}
	c := command{Name: "tool", Package: "./cmd/tool", LDFlags: []string{"-X 'main.channel=beta builds'"}}
	flags := p.commandLdflags(c, "-s", "-w")
	for _, part := range []string{"-X main.version=1.0.0", "-X main.channel=app", "-X 'main.channel=beta builds'", "-X '" + corePath + ".appVersion=1.0.0'", "-X '" + corePath + ".appID=dev.example.app'", "-X " + corePath + ".appRelease=1", "-s -w"} {
		if !strings.Contains(flags, part) {
			t.Errorf("ldflags %q lack %q", flags, part)
		}
	}
	if strings.Index(flags, "main.channel=app") > strings.Index(flags, "main.channel=beta") || strings.Index(flags, "main.channel=beta") > strings.Index(flags, corePath) {
		t.Errorf("ldflags %q are out of order", flags)
	}

	// Built for this machine, the command runs with the values its flags gave.
	system, arch := runtime.GOOS, goArchName(runtime.GOARCH)
	if system == "darwin" {
		system = "macos"
	}
	target := system + "_" + arch
	if !slices.Contains([]string{"macos", "windows", "linux"}, system) {
		t.Skipf("no command target for %s", target)
	}
	p.Commands = []command{c}
	dir := t.TempDir()
	if err := p.addCommands(t.Context(), target, dir); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(filepath.Join(dir, commandFile("tool", target))).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "1.0.0 beta builds" {
		t.Errorf("the command prints %q, want its version and channel from the flags", got)
	}
}

func TestAddCommandsRefuses(t *testing.T) {
	p := commandFixture(t)
	for _, c := range []struct {
		name   string
		cmd    command
		target string
		before string
		err    string
	}{
		{"a package that is no main package", command{Name: "tool", Package: "./lib"}, "linux_x64", "", "./lib built no executable"},
		{"a package that does not exist", command{Name: "tool", Package: "./missing"}, "linux_x64", "", "tool's package ./missing"},
		{"a file the package holds", command{Name: "tool", Package: "./cmd/tool"}, "linux_x64", "tool", "tool collides with"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p.Commands = []command{c.cmd}
			dir := t.TempDir()
			if c.before != "" {
				writeFiles(t, dir, map[string]string{c.before: "resource"})
			}
			err := p.addCommands(t.Context(), c.target, dir)
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Fatalf("error %v, want %q", err, c.err)
			}
		})
	}
}

// The store's packages run only the app's own executable, so they leave the commands out.
func TestStorePackagesLeaveOutCommands(t *testing.T) {
	p := commandFixture(t)
	p.Commands = []command{{Name: "tool", Package: "./cmd/tool"}}
	p.Updates.Channel = channelStore
	dir := t.TempDir()
	if err := p.addCommands(t.Context(), "linux_x64", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tool")); err == nil {
		t.Error("the store's package holds the command")
	}
	p.Windows.Path = true
	if p.windowsPath() {
		t.Error("the MSIX puts its folder on Path")
	}
}

// codesign signs the commands with the identity, the hardened runtime and a timestamp but none of the app's entitlements, after the libraries and before the bundle, whose signature seals them.
func TestSignMacAppSignsCommandsBeforeBundle(t *testing.T) {
	app := filepath.Join(t.TempDir(), "App.app")
	writeFiles(t, app, map[string]string{
		"Contents/MacOS/App":                "app",
		"Contents/MacOS/tool":               "tool",
		"Contents/Frameworks/fixture.dylib": "lib",
	})
	p := &project{Name: "App", ID: "dev.example.app", Release: true}
	p.Commands = []command{{Name: "tool", Package: "./cmd/tool"}, {Name: "absent", Package: "./cmd/absent"}}
	p.Sign.Identity = "Developer ID Application: Example (ABCDE12345)"
	p.Sign.Notarize = "nsgo"
	p.Apple.Info = map[string]any{"NSMicrophoneUsageDescription": "Calls"}
	var signed [][]string
	ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
		if cmd.name != "codesign" {
			t.Fatalf("unexpected command %s %q", cmd.name, cmd.args)
		}
		signed = append(signed, cmd.args)
		return nil, nil
	})
	if err := signMacApp(ctx, p, app); err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, args := range signed {
		order = append(order, filepath.Base(args[len(args)-1]))
	}
	if want := []string{"fixture.dylib", "tool", "App.app"}; !slices.Equal(order, want) {
		t.Fatalf("signed %q, want %q", order, want)
	}
	want := []string{"--force", "--sign", p.Sign.Identity, "--options", "runtime", "--timestamp", "--identifier", "dev.example.app.tool", filepath.Join(app, "Contents", "MacOS", "tool")}
	if !slices.Equal(signed[1], want) {
		t.Errorf("the command is signed with %q, want %q", signed[1], want)
	}
	if !slices.Contains(signed[2], "--entitlements") {
		t.Errorf("the app is signed without its entitlements: %q", signed[2])
	}
}

// Without an identity, the commands are signed ad hoc before the bundle, as lipo broke their signatures in a universal app.
func TestSignUniversalAdHocSignsCommands(t *testing.T) {
	app := filepath.Join(t.TempDir(), "App.app")
	writeFiles(t, app, map[string]string{"Contents/MacOS/App": "app", "Contents/MacOS/tool": "tool"})
	p := &project{Name: "App", ID: "dev.example.app"}
	p.Commands = []command{{Name: "tool", Package: "./cmd/tool"}}
	var order []string
	var signed [][]string
	ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
		if want := []string{"--force", "--sign", "-"}; cmd.name != "codesign" || !slices.Equal(cmd.args[:3], want) {
			t.Fatalf("unexpected command %s %q", cmd.name, cmd.args)
		}
		signed = append(signed, cmd.args)
		order = append(order, filepath.Base(cmd.args[len(cmd.args)-1]))
		return nil, nil
	})
	if err := signUniversalAdHoc(ctx, p, app); err != nil {
		t.Fatal(err)
	}
	if want := []string{"tool", "App.app"}; !slices.Equal(order, want) {
		t.Fatalf("signed %q, want %q", order, want)
	}
	if want := []string{"--force", "--sign", "-", "--identifier", "dev.example.app.tool", filepath.Join(app, "Contents", "MacOS", "tool")}; !slices.Equal(signed[0], want) {
		t.Errorf("the command is signed with %q, want %q", signed[0], want)
	}
}

// The Debian and Arch packages install the commands beside the app and own a link to each in /usr/bin, which removing them removes; the AppImage holds them beside the app.
func TestLinuxPackagesLinkCommands(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"App": "binary", "tool": "command", "dev.example.app.png": "icon"})
	os.Chmod(filepath.Join(dir, "App"), 0o755)
	os.Chmod(filepath.Join(dir, "tool"), 0o755)
	p := &project{Dir: dir, Name: "App", ID: "dev.example.app", Version: "1.0.0"}
	p.Commands = []command{{Name: "tool", Package: "./cmd/tool"}}
	files, err := linuxPackageFiles(p, dir)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]linuxPackageEntry{}
	for _, f := range files {
		byName[f.name] = f
	}
	if f := byName["opt/app/tool"]; f.mode != 0o755 || string(f.data) != "command" {
		t.Errorf("opt/app/tool is %+v", f)
	}
	if f := byName["usr/bin/tool"]; f.link != "/opt/app/tool" {
		t.Errorf("usr/bin/tool links to %q", f.link)
	}
	if f := byName["usr/bin/app"]; f.link != "/opt/app/App" {
		t.Errorf("usr/bin/app links to %q", f.link)
	}
	deb, err := debPackage(p, files, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := archPackage(p, files, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	for name, archive := range map[string][]byte{"deb": arMembers(t, deb)["data.tar.gz"], "arch": pkg} {
		prefix := ""
		if name == "deb" {
			prefix = "./"
		}
		headers := tarHeaders(t, archive)
		if h := headers[prefix+"usr/bin/tool"]; h == nil || h.Typeflag != tar.TypeSymlink || h.Linkname != "/opt/app/tool" {
			t.Errorf("the %s package's usr/bin/tool is %+v", name, h)
		}
		if h := headers[prefix+"opt/app/tool"]; h == nil || h.Typeflag != tar.TypeReg || h.Mode != 0o755 {
			t.Errorf("the %s package's opt/app/tool is %+v", name, h)
		}
	}
	entries, err := appDirEntries(p, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(entries, func(e linuxPackageEntry) bool { return e.name == "tool" && e.mode == 0o755 }) {
		t.Error("the AppImage lacks the command")
	}
	if slices.ContainsFunc(entries, func(e linuxPackageEntry) bool { return strings.HasPrefix(e.name, "usr/bin/") }) {
		t.Error("the AppImage links commands into usr/bin")
	}

	// Without commands, the package links the app alone.
	p.Commands = nil
	files, err = linuxPackageFiles(p, dir)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(files, func(f linuxPackageEntry) bool { return f.name == "usr/bin/tool" }) {
		t.Error("a package without commands links tool")
	}
}

// tarHeaders reads the headers of a gzip-compressed tar archive by name.
func tarHeaders(t *testing.T, data []byte) map[string]*tar.Header {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]*tar.Header{}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return headers
		}
		if err != nil {
			t.Fatal(err)
		}
		headers[h.Name] = h
	}
}

// A signed Windows package signs the commands as the app's executable, replacing a signature they carried.
func TestSignsWindowsCommands(t *testing.T) {
	image := buildTestPE(t)
	other := testWindowsKey(t)
	signedElsewhere, err := authenticode.Sign(image, other, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"App.exe": string(image), "tool.exe": string(signedElsewhere)})
	key := testWindowsKey(t)
	cert := filepath.Join(t.TempDir(), "release.p12")
	writeTestCertificate(t, cert, key)
	p := &project{Dir: dir, Name: "App", ID: "dev.example.app", Version: "1.0.0", Release: true}
	p.Commands = []command{{Name: "tool", Package: "./cmd/tool"}}
	p.Sign.Certificate = cert
	p.Sign.TimestampURL = "none"
	if err := windowsSignerOf(p).signWindowsPackage(t.Context(), p, dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "tool.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(data, signedElsewhere) {
		t.Error("the command keeps another signature")
	}
	if !bytes.Contains(data, key.Cert.Raw) {
		t.Error("the command is not signed with the app's certificate")
	}
}

// Homebrew's cask links each command in the app onto its PATH.
func TestCaskLinksCommands(t *testing.T) {
	p, _ := updatingProject(t, "")
	p.Commands = []command{{Name: "tool", Package: "./cmd/tool"}, {Name: "tool-admin", Package: "./cmd/admin"}}
	if err := p.cleanReleaseDir(); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, p.releaseDir(), map[string]string{"hello-1.2.0-macos-arm64.dmg": "arm"})
	if err := p.writePackageManifests(t.Context(), []string{"macos_arm64"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(p.releaseDir(), "homebrew", "hello.rb")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "  app \"Hello.app\"\n  binary \"#{appdir}/Hello.app/Contents/MacOS/tool\"\n  binary \"#{appdir}/Hello.app/Contents/MacOS/tool-admin\"\n"
	if !strings.Contains(string(data), want) {
		t.Errorf("cask lacks %q:\n%s", want, data)
	}
	checkRubySyntax(t, path)
}
