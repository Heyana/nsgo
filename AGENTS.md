# NSGo

NSGo is a cross-platform native UI engine with a Go backend, ported from the design of NSH. Read [ARCHITECTURE.md](ARCHITECTURE.md) before coding, then the relevant subjects.

## Principles

- Treat `nsh/` as read-only reference. Never edit, commit or vendor it; it is listed in `.gitignore`. Neorio copies it into new worktrees from the main checkout.
- Before designing a subsystem, read how NSH does it in `nsh/docs/dev/` and its crate's module docs.
- Write idiomatic Go. Port NSH's design and names, not its Rust idioms.
- Assume only macOS, Xcode and Go on a user's machine. Building every target except the x86_64 emulator's from there must need nothing else: no NDK, Android SDK, JDK, GTK or Windows toolchain.
- The `android_x64` target is the one exception: Go links android/amd64 only with cgo, so it needs an NDK.
- Never hand-write C. Reach system APIs through purego or `syscall`, and use cgo only where Go itself demands it (linking for iOS and x86_64 Android).
- Prebuild what users cannot build with Go and Xcode, such as the Android runtime's `classes.dex`, and commit it with the command that rebuilds it (`platform/android/runtime/build.sh`).
- Keep `core` free of cgo and platform imports.
- Write configuration files people edit by hand in YAML, never JSON, including ones ported from NSH's JSON files.
- Mark unimplemented designs as goals or open questions; never document them as existing behavior.

## Components

`base` and `ui` split by what an app cannot build itself, not by how NSH's crates are cut. This outranks NSH: `nsh-base` is the reference for a component's behavior and names, never for which package it belongs to.

- `base` holds the basic implementations of what the engine must own, and nothing else: text (`core.Text`), a pressable view with a role, a virtualized list, the overlay behind popups, menus, dialogs and sheets, navigation, safe areas and the keyboard.
- Each of those takes the app's styles part by part and has no look.
- `ui` composes `base` into everything else: styled controls and variants, fields and selects, settings, a sidebar, a command palette, toasts. A component looking complex is the sign it belongs here, since its complexity is arrangement rather than engine access.
- A component belongs in `base` only when composing it from `base`'s parts cannot reach what it needs: a role, focus, platform input, virtualization or a system surface. Sharing a look with `ui` is never a reason.
- When in doubt, build it in `ui`; move it down once a second component needs the same behavior and cannot share it another way.

## Working with the user

- Never create sub-Agents or Git worktrees; work directly in the current checkout.
- Confirm before choosing an answer to an open question in [ARCHITECTURE.md](ARCHITECTURE.md#open-questions).
- Commit or push only when the user asks.

## Verification

Run `go vet ./...` and `go test ./...` from the repository root once Go code exists. The development setup lives in [README.md](README.md#development).

Run `go tool nsgo build -target web -open=false examples/gallery` when changing the website; the website is the gallery's web build.

## Documentation

- Update a document only when a fact its reader needs changes.
- Avoid secondary labels and descriptions that restate a heading or value; let the layout explain.
- Keep each document within its budget and written for its reader.
- Record completed user-visible outcomes as one-clause `Unreleased` bullets, ordered by type and merging related ones.
- Repair documents you touch instead of only appending to them.
- Keep `README.md` and `README.zh-CN.md` equivalent; everything else, including code comments, is English.
- Keep documentation on the code branch and reconcile it during merges.
