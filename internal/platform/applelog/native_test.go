//go:build darwin && !ios

package applelog

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func TestNativePublicMessage(t *testing.T) {
	if os.Getenv("NSGO_TEST_UNIFIED_LOG") != "1" {
		t.Skip("set NSGO_TEST_UNIFIED_LOG=1 to read this process's unified log")
	}
	subsystem := fmt.Sprintf("dev.nsgo.test.logs.%d", os.Getpid())
	sink, err := newLog(subsystem, "nsgo")
	if err != nil {
		t.Fatal(err)
	}
	message := "nsgo unified log: 中文 100% %@ %s"
	sink.write(errorType, message)
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		out, err := exec.CommandContext(ctx, "/usr/bin/log", "show", "--style", "json", "--last", "1m", "--predicate", fmt.Sprintf("subsystem == %q", subsystem)).CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("read this test's unified log: %v: %s", err, out)
		}
		var records []struct {
			Message string `json:"eventMessage"`
		}
		if err := json.Unmarshal(out, &records); err != nil {
			t.Fatalf("decode unified log: %v: %s", err, out)
		}
		for _, record := range records {
			if strings.Contains(record.Message, message) {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("the unified log did not decode the purego message")
}

// A section outside its segment must never become an unsafe memory read.
func TestImageFormatBounds(t *testing.T) {
	image := make([]byte, 256)
	binary.LittleEndian.PutUint32(image, 0xfeedfacf)
	binary.LittleEndian.PutUint32(image[20:], 152)
	command := image[32:184]
	binary.LittleEndian.PutUint32(command, 0x19)
	binary.LittleEndian.PutUint32(command[4:], 152)
	copy(command[8:], "__TEXT")
	binary.LittleEndian.PutUint64(command[24:], 0x1000)
	binary.LittleEndian.PutUint64(command[32:], uint64(len(image)))
	binary.LittleEndian.PutUint32(command[64:], 1)
	section := command[72:]
	copy(section, "__oslogstring")
	binary.LittleEndian.PutUint64(section[32:], 0x10c0)
	binary.LittleEndian.PutUint64(section[40:], 12)
	copy(image[192:], publicFormat)
	if _, object, err := imageFormat(unsafe.Pointer(&image[0])); err != nil || object {
		t.Fatalf("valid format: object %v, %v", object, err)
	}
	binary.LittleEndian.PutUint64(section[32:], 0x10ff)
	if _, _, err := imageFormat(unsafe.Pointer(&image[0])); err == nil {
		t.Fatal("accepted section extending past segment")
	}
	binary.LittleEndian.PutUint64(section[32:], 0xfff)
	if _, _, err := imageFormat(unsafe.Pointer(&image[0])); err == nil {
		t.Fatal("accepted section before segment")
	}
}
