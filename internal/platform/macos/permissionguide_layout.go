//go:build darwin && !ios

package macos

import (
	"path/filepath"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// What the permission guide computes without AppKit, after PermissionFlow (https://github.com/jaywcjlove/PermissionFlow) through MyGo's port: the panes, which take an app dragged into their list, and where the panel goes.

// privacyAnchors are the anchors of the panes of System Settings' Privacy & Security, which x-apple.systempreferences:com.apple.preference.security?anchor opens.
var privacyAnchors = map[string]string{
	driver.PermissionAccessibility:       "Privacy_Accessibility",
	driver.PermissionScreenRecording:     "Privacy_ScreenCapture",
	driver.PermissionInputMonitoring:     "Privacy_ListenEvent",
	driver.PermissionFullDiskAccess:      "Privacy_AllFiles",
	driver.PermissionAppManagement:       "Privacy_AppBundles",
	driver.PermissionDeveloperTools:      "Privacy_DevTools",
	driver.PermissionAutomation:          "Privacy_Automation",
	driver.PermissionMicrophone:          "Privacy_Microphone",
	driver.PermissionCamera:              "Privacy_Camera",
	driver.PermissionContacts:            "Privacy_Contacts",
	driver.PermissionCalendars:           "Privacy_Calendars",
	driver.PermissionReminders:           "Privacy_Reminders",
	driver.PermissionPhotos:              "Privacy_Photos",
	driver.PermissionBluetooth:           "Privacy_Bluetooth",
	driver.PermissionMediaLibrary:        "Privacy_Media",
	driver.PermissionLocalNetwork:        "Privacy_LocalNetwork",
	driver.PermissionLocation:            "Privacy_LocationServices",
	driver.PermissionBackgroundLocation:  "Privacy_LocationServices",
	driver.PermissionMotion:              "Privacy_Motion",
	driver.PermissionActivityRecognition: "Privacy_Motion",
}

// privacySettingsURL is the URL of the pane that gives the permission name, "" for none.
func privacySettingsURL(name string) string {
	if name == driver.PermissionNotifications {
		return "x-apple.systempreferences:com.apple.preference.notifications"
	}
	if _, ok := driver.AutomationTarget(name); ok {
		name = driver.PermissionAutomation
	}
	anchor, ok := privacyAnchors[name]
	if !ok {
		return ""
	}
	return "x-apple.systempreferences:com.apple.preference.security?" + anchor
}

// takesDrag says the list of the permission's pane takes an app dragged into it; the others list only the apps that asked with the system's prompt.
func takesDrag(name string) bool {
	switch name {
	case driver.PermissionAccessibility, driver.PermissionScreenRecording, driver.PermissionInputMonitoring,
		driver.PermissionFullDiskAccess, driver.PermissionAppManagement, driver.PermissionDeveloperTools,
		driver.PermissionBluetooth, driver.PermissionMediaLibrary:
		return true
	}
	return false
}

// appDisplayName is what the panel calls the app at path: its file's name without its extension.
func appDisplayName(path string) string {
	name := filepath.Base(filepath.Clean(path))
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// textRun is a part of the panel's text, bold or not.
type textRun struct {
	text string
	bold bool
}

// runMarker stands in a message for text said in bold, so a translation may order the bold parts as its language does.
type runMarker struct{ token, text string }

// The markers of the guide's message, which no translation holds.
const (
	guideAppMarker        = "\x00nsgo.guide.app\x00"
	guidePermissionMarker = "\x00nsgo.guide.permission\x00"
)

// splitRuns cuts text at the markers' tokens, each becoming its text in bold.
func splitRuns(text string, markers ...runMarker) []textRun {
	var runs []textRun
	for text != "" {
		at, found := -1, runMarker{}
		for _, m := range markers {
			if i := strings.Index(text, m.token); i >= 0 && (at < 0 || i < at) {
				at, found = i, m
			}
		}
		if at < 0 {
			runs = append(runs, textRun{text: text})
			break
		}
		if at > 0 {
			runs = append(runs, textRun{text: text[:at]})
		}
		runs = append(runs, textRun{text: found.text, bold: true})
		text = text[at+len(found.token):]
	}
	return runs
}

// guideRect is a rectangle in points: in AppKit's screen coordinates from the main display's bottom left, y going up, or in the window server's from its top left, y going down.
type guideRect struct{ X, Y, W, H float64 }

func (r guideRect) maxX() float64 { return r.X + r.W }
func (r guideRect) maxY() float64 { return r.Y + r.H }

func (r guideRect) intersects(o guideRect) bool {
	return r.X < o.maxX() && o.X < r.maxX() && r.Y < o.maxY() && o.Y < r.maxY()
}

// flipRect turns r in the window server's coordinates into AppKit's, the main display being mainHeight tall.
func flipRect(r guideRect, mainHeight float64) guideRect {
	return guideRect{X: r.X, Y: mainHeight - r.Y - r.H, W: r.W, H: r.H}
}

// The panel's geometry, in points.
const (
	// settingsSidebar is the width of System Settings' sidebar: the panel lines up with the pane on its right, where the list is.
	settingsSidebar = 230
	guideMinWidth   = 240
	screenInset     = 12
)

// guideWidth is the panel's width under System Settings' window at settings, on a screen whose visible part is screen.
func guideWidth(settings, screen guideRect) float64 {
	return min(max(guideMinWidth, settings.W-settingsSidebar), screen.W-2*screenInset)
}

// guideFrame places a panel height tall under System Settings' window at settings: lined up with its pane, right under it, and inside the screen, over the window's bottom when there is no room under it.
func guideFrame(settings, screen guideRect, height float64) guideRect {
	w := guideWidth(settings, screen)
	x := max(screen.X+screenInset, min(settings.X+settingsSidebar, screen.maxX()-w-screenInset))
	y := max(screen.Y+screenInset, min(settings.Y-height, screen.maxY()-height-screenInset))
	return guideRect{X: x, Y: y, W: w, H: height}
}

// screenOf is the visible part of the screen showing most of r, else the first.
func screenOf(r guideRect, screens []guideRect) guideRect {
	best, area := guideRect{}, -1.0
	for i, s := range screens {
		if i == 0 {
			best = s
		}
		if !r.intersects(s) {
			continue
		}
		if a := (min(r.maxX(), s.maxX()) - max(r.X, s.X)) * (min(r.maxY(), s.maxY()) - max(r.Y, s.Y)); a > area {
			best, area = s, a
		}
	}
	return best
}

// settingsWindow is what the window server tells of one of System Settings' windows.
type settingsWindow struct {
	bounds guideRect
	layer  int
	alpha  float64
}

// mainSettingsWindow is System Settings' window among its windows: the largest showing one of the normal layer, leaving out its small helper windows.
func mainSettingsWindow(windows []settingsWindow) (guideRect, bool) {
	var best guideRect
	found := false
	for _, w := range windows {
		if w.layer != 0 || w.alpha <= 0 || w.bounds.W <= 320 || w.bounds.H <= 240 {
			continue
		}
		if !found || w.bounds.W*w.bounds.H > best.W*best.H {
			best, found = w.bounds, true
		}
	}
	return best, found
}
