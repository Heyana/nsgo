//go:build !nsgodev

package core

import (
	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Release builds keep zero-sized placeholders and empty inlineable hooks so none of the causal-chain diagnostics code is linked.
type (
	causalState      struct{}
	scopeCausal      struct{}
	signalCausal     struct{}
	compCausal       struct{}
	cleanupCausal    struct{}
	soundWaitCausal  struct{}
	signalWriteToken struct{}
	compRunToken     struct{}
	disposalToken    struct{}
)

func (*signalCausal) setDevID(uint64)                                      {}
func (*runtime) noteNewComputation(*computation)                           {}
func (*runtime) beginSignalWrite(*signalCausal) signalWriteToken           { return signalWriteToken{} }
func (*runtime) endSignalWrite(signalWriteToken)                           {}
func (*runtime) noteMarked(*computation)                                   {}
func (*computation) clearPendingCause()                                    {}
func (*runtime) beginCompRun(*computation) compRunToken                    { return compRunToken{} }
func (*runtime) endCompRun(*computation, compRunToken)                     {}
func (*runtime) beginMemoNotify(compRunToken) signalWriteToken             { return signalWriteToken{} }
func (*runtime) endMemoNotify(signalWriteToken)                            {}
func (*runtime) takeCommitTriggers(int) []driver.Trigger                   { return nil }
func (*runtime) beginExplicitDisposal(*Scope) disposalToken                { return disposalToken{} }
func (*runtime) beginRerunDisposal(*computation) disposalToken             { return disposalToken{} }
func (*runtime) beginCascadeDisposal(*Scope) disposalToken                 { return disposalToken{} }
func (*runtime) endDisposal(disposalToken)                                 {}
func (*runtime) noteCleanupRan(*cleanup)                                   {}
func (*Scope) annotateCleanup(*cleanup, string)                            {}
func (*Scope) annotateCleanupSite(*cleanup, string, string)                {}
func (*Scope) annotateSurfaceCleanup(*cleanup, driver.SurfaceKind)         {}
func (*soundWaitCausal) captureSite(*runtime)                              {}
func (*soundWaitCausal) loadSite() string                                  { return "" }
func (*devSurface) fillOwnerCausal(*Scope, *devtools.Owner)                {}
func (*devSurface) fillCompCausal(*computation, *devtools.Computation)     {}
