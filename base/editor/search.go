package editor

import (
	"regexp"
	"sort"
)

// maxMatches is the most matches a search finds.
const maxMatches = 10_000

// span is a range of the text.
type span struct{ from, to pos }

// compile is the expression of q: a plain query escaped, whole words
// bounded, case folded unless it matters.
func compile(q Search) (*regexp.Regexp, error) {
	expr := q.Query
	if !q.Regex {
		expr = regexp.QuoteMeta(expr)
	}
	if q.WholeWord {
		expr = `\b(?:` + expr + `)\b`
	}
	if !q.CaseSensitive {
		expr = `(?i)` + expr
	}
	return regexp.Compile(expr)
}

// findAll is the matches of re line by line, within scope when it is set.
func findAll(r *rope, re *regexp.Regexp, scope []span) []span {
	var out []span
	for line := range r.lines() {
		l := r.line(line)
		for _, m := range re.FindAllStringIndex(l, -1) {
			if m[0] == m[1] {
				continue
			}
			sp := span{pos{line, m[0]}, pos{line, m[1]}}
			if len(scope) > 0 && !inScope(sp, scope) {
				continue
			}
			out = append(out, sp)
			if len(out) >= maxMatches {
				return out
			}
		}
	}
	return out
}

func inScope(sp span, scope []span) bool {
	for _, s := range scope {
		if !sp.from.less(s.from) && !s.to.less(sp.to) {
			return true
		}
	}
	return false
}

// matchAfter is the index of the first match at or after p, wrapping to 0.
func matchAfter(matches []span, p pos) int {
	i := sort.Search(len(matches), func(i int) bool { return !matches[i].from.less(p) })
	if i == len(matches) {
		return 0
	}
	return i
}

// matchBefore is the index of the last match before p, wrapping to the
// last.
func matchBefore(matches []span, p pos) int {
	i := sort.Search(len(matches), func(i int) bool { return !matches[i].from.less(p) }) - 1
	if i < 0 {
		return len(matches) - 1
	}
	return i
}

// expand is a replacement for match m of re in its line: $1 and ${name}
// expanded for expressions.
func expand(re *regexp.Regexp, regex bool, line string, m span, with string) string {
	if !regex {
		return with
	}
	idx := re.FindStringSubmatchIndex(line[m.from.col:])
	if idx == nil {
		return with
	}
	return string(re.ExpandString(nil, with, line[m.from.col:], idx))
}
