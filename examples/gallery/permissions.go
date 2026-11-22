package main

import (
	"strings"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/ui"
)

// permissionTitle is the gallery's name of p.
func permissionTitle(tr locales.Messages, p core.Permission) string {
	switch p {
	case core.PermissionAccessibility:
		return tr.PermissionAccessibility()
	case core.PermissionScreenRecording:
		return tr.PermissionScreenRecording()
	case core.PermissionInputMonitoring:
		return tr.PermissionInputMonitoring()
	case core.PermissionFullDiskAccess:
		return tr.PermissionFullDiskAccess()
	case core.PermissionAppManagement:
		return tr.PermissionAppManagement()
	case core.PermissionDeveloperTools:
		return tr.PermissionDeveloperTools()
	case core.PermissionAutomation:
		return tr.PermissionAutomation()
	case core.PermissionMicrophone:
		return tr.PermissionMicrophone()
	case core.PermissionCamera:
		return tr.PermissionCamera()
	case core.PermissionContacts:
		return tr.PermissionContacts()
	case core.PermissionCalendars:
		return tr.PermissionCalendars()
	case core.PermissionReminders:
		return tr.PermissionReminders()
	case core.PermissionPhotos:
		return tr.PermissionPhotos()
	case core.PermissionBluetooth:
		return tr.PermissionBluetooth()
	case core.PermissionLocation:
		return tr.PermissionLocation()
	case core.PermissionBackgroundLocation:
		return tr.PermissionBackgroundLocation()
	case core.PermissionActivityRecognition:
		return tr.PermissionActivityRecognition()
	case core.PermissionMotion:
		return tr.PermissionMotion()
	case core.PermissionNotifications:
		return tr.PermissionNotifications()
	}
	return tr.PermissionMediaLibrary()
}

// permissionsCard is the system's permissions: each one's status, asking for it, its settings and, on desktops, the guide to them. Those the platform has not collapse into one note.
func permissionsCard(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	colors := ui.ColorsOf(cx)
	env := cx.Env()
	statuses := map[core.Permission]*core.Signal[core.PermissionStatus]{}
	for _, p := range core.Permissions() {
		statuses[p] = core.NewSignal(cx, core.PermissionUnknown)
	}
	refresh := func() {
		for _, p := range core.Permissions() {
			cx.PermissionStatus(p, statuses[p].Set)
		}
	}
	// The statuses are read as the card opens, whether or not the app is in front, and again as the app comes back, showing what the user changed in the settings.
	read := false
	cx.Effect(func() {
		if env.Lifecycle() == core.LifecycleActive || !read {
			read = true
			cx.Untrack(refresh)
		}
	})
	guiding := core.NewSignal(cx, core.Permission(""))
	stopGuide := func() {}
	small := func(label, id string, fn func()) ui.Button {
		return ui.Button{Text: label, Variant: ui.ButtonSecondary, Size: ui.SizeSmall, OnPress: fn, TestID: id}
	}
	row := func(p core.Permission) core.Node {
		st := statuses[p]
		key := string(p)
		buttons := []core.Node{
			small(tr.SystemAskPermission(), "request-"+key, func() { cx.RequestPermission(p, st.Set) }),
			small(tr.SystemPermissionSettings(), "settings-"+key, func() { cx.OpenPermissionSettings(p, nil) }),
		}
		if desktop.IsAvailable(cx) {
			guide := small(tr.SystemGuidePermission(), "guide-"+key, func() {
				guiding.Set(p)
				stopGuide = desktop.GuidePermission(cx, p, desktop.GuideOptions{}, func(s core.PermissionStatus) {
					st.Set(s)
					if guiding.GetUntracked() == p {
						guiding.Set("")
					}
				})
			})
			guide.Disabled = func() bool { return guiding.Get() != "" }
			// The guide button stays while its guide shows: the guide ends with the button that started it.
			buttons = append(buttons, guide, core.Show{When: func() bool { return guiding.Get() == p },
				Then: func(*core.Scope) core.Node { return small(tr.SystemStopGuide(), "stop-guide", func() { stopGuide() }) }})
		}
		return core.Show{When: func() bool { return st.Get() != core.PermissionUnavailable }, Then: func(*core.Scope) core.Node {
			label := ui.Textf(func() string { return permissionTitle(tr, p) }, 13, ui.Regular, colors.Muted)
			label.Style.FlexGrow, label.Style.FlexShrink = 1, 1
			status := core.Text{ContentFunc: func() string { return st.Get().String() }, TestID: "permission-" + key,
				Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(colors.Faint)}
			return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8}, Children: []core.Node{label, status}},
				ui.Wrap{Children: buttons},
			}}
		}}
	}
	children := []core.Node{}
	for _, p := range core.Permissions() {
		children = append(children, row(p))
	}
	unavailable := func() string {
		var titles []string
		for _, p := range core.Permissions() {
			if statuses[p].Get() == core.PermissionUnavailable {
				titles = append(titles, permissionTitle(tr, p))
			}
		}
		return strings.Join(titles, tr.SystemListSeparator())
	}
	children = append(children, core.Show{When: func() bool { return unavailable() != "" }, Then: func(*core.Scope) core.Node {
		return Note{TextFunc: func() string { return tr.SystemPermissionsUnavailable(unavailable()) }}
	}})
	return Card{Title: tr.SystemPermissions(), Description: tr.SystemPermissionsSummary(),
		Actions: []core.Node{ui.Badge{Text: tr.SystemNsgo()}}, Children: children}
}
