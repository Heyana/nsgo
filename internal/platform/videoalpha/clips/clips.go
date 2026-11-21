// Package clips holds the tiny video files the backends' tests play. Only tests import it.
package clips

import _ "embed"

// VP8Alpha and VP9Alpha are 16 by 16 pixels of VP8 and VP9 with alpha in WebM (libvpx and libvpx-vp9 with yuva420p), and VP9 the same picture with no alpha.
var (
	//go:embed vp8a.webm
	VP8Alpha []byte
	//go:embed vp9a.webm
	VP9Alpha []byte
	//go:embed vp9.webm
	VP9 []byte
)

// HEVCAlpha is 64 by 64 pixels of HEVC with alpha in a QuickTime file (hevc_videotoolbox with alpha_quality), and HEVC the same picture, all blue, with no alpha.
var (
	//go:embed hevca.mov
	HEVCAlpha []byte
	//go:embed hevc.mov
	HEVC []byte
)

// PiP changes from blue to green at one second, using H.264 Baseline.
//
//go:embed pip.mov
var PiP []byte
