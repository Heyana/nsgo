package core

// Regions are spans of their parent's children that change: each owns
// the scope its nodes were built under and disposes it when they go.

// Dynamic builds its nodes again whenever what fn read changes.
type Dynamic func(cx *Scope) Node

// Render implements Node.
func (d Dynamic) Render(*Scope) Node { return d }

// Show builds Then while When is true and Else otherwise. Each branch is
// built only when When changes.
type Show struct {
	When func() bool
	Then func(cx *Scope) Node
	Else func(cx *Scope) Node
}

// Render implements Node.
func (s Show) Render(*Scope) Node { return s }

// Catch builds Try and, when it or anything under it panics, Fallback
// instead. Calling reset builds Try again.
type Catch struct {
	Try      func(cx *Scope) Node
	Fallback func(cx *Scope, err error, reset func()) Node
}

// Render implements Node.
func (c Catch) Render(*Scope) Node { return c }

// Each builds a row per item, matched to items by key: a row whose key
// stays is kept and moved, its item and index updated; rows are built for
// new keys and disposed for keys gone. Go methods cannot take type
// parameters, so Each is a function.
func Each[T any, K comparable](items func() []T, key func(T) K,
	row func(cx *Scope, item func() T, index func() int) Node) Node {
	return &each[T, K]{items: items, key: key, row: row}
}

type each[T any, K comparable] struct {
	items func() []T
	key   func(T) K
	row   func(cx *Scope, item func() T, index func() int) Node
}

func (e *each[T, K]) Render(*Scope) Node { return e }

// eachRow is one row of an Each.
type eachRow[T any] struct {
	scope *Scope
	item  *Signal[T]
	index *Signal[int]
	node  *node // the row's region
}

// mount implements regionMounter: one render effect matches rows to keys.
// Rows belong to owner, not to the effect, so they outlive its runs.
func (e *each[T, K]) mount(t *Tree, region *node, owner *Scope) {
	rows := map[K]*eachRow[T]{}
	var extra []*eachRow[T] // rows of duplicate keys, built again each run
	owner.renderEffect(func() {
		items := e.items()
		owner.Untrack(func() {
			next := make(map[K]*eachRow[T], len(items))
			var nextExtra []*eachRow[T]
			order := make([]*node, 0, len(items))
			for i, it := range items {
				k := e.key(it)
				r, kept := rows[k]
				if kept {
					delete(rows, k)
					r.item.Set(it)
					r.index.Set(i)
				} else {
					r = &eachRow[T]{scope: &Scope{}}
					owner.adopt(r.scope)
					r.item = NewSignalFunc(r.scope, it, nil)
					r.index = NewSignal(r.scope, i)
					r.node = t.newRegion(region, r.scope)
					item, index := r.item.Get, r.index.Get
					t.within(r.scope, func() { t.mount(r.scope, r.node, e.row(r.scope, item, index)) })
				}
				if _, dup := next[k]; dup {
					nextExtra = append(nextExtra, r)
				} else {
					next[k] = r
				}
				order = append(order, r.node)
			}
			for _, r := range rows {
				r.scope.Dispose()
				t.discard(r.node)
			}
			for _, r := range extra {
				r.scope.Dispose()
				t.discard(r.node)
			}
			rows, extra = next, nextExtra
			t.setChildren(region, order)
		})
	})
}
