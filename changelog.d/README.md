# Changelog fragments

A pull request with a change users can see records it here, in a file of its own, instead of in `CHANGELOG.md`, so pull requests no longer conflict over the top of `Unreleased`. Name the file after the change in kebab case, such as the branch's name (`keep-caret-after-paste.md`), and put each one-clause bullet, with its reference, under the Keep a Changelog type it is:

```markdown
### Fixed

- Keep the caret after a paste on Linux (#12 by @you).
```

The types are `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed` and `Security`; a fragment may hold several. `CHANGELOG.md` shows no heading per type: a release lists the bullets in that order, then by file name. `go run ./tools/changelog check` validates the fragments, as `go tool nsgo test` does.

To release, run `go run ./tools/changelog release X.Y.Z`, which folds the fragments into `CHANGELOG.md` and deletes them, then turns `Unreleased` into the version's section; edit the section, merge it and tag `vX.Y.Z` on main. A branch started before fragments moves the bullets it added to `CHANGELOG.md` into one with `go run ./tools/changelog adopt -base origin/main -kind Fixed <name>`.
