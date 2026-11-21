//go:build darwin || ios

package apple

import "errors"

// IntersectRects returns the overlap of a and b, which has no size when
// neither extends inside the other.
func IntersectRects(a, b CGRect) CGRect {
	left, top := max(a.Origin.X, b.Origin.X), max(a.Origin.Y, b.Origin.Y)
	right := min(a.Origin.X+a.Size.Width, b.Origin.X+b.Size.Width)
	bottom := min(a.Origin.Y+a.Size.Height, b.Origin.Y+b.Size.Height)
	return Rect(left, top, max(0, right-left), max(0, bottom-top))
}

// CroppedPNG encodes a region of an already composited CGImage in sRGB.
func CroppedPNG(image uintptr, region CGRect) ([]byte, error) {
	if image == 0 {
		return nil, errors.New("apple: capture produced no image")
	}
	w, h := ImageSize(image)
	region = IntersectRects(region, Rect(0, 0, float64(w), float64(h)))
	crop := cgImageCreateWithImageInRect(image, region)
	defer Release(crop)
	if crop == 0 {
		return nil, errors.New("apple: capture has no pixels in its bounds")
	}
	w, h = ImageSize(crop)
	bitmapsLoaded.Do(loadBitmaps)
	const premultipliedFirst, byteOrder32Little = 2, 2 << 12
	ctx := cgBitmapContextCreate(0, uintptr(w), uintptr(h), 8, uintptr(w*4), srgb, premultipliedFirst|byteOrder32Little)
	if ctx == 0 {
		return nil, errors.New("apple: cannot make an sRGB capture")
	}
	defer Release(ctx)
	cgContextDrawImage(ctx, Rect(0, 0, float64(w), float64(h)), crop)
	bitmap := cgBitmapContextCreateImage(ctx)
	defer Release(bitmap)
	return PNGImage(bitmap)
}
