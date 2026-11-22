### Fixed

- Let steps and waits under `nsgo test -target` move the test's clock while a style transition waits for its `OnTransitionEnd`, as they do headless, so a dialog or popover closed through its `EndingStyle` leaves the screen instead of staying and taking presses.
