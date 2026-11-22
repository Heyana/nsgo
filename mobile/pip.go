package mobile

import "github.com/nsgo-dev/nsgo/core"

// PictureInPicture renders an independent NSGo tree into iOS Picture in
// Picture. The system window takes only play, pause and skip commands;
// other input is unavailable. Background suspension retains the last
// rendered frame. Ref starts and stops it; deleting its scope stops it.
// Unsupported platforms call OnState with Supported=false. Its layer
// renderer supports views, text, images, Canvas and Video frames exposed by AVFoundation. Web views, Metal shaders and backdrop effects do not render. Use the outer Ref to start the session and forward its playback callbacks to the Video when the two share playback state.
type PictureInPicture = core.PictureInPicture

// PictureInPictureState describes support, readiness, activity and errors.
type PictureInPictureState = core.PictureInPictureState
