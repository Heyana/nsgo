package main

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// diffUnifiedContext is the number of unchanged context lines included before
// and after each changed region in a unified diff hunk.
const diffUnifiedContext = 3

// diffFileKind describes how a file changed between old and new trees.
type diffFileKind int

const (
	diffModified diffFileKind = iota
	diffAdded
	diffDeleted
)

// diffLineOp is one line operation in a line-level diff.
type diffLineOp int

const (
	diffOpEqual diffLineOp = iota
	diffOpDelete
	diffOpInsert
)

// diffLine is a single line produced by diffmatchpatch over lines.
type diffLine struct {
	op         diffLineOp
	text       string // line text without its trailing '\n'
	hasNewline bool   // whether the line originally ended with '\n'
}

// diffHunk is one unified diff hunk with 1-based line ranges.
type diffHunk struct {
	oldStart, oldCount int
	newStart, newCount int
	lines              []diffLine
}

// fileDiff describes the change to a single file relative to the module root.
type fileDiff struct {
	Path       string // slash-separated path from module root
	Kind       diffFileKind
	Binary     bool
	OldSize    int
	NewSize    int
	Insertions int
	Deletions  int
	Hunks      []diffHunk
}

// normalizeLineEndings replaces CRLF with LF so checkouts with core.autocrlf
// do not appear edited.
func normalizeLineEndings(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

// isBinaryContent reports whether a file should be treated as binary based on
// its extension or bytes (NUL byte or invalid UTF-8).
func isBinaryContent(relPath string, data []byte) bool {
	if isKnownBinaryExt(relPath) {
		return true
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return true
	}
	return !utf8.Valid(data)
}

func isKnownBinaryExt(relPath string) bool {
	low := strings.ToLower(relPath)
	dot := strings.LastIndexByte(low, '.')
	if dot < 0 {
		return false
	}
	switch low[dot:] {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".avif", ".ico",
		".ttf", ".otf", ".woff2",
		".wav", ".mp3", ".ogg",
		".wasm", ".zip", ".pdf", ".bin":
		return true
	}
	return false
}

// diffLines computes line-level edits between oldText and newText using
// diffmatchpatch over lines.
func diffLines(oldText, newText string) []diffLine {
	if oldText == "" && newText == "" {
		return nil
	}
	dmp := diffmatchpatch.New()
	dmp.DiffTimeout = 0
	chars1, chars2, lineArray := dmp.DiffLinesToChars(oldText, newText)
	raw := dmp.DiffMain(chars1, chars2, false)
	diffs := dmp.DiffCharsToLines(raw, lineArray)

	var lines []diffLine
	for _, d := range diffs {
		var op diffLineOp
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			op = diffOpEqual
		case diffmatchpatch.DiffDelete:
			op = diffOpDelete
		case diffmatchpatch.DiffInsert:
			op = diffOpInsert
		}
		lines = append(lines, splitDiffTextLines(op, d.Text)...)
	}
	return lines
}

// splitDiffTextLines splits s into individual lines, preserving whether the
// final line ended with '\n'.
func splitDiffTextLines(op diffLineOp, s string) []diffLine {
	if s == "" {
		return nil
	}
	var out []diffLine
	for len(s) > 0 {
		if idx := strings.IndexByte(s, '\n'); idx >= 0 {
			out = append(out, diffLine{op: op, text: s[:idx], hasNewline: true})
			s = s[idx+1:]
		} else {
			out = append(out, diffLine{op: op, text: s, hasNewline: false})
			break
		}
	}
	return out
}

// groupHunks groups line edits into unified diff hunks with up to context lines
// of surrounding equal lines, merging hunks whose context windows overlap.
func groupHunks(lines []diffLine, context int) (hunks []diffHunk, insertions, deletions int) {
	// Track 1-based old and new line numbers before each element of lines.
	type span struct{ start, end int }
	var changeSpans []span
	for i := 0; i < len(lines); {
		if lines[i].op == diffOpEqual {
			i++
			continue
		}
		j := i
		for j < len(lines) && lines[j].op != diffOpEqual {
			if lines[j].op == diffOpInsert {
				insertions++
			} else {
				deletions++
			}
			j++
		}
		start := max(0, i-context)
		end := min(len(lines), j+context)
		if len(changeSpans) > 0 && start <= changeSpans[len(changeSpans)-1].end {
			changeSpans[len(changeSpans)-1].end = end
		} else {
			changeSpans = append(changeSpans, span{start: start, end: end})
		}
		i = j
	}
	if len(changeSpans) == 0 {
		return nil, 0, 0
	}

	// Precompute cumulative old and new line positions before each index.
	oldPos := make([]int, len(lines)+1)
	newPos := make([]int, len(lines)+1)
	for i, l := range lines {
		oldPos[i+1] = oldPos[i]
		newPos[i+1] = newPos[i]
		switch l.op {
		case diffOpEqual:
			oldPos[i+1]++
			newPos[i+1]++
		case diffOpDelete:
			oldPos[i+1]++
		case diffOpInsert:
			newPos[i+1]++
		}
	}

	hunks = make([]diffHunk, 0, len(changeSpans))
	for _, sp := range changeSpans {
		slice := lines[sp.start:sp.end]
		oldCount := oldPos[sp.end] - oldPos[sp.start]
		newCount := newPos[sp.end] - newPos[sp.start]
		oldStart := oldPos[sp.start] + 1
		if oldCount == 0 {
			oldStart = oldPos[sp.start]
		}
		newStart := newPos[sp.start] + 1
		if newCount == 0 {
			newStart = newPos[sp.start]
		}
		hunks = append(hunks, diffHunk{
			oldStart: oldStart,
			oldCount: oldCount,
			newStart: newStart,
			newCount: newCount,
			lines:    slice,
		})
	}
	return hunks, insertions, deletions
}

// buildTextFileDiff builds a fileDiff between oldNorm and newNorm (already LF
// normalized). Returns nil when there are no differences and kind is modified.
func buildTextFileDiff(modRelPath string, kind diffFileKind, oldNorm, newNorm []byte) *fileDiff {
	switch kind {
	case diffAdded:
		lines := splitDiffTextLines(diffOpInsert, string(newNorm))
		hunks, ins, del := groupHunks(lines, diffUnifiedContext)
		return &fileDiff{
			Path:       modRelPath,
			Kind:       diffAdded,
			OldSize:    0,
			NewSize:    len(newNorm),
			Insertions: ins,
			Deletions:  del,
			Hunks:      hunks,
		}
	case diffDeleted:
		lines := splitDiffTextLines(diffOpDelete, string(oldNorm))
		hunks, ins, del := groupHunks(lines, diffUnifiedContext)
		return &fileDiff{
			Path:       modRelPath,
			Kind:       diffDeleted,
			OldSize:    len(oldNorm),
			NewSize:    0,
			Insertions: ins,
			Deletions:  del,
			Hunks:      hunks,
		}
	default:
		lines := diffLines(string(oldNorm), string(newNorm))
		hunks, ins, del := groupHunks(lines, diffUnifiedContext)
		if len(hunks) == 0 {
			return nil
		}
		return &fileDiff{
			Path:       modRelPath,
			Kind:       diffModified,
			OldSize:    len(oldNorm),
			NewSize:    len(newNorm),
			Insertions: ins,
			Deletions:  del,
			Hunks:      hunks,
		}
	}
}

// formatUnifiedDiff renders diffs as a git-compatible unified diff with paths
// relative to the module root so git apply and git apply -R accept it.
func formatUnifiedDiff(diffs []fileDiff) string {
	var b strings.Builder
	for _, d := range diffs {
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", d.Path, d.Path)
		switch d.Kind {
		case diffAdded:
			b.WriteString("new file mode 100644\n")
		case diffDeleted:
			b.WriteString("deleted file mode 100644\n")
		}
		if d.Binary {
			switch d.Kind {
			case diffAdded:
				fmt.Fprintf(&b, "Binary files /dev/null and b/%s differ\n", d.Path)
			case diffDeleted:
				fmt.Fprintf(&b, "Binary files a/%s and /dev/null differ\n", d.Path)
			default:
				fmt.Fprintf(&b, "Binary files a/%s and b/%s differ\n", d.Path, d.Path)
			}
			continue
		}
		if len(d.Hunks) == 0 {
			continue
		}
		oldHeader := "a/" + d.Path
		newHeader := "b/" + d.Path
		if d.Kind == diffAdded {
			oldHeader = "/dev/null"
		}
		if d.Kind == diffDeleted {
			newHeader = "/dev/null"
		}
		fmt.Fprintf(&b, "--- %s\n", oldHeader)
		fmt.Fprintf(&b, "+++ %s\n", newHeader)
		for _, h := range d.Hunks {
			fmt.Fprintf(&b, "@@ -%s +%s @@\n", formatHunkRange(h.oldStart, h.oldCount), formatHunkRange(h.newStart, h.newCount))
			for _, l := range h.lines {
				switch l.op {
				case diffOpEqual:
					b.WriteByte(' ')
				case diffOpDelete:
					b.WriteByte('-')
				case diffOpInsert:
					b.WriteByte('+')
				}
				b.WriteString(l.text)
				b.WriteByte('\n')
				if !l.hasNewline {
					b.WriteString("\\ No newline at end of file\n")
				}
			}
		}
	}
	return b.String()
}

func formatHunkRange(start, count int) string {
	if count == 1 {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d,%d", start, count)
}

// formatDiffStat formats a git diff --stat style summary of changed lines per
// file and total insertions/deletions.
func formatDiffStat(diffs []fileDiff) string {
	if len(diffs) == 0 {
		return ""
	}
	maxPath := 0
	maxChanges := 0
	totalIns := 0
	totalDel := 0
	for _, d := range diffs {
		if len(d.Path) > maxPath {
			maxPath = len(d.Path)
		}
		if !d.Binary {
			changes := d.Insertions + d.Deletions
			if changes > maxChanges {
				maxChanges = changes
			}
			totalIns += d.Insertions
			totalDel += d.Deletions
		}
	}

	numWidth := len(fmt.Sprintf("%d", maxChanges))
	const maxBarWidth = 40

	var b strings.Builder
	for _, d := range diffs {
		if d.Binary {
			fmt.Fprintf(&b, " %-*s | Bin %d -> %d bytes\n", maxPath, d.Path, d.OldSize, d.NewSize)
			continue
		}
		changes := d.Insertions + d.Deletions
		plus, minus := scaleDiffBar(d.Insertions, d.Deletions, maxChanges, maxBarWidth)
		bar := strings.Repeat("+", plus) + strings.Repeat("-", minus)
		if bar != "" {
			fmt.Fprintf(&b, " %-*s | %*d %s\n", maxPath, d.Path, numWidth, changes, bar)
		} else {
			fmt.Fprintf(&b, " %-*s | %*d\n", maxPath, d.Path, numWidth, changes)
		}
	}

	fileWord := "files"
	if len(diffs) == 1 {
		fileWord = "file"
	}
	fmt.Fprintf(&b, " %d %s changed", len(diffs), fileWord)
	if totalIns > 0 {
		word := "insertions(+)"
		if totalIns == 1 {
			word = "insertion(+)"
		}
		fmt.Fprintf(&b, ", %d %s", totalIns, word)
	}
	if totalDel > 0 {
		word := "deletions(-)"
		if totalDel == 1 {
			word = "deletion(-)"
		}
		fmt.Fprintf(&b, ", %d %s", totalDel, word)
	}
	b.WriteByte('\n')
	return b.String()
}

func scaleDiffBar(ins, del, maxChanges, maxWidth int) (plus, minus int) {
	if maxChanges <= maxWidth || maxChanges == 0 {
		return ins, del
	}
	total := ins + del
	if total == 0 {
		return 0, 0
	}
	width := (total*maxWidth + maxChanges/2) / maxChanges
	if width == 0 {
		width = 1
	}
	if ins > 0 && del == 0 {
		return width, 0
	}
	if del > 0 && ins == 0 {
		return 0, width
	}
	plus = (ins*width + total/2) / total
	if plus == 0 {
		plus = 1
	}
	if plus >= width && width > 1 {
		plus = width - 1
	}
	minus = width - plus
	return plus, minus
}
