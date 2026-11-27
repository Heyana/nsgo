### Added

- Expose `Purchases.manageSubscriptions()` and `Purchases.requestRefund(transactionId)` in TSX and add `nsgotest.App.RefundError` to simulate refund failures in tests (#1368 by @houko).

### Fixed

- Retain the StoreKit presentation window or scene synchronously in `nsgo_storekit_call` before entering the async `Task`, and open macOS subscriptions URLs on the main actor (#1368 by @houko).
