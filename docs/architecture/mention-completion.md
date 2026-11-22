# Mention completion

This is the design of issue [#46](https://github.com/nsgo-dev/nsgo/issues/46), as it is built. The trigger and the query belong to the text field, in `core`; the list of rows is `base.SuggestList`, which owns its behavior, state and accessibility as [components](components.md) says, with its look in `ui.SuggestList`; a chat composer only supplies its rows.

## State and field boundary

The state is a `core.Suggest`, made by `NewSuggest(cx, SuggestOptions)` in the field's owner and passed to `core.Input` (and therefore to `base.Input`, its alias) as `Suggest`. It watches the field's value and UTF-16 caret, recognizes a trigger such as `@`, and exposes whether it is open, the query, the caret's anchor, `CaretEvent.PageBounds`, and whether an input method composes. `Accept(text)` replaces the trigger and query with text and `Close` closes it; the field remains the source of truth for its controlled value. It holds no rows. [Text](text.md) describes how the core feeds it the field's events.

Its rules are those of `base/richeditor`'s `Suggestion`, the precedent #46 names, on a plain field's text, and its options are `SuggestionOptions`' `Char` (as `Trigger`), `AllowSpaces` and `AllowedPrefixes`, with `OnOpen` and `OnClose`. The trigger is valid at the start of a line, and after a space or, when there are some, after one of the allowed prefixes instead. A selection, a deleted trigger, a disallowed space, a caret that leaves the query, a value the app sets or the field losing the focus or unmounting closes the list. The query is the text between the trigger and the caret, counted in UTF-16 units so an emoji before the trigger does not shift the insertion point incorrectly.

Escape and `Close` dismiss the trigger they close, as `Suggestion`'s `dismissedFrom` does: the state keeps that trigger's offset, moved by edits before it, and stays closed on it while its match holds, however the query grows or the caret moves within it. It opens again only once the match ends or a trigger is typed elsewhere. `Accept` dismisses the same way, so an `Insert` that starts with the trigger, such as `@alice`, does not reopen the list on the text it inserted.

## The list

The list is a `base.SuggestList`, given the state and the app's `Items(query)`, which shows the rows in a `core.Overlay` anchored to the caret (`AnchorBounds`, held to the room on its side) while the state is open and the rows are not empty. It takes a provider-neutral row:

```go
type SuggestItem struct {
	ID, Label, Detail string
	Insert           string
	Disabled         bool
}
```

`Insert` replaces the trigger and query; when it is empty, `Label` is inserted. The list does not fetch a network resource, parse a chat protocol or decide which user a row represents.

It runs on the listbox `Select` and `Combobox` share (`base/listbox.go`), as a list beside a search field, so its rows, highlight, paging, the pointer's highlight and announcements have one implementation. It values the rows by their index, so the app's IDs need not be unique, and gives the listbox no query, so it shows the app's rows in the app's order and filters none; the highlight starts on the first enabled row as the rows show and as the query changes. Like other `base` controls it takes the app's parts, the popup's, the list's, the rows' and `RenderItem`, and has no look; `ui.SuggestList` gives it a menu's panel, a dropdown's rows and a row's label over its detail, with `Leading` drawing what goes before the label. An input other than Composer gets the same behavior by placing a `SuggestList` beside its field. The overlay scrolls its rows, a `List` with `ListViewportParent`, and never changes the field's layout. A row's icon and visual style belong to the caller.

## Keys and accessibility

While the list shows and no input method composes in the field, Up and Down move the highlight over the enabled rows and around the ends, Page Up and Page Down by the rows in view, stopping at the ends, all repeating as they are held; Return and Tab accept the highlighted row, and Escape closes the list. As the listbox's keys beside a search field, they are bound with `KeyAlways` inside a `Show` of that state, Return and Tab only while an enabled row is highlighted, so a text field's normal caret and submit behavior returns when the list closes, has no enabled row or an input method composes. Acceptance reports one controlled `OnChange` and moves the caret after the inserted text in the same batch.

A press on a row accepts it, as a listbox row's does: desktops accept as the button goes down, phones on the press. The rows take no focus (`NoFocus`) and the list's `Scroll` keeps the software keyboard for taps on them (`KeyboardTapsHandled`), so the field keeps the keyboard focus and a phone's software keyboard stays up. A software keyboard types into the field rather than pressing bound keys, so on a phone pressing a row is how one is chosen, and its Return types a line in Composer, which ends the query as a disallowed space does. On iOS the accepted text goes into the focused editor through UITextInput between the input delegate's will and did calls, which Apple's keyboards and Sogou's follow; a third-party keyboard that counts the characters of the word it is completing, as Qwen's English mode does, may still delete that many from the accepted text on its next key, as it does after any tap that moves the caret, Apple's own text fields included.

The rows take the option role in one listbox, which the list's `Label` names, and the field takes the combobox role with `Expanded` following the list's `OnOpen` and `OnClose`, as `base.Combobox`'s field does; the app gives the field both, as Composer does. The keyboard focus stays in the field and `core` has no active-descendant attribute, so a highlight the keyboard moves, the first row's among them as the rows show or the query changes, is announced with `Scope.Announce` while a screen reader runs, as the listbox does beside a search field.

`Composer`'s `Suggest` field maps the app's rows to the list's `Items` and places `ui`'s form of the list above the caret, beside its editor; `SuggestLabel` names the list apart from the editor's `Label`:

```go
// Suggest offers rows for the query typed after "@" at the caret. Choosing a row replaces the "@" and the query with its Insert, or its Label when Insert is empty.
Suggest func(query string) []Suggestion

// Suggestion is a Composer's row: a SuggestItem and the icon drawn before it.
type Suggestion struct {
	base.SuggestItem
	Icon func() *core.SVG
}
```

It does not own token rules, rows' behavior or key capture, so another input can use the same completion behavior. The gallery's chat demo remains an example rather than a dependency of the component.

## Query timing and results

`Items` runs synchronously and untracked on the engine goroutine, in a memo of the list's, once per distinct query while the state is open and again as it opens; a panic in it reaches the nearest `Catch`. A synchronous callback must be bounded: an app may memoize it, or keep slow data in its own resource state and return what it has, and rows from asynchronous data refresh as the query changes. Results are ordered by the app. Since the call returns the rows for the query it is given, no result arrives for an older query. An empty result shows no list and leaves Return and Tab to the field.

## Decisions

The user confirmed these answers, the ones the implementation has, on 2026-11-15:

1. `Items` runs synchronously on every query change, on the engine goroutine; the stable API takes no app-owned async resource.
2. An empty result closes the overlay: nothing shows, the field is not expanded and its keys are its own, and the list has no empty state.
3. Tab accepts a row as Return does, and neither submits a row when none is: with no enabled row highlighted they are the field's, Return submitting or typing a line and Tab moving the focus.
4. A `Suggest` has one trigger and one allowed-prefix rule, and a field takes one `Suggest`.
5. `SuggestItem.Insert` replaces only the trigger and the query; there is no explicit edit range.

## Verification

These checks exist:

- `core` tests cover trigger prefixes and a line's start, spaces, UTF-16 offsets, offsets moved by edits, a selection and the caret closing and reopening the query, acceptance into an uncontrolled field, a query that stays closed as the caret moves after Escape and after accepting an `Insert` that starts with the trigger, a query ending as its field unmounts, and a `Suggest` whose field unmounts and mounts again; and `Reveal` stopping at an overlay;
- `base` tests cover the list's keys over enabled rows and the ends, Tab, the field's combobox role and `Expanded`, the rows' option role and `NoFocus`, the highlight's announcement while a screen reader runs, the field's keys with no rows, with only disabled rows, while an input method composes and after Escape, one `Items` call per query, closing as the field loses the focus, a desktop's row accepted as the button goes down, a phone's tapped row keeping the keyboard and the focus, and the overlay at the caret;
- `ui/chat` tests type `@re`, choose a row with Down and Return in the middle of the draft and find one draft change, the row inserted with the caret after it, the list gone, the editor no longer expanded and nothing sent, and cover disabled rows and a row's detail, scrolling rows, Escape, another field taking the focus, a query with no rows, a draft cleared by sending, a row pressed on a phone with the field still focused, and a row accepted as a desktop's button goes down.

`TestComposerSuggestionsTapOnAPhone` links the native host on mobile targets. It first focuses the editor and observes a real software keyboard, then presses a `KeyboardTapsNever` control and waits for keyboard dismissal and blur. After reopening the keyboard and accepting a mention, it checks insertion, the closed list and retained focus, and observes keyboard visibility for another 500 ms. It passed with `nsgo test -target ios_simulator` on iPhone 16 / iOS 18.4 and `-target android` on an API 35 emulator on 2026-11-22. Android's runtime was rebuilt from the Java source for that run; CI rebuilds the shipped runtime after merge.
