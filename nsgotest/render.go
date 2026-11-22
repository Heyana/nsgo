package nsgotest

import "errors"

// ErrNoPlayer means this target has no windowless canvas player.
var ErrNoPlayer = errors.New("nsgotest: no windowless canvas player on this target")
