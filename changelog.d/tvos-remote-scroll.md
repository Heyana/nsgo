### Added

- Scroll overflowing tvOS `Scroll` and `List` views that contain no focusable views with the Apple TV remote's touch surface by enabling `UITouchTypeIndirect` on `UIScrollView.panGestureRecognizer` and focusability while preserving focus-driven scrolling for focusable children (#1483 by @houko).
