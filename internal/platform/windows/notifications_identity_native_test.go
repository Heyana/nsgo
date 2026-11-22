//go:build windows

package windows

import (
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Check the native Tag/Group setters without displaying or scheduling a toast.
func TestToastIdentityUsesAcceptedNativeTags(t *testing.T) {
	defer apartmentThread()()
	for _, scheduled := range []bool{false, true} {
		for _, id := range []string{"chat:42 ", strings.Repeat("long-id:", 40), strings.Repeat("🦊", 9)} {
			n := driver.Notification{ID: id, Title: "NSGo identity test"}
			launch := toastLaunch(id)
			doc := toastDocument(n, launch)
			if doc == nil {
				t.Fatal("cannot create toast XML")
			}
			name, factoryIID, identityIID := "Windows.UI.Notifications.ToastNotification", &iidToastFactory, &iidToastNotification2
			if scheduled {
				name, factoryIID, identityIID = "Windows.UI.Notifications.ScheduledToastNotification", &iidScheduledToasts, &iidScheduledToast2
			}
			factory := activationFactory(name, factoryIID)
			if factory == nil {
				doc.release()
				t.Fatal("toast factory unavailable")
			}
			var toast *comObject
			args := []uintptr{doc.ptr()}
			if scheduled {
				ticks := uint64(time.Now().Add(time.Minute).UnixNano()/100 + 116444736000000000)
				args = append(args, u64Args(ticks)...)
			}
			hr := factory.call(toastFactoryCreate, append(args, uintptr(unsafe.Pointer(&toast)))...)
			factory.release()
			doc.release()
			if failed(hr) {
				t.Fatal(hresultError("create toast", hr))
			}
			if !setToastIdentity(toast, identityIID, id) {
				toast.release()
				t.Fatalf("native identity rejected %q (scheduled=%v)", id, scheduled)
			}
			identity := toast.query(identityIID)
			var tag, group uintptr
			tagHR := identity.call(7, uintptr(unsafe.Pointer(&tag)))
			groupHR := identity.call(9, uintptr(unsafe.Pointer(&group)))
			wantTag, wantGroup := toastIdentity(id)
			if failed(tagHR) || failed(groupHR) || hstringText(tag) != wantTag || hstringText(group) != wantGroup {
				t.Errorf("native identity = %q/%q, want %q/%q", hstringText(tag), hstringText(group), wantTag, wantGroup)
			}
			deleteHString(tag)
			deleteHString(group)
			identity.release()
			toast.release()
		}
	}
}
