// The Live Activities of NSGoWidgetKit, the library among the app's
// frameworks: package widget starts, updates and ends them through these
// functions with purego, as ActivityKit has no Objective-C interface, and
// hears presses and push tokens through the callback it gives
// nsgo_activity_listen. Errors return as strings the caller frees with
// nsgo_free: "ended" for an activity that no longer runs.

#if os(iOS)
import ActivityKit
import Foundation

private func failure(_ s: String) -> UnsafeMutablePointer<CChar>? { strdup(s) }

@_cdecl("nsgo_free")
public func nsgoFree(_ p: UnsafeMutablePointer<CChar>?) { free(p) }

@available(iOS 16.1, *)
private func state(_ json: UnsafePointer<CChar>) -> NSGoActivityAttributes.ContentState? {
    let data = Data(bytes: json, count: strlen(json))
    return try? JSONDecoder().decode(NSGoActivityAttributes.ContentState.self, from: data)
}

@available(iOS 16.1, *)
private func find(_ id: String) -> Activity<NSGoActivityAttributes>? {
    Activity<NSGoActivityAttributes>.activities.first {
        $0.attributes.id == id && $0.activityState != .ended && $0.activityState != .dismissed
    }
}

/// watch reports the push tokens of a, as APNs gives and renews them.
@available(iOS 16.1, *)
private func watch(_ a: Activity<NSGoActivityAttributes>) {
    let id = a.attributes.id
    Task {
        for await token in a.pushTokenUpdates {
            Presses.report(eventToken, id, token.map { String(format: "%02x", $0) }.joined())
        }
    }
}

@_cdecl("nsgo_activity_enabled")
public func nsgoActivityEnabled() -> Int32 {
    guard #available(iOS 16.1, *) else { return -1 }
    return ActivityAuthorizationInfo().areActivitiesEnabled ? 1 : 0
}

@_cdecl("nsgo_activity_start")
public func nsgoActivityStart(_ kind: UnsafePointer<CChar>, _ id: UnsafePointer<CChar>, _ json: UnsafePointer<CChar>, _ push: Int32) -> UnsafeMutablePointer<CChar>? {
    guard #available(iOS 16.1, *) else { return failure("Live Activities need iOS 16.1") }
    guard let st = state(json) else { return failure("the activity's content is not JSON") }
    let attributes = NSGoActivityAttributes(kind: String(cString: kind), id: String(cString: id))
    do {
        let a: Activity<NSGoActivityAttributes>
        if #available(iOS 16.2, *) {
            a = try Activity.request(attributes: attributes, content: ActivityContent(state: st, staleDate: st.stale), pushType: push != 0 ? .token : nil)
        } else {
            a = try Activity.request(attributes: attributes, contentState: st, pushType: push != 0 ? .token : nil)
        }
        if push != 0 { watch(a) }
        return nil
    } catch {
        return failure("ActivityKit refused the activity: \(error.localizedDescription)")
    }
}

private struct Alert: Decodable {
    let title, body: String
}

@_cdecl("nsgo_activity_update")
public func nsgoActivityUpdate(_ id: UnsafePointer<CChar>, _ json: UnsafePointer<CChar>, _ alert: UnsafePointer<CChar>) -> UnsafeMutablePointer<CChar>? {
    guard #available(iOS 16.1, *) else { return failure("Live Activities need iOS 16.1") }
    guard let a = find(String(cString: id)) else { return failure("ended") }
    guard let st = state(json) else { return failure("the activity's content is not JSON") }
    let al = strlen(alert) > 0 ? try? JSONDecoder().decode(Alert.self, from: Data(bytes: alert, count: strlen(alert))) : nil
    Presses.run {
        let config = al.map { AlertConfiguration(title: LocalizedStringResource(stringLiteral: $0.title), body: LocalizedStringResource(stringLiteral: $0.body), sound: .default) }
        if #available(iOS 16.2, *) {
            await a.update(ActivityContent(state: st, staleDate: st.stale), alertConfiguration: config)
        } else {
            await a.update(using: st, alertConfiguration: config)
        }
    }
    return nil
}

@_cdecl("nsgo_activity_end")
public func nsgoActivityEnd(_ id: UnsafePointer<CChar>, _ json: UnsafePointer<CChar>, _ hasFinal: Int32, _ dismissal: Int32, _ at: Double) -> UnsafeMutablePointer<CChar>? {
    guard #available(iOS 16.1, *) else { return failure("Live Activities need iOS 16.1") }
    guard let a = find(String(cString: id)) else { return failure("ended") }
    let st = hasFinal != 0 ? state(json) : nil
    let policy: ActivityUIDismissalPolicy
    switch dismissal {
    case 1: policy = .immediate
    case 2: policy = .after(Date(timeIntervalSince1970: at))
    default: policy = .default
    }
    Presses.run {
        if #available(iOS 16.2, *) {
            await a.end(st.map { ActivityContent(state: $0, staleDate: nil) }, dismissalPolicy: policy)
        } else {
            await a.end(using: st, dismissalPolicy: policy)
        }
    }
    return nil
}

private struct Listed: Encodable {
    let id, kind: String
}

@_cdecl("nsgo_activity_list")
public func nsgoActivityList() -> UnsafeMutablePointer<CChar>? {
    guard #available(iOS 16.1, *) else { return strdup("[]") }
    let running = Activity<NSGoActivityAttributes>.activities
        .filter { $0.activityState != .ended && $0.activityState != .dismissed }
        .map { Listed(id: $0.attributes.id, kind: $0.attributes.kind) }
    let data = (try? JSONEncoder().encode(running)) ?? Data("[]".utf8)
    return strdup(String(decoding: data, as: UTF8.self))
}

func listenActivityTokens() {
    guard #available(iOS 16.1, *) else { return }
    // The tokens of activities of earlier runs and of those pushes start,
    // and the token to start them with.
    for a in Activity<NSGoActivityAttributes>.activities where a.pushToken != nil || a.activityState == .active {
        watch(a)
    }
    if #available(iOS 17.2, *) {
        Task {
            for await a in Activity<NSGoActivityAttributes>.activityUpdates { watch(a) }
        }
        Task {
            for await token in Activity<NSGoActivityAttributes>.pushToStartTokenUpdates {
                Presses.report(eventStartToken, "", token.map { String(format: "%02x", $0) }.joined())
            }
        }
    }
}
#endif
