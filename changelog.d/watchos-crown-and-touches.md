### Added

- Route watchOS Digital Crown rotation through SwiftUI `.digitalCrownRotation` to views that pan (`OnWheel`/`OnPan`) or turn a value (`increment`/`decrement` actions) while keeping native `ScrollView` crown scrolling, and post simulator IOHID touches to SwiftUI's hosting window in the watchOS test host so scrolls and text fields receive touches as from a finger (#1492).
