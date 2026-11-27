### Fixed

- Assign each `nsgotest.App.Touch` finger a unique incrementing `Finger.ID()`, track all held fingers in `EventTouch` with `Changed` set only on the active finger, and report pointer modality on touch and drag start (#2651).
