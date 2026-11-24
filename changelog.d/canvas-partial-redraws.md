### Changed

- Redraw only what a canvas's animated values change on Windows and the web, skip Windows canvas paints that cannot change a pixel, and leave the decoration of views outside a Windows partial frame's damage unpainted (#1331 by @oneo-me).
