package main

import (
	"testing"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/nsgotest"
)

func TestGallery(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Press("id:open-Controls")
	app.Press("label:Agree")
	if !app.Find("role:checkbox Agree").Props.Checked {
		t.Fatal("the checkbox did not check")
	}
	app.Press("label:L")
	if !app.Find("role:radio L").Props.Checked {
		t.Fatal("the radio did not select")
	}
	app.Back()
	app.Press("id:open-Popups")
	app.Press("text:Menu")
	app.Press("label:Paste")
	app.Find("text:Picked Paste")
	app.Press("text:Dialog")
	app.Find("text:A dialog")
	app.Back()
	if app.Exists("text:A dialog") {
		t.Fatal("back did not close the dialog")
	}
	app.Back()
	app.Press("id:open-Images")
	app.Load("id:logo", core.Size{Width: 192, Height: 192})
	if got := app.Find("id:logo").Frame.Size; got != (core.Size{Width: 96, Height: 96}) {
		t.Fatalf("logo = %v, want its declared size", got)
	}
}
