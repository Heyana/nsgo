//go:build darwin && !nsgowatch

package gameservices

import (
	"fmt"
	"structs"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

const (
	gameKitPath          = "/System/Library/Frameworks/GameKit.framework/GameKit"
	nsUTF8StringEncoding = 4

	// GKErrorCode values from GameKit's GKError.h.
	gkErrorCancelled        = 2
	gkErrorNotAuthenticated = 6

	// GKGameCenterViewControllerState values.
	gkVCStateLeaderboards = 0
	gkVCStateAchievements = 1

	// GKLeaderboardPlayerScope values.
	gkPlayerScopeGlobal  = 0
	gkPlayerScopeFriends = 1

	// GKLeaderboardTimeScope values.
	gkTimeScopeToday   = 0
	gkTimeScopeWeek    = 1
	gkTimeScopeAllTime = 2
)

type nsRange struct {
	_                structs.HostLayout
	Location, Length uint
}

func sel(name string) objc.SEL { return objc.RegisterName(name) }

func responds(obj objc.ID, s objc.SEL) bool {
	return obj != 0 && objc.Send[bool](obj, sel("respondsToSelector:"), s)
}

var loadGameKit = sync.OnceValue(func() bool {
	if _, err := purego.Dlopen(gameKitPath, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		return false
	}
	return objc.GetClass("GKLocalPlayer") != 0
})

var (
	mainOnce       sync.Once
	mainMu         sync.Mutex
	mainFuncs      []func()
	dispatchAsyncF func(queue, ctx, work uintptr)
	mainQueue      uintptr
	mainWork       uintptr
)

func initMainDispatch() {
	lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return
	}
	q, err := purego.Dlsym(lib, "_dispatch_main_q")
	if err != nil {
		return
	}
	purego.RegisterLibFunc(&dispatchAsyncF, lib, "dispatch_async_f")
	mainQueue = q
	mainWork = purego.NewCallback(func(uintptr) {
		mainMu.Lock()
		fns := mainFuncs
		mainFuncs = nil
		mainMu.Unlock()
		for _, fn := range fns {
			fn()
		}
	})
}

func onMain(fn func()) {
	mainOnce.Do(initMainDispatch)
	if dispatchAsyncF == nil || mainQueue == 0 {
		fn()
		return
	}
	mainMu.Lock()
	mainFuncs = append(mainFuncs, fn)
	first := len(mainFuncs) == 1
	mainMu.Unlock()
	if first {
		dispatchAsyncF(mainQueue, 0, mainWork)
	}
}

// newNSString returns a retained NSString containing s. Callers release it.
func newNSString(s string) objc.ID {
	cls := objc.GetClass("NSString")
	if cls == 0 {
		return 0
	}
	alloc := objc.ID(cls).Send(sel("alloc"))
	if len(s) == 0 {
		return alloc.Send(sel("init"))
	}
	b := []byte(s)
	return alloc.Send(sel("initWithBytes:length:encoding:"), unsafe.Pointer(unsafe.SliceData(b)), uint(len(b)), uint(nsUTF8StringEncoding))
}

func goString(ns objc.ID) string {
	if ns == 0 || !responds(ns, sel("UTF8String")) {
		return ""
	}
	p := objc.Send[*byte](ns, sel("UTF8String"))
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

func nsError(err objc.ID) error {
	if err == 0 {
		return nil
	}
	code := 0
	if responds(err, sel("code")) {
		code = objc.Send[int](err, sel("code"))
	}
	desc := ""
	if responds(err, sel("localizedDescription")) {
		desc = goString(err.Send(sel("localizedDescription")))
	}
	switch code {
	case gkErrorCancelled:
		return ErrCancelled
	case gkErrorNotAuthenticated:
		return ErrUnauthenticated
	}
	if desc != "" {
		return fmt.Errorf("%w: %s", ErrUnavailable, desc)
	}
	return ErrUnavailable
}

func localPlayer() objc.ID {
	cls := objc.GetClass("GKLocalPlayer")
	if cls == 0 || !responds(objc.ID(cls), sel("localPlayer")) {
		return 0
	}
	return objc.ID(cls).Send(sel("localPlayer"))
}

func isAuthenticated(lp objc.ID) bool {
	return lp != 0 && responds(lp, sel("isAuthenticated")) && objc.Send[bool](lp, sel("isAuthenticated"))
}

func readPlayer(p objc.ID) Player {
	if p == 0 {
		return Player{}
	}
	var id string
	switch {
	case responds(p, sel("gamePlayerID")):
		id = goString(p.Send(sel("gamePlayerID")))
	case responds(p, sel("teamPlayerID")):
		id = goString(p.Send(sel("teamPlayerID")))
	case responds(p, sel("playerID")):
		id = goString(p.Send(sel("playerID")))
	}
	var name string
	switch {
	case responds(p, sel("displayName")):
		name = goString(p.Send(sel("displayName")))
	case responds(p, sel("alias")):
		name = goString(p.Send(sel("alias")))
	}
	return Player{
		ID:            id,
		DisplayName:   name,
		Authenticated: isAuthenticated(p),
	}
}

func gkPlayerScope(scope PlayerScope) int {
	if scope == Friends {
		return gkPlayerScopeFriends
	}
	return gkPlayerScopeGlobal
}

func gkTimeScope(scope TimeScope) int {
	switch scope {
	case Today:
		return gkTimeScopeToday
	case Week:
		return gkTimeScopeWeek
	}
	return gkTimeScopeAllTime
}

var gameCenterDelegate = sync.OnceValue(func() objc.ID {
	var protocols []*objc.Protocol
	if p := objc.GetProtocol("GKGameCenterControllerDelegate"); p != nil {
		protocols = append(protocols, p)
	}
	cls, err := objc.RegisterClass("NsgoGameCenterDelegate", objc.GetClass("NSObject"), protocols, nil, []objc.MethodDef{
		{
			Cmd: sel("gameCenterViewControllerDidFinish:"),
			Fn: func(_ objc.ID, _ objc.SEL, vc objc.ID) {
				dismissGameCenterVC(vc)
			},
		},
	})
	if err != nil {
		cls = objc.GetClass("NsgoGameCenterDelegate")
	}
	if cls == 0 {
		return 0
	}
	return objc.ID(cls).Send(sel("new"))
})

func platformAuthenticate(done func(Player, error)) {
	onMain(func() {
		if !loadGameKit() {
			done(Player{}, ErrUnavailable)
			return
		}
		lp := localPlayer()
		if lp == 0 {
			done(Player{}, ErrUnavailable)
			return
		}
		if isAuthenticated(lp) {
			done(readPlayer(lp), nil)
			return
		}
		if !responds(lp, sel("setAuthenticateHandler:")) {
			done(Player{}, ErrUnavailable)
			return
		}
		var once sync.Once
		finish := func(p Player, err error) {
			once.Do(func() { done(p, err) })
		}
		block := objcblock.New(func(_ objc.Block, vc, err objc.ID) {
			if vc != 0 {
				onMain(func() { presentGameCenterVC(vc) })
				return
			}
			if isAuthenticated(lp) {
				finish(readPlayer(lp), nil)
				return
			}
			if err != 0 {
				finish(Player{}, nsError(err))
				return
			}
			finish(Player{}, ErrUnauthenticated)
		})
		lp.Send(sel("setAuthenticateHandler:"), block)
		block.Release()
	})
}

func platformReportAchievement(id string, percent float64, done func(error)) {
	onMain(func() {
		if !loadGameKit() {
			done(ErrUnavailable)
			return
		}
		lp := localPlayer()
		if !isAuthenticated(lp) {
			done(ErrUnauthenticated)
			return
		}
		achCls := objc.GetClass("GKAchievement")
		if achCls == 0 || !responds(objc.ID(achCls), sel("reportAchievements:withCompletionHandler:")) {
			done(ErrUnavailable)
			return
		}
		alloc := objc.ID(achCls).Send(sel("alloc"))
		if !responds(alloc, sel("initWithIdentifier:")) {
			alloc.Send(sel("release"))
			done(ErrUnavailable)
			return
		}
		nsID := newNSString(id)
		defer nsID.Send(sel("release"))
		ach := alloc.Send(sel("initWithIdentifier:"), nsID)
		if ach == 0 {
			done(ErrUnavailable)
			return
		}
		if responds(ach, sel("setPercentComplete:")) {
			ach.Send(sel("setPercentComplete:"), percent)
		}
		if percent >= 100 && responds(ach, sel("setShowsCompletionBanner:")) {
			ach.Send(sel("setShowsCompletionBanner:"), true)
		}
		arr := objc.ID(objc.GetClass("NSArray")).Send(sel("arrayWithObject:"), ach)
		block := objcblock.New(func(_ objc.Block, err objc.ID) {
			onMain(func() { ach.Send(sel("release")) })
			if err != 0 {
				done(nsError(err))
				return
			}
			done(nil)
		})
		objc.ID(achCls).Send(sel("reportAchievements:withCompletionHandler:"), arr, block)
		block.Release()
	})
}

func platformSubmitScore(leaderboardID string, score int64, done func(error)) {
	onMain(func() {
		if !loadGameKit() {
			done(ErrUnavailable)
			return
		}
		lp := localPlayer()
		if !isAuthenticated(lp) {
			done(ErrUnauthenticated)
			return
		}
		nsID := newNSString(leaderboardID)
		defer nsID.Send(sel("release"))

		// Prefer the modern iOS 14 / macOS 11 GKLeaderboard submission API.
		if lbCls := objc.GetClass("GKLeaderboard"); lbCls != 0 &&
			responds(objc.ID(lbCls), sel("submitScore:context:player:leaderboardIDs:completionHandler:")) {
			ids := objc.ID(objc.GetClass("NSArray")).Send(sel("arrayWithObject:"), nsID)
			block := objcblock.New(func(_ objc.Block, err objc.ID) {
				if err != 0 {
					done(nsError(err))
					return
				}
				done(nil)
			})
			objc.ID(lbCls).Send(sel("submitScore:context:player:leaderboardIDs:completionHandler:"),
				int(score), uint(0), lp, ids, block)
			block.Release()
			return
		}

		// Legacy GKScore fallback for older OS versions.
		scoreCls := objc.GetClass("GKScore")
		if scoreCls == 0 || !responds(objc.ID(scoreCls), sel("reportScores:withCompletionHandler:")) {
			done(ErrUnavailable)
			return
		}
		alloc := objc.ID(scoreCls).Send(sel("alloc"))
		if !responds(alloc, sel("initWithLeaderboardIdentifier:")) {
			alloc.Send(sel("release"))
			done(ErrUnavailable)
			return
		}
		gkScore := alloc.Send(sel("initWithLeaderboardIdentifier:"), nsID)
		if gkScore == 0 {
			done(ErrUnavailable)
			return
		}
		if responds(gkScore, sel("setValue:")) {
			gkScore.Send(sel("setValue:"), score)
		}
		arr := objc.ID(objc.GetClass("NSArray")).Send(sel("arrayWithObject:"), gkScore)
		block := objcblock.New(func(_ objc.Block, err objc.ID) {
			onMain(func() { gkScore.Send(sel("release")) })
			if err != 0 {
				done(nsError(err))
				return
			}
			done(nil)
		})
		objc.ID(scoreCls).Send(sel("reportScores:withCompletionHandler:"), arr, block)
		block.Release()
	})
}

func platformShowAchievements() {
	onMain(func() {
		if !loadGameKit() || !isAuthenticated(localPlayer()) {
			return
		}
		vcCls := objc.GetClass("GKGameCenterViewController")
		if vcCls == 0 {
			return
		}
		alloc := objc.ID(vcCls).Send(sel("alloc"))
		var vc objc.ID
		switch {
		case responds(alloc, sel("initWithState:")):
			vc = alloc.Send(sel("initWithState:"), int(gkVCStateAchievements))
		case responds(alloc, sel("init")):
			vc = alloc.Send(sel("init"))
			if responds(vc, sel("setViewState:")) {
				vc.Send(sel("setViewState:"), int(gkVCStateAchievements))
			}
		default:
			alloc.Send(sel("release"))
			return
		}
		if vc == 0 {
			return
		}
		if d := gameCenterDelegate(); d != 0 && responds(vc, sel("setGameCenterDelegate:")) {
			vc.Send(sel("setGameCenterDelegate:"), d)
		}
		presentGameCenterVC(vc)
		vc.Send(sel("release"))
	})
}

func platformShowLeaderboard(leaderboardID string) {
	onMain(func() {
		if !loadGameKit() || !isAuthenticated(localPlayer()) {
			return
		}
		vcCls := objc.GetClass("GKGameCenterViewController")
		if vcCls == 0 {
			return
		}
		nsID := newNSString(leaderboardID)
		defer nsID.Send(sel("release"))
		alloc := objc.ID(vcCls).Send(sel("alloc"))
		var vc objc.ID
		switch {
		case responds(alloc, sel("initWithLeaderboardID:playerScope:timeScope:")):
			vc = alloc.Send(sel("initWithLeaderboardID:playerScope:timeScope:"),
				nsID, int(gkPlayerScopeGlobal), int(gkTimeScopeAllTime))
		case responds(alloc, sel("init")):
			vc = alloc.Send(sel("init"))
			if responds(vc, sel("setViewState:")) {
				vc.Send(sel("setViewState:"), int(gkVCStateLeaderboards))
			}
			if responds(vc, sel("setLeaderboardIdentifier:")) {
				vc.Send(sel("setLeaderboardIdentifier:"), nsID)
			}
		default:
			alloc.Send(sel("release"))
			return
		}
		if vc == 0 {
			return
		}
		if d := gameCenterDelegate(); d != 0 && responds(vc, sel("setGameCenterDelegate:")) {
			vc.Send(sel("setGameCenterDelegate:"), d)
		}
		presentGameCenterVC(vc)
		vc.Send(sel("release"))
	})
}

func platformLoadScores(leaderboardID string, scope PlayerScope, timeScope TimeScope, limit int, done func([]ScoreEntry, error)) {
	onMain(func() {
		if !loadGameKit() {
			done(nil, ErrUnavailable)
			return
		}
		if !isAuthenticated(localPlayer()) {
			done(nil, ErrUnauthenticated)
			return
		}
		lbCls := objc.GetClass("GKLeaderboard")
		if lbCls == 0 || !responds(objc.ID(lbCls), sel("loadLeaderboardsWithIDs:completionHandler:")) {
			done(nil, ErrUnavailable)
			return
		}
		nsID := newNSString(leaderboardID)
		defer nsID.Send(sel("release"))
		ids := objc.ID(objc.GetClass("NSArray")).Send(sel("arrayWithObject:"), nsID)
		loadBlock := objcblock.New(func(_ objc.Block, boards, err objc.ID) {
			if err != 0 {
				done(nil, nsError(err))
				return
			}
			if boards == 0 || !responds(boards, sel("count")) || objc.Send[uint](boards, sel("count")) == 0 {
				done(nil, nil)
				return
			}
			lb := boards.Send(sel("objectAtIndex:"), uint(0))
			if !responds(lb, sel("loadEntriesForPlayerScope:timeScope:range:completionHandler:")) {
				done(nil, ErrUnavailable)
				return
			}
			entriesBlock := objcblock.New(func(_ objc.Block, _ objc.ID, entries objc.ID, _ int, err objc.ID) {
				if err != 0 {
					done(nil, nsError(err))
					return
				}
				if entries == 0 || !responds(entries, sel("count")) {
					done(nil, nil)
					return
				}
				n := objc.Send[uint](entries, sel("count"))
				out := make([]ScoreEntry, 0, n)
				for i := range n {
					e := entries.Send(sel("objectAtIndex:"), i)
					if e == 0 {
						continue
					}
					entry := ScoreEntry{LeaderboardID: leaderboardID}
					if responds(e, sel("score")) {
						entry.Score = int64(objc.Send[int](e, sel("score")))
					}
					if responds(e, sel("rank")) {
						entry.Rank = objc.Send[int](e, sel("rank"))
					}
					if responds(e, sel("player")) {
						p := readPlayer(e.Send(sel("player")))
						entry.PlayerID = p.ID
						entry.DisplayName = p.DisplayName
					}
					out = append(out, entry)
				}
				done(out, nil)
			})
			lb.Send(sel("loadEntriesForPlayerScope:timeScope:range:completionHandler:"),
				gkPlayerScope(scope), gkTimeScope(timeScope), nsRange{Location: 1, Length: uint(limit)}, entriesBlock)
			entriesBlock.Release()
		})
		objc.ID(lbCls).Send(sel("loadLeaderboardsWithIDs:completionHandler:"), ids, loadBlock)
		loadBlock.Release()
	})
}
