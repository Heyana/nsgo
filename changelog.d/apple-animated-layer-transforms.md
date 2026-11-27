### Changed

- Cache animated layer bounds, anchor point, backing layer, and `setTransform:` dispatch on macOS and iOS, and handle same-opacity animated background colors and unchanged clip holders on the fast path (#1327 by @houko).
