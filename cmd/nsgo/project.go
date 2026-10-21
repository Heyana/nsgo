package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// project is an app's nsgo.yaml and where it lives.
type project struct {
	Dir string `yaml:"-"`
	// Release strips symbols and debug information for the smallest
	// binaries; nsgo build sets it, nsgo dev does not.
	Release bool `yaml:"-"`

	Name    string `yaml:"name"`
	ID      string `yaml:"id"`
	Version string `yaml:"version"`
	Window  struct {
		Width  int `yaml:"width"`
		Height int `yaml:"height"`
	} `yaml:"window"`
	Dev struct {
		Target string `yaml:"target"`
	} `yaml:"dev"`
	Build struct {
		Output  string   `yaml:"output"`
		Targets []string `yaml:"targets"`
	} `yaml:"build"`
}

func load(dir string) (*project, error) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(abs, "nsgo.yaml"))
	if err != nil {
		return nil, err
	}
	p := &project{Dir: abs}
	if err := yaml.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("nsgo.yaml: name is required")
	}
	if p.ID == "" {
		p.ID = "dev.nsgo." + p.Name
	}
	if p.Version == "" {
		p.Version = "0.0.0"
	}
	if p.Window.Width == 0 || p.Window.Height == 0 {
		p.Window.Width, p.Window.Height = 480, 640
	}
	if p.Dev.Target == "" {
		p.Dev.Target = "native"
	}
	if p.Build.Output == "" {
		p.Build.Output = "dist"
	}
	if !filepath.IsAbs(p.Build.Output) {
		p.Build.Output = filepath.Join(abs, p.Build.Output)
	}
	if len(p.Build.Targets) == 0 {
		p.Build.Targets = []string{"macos_" + goArchName(runtime.GOARCH)}
	}
	for _, t := range p.Build.Targets {
		if !slices.Contains(targets, t) {
			return nil, fmt.Errorf("nsgo.yaml: unknown target %q; use one of %s", t, strings.Join(targets, ", "))
		}
	}
	return p, nil
}

// cache is the project's directory for development builds.
func (p *project) cache(parts ...string) string {
	return filepath.Join(append([]string{p.Dir, ".nsgo"}, parts...)...)
}

// ldflags passes nsgo.yaml's name and window size to nsgo.Run.
func (p *project) ldflags(extra ...string) string {
	const pkg = "github.com/oneo-me/nsgo"
	flags := fmt.Sprintf("-X '%s.name=%s' -X %s.width=%d -X %s.height=%d",
		pkg, p.Name, pkg, p.Window.Width, pkg, p.Window.Height)
	if p.Release {
		flags += " -s -w"
	}
	for _, e := range extra {
		flags += " " + e
	}
	return flags
}

// goArchName maps Go's amd64 to x64, the name packages use.
func goArchName(arch string) string {
	if arch == "amd64" {
		return "x64"
	}
	return arch
}
