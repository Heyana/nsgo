package mobile

import "github.com/nsgo-dev/nsgo/core"

type (
	// DataSync is a handle to a reference-counted Android dataSync foreground
	// service session. Multiple concurrent transfers (downloads, uploads, or sync)
	// may each call StartDataSync and share one foreground service and progress
	// notification; the service stops once every active handle calls Stop. All
	// methods are safe on a nil handle and from any goroutine.
	DataSync = core.DataSync

	// DataSyncOptions configures a dataSync foreground service session.
	DataSyncOptions = core.DataSyncOptions

	// DataSyncNotification describes the ongoing notification shown while an
	// Android dataSync foreground service session is active.
	DataSyncNotification = core.DataSyncNotification
)

var (
	// ErrDataSyncNotAllowed reports that Android refused to start the foreground
	// service outside an eligible foreground state
	// (ForegroundServiceStartNotAllowedException).
	ErrDataSyncNotAllowed = core.ErrDataSyncNotAllowed

	// ErrDataSyncUnavailable reports that the Android app does not declare
	// DataSyncService (nsgo.yaml's android.data_sync: true) or the runtime does
	// not support it.
	ErrDataSyncUnavailable = core.ErrDataSyncUnavailable

	// ErrDataSyncTimedOut reports that Android ended the dataSync foreground
	// service because its system time limit expired (API 35+ Service.onTimeout).
	ErrDataSyncTimedOut = core.ErrDataSyncTimedOut
)

// DataSyncSupported reports whether Android's dataSync foreground service is
// declared in the app's manifest (nsgo.yaml's android.data_sync: true) and
// supported by the runtime. It returns false on non-Android platforms.
func DataSyncSupported() bool { return core.DataSyncSupported() }

// StartDataSync acquires a reference to Android's dataSync foreground service,
// keeping the Go process alive across backgrounding and Activity destruction
// until every active handle calls Stop. On non-Android platforms it returns a
// safe no-op handle with a nil error.
func StartDataSync(opts DataSyncOptions) (*DataSync, error) { return core.StartDataSync(opts) }
