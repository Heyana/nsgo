package windowsinstaller

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
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
)

// TestWindowsInstallerNative consumes two normally built fixture installers. Opt in only in an owned interactive session; no compiler is needed there.
func TestWindowsInstallerNative(t *testing.T) {
	root := os.Getenv("NSGO_INSTALLER_NATIVE")
	if root == "" {
		t.Skip("set NSGO_INSTALLER_NATIVE to an owned fixture directory")
	}
	var meta struct{ ID, Extension, OldExtension, OtherExtension, Scheme, OldScheme, TakenScheme, Name, Exe string }
	data, err := os.ReadFile(filepath.Join(root, "native-meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	// JSON names match the staging metadata, independent of Go's field spelling.
	var fields map[string]string
	if err = json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	meta.ID = fields["id"]
	meta.Extension = fields["extension"]
	meta.OldExtension = fields["old_extension"]
	meta.OtherExtension = fields["other_extension"]
	meta.Scheme = fields["scheme"]
	meta.OldScheme = fields["old_scheme"]
	meta.TakenScheme = fields["taken_scheme"]
	meta.Name = fields["name"]
	meta.Exe = fields["exe"]
	if !strings.HasPrefix(meta.ID, "dev.nsgo.test.installer1408.") || len(meta.ID) != len("dev.nsgo.test.installer1408.")+8 {
		t.Fatal("refusing non-fixture identity")
	}
	for _, v := range []string{meta.Extension, meta.OldExtension, meta.OtherExtension, meta.Scheme, meta.OldScheme, meta.TakenScheme} {
		if v == "" || strings.ContainsAny(v, `\/`) {
			t.Fatal("invalid fixture registration")
		}
	}
	m := Manifest{ID: meta.ID, Name: meta.Name, Exe: meta.Exe}
	installed := filepath.Join(root, "installed 日本語  two & ! % space")
	uninstallKey := m.UninstallKey()
	prog := func(suffix string) string {
		sum := sha256.Sum256([]byte(meta.ID + suffix))
		return fmt.Sprintf("%s.File.%x", meta.ID, sum[:8])
	}
	document, reassigned := prog(".document"), prog(".reassigned")
	originalPath, originalKind, pathErr := readValue("Environment", "Path")
	if pathErr != nil && pathErr != syscall.ERROR_FILE_NOT_FOUND {
		t.Fatal(pathErr)
	}
	restorePath := func() {
		if pathErr == syscall.ERROR_FILE_NOT_FOUND {
			_ = deleteValue("Environment", "Path")
		} else {
			_ = writeValue("Environment", "Path", originalKind, originalPath)
		}
	}
	keys := []string{uninstallKey, `Software\Classes\` + document, `Software\Classes\` + reassigned}
	for _, v := range []string{meta.Extension, meta.OldExtension, meta.OtherExtension} {
		keys = append(keys, `Software\Classes\.`+v)
	}
	for _, v := range []string{meta.Scheme, meta.OldScheme, meta.TakenScheme} {
		keys = append(keys, `Software\Classes\`+v)
	}
	for _, key := range keys {
		h, err := openKey(key, false)
		if err == nil {
			closeKey(h)
			t.Fatal("fixture registration already exists")
		}
		if err != syscall.ERROR_FILE_NOT_FOUND {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		// Best effort only for our isolated registration namespace and install root.
		if _, err := os.Stat(filepath.Join(installed, "Uninstall.exe")); err == nil {
			_ = exec.Command(filepath.Join(installed, "Uninstall.exe"), "/S").Run()
		}
		for _, key := range keys {
			_ = deleteTree(key)
		}
		restorePath()
	})
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	exists := func(key string) bool {
		h, err := openKey(key, false)
		if err == nil {
			closeKey(h)
		}
		return err == nil
	}
	wait := func(label string, fn func() bool) {
		t.Helper()
		deadline := time.Now().Add(40 * time.Second)
		for !fn() {
			if time.Now().After(deadline) {
				t.Fatal("timed out: " + label)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	install := func(version string, override bool) {
		t.Helper()
		setup := filepath.Join(root, "setup-"+version+".exe")
		command := exec.CommandContext(t.Context(), setup)
		line := `"` + setup + `" /S`
		if override {
			line += " /D=" + installed
		}
		command.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("install %s: %v %s", version, err, out)
		}
	}
	must(os.MkdirAll(installed, 0700))
	sentinel := filepath.Join(installed, "keep-user-data.txt")
	must(os.WriteFile(sentinel, []byte("preserve"), 0600))
	must(writeString(`Software\Classes\.`+meta.Extension, "", "Other.App"))
	must(writeString(`Software\Classes\.`+meta.Extension+`\OpenWithProgids`, "Other.App", ""))
	must(writeString(`Software\Classes\.`+meta.OldExtension, "", "Other.Old"))
	install("v1", true)
	if got := readString(uninstallKey, "InstallLocation"); got != installed {
		t.Fatalf("custom directory changed: %q", got)
	}
	if !sameDirectory(readString(uninstallKey, pathOwnedValue), installed) {
		t.Fatal("installer did not own its new PATH entry")
	}
	if readString(`Software\Classes\.`+meta.Extension, "") != "Other.App" {
		t.Fatal("installer changed default handler")
	}
	start, err := shellFolder(2)
	must(err)
	shortcutFile := filepath.Join(start, meta.Name+".lnk")
	if _, err = os.Stat(shortcutFile); err != nil {
		t.Fatal(err)
	}
	documentFile := filepath.Join(root, "日本語 document."+meta.Extension)
	must(os.WriteFile(documentFile, nil, 0600))
	nativeShellOpen(t, documentFile, document)
	link := meta.Scheme + "://entry/word?q=a%20b"
	nativeShellOpen(t, link, "")
	calls, err := os.ReadFile(filepath.Join(installed, "fixture-calls.jsonl"))
	must(err)
	var sawDocument, sawLink bool
	for _, line := range bytes.Split(calls, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var call struct{ Args []string }
		must(json.Unmarshal(line, &call))
		if len(call.Args) == 1 {
			got, e := os.Stat(call.Args[0])
			want, we := os.Stat(documentFile)
			sawDocument = sawDocument || (e == nil && we == nil && os.SameFile(got, want))
		}
		if len(call.Args) == 2 && call.Args[0] == "--" && call.Args[1] == link {
			sawLink = true
		}
	}
	if !sawDocument || !sawLink {
		t.Fatalf("shell argument delivery: document=%t link=%t", sawDocument, sawLink)
	}
	must(writeString(`Software\Classes\`+reassigned+`\shell\open\command`, "", `"foreign.exe" "%1"`))
	must(writeString(`Software\Classes\`+meta.TakenScheme+`\shell\open\command`, "", `"foreign.exe" "%1"`))
	app := exec.Command(filepath.Join(installed, meta.Exe), "--stay")
	must(app.Start())
	defer app.Process.Kill()
	exited := make(chan error, 1)
	go func() { exited <- app.Wait() }()
	install("v2", false)
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("upgrade kept the old app running")
	}
	version, err := os.ReadFile(filepath.Join(installed, "version.txt"))
	must(err)
	if strings.TrimSpace(string(version)) != "v2" {
		t.Fatal("upgrade did not replace app payload")
	}
	if readString(uninstallKey, "InstallLocation") != installed {
		t.Fatal("upgrade moved custom install")
	}
	if exists(`Software\Classes\` + meta.OldScheme) {
		t.Fatal("upgrade kept dropped scheme")
	}
	if _, _, err := readValue(`Software\Classes\.`+meta.OldExtension+`\OpenWithProgids`, document); err == nil {
		t.Fatal("upgrade kept dropped extension")
	}
	foreign := func() {
		t.Helper()
		for _, key := range []string{reassigned, meta.TakenScheme} {
			if readString(`Software\Classes\`+key+`\shell\open\command`, "") != `"foreign.exe" "%1"` {
				t.Fatal("reassigned registration was overwritten")
			}
		}
	}
	foreign()
	temporary := filepath.Join(root, "uninstall-temp")
	must(os.MkdirAll(temporary, 0700))
	uninstall := func() {
		t.Helper()
		command := exec.CommandContext(t.Context(), filepath.Join(installed, "Uninstall.exe"), "/S")
		command.Env = append(os.Environ(), "TMP="+temporary, "TEMP="+temporary)
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("uninstall: %v %s", err, out)
		}
		wait("installed uninstaller cleanup", func() bool { _, err := os.Stat(filepath.Join(installed, "Uninstall.exe")); return os.IsNotExist(err) })
		wait("temporary uninstaller cleanup", func() bool { entries, err := os.ReadDir(temporary); return err == nil && len(entries) == 0 })
	}
	uninstall()
	foreign()
	if exists(uninstallKey) || exists(`Software\Classes\`+document) || exists(`Software\Classes\`+meta.Scheme) {
		t.Fatal("uninstall kept owned registry entries")
	}
	if _, err = os.Stat(sentinel); err != nil {
		t.Fatal("uninstall removed user data")
	}
	if _, err = os.Stat(shortcutFile); !os.IsNotExist(err) {
		t.Fatal("uninstall kept shortcut")
	}
	pathAfter, kind, err := readValue("Environment", "Path")
	if pathErr == nil {
		must(err)
		if kind != originalKind || !bytes.Equal(pathAfter, originalPath) {
			t.Fatalf("PATH restoration differs: before=%d bytes after=%d bytes", len(originalPath), len(pathAfter))
		}
	}
	// A preexisting PATH entry belongs to the user, even across upgrades.
	_, err = updatePath(installed, false)
	must(err)
	preseed, preseedKind, err := readValue("Environment", "Path")
	must(err)
	install("v2", true)
	if readString(uninstallKey, pathOwnedValue) != "" {
		t.Fatal("installer claimed preexisting PATH")
	}
	uninstall()
	after, afterKind, err := readValue("Environment", "Path")
	must(err)
	if afterKind != preseedKind || !bytes.Equal(after, preseed) {
		t.Fatal("uninstaller removed preexisting PATH entry")
	}
	restorePath()
	t.Log("native install, shell delivery, upgrade, process shutdown, guarded registrations, PATH ownership, shortcuts and safe uninstall passed")
}

func nativeShellOpen(t *testing.T, file, class string) {
	t.Helper()
	err := withCOM(func() error {
		type shellExecuteInfo struct {
			Size, Mask                        uint32
			Wnd                               uintptr
			Verb, File, Parameters, Directory *uint16
			Show                              int32
			Instance, IDList                  uintptr
			Class                             *uint16
			Key                               uintptr
			HotKey                            uint32
			Icon, Process                     uintptr
		}
		info := shellExecuteInfo{Mask: 0x540, File: wide(file), Verb: wide("open")}
		if class != "" {
			info.Mask |= 1
			info.Class = wide(class)
		}
		info.Size = uint32(unsafe.Sizeof(info))
		var pins runtime.Pinner
		pins.Pin(info.File)
		pins.Pin(info.Verb)
		if info.Class != nil {
			pins.Pin(info.Class)
		}
		defer pins.Unpin()
		ok, _, err := shell32.NewProc("ShellExecuteExW").Call(ptr(&info))
		if ok == 0 {
			return err
		}
		if info.Process == 0 {
			return fmt.Errorf("ShellExecuteEx returned no app process")
		}
		defer syscall.CloseHandle(syscall.Handle(info.Process))
		result, _, err := kernel32.NewProc("WaitForSingleObject").Call(info.Process, 10000)
		if result != 0 {
			kernel32.NewProc("TerminateProcess").Call(info.Process, 1)
			return fmt.Errorf("shell app wait: %d %v", result, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Cleanup must work after success and cancellation, including an empty selected directory, without interpreting punctuation in paths as shell source.
func TestUninstallerCleanup(t *testing.T) {
	for _, mode := range []string{"cancelled", "completed", "empty"} {
		t.Run(mode, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "日本語  & ! % cleanup")
			helperDir := filepath.Join(root, "helper")
			installedDir := filepath.Join(root, "installed")
			if err := os.MkdirAll(helperDir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(installedDir, 0700); err != nil {
				t.Fatal(err)
			}
			helper := filepath.Join(helperDir, "Uninstall.exe")
			original := filepath.Join(installedDir, "Uninstall.exe")
			for _, file := range []string{helper, original} {
				if err := os.WriteFile(file, []byte("owned cleanup probe"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			sentinel := filepath.Join(installedDir, "keep.txt")
			if mode != "empty" {
				if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			installed := original
			if mode == "cancelled" {
				installed = ""
			}
			removeTemporaryUninstaller(helper, installed, "dev.nsgo.test.cleanup."+mode)
			deadline := time.Now().Add(10 * time.Second)
			for {
				_, err := os.Stat(helperDir)
				if os.IsNotExist(err) {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("temporary helper directory remained")
				}
				time.Sleep(50 * time.Millisecond)
			}
			if mode == "cancelled" {
				if _, err := os.Stat(original); err != nil {
					t.Fatal("cancelled uninstall removed the installed program")
				}
			}
			if mode == "completed" {
				if _, err := os.Stat(original); !os.IsNotExist(err) {
					t.Fatal("completed uninstall kept its program")
				}
			}
			if mode == "empty" {
				if _, err := os.Stat(installedDir); !os.IsNotExist(err) {
					t.Fatal("empty installation directory remained")
				}
			} else {
				if data, err := os.ReadFile(sentinel); err != nil || string(data) != "keep" {
					t.Fatal("cleanup removed user data")
				}
			}
		})
	}
}
