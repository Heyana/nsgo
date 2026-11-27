// Host.swift: the SwiftUI app an Apple Watch runs an NSGo app in, which
// nsgo links into the app's Go executable for watchOS. watchOS has no
// public UIKit or Core Animation, so the views the engine mounts are
// SwiftUI's: Go's main runs the app here with nsgo_watch_main, applies
// each commit to the Node tree through nsgo_watch_apply and draws each
// node's look and content with Core Graphics into its Canvas; this file
// places them, scrolls, edits text and shows alerts, and reports touches,
// sizes and scrolls back to Go.

import SwiftUI
import Observation
import ImageIO
import AuthenticationServices
import AVFAudio
import EventKit
import CoreBluetooth

// MARK: The bridge

/// What Go gives the host as nsgo_watch_connect's table, in this order.
struct Bridge {
	/// The surface's size, scale, safe area and traits.
	var size: @convention(c) (Double, Double, Double, Double, Double, Double, Double, UInt32) -> Void
	/// A touch: its phase (0 began, 1 moved, 2 ended, 3 cancelled), where
	/// it is on the surface and when, in seconds.
	var touch: @convention(c) (UInt8, Double, Double, Double) -> Void
	/// Draws node id's look and content into a context w by h, its y down.
	var draw: @convention(c) (UInt32, UnsafeMutableRawPointer, Double, Double) -> Void
	/// A scroll view's offset, and its phase (0 offset alone, 1 dragging,
	/// 2 gliding, 3 at rest, 4 animating).
	var scroll: @convention(c) (UInt32, Double, Double, UInt8) -> Void
	/// A display frame began, at seconds.
	var frame: @convention(c) (Double) -> Void
	/// A field's text changed (0), was submitted (1), took the focus (2)
	/// or lost it (3).
	var text: @convention(c) (UInt32, UInt8, UnsafePointer<UInt8>?, Int) -> Void
	/// Alert id was answered with the action at index.
	var alert: @convention(c) (UInt32, Int32) -> Void
	/// The app became active (0), inactive (1) or went to the background (2).
	var lifecycle: @convention(c) (UInt8) -> Void
	/// The PNG nsgo_watch_snapshot rendered.
	var snapshot: @convention(c) (UnsafePointer<UInt8>?, Int) -> Void
	/// Authentication session id ended: callback (0), cancelled (1), failed (2) or unavailable (3), with a callback URL or error in UTF-8.
	var auth: @convention(c) (UInt32, UInt8, UnsafePointer<UInt8>?, Int) -> Void
	/// Permission request id answered with driver.Permission* status code.
	var permission: @convention(c) (UInt32, UInt8) -> Void
}

var bridge: Bridge?

/// nsgo_watch_connect takes the bridge: n function pointers in Bridge's
/// order.
@_cdecl("nsgo_watch_connect")
public func nsgoWatchConnect(_ fns: UnsafePointer<UnsafeRawPointer>, _ n: Int) {
	guard n >= 11 else { return }
	bridge = Bridge(
		size: unsafeBitCast(fns[0], to: (@convention(c) (Double, Double, Double, Double, Double, Double, Double, UInt32) -> Void).self),
		touch: unsafeBitCast(fns[1], to: (@convention(c) (UInt8, Double, Double, Double) -> Void).self),
		draw: unsafeBitCast(fns[2], to: (@convention(c) (UInt32, UnsafeMutableRawPointer, Double, Double) -> Void).self),
		scroll: unsafeBitCast(fns[3], to: (@convention(c) (UInt32, Double, Double, UInt8) -> Void).self),
		frame: unsafeBitCast(fns[4], to: (@convention(c) (Double) -> Void).self),
		text: unsafeBitCast(fns[5], to: (@convention(c) (UInt32, UInt8, UnsafePointer<UInt8>?, Int) -> Void).self),
		alert: unsafeBitCast(fns[6], to: (@convention(c) (UInt32, Int32) -> Void).self),
		lifecycle: unsafeBitCast(fns[7], to: (@convention(c) (UInt8) -> Void).self),
		snapshot: unsafeBitCast(fns[8], to: (@convention(c) (UnsafePointer<UInt8>?, Int) -> Void).self),
		auth: unsafeBitCast(fns[9], to: (@convention(c) (UInt32, UInt8, UnsafePointer<UInt8>?, Int) -> Void).self),
		permission: unsafeBitCast(fns[10], to: (@convention(c) (UInt32, UInt8) -> Void).self))
}

/// nsgo_watch_main runs the app on the calling thread, the main thread,
/// once Go connected the bridge. It does not return.
@_cdecl("nsgo_watch_main")
public func nsgoWatchMain() {
	NSGoWatchApp.main()
}

/// nsgo_watch_apply applies a batch of ops Go wrote (Store.apply), on the
/// main thread.
@_cdecl("nsgo_watch_apply")
public func nsgoWatchApply(_ data: UnsafePointer<UInt8>, _ n: Int) {
	Store.shared.apply(Reader(base: data, count: n))
}

/// nsgo_watch_snapshot renders the surface and hands Go the PNG, for
/// tests' screenshots.
@MainActor @_cdecl("nsgo_watch_snapshot")
public func nsgoWatchSnapshot() {
	let store = Store.shared
	let renderer = ImageRenderer(content: ZStack(alignment: .topLeading) {
		Color.black
		if let root = store.root { NodeView(node: root) }
	}
	.frame(width: store.size.width, height: store.size.height, alignment: .topLeading)
	.environment(\.snapshotting, true))
	renderer.scale = store.scale
	guard let image = renderer.cgImage else { return }
	let data = NSMutableData()
	guard let dest = CGImageDestinationCreateWithData(data, "public.png" as CFString, 1, nil) else { return }
	CGImageDestinationAddImage(dest, image, nil)
	guard CGImageDestinationFinalize(dest) else { return }
	bridge?.snapshot(data.bytes.assumingMemoryBound(to: UInt8.self), data.length)
}

/// nsgo_watch_tick starts the display frames (on 1) or stops them.
@_cdecl("nsgo_watch_tick")
public func nsgoWatchTick(_ on: UInt8) {
	Store.shared.ticking = on != 0
}

func sendText(_ id: UInt32, _ kind: UInt8, _ s: String) {
	var s = s
	s.withUTF8 { b in bridge?.text(id, kind, b.baseAddress, b.count) }
}

// MARK: Authentication

/// The framework presents the watch's authentication UI itself. It has no presentationContextProvider on watchOS; the SwiftUI host owns its lifetime.
final class AuthSessions {
	private final class Entry { var session: ASWebAuthenticationSession? }
	private var sessions: [UInt32: Entry] = [:]

	func open(_ id: UInt32, url: String, scheme: String, host: String, path: String, ephemeral: Bool) {
		cancel(id)
		guard let url = URL(string: url), ["http", "https"].contains(url.scheme?.lowercased() ?? "") else {
			report(id, 2, "ASWebAuthenticationSession needs an HTTP or HTTPS authorization URL")
			return
		}
		let entry = Entry()
		let completed: ASWebAuthenticationSession.CompletionHandler = { [weak self, weak entry] callback, error in
			DispatchQueue.main.async {
				guard let self, let entry, self.sessions[id] === entry else { return }
				self.sessions.removeValue(forKey: id)
				if let callback {
					self.report(id, 0, callback.absoluteString)
				} else if let error = error as? ASWebAuthenticationSessionError, error.code == .canceledLogin {
					self.report(id, 1, "")
				} else {
					self.report(id, 2, error?.localizedDescription ?? "ASWebAuthenticationSession returned no callback")
				}
			}
		}
		let session: ASWebAuthenticationSession
		if scheme.lowercased() == "https" {
			if #available(watchOS 10.4, *) {
				session = ASWebAuthenticationSession(url: url, callback: .https(host: host, path: path), completionHandler: completed)
			} else {
				report(id, 3, "HTTPS authentication callbacks need watchOS 10.4")
				return
			}
		} else {
			session = ASWebAuthenticationSession(url: url, callbackURLScheme: scheme, completionHandler: completed)
		}
		entry.session = session
		sessions[id] = entry
		session.prefersEphemeralWebBrowserSession = ephemeral
		if !session.start(), sessions[id] === entry {
			sessions.removeValue(forKey: id)
			report(id, 2, "ASWebAuthenticationSession did not start")
		}
	}

	func cancel(_ id: UInt32) {
		let entry = sessions.removeValue(forKey: id)
		entry?.session?.cancel()
	}

	private func report(_ id: UInt32, _ outcome: UInt8, _ value: String) {
		var value = value
		value.withUTF8 { bytes in bridge?.auth(id, outcome, bytes.baseAddress, bytes.count) }
	}
}

// MARK: Permissions

/// Permissions queries and requests watchOS microphone, calendar, reminder and
/// Bluetooth authorization on the main thread, verifying the app's Info.plist
/// usage descriptions before prompting so the system never terminates the app.
final class Permissions: NSObject, CBCentralManagerDelegate {
	// Driver PermissionStatus values (internal/driver/permissions.go).
	private static let notAsked: UInt8 = 0
	private static let granted: UInt8 = 1
	private static let denied: UInt8 = 2
	private static let unknown: UInt8 = 3
	private static let unavailable: UInt8 = 5

	// Host permission kinds (internal/platform/watchos/permissions.go).
	private static let kindMicrophone: UInt8 = 1
	private static let kindCalendars: UInt8 = 2
	private static let kindReminders: UInt8 = 3
	private static let kindBluetooth: UInt8 = 4

	private var eventStore: EKEventStore?
	private var centralManager: CBCentralManager?
	private var bluetoothWaiters: [UInt32] = []
	private var bluetoothTimer: Timer?
	private var bluetoothDeadline: Date?

	func handle(_ id: UInt32, kind: UInt8, ask: Bool) {
		let current = status(for: kind)
		if !ask || current != Self.notAsked {
			report(id, current)
			return
		}
		switch kind {
		case Self.kindMicrophone:
			guard checkUsage(name: "microphone", key: "NSMicrophoneUsageDescription") else {
				report(id, Self.unavailable)
				return
			}
			requestMicrophone(id)
		case Self.kindCalendars:
			requestEventKit(id, entity: .event, name: "calendars")
		case Self.kindReminders:
			requestEventKit(id, entity: .reminder, name: "reminders")
		case Self.kindBluetooth:
			guard checkUsage(name: "bluetooth", key: "NSBluetoothAlwaysUsageDescription") else {
				report(id, Self.unavailable)
				return
			}
			requestBluetooth(id)
		default:
			report(id, Self.unavailable)
		}
	}

	private func status(for kind: UInt8) -> UInt8 {
		switch kind {
		case Self.kindMicrophone:
			return microphoneStatus()
		case Self.kindCalendars:
			return eventKitStatus(.event)
		case Self.kindReminders:
			return eventKitStatus(.reminder)
		case Self.kindBluetooth:
			return bluetoothStatus()
		default:
			return Self.unavailable
		}
	}

	private func microphoneStatus() -> UInt8 {
		if #available(watchOS 10.0, *) {
			switch AVAudioApplication.shared.recordPermission {
			case .undetermined: return Self.notAsked
			case .granted: return Self.granted
			case .denied: return Self.denied
			@unknown default: return Self.unknown
			}
		} else {
			switch AVAudioSession.sharedInstance().recordPermission {
			case .undetermined: return Self.notAsked
			case .granted: return Self.granted
			case .denied: return Self.denied
			@unknown default: return Self.unknown
			}
		}
	}

	private func requestMicrophone(_ id: UInt32) {
		if #available(watchOS 10.0, *) {
			AVAudioApplication.requestRecordPermission { [weak self] _ in
				DispatchQueue.main.async {
					guard let self else { return }
					self.report(id, self.microphoneStatus())
				}
			}
		} else {
			AVAudioSession.sharedInstance().requestRecordPermission { [weak self] _ in
				DispatchQueue.main.async {
					guard let self else { return }
					self.report(id, self.microphoneStatus())
				}
			}
		}
	}

	private func eventKitStatus(_ entity: EKEntityType) -> UInt8 {
		switch EKEventStore.authorizationStatus(for: entity) {
		case .notDetermined: return Self.notAsked
		case .fullAccess: return Self.granted
		case .restricted, .denied, .writeOnly: return Self.denied
		@unknown default: return Self.unknown
		}
	}

	private func sharedEventStore() -> EKEventStore {
		if let store = eventStore { return store }
		let store = EKEventStore()
		eventStore = store
		return store
	}

	private func requestEventKit(_ id: UInt32, entity: EKEntityType, name: String) {
		if #available(watchOS 10.0, *) {
			let key = entity == .reminder ? "NSRemindersFullAccessUsageDescription" : "NSCalendarsFullAccessUsageDescription"
			guard checkUsage(name: name, key: key) else {
				report(id, Self.unavailable)
				return
			}
			let store = sharedEventStore()
			let done: EKEventStoreRequestAccessCompletionHandler = { [weak self] _, _ in
				DispatchQueue.main.async {
					guard let self else { return }
					self.report(id, self.eventKitStatus(entity))
				}
			}
			if entity == .reminder {
				store.requestFullAccessToReminders(completion: done)
			} else {
				store.requestFullAccessToEvents(completion: done)
			}
		} else {
			let key = entity == .reminder ? "NSRemindersUsageDescription" : "NSCalendarsUsageDescription"
			guard checkUsage(name: name, key: key) else {
				report(id, Self.unavailable)
				return
			}
			sharedEventStore().requestAccess(to: entity) { [weak self] _, _ in
				DispatchQueue.main.async {
					guard let self else { return }
					self.report(id, self.eventKitStatus(entity))
				}
			}
		}
	}

	private func bluetoothStatus() -> UInt8 {
		switch CBManager.authorization {
		case .notDetermined: return Self.notAsked
		case .allowedAlways: return Self.granted
		case .restricted, .denied: return Self.denied
		@unknown default: return Self.unknown
		}
	}

	private func requestBluetooth(_ id: UInt32) {
		bluetoothWaiters.append(id)
		if centralManager == nil {
			centralManager = CBCentralManager(delegate: self, queue: .main, options: [CBCentralManagerOptionShowPowerAlertKey: false])
		}
		bluetoothDeadline = Date().addingTimeInterval(600)
		if bluetoothTimer == nil {
			bluetoothTimer = Timer.scheduledTimer(withTimeInterval: 0.25, repeats: true) { [weak self] _ in
				self?.finishBluetoothIfReady()
			}
		}
		finishBluetoothIfReady()
	}

	func centralManagerDidUpdateState(_ central: CBCentralManager) {
		finishBluetoothIfReady()
	}

	private func finishBluetoothIfReady() {
		guard !bluetoothWaiters.isEmpty else { return }
		let st = bluetoothStatus()
		let expired = bluetoothDeadline.map { Date() >= $0 } ?? false
		guard st != Self.notAsked || expired else { return }
		let waiters = bluetoothWaiters
		bluetoothWaiters.removeAll()
		bluetoothTimer?.invalidate()
		bluetoothTimer = nil
		bluetoothDeadline = nil
		centralManager?.delegate = nil
		centralManager = nil
		for id in waiters {
			report(id, st)
		}
	}

	private func checkUsage(name: String, key: String) -> Bool {
		if let value = Bundle.main.object(forInfoDictionaryKey: key) {
			if let text = value as? String {
				if !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
					return true
				}
			} else {
				return true
			}
		}
		NSLog("nsgo: requesting %@ needs %@ in the app's Info.plist (nsgo.yaml's apple.info)", name, key)
		return false
	}

	private func report(_ id: UInt32, _ status: UInt8) {
		bridge?.permission(id, status)
	}
}

// MARK: The tree

/// Reader reads what Go's writer wrote: little-endian numbers and
/// length-prefixed UTF-8.
struct Reader {
	let base: UnsafePointer<UInt8>
	let count: Int
	var at = 0

	var more: Bool { at < count }

	mutating func u8() -> UInt8 {
		defer { at += 1 }
		return base[at]
	}

	mutating func u32() -> UInt32 {
		var v: UInt32 = 0
		memcpy(&v, base + at, 4)
		at += 4
		return UInt32(littleEndian: v)
	}

	mutating func f64() -> Double {
		var v: UInt64 = 0
		memcpy(&v, base + at, 8)
		at += 8
		return Double(bitPattern: UInt64(littleEndian: v))
	}

	mutating func rect() -> CGRect { CGRect(x: f64(), y: f64(), width: f64(), height: f64()) }

	mutating func string() -> String {
		let n = Int(u32())
		defer { at += n }
		return String(decoding: UnsafeBufferPointer(start: base + at, count: n), as: UTF8.self)
	}
}

/// The kinds of node Go makes, as driver.ViewKind numbers them.
enum Kind: UInt8 {
	case view = 1, text, input, scroll, image, modal, canvas
}

/// StickyCell is one sticky view's effective main-axis travel in its scroll
/// view (opStickyCells), matching driver.StickyCell.StickyShift.
struct StickyCell: Equatable {
	let cell: UInt32
	let start: Double
	let end: Double

	func shift(at offset: Double) -> Double {
		let travel = end - start
		if travel.isNaN { return 0 }
		return min(max(offset - start, min(travel, 0)), max(travel, 0))
	}
}

/// Node is one view the engine mounted.
@Observable final class Node: Identifiable {
	let id: UInt32
	let kind: Kind
	var children: [Node] = []
	@ObservationIgnored weak var parent: Node?

	var frame = CGRect.zero
	var opacity = 1.0
	var hidden = false
	/// clips says the children show only inside the box, rounded by radii
	/// (top left, top right, bottom right, bottom left).
	var clips = false
	var radii = RectangleCornerRadii()
	/// inset is how far the clip stands inside the box: the border's
	/// widths, as CSS clips overflow to the padding box.
	var inset = EdgeInsets()
	/// transform moves the node from its own top left corner.
	var transform = CGAffineTransform.identity
	/// draws says Go draws the node's look into ink, a rectangle in the
	/// node's own coordinates; version changes each time it draws again.
	var draws = false
	/// culled says the node is too far from what shows to be made.
	var culled = false
	var ink = CGRect.zero
	var version = 0

	// A scroll view's.
	var horizontal = false
	var indicator = true
	var content = CGSize.zero
	var locked = false
	var position = ScrollPosition()
	/// offset is where the scroll view last reported it was, from where its content rests.
	@ObservationIgnored var offset = CGPoint.zero
	/// stickyCells are the sticky views pinned within this scroll view.
	@ObservationIgnored var stickyCells: [StickyCell] = []

	// Sticky placement on a pinned cell (opStickyCells).
	var sticky = false
	var stickyShift = CGPoint.zero
	@ObservationIgnored weak var stickyOwner: Node?

	// A text field's.
	var text = ""
	var placeholder = ""
	var secure = false
	var disabled = false
	var focused = false

	// Accessibility (opAccess).
	var a11yFlags: UInt8 = 0
	var a11yTraits: UInt32 = 0
	var a11yLabel = ""
	var a11yHint = ""
	var a11yValue = ""

	var a11yElement: Bool { a11yFlags & 1 != 0 }
	var a11yHidden: Bool { a11yFlags & 2 != 0 }
	var a11yDisabled: Bool { a11yFlags & 4 != 0 }
	var a11yModal: Bool { a11yFlags & 8 != 0 }

	var swiftTraits: AccessibilityTraits {
		var t: AccessibilityTraits = []
		if a11yTraits & (1 << 0) != 0 { _ = t.insert(.isButton) }
		if a11yTraits & (1 << 1) != 0 { _ = t.insert(.isLink) }
		if a11yTraits & (1 << 2) != 0 { _ = t.insert(.isHeader) }
		if a11yTraits & (1 << 3) != 0 { _ = t.insert(.isImage) }
		if a11yTraits & (1 << 4) != 0 { _ = t.insert(.isStaticText) }
		if a11yTraits & (1 << 5) != 0 { _ = t.insert(.isSelected) }
		if a11yTraits & (1 << 6) != 0 { _ = t.insert(.isSearchField) }
		if a11yTraits & (1 << 7) != 0 { _ = t.insert(.updatesFrequently) }
		if a11yTraits & (1 << 8) != 0 { _ = t.insert(.isTabBar) }
		if a11yModal { _ = t.insert(.isModal) }
		return t
	}

	init(id: UInt32, kind: Kind) {
		self.id = id
		self.kind = kind
	}
}

/// An alert the app shows.
struct Alert: Identifiable {
	let id: UInt32
	let title: String
	let message: String
	let actions: [(String, UInt8)]
}

@Observable final class Store {
	static let shared = Store()

	@ObservationIgnored var nodes: [UInt32: Node] = [:]
	@ObservationIgnored private var authSessions = AuthSessions()
	@ObservationIgnored private var permissions = Permissions()
	var root: Node?
	var ticking = false
	var alert: Alert?
	/// The surface's size and pixels a point, for snapshots.
	@ObservationIgnored var size = CGSize.zero
	@ObservationIgnored var scale = 2.0

	/// The ops Go writes, as watchos.go's op constants number them.
	enum Op: UInt8 {
		case create = 1, insert, remove, delete, frame, look, ink, redraw, scroll, scrollTo, input, focus, blur,
			alert, root, lock, dismiss, cull, access, authSession, permission, stickyCells
	}

	func apply(_ r: Reader) {
		var r = r
		while r.more {
			guard let op = Op(rawValue: r.u8()) else { return }
			switch op {
			case .create:
				let id = r.u32()
				nodes[id] = Node(id: id, kind: Kind(rawValue: r.u8()) ?? .view)
			case .insert:
				let id = r.u32(), parent = r.u32(), before = r.u32()
				guard let n = nodes[id], let p = nodes[parent] else { continue }
				detach(n)
				// Before is the sibling n goes under; 0 puts it on top.
				if before != 0, let i = p.children.firstIndex(where: { $0.id == before }) {
					p.children.insert(n, at: i)
				} else {
					p.children.append(n)
				}
				n.parent = p
			case .remove:
				if let n = nodes[r.u32()] { detach(n) }
			case .delete:
				let id = r.u32()
				if let n = nodes.removeValue(forKey: id) {
					clearSticky(n)
					detach(n)
				}
			case .frame:
				let id = r.u32(), f = r.rect()
				if let n = nodes[id], n.frame != f { n.frame = f }
			case .look:
				let id = r.u32()
				let opacity = r.f64(), hidden = r.u8() != 0, clips = r.u8() != 0
				let radii = RectangleCornerRadii(topLeading: r.f64(), bottomLeading: r.f64(), bottomTrailing: r.f64(), topTrailing: r.f64())
				let inset = EdgeInsets(top: r.f64(), leading: r.f64(), bottom: r.f64(), trailing: r.f64())
				let t = CGAffineTransform(a: r.f64(), b: r.f64(), c: r.f64(), d: r.f64(), tx: r.f64(), ty: r.f64())
				guard let n = nodes[id] else { continue }
				if n.opacity != opacity { n.opacity = opacity }
				if n.hidden != hidden { n.hidden = hidden }
				if n.clips != clips { n.clips = clips }
				if n.radii != radii { n.radii = radii }
				if n.inset != inset { n.inset = inset }
				if n.transform != t { n.transform = t }
			case .ink:
				let id = r.u32(), draws = r.u8() != 0, ink = r.rect()
				guard let n = nodes[id] else { continue }
				if n.draws != draws { n.draws = draws }
				if n.ink != ink { n.ink = ink }
				n.version += 1
			case .redraw:
				nodes[r.u32()]?.version += 1
			case .scroll:
				let id = r.u32(), horizontal = r.u8() != 0, indicator = r.u8() != 0
				let content = CGSize(width: r.f64(), height: r.f64())
				guard let n = nodes[id] else { continue }
				let axisChanged = n.horizontal != horizontal
				if axisChanged { n.horizontal = horizontal }
				if n.indicator != indicator { n.indicator = indicator }
				if n.content != content { n.content = content }
				if axisChanged && !n.stickyCells.isEmpty { pinSticky(n) }
			case .scrollTo:
				let id = r.u32(), x = r.f64(), y = r.f64(), animated = r.u8() != 0
				guard let n = nodes[id] else { continue }
				let pt = CGPoint(x: x, y: y)
				// ScrollPosition measures a point from where the content rests, as Go does.
				if animated {
					withAnimation { n.position.scrollTo(point: pt) }
				} else {
					n.position.scrollTo(point: pt)
					n.offset = pt
					if !n.stickyCells.isEmpty { pinSticky(n) }
				}
			case .input:
				let id = r.u32(), placeholder = r.string(), secure = r.u8() != 0, disabled = r.u8() != 0
				guard let n = nodes[id] else { continue }
				n.placeholder = placeholder
				n.secure = secure
				n.disabled = disabled
				let text = r.string()
				if n.text != text { n.text = text }
			case .focus:
				nodes[r.u32()]?.focused = true
			case .blur:
				nodes[r.u32()]?.focused = false
			case .alert:
				let id = r.u32(), title = r.string(), message = r.string()
				var actions: [(String, UInt8)] = []
				for _ in 0..<r.u8() { actions.append((r.string(), r.u8())) }
				alert = Alert(id: id, title: title, message: message, actions: actions)
			case .root:
				let n = nodes[r.u32()]
				if root !== n { root = n }
			case .cull:
				let id = r.u32(), culled = r.u8() != 0
				if let n = nodes[id], n.culled != culled { n.culled = culled }
			case .dismiss:
				if alert?.id == r.u32() { alert = nil }
			case .lock:
				let id = r.u32(), locked = r.u8() != 0
				if let n = nodes[id], n.locked != locked { n.locked = locked }
			case .access:
				let id = r.u32(), flags = r.u8(), traits = r.u32()
				let label = r.string(), hint = r.string(), value = r.string()
				guard let n = nodes[id] else { continue }
				if n.a11yFlags != flags { n.a11yFlags = flags }
				if n.a11yTraits != traits { n.a11yTraits = traits }
				if n.a11yLabel != label { n.a11yLabel = label }
				if n.a11yHint != hint { n.a11yHint = hint }
				if n.a11yValue != value { n.a11yValue = value }
			case .authSession:
				let id = r.u32(), command = r.u8()
				if command == 1 {
					let url = r.string(), scheme = r.string(), host = r.string(), path = r.string(), ephemeral = r.u8() != 0
					authSessions.open(id, url: url, scheme: scheme, host: host, path: path, ephemeral: ephemeral)
				} else if command == 2 {
					authSessions.cancel(id)
				}
			case .permission:
				let id = r.u32(), kind = r.u8(), ask = r.u8() != 0
				permissions.handle(id, kind: kind, ask: ask)
			case .stickyCells:
				let id = r.u32()
				let count = Int(r.u32())
				var cells: [StickyCell] = []
				cells.reserveCapacity(count)
				for _ in 0..<count {
					cells.append(StickyCell(cell: r.u32(), start: r.f64(), end: r.f64()))
				}
				guard let n = nodes[id] else { continue }
				setSticky(n, cells: cells)
			}
		}
	}

	func setSticky(_ scroll: Node, cells: [StickyCell]) {
		let nextIDs = Set(cells.map(\.cell))
		for old in scroll.stickyCells where !nextIDs.contains(old.cell) {
			if let cell = nodes[old.cell], cell.stickyOwner === scroll {
				cell.stickyOwner = nil
				if cell.sticky { cell.sticky = false }
				if cell.stickyShift != .zero { cell.stickyShift = .zero }
			}
		}
		scroll.stickyCells = cells
		pinSticky(scroll)
	}

	func clearSticky(_ scroll: Node) {
		guard !scroll.stickyCells.isEmpty else { return }
		for old in scroll.stickyCells {
			if let cell = nodes[old.cell], cell.stickyOwner === scroll {
				cell.stickyOwner = nil
				if cell.sticky { cell.sticky = false }
				if cell.stickyShift != .zero { cell.stickyShift = .zero }
			}
		}
		scroll.stickyCells = []
	}

	func pinSticky(_ scroll: Node) {
		guard !scroll.stickyCells.isEmpty else { return }
		let axis = scroll.horizontal ? scroll.offset.x : scroll.offset.y
		for entry in scroll.stickyCells {
			guard let cell = nodes[entry.cell] else { continue }
			let d = entry.shift(at: axis)
			let shift = scroll.horizontal ? CGPoint(x: d, y: 0) : CGPoint(x: 0, y: d)
			cell.stickyOwner = scroll
			if !cell.sticky { cell.sticky = true }
			if cell.stickyShift != shift { cell.stickyShift = shift }
		}
	}

	func detach(_ n: Node) {
		if let p = n.parent {
			p.children.removeAll { $0 === n }
			n.parent = nil
		}
	}
}

// MARK: The views

extension EnvironmentValues {
	/// snapshotting is set for what nsgo_watch_snapshot renders.
	@Entry var snapshotting = false
}

/// NodeView shows a node at its frame in its parent: its look under its
/// children, which it clips when it clips. A leaf is its look alone, so
/// most nodes cost SwiftUI a Canvas and their placement.
struct NodeView: View {
	let node: Node

	var body: some View {
		content
			.modifier(NodeAccess(node: node))
			.opacity(node.hidden ? 0 : node.opacity)
			.allowsHitTesting(!node.hidden)
			.transformEffect(node.transform)
			.offset(x: node.frame.minX + node.stickyShift.x, y: node.frame.minY + node.stickyShift.y)
			.zIndex(node.sticky ? 1 : 0)
	}

	@ViewBuilder var content: some View {
		switch node.kind {
		case .scroll:
			ZStack(alignment: .topLeading) {
				look
				ScrollNode(node: node)
			}
			.frame(width: node.frame.width, height: node.frame.height, alignment: .topLeading)
		case .input:
			ZStack(alignment: .topLeading) {
				look
				FieldNode(node: node)
			}
			.frame(width: node.frame.width, height: node.frame.height, alignment: .topLeading)
		default:
			if node.children.isEmpty {
				look.frame(width: node.frame.width, height: node.frame.height, alignment: .topLeading)
			} else {
				ZStack(alignment: .topLeading) {
					look
					Children(node: node)
				}
				.frame(width: node.frame.width, height: node.frame.height, alignment: .topLeading)
			}
		}
	}

	@ViewBuilder var look: some View {
		if node.draws {
			Drawn(id: node.id, version: node.version)
				.frame(width: node.ink.width, height: node.ink.height)
				.offset(x: node.ink.minX, y: node.ink.minY)
				.allowsHitTesting(false)
		}
	}
}

/// NodeAccess applies opAccess to non-input nodes; FieldNode applies its own
/// accessibility modifiers directly to the underlying TextField/SecureField.
struct NodeAccess: ViewModifier {
	let node: Node

	@ViewBuilder func body(content: Content) -> some View {
		if node.kind == .input {
			content.accessibilityHidden(node.a11yHidden)
		} else if node.a11yElement {
			content
				.accessibilityElement(children: .ignore)
				.accessibilityLabel(Text(verbatim: node.a11yLabel))
				.accessibilityHint(Text(verbatim: node.a11yHint))
				.accessibilityValue(Text(verbatim: node.a11yValue))
				.accessibilityAddTraits(node.swiftTraits)
				.disabled(node.a11yDisabled)
				.accessibilityHidden(node.a11yHidden)
		} else {
			content
				.accessibilityAddTraits(node.swiftTraits)
				.accessibilityHidden(node.a11yHidden)
		}
	}
}

/// ClipBox is where a node clips: its box inset by the border's widths,
/// with the corners there.
struct ClipBox: Shape {
	let radii: RectangleCornerRadii
	let inset: EdgeInsets

	func path(in rect: CGRect) -> Path {
		let box = CGRect(x: rect.minX + inset.leading, y: rect.minY + inset.top,
			width: max(rect.width - inset.leading - inset.trailing, 0),
			height: max(rect.height - inset.top - inset.bottom, 0))
		return UnevenRoundedRectangle(cornerRadii: radii).path(in: box)
	}
}

/// Children are a node's children, each placed at its frame: in the
/// node's own stack, or in one clipped to its box when it clips.
struct Children: View {
	let node: Node

	var body: some View {
		if node.clips {
			ZStack(alignment: .topLeading) {
				ForEach(node.children.filter { !$0.culled }) { NodeView(node: $0) }
			}
			.frame(width: node.frame.width, height: node.frame.height, alignment: .topLeading)
			.clipShape(ClipBox(radii: node.radii, inset: node.inset))
		} else {
			ForEach(node.children.filter { !$0.culled }) { NodeView(node: $0) }
		}
	}
}

/// Drawn is the Canvas Go draws a node's look into; a new version draws it
/// again.
struct Drawn: View {
	let id: UInt32
	let version: Int

	var body: some View {
		Canvas { ctx, size in
			ctx.withCGContext { cg in
				bridge?.draw(id, Unmanaged.passUnretained(cg).toOpaque(), size.width, size.height)
			}
		}
	}
}

/// ScrollNode scrolls a scroll view's children, which Go placed in its
/// content, and reports where it is.
struct ScrollNode: View {
	@Bindable var node: Node
	@Environment(\.snapshotting) private var snapshotting

	@ViewBuilder var body: some View {
		if snapshotting {
			// ImageRenderer draws no ScrollView: the content where it is.
			ZStack(alignment: .topLeading) {
				ForEach(node.children.filter { !$0.culled }) { NodeView(node: $0) }
			}
			.offset(x: -node.offset.x, y: -node.offset.y)
			.frame(width: node.frame.width, height: node.frame.height, alignment: .topLeading)
			.clipped()
		} else {
			scroller
		}
	}

	@ViewBuilder var scroller: some View {
		let size = CGSize(width: node.horizontal ? max(node.content.width, node.frame.width) : node.frame.width,
			height: node.horizontal ? node.frame.height : max(node.content.height, node.frame.height))
		ScrollView(node.horizontal ? .horizontal : .vertical, showsIndicators: node.indicator) {
			ZStack(alignment: .topLeading) {
				ForEach(node.children.filter { !$0.culled }) { NodeView(node: $0) }
			}
			.frame(width: size.width, height: size.height, alignment: .topLeading)
		}
		.scrollPosition($node.position)
		.scrollDisabled(node.locked)
		.contentMargins(0, for: .scrollContent)
		// A scroll view that starts within the top of the screen that is not safe extends up to the screen's edge and insets its content by as much, which leaves the content where Go put it: report where it is from there, as ScrollPosition measures it, so it rests at zero (#814).
		.onScrollGeometryChange(for: CGPoint.self) { CGPoint(x: $0.contentOffset.x + $0.contentInsets.leading, y: $0.contentOffset.y + $0.contentInsets.top) } action: { _, at in
			node.offset = at
			Store.shared.pinSticky(node)
			bridge?.scroll(node.id, at.x, at.y, 0)
		}
		.onScrollPhaseChange { _, phase in
			let code: UInt8 = switch phase {
			case .interacting, .tracking: 1
			case .decelerating: 2
			case .animating: 4
			default: 3
			}
			bridge?.scroll(node.id, .nan, .nan, code)
		}
		.frame(width: node.frame.width, height: node.frame.height)
		.clipShape(ClipBox(radii: node.radii, inset: node.inset))
	}
}

/// FieldNode is a text field: Go draws its text, and a TextField all but
/// transparent over it takes the taps that open the system's text input.
struct FieldNode: View {
	@Bindable var node: Node
	@FocusState private var focus: Bool

	var body: some View {
		ZStack(alignment: .topLeading) {
			Group {
				if node.secure {
					SecureField(node.placeholder, text: $node.text)
				} else {
					TextField(node.placeholder, text: $node.text)
				}
			}
			.focused($focus)
			.disabled(node.disabled || node.a11yDisabled)
			.accessibilityLabel(Text(verbatim: node.a11yLabel.isEmpty ? node.placeholder : node.a11yLabel))
			.accessibilityHint(Text(verbatim: node.a11yHint))
			.accessibilityValue(Text(verbatim: fieldValue))
			.accessibilityAddTraits(node.swiftTraits)
			.accessibilityHidden(node.a11yHidden)
			.opacity(0.02)
			.frame(width: node.frame.width, height: node.frame.height)
			.onChange(of: node.text) { _, text in sendText(node.id, 0, text) }
			.onSubmit { sendText(node.id, 1, node.text) }
			.onChange(of: focus) { _, on in
				node.focused = on
				sendText(node.id, on ? 2 : 3, node.text)
			}
			.onChange(of: node.focused, initial: true) { _, on in
				if focus != on { focus = on }
			}
			Children(node: node)
		}
	}

	private var fieldValue: String {
		if node.secure { return node.a11yValue }
		if node.a11yValue.isEmpty { return node.text }
		if node.text.isEmpty || node.a11yValue == node.text { return node.a11yValue }
		return "\(node.text), \(node.a11yValue)"
	}
}

/// Ticker reports a display frame while Go asks for them.
struct Ticker: View {
	var body: some View {
		TimelineView(.animation) { ctx in
			Color.clear.onChange(of: ctx.date) { _, date in
				bridge?.frame(date.timeIntervalSinceReferenceDate)
			}
		}
	}
}

/// Root is the surface: the root node over the whole screen, the touches
/// on it and what it reports of its size and traits.
struct Root: View {
	@State private var store = Store.shared
	@Environment(\.displayScale) private var scale
	@Environment(\.colorScheme) private var scheme
	@Environment(\.accessibilityReduceMotion) private var reduceMotion
	@Environment(\.colorSchemeContrast) private var contrast
	@Environment(\.accessibilityVoiceOverEnabled) private var voiceOver
	@Environment(\.scenePhase) private var phase
	@Environment(\.dynamicTypeSize) private var typeSize
	@State private var touching = false
	@State private var lastTouch = CGPoint.zero
	@GestureState private var gestureActive = false

	var body: some View {
		GeometryReader { geo in
			ZStack(alignment: .topLeading) {
				if store.ticking { Ticker().frame(width: 0, height: 0) }
				if let root = store.root { NodeView(node: root) }
			}
			.frame(width: geo.size.width, height: geo.size.height, alignment: .topLeading)
			.contentShape(Rectangle())
			.simultaneousGesture(
				DragGesture(minimumDistance: 0, coordinateSpace: .named("nsgo"))
					.updating($gestureActive) { _, state, _ in state = true }
					.onChanged { g in
						lastTouch = g.location
						bridge?.touch(touching ? 1 : 0, g.location.x, g.location.y, g.time.timeIntervalSinceReferenceDate)
						touching = true
					}
					.onEnded { g in
						touching = false
						lastTouch = g.location
						bridge?.touch(2, g.location.x, g.location.y, g.time.timeIntervalSinceReferenceDate)
					})
			.onChange(of: gestureActive) { _, active in
				if !active && touching {
					touching = false
					bridge?.touch(3, lastTouch.x, lastTouch.y, Date.timeIntervalSinceReferenceDate)
				}
			}
			.onAppear { report(geo) }
			.onChange(of: geo.size) { report(geo) }
			.onChange(of: scheme) { report(geo) }
			.onChange(of: reduceMotion) { report(geo) }
			.onChange(of: contrast) { report(geo) }
			.onChange(of: voiceOver) { report(geo) }
			.onChange(of: typeSize) { report(geo) }
		}
		.coordinateSpace(.named("nsgo"))
		.ignoresSafeArea()
		.alert(store.alert?.title ?? "", isPresented: Binding(get: { store.alert != nil }, set: { if !$0 { store.alert = nil } }),
			presenting: store.alert) { a in
			ForEach(Array(a.actions.enumerated()), id: \.offset) { i, action in
				Button(action.0, role: action.1 == 2 ? .destructive : action.1 == 1 ? .cancel : nil) {
					bridge?.alert(a.id, Int32(i))
				}
			}
		} message: { a in
			if !a.message.isEmpty { Text(a.message) }
		}
		.onChange(of: phase, initial: true) { _, p in
			bridge?.lifecycle(p == .active ? 0 : p == .inactive ? 1 : 2)
		}
	}

	/// report sends the surface's size, scale, safe area and traits: dark
	/// (1), reduce motion (2) and VoiceOver (4), with the text size's
	/// step from the default in the bits above 8.
	func report(_ geo: GeometryProxy) {
		store.size = geo.size
		store.scale = scale
		let insets = geo.safeAreaInsets
		var flags: UInt32 = 0
		if scheme == .dark { flags |= 1 }
		if reduceMotion { flags |= 2 }
		if contrast == .increased { flags |= 8 }
		if voiceOver { flags |= 4 }
		let steps: [DynamicTypeSize] = [.xSmall, .small, .medium, .large, .xLarge, .xxLarge, .xxxLarge,
			.accessibility1, .accessibility2, .accessibility3, .accessibility4, .accessibility5]
		let step = (steps.firstIndex(of: typeSize) ?? 3) - 3
		flags |= UInt32(bitPattern: Int32(step + 128)) << 8
		bridge?.size(geo.size.width, geo.size.height, scale, insets.top, insets.leading, insets.bottom, insets.trailing, flags)
	}
}

struct NSGoWatchApp: App {
	var body: some Scene {
		WindowGroup { Root() }
	}
}
