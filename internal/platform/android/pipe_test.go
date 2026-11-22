package android

import (
	"io/fs"
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
	for name, want := range map[string]int{
		"ITEM_SAVE_PREFLIGHT_ANSWER": int(driver.ItemSavePreflightAnswer),
		"ITEM_SAVE_FILE_PREFLIGHT":   int(driver.ItemSaveFilePreflight),
		"EVENT_SAVE_PREFLIGHT":       int(driver.EventSavePreflight),
	} {
		if got := value(name); got != want {
			t.Errorf("Protocol.java %s = %d, driver = %d", name, got, want)
		}
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

	block := regexp.MustCompile(`(?s)// Extended Go-to-Java message kinds[^\n]*\nconst \((.*?)\)`).FindStringSubmatch(goSource)
	if block == nil {
		t.Fatal("android.go has no extended message kinds")
	}
	goKinds := map[string]int{}
	for _, m := range regexp.MustCompile(`\bext(\w+) += (\d+)`).FindAllStringSubmatch(block[1], -1) {
		n, _ := strconv.Atoi(m[2])
		goKinds[javaConstantName(m[1])] = n
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
