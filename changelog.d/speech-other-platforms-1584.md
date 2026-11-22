### Added

- Support `Scope.StartSpeech` on Android (`android.speech.SpeechRecognizer` with `RECORD_AUDIO` permission handling), Windows (WinRT `Windows.Media.SpeechRecognition.SpeechRecognizer` and `ContinuousRecognitionSession`) and the web (`SpeechRecognition` / `webkitSpeechRecognition`, reporting `unavailable` when absent) (#1584).
