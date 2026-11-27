package main

import (
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

	projectcfg "github.com/nsgo-dev/nsgo/project"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// TestWindowsURLSchemesNative installs an isolated Go installer, opens a link of its scheme through the shell, drops the scheme in an upgrade and uninstalls it, keeping a scheme another app took meanwhile. Only the Test workflow opts into writing test keys.
func TestWindowsURLSchemesNative(t *testing.T) {
	if os.Getenv("NSGO_FILE_ASSOCIATIONS") == "" {
		t.Skip("set NSGO_FILE_ASSOCIATIONS=1 to test Windows install and uninstall")
	}
	dir, pkg, installed := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "installed app")
	stamp := time.Now().UnixNano()
	id := fmt.Sprintf("dev.nsgo.schemes.%d", stamp)
	scheme, dropped, taken := fmt.Sprintf("nsgotest%d", stamp), fmt.Sprintf("nsgotest%dold", stamp), fmt.Sprintf("nsgotest%dtaken", stamp)
	p := &project{Config: projectcfg.Config{Dir: dir, Name: "Test App", ID: id, Version: "1.0.0", URLSchemes: []string{scheme, dropped, taken}}, Release: true}
	writeFiles(t, dir, map[string]string{
		"go.mod": "module urlschemestest\n\ngo 1.27.3\n",
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
	d, err := windowsInstallerPlan(t.Context(), p, pkg, "x64")
	if err != nil {
		t.Fatal(err)
	}
	meta := d.UninstallKey
	// Cleanup is bounded to this test's unique keys, including failures.
	t.Cleanup(func() {
		for _, key := range []string{meta, `Software\Classes\` + scheme, `Software\Classes\` + dropped, `Software\Classes\` + taken} {
			exec.Command("reg", "delete", `HKCU\`+key, "/f").Run()
		}
	})
	exists := func(key string) bool {
		k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.READ)
		if err == nil {
			k.Close()
		}
		return err == nil
	}
	install := func() {
		t.Helper()
		setup, err := buildWindowsInstaller(t.Context(), p, pkg, "x64")
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(t.Context(), setup)
		// The installer accepts /D to be last and unquoted, even with spaces.
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `"` + setup + `" /S /D=` + installed}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("install: %v\n%s", err, out)
		}
	}
	install()
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\`+scheme, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := k.GetStringValue("URL Protocol"); err != nil {
		t.Errorf("the scheme lacks URL Protocol: %v", err)
	}
	k.Close()
	result := filepath.Join(t.TempDir(), "opened")
	t.Setenv("NSGO_ASSOC_OUTPUT", result)
	link := scheme + `://entry/word?q=a%20b`
	openLink(t, link)
	got, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	if args := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(got), "\r\n", "\n")), "\n"); len(args) != 2 || args[0] != "--" || !strings.EqualFold(args[1], link) {
		t.Fatalf("the shell started the app with %q, want -- and %q", args, link)
	}
	// Another app takes one scheme; an upgrade drops another.
	tk, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+taken+`\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	tk.SetStringValue("", `"foreign.exe" "%1"`)
	tk.Close()
	p.URLSchemes = []string{scheme, taken}
	install()
	if exists(`Software\Classes\` + dropped) {
		t.Error("the upgrade kept a dropped scheme")
	}
	if !exists(`Software\Classes\` + scheme) {
		t.Error("the upgrade lost a scheme it keeps")
	}
	tk, _, _ = registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+taken+`\shell\open\command`, registry.SET_VALUE)
	tk.SetStringValue("", `"foreign.exe" "%1"`)
	tk.Close()
	cmd := exec.CommandContext(t.Context(), filepath.Join(installed, "Uninstall.exe"), "/S")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("uninstall: %v\n%s", err, out)
	}
	// The uninstaller may finish deleting its self-copy just after the process that launched it exits.
	deadline := time.Now().Add(2 * time.Second)
	for (exists(meta) || exists(`Software\Classes\`+scheme)) && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if exists(meta) || exists(`Software\Classes\`+scheme) {
		t.Error("uninstall kept the scheme or its record")
	}
	if !exists(`Software\Classes\` + taken) {
		t.Error("uninstall removed a scheme another app took")
	}
}

// openLink opens link as a browser hands one to the shell, and waits for the app it started.
func openLink(t *testing.T, link string) {
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
	// SEE_MASK_NOCLOSEPROCESS | SEE_MASK_NOASYNC | SEE_MASK_FLAG_NO_UI
	info := shellExecuteInfo{Mask: 0x540, File: windows.StringToUTF16Ptr(link), Verb: windows.StringToUTF16Ptr("open")}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, err := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		t.Fatalf("ShellExecuteEx: %v", err)
	}
	if info.Process == 0 {
		t.Fatal("ShellExecuteEx started no process")
	}
	defer windows.CloseHandle(info.Process)
	if state, err := windows.WaitForSingleObject(info.Process, 10000); err != nil || state != windows.WAIT_OBJECT_0 {
		windows.TerminateProcess(info.Process, 1)
		t.Fatalf("the app did not exit: %d, %v", state, err)
	}
}
