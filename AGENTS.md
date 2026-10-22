# NSGo

NSGo is a cross-platform native UI engine with a Go backend, ported from the design of NSH. Read [ARCHITECTURE.md](ARCHITECTURE.md) before coding, then the relevant subjects.

## Principles

- Treat `nsh/` as read-only reference. Never edit, commit or vendor it; it is listed in `.gitignore`. Neorio copies it into new worktrees from the main checkout.
- Before designing a subsystem, read how NSH does it in `nsh/docs/dev/` and its crate's module docs.
- Write idiomatic Go. Port NSH's design and names, not its Rust idioms.
- Assume only macOS, Xcode and Go on a user's machine. Building for every target from there must need nothing else: no NDK, Android SDK, JDK, GTK or Windows toolchain.
- Never hand-write C. Reach system APIs through purego or `syscall`, and use cgo only where Go itself demands it (linking for iOS).
- Prebuild what users cannot build with Go and Xcode, such as the Android runtime's `classes.dex`, and commit it with the command that rebuilds it (`platform/android/runtime/build.sh`).
- Keep `core` free of cgo and platform imports.
- Write configuration files people edit by hand in YAML, never JSON, including ones ported from NSH's JSON files.
- Mark unimplemented designs as goals or open questions; never document them as existing behavior.

## Working with the user

- Confirm before choosing an answer to an open question in [ARCHITECTURE.md](ARCHITECTURE.md#open-questions).
- Commit or push only when the user asks.

## Verification

Run `go vet ./...` and `go test ./...` from the repository root once Go code exists. The development setup lives in [README.md](README.md#development).

Run `bun run typecheck`, `bun run check` and `bun run build` in `website/` when changing the site; its commands are in [website/README.md](website/README.md#commands).

## Documentation

- Update a document only when a fact its reader needs changes.
- Keep each document within its budget and written for its reader.
- Record completed user-visible outcomes as one-clause `Unreleased` bullets, ordered by type and merging related ones.
- Repair documents you touch instead of only appending to them.
- Keep `README.md` and `README.zh-CN.md` equivalent; everything else, including code comments, is English.
- Keep documentation on the code branch and reconcile it during merges.
