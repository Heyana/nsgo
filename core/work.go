package core

// workObserver receives transitions between scopes on the engine goroutine.
type workObserver struct{ active func(bool) }

// ObserveWork implements tsx's engine-time accounting for this scope and its
// descendants. active runs on the engine goroutine when owned reactive,
// mounting, commit or layout work starts or stops. Nested work is counted once;
// work belonging to another observer or an unobserved scope pauses it.
// Install it once, before building the scope's views. Disposal removes it.
func (cx *Scope) ObserveWork(active func(bool)) {
	cx.rt.check()
	if cx.work != nil || active == nil || cx.disposed {
		panic("core: invalid work observer")
	}
	cx.work = &workObserver{active: active}
	if !cx.rt.workObserved {
		cx.rt.workObserved = true
		// Views built before observation still switch attribution when a walk
		// reaches them from an observed view, including held or reparented views.
		for _, tree := range cx.rt.surfaces {
			tree.observeLayoutWork()
		}
		if cx.rt.tree != nil {
			cx.rt.tree.observeLayoutWork()
		}
	}
	cx.addCleanup(func() { cx.work = nil })
}

// BeginWork implements tsx's explicit JavaScript-entry spans. It switches
// engine work to this scope's nearest observer and returns its restore function,
// or nil when attribution did not change. Call the restore function before
// running unrelated work. Both calls belong to the engine goroutine.
func (cx *Scope) BeginWork() func() {
	if cx == nil {
		return nil
	}
	return cx.rt.beginWork(cx)
}

func (rt *runtime) beginWork(cx *Scope) func() {
	if !rt.workObserved {
		return nil
	}
	var next *workObserver
	for owner := cx; owner != nil; owner = owner.parent {
		if owner.work != nil {
			next = owner.work
			break
		}
	}
	prev := rt.activeWork
	if prev == next {
		return nil
	}
	rt.setWork(next)
	return func() { rt.setWork(prev) }
}

func (rt *runtime) setWork(next *workObserver) {
	if rt.activeWork != nil {
		rt.activeWork.active(false)
	}
	rt.activeWork = next
	if next != nil {
		next.active(true)
	}
}

func (t *Tree) observeLayoutWork() {
	for _, n := range t.nodes {
		if n != nil && n.owner != nil {
			n.lstate.BeginWork = n.owner.BeginWork
		}
	}
}
