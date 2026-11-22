//go:build ios

package ios

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func (*surface) OpenAudioOutput(c driver.AudioOutputConfig) (driver.AudioOutput, error) {
	if onTV() {
		return nil, driver.ErrAudioOutputUnsupported
	}
	return apple.OpenAudioOutput(c)
}
