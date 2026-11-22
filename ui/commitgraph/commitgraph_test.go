package commitgraph

import (
	"fmt"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

func history(n int) []Commit {
	day := time.Date(2026, 1, 5, 13, 7, 0, 0, time.UTC)
	commits := []Commit{
		{Hash: "merge1234567", Parents: []string{"c0", "side"}, Subject: "Merge side", Author: "Ada", Date: day,
			Refs: []Ref{{Name: "main", Kind: RefHead}, {Name: "origin/main", Kind: RefRemote, Current: true}, {Name: "v1.0", Kind: RefTag}}, Head: true},
		{Hash: "side", Parents: []string{"c0"}, Subject: "Side work", Author: "Ken", Date: day, More: true},
	}
	for i := range n {
		commit := Commit{Hash: fmt.Sprint("c", i), Subject: fmt.Sprint("Change ", i), Author: "Ada", Date: day}
		if i < n-1 {
			commit.Parents = []string{fmt.Sprint("c", i+1)}
		}
		commits = append(commits, commit)
	}
	return commits
}

func start(t *testing.T, width core.Points, commits []Commit, view View) *nsgotest.App {
	t.Helper()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		view.Commits = func() []Commit { return commits }
		view.Selection = ui.NewSelection(cx, false)
		view.TestID = "commits"
		return view
	}, nsgotest.Size(width, 300))
	app.Settle()
	return app
}

// TestViewListsCommits shows a commit's refs, subject, author and hash, and
// builds only the rows near what shows.
func TestViewListsCommits(t *testing.T) {
	var activated string
	app := start(t, 900, history(500), View{OnActivate: func(c Commit) { activated = c.Hash }})
	for _, want := range []string{"text:Merge side", "text:/^main$/", "text:origin/main", "text:v1.0", "text:merge12", "text:Side work"} {
		if !app.Exists(want) {
			t.Fatalf("%s:\n%s", want, app.Dump())
		}
	}
	if app.Exists("text:Change 400") {
		t.Fatal("the list built rows far from what shows")
	}
	app.Press("text:Side work")
	app.Key("enter")
	if activated != "side" {
		t.Fatalf("activated %q", activated)
	}
}

// TestViewCollapsesAMerge folds the side branch into the merge's row.
func TestViewCollapsesAMerge(t *testing.T) {
	app := start(t, 900, history(5), View{})
	app.Press("label:Collapse Merge")
	if app.Exists("text:Side work") || !app.Exists("text:Change 1") {
		t.Fatal(app.Dump())
	}
	app.Press("label:Expand Merge")
	if !app.Exists("text:Side work") {
		t.Fatal(app.Dump())
	}
}

// TestViewLeavesOutDetailsWhenNarrow keeps refs, subjects and the author's
// avatar on a phone, and no avatar for a row without an author.
func TestViewLeavesOutDetailsWhenNarrow(t *testing.T) {
	commits := append([]Commit{{Hash: "changes", Parents: []string{"merge1234567"}, Subject: "Changes"}}, history(5)...)
	app := start(t, 390, commits, View{})
	if !app.Exists("text:Merge side") || app.Exists("text:merge12") || app.Exists("text:Ken") {
		t.Fatal(app.Dump())
	}
	if avatar := app.Find("label:Ken"); avatar.Frame.Size.Width != 16 {
		t.Fatal(app.Dump())
	}
	// The subject's box and the avatar, or the box alone.
	if len(app.Find("text:Side work").Parent.Parent.Children) != 2 || len(app.Find("text:Changes").Parent.Parent.Children) != 1 {
		t.Fatal(app.Dump())
	}
}

// TestViewShowsDetailsWhenWide keeps the author's avatar and name, hash and
// date in a wide row.
func TestViewShowsDetailsWhenWide(t *testing.T) {
	app := start(t, 900, history(5), View{})
	for _, want := range []string{"label:Ken", "text:Ken", "text:side"} {
		if !app.Exists(want) {
			t.Fatalf("%s:\n%s", want, app.Dump())
		}
	}
}

// TestRefsGiveWayToTheSubject narrows a row of long refs: it stays inside the
// list and the subject keeps its room.
func TestRefsGiveWayToTheSubject(t *testing.T) {
	long := "feature/a-branch-with-a-long-name"
	app := start(t, 350, []Commit{{Hash: "a", Subject: "Subject", Refs: []Ref{{Name: long}, {Name: "origin/" + long, Kind: RefRemote}}}}, View{})
	subject := app.Find("text:Subject").Parent
	if subject.Frame.Size.Width < 120 || subject.Parent.Frame.Size.Width > 350 {
		t.Fatal(app.Dump())
	}
}

func TestShortHash(t *testing.T) {
	for commit, want := range map[*Commit]string{{Hash: "abcdefghij"}: "abcdefg", {Hash: "abc"}: "abc", {Hash: "abcdefghij", Short: "ab"}: "ab"} {
		if got := shortHash(*commit); got != want {
			t.Fatalf("%q, want %q", got, want)
		}
	}
}
