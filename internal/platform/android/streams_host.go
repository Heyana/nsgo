//go:build nsgoandroidhost && !android

package android

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

const hostRuntime = true

// The device and development computer's monotonic clocks have different epochs. Let the engine use the time it received the frame.
func frameTime(r *reader) time.Time {
	r.u64() // The Android frame timestamp belongs to the device clock.
	return time.Time{}
}

func runtimeStreams() (io.Reader, io.WriteCloser) {
	addr := os.Getenv("NSGO_ANDROID_HOST_ADDRESS")
	token := os.Getenv("NSGO_ANDROID_HOST_TOKEN")
	if addr == "" || len(token) != 32 {
		panic("android: the host runtime requires nsgo dev -android-host")
	}
	// adb may have installed the forward before the Activity has entered its accept loop. Wait for the runtime to acknowledge the connection.
	var err error
	for until := time.Now().Add(8 * time.Second); time.Now().Before(until); time.Sleep(50 * time.Millisecond) {
		var conn net.Conn
		conn, err = net.DialTimeout("tcp4", addr, time.Second)
		if err != nil {
			continue
		}
		var ready [1]byte
		conn.SetDeadline(time.Now().Add(time.Second))
		if _, err = io.WriteString(conn, token); err == nil {
			_, err = io.ReadFull(conn, ready[:])
		}
		if err == nil && ready[0] == 0xa7 {
			conn.SetDeadline(time.Time{})
			return conn, conn
		}
		if err == nil {
			err = fmt.Errorf("unexpected development socket acknowledgement %x", ready)
		}
		conn.Close()
	}
	panic(fmt.Sprintf("android: host runtime at %s did not accept the connection: %v", addr, err))
}
