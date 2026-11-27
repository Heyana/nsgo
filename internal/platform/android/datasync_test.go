package android

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// TestDataSyncCapabilityAndProtocol checks that capDataSync is a free single bit
// in the fourth environment capability byte advertised by NsgoView, that
// Protocol.java and android.go agree on CAP_DATA_SYNC / EXT_DATA_SYNC /
// DATA_SYNC_REPORT, and that NsgoView.keeps() retains the engine while
// DataSyncService is active.
func TestDataSyncCapabilityAndProtocol(t *testing.T) {
	testhost.RequireSource(t)

	if capDataSync == 0 || capDataSync&(capDataSync-1) != 0 ||
		capDataSync&(capExtendedSensors|capAuthSession|capVideoPiP|capCapture|capFolderHandles|capFolderPersist|capFetchNoRedirect) != 0 {
		t.Fatalf("capDataSync %d is not a free single bit of the fourth byte", capDataSync)
	}

	protocol, err := os.ReadFile("java/dev/oneo/nsgo/Protocol.java")
	if err != nil {
		t.Fatal(err)
	}
	goSrc, err := os.ReadFile("android.go")
	if err != nil {
		t.Fatal(err)
	}

	checkJavaConst := func(name string, want int) {
		t.Helper()
		re := regexp.MustCompile(`static final int ` + name + ` = (\d+);`)
		m := re.FindSubmatch(protocol)
		if m == nil || string(m[1]) != strconv.Itoa(want) {
			t.Fatalf("Protocol.java %s = %q, want %d", name, m, want)
		}
	}
	checkJavaConst("CAP_DATA_SYNC", capDataSync)

	goReport := regexp.MustCompile(`msgDataSyncReport\s*=\s*(\d+)`).FindSubmatch(goSrc)
	if goReport == nil {
		t.Fatal("android.go lacks msgDataSyncReport")
	}
	reportNum, _ := strconv.Atoi(string(goReport[1]))
	checkJavaConst("DATA_SYNC_REPORT", reportNum)

	view, err := os.ReadFile("java/dev/oneo/nsgo/NsgoView.java")
	if err != nil {
		t.Fatal(err)
	}
	viewSrc := string(view)
	if !strings.Contains(viewSrc, "Protocol.CAP_DATA_SYNC") {
		t.Fatal("NsgoView.java does not advertise Protocol.CAP_DATA_SYNC")
	}
	if !strings.Contains(viewSrc, "DataSyncService.active(engine)") {
		t.Fatal("NsgoView.java does not check DataSyncService.active(engine) in keeps()")
	}
	if !strings.Contains(viewSrc, "onDataSyncEnded()") {
		t.Fatal("NsgoView.java does not handle onDataSyncEnded()")
	}

	for _, c := range []struct {
		fourth []byte
		want   bool
	}{
		{nil, false},
		{[]byte{0}, false},
		{[]byte{capFolderHandles}, false},
		{[]byte{capDataSync}, true},
		{[]byte{capExtendedSensors | capFolderHandles | capDataSync}, true},
	} {
		var w writer
		writeBaseEnvironment(&w, "en-US")
		w.byte(1)
		for range 4 {
			w.str("")
		}
		w.byte(capExtendedMessages)
		w.flags(false)
		w.str("")
		w.buf = append(w.buf, c.fourth...)
		r := reader{w.buf}
		if _, _, fourth := readEnvironmentCapabilities(&r); (fourth&capDataSync != 0) != c.want {
			t.Errorf("fourth %v: dataSync = %v, want %v", c.fourth, fourth&capDataSync != 0, c.want)
		}
	}
}
