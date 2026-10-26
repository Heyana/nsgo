# Editor and DiffView

`base/editor` is NSH's `Editor` (`nsh/docs/dev/editor.md`) and `base/diff` its `DiffView` (`nsh/docs/dev/diff.md`). Read this before changing either, or the highlighting they share in `base/internal/syntax`. Each is a package of its own because its highlighting links chroma's lexers.

## Editor

| File | What |
| --- | --- |
| `rope.go` | The text as lines in chunks of 512: a line is a binary search over the chunks, an edit splices the chunks it touches |
| `buffer.go` | Edits as change lists, carrying places through them, undo grouped as editors group it, UTF-16 columns |
| `state.go` | Selections (the primary one last, merged when they overlap), moves, and every editing command at every selection |
| `display.go`, `fold.go` | Display rows: soft wrap at columns, folds by indentation or the app's, tabs and wide characters as columns |
| `highlight.go`, `search.go` | Highlighting jobs and their tokens by line; find and replace on `regexp`, line by line |
| `editor.go`, `view.go`, `api.go` | The controller (props, keys, the sink, presses, completion, definitions, hovers), the view's layers, the API and the ref |

- Places are a line and a byte column inside the editor; the API speaks UTF-16 columns, as language servers do. Lines end with `\n` only.
- An edit is one `buffer.apply` of every selection's edits, which returns the changes applied, the last edit first; selections, marks, folds and tokens move through them. Typing at one place and deleting merge into one undo step; a white space after a word, a move or a press starts another. 1000 steps are kept.
- `Value` replaces the text as one edit of what differs, so carets and folds away from the change stay, and the value `OnChange` just reported costs nothing.
- Without folds or soft wrap the display rows are the lines and nothing is laid out, so 200 000 lines open at once. Widths are columns of the font's `0`; past 20 000 lines only the edited lines widen the scroll width.
- The view is a vertical `Scroll` of the gutter and the text area (a horizontal `Scroll` without soft wrap). Only the rows of a `window.Rows` window are made, each a `Text` whose `SpansFunc` follows the revisions of the text, the highlighting and the marks. Selections, matches, brackets, the active line, the definition link and the carets are layers of boxes over the rows in view.
- A native `Input`, the sink, sits invisible at the primary caret, so input methods and soft keyboards work there; its changes become backspaces and typed text, its pastes `paste`. The keys are bound with `KeyAlways` while the sink has the focus and no composition runs; the completion list binds its own after them, so they win while it shows.
- On desktops a press places the caret (two clicks a word, three a line, Shift extends, Alt adds a caret) and a responder claim on the first move drags; on touch screens a tap places it, a double tap or long press selects a word with handles a finger drags, and asks for the edit menu with `OnContextMenu`.

Highlighting stays off the engine goroutine: a job lexes a snapshot of up to 2000 lines with chroma on a goroutine of its own and posts their tokens back. It starts at the last line at column 0 before the first line needing tokens, at most 200 lines back, where the lexer is most likely in its root state, and stops once three lines past the changed ones come out as they were. Jobs go first to the rows in view and 1000 lines past them. An edit moves the tokens of the lines it touches with the text, so colors do not flicker while the next job runs.

## DiffView

- `model.go`: lines keep their line breaks, so a missing final one is a change. The line diff is Myers' algorithm after the common prefix and suffix, bounded to 20 000 steps; within a change, deleted lines pair with the added lines most like them (a weighted longest common subsequence of similarity, at least 0.4, up to 64 lines a side; the rest in order), and a pair's changed words come from a Myers diff of its tokens, none when over 70 % changed.
- `patch.go` reads git's and `diff -u`'s unified diffs: renames, copies, new, deleted and binary files, missing final line breaks. A patch's file shows its hunks' lines only, under separators of their headers, numbered as in the file.
- `view.go` joins a patch's files into two texts, so rows, numbers and hunks count across files; each file's sides are lexed on a goroutine. Rows are of one height, so a window of them is found by division.

## Where NSGo departs from NSH

| NSH | NSGo | Why |
| --- | --- | --- |
| A default look from the theme's tokens | No look but the highlighting's palette: `Styles` gives every part | `base` has no look; themes are the app's |
| `ropey` | Lines in chunks | Lines are what every part reads; chunks bound an edit's work |
| syntect's state kept every 16 lines | chroma from a line at column 0 before the change, until its tokens meet the old ones | chroma keeps no state to resume from |
| imara-diff's histogram with git's indent heuristic | Myers after the common prefix and suffix | One algorithm in Go, no dependency |
| The split sides scroll sideways on their own, kept together | The rows' box scrolls sideways as one, gutters included | One scroll view; no echo to break |
| Tailwind classes and `PartStyle`s by part | A `core.Style` per part | Styles are Go values |
