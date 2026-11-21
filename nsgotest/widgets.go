package nsgotest

import (
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/widget"
)

// Widgets records widget.Update calls in App.Widgets and sets widget.Available.
// When available is false, widget.Update returns widget.ErrNoWidgets. The
// widget service is process-wide, so tests using it must run one at a time.
func Widgets(available bool) Option {
	return func(a *App) { a.widgetRecorder().available = &available }
}

// LiveActivities records activity starts, updates and ends in
// App.LiveActivities. When enabled is false, widget.ActivitiesEnabled reports
// false and widget.StartActivity returns widget.ErrActivitiesDisabled. Like
// Widgets, this option takes the process-wide service for the test.
func LiveActivities(enabled bool) Option {
	return func(a *App) { a.widgetRecorder().activitiesEnabled = &enabled }
}

// ActivityRecord is a Live Activity the app started. Changes are in order
// from the initial content. End is nil while the activity is running.
type ActivityRecord struct {
	Activity *widget.Activity
	Options  widget.ActivityOptions
	Changes  []ActivityChange
	End      *ActivityEnd
}

// ActivityChange is the content the app showed at start or in an update.
type ActivityChange struct {
	At                    time.Time
	Content               widget.ActivityContent
	Alert                 bool
	AlertTitle, AlertBody string
}

// ActivityEnd records the end of an activity and its optional final content.
type ActivityEnd struct {
	At        time.Time
	Final     *widget.ActivityContent
	Dismissal widget.Dismissal
}

type widgetServices struct {
	available, activitiesEnabled *bool
	now                          func() time.Time
	mu                           sync.Mutex
	timelines                    map[string][]widget.Entry
	activities                   []ActivityRecord
}

func (a *App) widgetRecorder() *widgetServices {
	if a.widgets == nil {
		a.widgets = &widgetServices{}
	}
	return a.widgets
}

func (w *widgetServices) connect(a *App) {
	a.t.Helper()
	w.now = a.Clock.WallTime
	if a.measure && onTarget() {
		w.now = time.Now
	}
	var host driver.WidgetTest[widget.Entry, *widget.Activity, widget.ActivityContent, widget.ActivityOptions, widget.Dismissal]
	if w.available != nil {
		host.Update = w.updateWidget
		host.Available = func() bool { return *w.available }
		w.timelines = make(map[string][]widget.Entry)
	}
	if w.activitiesEnabled != nil {
		host.StartActivity = w.startActivity
		host.UpdateActivity = w.updateActivity
		host.EndActivity = w.endActivity
		host.Activities = w.runningActivities
		host.ActivitiesEnabled = func() bool { return *w.activitiesEnabled }
	}
	disconnect, ok := driver.ConnectWidgetTest(&host)
	if !ok {
		a.t.Fatal("nsgotest: another app owns the widget services; run widget tests one at a time")
		return
	}
	a.t.Cleanup(disconnect)
}

func (w *widgetServices) updateWidget(kind string, entries []widget.Entry) error {
	if !*w.available {
		return widget.ErrNoWidgets
	}
	entries = cloneWidgetEntries(entries)
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	for i := range entries {
		if entries[i].Date.IsZero() {
			entries[i].Date = now
		}
	}
	w.timelines[kind] = entries
	return nil
}

// Widgets returns a snapshot of the latest timeline published for each kind,
// including cleared timelines. Entries with no date use the app's wall time.
func (a *App) Widgets() map[string][]widget.Entry {
	if a.widgets == nil {
		return nil
	}
	w := a.widgets
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make(map[string][]widget.Entry, len(w.timelines))
	for kind, entries := range w.timelines {
		out[kind] = cloneWidgetEntries(entries)
	}
	return out
}

func (w *widgetServices) startActivity(a *widget.Activity, c widget.ActivityContent, o widget.ActivityOptions) error {
	if !*w.activitiesEnabled {
		return widget.ErrActivitiesDisabled
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.activities = append(w.activities, ActivityRecord{
		Activity: a, Options: o,
		Changes: []ActivityChange{{At: w.now(), Content: cloneActivityContent(c)}},
	})
	return nil
}

// runningActivity finds the record a handle refers to. The caller holds w.mu.
func (w *widgetServices) runningActivity(a *widget.Activity) *ActivityRecord {
	for i := range w.activities {
		record := &w.activities[i]
		if record.Activity.ID() == a.ID() && record.End == nil {
			return record
		}
	}
	return nil
}

func (w *widgetServices) updateActivity(a *widget.Activity, c widget.ActivityContent, alert *driver.ActivityAlert) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	record := w.runningActivity(a)
	if record == nil {
		return widget.ErrActivityEnded
	}
	change := ActivityChange{At: w.now(), Content: cloneActivityContent(c)}
	if alert != nil {
		change.Alert, change.AlertTitle, change.AlertBody = true, alert.Title, alert.Body
	}
	record.Changes = append(record.Changes, change)
	return nil
}

func (w *widgetServices) endActivity(a *widget.Activity, final *widget.ActivityContent, d widget.Dismissal) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	record := w.runningActivity(a)
	if record == nil {
		return widget.ErrActivityEnded
	}
	record.End = &ActivityEnd{At: w.now(), Final: cloneFinalContent(final), Dismissal: d}
	return nil
}

func (w *widgetServices) runningActivities() []*widget.Activity {
	w.mu.Lock()
	defer w.mu.Unlock()
	var running []*widget.Activity
	for _, record := range w.activities {
		if record.End == nil {
			running = append(running, record.Activity)
		}
	}
	return running
}

// LiveActivities returns snapshots of the activities the app started, in
// start order, including those that ended.
func (a *App) LiveActivities() []ActivityRecord {
	if a.widgets == nil {
		return nil
	}
	w := a.widgets
	w.mu.Lock()
	defer w.mu.Unlock()
	out := slices.Clone(w.activities)
	for i := range out {
		record := &out[i]
		record.Changes = slices.Clone(record.Changes)
		for j := range record.Changes {
			record.Changes[j].Content = cloneActivityContent(record.Changes[j].Content)
		}
		if record.End != nil {
			end := *record.End
			end.Final = cloneFinalContent(end.Final)
			record.End = &end
		}
	}
	return out
}

func cloneWidgetEntries(entries []widget.Entry) []widget.Entry {
	out := slices.Clone(entries)
	for i := range out {
		entry := &out[i]
		entry.View = cloneWidgetView(entry.View)
		entry.Background = cloneWidgetView(entry.Background)
		if entry.Families != nil {
			families := make(map[widget.Family]widget.View, len(entry.Families))
			for family, view := range entry.Families {
				families[family] = cloneWidgetView(view)
			}
			entry.Families = families
		}
	}
	return out
}

func cloneActivityContent(c widget.ActivityContent) widget.ActivityContent {
	c.Actions = slices.Clone(c.Actions)
	for _, view := range []*widget.View{&c.LockScreen, &c.Leading, &c.Trailing, &c.Center, &c.Bottom, &c.CompactLeading, &c.CompactTrailing, &c.Minimal} {
		*view = cloneWidgetView(*view)
	}
	return c
}

func cloneFinalContent(c *widget.ActivityContent) *widget.ActivityContent {
	if c == nil {
		return nil
	}
	out := cloneActivityContent(*c)
	return &out
}

func cloneWidgetView(view widget.View) widget.View {
	// The built-in views have value receivers. Keep the caller's concrete
	// type when they supply a pointer to one.
	value := reflect.ValueOf(view)
	if value.Kind() == reflect.Pointer && !value.IsNil() {
		if v, ok := value.Elem().Interface().(widget.View); ok {
			out := reflect.New(value.Type().Elem())
			out.Elem().Set(reflect.ValueOf(cloneWidgetView(v)))
			return out.Interface().(widget.View)
		}
	}
	switch v := view.(type) {
	case widget.Row:
		v.Children = cloneWidgetChildren(v.Children)
		return v
	case widget.Column:
		v.Children = cloneWidgetChildren(v.Children)
		return v
	case widget.Stack:
		v.Children = cloneWidgetChildren(v.Children)
		return v
	case widget.Button:
		v.Children = cloneWidgetChildren(v.Children)
		return v
	case widget.Image:
		v.Data = slices.Clone(v.Data)
		return v
	default:
		return view
	}
}

func cloneWidgetChildren(children []widget.View) []widget.View {
	out := slices.Clone(children)
	for i := range out {
		out[i] = cloneWidgetView(out[i])
	}
	return out
}
