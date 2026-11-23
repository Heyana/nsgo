### Fixed

- `audio.OutputLatency` on Android answers at once with an unknown latency instead of never calling back, and its query no longer makes the runtime drop the batch with an unknown audio command (#2182 by @houko).
