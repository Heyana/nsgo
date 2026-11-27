// Pixel bytes stay in the logic layer. A WeChat image view's component
// gives us its canvas node so each version can update the same bitmap.
'use strict'

const frames = new Map()
const canvases = new Map()

function buffer(id, length) {
  let frame = frames.get(id)
  if (!frame) frames.set(id, frame = {})
  if (!frame.bytes || frame.bytes.length !== length) frame.bytes = new Uint8Array(length)
  return frame.bytes
}

function show(id, width, height, tint) {
  const frame = frames.get(id)
  frame.width = width
  frame.height = height
  frame.tint = Array.isArray(tint) && tint.length === 4 ? tint : null
  const target = canvases.get(id)
  if (target) paint(target, frame)
}

function attach(id, canvas) {
  const ctx = canvas.getContext('2d')
  if (!ctx || !ctx.createImageData || !ctx.putImageData) return false
  const target = { canvas, ctx }
  canvases.set(id, target)
  const frame = frames.get(id)
  if (frame) paint(target, frame)
  return true
}

function detach(id, canvas) {
  const target = canvases.get(id)
  if (target && target.canvas === canvas) canvases.delete(id)
}

function release(id) {
  frames.delete(id)
}

function clear() {
  frames.clear()
  canvases.clear()
}

// An offscreen copy lets a Canvas draw the same pixels as a source image.
function offscreen(api, bytes, width, height) {
  const canvas = api.createOffscreenCanvas({ type: '2d', width, height })
  const ctx = canvas.getContext('2d')
  return paint({ canvas, ctx }, { bytes, width, height }) ? canvas : null
}

function paint(target, frame) {
  const { canvas, ctx } = target
  const { bytes, width, height } = frame
  if (!ctx || !ctx.createImageData || !ctx.putImageData || !width || !height) return false
  if (!target.data || target.data.width !== width || target.data.height !== height) {
    canvas.width = width
    canvas.height = height
    target.data = ctx.createImageData(width, height)
  }
  const rgba = target.data.data
  rgba.set(bytes)
  if (frame.tint) {
    const [r, g, b, a] = frame.tint
    for (let i = 0; i < rgba.length; i += 4) {
      rgba[i] = r
      rgba[i + 1] = g
      rgba[i + 2] = b
      rgba[i + 3] = Math.floor((rgba[i + 3] * a + 127) / 255)
    }
  } else {
    // ImageData takes straight alpha. Go's image.RGBA is premultiplied.
    for (let i = 0; i < rgba.length; i += 4) {
      const alpha = rgba[i + 3]
      if (alpha === 0 || alpha === 255) continue
      for (let c = 0; c < 3; c++) rgba[i + c] = Math.min(Math.floor(rgba[i + c] * 255 / alpha), 255)
    }
  }
  ctx.putImageData(target.data, 0, 0)
  return true
}

module.exports = { buffer, show, attach, detach, release, clear, offscreen }
