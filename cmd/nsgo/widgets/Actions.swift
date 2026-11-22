// Shared widget and Live Activity intents. The app library runs Go callbacks; the extension archives buttons with the same intent and parameters.

import AppIntents
import Foundation

/// The same intent carries widget buttons and Live Activity actions. An empty activity identifies a normal widget press.
#if os(iOS)
@available(iOS 17.0, *)
typealias NSGoActionProtocol = LiveActivityIntent
#else
@available(macOS 14.0, *)
typealias NSGoActionProtocol = AppIntent
#endif

@available(iOS 17.0, macOS 14.0, *)
struct NSGoActionIntent: NSGoActionProtocol {
    static var title: LocalizedStringResource = "Action"
    static var isDiscoverable: Bool = false

    @Parameter(title: "Activity") var activity: String
    @Parameter(title: "Action") var action: String

    init() {}

    init(activity: String, action: String) {
        self.activity = activity
        self.action = action
    }

    func perform() async throws -> some IntentResult {
        #if NSGO_APP
        await Presses.press(activity: activity, action: action)
        #endif
        return .result()
    }
}

#if os(macOS) && NSGO_APP
// This conformance routes the intent to the app process without opening a window. It cannot be compiled into an application extension.
@available(macOS 14.0, *)
extension NSGoActionIntent: ForegroundContinuableIntent {}
#endif

#if NSGO_APP
/// The events the callback reports, as package widget's.
let eventPress: Int32 = 1, eventToken: Int32 = 2, eventStartToken: Int32 = 3

public typealias Callback = @convention(c) (Int32, UnsafePointer<CChar>, UnsafePointer<CChar>) -> Void

/// Presses keeps the callback and what came before it: presses wait for it, as the system starts the app for one before its main sets it, and tokens are kept.
enum Presses {
    static let lock = NSLock()
    static var callback: Callback?
    static var waiting: [CheckedContinuation<Void, Never>] = []
    static var tokens: [(Int32, String, String)] = []
    /// tasks are the updates and ends under way, which a press waits for, as the system may suspend the app once it returns.
    static var tasks: [Task<Void, Never>] = []

    static func report(_ event: Int32, _ id: String, _ value: String) {
        let cb: Callback? = lock.withLock {
            if callback == nil { tokens.append((event, id, value)) }
            return callback
        }
        guard let cb else { return }
        id.withCString { i in value.withCString { v in cb(event, i, v) } }
    }

    static func press(activity: String, action: String) async {
        if lock.withLock({ callback == nil }) {
            await withCheckedContinuation { (c: CheckedContinuation<Void, Never>) in
                let ready = lock.withLock {
                    if callback == nil { waiting.append(c) }
                    return callback != nil
                }
                if ready { c.resume() }
            }
        }
        // The handler runs Go, which may take a while: not on a thread of the cooperative pool.
        await withCheckedContinuation { (c: CheckedContinuation<Void, Never>) in
            Thread.detachNewThread {
                report(eventPress, activity, action)
                c.resume()
            }
        }
        let pending = lock.withLock {
            defer { tasks.removeAll() }
            return tasks
        }
        for t in pending { await t.value }
    }

    static func run(_ work: @escaping @Sendable () async -> Void) {
        let t = Task { await work() }
        lock.withLock { tasks.append(t) }
    }
}

@_cdecl("nsgo_activity_listen")
public func nsgoActivityListen(_ cb: Callback) {
    let (waiting, tokens) = Presses.lock.withLock {
        Presses.callback = cb
        defer {
            Presses.waiting.removeAll()
            Presses.tokens.removeAll()
        }
        return (Presses.waiting, Presses.tokens)
    }
    for (event, id, value) in tokens { Presses.report(event, id, value) }
    for c in waiting { c.resume() }
    #if os(iOS)
    if Bundle.main.object(forInfoDictionaryKey: "NSSupportsLiveActivities") as? Bool == true {
        listenActivityTokens()
    }
    #endif
}
#endif
