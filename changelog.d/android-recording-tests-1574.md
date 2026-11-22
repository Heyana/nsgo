### Fixed

- Android recordings fall back to the device's sample rate when the rate asked for is outside what `AudioFormat` describes, rather than ending as unavailable, and a recording whose Go process ended no longer keeps the app recording while the next process's pipe is full (#1574 by @houko).
