package editor

import "sort"

// foldRange is lines start to end: a fold shows start and hides the rest.
type foldRange struct{ start, end int }

// indentFolds are the folds by indentation, as VS Code's: a line followed
// by lines indented deeper starts a fold hiding them, blank lines inside
// included, and the line that ends the block at the start's indentation
// (a closing bracket) stays visible.
func indentFolds(r *rope, tab int) []foldRange {
	n := r.lines()
	indent := make([]int, n)
	for i := range n {
		indent[i] = indentOf(r.line(i), tab)
	}
	var out []foldRange
	// open holds the lines whose fold is still open, by indentation.
	type opened struct{ line, indent int }
	var open []opened
	last := -1 // the last non-blank line
	closeTo := func(level int) {
		for len(open) > 0 && open[len(open)-1].indent >= level {
			o := open[len(open)-1]
			open = open[:len(open)-1]
			if last > o.line {
				out = append(out, foldRange{o.line, last})
			}
		}
	}
	for i := range n {
		if indent[i] < 0 {
			continue
		}
		closeTo(indent[i])
		// A line deeper than the last one opens a fold at the last one.
		if last >= 0 && indent[i] > indent[last] && (len(open) == 0 || open[len(open)-1].line != last) {
			open = append(open, opened{last, indent[last]})
		}
		last = i
	}
	closeTo(-1)
	sort.Slice(out, func(i, j int) bool { return out[i].start < out[j].start })
	return out
}

// indentOf is text's indentation in columns, -1 for a blank line.
func indentOf(text string, tab int) int {
	col := 0
	for _, c := range text {
		switch c {
		case ' ':
			col++
		case '\t':
			col += tab - col%tab
		default:
			return col
		}
	}
	return -1
}

// outermost is the folds of all not inside another, sorted.
func outermost(all []foldRange) []foldRange {
	sorted := append([]foldRange(nil), all...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].start != sorted[j].start {
			return sorted[i].start < sorted[j].start
		}
		return sorted[i].end > sorted[j].end
	})
	var out []foldRange
	for _, f := range sorted {
		if len(out) > 0 && f.start <= out[len(out)-1].end {
			continue
		}
		out = append(out, f)
	}
	return out
}

// carryFold moves f through changes: lines added or removed inside it
// grow or shrink it; one whose start line changed is dropped.
func carryFold(f foldRange, changes []change) (foldRange, bool) {
	for _, c := range changes {
		added := c.newEnd.line - c.oldEnd.line
		switch {
		case c.oldEnd.line < f.start:
			f.start += added
			f.end += added
		case c.from.line > f.end:
		case c.from.line == f.start && c.oldEnd.line == f.start && added == 0:
		case c.from.line >= f.start && c.oldEnd.line <= f.end:
			f.end += added
		default:
			return f, false
		}
		if f.end <= f.start {
			return f, false
		}
	}
	return f, true
}
