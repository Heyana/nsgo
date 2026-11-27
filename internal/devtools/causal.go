package devtools

// SignalCause identifies one signal write that scheduled downstream computations.
type SignalCause struct {
	Seq        uint64 `json:"seq"`
	SignalID   uint64 `json:"signalID"`
	Generation uint64 `json:"generation"`
	Site       string `json:"site,omitempty"`
}

// TriggerRun links a signal write to a computation it scheduled and ran.
type TriggerRun struct {
	Cause       SignalCause `json:"cause"`
	Kind        string      `json:"kind"`
	Owner       uint64      `json:"owner"`
	Site        string      `json:"site,omitempty"`
	Duration    int64       `json:"duration"`
	Retriggered int         `json:"retriggered,omitempty"`
}

// CauseSummary pairs a signal write with the computation runs attributed to it.
type CauseSummary struct {
	Cause    SignalCause  `json:"cause"`
	Triggers []TriggerRun `json:"triggers,omitempty"`
}

// CleanupRecord describes one cleanup executed during scope disposal.
type CleanupRecord struct {
	Label string `json:"label,omitempty"`
	Site  string `json:"site,omitempty"`
}

// DisposalRecord describes one scope disposal and the cleanups it executed.
type DisposalRecord struct {
	Seq        uint64          `json:"seq"`
	RootOwner  uint64          `json:"rootOwner"`
	Owner      uint64          `json:"owner"`
	OwnerName  string          `json:"ownerName,omitempty"`
	Reason     string          `json:"reason"`
	Caller     string          `json:"caller,omitempty"`
	TriggerSeq uint64          `json:"triggerSeq,omitempty"`
	Cleanups   []CleanupRecord `json:"cleanups,omitempty"`
}

// CausalSurface is optionally implemented by Surface in nsgodev builds to expose signal-trigger and scope-disposal causal chains.
type CausalSurface interface {
	Surface
	// Causes returns the most recent n signal writes with the computation runs each triggered, oldest first.
	Causes(n int) []CauseSummary
	// Triggers returns the most recent n signal-triggered computation runs, oldest first.
	Triggers(n int) []TriggerRun
	// Disposals returns the most recent n scope disposals and their executed cleanups, oldest first.
	Disposals(n int) []DisposalRecord
}
