//go:build darwin && !ios

package macos

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func (*surface) OpenAudioOutput(c driver.AudioOutputConfig) (driver.AudioOutput, error) {
	return apple.OpenAudioOutput(c)
}
