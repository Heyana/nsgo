//go:build android

package widget

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// The Java runtime sets these before it starts Go and observes each kind's
// timeline directory. Publishing therefore also works before nsgo.Run, and the
// provider can reload and advance persisted timelines without starting Go.
func publish(kind string, entries []Entry, now time.Time) error {
	root := os.Getenv("NSGO_WIDGETS_DIR")
	kinds := strings.Split(os.Getenv("NSGO_WIDGET_KINDS"), ",")
	if root == "" || len(kinds) == 0 || kinds[0] == "" {
		return ErrNoWidgets
	}
	if !slices.Contains(kinds, kind) {
		return fmt.Errorf("widget: kind %q is not listed in nsgo.yaml", kind)
	}
	if err := writeTimeline(filepath.Join(root, kind), entries, now); err != nil {
		return fmt.Errorf("widget: %w", err)
	}
	return nil
}

func available() bool {
	return os.Getenv("NSGO_WIDGETS_DIR") != "" && os.Getenv("NSGO_WIDGET_KINDS") != ""
}
