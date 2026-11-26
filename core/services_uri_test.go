package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestContentURIFileServices(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	h.commit()
	h.tree.SetEnvironment(Environment{OS: OSAndroid})
	h.commit()
	var opened int
	var openErr error
	cx.OpenFile(SharedFile{URI: "content://downloads/all/report.pdf", Name: "report.pdf"}, func(ok bool, err error) {
		opened++
		openErr = err
	})
	it := command(h.commit(), driver.ItemOpenFile)
	if it == nil || it.Open.URI != "content://downloads/all/report.pdf" || it.Open.Path != "" || it.Open.MIME != "application/pdf" {
		t.Fatalf("open URI = %+v", it)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventFiles, Count: it.Count, Finished: true})
	if opened != 1 || openErr != nil {
		t.Fatalf("open result = %d, %v", opened, openErr)
	}

	var shown int
	var shareErr error
	cx.Share(ShareOptions{Files: []SharedFile{{URI: "content://downloads/all/report.pdf", Name: "report.pdf"}}}, func(ok bool, err error) {
		if ok {
			shown++
		}
		shareErr = err
	})
	it = command(h.commit(), driver.ItemShare)
	if it == nil || len(it.Share.Files) != 1 || it.Share.Files[0].URI != "content://downloads/all/report.pdf" {
		t.Fatalf("share URI = %+v", it)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventFiles, Count: it.Count, Finished: true})
	if shown != 1 || shareErr != nil {
		t.Fatalf("share result = %d, %v", shown, shareErr)
	}
	openErr = nil
	cx.OpenFile(SharedFile{URI: "file:///tmp/report.pdf", Name: "report.pdf"}, func(_ bool, err error) { openErr = err })
	if openErr == nil || command(h.commit(), driver.ItemOpenFile) != nil {
		t.Fatal("non-content URI was handed to Android")
	}

	h.tree.SetEnvironment(Environment{OS: OSMacOS})
	h.commit()
	cx.OpenFile(SharedFile{URI: "content://downloads/all/report.pdf", Name: "report.pdf"}, func(_ bool, err error) { openErr = err })
	if openErr == nil || command(h.commit(), driver.ItemOpenFile) != nil {
		t.Fatal("non-Android open accepted a content URI")
	}
}
