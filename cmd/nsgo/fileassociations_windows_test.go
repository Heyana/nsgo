package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// TestWindowsFileAssociationsNative installs an isolated NSIS package, opens a spaced/Unicode path through its ProgID, upgrades its declarations and uninstalls it. Only the Test workflow opts into writing test keys.
func TestWindowsFileAssociationsNative(t *testing.T) {
	if os.Getenv("NSGO_FILE_ASSOCIATIONS") == "" {
		t.Skip("set NSGO_FILE_ASSOCIATIONS=1 to test NSIS install and uninstall")
	}
	if nsisCompiler() == "" {
		t.Fatal("native file association verification needs NSIS")
	}
	dir, pkg, installed := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "installed app")
	id := fmt.Sprintf("dev.nsgo.associations.%d", time.Now().UnixNano())
	ext := fmt.Sprintf("nsgo%d", time.Now().UnixNano())
	p := &project{Dir: dir, Name: "Test App", ID: id, Version: "1.0.0", Release: true}
	p.FileTypes = []fileType{
		{Name: "Document", ID: id + ".document", Extensions: []string{ext, ext + "old"}, Role: "viewer"},
		{Name: "Reassigned", ID: id + ".reassigned", Extensions: []string{ext + "other"}},
	}
	writeFiles(t, dir, map[string]string{
		"go.mod": "module fileassociationstest\n\ngo 1.27.3\n",
		"main.go": `package main
import ("fmt"; "os"; "strings")
func main() {
  if len(os.Args) < 2 || strings.HasPrefix(os.Args[1], "--nsgo-") { return }
  f, err := os.Create(os.Getenv("NSGO_ASSOC_OUTPUT")); if err != nil { panic(err) }; defer f.Close()
  for _, arg := range os.Args[1:] { fmt.Fprintln(f, arg) }
}
`,
	})
	if err := goBuild(t.Context(), p, filepath.Join(pkg, p.Name+".exe"), []string{"CGO_ENABLED=0"}); err != nil {
		t.Fatal(err)
	}
	d, err := nsisScript(t.Context(), p, pkg, "x64")
	if err != nil {
		t.Fatal(err)
	}
	prog, reassigned := d.FileTypes[0].ProgID, d.FileTypes[1].ProgID
	meta := d.UninstallKey
	// Cleanup is bounded to this test's unique keys, including failures.
	t.Cleanup(func() {
		for _, key := range []string{meta, `Software\Classes\` + prog, `Software\Classes\` + reassigned,
			`Software\Classes\.` + ext, `Software\Classes\.` + ext + "old", `Software\Classes\.` + ext + "other"} {
			exec.Command("reg", "delete", `HKCU\`+key, "/f").Run()
		}
	})
	set := func(key, name, value string) {
		t.Helper()
		k, _, err := registry.CreateKey(registry.CURRENT_USER, key, registry.SET_VALUE)
		if err != nil {
			t.Fatal(err)
		}
		defer k.Close()
		if err := k.SetStringValue(name, value); err != nil {
			t.Fatal(err)
		}
	}
	read := func(key, name string) (string, error) {
		k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE)
		if err != nil {
			return "", err
		}
		defer k.Close()
		value, _, err := k.GetStringValue(name)
		return value, err
	}
	set(`Software\Classes\.`+ext, "", "Other.App")
	set(`Software\Classes\.`+ext+`\OpenWithProgids`, "Other.App", "")
	// Another app's default with no Open With values of its own leaves the extension's key with values alone once ours go; it must survive.
	set(`Software\Classes\.`+ext+"old", "", "Other.Old")
	install := func() {
		t.Helper()
		setup, err := buildNSIS(t.Context(), p, pkg, "x64")
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(t.Context(), setup)
		// NSIS requires /D to be last and unquoted, even with spaces.
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `"` + setup + `" /S /D=` + installed}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("install: %v\n%s", err, out)
		}
	}
	install()
	for _, suffix := range []string{"", "old"} {
		if _, err := read(`Software\Classes\.`+ext+suffix+`\OpenWithProgids`, prog); err != nil {
			t.Fatalf("Open With lacks the installed app: %v", err)
		}
	}
	if got, err := read(`Software\Classes\.`+ext, ""); err != nil || got != "Other.App" {
		t.Fatalf("installer replaced the default: %q, %v", got, err)
	}
	path := filepath.Join(t.TempDir(), "日本語 document."+ext)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(t.TempDir(), "opened")
	t.Setenv("NSGO_ASSOC_OUTPUT", result)
	openAssociation(t, prog, path)
	got, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSuffix(string(got), "\n"), "\n")
	if len(args) != 1 {
		t.Fatalf("Shell file arguments = %q; want one path", got)
	}
	// The Shell may expand a TEMP path's 8.3 directory names. Compare file identity, rather than the spelling of two paths to one file.
	wantInfo, wantErr := os.Stat(path)
	gotInfo, gotErr := os.Stat(args[0])
	if wantErr != nil || gotErr != nil || !os.SameFile(wantInfo, gotInfo) {
		t.Fatalf("Shell opened %q instead of %q: %v, %v", args[0], path, gotErr, wantErr)
	}
	// Another program replaces one command; neither upgrade nor uninstall may remove that registration. The old extension of our type must go.
	set(`Software\Classes\`+reassigned+`\shell\open\command`, "", `"foreign.exe" "%1"`)
	p.FileTypes = p.FileTypes[:1]
	p.FileTypes[0].Extensions = []string{ext}
	install()
	if _, err := read(`Software\Classes\.`+ext+`old\OpenWithProgids`, prog); err == nil {
		t.Error("upgrade kept a dropped extension")
	}
	if got, err := read(`Software\Classes\.`+ext+"old", ""); err != nil || got != "Other.Old" {
		t.Errorf("upgrade removed another app's default of a dropped extension: %q, %v", got, err)
	}
	uninstaller := filepath.Join(installed, "Uninstall.exe")
	// Use the same silent invocation as the shipped install script. Let Go quote the spaced executable path and pass /S as a real argument.
	cmd := exec.CommandContext(t.Context(), uninstaller, "/S")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("uninstall: %v\n%s", err, out)
	}
	// NSIS may finish deleting its self-copy just after the process that launched it exits. Wait briefly for the registry transaction to settle.
	deadline := time.Now().Add(2 * time.Second)
	for {
		metaKey, metaErr := registry.OpenKey(registry.CURRENT_USER, meta, registry.READ)
		if metaErr == nil {
			metaKey.Close()
		}
		progKey, progErr := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\`+prog, registry.READ)
		if progErr == nil {
			progKey.Close()
		}
		if metaErr != nil && progErr != nil {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	for _, key := range []string{meta, `Software\Classes\` + prog} {
		if k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.READ); err == nil {
			k.Close()
			t.Errorf("uninstall kept %s", key)
		}
	}
	if _, err := read(`Software\Classes\.`+ext+`\OpenWithProgids`, prog); err == nil {
		t.Error("uninstall kept our Open With value")
	}
	if _, err := read(`Software\Classes\.`+ext+`\OpenWithProgids`, "Other.App"); err != nil {
		t.Error("uninstall removed another app's Open With value")
	}
	if got, err := read(`Software\Classes\.`+ext, ""); err != nil || got != "Other.App" {
		t.Error("uninstall changed the default handler")
	}
	if got, err := read(`Software\Classes\`+reassigned+`\shell\open\command`, ""); err != nil || !strings.Contains(got, "foreign.exe") {
		t.Error("uninstall removed a reassigned ProgID")
	}
}

// openAssociation uses Explorer's class selection without changing the user's default handler or showing an Open With chooser.
func openAssociation(t *testing.T, progID, file string) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole := windows.NewLazySystemDLL("ole32.dll")
	hr, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2)
	if int32(hr) < 0 {
		t.Fatalf("CoInitializeEx: %#x", hr)
	}
	defer ole.NewProc("CoUninitialize").Call()
	type shellExecuteInfo struct {
		Size, Mask                        uint32
		Wnd                               windows.Handle
		Verb, File, Parameters, Directory *uint16
		Show                              int32
		Instance                          windows.Handle
		IDList                            uintptr
		Class                             *uint16
		Key                               windows.Handle
		HotKey                            uint32
		Icon, Process                     windows.Handle
	}
	info := shellExecuteInfo{Mask: 0x541, Show: 0, File: windows.StringToUTF16Ptr(file), Class: windows.StringToUTF16Ptr(progID), Verb: windows.StringToUTF16Ptr("open")}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, err := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		t.Fatalf("ShellExecuteEx: %v", err)
	}
	defer windows.CloseHandle(info.Process)
	if state, err := windows.WaitForSingleObject(info.Process, 10000); err != nil || state != windows.WAIT_OBJECT_0 {
		windows.TerminateProcess(info.Process, 1)
		t.Fatalf("document app did not exit: %d, %v", state, err)
	}
}

// TestNSISPathOwnershipNative installs a windows.command_line package and checks that its installers and uninstallers take off the user's Path only the folder an installer added: never an entry the user had, the old folder on a move, and nothing from an uninstaller the move left behind. The user's Path is restored afterwards.
func TestNSISPathOwnershipNative(t *testing.T) {
	if os.Getenv("NSGO_FILE_ASSOCIATIONS") == "" {
		t.Skip("set NSGO_FILE_ASSOCIATIONS=1 to test NSIS install and uninstall")
	}
	if nsisCompiler() == "" {
		t.Fatal("native Path verification needs NSIS")
	}
	env, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	saved, kind, savedErr := env.GetStringValue("Path")
	dir, pkg := t.TempDir(), t.TempDir()
	a, b := filepath.Join(t.TempDir(), "app a"), filepath.Join(t.TempDir(), "app b")
	p := &project{Dir: dir, Name: "Path App", ID: fmt.Sprintf("dev.nsgo.path.%d", time.Now().UnixNano()), Version: "1.0.0", Release: true}
	p.Windows.CommandLine = true
	t.Cleanup(func() {
		defer env.Close()
		var err error
		switch {
		case savedErr != nil:
			if err = env.DeleteValue("Path"); errors.Is(err, registry.ErrNotExist) {
				err = nil
			}
		case kind == registry.EXPAND_SZ:
			err = env.SetExpandStringValue("Path", saved)
		default:
			err = env.SetStringValue("Path", saved)
		}
		if err != nil {
			t.Errorf("restoring the user's Path: %v", err)
		}
		exec.Command("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\`+p.ID, "/f").Run()
	})
	writeFiles(t, dir, map[string]string{
		"go.mod":  "module pathownershiptest\n\ngo 1.27.3\n",
		"main.go": "package main\n\nfunc main() {}\n",
	})
	if err := goBuild(t.Context(), p, filepath.Join(pkg, p.Name+".exe"), []string{"CGO_ENABLED=0"}); err != nil {
		t.Fatal(err)
	}
	setup, err := buildNSIS(t.Context(), p, pkg, "x64")
	if err != nil {
		t.Fatal(err)
	}
	run := func(name string, cmd *exec.Cmd) {
		t.Helper()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		// The uninstaller finishes from a copy of itself after the process that started it exits.
		time.Sleep(2 * time.Second)
	}
	install := func(folder string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), setup)
		// NSIS requires /D to be last and unquoted, even with spaces.
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `"` + setup + `" /S /D=` + folder}
		run("install", cmd)
	}
	uninstall := func(folder string) {
		t.Helper()
		run("uninstall", exec.CommandContext(t.Context(), filepath.Join(folder, "Uninstall.exe"), "/S"))
	}
	onPath := func(folder string) bool {
		t.Helper()
		path, _, err := env.GetStringValue("Path")
		if err != nil {
			return false
		}
		for _, entry := range strings.Split(path, ";") {
			if strings.EqualFold(strings.TrimRight(entry, `\`), folder) {
				return true
			}
		}
		return false
	}
	set := func(entries ...string) {
		t.Helper()
		if err := env.SetExpandStringValue("Path", strings.Join(entries, ";")); err != nil {
			t.Fatal(err)
		}
	}

	// A folder the user had on Path stays after the uninstall.
	set(saved, a)
	install(a)
	uninstall(a)
	if !onPath(a) {
		t.Error("the uninstaller removed the user's own Path entry")
	}

	// The installer adds the folder, a move takes it off for the new one, the uninstaller the move left behind keeps the new one, and the new uninstaller removes it.
	set(saved)
	install(a)
	if !onPath(a) {
		t.Fatal("the installer did not put its folder on Path")
	}
	install(b)
	if onPath(a) || !onPath(b) {
		t.Errorf("after moving, Path holds the old folder %v and the new one %v", onPath(a), onPath(b))
	}
	uninstall(a)
	if !onPath(b) {
		t.Error("the moved-from uninstaller removed the installation's Path entry")
	}
	uninstall(b)
	if onPath(b) {
		t.Error("the uninstaller left its folder on Path")
	}
}
