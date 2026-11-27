package watchos

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type decodedOp struct {
	op        byte
	id        driver.NodeID
	parent    driver.NodeID
	before    driver.NodeID
	kind      byte
	frame     driver.Bounds
	opacity   float64
	hidden    bool
	clips     bool
	transform [6]float64
}

// decodeOps parses the binary op stream written by writer, matching Host.swift's Reader.
func decodeOps(t *testing.T, buf []byte) []decodedOp {
	t.Helper()
	var ops []decodedOp
	at := 0
	u8 := func() byte {
		if at >= len(buf) {
			t.Fatalf("truncated u8 at %d", at)
		}
		v := buf[at]
		at++
		return v
	}
	u32 := func() uint32 {
		if at+4 > len(buf) {
			t.Fatalf("truncated u32 at %d", at)
		}
		v := binary.LittleEndian.Uint32(buf[at : at+4])
		at += 4
		return v
	}
	f64 := func() float64 {
		if at+8 > len(buf) {
			t.Fatalf("truncated f64 at %d", at)
		}
		v := math.Float64frombits(binary.LittleEndian.Uint64(buf[at : at+8]))
		at += 8
		return v
	}
	for at < len(buf) {
		op := u8()
		var d decodedOp
		d.op = op
		switch op {
		case opCreate:
			d.id = driver.NodeID(u32())
			d.kind = u8()
		case opInsert:
			d.id = driver.NodeID(u32())
			d.parent = driver.NodeID(u32())
			d.before = driver.NodeID(u32())
		case opRemove, opDelete, opRedraw, opFocus, opBlur, opRoot, opDismiss, opRetire:
			d.id = driver.NodeID(u32())
		case opFrame:
			d.id = driver.NodeID(u32())
			d.frame = driver.Bounds{
				Origin: driver.Point{X: driver.Points(f64()), Y: driver.Points(f64())},
				Size:   driver.Size{Width: driver.Points(f64()), Height: driver.Points(f64())},
			}
		case opLook:
			d.id = driver.NodeID(u32())
			d.opacity = f64()
			d.hidden = u8() != 0
			d.clips = u8() != 0
			for range 8 { // 4 radii + 4 insets
				_ = f64()
			}
			for i := range 6 {
				d.transform[i] = f64()
			}
		case opCull, opLock:
			d.id = driver.NodeID(u32())
			_ = u8()
		default:
			t.Fatalf("unexpected op %d at offset %d", op, at-1)
		}
		ops = append(ops, d)
	}
	return ops
}

func takeOps(t *testing.T, tr *watchTree) []decodedOp {
	t.Helper()
	ops := decodeOps(t, tr.w.buf)
	tr.w.buf = tr.w.buf[:0]
	return ops
}

func hasOp(ops []decodedOp, op byte, id driver.NodeID) bool {
	for _, o := range ops {
		if o.op == op && o.id == id {
			return true
		}
	}
	return false
}

func lastLookOp(ops []decodedOp, id driver.NodeID) (decodedOp, bool) {
	for i := len(ops) - 1; i >= 0; i-- {
		if ops[i].op == opLook && ops[i].id == id {
			return ops[i], true
		}
	}
	return decodedOp{}, false
}

func rectBounds(x, y, w, h driver.Points) driver.Bounds {
	return driver.Bounds{Origin: driver.Point{X: x, Y: y}, Size: driver.Size{Width: w, Height: h}}
}

func nearFloat(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func TestLeavingGhostFadesOutAndEmitsDelayedDelete(t *testing.T) {
	t0 := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	var tr watchTree
	tr.initTree(t0)

	tr.batch = 1
	root := tr.createNode(driver.KindView, 1)
	card := tr.createNode(driver.KindView, 2)
	child := tr.createNode(driver.KindText, 3)
	tr.insertNode(card, root, nil, nil)
	tr.insertNode(child, card, nil, nil)
	tr.setRootNode(1)
	tr.setNodeFrame(root, rectBounds(0, 0, 200, 200), nil, t0)
	tr.setNodeFrame(card, rectBounds(10, 20, 100, 80), nil, t0)
	tr.setNodeFrame(child, rectBounds(5, 5, 40, 20), nil, t0)
	st := driver.DefaultViewStyle
	st.Opacity = 0.8
	tr.setNodeStyle(card, &st, t0, false)
	tr.flushNodeLooks(t0)
	_ = takeOps(t, &tr)

	cardHost, childHost := card.hostID, child.hostID

	// Delete card and its child during a 400ms linear opacity layout animation.
	tr.batch = 2
	delStart := t0.Add(time.Second)
	lb := tr.newLayoutBatch(&driver.LayoutAnimation{
		Delete: &driver.LayoutTransition{Curve: driver.LayoutLinear, Property: driver.LayoutOpacity, Duration: 400 * time.Millisecond},
	}, delStart, false)

	var retired, destroyed []driver.NodeID
	onRetire := func(n *watchNode) { retired = append(retired, n.hostID) }
	onDestroy := func(n *watchNode) { destroyed = append(destroyed, n.hostID) }

	if !tr.deleteNode(card, lb, onRetire, onDestroy) {
		t.Fatal("expected card to be retained as a leaving ghost")
	}
	if !tr.deleteNode(child, lb, onRetire, onDestroy) {
		t.Fatal("expected child inside leaving card to retire with it")
	}
	tr.flushNodeLooks(delStart)
	ops := takeOps(t, &tr)

	if hasOp(ops, opDelete, cardHost) || hasOp(ops, opDelete, childHost) {
		t.Fatalf("emitted immediate opDelete during leaving animation: %+v", ops)
	}
	if !hasOp(ops, opRetire, cardHost) || !hasOp(ops, opRetire, childHost) {
		t.Fatalf("expected opRetire for card (%d) and child (%d), got %+v", cardHost, childHost, ops)
	}
	if tr.node(2) != nil || tr.node(3) != nil {
		t.Fatal("logical IDs were not freed immediately upon retirement")
	}
	if tr.hostNode(cardHost) != card || tr.hostNode(childHost) != child {
		t.Fatal("hostNode lookup lost retired ghost or its child")
	}
	if len(root.children) != 0 || len(root.leaving) != 1 || root.leaving[0] != card {
		t.Fatalf("root children=%d leaving=%d, want 0 and 1", len(root.children), len(root.leaving))
	}
	if len(card.children) != 1 || card.children[0] != child {
		t.Fatalf("leaving card lost retired child: %+v", card.children)
	}
	if len(tr.motions) != 1 || !tr.motions[card] {
		t.Fatalf("motions = %v, want only leaving card", tr.motions)
	}
	if len(destroyed) != 0 {
		t.Fatalf("onDestroy ran before exit finished: %v", destroyed)
	}

	// Step to 25% (100ms): opacity should be 0.8 * 0.75 = 0.6, no opDelete.
	t100 := delStart.Add(100 * time.Millisecond)
	tr.stepNodes(t100, onDestroy)
	tr.flushNodeLooks(t100)
	ops = takeOps(t, &tr)
	if hasOp(ops, opDelete, cardHost) || hasOp(ops, opDelete, childHost) {
		t.Fatalf("premature opDelete at 100ms: %+v", ops)
	}
	lk, ok := lastLookOp(ops, cardHost)
	if !ok || !nearFloat(lk.opacity, 0.6, 1e-4) {
		t.Fatalf("at 100ms look = (%+v, %v), want opacity 0.6", lk, ok)
	}
	if hasOp(ops, opFrame, cardHost) {
		t.Fatalf("leaving ghost unnecessarily re-emitted unchanged opFrame: %+v", ops)
	}

	// Step to 50% (200ms): opacity should be 0.8 * 0.5 = 0.4.
	t200 := delStart.Add(200 * time.Millisecond)
	tr.stepNodes(t200, onDestroy)
	tr.flushNodeLooks(t200)
	ops = takeOps(t, &tr)
	lk, ok = lastLookOp(ops, cardHost)
	if !ok || !nearFloat(lk.opacity, 0.4, 1e-4) {
		t.Fatalf("at 200ms look = (%+v, %v), want opacity 0.4", lk, ok)
	}

	// Step to completion (400ms): emits opDelete for both child and card, frees host IDs.
	t400 := delStart.Add(400 * time.Millisecond)
	tr.stepNodes(t400, onDestroy)
	tr.flushNodeLooks(t400)
	ops = takeOps(t, &tr)
	if !hasOp(ops, opDelete, childHost) || !hasOp(ops, opDelete, cardHost) {
		t.Fatalf("at 400ms expected opDelete for child (%d) and card (%d), got %+v", childHost, cardHost, ops)
	}
	if tr.hostNode(cardHost) != nil || tr.hostNode(childHost) != nil {
		t.Fatal("hostNode still retains completed ghost")
	}
	if len(tr.leaving) != 0 || len(root.leaving) != 0 || len(tr.motions) != 0 {
		t.Fatalf("leftover state: leaving=%d root.leaving=%d motions=%d", len(tr.leaving), len(root.leaving), len(tr.motions))
	}
	if tr.ids.live() != 1 {
		t.Fatalf("live host IDs = %d, want 1 (root only)", tr.ids.live())
	}
	if !slices.Equal(retired, []driver.NodeID{cardHost, childHost}) {
		t.Fatalf("onRetire called with %v, want [%d %d]", retired, cardHost, childHost)
	}
	if !slices.Equal(destroyed, []driver.NodeID{childHost, cardHost}) {
		t.Fatalf("onDestroy called with %v, want [%d %d]", destroyed, childHost, cardHost)
	}
}

func TestLeavingGhostScaleDelaySpringAndMidEntrance(t *testing.T) {
	t0 := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	var tr watchTree
	tr.initTree(t0)

	tr.batch = 1
	root := tr.createNode(driver.KindView, 1)
	scaled := tr.createNode(driver.KindView, 2)
	delayed := tr.createNode(driver.KindView, 3)
	spring := tr.createNode(driver.KindView, 4)
	tr.insertNode(scaled, root, nil, nil)
	tr.insertNode(delayed, root, nil, nil)
	tr.insertNode(spring, root, nil, nil)
	tr.setRootNode(1)
	tr.setNodeFrame(root, rectBounds(0, 0, 200, 200), nil, t0)
	tr.setNodeFrame(scaled, rectBounds(0, 0, 100, 60), nil, t0)
	tr.setNodeFrame(delayed, rectBounds(0, 60, 80, 40), nil, t0)
	tr.setNodeFrame(spring, rectBounds(0, 100, 80, 40), nil, t0)
	tr.flushNodeLooks(t0)
	_ = takeOps(t, &tr)

	// 1. Scale exit: shrinks around center (50, 30) while keeping opacity 1.
	t1 := t0.Add(time.Second)
	lbScale := tr.newLayoutBatch(&driver.LayoutAnimation{
		Delete: &driver.LayoutTransition{Curve: driver.LayoutLinear, Property: driver.LayoutScale, Duration: 200 * time.Millisecond},
	}, t1, false)
	if !tr.deleteNode(scaled, lbScale, nil, nil) {
		t.Fatal("scaled view did not leave")
	}
	tr.flushNodeLooks(t1)
	_ = takeOps(t, &tr)

	tHalf := t1.Add(100 * time.Millisecond)
	tr.stepNodes(tHalf, nil)
	tr.flushNodeLooks(tHalf)
	ops := takeOps(t, &tr)
	lk, ok := lastLookOp(ops, scaled.hostID)
	if !ok || !nearFloat(lk.opacity, 1, 1e-4) {
		t.Fatalf("scale exit changed opacity: %+v (ok=%v)", lk, ok)
	}
	// Scale 0.5 about center (50, 30) => a=0.5, d=0.5, tx=25, ty=15.
	if !nearFloat(lk.transform[0], 0.5, 1e-3) || !nearFloat(lk.transform[3], 0.5, 1e-3) ||
		!nearFloat(lk.transform[4], 25, 1e-2) || !nearFloat(lk.transform[5], 15, 1e-2) {
		t.Fatalf("scale exit transform = %v, want [0.5 0 0 0.5 25 15]", lk.transform)
	}
	tr.stepNodes(t1.Add(200*time.Millisecond), nil)
	ops = takeOps(t, &tr)
	if !hasOp(ops, opDelete, scaled.hostID) {
		t.Fatalf("scale exit did not emit opDelete at end: %+v", ops)
	}

	// 2. Delayed exit: holds full opacity during delay, fades after delay, deletes at delay+duration.
	t2 := t1.Add(time.Second)
	lbDelay := tr.newLayoutBatch(&driver.LayoutAnimation{
		Delete: &driver.LayoutTransition{Curve: driver.LayoutLinear, Property: driver.LayoutOpacity, Delay: 100 * time.Millisecond, Duration: 200 * time.Millisecond},
	}, t2, false)
	if !tr.deleteNode(delayed, lbDelay, nil, nil) {
		t.Fatal("delayed view did not leave")
	}
	tr.flushNodeLooks(t2)
	_ = takeOps(t, &tr)

	tr.stepNodes(t2.Add(50*time.Millisecond), nil)
	tr.flushNodeLooks(t2.Add(50 * time.Millisecond))
	ops = takeOps(t, &tr)
	if hasOp(ops, opDelete, delayed.hostID) {
		t.Fatal("delayed ghost deleted during delay")
	}
	if op, found := lastLookOp(ops, delayed.hostID); found && !nearFloat(op.opacity, 1, 1e-4) {
		t.Fatalf("delayed ghost faded during delay: opacity=%v", op.opacity)
	}
	tr.stepNodes(t2.Add(200*time.Millisecond), nil)
	tr.flushNodeLooks(t2.Add(200 * time.Millisecond))
	ops = takeOps(t, &tr)
	lk, ok = lastLookOp(ops, delayed.hostID)
	if !ok || !nearFloat(lk.opacity, 0.5, 1e-4) {
		t.Fatalf("delayed ghost at midpoint = (%+v, %v), want opacity 0.5", lk, ok)
	}
	tr.stepNodes(t2.Add(300*time.Millisecond), nil)
	ops = takeOps(t, &tr)
	if !hasOp(ops, opDelete, delayed.hostID) {
		t.Fatal("delayed ghost did not emit opDelete at delay+duration")
	}

	// 3. Spring exit: runs until settled, clamps opacity in [0, 1], then emits opDelete.
	t3 := t2.Add(time.Second)
	springTr := driver.LayoutTransition{Curve: driver.LayoutSpring, Response: 250 * time.Millisecond, DampingRatio: 0.5, Property: driver.LayoutOpacity}
	lbSpring := tr.newLayoutBatch(&driver.LayoutAnimation{Delete: &springTr}, t3, false)
	if !tr.deleteNode(spring, lbSpring, nil, nil) {
		t.Fatal("spring view did not leave")
	}
	tr.flushNodeLooks(t3)
	_ = takeOps(t, &tr)

	for step := 1; step <= 20; step++ {
		at := t3.Add(time.Duration(step) * 20 * time.Millisecond)
		tr.stepNodes(at, nil)
		tr.flushNodeLooks(at)
		for _, o := range takeOps(t, &tr) {
			if o.op == opLook && o.id == spring.hostID && (o.opacity < 0 || o.opacity > 1) {
				t.Fatalf("spring exit opacity out of [0,1]: %v", o.opacity)
			}
		}
	}
	end := t3.Add(springTr.End() + time.Second)
	tr.stepNodes(end, nil)
	ops = takeOps(t, &tr)
	if !hasOp(ops, opDelete, spring.hostID) && tr.hostNode(spring.hostID) != nil {
		t.Fatalf("spring exit did not finish and delete by %v", springTr.End())
	}

	// 4. Mid-entrance deletion: starts leaving smoothly from current appear progress (0.4), not 1.0.
	t4 := end.Add(time.Second)
	lbIn := tr.newLayoutBatch(&driver.LayoutAnimation{
		Create: &driver.LayoutTransition{Curve: driver.LayoutLinear, Property: driver.LayoutOpacity, Duration: 500 * time.Millisecond},
	}, t4, false)
	entering := tr.createNode(driver.KindView, 5)
	lbIn.created[entering] = true
	tr.insertNode(entering, root, nil, lbIn)
	tr.setNodeFrame(entering, rectBounds(0, 0, 50, 50), lbIn, t4)
	tr.flushNodeLooks(t4)
	_ = takeOps(t, &tr)

	tMidIn := t4.Add(200 * time.Millisecond) // part = 0.4
	tr.stepNodes(tMidIn, nil)
	tr.flushNodeLooks(tMidIn)
	_ = takeOps(t, &tr)

	lbOut := tr.newLayoutBatch(&driver.LayoutAnimation{
		Delete: &driver.LayoutTransition{Curve: driver.LayoutLinear, Property: driver.LayoutOpacity, Duration: 200 * time.Millisecond},
	}, tMidIn, false)
	if !tr.deleteNode(entering, lbOut, nil, nil) {
		t.Fatal("entering view did not leave")
	}
	tr.stepNodes(tMidIn.Add(100*time.Millisecond), nil) // halfway out from 0.4 => 0.2
	tr.flushNodeLooks(tMidIn.Add(100 * time.Millisecond))
	ops = takeOps(t, &tr)
	lk, ok = lastLookOp(ops, entering.hostID)
	if !ok || !nearFloat(lk.opacity, 0.2, 1e-4) {
		t.Fatalf("mid-entrance leave opacity = (%+v, %v), want 0.2", lk, ok)
	}
}

func TestImmediateIDReuseDoesNotCollideWithLeavingGhost(t *testing.T) {
	t0 := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	var tr watchTree
	tr.initTree(t0)

	tr.batch = 1
	root := tr.createNode(driver.KindView, 1)
	oldCard := tr.createNode(driver.KindView, 2)
	oldChild := tr.createNode(driver.KindText, 3)
	sibling := tr.createNode(driver.KindView, 4)
	tr.insertNode(oldCard, root, nil, nil)
	tr.insertNode(oldChild, oldCard, nil, nil)
	tr.insertNode(sibling, root, nil, nil)
	tr.setRootNode(1)
	tr.setNodeFrame(root, rectBounds(0, 0, 200, 200), nil, t0)
	tr.setNodeFrame(oldCard, rectBounds(0, 0, 100, 100), nil, t0)
	tr.setNodeFrame(oldChild, rectBounds(10, 10, 30, 20), nil, t0)
	tr.setNodeFrame(sibling, rectBounds(0, 100, 50, 50), nil, t0)
	tr.flushNodeLooks(t0)
	_ = takeOps(t, &tr)

	oldCardHost, oldChildHost, siblingHost := oldCard.hostID, oldChild.hostID, sibling.hostID

	// Same-batch ID reuse: delete 2 & 3 with LayoutAnimation, then immediately recreate ID 2 before sibling 4.
	tr.batch = 2
	t1 := t0.Add(time.Second)
	dur := 400 * time.Millisecond
	lt := &driver.LayoutTransition{Curve: driver.LayoutLinear, Property: driver.LayoutOpacity, Duration: dur}
	lb := tr.newLayoutBatch(&driver.LayoutAnimation{Create: lt, Update: lt, Delete: lt}, t1, false)

	retireCalls := map[*watchNode]int{}
	destroyCalls := map[*watchNode]int{}
	onRetire := func(n *watchNode) { retireCalls[n]++ }
	onDestroy := func(n *watchNode) { destroyCalls[n]++ }

	if !tr.deleteNode(oldCard, lb, onRetire, onDestroy) {
		t.Fatal("oldCard was not retained as leaving ghost")
	}
	if !tr.deleteNode(oldChild, lb, onRetire, onDestroy) {
		t.Fatal("oldChild did not retire with oldCard")
	}

	freshCard := tr.createNode(driver.KindView, 2)
	lb.created[freshCard] = true
	tr.insertNode(freshCard, root, sibling, lb)
	tr.setNodeFrame(freshCard, rectBounds(0, 0, 100, 40), lb, t1)
	tr.setNodeFrame(sibling, rectBounds(0, 40, 50, 50), lb, t1)
	tr.flushNodeLooks(t1)
	ops := takeOps(t, &tr)

	if freshCard.hostID == oldCardHost || freshCard.hostID == oldChildHost {
		t.Fatalf("freshCard hostID %d collided with leaving ghost (%d, %d)", freshCard.hostID, oldCardHost, oldChildHost)
	}
	if tr.node(2) != freshCard || tr.hostNode(freshCard.hostID) != freshCard {
		t.Fatal("views[2] or byHost does not point to freshCard")
	}
	if tr.hostNode(oldCardHost) != oldCard || tr.hostNode(oldChildHost) != oldChild {
		t.Fatal("leaving ghost host entries were clobbered by ID reuse")
	}
	// Verify opInsert used freshCard.hostID and siblingHost (never oldCardHost).
	insertedFresh := false
	for _, o := range ops {
		if o.op == opInsert && o.id == freshCard.hostID && o.parent == root.hostID && o.before == siblingHost {
			insertedFresh = true
		}
	}
	if !insertedFresh {
		t.Fatalf("missing opInsert for freshCard(%d) before sibling(%d): %+v", freshCard.hostID, siblingHost, ops)
	}

	// Next-batch ID reuse while oldCard is STILL animating out: delete sibling 4, then recreate ID 4 in next batch.
	tMid := t1.Add(200 * time.Millisecond)
	tr.stepNodes(tMid, onDestroy)
	tr.flushNodeLooks(tMid)
	ops = takeOps(t, &tr)
	if lk, ok := lastLookOp(ops, oldCardHost); !ok || !nearFloat(lk.opacity, 0.5, 1e-4) {
		t.Fatalf("oldCard mid opacity = (%+v, %v), want 0.5", lk, ok)
	}
	if lk, ok := lastLookOp(ops, freshCard.hostID); !ok || !nearFloat(lk.opacity, 0.5, 1e-4) {
		t.Fatalf("freshCard mid opacity = (%+v, %v), want 0.5", lk, ok)
	}

	tr.batch = 3
	lbDel4 := tr.newLayoutBatch(&driver.LayoutAnimation{Delete: lt}, tMid, false)
	if !tr.deleteNode(sibling, lbDel4, onRetire, onDestroy) {
		t.Fatal("sibling did not leave")
	}
	tr.batch = 4
	fresh4 := tr.createNode(driver.KindView, 4)
	tr.insertNode(fresh4, root, nil, nil)
	tr.setNodeFrame(fresh4, rectBounds(0, 60, 50, 50), nil, tMid)
	if fresh4.hostID == siblingHost || fresh4.hostID == oldCardHost {
		t.Fatalf("next-batch fresh4 hostID %d collided with active ghosts (%d, %d)", fresh4.hostID, siblingHost, oldCardHost)
	}
	tr.flushNodeLooks(tMid)
	_ = takeOps(t, &tr)

	// Finish oldCard (at t1 + 400ms): must delete ONLY oldCard & oldChild, leaving freshCard & fresh4 intact.
	tEnd1 := t1.Add(dur)
	tr.stepNodes(tEnd1, onDestroy)
	tr.flushNodeLooks(tEnd1)
	ops = takeOps(t, &tr)
	if !hasOp(ops, opDelete, oldCardHost) || !hasOp(ops, opDelete, oldChildHost) {
		t.Fatalf("oldCard/oldChild not deleted at end: %+v", ops)
	}
	if hasOp(ops, opDelete, freshCard.hostID) || hasOp(ops, opDelete, fresh4.hostID) || hasOp(ops, opDelete, siblingHost) {
		t.Fatalf("deleted wrong host IDs: %+v", ops)
	}
	if tr.node(2) != freshCard || tr.hostNode(freshCard.hostID) != freshCard {
		t.Fatal("finishing oldCard corrupted freshCard")
	}
	// onRetire must have run exactly once per retired view (never again on bury, so animator.Forget never touches freshCard).
	if retireCalls[oldCard] != 1 || retireCalls[oldChild] != 1 || destroyCalls[oldCard] != 1 || destroyCalls[oldChild] != 1 {
		t.Fatalf("callback counts mismatch: retire=%v destroy=%v", retireCalls, destroyCalls)
	}

	// Finish sibling (at tMid + 400ms).
	tr.stepNodes(tMid.Add(dur), onDestroy)
	ops = takeOps(t, &tr)
	if !hasOp(ops, opDelete, siblingHost) || hasOp(ops, opDelete, fresh4.hostID) {
		t.Fatalf("sibling finish ops = %+v", ops)
	}
	if tr.node(4) != fresh4 || tr.hostNode(fresh4.hostID) != fresh4 {
		t.Fatal("finishing sibling corrupted fresh4")
	}
}

func TestDeleteWithoutLayoutAnimationAndNonEligibleViewsEmitImmediateDelete(t *testing.T) {
	t0 := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	var tr watchTree
	tr.initTree(t0)

	tr.batch = 1
	root := tr.createNode(driver.KindView, 1)
	plain := tr.createNode(driver.KindView, 2)
	hidden := tr.createNode(driver.KindView, 3)
	culled := tr.createNode(driver.KindView, 4)
	zero := tr.createNode(driver.KindView, 5)
	modal := tr.createNode(driver.KindModal, 6)
	matchedOld := tr.createNode(driver.KindView, 7)
	reduced := tr.createNode(driver.KindView, 8)
	instant := tr.createNode(driver.KindView, 9)

	for _, n := range []*watchNode{plain, hidden, culled, zero, modal, matchedOld, reduced, instant} {
		tr.insertNode(n, root, nil, nil)
		tr.setNodeFrame(n, rectBounds(0, 0, 40, 40), nil, t0)
	}
	tr.setRootNode(1)
	tr.setNodeFrame(root, rectBounds(0, 0, 200, 200), nil, t0)
	hidden.style.Hidden = true
	culled.culled = true
	tr.setNodeFrame(zero, rectBounds(0, 0, 0, 0), nil, t0)
	tr.flushNodeLooks(t0)
	_ = takeOps(t, &tr)

	t1 := t0.Add(time.Second)

	// 1. No layout animation -> immediate opDelete.
	if tr.deleteNode(plain, nil, nil, nil) {
		t.Fatal("plain delete without LayoutAnimation retained a ghost")
	}
	if ops := takeOps(t, &tr); !hasOp(ops, opDelete, plain.hostID) || hasOp(ops, opRetire, plain.hostID) {
		t.Fatalf("plain delete ops = %+v, want immediate opDelete only", ops)
	}

	// 2. ReduceMotion -> newLayoutBatch returns nil -> immediate opDelete.
	lt := &driver.LayoutTransition{Curve: driver.LayoutLinear, Duration: 300 * time.Millisecond}
	lbReduced := tr.newLayoutBatch(&driver.LayoutAnimation{Delete: lt}, t1, true)
	if lbReduced != nil {
		t.Fatal("reduced motion produced non-nil layoutBatch")
	}
	if tr.deleteNode(reduced, lbReduced, nil, nil) {
		t.Fatal("reduced motion retained a ghost")
	}
	if ops := takeOps(t, &tr); !hasOp(ops, opDelete, reduced.hostID) {
		t.Fatalf("reduced motion ops = %+v, want immediate opDelete", ops)
	}

	// 3. Hidden, culled, zero-size, modal, and matched old views -> immediate opDelete.
	lb := tr.newLayoutBatch(&driver.LayoutAnimation{Update: lt, Delete: lt}, t1, false)
	matchedNew := tr.createNode(driver.KindView, 10)
	lb.created[matchedNew] = true
	tr.insertNode(matchedNew, root, nil, lb)
	tr.setNodeFrame(matchedNew, rectBounds(50, 50, 40, 40), lb, t1)
	tr.matchNode(matchedNew, matchedOld, lb)
	_ = takeOps(t, &tr)

	for name, n := range map[string]*watchNode{
		"hidden":     hidden,
		"culled":     culled,
		"zero":       zero,
		"modal":      modal,
		"matchedOld": matchedOld,
	} {
		if tr.deleteNode(n, lb, nil, nil) {
			t.Fatalf("%s view unexpectedly retained a leaving ghost", name)
		}
		if ops := takeOps(t, &tr); !hasOp(ops, opDelete, n.hostID) || hasOp(ops, opRetire, n.hostID) {
			t.Fatalf("%s delete ops = %+v, want immediate opDelete", name, ops)
		}
	}

	// 4. Zero-duration, zero-delay Delete transition -> immediate opDelete.
	lbInstant := tr.newLayoutBatch(&driver.LayoutAnimation{Delete: &driver.LayoutTransition{Curve: driver.LayoutLinear}}, t1, false)
	if tr.deleteNode(instant, lbInstant, nil, nil) {
		t.Fatal("zero-duration Delete retained a ghost")
	}
	if ops := takeOps(t, &tr); !hasOp(ops, opDelete, instant.hostID) {
		t.Fatalf("instant delete ops = %+v, want immediate opDelete", ops)
	}
	if len(tr.leaving) != 0 {
		t.Fatalf("leaving = %d, want 0", len(tr.leaving))
	}
}

func TestHitTestIgnoresLeavingGhosts(t *testing.T) {
	t0 := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	var tr watchTree
	tr.initTree(t0)

	root := tr.createNode(driver.KindView, 1)
	under := tr.createNode(driver.KindView, 2)
	overlay := tr.createNode(driver.KindView, 3)
	badge := tr.createNode(driver.KindView, 4)
	under.events = 1 << driver.EventPress
	overlay.events = 1 << driver.EventPress
	badge.events = 1 << driver.EventPress

	tr.insertNode(under, root, nil, nil)
	tr.insertNode(overlay, root, nil, nil)
	tr.insertNode(badge, overlay, nil, nil)
	tr.setRootNode(1)
	tr.setNodeFrame(root, rectBounds(0, 0, 200, 200), nil, t0)
	tr.setNodeFrame(under, rectBounds(0, 0, 100, 100), nil, t0)
	tr.setNodeFrame(overlay, rectBounds(0, 0, 100, 100), nil, t0)
	tr.setNodeFrame(badge, rectBounds(10, 10, 40, 40), nil, t0)

	pressMatch := func(n *watchNode) bool { return n.events.Has(driver.EventPress) }
	if got := tr.hitNode(root, driver.Point{X: 25, Y: 25}, t0, pressMatch); got != badge {
		t.Fatalf("before delete hit = %v, want badge", got)
	}

	// Delete overlay + badge with an active layout animation.
	t1 := t0.Add(time.Second)
	lb := tr.newLayoutBatch(&driver.LayoutAnimation{
		Delete: &driver.LayoutTransition{Curve: driver.LayoutLinear, Duration: 500 * time.Millisecond},
	}, t1, false)
	if !tr.deleteNode(overlay, lb, nil, nil) || !tr.deleteNode(badge, lb, nil, nil) {
		t.Fatal("overlay/badge did not retire as leaving ghost")
	}

	// Hit-testing must immediately ignore the leaving ghost and its retired child, reaching under.
	if got := tr.hitNode(root, driver.Point{X: 25, Y: 25}, t1, pressMatch); got != under {
		t.Fatalf("after delete hit = %v, want under (%v)", got, under)
	}
	if got := tr.hitNode(overlay, driver.Point{X: 25, Y: 25}, t1, pressMatch); got != nil {
		t.Fatalf("direct hitNode on retired ghost returned %v, want nil", got)
	}

	// Reusing ID 3 elsewhere still hit-tests only the new view, never the ghost.
	fresh := tr.createNode(driver.KindView, 3)
	fresh.events = 1 << driver.EventPress
	tr.insertNode(fresh, root, nil, nil)
	tr.setNodeFrame(fresh, rectBounds(120, 120, 50, 50), nil, t1)
	if got := tr.hitNode(root, driver.Point{X: 130, Y: 130}, t1, pressMatch); got != fresh {
		t.Fatalf("hit on reused ID = %v, want fresh", got)
	}
	if got := tr.hitNode(root, driver.Point{X: 25, Y: 25}, t1, pressMatch); got != under {
		t.Fatalf("hit under leaving ghost after ID reuse = %v, want under", got)
	}
}

func TestAncestorDeletionAndSurfaceTeardownReapGhosts(t *testing.T) {
	t0 := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	var tr watchTree
	tr.initTree(t0)

	root := tr.createNode(driver.KindView, 1)
	section := tr.createNode(driver.KindView, 2)
	item := tr.createNode(driver.KindView, 3)
	badge := tr.createNode(driver.KindText, 4)
	other := tr.createNode(driver.KindView, 5)
	otherChild := tr.createNode(driver.KindText, 6)
	tr.insertNode(section, root, nil, nil)
	tr.insertNode(item, section, nil, nil)
	tr.insertNode(badge, item, nil, nil)
	tr.insertNode(other, root, nil, nil)
	tr.insertNode(otherChild, other, nil, nil)
	tr.setRootNode(1)
	tr.setNodeFrame(root, rectBounds(0, 0, 200, 200), nil, t0)
	tr.setNodeFrame(section, rectBounds(0, 0, 100, 100), nil, t0)
	tr.setNodeFrame(item, rectBounds(0, 0, 80, 40), nil, t0)
	tr.setNodeFrame(badge, rectBounds(5, 5, 20, 10), nil, t0)
	tr.setNodeFrame(other, rectBounds(0, 100, 80, 40), nil, t0)
	tr.setNodeFrame(otherChild, rectBounds(5, 5, 20, 10), nil, t0)
	tr.flushNodeLooks(t0)
	_ = takeOps(t, &tr)

	t1 := t0.Add(time.Second)
	lb := tr.newLayoutBatch(&driver.LayoutAnimation{
		Delete: &driver.LayoutTransition{Curve: driver.LayoutLinear, Duration: 500 * time.Millisecond},
	}, t1, false)
	if !tr.deleteNode(item, lb, nil, nil) || !tr.deleteNode(badge, lb, nil, nil) {
		t.Fatal("item/badge did not retire")
	}
	if !tr.deleteNode(other, lb, nil, nil) || !tr.deleteNode(otherChild, lb, nil, nil) {
		t.Fatal("other/otherChild did not retire")
	}
	_ = takeOps(t, &tr)

	// Deleting ancestor section without layout animation must reap item & badge ghosts too.
	var destroyed []driver.NodeID
	onDestroy := func(n *watchNode) { destroyed = append(destroyed, n.hostID) }
	if tr.deleteNode(section, nil, nil, onDestroy) {
		t.Fatal("section deleted without animation should not leave")
	}
	ops := takeOps(t, &tr)
	for _, id := range []driver.NodeID{badge.hostID, item.hostID, section.hostID} {
		if !hasOp(ops, opDelete, id) {
			t.Fatalf("ancestor delete missed opDelete for hostID %d: %+v", id, ops)
		}
		if tr.hostNode(id) != nil {
			t.Fatalf("hostID %d still in byHost after ancestor delete", id)
		}
	}
	if len(tr.leaving) != 1 || tr.leaving[0] != other {
		t.Fatalf("remaining leaving ghosts = %v, want [other]", tr.leaving)
	}

	// Surface teardown must reap every remaining leaving ghost and retired descendant.
	destroyed = nil
	tr.closeLeaving(onDestroy)
	ops = takeOps(t, &tr)
	if !hasOp(ops, opDelete, otherChild.hostID) || !hasOp(ops, opDelete, other.hostID) {
		t.Fatalf("closeLeaving missed opDelete for remaining ghosts: %+v", ops)
	}
	if len(tr.leaving) != 0 || len(root.leaving) != 0 || len(tr.motions) != 0 {
		t.Fatalf("leftover state after teardown: leaving=%d root.leaving=%d motions=%d",
			len(tr.leaving), len(root.leaving), len(tr.motions))
	}
	if tr.ids.live() != 1 || tr.hostNode(other.hostID) != nil || tr.hostNode(otherChild.hostID) != nil {
		t.Fatalf("leaked host IDs after teardown: live=%d", tr.ids.live())
	}
	if !slices.Equal(destroyed, []driver.NodeID{otherChild.hostID, other.hostID}) {
		t.Fatalf("teardown onDestroy = %v, want [%d %d]", destroyed, otherChild.hostID, other.hostID)
	}
}
