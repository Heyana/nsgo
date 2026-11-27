### Fixed

- Resize and reposition Linux X11 fitted panels atomically with `XMoveResizeWindow` when their content grows or shrinks, and apply fitted sizes before pre-map placement so unmanaged panels keep their anchor (#1394).
- Commit Windows DirectComposition visual trees for transparent and material surfaces even when neither WebView nor GPU views are enabled, and resize anchored fitted panels beside their anchor in one `SetWindowPos` call (#1394).
