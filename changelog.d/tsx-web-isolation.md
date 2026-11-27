### Added

- Isolate sandboxed `tsx.View` and `tsx.Load` packages on the web in an opaque-origin `<iframe sandbox="allow-scripts">` with strict CSP over a private `MessagePort` bridge (#1522).
