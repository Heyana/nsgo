//go:build android

package android

import (
	"encoding/binary"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	androidruntime "github.com/nsgo-dev/nsgo/internal/platform/android/runtime"
)

func TestRequestReviewEncodesWithNoExtraPayload(t *testing.T) {
	var w writer
	encode(&w, &driver.MountBatch{
		Root:  1,
		Items: []driver.MountItem{{Kind: driver.ItemRequestReview}},
	}, false, nil, nil, nil, nil)
	r := reader{w.buf}
	if root := r.u32(); root != 1 {
		t.Fatalf("root = %d, want 1", root)
	}
	if anim := r.byte(); anim != 0 {
		t.Fatalf("layout animation kind = %d, want 0", anim)
	}
	if first := r.byte(); first != 0 {
		t.Fatalf("first flag = %d, want 0", first)
	}
	if n := r.u32(); n != 1 {
		t.Fatalf("item count = %d, want 1", n)
	}
	if kind := driver.ItemKind(r.byte()); kind != driver.ItemRequestReview {
		t.Fatalf("item kind = %v, want ItemRequestReview", kind)
	}
	if id := r.u32(); id != 0 {
		t.Fatalf("item id = %d, want 0", id)
	}
	if len(r.buf) != 0 {
		t.Fatalf("trailing payload = %d bytes, want 0", len(r.buf))
	}
}

// The Android CI runner rebuilds the runtime before packaging test apps. Check class definitions, rather than mere string references in NSGo's reflection.
func TestStandaloneReviewSDKClasses(t *testing.T) {
	data := androidruntime.ClassesDex
	if len(data) < 112 {
		t.Fatal("runtime DEX header is missing")
	}
	u32 := func(at uint32) uint32 { return binary.LittleEndian.Uint32(data[at : at+4]) }
	stringsAt, typesAt := u32(60), u32(68)
	classes, classesAt := u32(96), u32(100)
	defined := map[string]bool{}
	for i := uint32(0); i < classes; i++ {
		class := u32(classesAt + i*32)
		name := u32(typesAt + class*4)
		at := u32(stringsAt + name*4)
		// DEX string_data starts with its UTF-16 length as ULEB128.
		for {
			b := data[at]
			at++
			if b < 128 {
				break
			}
		}
		end := at
		for data[end] != 0 {
			end++
		}
		defined[string(data[at:end])] = true
	}
	for _, class := range []string{
		"Lcom/google/android/play/core/review/ReviewManagerFactory;",
		"Lcom/google/android/play/core/review/ReviewManager;",
		"Lcom/google/android/play/core/review/ReviewInfo;",
		"Lcom/google/android/play/core/common/PlayCoreDialogWrapperActivity;",
		"Lcom/google/android/gms/tasks/Task;",
		"Lcom/google/android/gms/tasks/OnCompleteListener;",
	} {
		if !defined[class] {
			t.Errorf("standalone runtime lacks SDK class %s", class)
		}
	}
}
