//go:build android

package android

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestPictureInPictureCommandKeepsLegacyBatchAlignment(t *testing.T) {
	batch := &driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemPictureInPictureCommand, ID: 7, Count: driver.PictureInPictureStart},
		{Kind: driver.ItemVideoSeek, ID: 7, Time: 900 * time.Millisecond},
	}}
	var w writer
	encode(&w, batch, false, nil, nil, nil, nil)
	r := reader{buf: w.buf}
	if r.u32() != 1 || r.byte() != 0 || r.byte() != 0 || r.u32() != 2 {
		t.Fatal("invalid batch header")
	}
	if r.byte() != byte(driver.ItemNativeCommand) || r.u32() != 7 || r.str() != driver.PictureInPictureEventName {
		t.Fatal("PiP is not encoded as the native command older runtimes can consume")
	}
	var command struct{ Command uint32 }
	if err := json.Unmarshal([]byte(r.str()), &command); err != nil || command.Command != driver.PictureInPictureStart {
		t.Fatalf("PiP command = %+v, %v", command, err)
	}
	if r.byte() != byte(driver.ItemVideoSeek) || r.u32() != 7 || r.u64() != 900 {
		t.Fatal("the following video seek lost its wire alignment")
	}
}
