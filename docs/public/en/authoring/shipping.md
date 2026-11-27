---
title: Words, tests and publishing
description: Translate what a component says, test it as a user would, and offer it to Go and TSX apps.
group: Writing components
order: 74
---

## Words

A component says its own words, such as a button's name for screen readers, through its package's catalogs rather than string literals. Put a `locales/` package in the library with the source language's catalog, and a file that generates the code:

```yaml
# locales/en.yaml
clear:
  message: Clear
  description: What screen readers say of the button that empties a chip input.
```

```go
// Package locales says the chips' words in the language the app shows.
package locales

//go:generate go tool nsgo i18n
```

`go generate ./locales` writes a function per message, which the component calls with its scope: `locales.Clear(cx)` says it in the language the app shows. Commit the generated `catalog_gen.go`, so apps build with Go alone. Add a file per language you translate to (`zh-Hans.yaml`); an app supplies others in its own catalogs, under a key naming your package's import path, as for [NSGo's own words](../localization.md#nsgos-own-words).

## Tests

`nsgotest` runs a component the way a user uses it, headless. Mount it in a small root and act on it by role and text:

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestToggle(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		return chips.Toggle{On: on.Get, OnChange: on.Set, Label: "Bold"}
	})
	app.Press("role:switch Bold")
	if !app.Find("role:switch Bold").Props.Checked {
		t.Fatal("the toggle did not turn on")
	}
}
```

From the library's root, `nsgo test` takes every package, and `-target macos` or another target runs the same tests in a native window. Test both appearances and a pseudo locale (`-lang en-XA`) to catch hard-coded colors and words. [Testing](../testing.md) covers the rest.

## Publishing

A component library is a Go module: apps add it with `nsgo add <module>` (or `go get`) and import its packages. Keep its unstyled and styled forms in separate packages when both exist, so an app with a look of its own imports no styles it does not use.

Libraries listed in NSGo's built-in index ([`cmd/nsgo/registry.yaml`](https://github.com/nsgo-dev/nsgo/blob/main/cmd/nsgo/registry.yaml)) appear in `nsgo add -list` and can be added by short name (`nsgo add <name>`). To list a library, open a pull request adding an entry to `cmd/nsgo/registry.yaml` in canonical form (sorted by name, fields in schema order, two-space indent). A listed library needs:

- a tagged release (never an untagged commit);
- a module path that resolves without a redirect;
- a resolved dependency graph that keeps apps on a released NSGo;
- an SPDX license expression.

### Registry checks

NSGo's [registry workflow](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/registry.yml) runs weekly and on pull requests that change `cmd/nsgo/registry.yaml`. For each listed item it checks that:

- `@upgrade` (or the item's pinned `version`) resolves to a semantic release tag, never a pseudo-version or pre-release, and the module's `go.mod` declares the listed path;
- the repository URL named by the module path answers without redirecting;
- adding the item to a scratch module requiring NSGo's latest release keeps NSGo on a released version;
- `nsgo-module.yaml` (when present) and every linked `nsgo-natives.yaml` parse cleanly;
- for a library, `go tool nsgo check -target linux_x64` passes for a scratch package importing its declared `import` packages (compiling without running or testing third-party code);
- for a component, the module's `nsgo-module.yaml` at `@upgrade` declares it, and (for modules other than NSGo itself, whose `check.yml` vets components in place) copying it into a scratch app requiring NSGo's latest release passes `go tool nsgo check -target linux_x64`.

TSX apps reach Go components once they are registered, in an `init` function:

```go
func init() {
	tsx.Register("example.com/chips", map[string]core.Node{"Toggle": Toggle{}})
}
```

`import { Toggle } from "example.com/chips"` then takes the fields as props ([TSX apps](../tsx.md#go-from-tsx)).

### Module manifest

An optional `nsgo-module.yaml` beside `go.mod` at the module root tells `nsgo add` and `nsgo check` what apps should know about the library:

```yaml
version: 1
import: [player]
targets: [macos, windows_x64, linux_x64, ios, ios_simulator]
notes: |
  FFmpeg's libraries are LGPL 2.1: offer their source beside your downloads,
  credit FFmpeg and allow replacing them.
components:
  chat: ui/chat
  file-picker: ui/filepicker
```

- The file is strict: one YAML document of at most 1 MiB, with unknown fields, anchors, aliases and merge keys refused. `nsgo build` never reads it.
- `import`: package directories relative to the module root (`.` for the root). Each must be a non-`main` package of this module, never of a nested module. `nsgo add` prints an import line for each; without the file (or when `import` is omitted), it prints the root package's.
- `targets`: target names and groups as `targets.Expand` takes them (`macos`, `windows_x64`, `all`, `miniprogram`, `tui`). `nsgo add`'s plan names any of the app's `build.targets` the list lacks, and `nsgo check` warns for each target it checks about any linked module whose list lacks that target.
- `notes`: plain text of at most 4 KiB, with no control or Unicode format characters other than line breaks. `nsgo add` prints it last, under `From <module>, the author's notes:`, with each line marked.
- `components`: a map from component names (`1` to `64` characters of `[a-z0-9-]`, not starting or ending with `-`) to package directories below the module root. Each directory must be a non-`main` package of this module whose last path element equals its Go package name. `nsgo add`'s plan lists each with `nsgo add <module>#<name>`.
- New fields are added within `version: 1`. When a module's `go.mod` requires a newer NSGo than the running tool and uses a field the tool does not know, `nsgo` fails naming the `go get github.com/nsgo-dev/nsgo@<version>` command that brings it.
