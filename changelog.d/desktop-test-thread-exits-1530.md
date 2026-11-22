### Fixed

- Propagate `Fatal`, `FailNow`, `SkipNow` and panics from Windows `onMain` and Linux `onLoop` test helpers back to the test goroutine immediately while keeping the window and shared GTK threads alive for cleanup and later tests (#410, #1530).
