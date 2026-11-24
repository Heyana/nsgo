package windowsinstaller

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var advapi = loadLibrary("advapi32.dll")
var regOpen = advapi.NewProc("RegOpenKeyExW")
var regCreate = advapi.NewProc("RegCreateKeyExW")
var regClose = advapi.NewProc("RegCloseKey")
var regQuery = advapi.NewProc("RegQueryValueExW")
var regSet = advapi.NewProc("RegSetValueExW")
var regDeleteValue = advapi.NewProc("RegDeleteValueW")
var regDeleteTree = advapi.NewProc("RegDeleteTreeW")
var regDeleteKey = advapi.NewProc("RegDeleteKeyW")
var regEnumValue = advapi.NewProc("RegEnumValueW")
var regEnumKey = advapi.NewProc("RegEnumKeyExW")

const currentUser = uintptr(0x80000001)
const keyAccess = 0x2001f // query, set, enumerate, notify, create; per-user only

func wide(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func nativeError(result uintptr) error {
	if result != 0 {
		return syscall.Errno(result)
	}
	return nil
}

func openKey(name string, create bool) (uintptr, error) {
	var key uintptr
	var result uintptr
	if create {
		result, _, _ = regCreate.Call(currentUser, ptr(wide(name)), 0, 0, 0, keyAccess, 0, ptr(&key), 0)
	} else {
		result, _, _ = regOpen.Call(currentUser, ptr(wide(name)), 0, keyAccess, ptr(&key))
	}
	return key, nativeError(result)
}
func closeKey(key uintptr) { regClose.Call(key) }
func readValue(key, name string) ([]byte, uint32, error) {
	h, err := openKey(key, false)
	if err != nil {
		return nil, 0, err
	}
	defer closeKey(h)
	var kind, size uint32
	result, _, _ := regQuery.Call(h, ptr(wide(name)), 0, ptr(&kind), 0, ptr(&size))
	if result != 0 {
		return nil, kind, nativeError(result)
	}
	for {
		data := make([]byte, size)
		var address unsafe.Pointer
		if len(data) > 0 {
			address = ptr(&data[0])
		}
		result, _, _ = regQuery.Call(h, ptr(wide(name)), 0, ptr(&kind), address, ptr(&size))
		if result == 234 {
			continue
		} // another writer enlarged the value
		if result != 0 {
			return nil, kind, nativeError(result)
		}
		return data[:size], kind, nil
	}
}
func readString(key, name string) string {
	data, kind, err := readValue(key, name)
	if err != nil || (kind != 1 && kind != 2) || len(data)%2 != 0 {
		return ""
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[i*2:])
	}
	return syscall.UTF16ToString(units)
}
func writeValue(key, name string, kind uint32, data []byte) error {
	h, err := openKey(key, true)
	if err != nil {
		return err
	}
	defer closeKey(h)
	var address unsafe.Pointer
	if len(data) > 0 {
		address = ptr(&data[0])
	}
	result, _, _ := regSet.Call(h, ptr(wide(name)), 0, uintptr(kind), address, uintptr(len(data)))
	return nativeError(result)
}
func writeString(key, name, value string) error { return writeText(key, name, value, 1) }
func writeText(key, name, value string, kind uint32) error {
	units, err := syscall.UTF16FromString(value)
	if err != nil {
		return err
	}
	data := make([]byte, len(units)*2)
	for i, n := range units {
		binary.LittleEndian.PutUint16(data[i*2:], n)
	}
	return writeValue(key, name, kind, data)
}
func readDWORD(key, name string) uint32 {
	data, kind, err := readValue(key, name)
	if err != nil || kind != 4 || len(data) != 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(data)
}
func writeDWORD(key, name string, value uint32) error {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, value)
	return writeValue(key, name, 4, data)
}
func deleteValue(key, name string) error {
	h, err := openKey(key, false)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return err
	}
	defer closeKey(h)
	result, _, _ := regDeleteValue.Call(h, ptr(wide(name)))
	if result == 2 {
		return nil
	}
	return nativeError(result)
}
func deleteTree(key string) error {
	result, _, _ := regDeleteTree.Call(currentUser, ptr(wide(key)))
	if result == 2 {
		return nil
	}
	return nativeError(result)
}
func keyNames(key string, values bool) ([]string, error) {
	h, err := openKey(key, false)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closeKey(h)
	var names []string
	for index := uint32(0); ; index++ {
		name := make([]uint16, 32768)
		size := uint32(len(name))
		var result uintptr
		if values {
			result, _, _ = regEnumValue.Call(h, uintptr(index), ptr(&name[0]), ptr(&size), 0, 0, 0, 0)
		} else {
			result, _, _ = regEnumKey.Call(h, uintptr(index), ptr(&name[0]), ptr(&size), 0, 0, 0, 0)
		}
		if result == 259 {
			return names, nil
		}
		if result != 0 {
			return nil, nativeError(result)
		}
		names = append(names, syscall.UTF16ToString(name[:size]))
	}
}
func deleteEmptyKey(key string) {
	children, e1 := keyNames(key, false)
	values, e2 := keyNames(key, true)
	if e1 == nil && e2 == nil && len(children) == 0 && len(values) == 0 {
		regDeleteKey.Call(currentUser, ptr(wide(key)))
	}
}

// removeDeclarations reads the earlier installation's metadata, including NSIS metadata. A changed open command belongs to another application and stays.
func removeDeclarations(m Manifest) error {
	meta := m.UninstallKey()
	types, err := keyNames(meta+`\FileTypes`, false)
	if err != nil {
		return err
	}
	for _, typ := range types {
		record := meta + `\FileTypes\` + typ
		class := `Software\Classes\` + typ
		expected := readString(record, "")
		if expected != "" && strings.EqualFold(expected, readString(class+`\shell\open\command`, "")) {
			extensions, err := keyNames(record, true)
			if err != nil {
				return err
			}
			for _, extension := range extensions {
				if extension == "" {
					continue
				}
				key := `Software\Classes\.` + extension
				if err := deleteValue(key+`\OpenWithProgids`, typ); err != nil {
					return err
				}
				deleteEmptyKey(key + `\OpenWithProgids`)
				deleteEmptyKey(key)
			}
			if err := deleteTree(class); err != nil {
				return err
			}
		}
		if err := deleteTree(record); err != nil {
			return err
		}
	}
	deleteEmptyKey(meta + `\FileTypes`)
	schemes, err := keyNames(meta+`\URLSchemes`, true)
	if err != nil {
		return err
	}
	for _, scheme := range schemes {
		key := `Software\Classes\` + scheme
		expected := readString(meta+`\URLSchemes`, scheme)
		if expected != "" && strings.EqualFold(expected, readString(key+`\shell\open\command`, "")) {
			if err := deleteTree(key); err != nil {
				return err
			}
		}
		if err := deleteValue(meta+`\URLSchemes`, scheme); err != nil {
			return err
		}
	}
	deleteEmptyKey(meta + `\URLSchemes`)
	shell32.NewProc("SHChangeNotify").Call(0x08000000, 0, 0, 0)
	return nil
}

// A class left after ownership cleanup was changed by somebody else. An upgrade must not immediately reclaim the same key it just chose to preserve.
func foreignDeclaration(key, command string) (bool, error) {
	handle, err := openKey(key, false)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	closeKey(handle)
	return !strings.EqualFold(readString(key+`\shell\open\command`, ""), command), nil
}
func registerDeclarations(m Manifest, dir string) error {
	if err := removeDeclarations(m); err != nil {
		return err
	}
	executable := filepath.Join(dir, m.Exe)
	command := `"` + executable + `" "%1"`
	for _, typ := range m.FileTypes {
		key := `Software\Classes\` + typ.ProgID
		foreign, err := foreignDeclaration(key, command)
		if err != nil {
			return err
		}
		if foreign {
			continue
		}
		record := m.UninstallKey() + `\FileTypes\` + typ.ProgID
		for _, entry := range []struct{ key, name, value string }{{key, "", typ.Name}, {key + `\DefaultIcon`, "", `"` + executable + `",0`}, {key + `\shell\open\command`, "", command}, {record, "", command}} {
			if err := writeString(entry.key, entry.name, entry.value); err != nil {
				return err
			}
		}
		for _, extension := range typ.Extensions {
			if err := writeString(`Software\Classes\.`+extension+`\OpenWithProgids`, typ.ProgID, ""); err != nil {
				return err
			}
			if err := writeString(record, extension, ""); err != nil {
				return err
			}
		}
	}
	command = `"` + executable + `" -- "%1"`
	for _, scheme := range m.URLSchemes {
		key := `Software\Classes\` + scheme
		foreign, err := foreignDeclaration(key, command)
		if err != nil {
			return err
		}
		if foreign {
			continue
		}
		for _, entry := range []struct{ key, name, value string }{{key, "", "URL:" + m.Name}, {key, "URL Protocol", ""}, {key + `\DefaultIcon`, "", `"` + executable + `",0`}, {key + `\shell\open\command`, "", command}, {m.UninstallKey() + `\URLSchemes`, scheme, command}} {
			if err := writeString(entry.key, entry.name, entry.value); err != nil {
				return err
			}
		}
	}
	shell32.NewProc("SHChangeNotify").Call(0x08000000, 0, 0, 0)
	return nil
}

func updatePath(dir string, remove bool) (bool, error) {
	const key = `Environment`
	value := readString(key, "Path")
	_, kind, err := readValue(key, "Path")
	if err != nil && !errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return false, err
	}
	if err == nil && kind != 1 && kind != 2 {
		return false, errors.New("the user Path is not a string registry value")
	}
	if err != nil {
		kind = 2
	}
	parts := strings.Split(value, ";")
	found := false
	kept := parts[:0]
	for _, part := range parts {
		expanded := expandEnvironment(strings.Trim(strings.TrimSpace(part), `"`))
		match := strings.EqualFold(filepath.Clean(expanded), filepath.Clean(dir))
		found = found || match
		if !(remove && match) {
			kept = append(kept, part)
		}
	}
	if remove && !found || !remove && found {
		return false, nil
	}
	if !remove {
		if value == "" {
			kept = nil
		}
		kept = append(kept, dir)
	}
	if err := writeText(key, "Path", strings.Join(kept, ";"), kind); err != nil {
		return false, err
	}
	var result uintptr
	user32.NewProc("SendMessageTimeoutW").Call(0xffff, 0x001a, 0, ptr(wide("Environment")), 2, 2000, ptr(&result))
	return true, nil
}
func expandEnvironment(value string) string {
	proc := kernel32.NewProc("ExpandEnvironmentStringsW")
	n, _, _ := proc.Call(ptr(wide(value)), 0, 0)
	if n == 0 {
		return value
	}
	data := make([]uint16, n)
	if ok, _, _ := proc.Call(ptr(wide(value)), ptr(&data[0]), n); ok == 0 {
		return value
	}
	return syscall.UTF16ToString(data)
}

// pathOwnedValue is the uninstall entry's value naming the folder an installer put on the user's Path, as NSIS's installers recorded it, so the uninstaller or a later installation takes that folder alone off Path.
const pathOwnedValue = "NsgoPathAddedDir"

func sameDirectory(a, b string) bool {
	return a != "" && b != "" && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func registerUninstall(m Manifest, dir, pathOwned string) error {
	key := m.UninstallKey()
	uninstaller := `"` + filepath.Join(dir, "Uninstall.exe") + `"`
	values := map[string]string{"DisplayName": m.Name, "DisplayVersion": m.Version, "DisplayIcon": filepath.Join(dir, m.Exe), "InstallLocation": dir, "UninstallString": uninstaller, "QuietUninstallString": uninstaller + " /S"}
	if m.Publisher != "" {
		values["Publisher"] = m.Publisher
	}
	for name, value := range values {
		if err := writeString(key, name, value); err != nil {
			return err
		}
	}
	if pathOwned != "" {
		if err := writeString(key, pathOwnedValue, pathOwned); err != nil {
			return err
		}
	} else if err := deleteValue(key, pathOwnedValue); err != nil {
		return err
	}
	for _, name := range []string{"NoModify", "NoRepair"} {
		if err := writeDWORD(key, name, 1); err != nil {
			return err
		}
	}
	return nil
}
func installationDirectory(m Manifest, override string) (string, error) {
	dir := override
	if dir == "" {
		dir = readString(m.UninstallKey(), "InstallLocation")
	}
	if dir == "" {
		dir = filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", m.Name)
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if filepath.Dir(dir) == dir {
		return "", fmt.Errorf("cannot install into a volume root")
	}
	return dir, nil
}
