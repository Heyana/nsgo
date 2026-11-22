### Added

- Repeat daily and weekly scheduled toast notifications on Windows (chaining `ScheduledToastNotification` while running and rearming persisted schedules on launch), schedule notifications natively through `UNUserNotificationCenter` on watchOS, keep scheduled notifications while the app or page runs on Linux and the web, and guard tvOS notification calls to badge-only support (#1540).
