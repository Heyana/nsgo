package main

import (
	"testing"

	"github.com/oneo-me/nsgo/nsgotest"
)

func TestAddAndCompleteTodos(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Find("text:2 to do")

	app.Type("id:draft", "Write tests")
	app.Find("text:Add “Write tests”?")
	app.Submit("id:draft")
	app.Find("text:3.")
	app.Find("text:Write tests")
	if got := app.Find("id:draft").Text; got != "" {
		t.Fatalf("draft = %q after adding, want empty", got)
	}

	for range 3 {
		app.Press("role:button Done >> nth=0")
	}
	app.Find("text:All done.")
	app.Find("text:0 to do")
}
