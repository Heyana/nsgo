package driver

import "sync/atomic"

// WidgetTest is the test backend for the widget package's process-wide
// services. Its type parameters keep widget's values out of the driver.
type WidgetTest[Entry, Activity, Content, Options, Dismissal any] struct {
	Update    func(kind string, entries []Entry) error
	Available func() bool

	StartActivity     func(Activity, Content, Options) error
	UpdateActivity    func(Activity, Content, *ActivityAlert) error
	EndActivity       func(Activity, *Content, Dismissal) error
	Activities        func() []Activity
	ActivitiesEnabled func() bool
}

// ActivityAlert is the alert that accompanies a Live Activity update.
type ActivityAlert struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

var widgetTest atomic.Pointer[any]

// ConnectWidgetTest installs host until the returned function disconnects it.
// The second result is false while a test already owns the widget services.
func ConnectWidgetTest[E, A, C, O, D any](host *WidgetTest[E, A, C, O, D]) (func(), bool) {
	var value any = host
	if !widgetTest.CompareAndSwap(nil, &value) {
		return nil, false
	}
	return func() { widgetTest.CompareAndSwap(&value, nil) }, true
}

// CurrentWidgetTest is the test backend for these widget types, or nil.
func CurrentWidgetTest[E, A, C, O, D any]() *WidgetTest[E, A, C, O, D] {
	if value := widgetTest.Load(); value != nil {
		return (*value).(*WidgetTest[E, A, C, O, D])
	}
	return nil
}
