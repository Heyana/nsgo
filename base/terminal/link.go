package terminal

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/base/terminal/internal/vt"
	"github.com/nsgo-dev/nsgo/core"
)

// linkRows bounds how many rows a link is looked for above and below the
// pointer's, through a line wrapping over them.
const linkRows = 32

// urlPattern matches URLs printed as text, of the schemes Ghostty's does
// that Scope.OpenURL opens. A URL stops at spaces, quotes and the
// punctuation and box drawing of tables and CJK text around it.
var urlPattern = regexp.MustCompile("(?i)\\b(?:(?:https?|ftp|ssh|git|gemini|gopher|ipfs|ipns)://|mailto:|tel:|magnet:\\?|news:)" +
	"[^\\s<>\"'`\\\\^|\\x{2500}-\\x{259f}，。；：！？、（）《》「」『』“”‘’【】…]+")

// link is a link the viewport shows.
type link struct {
	url   string
	path  string // local file, opened through Scope.OpenFile
	spans []linkSpan
}

// linkSpan is a link's cells in a row of the viewport, x0 to x1 inclusive.
type linkSpan struct{ y, x0, x1 int }

// same reports whether l is o, in the same cells.
func (l link) same(o link) bool {
	return l.url == o.url && slices.Equal(l.spans, o.spans)
}

// cells returns the columns the link takes in row y of the viewport.
func (l link) cells(y int) (x0, x1 int, ok bool) {
	for _, s := range l.spans {
		if s.y == y {
			return s.x0, s.x1, true
		}
	}
	return 0, 0, false
}

// gridCell is a cell of the screen and its scrollback.
type gridCell struct{ x, y int }

// linkAt returns the link in a cell of the viewport: a program's hyperlink
// (OSC 8), or a URL printed as text, which may wrap beyond the viewport.
// Terminal.linkAt also supplies the directory for local filesystem links.
func linkAt(term *vt.Terminal, col, row int) link {
	return linkAtDirectory(term, col, row, "")
}

// linkAt is called with t.mu held. A remote directory is never treated as
// a local one; local programs fall back to their configured start directory.
func (t *Terminal) linkAt(col, row int) link {
	return linkAtDirectory(t.term, col, row, t.linkDirectory())
}

func (t *Terminal) linkDirectory() string {
	if pwd := t.term.Pwd(); pwd != "" {
		return localFilePath(pwd)
	}
	if t.opts.Conn != nil {
		return ""
	}
	dir := t.opts.Dir
	if dir == "" {
		var err error
		dir, err = os.UserHomeDir()
		if err != nil {
			return ""
		}
	}
	dir, _ = filepath.Abs(dir)
	return dir
}

func linkAtDirectory(term *vt.Terminal, col, row int, dir string) link {
	ref, ok := term.CellAt(col, row)
	if !ok {
		return link{}
	}
	if col > 0 && term.CellWide(ref) == vt.SpacerTail {
		col--
		ref, ok = term.CellAt(col, row)
		if !ok {
			return link{}
		}
	}
	y, ok := term.ScreenRow(&ref)
	if !ok {
		return link{}
	}
	text, cells := wrappedText(term, y)
	i := slices.Index(cells, gridCell{col, y})
	if i < 0 {
		return link{}
	}
	var uri, path string
	start, end := i, i+1
	if uri = term.Hyperlink(ref); uri != "" {
		same := func(c gridCell) bool {
			ref, ok := term.ScreenCellAt(c.x, c.y)
			return ok && term.Hyperlink(ref) == uri
		}
		for start > 0 && same(cells[start-1]) {
			start--
		}
		for end < len(cells) && same(cells[end]) {
			end++
		}
	} else if start, end, ok = findURL(text, i); ok {
		uri = text[start:end]
	}
	if uri == "" && dir != "" {
		start, end, path = findPath(text, i, dir)
	} else if strings.HasPrefix(strings.ToLower(uri), "file:") && dir != "" {
		path = existingPath(uri, dir)
	}
	if path != "" {
		name := filepath.ToSlash(path)
		if !strings.HasPrefix(name, "/") {
			name = "/" + name
		}
		uri = (&url.URL{Scheme: "file", Path: name}).String()
	} else if uri == "" || core.CheckURL(uri) != nil {
		return link{}
	}
	_, rows := term.Size()
	return link{url: uri, path: path, spans: linkSpans(term, cells[start:end], y-row, rows)}
}

// wrappedText returns the text of the line that screen row y is a row of,
// joining the rows it soft-wraps over, and the cell of each of its bytes.
func wrappedText(term *vt.Terminal, y int) (string, []gridCell) {
	first, last := y, y
	for first > 0 && y-first < linkRows && term.ScreenRowWraps(first-1) {
		first--
	}
	for last-y < linkRows && term.ScreenRowWraps(last) {
		last++
	}
	var b strings.Builder
	var cells []gridCell
	for row := first; row <= last; row++ {
		text, cols := term.ScreenRowText(row)
		for i, r := range text {
			n, _ := b.WriteRune(r)
			for range n {
				cells = append(cells, gridCell{cols[i], row})
			}
		}
	}
	return b.String(), cells
}

// findURL returns the bytes of the URL in text that byte i is in.
func findURL(text string, i int) (start, end int, ok bool) {
	for _, m := range urlPattern.FindAllStringIndex(text, -1) {
		if i < m[0] {
			break
		}
		if end := m[0] + len(trimURL(text[m[0]:m[1]])); i < end {
			return m[0], end, true
		}
	}
	return 0, 0, false
}

// trimURL drops what ends a sentence after a URL, and the closing brackets
// of what the URL is in: "(see http://a.b/c_(d))." keeps the one it opens.
func trimURL(s string) string {
	for s != "" {
		r, size := utf8.DecodeLastRuneInString(s)
		i := strings.IndexRune(")]}", r)
		switch {
		case strings.ContainsRune(".,;:!?*_~", r):
		case i >= 0 && strings.Count(s, string(r)) > strings.Count(s, string("([{"[i])):
		default:
			return s
		}
		s = s[:len(s)-size]
	}
	return s
}

// linkSpans returns the spans of cells in the viewport whose first row is
// screen row top.
func linkSpans(term *vt.Terminal, cells []gridCell, top, rows int) []linkSpan {
	var spans []linkSpan
	for _, c := range cells {
		y := c.y - top
		if y < 0 || y >= rows {
			continue
		}
		if n := len(spans); n > 0 && spans[n-1].y == y {
			spans[n-1].x0, spans[n-1].x1 = min(spans[n-1].x0, c.x), max(spans[n-1].x1, c.x)
			continue
		}
		spans = append(spans, linkSpan{y, c.x, c.x})
	}
	for i := range spans {
		s := &spans[i]
		if ref, ok := term.ScreenCellAt(s.x1, s.y+top); ok && term.CellWide(ref) == vt.WideChar {
			s.x1++
		}
	}
	return spans
}

// localFilePath decodes file URLs only when their authority names this machine.
// OSC 7 may also report a plain absolute path.
func localFilePath(raw string) string {
	if !strings.HasPrefix(strings.ToLower(raw), "file:") {
		if filepath.IsAbs(raw) {
			return filepath.Clean(raw)
		}
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return ""
	}
	hostname, _ := os.Hostname()
	if host := u.Host; host != "" && !strings.EqualFold(host, "localhost") && !strings.EqualFold(host, hostname) && host != "127.0.0.1" && host != "[::1]" {
		return ""
	}
	path := filepath.FromSlash(u.Path)
	if runtime.GOOS == "windows" && len(path) > 2 && path[0] == '\\' && path[2] == ':' {
		path = path[1:]
	}
	if !filepath.IsAbs(path) {
		return ""
	}
	return filepath.Clean(path)
}

// pathTokens keeps quoted filenames together; unquoted paths end at whitespace
// or the separators used around terminal output. The filesystem distinguishes
// actual filenames from the ordinary words printed beside them.
var pathTokens = regexp.MustCompile("'[^'\\r\\n]+'|\"[^\"\\r\\n]+\"|[^\\s<>\"'`|\\x{2500}-\\x{259f}]+")
var pathPosition = regexp.MustCompile(`:[0-9]+(?::[0-9]+)?$`)

func findPath(text string, at int, dir string) (start, end int, path string) {
	for _, m := range pathTokens.FindAllStringIndex(text, -1) {
		if at < m[0] {
			break
		}
		if at >= m[1] {
			continue
		}
		start, end = m[0], m[1]
		if text[start] == '"' || text[start] == '\'' {
			start++
			end--
		} else {
			for start < end && strings.ContainsRune("([{", rune(text[start])) {
				start++
			}
		}
		for end > start {
			if p := existingPath(text[start:end], dir); p != "" && at >= start && at < end {
				return start, end, p
			}
			r, n := utf8.DecodeLastRuneInString(text[start:end])
			if strings.ContainsRune(")]},.;:!?", r) {
				end -= n
				continue
			}
			if suffix := pathPosition.FindStringIndex(text[start:end]); suffix != nil {
				end = start + suffix[0]
				continue
			}
			break
		}
		break
	}
	return 0, 0, ""
}

func existingPath(raw, dir string) string {
	if raw == "" || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return ""
	}
	path := raw
	if strings.HasPrefix(strings.ToLower(raw), "file:") {
		path = localFilePath(raw)
		if path == "" {
			return ""
		}
	} else {
		// A URL that was not an openable URL is not a relative filename.
		if parsed, err := url.Parse(raw); err == nil && parsed.Scheme != "" && !(runtime.GOOS == "windows" && len(parsed.Scheme) == 1) {
			return ""
		}
		if raw == "~" || strings.HasPrefix(raw, "~/") || runtime.GOOS == "windows" && strings.HasPrefix(raw, `~\`) {
			home, err := os.UserHomeDir()
			if err != nil {
				return ""
			}
			path = filepath.Join(home, strings.TrimLeft(raw[1:], `/\`))
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() && !info.Mode().IsRegular() {
		return ""
	}
	return path
}
