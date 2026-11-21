package moduletest

import (
	"go/importer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// skillPackages are the packages the skill names by their last element.
var skillPackages = map[string]string{
	"nsgo": "", "core": "core", "base": "base", "ui": "ui", "mobile": "mobile", "desktop": "desktop",
	"gamepad": "gamepad", "gameservices": "gameservices", "nsgotest": "nsgotest", "tsx": "tsx", "compiler": "tsx/compiler", "updater": "updater", "cloud": "cloud",
}

var (
	skillName     = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	skillLink     = regexp.MustCompile(`\]\(([^)#\s]+)(#[^)]*)?\)`)
	skillIdent    = regexp.MustCompile(`\b(nsgo|core|base|ui|mobile|desktop|gamepad|gameservices|nsgotest|tsx|compiler|updater|cloud)\.([A-Z]\w*)(?:\.([A-Z]\w*))?`)
	skillMethod   = regexp.MustCompile("(?:`|\\b)(cx|app)\\.([A-Z]\\w*)\\(")
	skillPackage  = regexp.MustCompile("`((?:base|ui|tsx|icons)/[a-z]+)`")
	skillReceiver = map[string][2]string{"cx": {"core", "Scope"}, "app": {"nsgotest", "App"}}
)

// TestSkills checks what skills/ tells agents against the API it names, so
// a rename that leaves the skill behind fails here: each skill's front
// matter, its relative links, and the exported names, methods and packages
// it mentions.
func TestSkills(t *testing.T) {
	// skills/ names the module's files from its root.
	t.Chdir(moduleRoot)
	skills, err := filepath.Glob("skills/*/SKILL.md")
	if err != nil || len(skills) == 0 {
		t.Fatalf("no skills/*/SKILL.md: %v", err)
	}
	for _, file := range skills {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		name, description := skillFrontMatter(string(src))
		if dir := filepath.Base(filepath.Dir(file)); name != dir || !skillName.MatchString(name) || len(name) > 64 {
			t.Errorf("%s: name %q must be its directory's, %q, in lowercase words and hyphens", file, name, dir)
		}
		if description == "" || len(description) > 1024 {
			t.Errorf("%s: description must be 1 to 1024 characters, not %d", file, len(description))
		}
	}

	imp := importer.ForCompiler(token.NewFileSet(), "source", nil)
	pkgs := map[string]*types.Package{}
	lookup := func(name string) *types.Package {
		if p, ok := pkgs[name]; ok {
			return p
		}
		path := "github.com/nsgo-dev/nsgo"
		if sub := skillPackages[name]; sub != "" {
			path += "/" + sub
		}
		p, err := imp.Import(path)
		if err != nil {
			t.Fatalf("import %s: %v", path, err)
		}
		pkgs[name] = p
		return p
	}

	err = filepath.WalkDir("skills", func(file string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(file, ".md") {
			return err
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		src := string(data)
		for _, m := range skillLink.FindAllStringSubmatch(src, -1) {
			if strings.Contains(m[1], ":") {
				continue
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(file), m[1])); err != nil {
				t.Errorf("%s: link to %s: %v", file, m[1], err)
			}
		}
		for _, m := range skillIdent.FindAllStringSubmatch(src, -1) {
			obj := lookup(m[1]).Scope().Lookup(m[2])
			if obj == nil {
				t.Errorf("%s: %s.%s does not exist", file, m[1], m[2])
				continue
			}
			if m[3] != "" && !skillHas(obj.Type(), m[3]) {
				t.Errorf("%s: %s.%s has no %s", file, m[1], m[2], m[3])
			}
		}
		for _, m := range skillMethod.FindAllStringSubmatch(src, -1) {
			recv := skillReceiver[m[1]]
			if obj := lookup(recv[0]).Scope().Lookup(recv[1]); obj == nil || !skillHas(obj.Type(), m[2]) {
				t.Errorf("%s: %s.%s has no method %s", file, recv[0], recv[1], m[2])
			}
		}
		for _, m := range skillPackage.FindAllStringSubmatch(src, -1) {
			if info, err := os.Stat(m[1]); err != nil || !info.IsDir() {
				t.Errorf("%s: package %s does not exist", file, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// skillFrontMatter returns the name and description of a SKILL.md.
func skillFrontMatter(src string) (name, description string) {
	rest, ok := strings.CutPrefix(src, "---\n")
	if !ok {
		return "", ""
	}
	head, _, _ := strings.Cut(rest, "\n---\n")
	for _, line := range strings.Split(head, "\n") {
		if v, ok := strings.CutPrefix(line, "name: "); ok {
			name = strings.TrimSpace(v)
		} else if v, ok := strings.CutPrefix(line, "description: "); ok {
			description = strings.TrimSpace(v)
		}
	}
	return name, description
}

// skillHas reports whether t, or a pointer to it, has a field or method
// named name.
func skillHas(t types.Type, name string) bool {
	if _, ok := t.Underlying().(*types.Signature); ok {
		return false
	}
	obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(t), true, nil, name)
	return obj != nil
}
