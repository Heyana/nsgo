package android

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// Run the shipped reader on the JVM with Android's context and log calls stubbed. A 32 MiB heap makes trusting a corrupt length fail loudly; a real child process verifies that a bad pipe is stopped and reported.
func TestAndroidPipeRejectsCorruptFrames(t *testing.T) {
	runJavaEngineCheck(t, "PipeCheck")
}

func TestAndroidProcessReportsExit(t *testing.T) {
	runJavaEngineCheck(t, "ProcessExitCheck")
}

func TestJavaAudioPhasesMatchDriver(t *testing.T) {
	testhost.RequireSource(t)
	data, err := os.ReadFile("java/dev/oneo/nsgo/Audio.java")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]driver.AudioPhase{
		"LOADED": driver.AudioLoaded, "OPENED": driver.AudioOpened,
		"PLAYING": driver.AudioPlaying, "PAUSED": driver.AudioPaused,
		"PROGRESS": driver.AudioProgress, "BUFFER": driver.AudioBuffer,
		"ENDED": driver.AudioEnded, "FAILURE": driver.AudioFailure,
		"PLAYBACK_ENDED": driver.AudioPlaybackEnded,
	} {
		match := regexp.MustCompile(`\b` + name + ` = (\d+)`).FindSubmatch(data)
		if match == nil {
			t.Errorf("Audio.java has no %s phase", name)
			continue
		}
		got, _ := strconv.Atoi(string(match[1]))
		if driver.AudioPhase(got) != want {
			t.Errorf("Audio.java %s = %d, driver = %d", name, got, want)
		}
	}
}

func runJavaEngineCheck(t *testing.T, check string) {
	t.Helper()
	javac, java := androidJDK(t)
	classes := t.TempDir()
	args := []string{"-d", classes, "java/dev/oneo/nsgo/Protocol.java", "java/dev/oneo/nsgo/NsgoEngine.java"}
	if err := filepath.WalkDir("testdata/pipe", func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".java") {
			args = append(args, path)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.CommandContext(t.Context(), javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-Xmx32m", "-cp", classes, "dev.oneo.nsgo."+check).CombinedOutput(); err != nil {
		t.Fatalf("java: %v\n%s", err, out)
	}
}

// TestJavaReaderTakesWhatGoWrites holds Protocol.readMessage's bounds to android.go's: every Go-to-Java kind passes its kind check, and the largest payload Go's writer lets through is the largest Java reads, so a big image or font fails as its own error rather than ending the process as a corrupt pipe.
func TestJavaReaderTakesWhatGoWrites(t *testing.T) {
	testhost.RequireSource(t)
	read := func(path ...string) string {
		data, err := os.ReadFile(filepath.Join(path...))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	goSource, java := read("android.go"), read("java", "dev", "oneo", "nsgo", "Protocol.java")

	// The kinds Go writes, from the const block's Go-to-Java part.
	start, end := strings.Index(goSource, "// Go to Java."), strings.Index(goSource, "// Java to Go.")
	if start < 0 || end < start {
		t.Fatal("android.go has no Go to Java message kinds")
	}
	lo, hi := -1, -1
	for _, m := range regexp.MustCompile(`\bmsg\w+ += (\d+)`).FindAllStringSubmatch(goSource[start:end], -1) {
		n, _ := strconv.Atoi(m[1])
		if lo < 0 || n < lo {
			lo = n
		}
		hi = max(hi, n)
	}
	bounds := regexp.MustCompile(`kind < (\w+) \|\| kind > (\w+)`).FindStringSubmatch(java)
	if bounds == nil {
		t.Fatal("Protocol.readMessage checks no kind range")
	}
	value := func(name string) int {
		m := regexp.MustCompile(`static final int ` + name + ` = (\d+);`).FindStringSubmatch(java)
		if m == nil {
			t.Fatalf("Protocol.java has no %s", name)
		}
		n, _ := strconv.Atoi(m[1])
		return n
	}
	if jlo, jhi := value(bounds[1]), value(bounds[2]); jlo != lo || jhi != hi {
		t.Errorf("Protocol.readMessage takes kinds %d to %d (%s to %s), android.go writes %d to %d", jlo, jhi, bounds[1], bounds[2], lo, hi)
	}

	size := regexp.MustCompile(`static final int MAX_MESSAGE_SIZE = ([\d *]+);`).FindStringSubmatch(java)
	if size == nil {
		t.Fatal("Protocol.java has no MAX_MESSAGE_SIZE")
	}
	javaMax := 1
	for _, f := range strings.Split(size[1], "*") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			t.Fatalf("MAX_MESSAGE_SIZE %q: %v", size[1], err)
		}
		javaMax *= n
	}
	if javaMax != maxMessage {
		t.Errorf("Protocol.MAX_MESSAGE_SIZE is %d, android's maxMessage %d", javaMax, maxMessage)
	}
	if err := checkMessage(6, maxMessage); err != nil {
		t.Errorf("a payload of the most Java reads: %v", err)
	}
	if err := checkMessage(6, maxMessage+1); err == nil || !strings.Contains(err.Error(), "64 MiB") {
		t.Errorf("a payload over the most Java reads: %v", err)
	}
}

// TestJavaExtendedKindsMatchGo holds Protocol.java's EXT_* constants, the kinds of msgExtended messages, to android.go's ext* constants, by name and number, and checks that Protocol.isExtended takes each of them, so that a kind renumbered on one side only, as a rebase does to kinds assigned in order, fails here rather than being read as another message.
func TestJavaExtendedKindsMatchGo(t *testing.T) {
	testhost.RequireSource(t)
	read := func(path ...string) string {
		data, err := os.ReadFile(filepath.Join(path...))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	goSource, java := read("android.go"), read("java", "dev", "oneo", "nsgo", "Protocol.java")

	goKinds, err := goExtendedKinds(goSource)
	if err != nil {
		t.Fatal(err)
	}
	javaKinds := map[string]int{}
	for _, m := range regexp.MustCompile(`static final int EXT_(\w+) = (\d+);`).FindAllStringSubmatch(java, -1) {
		n, _ := strconv.Atoi(m[2])
		javaKinds[m[1]] = n
	}
	accepts := regexp.MustCompile(`(?s)static boolean isExtended\(int kind\) \{(.*?)\n    \}`).FindStringSubmatch(java)
	if accepts == nil {
		t.Fatal("Protocol.java has no isExtended")
	}

	seen := map[int]string{}
	for name, n := range goKinds {
		if n < 1 || n > 255 {
			t.Errorf("android.go's extended kind %s is %d, outside 1 to 255", name, n)
		}
		if other, ok := seen[n]; ok {
			t.Errorf("android.go's extended kinds %s and %s are both %d", other, name, n)
		}
		seen[n] = name
		if got, ok := javaKinds[name]; !ok {
			t.Errorf("Protocol.java has no EXT_%s for android.go's extended kind %d", name, n)
		} else if got != n {
			t.Errorf("Protocol.java's EXT_%s is %d, android.go's %d", name, got, n)
		}
	}
	for name, n := range javaKinds {
		if _, ok := goKinds[name]; !ok {
			t.Errorf("Protocol.java's EXT_%s (%d) has no extended kind in android.go", name, n)
		}
		if !strings.Contains(accepts[1], "case EXT_"+name+":") {
			t.Errorf("Protocol.isExtended does not take EXT_%s", name)
		}
	}

	if got := javaConstantName("FrameRate"); got != "FRAME_RATE" {
		t.Errorf("javaConstantName(FrameRate) = %s", got)
	}
	if got := javaConstantName("DNSServers"); got != "DNS_SERVERS" {
		t.Errorf("javaConstantName(DNSServers) = %s", got)
	}
}

// TestGoExtendedKindsReadsTheWholeBlock checks goExtendedKinds on comments a pattern match would stop at, such as a parenthesis inside the block, and on the constants around it, which are not extended kinds.
func TestGoExtendedKindsReadsTheWholeBlock(t *testing.T) {
	src := `package android

const msgOther = 3

// Extended Go-to-Java message kinds, the sub-kind byte (see msgExtended).
const (
	// extFrameRate sets the display's frame rate (see Display.setFrameRate).
	extFrameRate = 1
	// extDNSServers asks for the DNS servers (a list).
	extDNSServers = 0x2
)

// The parts of an image (placeholder or source).
const (
	extNotAKind = 9
)
`
	got, err := goExtendedKinds(src)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]int{"FRAME_RATE": 1, "DNS_SERVERS": 2}; !maps.Equal(got, want) {
		t.Errorf("goExtendedKinds = %v, want %v", got, want)
	}
	if _, err := goExtendedKinds("package android\n\nconst extFoo = 1\n"); err == nil {
		t.Error("goExtendedKinds found a block in a source without one")
	}
	if _, err := goExtendedKinds("package android\n\n// Extended Go-to-Java message kinds.\nconst (\n\textFoo = 1 << 2\n)\n"); err == nil {
		t.Error("goExtendedKinds took a kind that is not a number")
	}
}

// goExtendedKinds parses android.go's source and returns the ext* constants of the const declaration documented as the extended Go-to-Java message kinds, by Protocol.java's names for them (EXT_ less): parsing rather than matching text, so a comment in the block, parentheses and all, cannot cut it short.
func goExtendedKinds(src string) (map[string]int, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "android.go", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST || gen.Doc == nil || !strings.HasPrefix(gen.Doc.Text(), "Extended Go-to-Java message kinds") {
			continue
		}
		kinds := map[string]int{}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "ext") {
					continue
				}
				var lit ast.Expr
				if i < len(vs.Values) {
					lit = vs.Values[i]
				}
				b, ok := lit.(*ast.BasicLit)
				if !ok || b.Kind != token.INT {
					return nil, fmt.Errorf("android.go's extended kind %s is not a number", name.Name)
				}
				n, err := strconv.ParseInt(b.Value, 0, 64)
				if err != nil {
					return nil, fmt.Errorf("android.go's extended kind %s: %v", name.Name, err)
				}
				kinds[javaConstantName(strings.TrimPrefix(name.Name, "ext"))] = int(n)
			}
		}
		return kinds, nil
	}
	return nil, errors.New("android.go has no extended message kinds")
}

// javaConstantName turns a Go name's camel case into Protocol.java's upper snake case: FrameRate is FRAME_RATE and DNSServers DNS_SERVERS.
func javaConstantName(name string) string {
	var b strings.Builder
	for i, r := range name {
		upper := r >= 'A' && r <= 'Z'
		if i > 0 && upper {
			prev := rune(name[i-1])
			next := i+1 < len(name) && name[i+1] >= 'a' && name[i+1] <= 'z'
			if prev >= 'a' && prev <= 'z' || prev >= '0' && prev <= '9' || prev >= 'A' && prev <= 'Z' && next {
				b.WriteByte('_')
			}
		}
		b.WriteRune(r)
	}
	return strings.ToUpper(b.String())
}

// TestEnvironmentSaysTheRuntimeReadsExtendedMessages holds the environment's third capability byte to what sendExtended waits for: readEnvironmentCapabilities returns the byte a runtime writes, none for one built before it, Protocol.java's CAP_EXTENDED_MESSAGES is android.go's capExtendedMessages, and NsgoView.environment sets it.
func TestEnvironmentSaysTheRuntimeReadsExtendedMessages(t *testing.T) {
	testhost.RequireSource(t)
	for _, c := range []struct {
		third []byte
		want  byte
	}{{nil, 0}, {[]byte{0}, 0}, {[]byte{capExtendedMessages}, capExtendedMessages}, {[]byte{255}, 255}} {
		var w writer
		writeBaseEnvironment(&w, "en-US")
		w.byte(1)
		w.str("h23")
		w.str("metric")
		w.str("celsius")
		w.str("US")
		w.buf = append(w.buf, c.third...)
		r := reader{w.buf}
		if _, third := readEnvironmentCapabilities(&r); third != c.want || len(r.buf) != 0 {
			t.Errorf("third byte %v: read %d with %d bytes left, want %d", c.third, third, len(r.buf), c.want)
		}
	}
	// Discovery's requests are extended messages, so a runtime offers it only with both bits.
	for _, c := range []struct {
		third byte
		want  bool
	}{{0, false}, {capDiscoverServices, false}, {capExtendedMessages, false}, {capExtendedMessages | capDiscoverServices, true}} {
		var w writer
		writeBaseEnvironment(&w, "en-US")
		w.byte(1)
		w.str("h23")
		w.str("metric")
		w.str("celsius")
		w.str("US")
		w.byte(c.third)
		if env, _ := readEnvironmentCapabilities(&reader{w.buf}); env.DiscoverServices != c.want {
			t.Errorf("third byte %d: DiscoverServices = %v, want %v", c.third, env.DiscoverServices, c.want)
		}
	}

	read := func(path ...string) string {
		data, err := os.ReadFile(filepath.Join(path...))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	java, view := read("java", "dev", "oneo", "nsgo", "Protocol.java"), read("java", "dev", "oneo", "nsgo", "NsgoView.java")
	if m := regexp.MustCompile(`static final int CAP_EXTENDED_MESSAGES = (\d+);`).FindStringSubmatch(java); m == nil || m[1] != strconv.Itoa(capExtendedMessages) {
		t.Errorf("Protocol.java's CAP_EXTENDED_MESSAGES is %v, android.go's capExtendedMessages %d", m, capExtendedMessages)
	}
	if m := regexp.MustCompile(`int third = ([^;]*);`).FindStringSubmatch(view); m == nil || !strings.Contains(m[1], "Protocol.CAP_EXTENDED_MESSAGES") {
		t.Errorf("NsgoView.environment's third capability byte is %v, without Protocol.CAP_EXTENDED_MESSAGES", m)
	}
	if m := regexp.MustCompile(`static final int CAP_DISCOVER_SERVICES = (\d+);`).FindStringSubmatch(java); m == nil || m[1] != strconv.Itoa(capDiscoverServices) {
		t.Errorf("Protocol.java's CAP_DISCOVER_SERVICES is %v, protocol.go's capDiscoverServices %d", m, capDiscoverServices)
	}
	if m := regexp.MustCompile(`int third = ([^;]*);`).FindStringSubmatch(view); m == nil || !strings.Contains(m[1], "Protocol.CAP_DISCOVER_SERVICES") {
		t.Errorf("NsgoView.environment's third capability byte is %v, without Protocol.CAP_DISCOVER_SERVICES", m)
	}
	if m := regexp.MustCompile(`static final int CAP_FETCH_TO_FILE = (\d+);`).FindStringSubmatch(java); m == nil || m[1] != strconv.Itoa(capFetchToFile) {
		t.Errorf("Protocol.java's CAP_FETCH_TO_FILE is %v, protocol.go's capFetchToFile %d", m, capFetchToFile)
	}
	if m := regexp.MustCompile(`int third = ([^;]*);`).FindStringSubmatch(view); m == nil || !strings.Contains(m[1], "Protocol.CAP_FETCH_TO_FILE") {
		t.Errorf("NsgoView.environment's third capability byte is %v, without Protocol.CAP_FETCH_TO_FILE", m)
	}
	if m := regexp.MustCompile(`static final int CAP_HAPTIC_PATTERNS = (\d+);`).FindStringSubmatch(java); m == nil || m[1] != strconv.Itoa(capHapticPatterns) {
		t.Errorf("Protocol.java's CAP_HAPTIC_PATTERNS is %v, protocol.go's capHapticPatterns %d", m, capHapticPatterns)
	}
	if m := regexp.MustCompile(`int third = ([^;]*);`).FindStringSubmatch(view); m == nil || !strings.Contains(m[1], "Protocol.CAP_HAPTIC_PATTERNS") {
		t.Errorf("NsgoView.environment's third capability byte is %v, without Protocol.CAP_HAPTIC_PATTERNS", m)
	}
}
