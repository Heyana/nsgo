### Added

- Document game input, drawing, audio, frame-timing and `Env` capabilities per secondary target (HarmonyOS, mini programs, watchOS and tvOS) in `docs/roadmap.md`, fix `Env` capability reporting on tvOS (`CanRecognizeSpeech` false; ignore `RequestOrientation`), HarmonyOS (`HasWebView` true) and mobile mini programs (`CanTakePhoto` true when `chooseMedia` is available), and add unit tests for each target's `Env` contract (#1117).
