//go:build js && wasm

package canvas2d

import (
	"encoding/binary"
	"math"
	"sync"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	atlasOnce sync.Once
	atlasFn   js.Value
)

// atlasPlayer loops in the JS host so the Go/JS boundary costs one call per atlas. Hosts that disallow generated functions use the same drawing operations through syscall/js. It is made on first use, with no package initializer, so an app that draws no canvas does not link this package (TestMinimalAppLinks).
func atlasPlayer() js.Value {
	atlasOnce.Do(func() { atlasFn = newAtlasPlayer() })
	return atlasFn
}

func newAtlasPlayer() (fn js.Value) {
	defer func() {
		if recover() != nil {
			fn = js.Undefined()
		}
	}()
	return js.Global().Get("Function").New("ctx", "img", "bytes", "density", `
const sprites = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
const alpha = ctx.globalAlpha;
for (let i = 0; i < bytes.byteLength; i += 36) {
	const sx = sprites.getFloat32(i, true) * density, sy = sprites.getFloat32(i + 4, true) * density;
	const w = sprites.getFloat32(i + 8, true), h = sprites.getFloat32(i + 12, true);
	const c = sprites.getFloat32(i + 16, true), s = sprites.getFloat32(i + 20, true);
	const x = sprites.getFloat32(i + 24, true), y = sprites.getFloat32(i + 28, true);
	ctx.globalAlpha = alpha * sprites.getFloat32(i + 32, true);
	if (s === 0 && c > 0) {
		ctx.drawImage(img, sx, sy, w * density, h * density, x, y, w * c, h * c);
	} else {
		ctx.save();
		ctx.transform(c, s, -s, c, x, y);
		ctx.drawImage(img, sx, sy, w * density, h * density, 0, 0, w, h);
		ctx.restore();
	}
}
ctx.globalAlpha = alpha;
`)
}

func (p *Player) atlas(op *driver.DrawOp) {
	imgSrc := p.Drawing.Image(op)
	sprites := p.Drawing.AtlasSprites(op)
	if imgSrc == nil || len(sprites) == 0 {
		return
	}
	img, density, ok := p.Image(*imgSrc, false)
	if !ok {
		return
	}
	p.Ctx.Set("imageSmoothingEnabled", op.Sampling != driver.ImageSamplingNearest)
	if draw := atlasPlayer(); draw.Type() == js.TypeFunction {
		data := make([]byte, 0, len(sprites)*36)
		for _, sprite := range sprites {
			b, t := sprite.Rect, sprite.Transform
			for _, v := range [9]float32{float32(b.Origin.X), float32(b.Origin.Y), float32(b.Size.Width), float32(b.Size.Height),
				t.SCos, t.SSin, t.TX, t.TY, sprite.Alpha} {
				data = binary.LittleEndian.AppendUint32(data, math.Float32bits(v))
			}
		}
		packet := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(packet, data)
		draw.Invoke(p.Ctx, img, packet, density)
		return
	}
	ctx := p.Ctx
	alpha := ctx.Get("globalAlpha").Float()
	for _, sprite := range sprites {
		b, t := sprite.Rect, sprite.Transform
		w, h := float64(b.Size.Width), float64(b.Size.Height)
		x, y := float64(b.Origin.X)*density, float64(b.Origin.Y)*density
		ctx.Set("globalAlpha", alpha*float64(sprite.Alpha))
		if t.SSin == 0 && t.SCos > 0 {
			ctx.Call("drawImage", img, x, y, w*density, h*density, t.TX, t.TY, w*float64(t.SCos), h*float64(t.SCos))
			continue
		}
		ctx.Call("save")
		ctx.Call("transform", t.SCos, t.SSin, -t.SSin, t.SCos, t.TX, t.TY)
		ctx.Call("drawImage", img, x, y, w*density, h*density, 0, 0, w, h)
		ctx.Call("restore")
	}
	ctx.Set("globalAlpha", alpha)
}
