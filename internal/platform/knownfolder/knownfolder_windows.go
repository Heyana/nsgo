package knownfolder

import (
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

var (
	procSHGetKnownFolderPath = syscall.NewLazyDLL("shell32.dll").NewProc("SHGetKnownFolderPath")
	procCoTaskMemFree        = syscall.NewLazyDLL("ole32.dll").NewProc("CoTaskMemFree")
)

func path(id string) string {
	folder, ok := parseGUID(id)
	if !ok {
		return ""
	}
	var p *uint16
	hr, _, _ := procSHGetKnownFolderPath.Call(uintptr(unsafe.Pointer(&folder)), 0, 0, uintptr(unsafe.Pointer(&p)))
	if p != nil {
		defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(p)))
	}
	if int32(hr) < 0 || p == nil {
		return ""
	}
	n := 0
	for *(*uint16)(unsafe.Add(unsafe.Pointer(p), 2*n)) != 0 {
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice(p, n))
}

// parseGUID parses "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx".
func parseGUID(s string) (g syscall.GUID, ok bool) {
	hex := strings.ReplaceAll(s, "-", "")
	if len(hex) != 32 || len(s) != 36 {
		return g, false
	}
	word := func(from, to int) uint64 {
		n, err := strconv.ParseUint(hex[from:to], 16, 64)
		if err != nil {
			ok = false
		}
		return n
	}
	ok = true
	g = syscall.GUID{Data1: uint32(word(0, 8)), Data2: uint16(word(8, 12)), Data3: uint16(word(12, 16))}
	for i := range g.Data4 {
		g.Data4[i] = byte(word(16+2*i, 18+2*i))
	}
	return g, ok
}
