// Package mimeinfo holds the fixed extension table shared by file pickers and packaging. It never reads the build host's MIME database.
package mimeinfo

import (
	"slices"
	"strings"
)

var imageExtensions = map[string]string{
	"jpg": "image/jpeg", "jpeg": "image/jpeg", "jpe": "image/jpeg", "png": "image/png", "gif": "image/gif",
	"webp": "image/webp", "heic": "image/heic", "heif": "image/heif", "avif": "image/avif",
	"tif": "image/tiff", "tiff": "image/tiff", "bmp": "image/bmp",
}

var fileTypes = map[string]string{
	"7z": "application/x-7z-compressed", "aac": "audio/aac", "apk": "application/vnd.android.package-archive",
	"avi": "video/x-msvideo", "bz2": "application/x-bzip2", "css": "text/css", "csv": "text/csv", "doc": "application/msword",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"epub": "application/epub+zip", "flac": "audio/flac", "flv": "video/x-flv", "gz": "application/gzip", "htm": "text/html",
	"html": "text/html", "ics": "text/calendar", "js": "text/javascript", "json": "application/json",
	"key": "application/x-iwork-keynote-sffkey", "log": "text/plain", "m4a": "audio/mp4", "m4v": "video/mp4",
	"md": "text/markdown", "mjs": "text/javascript", "mkv": "video/x-matroska", "mov": "video/quicktime", "mp3": "audio/mpeg",
	"mp4": "video/mp4", "numbers": "application/x-iwork-numbers-sffnumbers", "odp": "application/vnd.oasis.opendocument.presentation",
	"ods": "application/vnd.oasis.opendocument.spreadsheet", "odt": "application/vnd.oasis.opendocument.text",
	"oga": "audio/ogg", "ogg": "audio/ogg", "ogv": "video/ogg", "opus": "audio/opus", "otf": "font/otf",
	"pages": "application/x-iwork-pages-sffpages", "pdf": "application/pdf", "ppt": "application/vnd.ms-powerpoint",
	"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation", "rar": "application/vnd.rar",
	"rtf": "application/rtf", "svg": "image/svg+xml", "tar": "application/x-tar", "tgz": "application/gzip",
	"ttf": "font/ttf", "txt": "text/plain", "wasm": "application/wasm", "wav": "audio/wav", "weba": "audio/webm", "webm": "video/webm",
	"woff": "font/woff", "woff2": "font/woff2", "xls": "application/vnd.ms-excel",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xml": "application/xml",
	"xz": "application/x-xz", "yaml": "application/yaml", "yml": "application/yaml", "zip": "application/zip",
	"3gp": "video/3gpp",
}

// ImageForExtension returns a raster image's type, or "".
func ImageForExtension(ext string) string { return imageExtensions[strings.ToLower(ext)] }

// ForExtension returns the built-in type of ext, without a dot, or "".
func ForExtension(ext string) string {
	ext = strings.ToLower(ext)
	if m := imageExtensions[ext]; m != "" {
		return m
	}
	return fileTypes[ext]
}

// Extensions returns known extensions of a type or family, as image/*.
func Extensions(media string) []string {
	var extensions []string
	family, wild := strings.CutSuffix(media, "/*")
	for _, table := range []map[string]string{imageExtensions, fileTypes} {
		for ext, m := range table {
			if m == media || wild && strings.HasPrefix(m, family+"/") {
				extensions = append(extensions, ext)
			}
		}
	}
	slices.Sort(extensions)
	return extensions
}
