### Fixed

- Play Linux `core.AudioPlayer` streams through classic GStreamer `playbin` (`decodebin2`) instead of `GtkMediaFile` (`playbin3`/`decodebin3`), avoiding the GStreamer 1.28 `decodebin3` process abort (#2489).
