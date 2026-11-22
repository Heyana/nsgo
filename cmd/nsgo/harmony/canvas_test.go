package harmony_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var canvasCase = regexp.MustCompile(`(?m)^\s*case ([0-9]+):`)

// recorderOnlyKinds are the drawing kinds the recorder resolves itself, so players skip them.
var recorderOnlyKinds = map[driver.DrawKind]bool{driver.DrawTextAlign: true, driver.DrawTextBaseline: true}

// unplayedKinds are kinds canvasJSON sends that this player does not draw yet: HarmonyOS mesh playback arrives with the DrawVertices host change for #1211, which removes it from here.
var unplayedKinds = map[driver.DrawKind]bool{driver.DrawVertices: true}

// TestCanvasPlayerReadsEveryDrawKind checks that Index.ets's canvas player has a case for every drawing kind canvasJSON sends, under the driver's number, and none for a number the driver lacks. A kind the player has no case for is dropped silently, as DrawAtlas was before the host learned it.
func TestCanvasPlayerReadsEveryDrawKind(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	data, err := os.ReadFile(filepath.Join(root, "cmd", "nsgo", "harmony", "templates", "entry", "src", "main", "ets", "pages", "Index.ets"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	// Go sends an atlas as one op only to a host that reports it can play one, so a player with case 41 (DrawAtlas) must say so.
	if !strings.Contains(src, "CanvasAtlas: true") {
		t.Error("deviceJSON does not report CanvasAtlas, so Go keeps expanding atlases for this host")
	}
	start := strings.Index(src, "struct CanvasView {")
	if start < 0 {
		t.Fatal("Index.ets has no CanvasView: the pattern no longer matches")
	}
	body := src[start:]
	if end := strings.Index(body[1:], "\nstruct "); end >= 0 {
		body = body[:end+1]
	}
	read := map[driver.DrawKind]bool{}
	for _, m := range canvasCase.FindAllStringSubmatch(body, -1) {
		n, _ := strconv.Atoi(m[1])
		read[driver.DrawKind(n)] = true
	}
	known := map[driver.DrawKind]bool{}
	for k := driver.DrawSave; k.String() != "DrawKind("+strconv.Itoa(int(k))+")"; k++ {
		known[k] = true
		if !recorderOnlyKinds[k] && !unplayedKinds[k] && !read[k] {
			t.Errorf("the canvas player has no case for %s (%d)", k, k)
		}
	}
	if len(known) < int(driver.DrawAtlas) {
		t.Fatalf("found %d drawing kinds: the driver's names no longer end the loop", len(known))
	}
	for k := range read {
		if !known[k] || recorderOnlyKinds[k] {
			t.Errorf("the canvas player reads %d, which canvasJSON never sends", k)
		}
	}
}
