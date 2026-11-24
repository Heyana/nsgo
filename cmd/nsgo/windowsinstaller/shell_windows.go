package windowsinstaller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var kernel32 = loadLibrary("kernel32.dll")
var user32 = loadLibrary("user32.dll")
var shell32 = loadLibrary("shell32.dll")
var ole32 = loadLibrary("ole32.dll")

type guid struct {
	A    uint32
	B, C uint16
	D    [8]byte
}

var shellLinkClass = guid{0x00021401, 0, 0, [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
var shellLinkInterface = guid{0x000214f9, 0, 0, [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
var persistFileInterface = guid{0x0000010b, 0, 0, [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}

// COM returns native object pointers through CoCreateInstance's out parameter. Keeping them typed avoids converting native integer addresses into Go pointers.
type comObject struct{ vtable *[21]uintptr }

func comCall(object *comObject, index int, args ...any) uintptr {
	result, _, _ := callAddress(object.vtable[index], append([]any{ptr(object)}, args...)...)
	return result
}

func hresult(value uintptr) error {
	if int32(value) < 0 {
		return fmt.Errorf("Windows returned HRESULT %#x", uint32(value))
	}
	return nil
}
func withCOM(fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 2)
	if err := hresult(hr); err != nil {
		return err
	}
	defer ole32.NewProc("CoUninitialize").Call()
	return fn()
}
func newShellLink() (*comObject, *comObject, error) {
	var link, persist *comObject
	hr, _, _ := ole32.NewProc("CoCreateInstance").Call(ptr(&shellLinkClass), 0, 1, ptr(&shellLinkInterface), ptr(&link))
	if err := hresult(hr); err != nil {
		return nil, nil, err
	}
	if err := hresult(comCall(link, 0, ptr(&persistFileInterface), ptr(&persist))); err != nil {
		comCall(link, 2)
		return nil, nil, err
	}
	return link, persist, nil
}
func shortcut(file, target string, remove bool) error {
	link, persist, err := newShellLink()
	if err != nil {
		return err
	}
	defer comCall(link, 2)
	defer comCall(persist, 2)
	if remove {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			return nil
		}
		if err := hresult(comCall(persist, 5, ptr(wide(file)), 0)); err != nil {
			return err
		}
		var found [32768]uint16
		if err := hresult(comCall(link, 3, ptr(&found[0]), uintptr(len(found)), 0, 4)); err != nil {
			return err
		}
		if strings.EqualFold(filepath.Clean(syscall.UTF16ToString(found[:])), filepath.Clean(target)) {
			return os.Remove(file)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}
	if err := hresult(comCall(link, 20, ptr(wide(target)))); err != nil {
		return err
	}
	if err := hresult(comCall(link, 9, ptr(wide(filepath.Dir(target))))); err != nil {
		return err
	}
	if err := hresult(comCall(link, 17, ptr(wide(target)), 0)); err != nil {
		return err
	}
	return hresult(comCall(persist, 6, ptr(wide(file)), 1))
}
func shellFolder(id uintptr) (string, error) {
	var value [260]uint16
	hr, _, _ := shell32.NewProc("SHGetFolderPathW").Call(0, id, 0, 0, ptr(&value[0]))
	if err := hresult(hr); err != nil {
		return "", err
	}
	return syscall.UTF16ToString(value[:]), nil
}
func shortcuts(m Manifest, dir string, desktop, remove bool) error {
	target := filepath.Join(dir, m.Exe)
	start, err := shellFolder(2)
	if err != nil {
		return err
	}
	start = filepath.Join(start, m.Name+".lnk")
	if err := shortcut(start, target, remove); err != nil {
		return err
	}
	if desktop || remove {
		folder, err := shellFolder(0x10)
		if err != nil {
			return err
		}
		if err := shortcut(filepath.Join(folder, m.Name+".lnk"), target, remove); err != nil {
			return err
		}
	}
	flag := "--nsgo-register-toast=" + start
	if remove {
		flag = "--nsgo-unregister-toast"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, target, flag)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	_ = command.Run() // Older apps may have no toast registration command.
	return nil
}

type processEntry struct {
	Size, Usage, PID        uint32
	Heap                    uintptr
	Module, Threads, Parent uint32
	Priority                int32
	Flags                   uint32
	Name                    [260]uint16
}

func installedProcesses(dir string) ([]uint32, error) {
	snapshot, _, err := kernel32.NewProc("CreateToolhelp32Snapshot").Call(2, 0)
	if snapshot == ^uintptr(0) {
		return nil, err
	}
	defer syscall.CloseHandle(syscall.Handle(snapshot))
	entry := processEntry{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	next := kernel32.NewProc("Process32NextW")
	ok, _, _ := kernel32.NewProc("Process32FirstW").Call(snapshot, ptr(&entry))
	var pids []uint32
	for ok != 0 {
		if entry.PID != uint32(os.Getpid()) {
			handle, _, _ := kernel32.NewProc("OpenProcess").Call(0x1000, 0, uintptr(entry.PID))
			if handle != 0 {
				if imageInDirectory(handle, dir) {
					pids = append(pids, entry.PID)
				}
				syscall.CloseHandle(syscall.Handle(handle))
			}
		}
		ok, _, _ = next.Call(snapshot, ptr(&entry))
	}
	return pids, nil
}
func imageInDirectory(handle uintptr, dir string) bool {
	data := make([]uint16, 32768)
	size := uint32(len(data))
	ok, _, _ := kernel32.NewProc("QueryFullProcessImageNameW").Call(handle, 0, ptr(&data[0]), ptr(&size))
	if ok == 0 {
		return false
	}
	relative, err := filepath.Rel(dir, syscall.UTF16ToString(data[:size]))
	// The temporary uninstaller must leave its waiting original alive.
	return err == nil && filepath.IsLocal(relative) && !strings.EqualFold(relative, "Uninstall.exe")
}
func closeInstalledProcesses(pids []uint32, dir string) error {
	for _, pid := range pids {
		handle, _, err := kernel32.NewProc("OpenProcess").Call(0x101001, 0, uintptr(pid)) // terminate and wait
		if handle == 0 {
			if err == syscall.Errno(87) {
				continue
			}
			return err
		}
		// Recheck through the handle we will terminate: the app may have exited and Windows reused its PID while the user answered a prompt.
		if !imageInDirectory(handle, dir) {
			syscall.CloseHandle(syscall.Handle(handle))
			continue
		}
		ok, _, err := kernel32.NewProc("TerminateProcess").Call(handle, 0)
		if ok != 0 {
			result, _, _ := kernel32.NewProc("WaitForSingleObject").Call(handle, 5000)
			if result != 0 {
				ok = 0
				err = fmt.Errorf("the running app did not exit")
			}
		}
		syscall.CloseHandle(syscall.Handle(handle))
		if ok == 0 {
			return err
		}
	}
	return nil
}

// temporaryUninstaller keeps Windows' image lock off the installed files. The original waits for the child's result; a bounded batch then removes the two locked images.
func temporaryUninstaller(self, dir string, silent bool, language uint16) error {
	temp, err := os.MkdirTemp("", "nsgo-uninstall-")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(self)
	if err != nil {
		os.RemoveAll(temp)
		return err
	}
	copy := filepath.Join(temp, "Uninstall.exe")
	if err := os.WriteFile(copy, data, 0700); err != nil {
		os.RemoveAll(temp)
		return err
	}
	command := exec.Command(copy, "--nsgo-uninstall-from="+dir)
	if silent {
		command.Args = append(command.Args, "/S")
	}
	if language != 0 {
		command.Args = append(command.Args, fmt.Sprintf("/LANG=%d", language))
	}
	command.Dir = filepath.Dir(temp) // Do not hold the directory the cleanup must remove.
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	if err := command.Run(); err != nil {
		os.RemoveAll(temp)
		return err
	}
	return nil
}

// All shell text is fixed ASCII. Paths go through the child environment, so Unicode, percent signs, exclamation marks and ampersands never become shell source or depend on a console code page. Only exact files and empty folders go. Separate commands avoid nested, mutually exclusive IF groups in cmd: a false inner group can swallow the following branch inside a FOR body.
const uninstallTemporaryCleanupCommand = `for /L %n in (1,1,30) do @(del /f /q "%NSGO_UNINSTALL_COPY%" >nul 2>&1 & if not exist "%NSGO_UNINSTALL_COPY%" (rmdir "%NSGO_UNINSTALL_TEMP%" >nul 2>&1 & if not exist "%NSGO_UNINSTALL_TEMP%" exit /b) & ping -n 2 127.0.0.1 >nul)`
const uninstallCleanupCommand = `for /L %n in (1,1,30) do @(del /f /q "%NSGO_UNINSTALL_COPY%" >nul 2>&1 & del /f /q "%NSGO_UNINSTALL_ORIGINAL%" >nul 2>&1 & if not exist "%NSGO_UNINSTALL_COPY%" if not exist "%NSGO_UNINSTALL_ORIGINAL%" (rmdir "%NSGO_UNINSTALL_ROOT%" >nul 2>&1 & rmdir "%NSGO_UNINSTALL_TEMP%" >nul 2>&1 & if not exist "%NSGO_UNINSTALL_TEMP%" exit /b) & ping -n 2 127.0.0.1 >nul)`

func cleanupMarker(id string) string {
	sum := sha256.Sum256([]byte(id))
	return fmt.Sprintf("NSGoCleanup-%x", sum[:8])
}
func removeTemporaryUninstaller(self, installed, id string) {
	var directory [32768]uint16
	size, _, _ := kernel32.NewProc("GetSystemDirectoryW").Call(ptr(&directory[0]), uintptr(len(directory)))
	if size == 0 || size >= uintptr(len(directory)) {
		return
	}
	system := syscall.UTF16ToString(directory[:size])
	shell := filepath.Join(system, "cmd.exe")
	command := exec.Command(shell)
	command.Dir = system // Neither the installation nor its temporary copy stays held as cwd.
	root, cleanup := "", uninstallTemporaryCleanupCommand
	if installed != "" {
		root = filepath.Dir(installed)
		cleanup = uninstallCleanupCommand
	}
	command.Env = append(os.Environ(), "NSGO_UNINSTALL_COPY="+self, "NSGO_UNINSTALL_ORIGINAL="+installed, "NSGO_UNINSTALL_ROOT="+root, "NSGO_UNINSTALL_TEMP="+filepath.Dir(self))
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000, CmdLine: fmt.Sprintf(`"%s" /D /V:OFF /C echo %s >nul & %s`, shell, cleanupMarker(id), cleanup)}
	if command.Start() == nil {
		command.Process.Release()
	}
}
