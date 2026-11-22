package linux

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Pure-Go PipeWire native protocol probe for microphone (Audio/Source) device
// access. PipeWire's module-access and WirePlumber restrict global object
// visibility and PW_PERM_R bits on Audio/Source nodes for sandboxed Flatpak
// and Snap clients according to their device permissions.

const (
	pwCoreID     uint32 = 0
	pwRegistryID uint32 = 2

	pwCoreMethodHello       uint8 = 1
	pwCoreMethodSync        uint8 = 2
	pwCoreMethodGetRegistry uint8 = 5

	pwCoreEventDone  uint8 = 1
	pwCoreEventError uint8 = 3

	pwRegistryEventGlobal uint8 = 0

	pwProtocolVersion int32 = 3
	pwRegistryVersion int32 = 3
	pwSyncSeq         int32 = 42

	// PW_PERM_R (0400 octal) grants read/visibility access to a PipeWire object.
	pwPermR uint32 = 0o400

	spaTypeInt    uint32 = 4
	spaTypeString uint32 = 8
	spaTypeStruct uint32 = 14

	// Linux errno values carried negated in PipeWire Core::Error res.
	pwErrPerm  int32 = -1  // -EPERM
	pwErrAcces int32 = -13 // -EACCES

	maxPipeWireMessageSize = 1 << 20
	pipeWireProbeTimeout   = 1500 * time.Millisecond
)

type pipewireMicProbe uint8

const (
	// pipewireMicUnavailable means PipeWire could not be reached (no socket,
	// daemon not running, or protocol/IO error).
	pipewireMicUnavailable pipewireMicProbe = iota
	// pipewireMicGranted means PipeWire exposed at least one readable audio
	// capture node (Audio/Source, Audio/Source/Virtual, or Audio/Duplex).
	pipewireMicGranted
	// pipewireMicDenied means PipeWire or the sandbox explicitly denied access
	// (socket EACCES/EPERM, Core::Error -EACCES/-EPERM, or an audio capture
	// node exposed without PW_PERM_R).
	pipewireMicDenied
	// pipewireMicNoSources means PipeWire answered the registry sync cleanly
	// but exposed no audio capture nodes to this client.
	pipewireMicNoSources
)

// pipeWireSocketPath resolves the PipeWire native Unix socket path following
// PipeWire's own lookup order: PIPEWIRE_REMOTE (default "pipewire-0") inside
// PIPEWIRE_RUNTIME_DIR, XDG_RUNTIME_DIR, or USERPROFILE unless absolute.
func pipeWireSocketPath(getenv func(string) string) string {
	remote := strings.TrimSpace(getenv("PIPEWIRE_REMOTE"))
	if remote == "" {
		remote = "pipewire-0"
	}
	if filepath.IsAbs(remote) {
		return remote
	}
	for _, key := range []string{"PIPEWIRE_RUNTIME_DIR", "XDG_RUNTIME_DIR", "USERPROFILE"} {
		if dir := strings.TrimSpace(getenv(key)); dir != "" {
			return filepath.Join(dir, remote)
		}
	}
	return ""
}

// probePipeWireMicrophonePath connects to the PipeWire Unix socket at path and
// inspects registry visibility of audio capture nodes.
func probePipeWireMicrophonePath(path string, timeout time.Duration) pipewireMicProbe {
	if path == "" {
		return pipewireMicUnavailable
	}
	conn, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return pipewireMicDenied
		}
		return pipewireMicUnavailable
	}
	defer conn.Close()
	return probePipeWireMicrophoneConn(conn, timeout)
}

// probePipeWireMicrophoneConn performs a Core::Hello + Core::GetRegistry +
// Core::Sync round trip over conn and classifies microphone node access.
func probePipeWireMicrophoneConn(conn net.Conn, timeout time.Duration) pipewireMicProbe {
	if conn == nil {
		return pipewireMicUnavailable
	}
	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}

	helloPayload := encodeSPAStruct(appendSPAInt(nil, pwProtocolVersion))
	regBody := appendSPAInt(nil, pwRegistryVersion)
	regBody = appendSPAInt(regBody, int32(pwRegistryID))
	regPayload := encodeSPAStruct(regBody)
	syncBody := appendSPAInt(nil, int32(pwCoreID))
	syncBody = appendSPAInt(syncBody, pwSyncSeq)
	syncPayload := encodeSPAStruct(syncBody)

	var out []byte
	out = append(out, encodePipeWireMessage(pwCoreID, pwCoreMethodHello, 0, helloPayload)...)
	out = append(out, encodePipeWireMessage(pwCoreID, pwCoreMethodGetRegistry, 1, regPayload)...)
	out = append(out, encodePipeWireMessage(pwCoreID, pwCoreMethodSync, 2, syncPayload)...)
	if _, err := conn.Write(out); err != nil {
		return pipewireMicUnavailable
	}

	sawReadableSource := false
	sawBlockedSource := false

	for {
		id, opcode, payload, err := readPipeWireMessage(conn)
		if err != nil {
			if sawReadableSource {
				return pipewireMicGranted
			}
			if sawBlockedSource {
				return pipewireMicDenied
			}
			return pipewireMicUnavailable
		}

		switch {
		case id == pwCoreID && opcode == pwCoreEventDone:
			doneID, doneSeq, ok := parsePipeWireCoreDone(payload)
			if ok && doneID == int32(pwCoreID) && doneSeq == pwSyncSeq {
				if sawReadableSource {
					return pipewireMicGranted
				}
				if sawBlockedSource {
					return pipewireMicDenied
				}
				return pipewireMicNoSources
			}

		case id == pwCoreID && opcode == pwCoreEventError:
			res, ok := parsePipeWireCoreError(payload)
			if ok && (res == pwErrAcces || res == pwErrPerm) {
				return pipewireMicDenied
			}
			return pipewireMicUnavailable

		case id == pwRegistryID && opcode == pwRegistryEventGlobal:
			perms, ifaceType, props, ok := parsePipeWireRegistryGlobal(payload)
			if !ok || ifaceType != "PipeWire:Interface:Node" {
				continue
			}
			if isPipeWireAudioSourceClass(props["media.class"]) {
				if perms&pwPermR != 0 {
					sawReadableSource = true
				} else {
					sawBlockedSource = true
				}
			}
		}
	}
}

func isPipeWireAudioSourceClass(mediaClass string) bool {
	switch mediaClass {
	case "Audio/Source", "Audio/Source/Virtual", "Audio/Duplex":
		return true
	}
	return false
}

func spaPad8(n int) int {
	return (n + 7) &^ 7
}

func encodePipeWireMessage(id uint32, opcode uint8, seq uint32, payload []byte) []byte {
	msg := make([]byte, 16+len(payload))
	binary.NativeEndian.PutUint32(msg[0:4], id)
	binary.NativeEndian.PutUint32(msg[4:8], uint32(opcode)<<24|(uint32(len(payload))&0xffffff))
	binary.NativeEndian.PutUint32(msg[8:12], seq)
	binary.NativeEndian.PutUint32(msg[12:16], 0)
	copy(msg[16:], payload)
	return msg
}

func readPipeWireMessage(r io.Reader) (id uint32, opcode uint8, payload []byte, err error) {
	var hdr [16]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, 0, nil, err
	}
	id = binary.NativeEndian.Uint32(hdr[0:4])
	opSize := binary.NativeEndian.Uint32(hdr[4:8])
	opcode = uint8(opSize >> 24)
	size := int(opSize & 0xffffff)
	if size < 0 || size > maxPipeWireMessageSize {
		return 0, 0, nil, errors.New("pipewire message exceeds size limit")
	}
	payload = make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, 0, nil, err
	}
	return id, opcode, payload, nil
}

func appendSPAInt(buf []byte, v int32) []byte {
	var pod [16]byte
	binary.NativeEndian.PutUint32(pod[0:4], 4)
	binary.NativeEndian.PutUint32(pod[4:8], spaTypeInt)
	binary.NativeEndian.PutUint32(pod[8:12], uint32(v))
	return append(buf, pod[:]...)
}

func appendSPAString(buf []byte, s string) []byte {
	rawSize := len(s) + 1 // null-terminated C string
	padded := spaPad8(rawSize)
	pod := make([]byte, 8+padded)
	binary.NativeEndian.PutUint32(pod[0:4], uint32(rawSize))
	binary.NativeEndian.PutUint32(pod[4:8], spaTypeString)
	copy(pod[8:], s)
	return append(buf, pod...)
}

func encodeSPAStruct(body []byte) []byte {
	padded := spaPad8(len(body))
	pod := make([]byte, 8+padded)
	binary.NativeEndian.PutUint32(pod[0:4], uint32(len(body)))
	binary.NativeEndian.PutUint32(pod[4:8], spaTypeStruct)
	copy(pod[8:], body)
	return pod
}

// spaReader walks sequential SPA POD values inside a struct body.
type spaReader struct {
	b []byte
}

func openSPAStruct(payload []byte) (spaReader, bool) {
	if len(payload) < 8 {
		return spaReader{}, false
	}
	size := int(binary.NativeEndian.Uint32(payload[0:4]))
	typ := binary.NativeEndian.Uint32(payload[4:8])
	if typ != spaTypeStruct || size < 0 || 8+size > len(payload) {
		return spaReader{}, false
	}
	return spaReader{b: payload[8 : 8+size]}, true
}

func (r *spaReader) nextPod() (typ uint32, body []byte, ok bool) {
	if len(r.b) < 8 {
		return 0, nil, false
	}
	size := int(binary.NativeEndian.Uint32(r.b[0:4]))
	typ = binary.NativeEndian.Uint32(r.b[4:8])
	if size < 0 || 8+size > len(r.b) {
		return 0, nil, false
	}
	body = r.b[8 : 8+size]
	step := 8 + spaPad8(size)
	if step > len(r.b) {
		r.b = nil
	} else {
		r.b = r.b[step:]
	}
	return typ, body, true
}

func (r *spaReader) readInt() (int32, bool) {
	typ, body, ok := r.nextPod()
	if !ok || typ != spaTypeInt || len(body) < 4 {
		return 0, false
	}
	return int32(binary.NativeEndian.Uint32(body[0:4])), true
}

func (r *spaReader) readString() (string, bool) {
	typ, body, ok := r.nextPod()
	if !ok || typ != spaTypeString {
		return "", false
	}
	if n := len(body); n > 0 && body[n-1] == 0 {
		body = body[:n-1]
	}
	return string(body), true
}

func (r *spaReader) readStruct() (spaReader, bool) {
	typ, body, ok := r.nextPod()
	if !ok || typ != spaTypeStruct {
		return spaReader{}, false
	}
	return spaReader{b: body}, true
}

func parsePipeWireCoreDone(payload []byte) (id, seq int32, ok bool) {
	sr, ok := openSPAStruct(payload)
	if !ok {
		return 0, 0, false
	}
	id, ok = sr.readInt()
	if !ok {
		return 0, 0, false
	}
	seq, ok = sr.readInt()
	return id, seq, ok
}

func parsePipeWireCoreError(payload []byte) (res int32, ok bool) {
	sr, ok := openSPAStruct(payload)
	if !ok {
		return 0, false
	}
	if _, ok := sr.readInt(); !ok { // id
		return 0, false
	}
	if _, ok := sr.readInt(); !ok { // seq
		return 0, false
	}
	return sr.readInt() // res
}

func parsePipeWireRegistryGlobal(payload []byte) (perms uint32, ifaceType string, props map[string]string, ok bool) {
	sr, ok := openSPAStruct(payload)
	if !ok {
		return 0, "", nil, false
	}
	if _, ok := sr.readInt(); !ok { // global id
		return 0, "", nil, false
	}
	p, ok := sr.readInt()
	if !ok {
		return 0, "", nil, false
	}
	ifaceType, ok = sr.readString()
	if !ok {
		return 0, "", nil, false
	}
	if _, ok := sr.readInt(); !ok { // version
		return 0, "", nil, false
	}
	dict, ok := sr.readStruct()
	if !ok {
		return uint32(p), ifaceType, nil, true
	}
	nItems, ok := dict.readInt()
	if !ok || nItems < 0 {
		return uint32(p), ifaceType, nil, true
	}
	props = make(map[string]string, min(int(nItems), 64))
	for range nItems {
		k, ok := dict.readString()
		if !ok {
			break
		}
		v, ok := dict.readString()
		if !ok {
			break
		}
		props[k] = v
	}
	return uint32(p), ifaceType, props, true
}
