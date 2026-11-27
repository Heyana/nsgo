package main

import (
	"archive/zip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type itchBuild struct {
	dir, destination string
}

var itchProject = regexp.MustCompile(`^[a-z0-9_-]+/[a-z0-9_-]+$`)
var itchChannel = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// publishItch sends existing NSGo packages to itch.io through butler.
func publishItch(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("publish itch", flag.ExitOnError)
	targets := flags.String("target", "", "comma-separated web or desktop `targets` to upload")
	output := flags.String("output", "", "the build's output `directory` (default: nsgo.yaml's build.output)")
	version := flags.String("version", "", "the build's `version` (default: nsgo.yaml's version)")
	if err := parseCommand(flags, args, 1); err != nil {
		return err
	}
	requested := splitList(*targets)
	if len(requested) == 0 {
		return errors.New("publish itch: set -target to the built web or desktop targets to upload")
	}
	p, err := load(flags.Arg(0))
	if err != nil {
		return err
	}
	if *output != "" {
		p.Build.Output = p.resolve(*output)
	}
	if *version != "" {
		p.Version = *version
	}
	builds, err := p.itchBuilds(requested)
	if err != nil {
		return err
	}
	key := os.Getenv("BUTLER_API_KEY")
	if key == "" {
		return errors.New("publish itch: set BUTLER_API_KEY to your itch.io API key")
	}
	butler, cleanup, err := itchButler(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	for _, build := range builds {
		step := begin("uploading %s to itch.io", build.destination)
		cmd := exec.CommandContext(ctx, butler, "push", build.dir, build.destination, "--userversion", p.Version)
		if out, err := cmd.CombinedOutput(); err != nil {
			step.fail()
			return fmt.Errorf("butler push %s: %w\n%s", build.destination, err, strings.ReplaceAll(string(out), key, "[redacted]"))
		}
		step.done("uploaded "+build.destination, "")
	}
	return nil
}

func (p *project) itchBuilds(targets []string) ([]itchBuild, error) {
	if !itchProject.MatchString(p.Itch.Project) {
		return nil, errors.New("publish itch: set itch.project in nsgo.yaml to the itch.io project as owner/game")
	}
	targets, err := expandTargets(targets)
	if err != nil {
		return nil, err
	}
	builds := make([]itchBuild, 0, len(targets))
	destinations := map[string]string{}
	for _, target := range targets {
		channel, marker := p.itchTarget(target)
		if channel == "" {
			return nil, fmt.Errorf("publish itch: %s is not a web or desktop target", target)
		}
		if custom, exists := p.Itch.Channels[target]; exists {
			channel = custom
		}
		if !itchChannel.MatchString(channel) {
			return nil, fmt.Errorf("publish itch: itch.channels.%s needs a channel name with letters, digits, hyphens, or underscores", target)
		}
		channelKey := strings.ToLower(channel)
		if other := destinations[channelKey]; other != "" {
			return nil, fmt.Errorf("publish itch: %s and %s both use channel %q; give them different itch.channels", other, target, channel)
		}
		destinations[channelKey] = target
		dir := p.packageDir(target)
		info, err := os.Stat(filepath.Join(dir, marker))
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("publish itch: %s needs a completed nsgo build (missing %s)", target, rel(filepath.Join(dir, marker)))
		}
		builds = append(builds, itchBuild{dir: dir, destination: p.Itch.Project + ":" + channel})
	}
	return builds, nil
}

func (p *project) itchTarget(target string) (channel, marker string) {
	system, arch, _ := strings.Cut(target, "_")
	switch system {
	case "web":
		return "html5", "index.html"
	case "macos":
		return "osx-" + arch, filepath.Join(p.Name+".app", "Contents", "MacOS", p.Name)
	case "windows":
		return "windows-" + arch, p.Name + ".exe"
	case "linux":
		return "linux-" + arch, p.Name
	}
	return "", ""
}

const butlerVersion = "15.32.0"

var butlerChecksums = map[string]string{
	"darwin-arm64":  "f1c460f08c02686b49f8da74344e5578ece9ea1a8bae1779445b5d98e0a794b3",
	"darwin-amd64":  "a92947c9556c9bfb0cc9bdc3814f342b30e0ee7269efc418658894a916ca9584",
	"linux-arm64":   "4f2da37785fd9cbc15052427490e4e4f5cd735a019f21554d6273fa9a3490152",
	"linux-amd64":   "2335971394ef6596f95ded0833e85ee28755e13761716ef4c91d6b11f69162f5",
	"windows-amd64": "5902c488b86bb3518ca00bf4e706d7687f229e3f0f28653cd943348ea9ec8878",
}

func itchButler(ctx context.Context) (path string, cleanup func(), err error) {
	if path, err := exec.LookPath("butler"); err == nil {
		return path, func() {}, nil
	}
	channel, executable := runtime.GOOS+"-"+runtime.GOARCH, "butler"
	if runtime.GOOS == "windows" {
		// itch.io ships an x64 binary, which Windows on Arm runs under emulation.
		channel, executable = "windows-amd64", "butler.exe"
	}
	checksum := butlerChecksums[channel]
	if checksum == "" {
		return "", nil, fmt.Errorf("publish itch: no bundled butler download for %s; install butler on PATH", channel)
	}
	release := pinned{
		name:   "butler-" + butlerVersion + "-" + channel + ".zip",
		url:    "https://broth.itch.zone/butler/" + channel + "/" + butlerVersion + "/archive/default",
		sha256: checksum,
	}
	archive, err := release.fetch(ctx)
	if err != nil {
		return "", nil, err
	}
	z, err := zip.OpenReader(archive)
	if err != nil {
		return "", nil, err
	}
	defer z.Close()
	// butler push does not require the 7-Zip libraries from the archive.
	entry, err := z.Open(executable)
	if err != nil {
		return "", nil, err
	}
	defer entry.Close()
	data, err := io.ReadAll(entry)
	if err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp("", "nsgo-butler-")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.RemoveAll(dir) }
	path = filepath.Join(dir, executable)
	if err := os.WriteFile(path, data, 0o700); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}
