//go:build darwin || linux || windows

package crashreport

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
)

// Capture directs fatal Go diagnostics to a file as well as stderr.
func Capture(logs, version, build string) (stop func(), err error) {
	root, err := openDirectory(logs)
	if err != nil {
		return nil, err
	}
	name := "report-" + rand.Text() + ".log"
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		root.Close()
		return nil, err
	}
	defer func() {
		if err != nil {
			file.Close()
			root.Remove(name)
			root.Close()
		}
	}()
	locked, err := lock(file)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, errors.New("crash output file is busy")
	}
	info := Header{Platform: runtime.GOOS, Version: version, Build: build, Kind: "go"}
	if err := json.NewEncoder(file).Encode(info); err != nil {
		return nil, err
	}
	start, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	if err := debug.SetCrashOutput(file, debug.CrashOptions{}); err != nil {
		return nil, err
	}
	return func() {
		debug.SetCrashOutput(nil, debug.CrashOptions{})
		if stat, err := file.Stat(); err == nil && stat.Size() == start {
			root.Remove(name)
		}
		file.Close()
		root.Close()
	}, nil
}

// Record publishes a completed exception report in one rename.
func Record(logs string, info Header, trace string) error {
	root, err := openDirectory(logs)
	if err != nil {
		return err
	}
	defer root.Close()
	id := rand.Text()
	tmp := ".report-" + id
	file, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	err = json.NewEncoder(file).Encode(info)
	if err == nil {
		_, err = io.WriteString(file, trace)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return root.Rename(tmp, "report-"+id+".log")
}

func openDirectory(logs string) (*os.Root, error) {
	dir := filepath.Join(logs, directory)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return os.OpenRoot(dir)
}
