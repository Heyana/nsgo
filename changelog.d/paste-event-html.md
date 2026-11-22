### Added

- Populate `PasteEvent.HTML` (`onPaste`'s `html`) from the clipboard on Windows (`CF_HTML`), Linux (`text/html` via GTK 4 / GDK) and Android (`ClipData.Item.getHtmlText`), matching macOS, iOS and the web (#1559 by @houko).
