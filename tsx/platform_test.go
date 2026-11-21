package tsx

import (
	"regexp"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

// TestMenuRolesMatchTheTypes holds the menu roles TSX reads to the MenuRole type desktop.d.ts declares, and to every role core has.
func TestMenuRolesMatchTheTypes(t *testing.T) {
	src, err := typesFS.ReadFile("types/desktop.d.ts")
	if err != nil {
		t.Fatal(err)
	}
	decl := regexp.MustCompile(`export type MenuRole = ([^;]+);`).FindSubmatch(src)
	if decl == nil {
		t.Fatal("desktop.d.ts declares no MenuRole")
	}
	var typed []string
	for _, m := range regexp.MustCompile(`"(\w+)"`).FindAllSubmatch(decl[1], -1) {
		typed = append(typed, string(m[1]))
	}
	var read []string
	roles := map[core.MenuRole]bool{}
	for name, r := range menuRoles {
		read = append(read, name)
		roles[r] = true
	}
	slices.Sort(typed)
	slices.Sort(read)
	if !slices.Equal(typed, read) {
		t.Errorf("desktop.d.ts has roles %q, TSX reads %q", typed, read)
	}
	for r := core.MenuAbout; r.Label() != ""; r++ {
		if !roles[r] {
			t.Errorf("TSX has no name for the role %q", r.Label())
		}
	}
}
