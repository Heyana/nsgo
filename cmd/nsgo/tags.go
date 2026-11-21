package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// goTags returns the tags a Go command for p takes: those in GOFLAGS, which a -tags on the command line would otherwise replace, then the app's build.tags, then extra, the tool's own, each once.
func goTags(ctx context.Context, p *project, env []string, extra ...string) ([]string, error) {
	flags, err := goFlags(ctx, p.Dir, env)
	if err != nil {
		return nil, err
	}
	tags := append(goFlagTags(flags), p.Build.Tags...)
	if p.steamBuild(envTarget(env)) {
		tags = append(tags, steamBuildTag)
	}
	tags = append(tags, extra...)
	return uniqueTags(tags), nil
}

// tagsArgs is the -tags flag for tags, or nothing for none, so GOFLAGS's own -tags still applies.
func tagsArgs(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	return []string{"-tags", strings.Join(tags, ",")}
}

// goFlags returns the GOFLAGS the go command takes in dir under env, which counts what go env -w set.
func goFlags(ctx context.Context, dir string, env []string) (string, error) {
	out, err := execute(ctx, processCommand{
		name: "go", args: []string{"env", "GOFLAGS"}, dir: dir, env: append(os.Environ(), env...),
	})
	if err != nil {
		return "", fmt.Errorf("go env GOFLAGS: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// goFlagTags returns the tags of GOFLAGS's -tags settings. The go command takes each GOFLAGS entry as -flag=value, with one dash or two, and a later -tags replaces an earlier one, as on a command line.
func goFlagTags(flags string) []string {
	var tags []string
	for _, field := range strings.Fields(flags) {
		if strings.HasPrefix(field, "--") {
			field = field[1:]
		}
		value, ok := strings.CutPrefix(field, "-tags=")
		if !ok {
			continue
		}
		tags = tags[:0]
		for _, tag := range strings.Split(value, ",") {
			if tag = strings.TrimSpace(tag); tag != "" {
				tags = append(tags, tag)
			}
		}
	}
	return tags
}

// uniqueTags returns tags without empty and repeated ones, in their first order.
func uniqueTags(tags []string) []string {
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag != "" && !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	return out
}

// validTag reports whether tag is a build tag a //go:build line can name: letters, digits, underscores and dots.
func validTag(tag string) bool {
	if tag == "" {
		return false
	}
	for _, r := range tag {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

// targetTags are the tags target's build adds to the app's, as buildHarmony, watchos.go and miniprogram.go pass them: nsgoharmony for harmonyos, nsgowatch for watchos and watchos_simulator, and miniprogramTags for every miniprogram_<platform>.
func targetTags(target string) []string {
	switch os, _, _ := strings.Cut(target, "_"); os {
	case "harmonyos":
		return []string{"nsgoharmony"}
	case "watchos":
		return []string{"nsgowatch"}
	case "miniprogram":
		return miniprogramTags
	}
	return nil
}
