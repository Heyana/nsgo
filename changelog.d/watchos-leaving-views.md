### Added

- Animate leaving views on watchOS during `LayoutAnimation.Delete` commits by retaining deleted subtrees as non-interactive leaving ghosts on the SwiftUI host with separate host node IDs so immediate `NodeID` reuse never collides (#1493).
