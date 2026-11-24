//go:build darwin

package apple

import (
	"fmt"
	"log"
	"math/bits"
	"math/rand/v2"
	"os"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/appstartup"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/privacy"
)

// Local Network privacy, which iOS and tvOS have from 14 and macOS from 15, has no API that tells or asks it: an app learns it by using the local network. The probe advertises a Bonjour service of NSGo's own type and browses for it, which shows the system's prompt the first time. Finding its own instance means the system lets the app through, and DNS-SD's kDNSServiceErr_PolicyDenied means the user refused; while the prompt shows, neither comes, so the probe waits.

const (
	localNetworkUsageKey = "NSLocalNetworkUsageDescription"
	// localNetworkWait is how long a probe waits for the user before answering unknown.
	localNetworkWait = 2 * time.Minute
	// localNetworkSettle is how long the app is active again before a refused probe tries once more, and localNetworkPoll how often it looks while the app is not.
	localNetworkSettle = 500 * time.Millisecond
	localNetworkPoll   = 250 * time.Millisecond
	// The probe's port, which nothing connects to: DNS-SD advertises no service on port 0.
	localNetworkPort = 9

	dnsServiceErrNoAuth       = -65555
	dnsServiceErrPolicyDenied = -65570
	dnsServiceFlagsAdd        = 1 << 1
)

var localNetwork struct {
	// learned is the status the last answered probe learned, plus one; 0 until one has. Any goroutine reads it.
	learned atomic.Int32
	// probe is the one asking now, nil for none. Main thread.
	probe *localNetworkProbe
}

var probeRegisterReply, probeBrowseReply uintptr

// localNetworkProbe is one request's advertisement and browse, and the requests waiting for its answer. Main thread.
type localNetworkProbe struct {
	name             string
	register, browse uintptr
	waiters          []func(int)
	deadline         *time.Timer
	// retried says a refusal came once already: DNS-SD may refuse while the prompt still shows, so the first refusal tries again once the app is active.
	retried bool
}

// LocalNetworkStatus is what the app knows of Local Network access without asking: not gated where the system keeps none from it, what a request in this run learned, and unknown before one has, as the system never tells it. Any goroutine.
func LocalNetworkStatus() int {
	if !LocalNetworkGated() {
		return driver.PermissionNotGated
	}
	if v := localNetwork.learned.Load(); v != 0 {
		return int(v - 1)
	}
	return driver.PermissionUnknown
}

// RequestLocalNetwork asks for Local Network access and calls answer with the status, on the main thread: granted once the probe finds its own instance, denied once DNS-SD refuses it, and unknown when the user has not answered within localNetworkWait. Each request probes again, as the user may change the setting at any time; a request while one probes waits for its answer. Without NSLocalNetworkUsageDescription in the Info.plist it logs the key and answers unavailable. Main thread.
func RequestLocalNetwork(answer func(int)) {
	if !LocalNetworkGated() {
		answer(driver.PermissionNotGated)
		return
	}
	if !privacy.DeclaresUsage(localNetworkUsageKey) {
		log.Printf("nsgo: requesting %s needs %s in the app's Info.plist (nsgo.yaml's apple.info)", driver.PermissionLocalNetwork, localNetworkUsageKey)
		answer(driver.PermissionUnavailable)
		return
	}
	probeLocalNetwork(answer)
}

// probeLocalNetwork has the probe asking now, or a new one, answer. Main thread.
func probeLocalNetwork(answer func(int)) {
	if p := localNetwork.probe; p != nil {
		p.waiters = append(p.waiters, answer)
		return
	}
	p := &localNetworkProbe{waiters: []func(int){answer}}
	localNetwork.probe = p
	p.deadline = time.AfterFunc(localNetworkWait, func() { OnMain(func() { p.finish(driver.PermissionUnknown) }) })
	p.start()
}

func loadLocalNetworkProbe() {
	loadBonjour()
	if probeRegisterReply == 0 {
		probeRegisterReply = purego.NewCallback(probeRegistered)
		probeBrowseReply = purego.NewCallback(probeBrowsed)
	}
}

// start advertises the probe's instance and browses for it.
func (p *localNetworkProbe) start() {
	loadLocalNetworkProbe()
	p.name = fmt.Sprintf("nsgo-%d-%08x", os.Getpid(), rand.Uint32())
	const local = "local."
	// Both Apple Go targets are little-endian; the ABI takes network byte order.
	port := bits.ReverseBytes16(localNetworkPort)
	if status := bonjourRegister(&p.register, 0, 0, p.name, appstartup.LocalNetworkProbeType, local, nil, port, 0, nil, probeRegisterReply, 0); status != 0 {
		p.register = 0
		p.failed(status)
		return
	}
	if status := bonjourQueue(p.register, mainQueue); status != 0 {
		p.failed(status)
		return
	}
	if status := bonjourBrowse(&p.browse, 0, 0, appstartup.LocalNetworkProbeType, local, probeBrowseReply, 0); status != 0 {
		p.browse = 0
		p.failed(status)
		return
	}
	if status := bonjourQueue(p.browse, mainQueue); status != 0 {
		p.failed(status)
	}
}

// release ends the probe's DNS-SD operations.
func (p *localNetworkProbe) release() {
	if p.register != 0 {
		bonjourRelease(p.register)
		p.register = 0
	}
	if p.browse != 0 {
		bonjourRelease(p.browse)
		p.browse = 0
	}
}

// live says p is the probe asking now.
func (p *localNetworkProbe) live() bool { return localNetwork.probe == p }

// failed answers DNS-SD's error status: a refusal, a type the Info.plist does not list, or any other failure, which tells nothing.
func (p *localNetworkProbe) failed(status int32) {
	p.release()
	switch status {
	case dnsServiceErrPolicyDenied:
		if p.retried {
			p.finish(driver.PermissionDenied)
			return
		}
		p.retried = true
		p.whenActive(p.start)
	case dnsServiceErrNoAuth:
		log.Printf("nsgo: requesting %s needs %s in the app's NSBonjourServices, which nsgo build adds with %s", driver.PermissionLocalNetwork, appstartup.LocalNetworkProbeType, localNetworkUsageKey)
		p.finish(driver.PermissionUnavailable)
	default:
		p.finish(driver.PermissionUnknown)
	}
}

// whenActive runs fn once the app has been active for localNetworkSettle, as the prompt makes an iOS app inactive while it shows, unless the probe has ended by then.
func (p *localNetworkProbe) whenActive(fn func()) {
	wait, next := localNetworkSettle, fn
	if !appActive() {
		wait, next = localNetworkPoll, func() { p.whenActive(fn) }
	}
	time.AfterFunc(wait, func() {
		OnMain(func() {
			if p.live() {
				next()
			}
		})
	})
}

// finish ends the probe and answers its requests with status, which a later check answers when it tells something.
func (p *localNetworkProbe) finish(status int) {
	if !p.live() {
		return
	}
	localNetwork.probe = nil
	p.deadline.Stop()
	p.release()
	if status == driver.PermissionGranted || status == driver.PermissionDenied {
		localNetwork.learned.Store(int32(status) + 1)
	}
	for _, answer := range p.waiters {
		answer(status)
	}
}

func probeRegistered(ref uintptr, _ uint32, status int32, name, _, _ *byte, _ uintptr) {
	p := localNetwork.probe
	if p == nil || ref != p.register {
		return
	}
	if status != 0 {
		p.failed(status)
		return
	}
	// The system may have renamed the instance after a conflict.
	if n := bonjourString(name); n != "" {
		p.name = n
	}
}

func probeBrowsed(ref uintptr, flags, _ uint32, status int32, name, _, _ *byte, _ uintptr) {
	p := localNetwork.probe
	if p == nil || ref != p.browse {
		return
	}
	if status != 0 {
		p.failed(status)
		return
	}
	if flags&dnsServiceFlagsAdd != 0 && bonjourString(name) == p.name {
		p.finish(driver.PermissionGranted)
	}
}
