package android

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// TestAutomationProtocolMatchesJava checks Go/Java parity for Android automation: the pipe message types (msgTest/TEST, msgTestReply/TEST_REPLY), the intent extras in TestAgent.java and NsgoActivity.java, debuggable-only gating in NsgoView.java, every step op sent by testsurface.go, and the build-tag split keeping TestAgent/automation out of release builds.
func TestAutomationProtocolMatchesJava(t *testing.T) {
	testhost.RequireSource(t)
	read := func(path ...string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(path...))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	androidGo := read("android.go")
	testSurfaceGo := read("testsurface.go")
	testHostGo := read("testhost.go")
	automationGo := read("automation.go")
	automationOffGo := read("automation_off.go")

	protocolJava := read("java", "dev", "oneo", "nsgo", "Protocol.java")
	testAgentJava := read("java", "dev", "oneo", "nsgo", "TestAgent.java")
	activityJava := read("java", "dev", "oneo", "nsgo", "NsgoActivity.java")
	viewJava := read("java", "dev", "oneo", "nsgo", "NsgoView.java")

	// 1. Pipe message IDs for test/automation steps and replies match Protocol.java.
	for goName, javaName := range map[string]string{
		"msgTest":      "TEST",
		"msgTestReply": "TEST_REPLY",
	} {
		g := regexp.MustCompile(`\b` + goName + ` += (\d+)`).FindStringSubmatch(androidGo)
		j := regexp.MustCompile(`static final int ` + javaName + ` = (\d+);`).FindStringSubmatch(protocolJava)
		if g == nil || j == nil || g[1] != j[1] {
			t.Errorf("android.go's %s is %v, Protocol.java's %s %v", goName, g, javaName, j)
		}
	}

	// 2. TestAgent intent extra constants match the CLI and NsgoActivity wires them.
	for name, want := range map[string]string{
		"EXTRA":            "nsgo.test",
		"AUTOMATION_EXTRA": "nsgo.automation",
	} {
		m := regexp.MustCompile(`static final String ` + name + ` = "([^"]+)";`).FindStringSubmatch(testAgentJava)
		if m == nil || m[1] != want {
			t.Errorf("TestAgent.java's %s is %v, want %q", name, m, want)
		}
	}
	if !strings.Contains(activityJava, "view.setAutomation(intent.getBooleanExtra(TestAgent.AUTOMATION_EXTRA, false));") {
		t.Error("NsgoActivity.java does not pass TestAgent.AUTOMATION_EXTRA to view.setAutomation")
	}

	// 3. NsgoView gates both test and automation on FLAG_DEBUGGABLE and only creates TestAgent when gated on.
	if !strings.Contains(viewJava, "ApplicationInfo.FLAG_DEBUGGABLE") {
		t.Error("NsgoView.java does not check ApplicationInfo.FLAG_DEBUGGABLE")
	}
	if !strings.Contains(viewJava, "void setAutomation(boolean enabled)") || !strings.Contains(viewJava, "if (enabled && isDebuggable()) automation = true;") {
		t.Error("NsgoView.java's setAutomation must require isDebuggable()")
	}
	if !strings.Contains(viewJava, "if (encoded != null && isDebuggable()) test = encoded;") {
		t.Error("NsgoView.java's setTest must require isDebuggable()")
	}
	if !strings.Contains(viewJava, "if (test != null || automation) {\n            agent = new TestAgent(this);") {
		t.Error("NsgoView.java must only instantiate TestAgent when test != null || automation")
	}
	if count := strings.Count(viewJava, "new TestAgent("); count != 1 {
		t.Errorf("NsgoView.java instantiates TestAgent %d times, want 1 (inside the debuggable gate)", count)
	}

	// 4. Every op testsurface.go sends has a case in TestAgent.java.
	var ops []string
	for _, m := range regexp.MustCompile(`"op":\s*"([^"]+)"`).FindAllStringSubmatch(testSurfaceGo, -1) {
		if !slices.Contains(ops, m[1]) {
			ops = append(ops, m[1])
		}
	}
	if len(ops) < 10 {
		t.Fatalf("found only %d ops in testsurface.go: %v", len(ops), ops)
	}
	for _, op := range ops {
		if !strings.Contains(testAgentJava, `case "`+op+`":`) {
			t.Errorf("testsurface.go sends op %q, which TestAgent.java does not handle", op)
		}
	}

	// 5. Build tags keep automation and testSurface out of release builds; automation also builds for host development (nsgoandroidhost).
	for name, pair := range map[string][2]string{
		"testhost.go":       {testHostGo, "//go:build android && nsgotest\n"},
		"testsurface.go":    {testSurfaceGo, "//go:build (android || nsgoandroidhost) && (nsgotest || nsgoautomation)\n"},
		"automation.go":     {automationGo, "//go:build (android || nsgoandroidhost) && nsgoautomation\n"},
		"automation_off.go": {automationOffGo, "//go:build (android || nsgoandroidhost) && !nsgoautomation\n"},
	} {
		if !strings.HasPrefix(pair[0], pair[1]) {
			t.Errorf("%s build tag = %q, want prefix %q", name, strings.SplitN(pair[0], "\n", 2)[0], strings.TrimSpace(pair[1]))
		}
	}
}

// TestAutomationWireEncoding checks the msgTest request and msgTestReply payload layout shared between Go (testsurface.go) and Java (TestAgent.handle, NsgoView.testReply).
func TestAutomationWireEncoding(t *testing.T) {
	payload, err := json.Marshal(map[string]any{"op": "press", "x": 12.5, "y": 34.0, "hold": int64(0)})
	if err != nil {
		t.Fatal(err)
	}
	var req writer
	req.u32(7)
	req.str(string(payload))
	rr := reader{buf: req.buf}
	if id, got := rr.u32(), rr.str(); id != 7 || got != string(payload) || len(rr.buf) != 0 {
		t.Fatalf("msgTest round-trip = (%d, %q, %d left), want (7, %q, 0)", id, got, len(rr.buf), payload)
	}

	var rep writer
	rep.u32(7)
	rep.str("")
	rep.str("iVBORw0KGgo=")
	pr := reader{buf: rep.buf}
	if id, msg, data := pr.u32(), pr.str(), pr.str(); id != 7 || msg != "" || data != "iVBORw0KGgo=" || len(pr.buf) != 0 {
		t.Fatalf("msgTestReply round-trip = (%d, %q, %q, %d left)", id, msg, data, len(pr.buf))
	}
}
