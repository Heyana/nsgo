# NSGo

NSGo is a cross-platform native UI engine with a Go backend, ported from the design of NSH. Read [ARCHITECTURE.md](ARCHITECTURE.md) before coding, then the relevant subjects.

## Principles

- Treat `nsh/` as read-only reference. Never edit, commit or vendor it; it is listed in `.gitignore`.
- Before designing a subsystem, read how NSH does it in `nsh/docs/dev/` and its crate's module docs.
- Write idiomatic Go. Port NSH's design and names, not its Rust idioms.
- Keep `core` free of cgo and platform imports.
- Mark unimplemented designs as goals or open questions; never document them as existing behavior.

## Working with the user

- Confirm before choosing an answer to an open question in [ARCHITECTURE.md](ARCHITECTURE.md#open-questions).
- Commit or push only when the user asks.

## Verification

Run `go vet ./...` and `go test ./...` from the repository root once Go code exists. The development setup lives in [README.md](README.md#development).

## Documentation

- Update a document only when a fact its reader needs changes.
- Keep each document within its budget and written for its reader.
- Record completed user-visible outcomes as one-clause `Unreleased` bullets, ordered by type and merging related ones.
- Repair documents you touch instead of only appending to them.
- Keep `README.md` and `README.zh-CN.md` equivalent; everything else, including code comments, is English.
- Keep documentation on the code branch and reconcile it during merges.
