// Package editor is base's code editor, a port of NSH's Editor (and GPUI
// Kit's): syntax highlighting, line numbers, several carets, folding, soft
// wrap, find and replace, diagnostics and decorations, completion and go to
// definition, with the keys of code editors. It draws its text itself, a
// Text per display row of the rows in view, over its own model: lines in a
// rope, edits as change lists, undo grouped as editors group it. It owns
// behavior and has no look of its own: Styles gives every part, from the
// caret to the completion list, its look; only the built-in highlighting
// colors the text, with NSH's palette.
//
// It is a package of its own because its highlighting links chroma's
// lexers.
package editor

import "github.com/oneo-me/nsgo/core"

// Position is a place in the text: a line from 0 and a column in UTF-16
// units, as a language server's.
type Position struct{ Line, Column int }

// Range is the text from Start to End.
type Range struct{ Start, End Position }

// Selection is a range from Anchor to Head, the caret.
type Selection struct{ Anchor, Head Position }

// Search is what to find: Query as text, or as a regular expression with
// Regex ($1 and ${name} in replacements), whole words only with
// WholeWord, and only inside what was selected when InSelection turned on.
type Search struct {
	Query                           string
	Regex, CaseSensitive, WholeWord bool
	InSelection                     bool
}

// SearchResult is what a search found: how many matches, which one the
// primary selection is on (-1 for none) and the expression's error.
type SearchResult struct {
	Count, Current int
	Err            string
}

// Severity is how serious a diagnostic is.
type Severity uint8

const (
	SeverityError Severity = iota
	SeverityWarning
	SeverityInfo
	SeverityHint
)

// Diagnostic is a problem marked in the text, as a language server's: its
// range, carried through edits until the app sets new ones, its severity's
// part under its text, and with Unnecessary or Deprecated the
// UnnecessaryText or DeprecatedText part over it.
type Diagnostic struct {
	Range       Range
	Severity    Severity
	Message     string
	Source      string
	Code        string
	Unnecessary bool
	Deprecated  bool
}

// Decoration styles a range of the text with Style, and the rows of its
// lines behind the text with LineStyle; Kind, when set, colors its text as
// the highlighting's kind of that name (keyword, string, comment, number,
// title, attr, built), the app's own highlighting.
type Decoration struct {
	Range     Range
	Style     core.Style
	LineStyle core.Style
	Kind      string
}

// FoldRange is lines StartLine to EndLine: a fold shows the first and
// hides the others.
type FoldRange struct{ StartLine, EndLine int }

// CompletionItem is an item of the completion list: Label shown, after
// Kind and before Detail; InsertText (Label when empty) replaces Range
// (from the query's start to the caret when nil); FilterText and SortText
// filter and order it; Documentation shows beside the list.
type CompletionItem struct {
	Label, Kind, Detail  string
	InsertText           string
	FilterText, SortText string
	Documentation        string
	Range                *Range
}

// CompletionTrigger is what asked for completions.
type CompletionTrigger uint8

const (
	// CompletionTyping is a word character typed at a word's end.
	CompletionTyping CompletionTrigger = iota
	// CompletionCharacter is one of CompletionTriggerCharacters typed.
	CompletionCharacter
	// CompletionInvoked is the app's ShowCompletions or the command.
	CompletionInvoked
)

// CompletionRequest asks the app for completions at Position: Query is
// the word before the caret. The app answers with Ref.PresentCompletions
// and ID; an answer to an older request is dropped.
type CompletionRequest struct {
	ID        int
	Position  Position
	Query     string
	Trigger   CompletionTrigger
	Character string
}

// DefinitionTrigger is what asked for a definition.
type DefinitionTrigger uint8

const (
	// DefinitionHover is the pointer over a word with the command key
	// held: the app answers with Ref.PresentDefinitionLink.
	DefinitionHover DefinitionTrigger = iota
	// DefinitionPress is a press on a word with the command key held.
	DefinitionPress
	// DefinitionKey is F12 at the caret.
	DefinitionKey
)

// DefinitionRequest asks the app where the word at Position is defined.
type DefinitionRequest struct {
	ID       int
	Position Position
	Word     Range
	Trigger  DefinitionTrigger
}

// HoverEvent is the mouse resting over a character: its position, its
// word, the pointer in the root view and the diagnostics there.
type HoverEvent struct {
	Position    Position
	Word        Range
	At          core.Point
	Modifiers   core.Modifiers
	Diagnostics []Diagnostic
}

// ContextMenuEvent asks for a menu: at Position in the text and At in the
// root view; after a long press or a double tap on a touch screen, with
// Selection the selection's box in the root view to open an edit menu
// over.
type ContextMenuEvent struct {
	Position  Position
	At        core.Point
	Selection core.Bounds
}

// Command is what Ref.Run runs, as its keys do.
type Command string

// The commands.
const (
	MoveLeft, MoveRight, MoveUp, MoveDown              Command = "moveLeft", "moveRight", "moveUp", "moveDown"
	MoveWordLeft, MoveWordRight                        Command = "moveWordLeft", "moveWordRight"
	MoveSubwordLeft, MoveSubwordRight                  Command = "moveSubwordLeft", "moveSubwordRight"
	MoveLineStart, MoveLineEnd                         Command = "moveLineStart", "moveLineEnd"
	MovePageUp, MovePageDown                           Command = "movePageUp", "movePageDown"
	MoveDocumentStart, MoveDocumentEnd                 Command = "moveDocumentStart", "moveDocumentEnd"
	SelectLeft, SelectRight, SelectUp, SelectDown      Command = "selectLeft", "selectRight", "selectUp", "selectDown"
	SelectWordLeft, SelectWordRight                    Command = "selectWordLeft", "selectWordRight"
	SelectSubwordLeft, SelectSubwordRight              Command = "selectSubwordLeft", "selectSubwordRight"
	SelectLineStart, SelectLineEnd                     Command = "selectLineStart", "selectLineEnd"
	SelectPageUp, SelectPageDown                       Command = "selectPageUp", "selectPageDown"
	SelectDocumentStart, SelectDocumentEnd             Command = "selectDocumentStart", "selectDocumentEnd"
	MoveToMatchingBracket                              Command = "moveToMatchingBracket"
	SelectAll, SelectLine, SelectNextOccurrence        Command = "selectAll", "selectLine", "selectNextOccurrence"
	SelectAllOccurrences                               Command = "selectAllOccurrences"
	AddCaretAbove, AddCaretBelow, SingleSelection      Command = "addCaretAbove", "addCaretBelow", "singleSelection"
	DeleteBackward, DeleteForward                      Command = "deleteBackward", "deleteForward"
	DeleteWordLeft, DeleteWordRight                    Command = "deleteWordLeft", "deleteWordRight"
	DeleteToLineStart, DeleteToLineEnd, DeleteLine     Command = "deleteToLineStart", "deleteToLineEnd", "deleteLine"
	Newline, NewlineBelow, NewlineAbove                Command = "newline", "newlineBelow", "newlineAbove"
	Indent, Outdent, ToggleComment, ToggleBlockComment Command = "indent", "outdent", "toggleComment", "toggleBlockComment"
	SortLinesAscending, SortLinesDescending            Command = "sortLinesAscending", "sortLinesDescending"
	TransformToUppercase, TransformToLowercase         Command = "transformToUppercase", "transformToLowercase"
	TrimTrailingWhitespace                             Command = "trimTrailingWhitespace"
	MoveLinesUp, MoveLinesDown                         Command = "moveLinesUp", "moveLinesDown"
	DuplicateLinesUp, DuplicateLinesDown               Command = "duplicateLinesUp", "duplicateLinesDown"
	JoinLines, TransposeCharacters                     Command = "joinLines", "transposeCharacters"
	Undo, Redo                                         Command = "undo", "redo"
	Fold, Unfold, FoldAll, UnfoldAll                   Command = "fold", "unfold", "foldAll", "unfoldAll"
	FindNext, FindPrevious, FindWithSelection          Command = "findNext", "findPrevious", "findWithSelection"
	Copy, Cut                                          Command = "copy", "cut"
	ShowCompletions, GoToDefinition, ShowCodeActions   Command = "showCompletions", "goToDefinition", "showCodeActions"
)

// Styles are the styles of the editor's parts, its whole look. A state's
// part merges over the part it follows.
type Styles struct {
	// Gutter is the column of line numbers and fold markers; LineNumber
	// every number, ActiveLineNumber those of lines with a caret;
	// FoldMarker a fold's marker, ⌄ or › folded.
	Gutter, LineNumber, ActiveLineNumber, FoldMarker core.Style
	// Content is the text area beside the gutter; Line every row;
	// ActiveLine behind the rows of lines with a caret.
	Content, Line, ActiveLine core.Style
	// Selection is behind selected text; Caret each caret, 1 point wide
	// unless it gives a width; Composition the text an input method
	// composes.
	Selection, Caret, Composition core.Style
	// SearchMatch and CurrentSearchMatch are behind the matches of Search
	// and the one the primary selection is on.
	SearchMatch, CurrentSearchMatch core.Style
	// MatchingBracket is the boxes of the bracket at the caret and its
	// partner; IndentGuide the line of each indentation level;
	// FoldPlaceholder what a folded line shows after its text.
	MatchingBracket, IndentGuide, FoldPlaceholder core.Style
	// Placeholder is the placeholder's text.
	Placeholder core.Style
	// The text of each severity's diagnostics, and of tagged ones.
	DiagnosticError, DiagnosticWarning, DiagnosticInfo, DiagnosticHint core.Style
	UnnecessaryText, DeprecatedText                                    core.Style
	// SelectionHandle is the knobs at a touch selection's ends.
	SelectionHandle core.Style
	// The completion list's box, items, selected item, the letters the
	// query matched, an item's detail and kind, and the documentation.
	CompletionMenu, CompletionItem, SelectedCompletionItem core.Style
	CompletionMatch, CompletionDetail, CompletionKind      core.Style
	CompletionDocumentation                                core.Style
	// DefinitionLink is the line under a definition link's text.
	DefinitionLink core.Style
}

// Editor is a code editor ([package editor]). The text is the app's with
// Value and OnChange, or its own from DefaultValue.
type Editor struct {
	Value        func() string
	DefaultValue string
	OnChange     func(text string)
	// Language is a language's name, alias, extension or file name ("go",
	// "rs", "main.ts", "Dockerfile"): its comments, brackets, indentation
	// and colors. Plain text without one.
	Language    string
	Placeholder string
	ReadOnly    bool
	AutoFocus   bool
	// NoLineNumbers hides the numbers; SoftWrap wraps long lines at the
	// editor's width.
	NoLineNumbers, SoftWrap bool
	// TabSize is the columns of a tab and of an indentation level, and
	// HardTabs indents with tabs; the language's when 0 and false.
	TabSize  int
	HardTabs bool
	// NoFolding removes the fold markers and folds; FoldRanges replaces
	// the folds by indentation with the app's.
	NoFolding  bool
	FoldRanges func() []FoldRange
	// NoHighlighting drops the built-in highlighting; NoAutoClose and
	// NoAutoIndent the closing of brackets and the indentation of new
	// lines; NoActiveLine and NoIndentGuides those parts.
	NoHighlighting, NoAutoClose, NoAutoIndent bool
	NoActiveLine, NoIndentGuides              bool
	// FontFamily (the monospaced font when empty), FontSize (13) and
	// LineHeight (20): the editor measures its text in them.
	FontFamily           string
	FontSize, LineHeight core.Points
	Search               func() Search
	Diagnostics          func() []Diagnostic
	Decorations          func() []Decoration
	// CompletionTriggerCharacters ask for completions as they are typed
	// ("." for members); a word character at a word's end does too while
	// OnCompletionRequest is set.
	CompletionTriggerCharacters string
	Styles                      Styles
	Label                       string
	Style                       core.Style
	StyleFunc                   func() core.Style
	Ref                         *Ref
	TestID                      string

	OnSelectionChange   func([]Selection)
	OnSearchChange      func(SearchResult)
	OnFoldChange        func([]FoldRange)
	OnFocus, OnBlur     func()
	OnCompletionRequest func(CompletionRequest)
	OnCompletionAccept  func(CompletionItem)
	OnDefinitionRequest func(DefinitionRequest)
	// OnCodeActionRequest receives the primary selection, for the app's
	// menu of a language server's code actions.
	OnCodeActionRequest func(Range)
	OnHover             func(HoverEvent)
	OnHoverEnd          func()
	OnContextMenu       func(ContextMenuEvent)
	OnLineNumberPress   func(line int)
	OnFindWithSelection func(text string)
}

// Ref is an editor's methods, bound once it mounts.
type Ref struct{ e *ed }
