---
title: Copying components
description: Copy declared Go components such as chat and file-picker into your app with nsgo add, rewrite imports, inspect changes with nsgo diff, and keep their license notices and source records.
group: Components
order: 35
---

When an app needs to change a component rather than only configure it, `nsgo add` copies a package that its module declares under `components:` in `nsgo-module.yaml` directly into the app. NSGo declares `ui`'s eight subpackages (`chat`, `code-editor`, `commit-graph`, `diff-view`, `file-picker`, `flow-diagram`, `rich-editor` and `terminal`) as copyable components and lists them in its built-in index:

```sh
nsgo add -list chat                              # find chat in NSGo's built-in index
nsgo add chat                                    # copy ui/chat into components/chat
nsgo add file-picker                             # copy ui/filepicker into components/filepicker
nsgo add -dir internal/components/chat chat      # place chat at another path in the module
nsgo add -tests=false file-picker                # omit _test.go files and testdata/
nsgo add -dry-run -diff chat                     # preview the plan and every copied file
```

You can also copy any module's declared component directly by path with `nsgo add <module>[@version]#<component>`, such as `nsgo add github.com/nsgo-dev/nsgo#file-picker`.

Once copied, the component is the app's own Go code.

## Where copies go

By default, a copy lands at `components/<package>` relative to `nsgo.yaml` (or relative to the module root when there is no `nsgo.yaml`), so `chat` lands in `components/chat` and `file-picker` lands in `components/filepicker`. Set `components.dir` in `nsgo.yaml` to change the base directory:

```yaml
components:
  dir: ui/components
```

Pass `-dir <path>` to place a single named component at another path inside the module; its last element must equal the component's Go package name. `nsgo add` refuses destinations outside the module, inside `vendor`, `testdata`, nested modules, hidden/underscore directories, symlinked parents, existing paths, or paths that collide case-insensitively with an existing entry.

## Version selection

When no `@version` is typed, `nsgo add` uses:

1. the version of the source module that `go.mod` already requires (this rule is final: if that version does not declare the component, `nsgo add` refuses and prints the `nsgo add <name>@<version>` command that takes a version that does);
2. otherwise, the version listed in the registry index (if added by name);
3. otherwise, `@upgrade` (the newest release).

A typed version higher than `go.mod`'s requirement announces the raise on the first line of the plan. A typed version lower than `go.mod`'s requirement leaves `go.mod` untouched and notes that the copy is older. Modules whose `go` directive is below `1.22` or newer than the app's planned `go` line are refused.

## What copying does

1. **Verifies the archive.** `nsgo add` downloads the module zip in an isolated directory using `add`'s proxy-only, `GOTOOLCHAIN=local` environment, validates the zip structure (`zip.CheckZip`), and checks its `dirhash.HashZip` digest against both the download's `Sum` and any entry in `go.sum` before reading files directly from the archive.
2. **Checks copy rules and flags.** Every file is checked against the copied-component allowlist, path restrictions, count/size limits (256 files, 1 MiB per file, 4 MiB total), UTF-8 without BOM, and Go AST rules (refusing `import "C"`, internal imports, `tsx` outside tests, `//go:build ignore`, foreign `//go:generate`, and disallowed `//go:embed` targets). Risky constructs (control/format characters, non-ASCII identifiers, imports of `unsafe`, `syscall`, `golang.org/x/sys/...`, `purego`, `os/exec`, `plugin`, and `//go:linkname`/`//go:wasm*`/`//go:cgo_*`/`//go:debug`) and `go vet` failures are shown with positions and refused under `-y`.
3. **Rewrites imports and comments.** Imports of the component itself, its `locales/` package (such as `ui/chat/locales`), and other components of the same module (which are copied automatically when needed) are rewritten to the copy's import path while keeping their import names. Whole-path occurrences in comments are rewritten too, string literals containing rewritten paths are listed in the plan, and every Go file is formatted with `go/format`.
4. **Generates catalogs.** When the component has `locales/*.yaml` (as `chat` does), `nsgo add` generates `locales/catalog_gen.go` in memory under the copy's final import path with this `nsgo`'s generator so `nsgo check` passes immediately. If the app's own catalogs supply the original `locales` path, the plan names the key to update once the copy is imported.
5. **Plans requirements and vets targets.** External and non-component same-module imports are added to `go.mod` as direct requirements (at least at the versions the source module requires, never lowering the app's). The planned copy is vetted with `go vet -mod=readonly` for every target in `build.targets` (or the host without `nsgo.yaml`); targets whose toolchain the host lacks are listed under `not vetted` and do not block `-y`.
6. **Writes atomically.** Under the module's check lock, each copy is staged in a hidden `.nsgo-copy-*` directory beside its destination and renamed into place along with `go.mod` and `go.sum`. Any error or interrupt restores the module cleanly.

## License notices and `nsgo-copy.yaml`

License and notice documents from the source module root and directories between it and the component (such as NSGo's `LICENSE` and `NOTICE`) are copied beside the component (duplicate filenames are refused). `nsgo add` classifies every license document the copy brings—including nested asset licenses such as `icons/OFL.txt`—and applies the app's `licenses` policy immediately.

During `nsgo build`, a directory holding `nsgo-copy.yaml` receives its own section in `THIRD_PARTY_NOTICES.txt` headed `<import path>, copied from <module> <version>`, keeping those texts separate from the app's own license section. For Apache-2.0 components such as `chat` and `file-picker`, mark files you modify as required by section 4(b); `nsgo diff -stat` lists every changed file.

Each copied directory holds a deterministic, hash-free `nsgo-copy.yaml` recording its source module, version, component name, test choice, copied files and generated files:

```yaml
# Written by nsgo add. nsgo diff compares this directory with the copy of this version.
version: 1
module: github.com/nsgo-dev/nsgo
module_version: v0.4.0
component: chat
tests: true
files:
  - LICENSE
  - NOTICE
  - actions.go
  - actions_test.go
  - chat.go
  - chat_test.go
  - composer.go
  - controls.go
  - conversation.go
  - effort.go
  - find.go
  - locales/en.yaml
  - locales/locales.go
  - locales/zh-Hans.yaml
  - native_test.go
  - reasoning.go
  - reasoning_test.go
generated:
  - locales/catalog_gen.go
```

Running `nsgo add` again at the same version reports the existing copy's version and directory without making changes. Moving a copy needs no extra command; `nsgo-copy.yaml` moves with it.

## Comparing copies with `nsgo diff`

Use `nsgo diff` to inspect what your app changed in a copy, what another version would change, or which copies in the module have local edits:

```sh
nsgo diff                              # list each copy, its version, and edited/unedited status
nsgo diff chat                         # unified diff of local edits against the recorded version
nsgo diff components/chat              # target a copy by directory path
nsgo diff chat@v0.5.0                  # unified diff from the recorded version to v0.5.0
nsgo diff chat@upgrade                 # unified diff from the recorded version to the newest release
nsgo diff -stat chat                   # changed lines per file (also lists modified files for Apache-2.0 §4(b))
nsgo diff -exit-code chat              # exit with status 1 when there are differences
```

- **Target resolution.** A directory holding `nsgo-copy.yaml` matches directly. A bare name matches the module's own `nsgo-copy.yaml` records first so it works offline; if two copies from different modules share the same component name, `nsgo diff` asks for the directory (or `module#component`). Otherwise it resolves the name through the built-in index or a configured `@registry/name` and matches a record by module and component.
- **Clean reference copy.** `nsgo diff` reconstructs the recorded (or requested) version directly from the verified module zip (`zip.CheckZip` and `dirhash.HashZip` checked against `go.sum`), applying the record's `tests` choice, ancestor licenses, Go import and comment rewrites (with sibling copies at their current record directories) and `go/format`, excluding generated catalog files such as `locales/catalog_gen.go`.
- **Git-compatible output.** Unified diffs use `diff --git` headers and module-root-relative paths with three lines of context, so `git apply` and `git apply -R` apply them directly. Binary files report `Binary files ... differ`.
- **Line endings and formatting.** Files are compared with CRLF read as LF, and Go files whose `go/format` output matches the recorded version count as unedited.
- **Offline use and unavailable versions.** Recorded versions are read from `GOMODCACHE` first so `nsgo diff` works offline under `GOPROXY=off`; otherwise they download through Go module proxies with `GOTOOLCHAIN=local`. When a recorded version can no longer be downloaded, every non-generated file counts as edited and the output explains why.
- **Safety.** Every path in `nsgo-copy.yaml` is validated on read (refusing escapes such as `../form/form.go` with the line number), all reads stay inside an `os.Root` at the copy's directory, and symlinks are refused.

## Updating a copy

Pass another version to `nsgo add` to update an existing copy file by file while keeping the app's own edits:

```sh
nsgo add chat@v0.5.0                  # update by component name from nsgo-copy.yaml
nsgo add chat@upgrade                 # update to the newest release
nsgo add chat@v0.4.0                  # downgrade to an earlier version
nsgo add components/chat@v0.5.0       # disambiguate by directory when two copies share a name
nsgo add -dry-run chat@v0.5.0         # preview per-file actions without writing
```

A component name is matched against the module's `nsgo-copy.yaml` records first so resolution works offline; if two copies from different modules share a name, pass the copy's directory or `module@version#component`.

For each file, `nsgo add` reconstructs the recorded version (base) and the target version (author) and compares them with the directory (app), reading CRLF as LF and formatting Go files before comparing:

- **App left the file alone:** takes the new version and preserves CRLF line endings if the file in the directory uses CRLF.
- **Author left it alone, or both made the same change:** stays untouched.
- **Both changed it differently:** the update is refused, naming every conflicting file and writing nothing.
- **Author dropped it:** deleted when the app left it alone; flagged when the app edited it (refused under `-y`, or confirmed interactively).
- **Author added it and a file of that name already exists** (compared case-insensitively): refused, writing nothing.

Around the files, `locales/catalog_gen.go` is regenerated from the resulting catalogs, other copied components imported from the same module move to the same version in the same plan, `go.mod` requirements, license policy, and target vetting work as for a new copy, and `nsgo-copy.yaml` records the new version and its files. A version whose module no longer declares the component is refused, naming that version.

### Carrying app edits over by hand

When both the app and the author edited the same file, `nsgo add` stops before writing anything rather than guessing at a line merge. Carry your edits over by hand with `nsgo diff`:

1. Save your own edits against the recorded version:
   ```sh
   nsgo diff chat > chat.patch
   ```
2. Inspect what the author changed in the target version:
   ```sh
   nsgo diff chat@v0.5.0
   ```
3. Temporarily restore the conflicting file to the recorded version (for example with `git apply -R` on that file's hunk from `chat.patch`, or by reverting your edits to it) so `nsgo add` sees it as unedited.
4. Run `nsgo add chat@v0.5.0` to take the new version cleanly and update `nsgo-copy.yaml`.
5. Re-apply your edits from `chat.patch` onto the updated file, then run `nsgo diff chat` and `nsgo check` to verify only your intended edits remain and the copy builds cleanly.
