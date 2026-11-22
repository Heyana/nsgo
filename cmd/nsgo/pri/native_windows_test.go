package pri

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsShellLoadsGeneratedIndex(t *testing.T) {
	dir := t.TempDir()
	blob, err := Write("NSGoFixture", "en-US", fixtureResources())
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "resources.pri")
	if err := os.WriteFile(file, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	source, _ := syscall.UTF16PtrFromString("@{" + file + "?ms-resource://NSGoFixture/Resources/AppDisplayName}")
	result := make([]uint16, 512)
	hr, _, _ := syscall.NewLazyDLL("shlwapi.dll").NewProc("SHLoadIndirectString").Call(uintptr(unsafe.Pointer(source)), uintptr(unsafe.Pointer(&result[0])), uintptr(len(result)), 0)
	if int32(hr) < 0 {
		t.Fatalf("SHLoadIndirectString: HRESULT=%08x", uint32(hr))
	}
	if got := syscall.UTF16ToString(result); got != "Fixture App" && got != "测试应用 🧪" {
		t.Fatalf("shell selected invalid candidate %q", got)
	}
}

func TestWindowsSDKParsesAllLanguageAndScaleCandidates(t *testing.T) {
	paths, _ := filepath.Glob(`C:\Program Files (x86)\Windows Kits\10\bin\*\x64\makepri.exe`)
	if len(paths) == 0 {
		t.Skip("Windows SDK makepri is not installed; MRT runtime test remains mandatory")
	}
	dir := t.TempDir()
	blob, err := Write("NSGoFixture", "en-US", fixtureResources())
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "resources.pri")
	if err := os.WriteFile(file, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	dump := filepath.Join(dir, "dump.xml")
	cmd := exec.CommandContext(t.Context(), paths[len(paths)-1], "dump", "/if", file, "/of", dump, "/dt", "detailed", "/o")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("makepri: %v\n%s", err, out)
	}
	data, err := os.ReadFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"Fixture App", "测试应用 🧪", `Logo.scale-100.png`, `Logo.scale-200.png`, `Logo.scale-400.png`, `name="Language"`, `name="Scale"`} {
		if !strings.Contains(string(data), value) {
			t.Errorf("native dump missing %q", value)
		}
	}
}

// Explicit contexts do not follow Shell's language preferences and do not change system settings. Set NSGO_MRM_DLL to a Microsoft Windows App Runtime MRM.dll.
func TestWindowsMRMSelectsLanguageAndScale(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	path := os.Getenv("NSGO_MRM_DLL")
	if path == "" {
		t.Skip("set NSGO_MRM_DLL for explicit MRT Core context acceptance")
	}
	handle, err := windows.LoadLibraryEx(path, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_DEFAULT_DIRS)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.FreeLibrary(handle)
	dll := &syscall.DLL{Name: path, Handle: syscall.Handle(handle)}
	proc := func(name string) *syscall.Proc {
		p, err := dll.FindProc(name)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	check := func(name string, hr uintptr) {
		t.Helper()
		if int32(hr) < 0 {
			t.Fatalf("%s HRESULT=%08x", name, uint32(hr))
		}
	}
	hr, _, _ := syscall.NewLazyDLL("ole32.dll").NewProc("CoInitializeEx").Call(0, 0)
	check("CoInitializeEx", hr)
	defer syscall.NewLazyDLL("ole32.dll").NewProc("CoUninitialize").Call()
	text := func(value string) *uint16 {
		p, err := syscall.UTF16PtrFromString(value)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	dir := t.TempDir()
	blob, err := Write("NSGoFixture", "en-US", fixtureResources())
	if err != nil {
		t.Fatal(err)
	}
	if input := os.Getenv("NSGO_PRI_NATIVE_FILE"); input != "" {
		blob, err = os.ReadFile(input)
		if err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(dir, "resources.pri")
	if err := os.WriteFile(file, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	var manager, context uintptr
	hr, _, _ = proc("MrmCreateResourceManager").Call(uintptr(unsafe.Pointer(text(file))), uintptr(unsafe.Pointer(&manager)))
	check("MrmCreateResourceManager", hr)
	defer proc("MrmDestroyResourceManager").Call(manager)
	hr, _, _ = proc("MrmCreateResourceContext").Call(manager, uintptr(unsafe.Pointer(&context)))
	check("MrmCreateResourceContext", hr)
	defer proc("MrmDestroyResourceContext").Call(context)
	load := func(id string) string {
		t.Helper()
		var value *uint16
		hr, _, _ := proc("MrmLoadStringResource").Call(manager, context, 0, uintptr(unsafe.Pointer(text(id))), uintptr(unsafe.Pointer(&value)))
		check("MrmLoadStringResource", hr)
		if value == nil {
			t.Fatal("MRT returned nil text")
		}
		defer proc("MrmFreeResource").Call(uintptr(unsafe.Pointer(value)))
		return windows.UTF16PtrToString(value)
	}
	set := func(key, value string) {
		t.Helper()
		hr, _, _ := proc("MrmSetQualifier").Call(context, uintptr(unsafe.Pointer(text(key))), uintptr(unsafe.Pointer(text(value))))
		check("MrmSetQualifier", hr)
	}
	for _, v := range []struct{ lang, want string }{{"en-US", "Fixture App"}, {"zh-CN", "测试应用 🧪"}, {"ja-JP", "Fixture App"}} {
		set("Language", v.lang)
		if got := load("Resources/AppDisplayName"); got != v.want {
			t.Errorf("language %s resolved %q want %q", v.lang, got, v.want)
		}
	}
	logos := []string{"Logo"}
	scales := []string{"100", "200", "400"}
	if os.Getenv("NSGO_PRI_NATIVE_FILE") != "" {
		logos = []string{"Square150x150Logo", "Square44x44Logo", "StoreLogo"}
		scales = []string{"100", "125", "150", "200", "400"}
	}
	for _, logo := range logos {
		for _, scale := range scales {
			set("Scale", scale)
			want := `Assets\` + logo + `.scale-` + scale + `.png`
			if got := load("Files/Assets/" + logo + ".png"); !strings.HasSuffix(got, want) {
				t.Errorf("scale %s resolved %q want suffix %q", scale, got, want)
			}
		}
	}
}
