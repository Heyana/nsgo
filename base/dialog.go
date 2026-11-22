package base

import (
	"github.com/nsgo-dev/nsgo/base/internal/parts"
	"github.com/nsgo-dev/nsgo/core"
)

// Dialog is a modal layer over the app: an optional backdrop and a
// popup centered on it.
type Dialog struct {
	Open         func() bool
	DefaultOpen  bool
	OnOpenChange func(open bool, reason Reason)
	OnOpen       func()
	OnClose      func()
	OnCancel     func()
	// OnConfirm, when set, is the default action: Return runs it while the dialog is the topmost modal, then asks to close with ReasonSelect, once however often Return is pressed as it goes. A focused button, menu row or text input inside keeps Return for itself (core.KeyAuto). Without it Return does what it did.
	OnConfirm func()

	Animation core.ModalAnimation
	// TransitionShader, when set, is how the dialog comes and goes
	// instead of Animation, where the platform draws it.
	TransitionShader       *core.TransitionShader
	NoCloseOnEscape        bool
	NoCloseOnBackdropPress bool
	// Presentation and NoCloseOnSwipe are iOS's, IOS and Android hold the
	// props only one of them has, and StatusBar is set over the app's
	// while the dialog is open, as core.Modal's are.
	Presentation   core.ModalPresentation
	NoCloseOnSwipe bool
	IOS            core.IOSModalProps
	Android        core.AndroidModalProps
	StatusBar      core.StatusBarSetting
	StatusBarFunc  func() core.StatusBarSetting
	// BackdropClass, BackdropStyle or BackdropStyleFunc, when set, draws a
	// backdrop whose press asks to close; BackdropTestID names it.
	BackdropClass     string
	BackdropStyle     core.Style
	BackdropStyleFunc func() core.Style
	BackdropTestID    string
	PopupClass        string
	PopupStyle        core.Style
	PopupStyleFunc    func() core.Style
	// StartingStyle styles the popup in its first frame and EndingStyle as
	// it goes (popupMotion), as Popover's, so a Transition in its style
	// comes in and goes in the tree, with the modal held until it went;
	// BackdropStartingStyle and BackdropEndingStyle style the backdrop in
	// those phases. Without them Animation is the platform's entrance.
	StartingClass         string
	StartingStyle         core.Style
	EndingClass           string
	EndingStyle           core.Style
	BackdropStartingClass string
	BackdropStartingStyle core.Style
	BackdropEndingClass   string
	BackdropEndingStyle   core.Style
	// Before and After are laid out in the layer around the popup, such
	// as spacers that place it off center.
	Before, After []core.Node
	// LayerClass and LayerStyle style the layer that holds the backdrop
	// and the popup, over its default, which centers the popup.
	LayerClass string
	LayerStyle core.Style
	Children   []core.Node
	// Label names the dialog for screen readers, which announce it with
	// its role as it opens, a dialog or with Role an alert dialog, and
	// AccessibilityDescription after it, as a confirmation's message.
	Label                        string
	LabelFunc                    func() string
	Role                         core.Role
	AccessibilityDescription     string
	AccessibilityDescriptionFunc func() string
	TestID                       string
}

// Render implements core.Node.
func (d Dialog) Render(cx *core.Scope) core.Node {
	st := newPopup(cx, d.Open, d.DefaultOpen, d.OnOpenChange, d.OnOpen, d.OnClose)
	st.onCancel = d.OnCancel
	if d.Open != nil {
		// Closed by the user, it stays closed while Open stays true, until
		// the app sets it false and true again.
		st.userClosed = core.NewSignal(cx, false)
		cx.Effect(func() {
			if !d.Open() {
				st.userClosed.Set(false)
			}
		})
		st.open = func() bool { return d.Open() && !st.userClosed.Get() }
	}
	motion := newPopupMotion(cx, st.open, parts.Part{Class: d.StartingClass, Style: d.StartingStyle},
		parts.Part{Class: d.EndingClass, Style: d.EndingStyle})
	// OnOpen and OnClose follow the platform's presentation and dismissal reports (EventModalShown, EventModalDismissed); the core reports a removed modal dismissed itself when the platform's report does not come (core.Modal.OnDismissed). A motion in the tree holds the modal until its ending transition finishes.
	disposed := false
	cx.Cleanup(func() { disposed = true })
	report := func(fn func()) func() {
		if fn == nil {
			return nil
		}
		return func() {
			if !disposed {
				cx.Untrack(fn)
			}
		}
	}
	m := core.Modal{Animation: d.Animation, TransitionShader: d.TransitionShader, Presentation: d.Presentation, NoCloseOnSwipe: d.NoCloseOnSwipe,
		NoCloseOnEscape: d.NoCloseOnEscape, NoCloseOnBackdropPress: d.NoCloseOnBackdropPress,
		IOS: d.IOS, Android: d.Android, StatusBar: d.StatusBar, StatusBarFunc: d.StatusBarFunc, Class: d.LayerClass,
		OnShown: report(d.OnOpen), OnDismissed: report(d.OnClose),
		Style: core.Style{JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter}.Merge(d.LayerStyle)}
	return modalLayer(st, motion, m, d)
}
