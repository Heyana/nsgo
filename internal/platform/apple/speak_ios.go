//go:build ios

package apple

import (
	"errors"

	"github.com/nsgo-dev/nsgo/internal/audiosession"
)

// holdSpeakSession holds the shared AVAudioSession on iOS, tvOS and watchOS at the level that plays spoken audio as long as an utterance speaks, pausing other apps' audio and heard with the Ring/Silent switch on silent, which internal/audiosession weighs against its other holders.
func holdSpeakSession() (leave func(), err error) {
	leave, err = audiosession.Hold(audiosession.Playback)
	if err != nil {
		return nil, errors.New("apple: cannot activate the audio session for speaking")
	}
	return leave, nil
}
