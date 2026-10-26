package diff

import (
	"fmt"
	"strconv"
	"strings"
)

// FileStatus is what a patch did to a file.
type FileStatus uint8

const (
	FileModified FileStatus = iota
	FileAdded
	FileDeleted
	FileRenamed
	FileCopied
)

// filePatch is a file of a patch: its paths, status and hunks.
type filePatch struct {
	oldPath, path string
	status        FileStatus
	binary        bool
	hunks         []patchHunk
}

// patchHunk is a hunk of a patch: where its lines start on each side, its
// header (the text after the second @@) and its lines, each with its
// first character (' ', '-' or '+').
type patchHunk struct {
	oldStart, newStart int
	header             string
	lines              []string
	// noEOL marks the old (0) and new (1) sides' last lines as lacking a
	// line break.
	noEOL [2]bool
}

// parsePatch reads the files of a unified diff: git diff's and git
// format-patch's (renames, copies, new, deleted and binary files; text
// before the first file skipped) and diff -u's.
func parsePatch(text string) ([]filePatch, error) {
	var files []filePatch
	var f *filePatch
	var h *patchHunk
	oldLeft, newLeft := 0, 0
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	newFile := func() {
		files = append(files, filePatch{})
		f, h = &files[len(files)-1], nil
	}
	strip := func(p string) string {
		p = strings.TrimSpace(p)
		if i := strings.IndexByte(p, '\t'); i >= 0 {
			p = p[:i]
		}
		if p == "/dev/null" {
			return ""
		}
		if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
			return p[2:]
		}
		return p
	}
	for n, line := range lines {
		if h != nil && (oldLeft > 0 || newLeft > 0) {
			switch {
			case strings.HasPrefix(line, " ") || line == "":
				oldLeft--
				newLeft--
				h.lines = append(h.lines, " "+strings.TrimPrefix(line, " "))
				continue
			case strings.HasPrefix(line, "-"):
				oldLeft--
				h.lines = append(h.lines, line)
				continue
			case strings.HasPrefix(line, "+"):
				newLeft--
				h.lines = append(h.lines, line)
				continue
			}
		}
		switch {
		case strings.HasPrefix(line, `\ `):
			if h != nil && len(h.lines) > 0 {
				last := h.lines[len(h.lines)-1]
				switch last[0] {
				case '-':
					h.noEOL[0] = true
				case '+':
					h.noEOL[1] = true
				default:
					h.noEOL = [2]bool{true, true}
				}
			}
		case strings.HasPrefix(line, "diff --git "):
			newFile()
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				f.oldPath, f.path = strip(parts[2]), strip(parts[3])
			}
		case strings.HasPrefix(line, "--- "):
			if f == nil || len(f.hunks) > 0 {
				newFile()
			}
			if p := strip(line[4:]); p != "" {
				f.oldPath = p
			} else {
				f.status = FileAdded
			}
		case strings.HasPrefix(line, "+++ "):
			if f == nil {
				newFile()
			}
			if p := strip(line[4:]); p != "" {
				f.path = p
			} else {
				f.status, f.path = FileDeleted, f.oldPath
			}
		case f != nil && strings.HasPrefix(line, "new file mode"):
			f.status = FileAdded
		case f != nil && strings.HasPrefix(line, "deleted file mode"):
			f.status = FileDeleted
		case f != nil && strings.HasPrefix(line, "rename from "):
			f.status, f.oldPath = FileRenamed, line[len("rename from "):]
		case f != nil && strings.HasPrefix(line, "rename to "):
			f.path = line[len("rename to "):]
		case f != nil && strings.HasPrefix(line, "copy from "):
			f.status, f.oldPath = FileCopied, line[len("copy from "):]
		case f != nil && strings.HasPrefix(line, "copy to "):
			f.path = line[len("copy to "):]
		case f != nil && (strings.HasPrefix(line, "Binary files") || strings.HasPrefix(line, "GIT binary patch")):
			f.binary = true
		case strings.HasPrefix(line, "@@ "):
			if f == nil {
				return nil, fmt.Errorf("line %d: a hunk before any file", n+1)
			}
			end := strings.Index(line[3:], " @@")
			if end < 0 {
				return nil, fmt.Errorf("line %d: a hunk header without its closing @@", n+1)
			}
			ranges := strings.Fields(line[3 : 3+end])
			if len(ranges) != 2 {
				return nil, fmt.Errorf("line %d: a hunk header needs two ranges", n+1)
			}
			os, oc, err1 := hunkRange(ranges[0], '-')
			ns, nc, err2 := hunkRange(ranges[1], '+')
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("line %d: a malformed hunk range", n+1)
			}
			// The text after the second @@, a function's name; the ranges
			// without one.
			header := strings.TrimSpace(line[3+end+3:])
			if header == "" {
				header = line[:3+end+3]
			}
			f.hunks = append(f.hunks, patchHunk{oldStart: os, newStart: ns, header: header})
			h = &f.hunks[len(f.hunks)-1]
			oldLeft, newLeft = oc, nc
		}
	}
	for i := range files {
		if files[i].path == "" {
			files[i].path = files[i].oldPath
		}
		if files[i].oldPath == "" && files[i].status != FileAdded {
			files[i].oldPath = files[i].path
		}
	}
	return files, nil
}

// hunkRange reads "-start,count" or "+start".
func hunkRange(s string, sign byte) (int, int, error) {
	if s == "" || s[0] != sign {
		return 0, 0, fmt.Errorf("no %c", sign)
	}
	start, count, ok := strings.Cut(s[1:], ",")
	a, err := strconv.Atoi(start)
	if err != nil {
		return 0, 0, err
	}
	c := 1
	if ok {
		if c, err = strconv.Atoi(count); err != nil {
			return 0, 0, err
		}
	}
	return a, c, nil
}

// texts is what a view shows of a patched file: its hunks' lines on each
// side, and each line's number in the file.
func (f filePatch) texts() (old, new string, oldNums, newNums []int) {
	var o, n strings.Builder
	for _, h := range f.hunks {
		oi, ni := h.oldStart, h.newStart
		for k, l := range h.lines {
			text := l[1:]
			last := k == len(h.lines)-1
			switch l[0] {
			case '-':
				o.WriteString(text + eol(last && h.noEOL[0]))
				oldNums = append(oldNums, oi)
				oi++
			case '+':
				n.WriteString(text + eol(last && h.noEOL[1]))
				newNums = append(newNums, ni)
				ni++
			default:
				o.WriteString(text + "\n")
				n.WriteString(text + "\n")
				oldNums, newNums = append(oldNums, oi), append(newNums, ni)
				oi, ni = oi+1, ni+1
			}
		}
	}
	return o.String(), n.String(), oldNums, newNums
}

func eol(none bool) string {
	if none {
		return ""
	}
	return "\n"
}
