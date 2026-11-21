package android

import (
	"encoding/binary"
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
					if op.Kind != driver.DrawImage && op.Kind != driver.DrawVertices || op.Image == nil || op.Image.Pixels == nil {
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
// thread. Each frame has its own size limit. The batch and its frames share
// the pipe lock, and a large RGBA slice goes straight to the pipe.
func (s *surface) sendBatch(frames []pixelFrame, body func(*writer)) error {
	if len(frames) == 0 {
		return s.send(msgBatch, body)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	batch := writer{append(s.buf[:0], msgBatch, 0, 0, 0, 0)}
	body(&batch)
	if err := checkMessage(msgBatch, len(batch.buf)-5); err != nil {
		log.Printf("nsgo: mount: %v", err)
		return err
	}
	for _, frame := range frames {
		px := frame.pixels
		header := writer{buf: make([]byte, 5, 64)}
		header.buf[0] = msgPixelFrame
		header.u32(uint32(frame.item))
		header.str(frame.key)
		header.u32(uint32(px.Rect.Dx()))
		header.u32(uint32(px.Rect.Dy()))
		size := len(header.buf) - 5 + len(px.Pix)
		if err := checkMessage(msgPixelFrame, size); err != nil {
			log.Printf("nsgo: pixel frame: %v", err)
			continue
		}
		binary.BigEndian.PutUint32(header.buf[1:], uint32(size))
		if _, err := s.out.Write(header.buf); err != nil {
			return err
		}
		if _, err := s.out.Write(px.Pix); err != nil {
			return err
		}
	}
	binary.BigEndian.PutUint32(batch.buf[1:], uint32(len(batch.buf)-5))
	s.out.Write(batch.buf)
	if cap(batch.buf) <= 1<<20 {
		s.buf = batch.buf
	}
	return nil
}
