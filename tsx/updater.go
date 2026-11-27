package tsx

import (
	"github.com/nsgo-dev/nsgo/tsx/internal/js"
	"github.com/nsgo-dev/nsgo/updater"
)

// nsgo/updater exposes package updater to TSX apps on the same platform
// updaters Go apps use. It is withheld from SandboxModules by default because
// checking, installing, restarting and changing updater preferences act on the
// host app; hosts opt a sandbox in through Sandbox.Modules.
func init() {
	moduleTable["nsgo/updater"] = []export{
		fn("checkForUpdate", (*runtime).updaterCheck),
		fn("checkInBackground", (*runtime).updaterCheckInBackground),
		fn("checkDaily", (*runtime).updaterCheckDaily),
		fn("capability", (*runtime).updaterCapability),
		fn("status", (*runtime).updaterStatus),
		fn("setAutomaticChecks", (*runtime).updaterSetAutomaticChecks),
		fn("setAutomaticDownloads", (*runtime).updaterSetAutomaticDownloads),
	}
}

var updaterManagedNames = map[updater.Managed]string{
	updater.ManagedUnavailable: "unavailable",
	updater.ManagedSelf:        "self",
	updater.ManagedPage:        "page",
	updater.ManagedDevelopment: "development",
	updater.ManagedChannel:     "channel",
	updater.ManagedStore:       "store",
}

var updaterPhaseNames = map[updater.Phase]string{
	updater.PhaseIdle:        "idle",
	updater.PhaseDownloading: "downloading",
	updater.PhaseInstalling:  "installing",
}

func (r *runtime) updaterCheck(_ js.FunctionCall) js.Value {
	updater.Check(r.cx)
	return js.Undefined()
}

func (r *runtime) updaterCheckInBackground(_ js.FunctionCall) js.Value {
	updater.CheckInBackground(r.cx)
	return js.Undefined()
}

func (r *runtime) updaterCheckDaily(_ js.FunctionCall) js.Value {
	updater.CheckDaily(r.cx)
	return js.Undefined()
}

func (r *runtime) updaterCapability(_ js.FunctionCall) js.Value {
	return r.updaterCapabilityValue(updater.CapabilityOf(r.cx))
}

func (r *runtime) updaterStatus(_ js.FunctionCall) js.Value {
	st := updater.Status(r.cx)
	o := r.vm.NewObject()
	_ = o.Set("capability", r.updaterCapabilityValue(st.Capability))
	_ = o.Set("automaticChecks", st.AutomaticChecks)
	_ = o.Set("automaticDownloads", st.AutomaticDownloads)
	_ = o.Set("available", r.nullableString(st.Available))
	_ = o.Set("notes", r.nullableString(st.Notes))
	if st.LastCheck.IsZero() {
		_ = o.Set("lastCheck", js.Null())
	} else {
		_ = o.Set("lastCheck", st.LastCheck.UnixMilli())
	}
	phase := updaterPhaseNames[st.Phase]
	if phase == "" {
		phase = "idle"
	}
	_ = o.Set("phase", phase)
	_ = o.Set("loaded", st.Loaded)
	_ = o.Set("total", st.Total)
	_ = o.Set("fraction", st.Fraction())
	return o
}

func (r *runtime) updaterSetAutomaticChecks(call js.FunctionCall) js.Value {
	updater.SetAutomaticChecks(r.cx, arg(call, 0).ToBoolean())
	return js.Undefined()
}

func (r *runtime) updaterSetAutomaticDownloads(call js.FunctionCall) js.Value {
	updater.SetAutomaticDownloads(r.cx, arg(call, 0).ToBoolean())
	return js.Undefined()
}

func (r *runtime) updaterCapabilityValue(c updater.Capability) *js.Object {
	o := r.vm.NewObject()
	managed := updaterManagedNames[c.Managed]
	if managed == "" {
		managed = "unavailable"
	}
	_ = o.Set("managed", managed)
	_ = o.Set("store", r.nullableString(c.Store))
	return o
}

func (r *runtime) nullableString(s string) js.Value {
	if s == "" {
		return js.Null()
	}
	return r.vm.ToValue(s)
}
