### Added

- Ship command-line programs beside desktop apps with `commands`, built from Go packages, with the app's linker flags, identity and their own `ldflags`, signed with the app, linked into `/usr/bin` by the Linux packages and into Homebrew's `bin` by its cask, and put on the user's `Path` by the Windows installer with `windows.path` (#2203 by @txchen).
