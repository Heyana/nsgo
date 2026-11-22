//go:build darwin

package applelog

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unicode/utf8"
)

const entryBytes = 960

var installOnce sync.Once
var installed bool

// Install routes the default Go logger to the main bundle's unified log. It
// leaves a custom writer alone. A terminal, file or pipe still gets its output,
// except when Xcode already shows the unified log; development always keeps it.
// The first call owns the process-wide standard logger, as it does stderr.
func Install(development bool) bool {
	installOnce.Do(func() { installed = install(log.Default(), development) })
	return installed
}

func install(standard *log.Logger, development bool) bool {
	if standard.Writer() != os.Stderr {
		return false
	}
	api, err := system()
	if err != nil {
		return false
	}
	sink, err := newLog(subsystem(api), "nsgo")
	if err != nil {
		return false
	}
	_, debugger := os.LookupEnv("IDE_DISABLED_OS_ACTIVITY_DT_MODE")
	_, legacyDebugger := os.LookupEnv("OS_ACTIVITY_DT_MODE")
	mirror := mirrorStderr(development, goesSomewhere(os.Stderr), debugger || legacyDebugger, os.Getenv("OS_ACTIVITY_MODE") == "disable")
	standard.SetOutput(&writer{stderr: os.Stderr, mirror: mirror, emit: func(text string) { sink.write(defaultType, text) }})
	return true
}

func subsystem(api *systemAPI) string {
	if bundle := api.bundle(); bundle != 0 {
		if id := api.bundleID(bundle); id != 0 {
			length := api.stringLength(id)
			if length > 0 && length <= 8192 {
				buf := make([]byte, length*4+1)
				if api.copyString(id, &buf[0], len(buf), 0x08000100) {
					return string(bytes.TrimRight(buf, "\x00"))
				}
			}
		}
	}
	if path, err := os.Executable(); err == nil {
		return filepath.Base(path)
	}
	return "nsgo"
}

func mirrorStderr(development, destination, debugger, disabled bool) bool {
	return development || destination && (!debugger || disabled)
}

func goesSomewhere(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	null, err := os.Stat(os.DevNull)
	if err != nil {
		return true
	}
	fd, ok := info.Sys().(*syscall.Stat_t)
	device, nullOK := null.Sys().(*syscall.Stat_t)
	return !ok || !nullOK || fd.Mode&syscall.S_IFMT != syscall.S_IFCHR || fd.Rdev != device.Rdev
}

type writer struct {
	stderr io.Writer
	mirror bool
	emit   func(string)
}

func (w *writer) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	text := strings.TrimSuffix(string(data), "\n")
	text = strings.ToValidUTF8(strings.ReplaceAll(text, "\x00", "\\0"), "\uFFFD")
	parts(text, w.emit)
	if w.mirror {
		return w.stderr.Write(data)
	}
	return len(data), nil
}

// parts keeps every entry under the store's roughly 1 KiB payload limit, at a
// line boundary where one fits and otherwise between UTF-8 characters.
func parts(text string, emit func(string)) {
	for len(text) > entryBytes {
		end := entryBytes
		for !utf8.RuneStart(text[end]) {
			end--
		}
		if newline := strings.LastIndexByte(text[:end], '\n'); newline > 0 {
			emit(text[:newline])
			text = text[newline+1:]
		} else {
			emit(text[:end])
			text = text[end:]
		}
	}
	emit(text)
}
