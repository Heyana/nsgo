### Fixed

- Avoid GStreamer 1.28 `decodebin3` aborts in Linux audio playback by keeping `GstPlay` on classic `playbin` unless `GST_PLAY_USE_PLAYBIN3` is set (#2489 by @houko).
