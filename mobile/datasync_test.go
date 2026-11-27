package mobile

import "testing"

func TestDataSyncExportedNoOp(t *testing.T) {
	if DataSyncSupported() {
		t.Fatal("DataSyncSupported() = true on non-Android without host")
	}
	ds, err := StartDataSync(DataSyncOptions{
		Notification: DataSyncNotification{Title: "Upload", Body: "photo.jpg", Current: 1, Total: 2},
	})
	if err != nil {
		t.Fatalf("StartDataSync: %v", err)
	}
	if ds.Active() {
		t.Error("Active() = true on non-Android")
	}
	ds.Update(DataSyncNotification{Title: "Upload", Current: 2, Total: 2})
	ds.Stop()
	select {
	case <-ds.Ended():
	default:
		t.Error("Ended() not closed after Stop")
	}
}
