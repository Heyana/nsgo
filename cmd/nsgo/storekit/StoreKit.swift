// The library that nsgo build puts among the frameworks of apps whose nsgo.yaml sets apple.purchases: internal/platform/apple calls it through purego to reach StoreKit 2, whose products and transactions have no Objective-C interface. It only carries requests and answers across, as JSON; the services' logic stays in Go.

import Foundation
import StoreKit
#if canImport(UIKit)
import UIKit
#endif
#if os(macOS)
import AppKit
#endif

/// Answer hears each answer: the call's ID, 0 for a transaction Transaction.updates delivers, and the answer's JSON.
public typealias NsgoStoreKitAnswer = @convention(c) (UInt32, UnsafePointer<UInt8>?, Int) -> Void

private let lock = NSLock()
private var answer: NsgoStoreKitAnswer?
private var updates: Task<Void, Never>?
/// products are those the app was given, kept to buy by ID.
private var products: [String: Product] = [:]
/// finishing reserves an unfinished transaction while its asynchronous finish runs.
private var finishing: Set<UInt64> = []

/// locked runs fn holding lock, as NSLocking's withLock does from iOS 16 and macOS 13.
private func locked<T>(_ fn: () throws -> T) rethrows -> T {
    lock.lock()
    defer { lock.unlock() }
    return try fn()
}

private struct Request: Decodable {
    var command: String
    var ids: [String]?
    var product: String?
    var accountToken: String?
    var quantity: Int?
    var offer: Offer?
    var transaction: String?
}

private struct Offer: Decodable {
    var id: String
    var keyID: String
    var nonce: String
    var signature: Data
    var timestamp: Int
}

private struct Answer: Encodable {
    var error: String?
    var message: String?
    var products: [ProductOut]?
    var transaction: TransactionOut?
    var transactions: [TransactionOut]?
    var refundStatus: String?
}

private struct ProductOut: Encodable {
    var id, type, title, description, displayPrice, price: String
    var currencyCode: String?
    var subscription: SubscriptionOut?
}

private struct SubscriptionOut: Encodable {
    var groupID: String
    var period: PeriodOut
    var introductoryOffer: OfferOut?
    var offers: [OfferOut]
}

private struct PeriodOut: Encodable {
    var unit: String
    var value: Int
}

private struct OfferOut: Encodable {
    var id: String?
    var displayPrice: String
    var period: PeriodOut
    var periodCount: Int
    var paymentMode: String
}

private struct TransactionOut: Encodable {
    var id, originalID, productID, productType: String
    /// The dates are milliseconds since the Unix epoch.
    var purchaseDate: Int64
    var expirationDate, revocationDate: Int64?
    var quantity: Int
    var accountToken: String?
    var familyShared, upgraded: Bool
    var environment: String
    var signed: String
    var verified: Bool
}

private struct Failure: Error {
    var code: String
    var message: String?
}

/// nsgo_storekit_start keeps fn, which hears every answer, and listens to Transaction.updates, as the app must from its launch to hear transactions made outside it.
@_cdecl("nsgo_storekit_start")
public func nsgoStoreKitStart(_ fn: NsgoStoreKitAnswer) {
    lock.lock()
    answer = fn
    let listening = updates != nil
    if !listening {
        updates = Task.detached {
            for await result in Transaction.updates {
                send(0, Answer(transaction: transactionOut(result)))
            }
        }
    }
    lock.unlock()
}

/// nsgo_storekit_call carries out request, a JSON request, and answers call id once.
@_cdecl("nsgo_storekit_call")
public func nsgoStoreKitCall(_ id: UInt32, _ request: UnsafePointer<CChar>) {
    let data = Data(String(cString: request).utf8)
    Task {
        var out = Answer()
        do {
            out = try await handle(try JSONDecoder().decode(Request.self, from: data))
        } catch let f as Failure {
            out.error = f.code
            out.message = f.message
        } catch StoreKitError.userCancelled {
            out.error = "cancelled"
        } catch {
            out.error = "failed"
            out.message = String(describing: error)
        }
        send(id, out)
    }
}

private func handle(_ r: Request) async throws -> Answer {
    switch r.command {
    case "products":
        let found = try await Product.products(for: r.ids ?? [])
        locked {
            for p in found {
                products[p.id] = p
            }
        }
        return Answer(products: found.map(productOut))
    case "buy":
        let id = r.product ?? ""
        var product = locked { products[id] }
        if product == nil {
            product = try await Product.products(for: [id]).first
        }
        guard let product else {
            throw Failure(code: "failed", message: "the store sells no \(id)")
        }
        var options: Set<Product.PurchaseOption> = []
        if let token = r.accountToken, let uuid = UUID(uuidString: token) {
            options.insert(.appAccountToken(uuid))
        }
        if let q = r.quantity, q > 1 {
            options.insert(.quantity(q))
        }
        if let o = r.offer, let nonce = UUID(uuidString: o.nonce) {
            options.insert(.promotionalOffer(offerID: o.id, keyID: o.keyID, nonce: nonce, signature: o.signature, timestamp: o.timestamp))
        }
        switch try await product.purchase(options: options) {
        case .success(let result):
            return Answer(transaction: transactionOut(result))
        case .pending:
            throw Failure(code: "pending")
        case .userCancelled:
            throw Failure(code: "cancelled")
        @unknown default:
            throw Failure(code: "failed")
        }
    case "finish":
        guard let id = UInt64(r.transaction ?? "") else {
            throw Failure(code: "notFound")
        }
        guard locked({ finishing.insert(id).inserted }) else {
            throw Failure(code: "notFound")
        }
        defer { _ = locked { finishing.remove(id) } }
        // Entitlements include finished purchases. Only StoreKit's unfinished sequence can tell whether this ID still needs delivery acknowledged.
        for await result in Transaction.unfinished where result.unsafePayloadValue.id == id {
            await result.unsafePayloadValue.finish()
            return Answer()
        }
        throw Failure(code: "notFound")
    case "entitlements":
        var out: [TransactionOut] = []
        for await result in Transaction.currentEntitlements {
            out.append(transactionOut(result))
        }
        return Answer(transactions: out)
    case "sync":
        try await AppStore.sync()
        return Answer()
    case "manageSubscriptions":
#if os(iOS)
        guard let scene = UIApplication.shared.connectedScenes.compactMap({ $0 as? UIWindowScene }).first else {
            throw Failure(code: "failed", message: "no active window scene")
        }
        try await AppStore.showManageSubscriptions(in: scene)
#elseif os(macOS)
        guard let url = URL(string: "https://apps.apple.com/account/subscriptions"), NSWorkspace.shared.open(url) else {
            throw Failure(code: "failed", message: "could not open subscriptions")
        }
#else
        guard let url = URL(string: "https://apps.apple.com/account/subscriptions") else {
            throw Failure(code: "failed", message: "could not open subscriptions")
        }
        await UIApplication.shared.open(url)
#endif
        return Answer()
    case "refund":
        guard let id = UInt64(r.transaction ?? "") else { throw Failure(code: "failed", message: "invalid transaction") }
#if os(iOS)
        guard let scene = UIApplication.shared.connectedScenes.compactMap({ $0 as? UIWindowScene }).first else {
            throw Failure(code: "failed", message: "no active window scene")
        }
        let status = try await Transaction.beginRefundRequest(for: id, in: scene)
#elseif os(macOS)
        guard let controller = NSApp.keyWindow?.contentViewController else {
            throw Failure(code: "failed", message: "no active window controller")
        }
        let status = try await Transaction.beginRefundRequest(for: id, in: controller)
#else
        throw Failure(code: "failed", message: "refund requests are unavailable on tvOS")
#endif
#if os(iOS) || os(macOS)
        switch status {
        case .success: return Answer(refundStatus: "success")
        case .userCancelled: return Answer(refundStatus: "cancelled")
        @unknown default: throw Failure(code: "failed")
        }
#endif
    default:
        throw Failure(code: "failed", message: "no command \(r.command)")
    }
}

/// send answers call id with a.
private func send(_ id: UInt32, _ a: Answer) {
    guard let fn = locked({ answer }), let data = try? JSONEncoder().encode(a) else {
        return
    }
    data.withUnsafeBytes { fn(id, $0.bindMemory(to: UInt8.self).baseAddress, $0.count) }
}

/// transactionOut writes a transaction with its JWS without changing its finish state.
private func transactionOut(_ result: VerificationResult<Transaction>) -> TransactionOut {
    let t = result.unsafePayloadValue
    var verified = false
    if case .verified = result {
        verified = true
    }
    var env = t.environmentStringRepresentation
    if #available(iOS 16.0, macOS 13.0, tvOS 16.0, *) {
        env = t.environment.rawValue
    }
    return TransactionOut(
        id: String(t.id), originalID: String(t.originalID), productID: t.productID, productType: typeName(t.productType),
        purchaseDate: millis(t.purchaseDate), expirationDate: t.expirationDate.map(millis), revocationDate: t.revocationDate.map(millis),
        quantity: t.purchasedQuantity, accountToken: t.appAccountToken?.uuidString.lowercased(),
        familyShared: t.ownershipType == .familyShared, upgraded: t.isUpgraded, environment: env,
        signed: result.jwsRepresentation, verified: verified)
}

private func millis(_ d: Date) -> Int64 { Int64((d.timeIntervalSince1970 * 1000).rounded()) }

private func typeName(_ t: Product.ProductType) -> String {
    switch t {
    case .consumable: return "consumable"
    case .nonConsumable: return "nonConsumable"
    case .autoRenewable: return "autoRenewable"
    case .nonRenewable: return "nonRenewable"
    default: return t.rawValue
    }
}

private func productOut(_ p: Product) -> ProductOut {
    var out = ProductOut(id: p.id, type: typeName(p.type), title: p.displayName, description: p.description, displayPrice: p.displayPrice,
                         price: NSDecimalNumber(decimal: p.price).stringValue, currencyCode: p.priceFormatStyle.currencyCode)
    if let s = p.subscription {
        out.subscription = SubscriptionOut(groupID: s.subscriptionGroupID, period: periodOut(s.subscriptionPeriod),
                                           introductoryOffer: s.introductoryOffer.map(offerOut), offers: s.promotionalOffers.map(offerOut))
    }
    return out
}

private func periodOut(_ p: Product.SubscriptionPeriod) -> PeriodOut {
    switch p.unit {
    case .day: return PeriodOut(unit: "day", value: p.value)
    case .week: return PeriodOut(unit: "week", value: p.value)
    case .month: return PeriodOut(unit: "month", value: p.value)
    case .year: return PeriodOut(unit: "year", value: p.value)
    @unknown default: return PeriodOut(unit: "day", value: p.value)
    }
}

private func offerOut(_ o: Product.SubscriptionOffer) -> OfferOut {
    var mode = "payAsYouGo"
    switch o.paymentMode {
    case .freeTrial: mode = "freeTrial"
    case .payUpFront: mode = "payUpFront"
    default: break
    }
    return OfferOut(id: o.id, displayPrice: o.displayPrice, period: periodOut(o.period), periodCount: o.periodCount, paymentMode: mode)
}
