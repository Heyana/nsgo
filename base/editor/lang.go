package editor

import (
	"path"
	"strings"
)

// language is what the editor knows of a language beyond its colors:
// comment tokens, brackets, the pairs that close as they are typed, extra
// word characters, what indents and outdents the next line, and the
// default indentation.
type language struct {
	name     string
	aliases  []string
	exts     []string
	files    []string
	line     string    // line comment
	block    [2]string // block comment
	brackets [][2]string
	pairs    [][2]string
	word     string // extra word characters
	// indentAfter and outdent are line ends and starts (trimmed) that
	// indent the next line and outdent their own.
	indentAfter []string
	outdent     []string
	tabSize     int
	hardTabs    bool
	// lexer is chroma's name for it.
	lexer string
}

var cBrackets = [][2]string{{"(", ")"}, {"[", "]"}, {"{", "}"}}
var cPairs = [][2]string{{"(", ")"}, {"[", "]"}, {"{", "}"}, {`"`, `"`}, {"'", "'"}}

var languages = []language{
	{name: "go", aliases: []string{"golang"}, exts: []string{"go"}, line: "//", block: [2]string{"/*", "*/"},
		brackets: cBrackets, pairs: append(cPairs, [2]string{"`", "`"}), tabSize: 4, hardTabs: true, lexer: "go"},
	{name: "rust", aliases: []string{"rs"}, exts: []string{"rs"}, line: "//", block: [2]string{"/*", "*/"},
		brackets: cBrackets, pairs: [][2]string{{"(", ")"}, {"[", "]"}, {"{", "}"}, {`"`, `"`}}, tabSize: 4, lexer: "rust"},
	{name: "typescript", aliases: []string{"ts", "tsx", "javascript", "js", "jsx", "mjs"}, exts: []string{"ts", "tsx", "js", "jsx", "mjs", "cjs"},
		line: "//", block: [2]string{"/*", "*/"}, brackets: cBrackets, pairs: append(cPairs, [2]string{"`", "`"}), word: "$",
		tabSize: 2, lexer: "typescript"},
	{name: "python", aliases: []string{"py"}, exts: []string{"py"}, line: "#", brackets: cBrackets, pairs: cPairs,
		indentAfter: []string{":"}, tabSize: 4, lexer: "python"},
	{name: "c", aliases: []string{"cpp", "c++", "h"}, exts: []string{"c", "h", "cc", "cpp", "hpp"}, line: "//",
		block: [2]string{"/*", "*/"}, brackets: cBrackets, pairs: cPairs, tabSize: 4, lexer: "c++"},
	{name: "java", aliases: []string{"kotlin", "kt", "swift"}, exts: []string{"java", "kt", "swift"}, line: "//",
		block: [2]string{"/*", "*/"}, brackets: cBrackets, pairs: cPairs, tabSize: 4, lexer: "java"},
	{name: "json", exts: []string{"json"}, brackets: [][2]string{{"[", "]"}, {"{", "}"}},
		pairs: [][2]string{{"[", "]"}, {"{", "}"}, {`"`, `"`}}, tabSize: 2, lexer: "json"},
	{name: "yaml", aliases: []string{"yml"}, exts: []string{"yaml", "yml"}, line: "#", brackets: cBrackets, pairs: cPairs,
		indentAfter: []string{":"}, tabSize: 2, lexer: "yaml"},
	{name: "html", aliases: []string{"xml", "svg"}, exts: []string{"html", "htm", "xml", "svg"}, block: [2]string{"<!--", "-->"},
		brackets: [][2]string{{"<", ">"}}, pairs: [][2]string{{"<", ">"}, {`"`, `"`}}, tabSize: 2, lexer: "html"},
	{name: "css", aliases: []string{"scss"}, exts: []string{"css", "scss"}, block: [2]string{"/*", "*/"}, brackets: cBrackets,
		pairs: cPairs, word: "-", tabSize: 2, lexer: "css"},
	{name: "markdown", aliases: []string{"md"}, exts: []string{"md", "markdown"}, block: [2]string{"<!--", "-->"},
		brackets: cBrackets, pairs: [][2]string{{"(", ")"}, {"[", "]"}, {"`", "`"}}, tabSize: 2, lexer: "markdown"},
	{name: "shell", aliases: []string{"sh", "bash", "zsh"}, exts: []string{"sh", "bash", "zsh"}, line: "#", brackets: cBrackets,
		pairs: cPairs, word: "$", tabSize: 2, lexer: "bash"},
	{name: "ruby", aliases: []string{"rb"}, exts: []string{"rb"}, line: "#", brackets: cBrackets, pairs: cPairs,
		indentAfter: []string{" do", "do"}, outdent: []string{"end"}, tabSize: 2, lexer: "ruby"},
	{name: "dockerfile", files: []string{"dockerfile"}, line: "#", brackets: cBrackets, pairs: cPairs, tabSize: 4, lexer: "docker"},
}

// plainText is the language of text no language claims.
var plainText = language{name: "plaintext", brackets: cBrackets, pairs: cPairs, tabSize: 4}

// languageOf finds a language by name, alias, extension or file name,
// case-insensitive.
func languageOf(name string) *language {
	n := strings.ToLower(strings.TrimSpace(name))
	base := path.Base(n)
	ext := strings.TrimPrefix(path.Ext(base), ".")
	for i := range languages {
		l := &languages[i]
		if l.name == n || has(l.aliases, n) || has(l.exts, n) || has(l.files, base) || ext != "" && has(l.exts, ext) {
			return l
		}
	}
	return &plainText
}

func has(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
