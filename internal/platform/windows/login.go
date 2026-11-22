//go:build windows

package windows

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	// loginRunKey holds the commands Explorer runs as the user signs in, one value an app.
	loginRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
	// loginApprovedKey holds the switches of Task Manager's Startup apps and Settings' Startup page, a binary value named as the Run value, whose first byte is odd while the user has the app turned off.
	loginApprovedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
)

var regDeleteValueW = advapi32.NewProc("RegDeleteValueW")

// errLoginPackaged keeps Run-key helpers out of an MSIX's virtualized registry;
// the window routes packaged requests through StartupTask instead.
var errLoginPackaged = errors.New("MSIX login registration uses StartupTask")

func registryError(result uintptr) error {
	if result == 0 {
		return nil
	}
	return syscall.Errno(result)
}

// packaged reports whether the app runs from an MSIX package.
func packaged() bool {
	var n uint32
	// APPMODEL_ERROR_NO_PACKAGE: the app is no package's.
	r, _, _ := getCurrentPackageFullName.Call(uintptr(unsafe.Pointer(&n)), 0)
	return r != 15700
}

// loginValueName gives the Run value a stable name even for an app built without an ID.
func loginValueName(id, executable string) string {
	if id != "" {
		return id
	}
	name := strings.TrimSuffix(filepath.Base(executable), filepath.Ext(executable))
	if name == "" || name == "." || name == `\` {
		return "NSGo"
	}
	return name
}

// windowsCommandArg quotes one argument using CommandLineToArgvW's rules.
func windowsCommandArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\v\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, r := range s {
		switch r {
		case '\\':
			slashes++
		case '"':
			b.WriteString(strings.Repeat("\\", slashes*2+1))
			b.WriteRune(r)
			slashes = 0
		default:
			b.WriteString(strings.Repeat("\\", slashes))
			b.WriteRune(r)
			slashes = 0
		}
	}
	b.WriteString(strings.Repeat("\\", slashes*2))
	b.WriteByte('"')
	return b.String()
}

func loginCommand(executable string) string {
	return windowsCommandArg(executable) + " --nsgo-launched-at-login"
}

// loginTurnedOff reports whether a StartupApproved value says the user turned the app off in Startup apps: 02 or 06 starts it, 03 or 07 does not, and a missing value starts it.
func loginTurnedOff(approved []byte) bool {
	return len(approved) > 0 && approved[0]&1 != 0
}

func setLaunchAtLogin(on bool, id string) {
	if err := setLoginEntry(on, id); err != nil {
		log.Printf("nsgo: launch at login: %v", err)
	}
}

func setLoginEntry(on bool, id string) error {
	if packaged() {
		return errLoginPackaged
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	name := loginValueName(id, executable)
	if on {
		var key syscall.Handle
		result, _, _ := regCreateKeyExW.Call(uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(wide(loginRunKey))), 0, 0, 0, syscall.KEY_SET_VALUE, 0, uintptr(unsafe.Pointer(&key)), 0)
		if err := registryError(result); err != nil {
			return err
		}
		value := utf16.Encode([]rune(loginCommand(executable) + "\x00"))
		result, _, _ = regSetValueExW.Call(uintptr(key), uintptr(unsafe.Pointer(wide(name))), 0, syscall.REG_SZ, uintptr(unsafe.Pointer(&value[0])), uintptr(len(value)*2))
		syscall.RegCloseKey(key)
		if err := registryError(result); err != nil {
			return err
		}
		// The app's own switch is the user's later word: an earlier "off" in Startup apps would keep the entry from running.
		return deleteRegValue(loginApprovedKey, name)
	}
	if err := deleteRegValue(loginRunKey, name); err != nil {
		return err
	}
	return deleteRegValue(loginApprovedKey, name)
}

// deleteRegValue deletes value name under HKCU's path; a missing one is no error.
func deleteRegValue(path, name string) error {
	var key syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, wide(path), 0, syscall.KEY_SET_VALUE, &key); err != nil {
		if err == syscall.ERROR_FILE_NOT_FOUND {
			return nil
		}
		return err
	}
	defer syscall.RegCloseKey(key)
	result, _, _ := regDeleteValueW.Call(uintptr(key), uintptr(unsafe.Pointer(wide(name))))
	if err := registryError(result); err != nil && err != syscall.ERROR_FILE_NOT_FOUND {
		return err
	}
	return nil
}

// regValue reads value name under HKCU's path and its type.
func regValue(path, name string) ([]byte, uint32, bool) {
	var key syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, wide(path), 0, syscall.KEY_READ, &key) != nil {
		return nil, 0, false
	}
	defer syscall.RegCloseKey(key)
	var kind, size uint32
	if syscall.RegQueryValueEx(key, wide(name), nil, &kind, nil, &size) != nil {
		return nil, 0, false
	}
	data := make([]byte, size)
	if size > 0 && syscall.RegQueryValueEx(key, wide(name), nil, &kind, &data[0], &size) != nil {
		return nil, 0, false
	}
	return data[:size], kind, true
}

func launchesAtLogin(id string) bool {
	if packaged() {
		return false
	}
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	name := loginValueName(id, executable)
	data, kind, ok := regValue(loginRunKey, name)
	if !ok || kind != syscall.REG_SZ || len(data) < 2 {
		return false
	}
	command := syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&data[0])), len(data)/2))
	if command != loginCommand(executable) {
		return false
	}
	approved, _, _ := regValue(loginApprovedKey, name)
	return !loginTurnedOff(approved)
}
