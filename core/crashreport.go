package core

import (
	"io"
	"time"

	"github.com/nsgo-dev/nsgo/internal/crashreport"
)

// CrashReport describes a diagnostic kept by an earlier process.
type CrashReport struct {
	// Version and Build identify the app that crashed.
	Version, Build string
	// Platform is the operating system, such as darwin, ios, android or windows.
	Platform string
	// Kind is go, objective-c, java or seh.
	Kind string
	// Time is when the report was written.
	Time time.Time

	report crashreport.Report
}

// CrashReports lists the app's pending crash reports, oldest first. It
// returns an empty list on targets without a logs directory.
func (cx *Scope) CrashReports() ([]CrashReport, error) {
	reports, err := crashreport.Pending(cx.LogsDir())
	if err != nil || len(reports) == 0 {
		return nil, err
	}
	result := make([]CrashReport, 0, len(reports))
	for _, r := range reports {
		result = append(result, CrashReport{Version: r.Version, Build: r.Build, Platform: r.Platform, Kind: r.Kind, Time: r.Time, report: r})
	}
	return result, nil
}

// Open opens the diagnostic text. The caller closes the reader when done.
func (r CrashReport) Open() (io.ReadCloser, error) { return r.report.Open() }

// Acknowledge removes the report from the app's logs directory.
func (r CrashReport) Acknowledge() error { return r.report.Acknowledge() }
