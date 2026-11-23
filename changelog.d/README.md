# Changelog fragments

A pull request with a change users can see records it here, in a file of its own, instead of in `CHANGELOG.md`, so pull requests no longer conflict over the top of `Unreleased`. Name the file after the change in kebab case, such as the branch's name (`keep-caret-after-paste.md`), and put each one-clause bullet, with its reference, under the Keep a Changelog type it is:

```markdown
### Fixed

- Keep the caret after a paste on Linux (#12 by @you).
```

The types are `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed` and `Security`; a fragment may hold several. `CHANGELOG.md` shows no heading per type: a release lists the bullets in that order, then by file name. `go run ./tools/changelog check` validates the fragments, as `go tool nsgo test` does.

To release, run `go run ./tools/changelog release X.Y.Z`, which folds the fragments into `CHANGELOG.md` and deletes them, then turns `Unreleased` into the version's section; edit the section, merge it and tag `vX.Y.Z` on main. Once `release.yml` has published the release, remove the section from `CHANGELOG.md`, which keeps only what is not released: the notes live on [Releases](https://github.com/nsgo-dev/nsgo/releases). A branch started before fragments moves the bullets it added to `CHANGELOG.md` into one with main's copy of the tool, which the branch lacks: `git show origin/main:tools/changelog/main.go > /tmp/changelog.go && go run /tmp/changelog.go adopt -base origin/main -kind Fixed <name>`; fold the result into the commit that added the bullets (`git commit --amend` on a one-commit branch), since a rebase that still replays that commit's edit to `CHANGELOG.md` still conflicts.
