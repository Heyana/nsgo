package js

// EngineProperties describes differences the TSX runtime relies on.
type EngineProperties struct {
	CanInterrupt         bool // The host can stop a running JS slice.
	ReactionsAfterCommit bool // Promise reactions run on a later page turn.
	HasNativeIntl        bool // Keep the engine's own internationalization APIs.
	SharesPageAuthority  bool // The realm can reach the containing page's APIs.
	CanLimitRegexps      bool // NSGo can bound the regular expression match time.
}

// Properties returns the properties of the engine selected for this build.
func Properties() EngineProperties { return engineProperties }
