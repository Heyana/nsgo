### Fixed

- Fail Linux `SetWebCookies` before writing any cookie when libsoup would reject a public-suffix domain, prepare batches before writing on Apple and Windows, and drop expired cookies in `nsgotest` (#1363).
