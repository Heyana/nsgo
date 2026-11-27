---
title: Copying components
description: Copy declared Go components into your app with nsgo add, rewrite imports, and keep their license notices and source records.
group: Components
order: 35
---

When an app needs to change a component rather than only configure it, `nsgo add` copies a package that its module declares under `components:` in `nsgo-module.yaml` directly into the app:

```sh
nsgo add github.com/acme/kit#field                  # copy into components/field
nsgo add github.com/acme/kit@v1.2.0#field           # copy a specific version
nsgo add -dir internal/components/field github.com/acme/kit#field
nsgo add -tests=false github.com/acme/kit#field     # omit _test.go files and testdata/
nsgo add -dry-run -diff github.com/acme/kit#field   # preview the plan and every file
```

Once copied, the component is the app's own Go code.

## Where copies go

By default, a copy lands at `components/<package>` relative to `nsgo.yaml` (or relative to the module root when there is no `nsgo.yaml`). Set `components.dir` in `nsgo.yaml` to change the base directory:

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
3. **Rewrites imports and comments.** Imports of the component itself, its `locales/` package, and other components of the same module (which are copied automatically when needed) are rewritten to the copy's import path while keeping their import names. Whole-path occurrences in comments are rewritten too, string literals containing rewritten paths are listed in the plan, and every Go file is formatted with `go/format`.
4. **Generates catalogs.** When the component has `locales/*.yaml`, `nsgo add` generates `locales/catalog_gen.go` in memory under the copy's final import path with this `nsgo`'s generator so `nsgo check` passes immediately. If the app's own catalogs supply the original `locales` path, the plan names the key to update once the copy is imported.
5. **Plans requirements and vets targets.** External and non-component same-module imports are added to `go.mod` as direct requirements (at least at the versions the source module requires, never lowering the app's). The planned copy is vetted with `go vet -mod=readonly` for every target in `build.targets` (or the host without `nsgo.yaml`); targets whose toolchain the host lacks are listed under `not vetted` and do not block `-y`.
6. **Writes atomically.** Under the module's check lock, each copy is staged in a hidden `.nsgo-copy-*` directory beside its destination and renamed into place along with `go.mod` and `go.sum`. Any error or interrupt restores the module cleanly.

## License notices and `nsgo-copy.yaml`

License and notice documents from the source module root and directories between it and the component are copied beside the component (duplicate filenames are refused). `nsgo add` classifies every license document the copy brings—including nested asset licenses such as `icons/OFL.txt`—and applies the app's `licenses` policy immediately.

During `nsgo build`, a directory holding `nsgo-copy.yaml` receives its own section in `THIRD_PARTY_NOTICES.txt` headed `<import path>, copied from <module> <version>`, keeping those texts separate from the app's own license section. For Apache-2.0 components, mark files you modify as required by section 4(b).

Each copied directory contains `nsgo-copy.yaml`, recording the source module, version, component name, test choice, copied files, and generated files (with no timestamps or hashes):

```yaml
# Written by nsgo add. nsgo diff compares this directory with the copy of this version.
version: 1
module: github.com/acme/kit
module_version: v1.0.0
component: field
tests: true
files:
  - LICENSE
  - field.go
  - locales/en.yaml
generated:
  - locales/catalog_gen.go
```

Running `nsgo add` again at the same version is a no-op that prints the copy's version and directory. Moving a copied directory requires no command because `nsgo-copy.yaml` moves with it.
