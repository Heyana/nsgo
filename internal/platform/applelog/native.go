//go:build darwin

// Package applelog sends Apple's packaged-app diagnostics to the unified log.
package applelog

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"runtime"
	"structs"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

const publicFormat = "%{public}s\x00"

const (
	defaultType uint8 = 0
	errorType   uint8 = 0x10
)

type dlInfo struct {
	_                           structs.HostLayout
	file, base, symbol, address unsafe.Pointer
}

type systemAPI struct {
	create       func(string, string) uintptr
	emit         func(unsafe.Pointer, uintptr, uint8, *byte, *byte, uint32)
	enabled      func(uintptr, uint8) bool
	dso          unsafe.Pointer
	format       *byte
	objectString bool
	makeString   func(uintptr, *byte, int, uint32, bool) uintptr
	release      func(uintptr)
	bundle       func() uintptr
	bundleID     func(uintptr) uintptr
	stringLength func(uintptr) int
	copyString   func(uintptr, *byte, int, uint32) bool
}

var system = sync.OnceValues(func() (*systemAPI, error) {
	lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, err
	}
	api := new(systemAPI)
	for _, binding := range []struct {
		name string
		fn   any
	}{
		{"os_log_create", &api.create}, {"_os_log_impl", &api.emit}, {"os_log_type_enabled", &api.enabled},
	} {
		address, err := purego.Dlsym(lib, binding.name)
		if err != nil {
			return nil, err
		}
		purego.RegisterFunc(binding.fn, address)
	}
	address, err := purego.Dlsym(lib, "dladdr")
	if err != nil {
		return nil, err
	}
	var locate func(uintptr, *dlInfo) int32
	purego.RegisterFunc(&locate, address)
	foundation, err := purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, err
	}
	logSymbol, err := purego.Dlsym(foundation, "NSLog")
	if err != nil {
		return nil, err
	}
	var info dlInfo
	if locate(logSymbol, &info) == 0 || info.base == nil {
		return nil, fmt.Errorf("Foundation has no Mach-O image")
	}
	format, objectString, err := imageFormat(info.base)
	if err != nil {
		return nil, err
	}
	api.dso, api.format, api.objectString = info.base, format, objectString
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, err
	}
	for _, binding := range []struct {
		name string
		fn   any
	}{
		{"CFStringCreateWithBytes", &api.makeString}, {"CFRelease", &api.release},
		{"CFBundleGetMainBundle", &api.bundle}, {"CFBundleGetIdentifier", &api.bundleID},
		{"CFStringGetLength", &api.stringLength}, {"CFStringGetCString", &api.copyString},
	} {
		address, err := purego.Dlsym(cf, binding.name)
		if err != nil {
			return nil, err
		}
		purego.RegisterFunc(binding.fn, address)
	}
	return api, nil
})

type nativeLog struct {
	api    *systemAPI
	object uintptr
}

func newLog(subsystem, category string) (*nativeLog, error) {
	api, err := system()
	if err != nil {
		return nil, err
	}
	object := api.create(subsystem, category)
	if object == 0 {
		return nil, fmt.Errorf("could not create the unified log")
	}
	return &nativeLog{api, object}, nil
}

func (l *nativeLog) write(kind uint8, text string) {
	if !l.api.enabled(l.object, kind) {
		return
	}
	message := append([]byte(text), 0)
	var pin runtime.Pinner
	pin.Pin(&message[0])
	defer pin.Unpin()
	// Clang's OSLogBufferLayout: non-scalar summary, one argument, a public StringKind descriptor (2<<4 | 2), its pointer's size, then the pointer. An Objective-C formatter takes ObjCObjKind (4) instead. _os_log_impl copies the string during this call, as it does a C stack's.
	buffer := [12]byte{2, 1, 0x22, 8}
	pointer := uintptr(unsafe.Pointer(&message[0]))
	if l.api.objectString {
		pointer = l.api.makeString(0, &message[0], len(message)-1, 0x08000100, false)
		if pointer == 0 {
			return
		}
		defer l.api.release(pointer)
		buffer[2] = 0x42
	}
	binary.LittleEndian.PutUint64(buffer[4:], uint64(pointer))
	l.api.emit(l.api.dso, l.object, kind, l.api.format, &buffer[0], uint32(len(buffer)))
	runtime.KeepAlive(message)
}

// imageFormat finds a canonical formatter in Foundation's registered log strings. Go's own read-only strings have no __oslogstring catalogue, so using one makes Console report a UUID compose failure even while the process lives. The argument descriptor, rather than the formatter's default, marks it public.
func imageFormat(base unsafe.Pointer) (*byte, bool, error) {
	header := unsafe.Slice((*byte)(base), 32)
	if binary.LittleEndian.Uint32(header) != 0xfeedfacf {
		return nil, false, fmt.Errorf("not a 64-bit Mach-O image")
	}
	size := binary.LittleEndian.Uint32(header[20:])
	if size > 1<<20 {
		return nil, false, fmt.Errorf("Mach-O load commands exceed 1 MiB")
	}
	commands := unsafe.Slice((*byte)(unsafe.Add(base, 32)), int(size))
	var text, address, length uint64
	for len(commands) >= 8 {
		size := int(binary.LittleEndian.Uint32(commands[4:]))
		if size < 8 || size > len(commands) {
			break
		}
		cmd := commands[:size]
		if binary.LittleEndian.Uint32(cmd) == 0x19 && len(cmd) >= 72 {
			if string(bytes.TrimRight(cmd[8:24], "\x00")) == "__TEXT" {
				text = binary.LittleEndian.Uint64(cmd[24:])
			}
			segmentAddress := binary.LittleEndian.Uint64(cmd[24:])
			segmentLength := binary.LittleEndian.Uint64(cmd[32:])
			count := int(binary.LittleEndian.Uint32(cmd[64:]))
			if count > (len(cmd)-72)/80 {
				return nil, false, fmt.Errorf("truncated Mach-O sections")
			}
			for sections := cmd[72 : 72+count*80]; len(sections) >= 80; sections = sections[80:] {
				if string(bytes.TrimRight(sections[:16], "\x00")) == "__oslogstring" {
					address, length = binary.LittleEndian.Uint64(sections[32:]), binary.LittleEndian.Uint64(sections[40:])
					if address < segmentAddress || length > segmentLength || address-segmentAddress > segmentLength-length {
						return nil, false, fmt.Errorf("Mach-O log strings exceed their segment")
					}
				}
			}
		}
		commands = commands[size:]
	}
	if text == 0 || address < text || length == 0 || length > 1<<24 {
		return nil, false, fmt.Errorf("Foundation has no unified log strings")
	}
	data := unsafe.Slice((*byte)(unsafe.Add(base, uintptr(address-text))), int(length))
	for _, format := range []string{publicFormat, "%{public}@\x00", "%{public}s\n\x00", "%{public}@\n\x00", "%@\x00", "%s\x00"} {
		for offset := 0; offset < len(data); {
			at := bytes.Index(data[offset:], []byte(format))
			if at < 0 {
				break
			}
			at += offset
			if at == 0 || data[at-1] == 0 {
				return &data[at], bytes.Contains([]byte(format), []byte("@")), nil
			}
			offset = at + 1
		}
	}
	return nil, false, fmt.Errorf("Foundation has no registered string log formatter")
}
