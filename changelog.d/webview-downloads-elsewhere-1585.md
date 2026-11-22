### Fixed

- Harden `WebView.OnDownload` on Windows (WebView2 `DownloadStarting` failure reporting, closed-view cancellation, stat byte-count fallback and COM lifecycle tests) and Android (`DownloadListener` redirect-safe header/cookie isolation, view-close cancellation, out-of-memory error handling and JVM/protocol tests) (#1585).
