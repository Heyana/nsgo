package driver

import (
	"bytes"
	"math"
	"path"
	"strings"
	"time"
)

// The system's photo picker, as NSH's picker module: the user picks
// photos, which reach the app as files. The platform shows its picker,
// prepares each image as the request says (PickRequest.Encoding) and
// answers with EventPick; a cancelled picker answers with no files.

// MaxPicked is the most files one pick answers with.
const MaxPicked = 100

// DefaultPickQuality is the JPEG quality images are encoded at when the
// request gives none.
const DefaultPickQuality = 0.9

// PickRequest is a pick as the platform carries it out.
type PickRequest struct {
	// Limit is the most images the user picks: 1 for a single pick, at
	// most MaxPicked.
	Limit int
	// MaxDimension scales images whose longer side has more pixels down
	// to it, encoding them again; 0 keeps their size.
	MaxDimension int
	// Quality is the JPEG quality images are encoded at, from 0 to 1;
	// 0 means DefaultPickQuality and keeps JPEGs that need no other
	// change as they are.
	Quality float32
	// Format says which formats images come in.
	Format PickFormat
}

// PickFormat says which formats picked images come in.
type PickFormat uint8

const (
	// PickCompatible keeps JPEG, PNG, GIF and WebP as they are and turns
	// any other format, such as HEIC or TIFF, into JPEG.
	PickCompatible PickFormat = iota
	// PickCurrent keeps the format the image is stored in, unless it is
	// encoded again to scale it or for a quality.
	PickCurrent
)

// PickedFile is an image the user picked.
type PickedFile struct {
	// Name has an extension that matches MIME (IMG_0042.jpg).
	Name string
	// MIME is its media type (image/jpeg).
	MIME string
	Data []byte
	// Size is its size in pixels as it shows, its orientation applied.
	Size Size
	// Modified is when it last changed, zero when the platform does not
	// say.
	Modified time.Time
}

// PickEncoding is what a platform does with one picked image.
type PickEncoding uint8

const (
	// PickKeep keeps its bytes as they are.
	PickKeep PickEncoding = iota
	// PickPNG decodes it, its orientation applied, scales it to fit
	// MaxDimension and encodes it as a PNG: an image that was a PNG.
	PickPNG
	// PickJPEG does the same and encodes a JPEG at the request's quality.
	PickJPEG
)

// JPEGQuality returns the quality JPEGs are encoded at, from 0 to 1.
func (r PickRequest) JPEGQuality() float32 {
	if r.Quality <= 0 {
		return DefaultPickQuality
	}
	return min(r.Quality, 1)
}

// Encoding returns what to do with an image of type mime whose pixels
// are size, its orientation applied.
func (r PickRequest) Encoding(mime string, size Size) PickEncoding {
	mime = strings.ToLower(mime)
	longer := max(size.Width, size.Height)
	scale := r.MaxDimension > 0 && longer > Points(r.MaxDimension)
	foreign := mime != "image/jpeg" && mime != "image/png" && mime != "image/gif" && mime != "image/webp"
	switch {
	case mime == "image/png" && scale:
		return PickPNG
	case scale, r.Format == PickCompatible && foreign, r.Quality > 0 && mime == "image/jpeg":
		return PickJPEG
	}
	return PickKeep
}

// ScaledSize returns the size an image of size pixels is scaled to: its
// longer side at most MaxDimension, its aspect ratio kept.
func (r PickRequest) ScaledSize(size Size) Size {
	w, h := max(size.Width, 1), max(size.Height, 1)
	longer := max(w, h)
	if r.MaxDimension <= 0 || longer <= Points(r.MaxDimension) {
		return size
	}
	fit := func(side Points) Points {
		return max(Points(math.Round(float64(side)*float64(r.MaxDimension)/float64(longer))), 1)
	}
	return Size{Width: fit(w), Height: fit(h)}
}

// DialogExtensions returns the file extensions a desktop's open dialog
// offers for r when the backend reads files as they are: only the
// formats every platform shows unless r keeps the current format.
func (r PickRequest) DialogExtensions() []string {
	if r.Format == PickCompatible {
		return []string{"jpg", "jpeg", "jpe", "png", "gif", "webp"}
	}
	return []string{"jpg", "jpeg", "jpe", "png", "gif", "webp", "heic", "heif", "avif", "tif", "tiff", "bmp"}
}

// FinishPick returns files as the app gets them: at most r.Limit, each
// with a type, a name matching it and, when size reads it, a size.
func FinishPick(r PickRequest, files []PickedFile, size func(data []byte) (Size, error)) []PickedFile {
	if n := max(r.Limit, 1); len(files) > n {
		files = files[:n]
	}
	for i := range files {
		f := &files[i]
		if f.MIME == "" {
			f.MIME = SniffImage(f.Data)
		}
		if f.MIME == "" {
			f.MIME = MIMEForName(f.Name)
		}
		if f.MIME == "" {
			f.MIME = "application/octet-stream"
		}
		if (f.Size.Width <= 0 || f.Size.Height <= 0) && size != nil {
			if s, err := size(f.Data); err == nil {
				f.Size = s
			}
		}
		f.Name = PickedName(f.Name, f.MIME)
	}
	return files
}

// SniffImage returns the media type an image's bytes start with, or ""
// for a format it does not know.
func SniffImage(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(b, []byte("GIF8")):
		return "image/gif"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp"
	case len(b) >= 12 && string(b[4:8]) == "ftyp":
		switch string(b[8:12]) {
		case "avif", "avis":
			return "image/avif"
		case "heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1":
			return "image/heic"
		}
	case bytes.HasPrefix(b, []byte("II*\x00")), bytes.HasPrefix(b, []byte("MM\x00*")):
		return "image/tiff"
	case bytes.HasPrefix(b, []byte("BM")):
		return "image/bmp"
	}
	return ""
}

var imageExtensions = map[string]string{
	"jpg": "image/jpeg", "jpeg": "image/jpeg", "jpe": "image/jpeg", "png": "image/png", "gif": "image/gif",
	"webp": "image/webp", "heic": "image/heic", "heif": "image/heif", "avif": "image/avif",
	"tif": "image/tiff", "tiff": "image/tiff", "bmp": "image/bmp",
}

// MIMEForName returns the image type a file name's extension says, or
// "".
func MIMEForName(name string) string {
	return imageExtensions[strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))]
}

// ImageExtension returns the extension of an image of type mime, or "".
func ImageExtension(mime string) string {
	switch m := strings.ToLower(mime); m {
	case "image/jpeg":
		return "jpg"
	case "image/png", "image/gif", "image/webp", "image/heic", "image/heif", "image/avif", "image/tiff", "image/bmp":
		return m[len("image/"):]
	}
	return ""
}

// PickedName returns name without its directories and with the
// extension of mime (IMG_0042.HEIC as a JPEG is IMG_0042.jpg); a name
// without a stem becomes image.
func PickedName(name, mime string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	ext := ImageExtension(mime)
	if ext == "" {
		if name == "" {
			return "file"
		}
		return name
	}
	if strings.EqualFold(MIMEForName(name), mime) {
		return name
	}
	stem := name
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 && (MIMEForName(name) != "" || len(name)-dot-1 <= 4) {
		stem = name[:dot]
	}
	if stem == "" {
		stem = "image"
	}
	return stem + "." + ext
}
