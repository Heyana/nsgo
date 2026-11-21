//go:build android

package android

import (
	"image"
	"log"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"
)

// Shaders draw with OpenGL ES 3.0 in the Java runtime's ShaderNode, a TextureView: Go translates each program into GLSL ES once and sends its source with the first item that draws it, and releases it once no view draws it (msgShaderRelease), with where each uniform lane lies in its std140 block; Java compiles it once for its context, writes the lanes, the animated ones each frame they change, and draws when they, the size or an image change. An image of pixels goes with the item, its bytes only when the core sent a new copy and the batch has room for them, and Java writes them into a texture the view keeps.

// glProgram is a program translated for Java.
type glProgram struct {
	key    *driver.ShaderProgram
	id     uint32
	source string // the fragment stage's GLSL ES, "" when it does not compile
	lanes  []int  // each lane's offset in the block
	types  []byte // and its type: a lane* constant
	size   int
	images int
	sent   bool
	views  int // the shader views drawing it, on every surface
}

// What a lane holds, as ShaderNode reads it: a 32-bit float, or a 32-bit
// integer from the value's integer part.
const (
	laneFloat byte = iota
	laneInt
	laneUint
	laneBool
)

// shaderItem is what an ItemShader sends: its program, whether the
// program's source goes with it, and which of its images' pixels do.
type shaderItem struct {
	prog   *glProgram
	source bool
	pixels []bool
}

// What an image of an ItemShader is, as ShaderNode.set reads it: a source Java loads by its key, pixels, or nothing.
const (
	shaderImageKey byte = iota
	shaderImagePixels
	shaderImageNone
)

// glMain is the fragment stage around the program's function: the
// position counts down from the top, as the program's does.
const glMain = `
uniform float nsgoHeight;
out vec4 nsgoColor;

void main() {
	vec2 p = vec2(gl_FragCoord.x, nsgoHeight - gl_FragCoord.y);
	nsgoColor = nsgoShader(vec4(p, gl_FragCoord.z, 1.0), p / U1);
}
`

// trackShaders translates the programs a batch's shader items draw and
// prepares the copies that fit. Only the latest item of a live shader
// can be retried, so a follow-up never restores superseded state.
func (s *surface) trackShaders(tv *treeView, b *driver.MountBatch) ([]shaderItem, []driver.MountItem) {
	var out []shaderItem
	var retry map[driver.NodeID]*driver.MountItem
	budget := maxShaderPixels
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case driver.ItemCreate, driver.ItemDelete:
			delete(tv.shaderPixels, it.ID)
			delete(retry, it.ID)
			s.drawWith(tv, it.ID, nil)
			continue
		case driver.ItemShader:
			delete(retry, it.ID)
			if it.Shader == nil {
				s.drawWith(tv, it.ID, nil)
				continue
			}
		default:
			continue
		}
		p := s.programs[it.Shader.Program]
		if p == nil {
			p = translate(it.Shader.Program)
			p.key = it.Shader.Program
			s.nextProgram++
			p.id = s.nextProgram
			if s.programs == nil {
				s.programs = map[*driver.ShaderProgram]*glProgram{}
			}
			s.programs[it.Shader.Program] = p
		}
		s.drawWith(tv, it.ID, p)
		// The core sends the same copy while the version stays, which Java has.
		pixels, held, left, deferred := shaderPixels(it.Shader.Images, tv.shaderPixels[it.ID], budget)
		budget = left
		if tv.shaderPixels == nil {
			tv.shaderPixels = map[driver.NodeID][]*image.RGBA{}
		}
		tv.shaderPixels[it.ID] = held
		out = append(out, shaderItem{prog: p, source: !p.sent, pixels: pixels})
		p.sent = true
		if deferred {
			if retry == nil {
				retry = map[driver.NodeID]*driver.MountItem{}
			}
			retry[it.ID] = it
		}
	}
	if len(retry) == 0 {
		return out, nil
	}
	pending := make([]driver.MountItem, 0, len(retry))
	for i := range b.Items {
		it := &b.Items[i]
		if retry[it.ID] == it {
			pending = append(pending, *it)
		}
	}
	return out, pending
}

// drawWith records that shader view id of tv draws p, nil for none. A
// program left without views waits for releasePrograms, so a later item
// of the batch can still draw it.
func (s *surface) drawWith(tv *treeView, id driver.NodeID, p *glProgram) {
	old := tv.shaders[id]
	if old == p {
		return
	}
	if p == nil {
		delete(tv.shaders, id)
	} else {
		p.views++
		if tv.shaders == nil {
			tv.shaders = map[driver.NodeID]*glProgram{}
		}
		tv.shaders[id] = p
	}
	if old != nil {
		old.views--
		if old.views == 0 {
			s.unusedPrograms = append(s.unusedPrograms, old)
		}
	}
}

// releasePrograms forgets the programs no shader view draws any more and
// has Java delete those it has, after the batches that stopped drawing them.
func (s *surface) releasePrograms() {
	var ids []uint32
	for _, p := range s.unusedPrograms {
		if p.views == 0 && s.programs[p.key] == p {
			delete(s.programs, p.key)
			if p.sent {
				ids = append(ids, p.id)
			}
		}
	}
	clear(s.unusedPrograms)
	s.unusedPrograms = s.unusedPrograms[:0]
	if len(ids) == 0 {
		return
	}
	s.send(msgShaderRelease, func(w *writer) {
		w.u32(uint32(len(ids)))
		for _, id := range ids {
			w.u32(id)
		}
	})
}

// translate turns prog into GLSL ES.
func translate(prog *driver.ShaderProgram) *glProgram {
	p := &glProgram{}
	if prog == nil {
		return p
	}
	ir := prog.IR
	p.source = "#version 300 es\nprecision highp float;\nprecision highp int;\n\n" +
		glsl.Fragment(ir, "nsgoShader", glsl.Options{FlipY: "true", Block: "NsgoUniforms"}) + glMain
	p.lanes, p.size = glsl.UniformLanes(ir)
	for _, t := range ir.LaneTypes() {
		k := laneFloat
		switch t {
		case shaderir.Int:
			k = laneInt
		case shaderir.Uint:
			k = laneUint
		case shaderir.Bool:
			k = laneBool
		}
		p.types = append(p.types, k)
	}
	p.images = ir.TextureCount
	return p
}

// writeShader writes an ItemShader as ShaderNode.set reads it.
func writeShader(w *writer, sh *driver.Shader, it shaderItem) {
	p := it.prog
	w.u32(p.id)
	w.byte(byte(sh.Role))
	w.byte(byte(sh.Pictures))
	w.flags(sh.Running, sh.Backwards)
	w.f64(sh.Overflow)
	w.flags(it.source)
	if it.source {
		w.str(p.source)
		w.u32(uint32(len(p.lanes)))
		for i, off := range p.lanes {
			w.u32(uint32(off))
			w.byte(p.types[i])
		}
		w.u32(uint32(p.size))
		w.u32(uint32(p.images))
	}
	w.u32(uint32(len(sh.Lanes)))
	for _, v := range sh.Lanes {
		w.f64(v)
	}
	w.u32(uint32(len(sh.Nodes)))
	for _, n := range sh.Nodes {
		w.u32(uint32(n.Lane))
		w.u32(n.Node)
		w.byte(byte(n.Channel))
	}
	w.u32(uint32(len(sh.Images)))
	for i, img := range sh.Images {
		px := img.Pixels
		if px == nil {
			w.byte(shaderImageKey)
			w.str(img.Key())
			continue
		}
		width, height := px.Rect.Dx(), px.Rect.Dy()
		n := 4 * width * height
		if n > maxShaderPixels {
			if i < len(it.pixels) && it.pixels[i] {
				log.Printf("nsgo: shader: an image of %d by %d pixels is over the %d MiB the Android runtime takes", width, height, maxShaderPixels>>20)
			}
			w.byte(shaderImageNone)
			continue
		}
		fresh := i < len(it.pixels) && it.pixels[i]
		w.byte(shaderImagePixels)
		w.u32(uint32(width))
		w.u32(uint32(height))
		w.flags(fresh)
		if fresh {
			// Premultiplied RGBA, top row first, as GLES takes it.
			w.buf = append(w.buf, px.Pix[:n]...)
		}
	}
}
