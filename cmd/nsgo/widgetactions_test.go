package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestWidgetActionIntent executes the shipping intent, including a press received before Go installs its callback and the updates it must await. A real widget click remains a separate system-host acceptance check.
func TestWidgetActionIntent(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("needs Xcode and macOS App Intents")
	}
	if err := exec.Command("xcrun", "--find", "swiftc").Run(); err != nil {
		t.Skip("needs Xcode's swiftc")
	}
	dir := t.TempDir()
	source, err := widgetSources.ReadFile("widgets/Actions.swift")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Actions.swift"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	const harness = `import Foundation

enum Receipt {
    static let lock = NSLock()
    static var presses: [String] = []
    static var updated = false
}

@main struct Verify {
    static func main() async throws {
        let early = Task { try await NSGoActionIntent(activity: "", action: "widget").perform() }
        let deadline = Date().addingTimeInterval(3)
        while Presses.lock.withLock({ Presses.waiting.isEmpty }) && Date() < deadline {
            try await Task.sleep(nanoseconds: 1_000_000)
        }
        precondition(Presses.lock.withLock { Presses.waiting.count == 1 })
        precondition(Receipt.lock.withLock { Receipt.presses.isEmpty })
        nsgoActivityListen { event, id, action in
            precondition(event == 1)
            Receipt.lock.withLock {
                Receipt.presses.append(String(cString: id) + ":" + String(cString: action))
            }
            Presses.run {
                try? await Task.sleep(nanoseconds: 50_000_000)
                Receipt.lock.withLock { Receipt.updated = true }
            }
        }
        _ = try await early.value
        precondition(Receipt.lock.withLock { Receipt.presses == [":widget"] && Receipt.updated })
        _ = try await NSGoActionIntent(activity: "activity-id", action: "activity").perform()
        precondition(Receipt.lock.withLock { Receipt.presses == [":widget", "activity-id:activity"] })
        print("widget and activity intents reached the callback and awaited updates")
    }
}
`
	if err := os.WriteFile(filepath.Join(dir, "Verify.swift"), []byte(harness), 0o600); err != nil {
		t.Fatal(err)
	}
	program := filepath.Join(dir, "verify")
	cmd := exec.CommandContext(t.Context(), "xcrun", "--sdk", "macosx", "swiftc", "-parse-as-library", "-swift-version", "5", "-DNSGO_APP", filepath.Join(dir, "Actions.swift"), filepath.Join(dir, "Verify.swift"), "-o", program)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile native intent: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), program).CombinedOutput(); err != nil {
		t.Fatalf("run native intent: %v\n%s", err, out)
	}
}
