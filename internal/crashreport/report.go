// Package crashreport keeps crash traces under an app's logs directory.
package crashreport

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const directory = "crashes"

// Header identifies the build and the source of a trace. It is the first
// line of the file, followed by the original diagnostic text.
type Header struct {
	Platform string `json:"platform"`
	Version  string `json:"version"`
	Build    string `json:"build"`
	Kind     string `json:"kind"`
}

type Report struct {
	Header
	Time       time.Time
	dir, name  string
	traceStart int64
}

// Pending lists complete reports, oldest first, skipping live writers.
func Pending(logs string) ([]Report, error) {
	if logs == "" {
		return nil, nil
	}
	dir := filepath.Join(logs, directory)
	root, err := os.OpenRoot(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	var reports []Report
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), "report-") || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		report, err := readReport(root, entry.Name())
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if report != nil {
			report.dir = dir
			reports = append(reports, *report)
		}
	}
	slices.SortStableFunc(reports, func(a, b Report) int { return a.Time.Compare(b.Time) })
	return reports, nil
}

func readReport(root *os.Root, name string) (*Report, error) {
	file, err := root.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	line, err := reader.ReadSlice('\n')
	var info Header
	if err != nil || json.Unmarshal(line, &info) != nil || info.Kind == "" || info.Platform == "" {
		return nil, nil
	}
	// Capture takes its lock before writing the header. A reader that finds
	// a fresh file must leave that first lock to its writer.
	locked, err := lock(file)
	if err != nil || !locked {
		return nil, err
	}
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	start := int64(len(line))
	if stat.Size() == start {
		// A clean or forcibly killed process can leave only the header.
		return nil, root.Remove(name)
	}
	return &Report{Header: info, Time: stat.ModTime(), name: name, traceStart: start}, nil
}

// Open starts reading at the first byte of the diagnostic text.
func (r Report) Open() (io.ReadCloser, error) {
	if r.name == "" {
		return nil, fs.ErrNotExist
	}
	root, err := os.OpenRoot(r.dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(r.name)
	if err != nil {
		return nil, err
	}
	if _, err := file.Seek(r.traceStart, io.SeekStart); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

// Acknowledge removes the report. A report already removed counts as acknowledged.
func (r Report) Acknowledge() error {
	if r.name == "" {
		return fs.ErrNotExist
	}
	root, err := os.OpenRoot(r.dir)
	if err == nil {
		defer root.Close()
		err = root.Remove(r.name)
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
