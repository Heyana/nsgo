package engine

import (
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLaunchSortsFilesFromLinks(t *testing.T) {
	dir := t.TempDir()
	doc, spaced := filepath.Join(dir, "notes.txt"), filepath.Join(dir, "日本 語.flow")
	for _, path := range []string{doc, spaced} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	urlPath := filepath.ToSlash(spaced)
	if filepath.VolumeName(spaced) != "" {
		urlPath = "/" + urlPath
	}
	fileURL := (&url.URL{Scheme: "file", Path: urlPath}).String()
	otherArgs := []string{`C:\Users\a.txt`, "file://otherhost/a.txt", "words", dir}
	// A Windows file association passes a path, and a Linux desktop entry
	// can pass a path or a file URL, without a separator.
	for _, preferLinks := range []bool{false, true} {
		got := parseLaunch(append([]string{doc, fileURL}, otherArgs...), nil, preferLinks)
		if want := []string{doc, spaced}; !slices.Equal(got.Files, want) || len(got.Links) != 0 {
			t.Errorf("document launch = %+v, want files %q and no links", got, want)
		}
	}

	links := []string{"umapp://connect?endpoint=https%3A%2F%2Fexample.com&key=k", "Umapp:settings", "https://example.com/a"}
	args := append([]string{links[0], doc, "--", spaced, fileURL, links[1], "--", links[2]}, otherArgs...)
	if got := parseLaunch(args, nil, false); !slices.Equal(got.Files, []string{doc}) || !slices.Equal(got.Links, links) {
		t.Errorf("launch with separator = %+v, want only %q and links %q", got, doc, links)
	}
	if got := parseLaunch([]string{"--", doc, fileURL}, nil, true); len(got.Files) != 0 || len(got.Links) != 0 {
		t.Errorf("launch with only arguments after -- = %+v, want no files or links", got)
	}

	// LaunchLinks is also used independently of file activation.
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	os.Args = append([]string{"app"}, args...)
	if got := LaunchLinks(); !slices.Equal(got, links) {
		t.Errorf("links %q, want %q", got, links)
	}
}

func TestLaunchFilesCustomParser(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range []string{"editor", "settings.yaml", "scene.flow"} {
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"editor", "-config", "settings.yaml", "scene.flow", "--", "editor"}
	absolute := func(name string) string {
		t.Helper()
		path, err := filepath.Abs(name)
		if err != nil {
			t.Fatal(err)
		}
		return path
	}
	doc := absolute("scene.flow")
	if got, want := parseLaunch(args, nil, false).Files, []string{absolute("editor"), absolute("settings.yaml"), doc}; !slices.Equal(got, want) {
		t.Errorf("default files = %q, want %q", got, want)
	}
	calls := 0
	got := parseLaunch(args, func(args []string) []string {
		calls++
		if want := []string{"editor", "-config", "settings.yaml", "scene.flow"}; !slices.Equal(args, want) {
			t.Fatalf("parser arguments = %q, want %q", args, want)
		}
		// The app's CLI owns the subcommand and the flag with its value.
		return []string{absolute(args[3])}
	}, false)
	if calls != 1 || !slices.Equal(got.Files, []string{doc}) {
		t.Errorf("custom launch = %+v with %d parser calls, want %q with one call", got, calls, doc)
	}
	got = parseLaunch(args, func([]string) []string { return nil }, false)
	if len(got.Files) != 0 || len(got.Links) != 0 {
		t.Errorf("disabled launch = %+v, want no files or links", got)
	}
	got = parseLaunch([]string{"--", "editor"}, func(args []string) []string {
		t.Fatalf("no file arguments before --, but called parser with %q", args)
		return nil
	}, false)
	if len(got.Files) != 0 {
		t.Errorf("separated launch = %+v, want no files", got)
	}
}

func TestLaunchWindowsLinkPrecedence(t *testing.T) {
	doc := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(doc, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := "umapp://settings"
	args := []string{doc, "--", link, doc}
	for _, parse := range []func([]string) []string{
		nil,
		func(args []string) []string {
			t.Fatalf("Windows link launch called the file parser with %q", args)
			return nil
		},
	} {
		if got := parseLaunch(args, parse, true); len(got.Files) != 0 || !slices.Equal(got.Links, []string{link}) {
			t.Errorf("Windows link launch = %+v, want only link %q", got, link)
		}
	}
}
