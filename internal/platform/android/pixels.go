//go:build android || nsgoandroidhost

package android

import (
	"image"
	"log"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// pixelFrame belongs to an item of a mount batch. Canvas frames have the
// key their draw operations use. An image view has only one frame.
type pixelFrame struct {
	item   int
	key    string
	pixels *image.RGBA
}

// trackCanvasPixels names the frames each drawing uses and sends its new
// copies once. Java keeps the bitmaps until the corresponding canvas stops
// using them, apart from the loader and cache for encoded images.
func (v *treeView) trackCanvasPixels(b *driver.MountBatch, frames *[]pixelFrame) map[int][]string {
	var canvases map[int][]string
	for i, it := range b.Items {
		switch it.Kind {
		case driver.ItemCreate, driver.ItemDelete:
			delete(v.canvasPixels, it.ID)
		case driver.ItemCanvas:
			var current map[string]*image.RGBA
			if it.Drawing != nil {
				for _, op := range it.Drawing.Ops {
					if op.Image == nil || op.Image.Pixels == nil {
						continue
					}
					px, key := op.Image.Pixels, op.Image.Key()
					if current[key] != nil {
						continue
					}
					if current == nil {
						current = map[string]*image.RGBA{}
						if canvases == nil {
							canvases = map[int][]string{}
						}
					}
					current[key] = px
					canvases[i] = append(canvases[i], key)
					if v.canvasPixels[it.ID][key] != px {
						*frames = append(*frames, pixelFrame{i, key, px})
					}
				}
			}
			if len(current) == 0 {
				delete(v.canvasPixels, it.ID)
			} else {
				if v.canvasPixels == nil {
					v.canvasPixels = map[driver.NodeID]map[string]*image.RGBA{}
				}
				v.canvasPixels[it.ID] = current
			}
		}
	}
	return canvases
}

// sendBatch sends frames ahead of the batch that applies them on the UI
// thread. Each frame has its own size limit. Other batches must wait
// till these frames are consumed, while unrelated pipe messages can pass.
func (s *surface) sendBatch(frames []pixelFrame, body func(*writer)) error {
	s.mountMu.Lock()
	defer s.mountMu.Unlock()
	if len(frames) == 0 {
		return s.send(msgBatch, body)
	}
	var batch writer
	body(&batch)
	if err := checkMessage(msgBatch, len(batch.buf)); err != nil {
		return err
	}
	for _, frame := range frames {
		px := frame.pixels
		var header writer
		header.u32(uint32(frame.item))
		header.str(frame.key)
		header.u32(uint32(px.Rect.Dx()))
		header.u32(uint32(px.Rect.Dy()))
		if err := checkMessage(msgPixelFrame, len(header.buf)+len(px.Pix)); err != nil {
			log.Printf("nsgo: pixel frame: %v", err)
			continue
		}
		if err := s.sendParts(msgPixelFrame, header.buf, px.Pix); err != nil {
			return err
		}
	}
	return s.sendParts(msgBatch, nil, batch.buf)
}
