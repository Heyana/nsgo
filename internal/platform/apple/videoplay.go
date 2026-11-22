//go:build darwin

package apple

import "github.com/nsgo-dev/nsgo/internal/platform/videoplay"

const videoPauseBackground = videoplay.Background

type videoPlayback = videoplay.State
