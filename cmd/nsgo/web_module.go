package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"
)

type wasmEncoding struct {
	name, extension string
	compress        func(io.Writer) io.WriteCloser
}

// Prefer Brotli when the browser accepts both at the same weight.
var wasmEncodings = [...]wasmEncoding{
	{"br", ".br", func(w io.Writer) io.WriteCloser { return brotli.NewWriter(w) }},
	{"gzip", ".gz", func(w io.Writer) io.WriteCloser { return gzip.NewWriter(w) }},
}

// writeWebWasmCopies leaves smaller representations beside the original
// module for servers that negotiate content encoding.
func writeWebWasmCopies(path string, data []byte) error {
	for _, encoding := range wasmEncodings {
		var out bytes.Buffer
		writer := encoding.compress(&out)
		_, err := writer.Write(data)
		if err := errors.Join(err, writer.Close()); err != nil {
			return err
		}
		if out.Len() < len(data) {
			if err := os.WriteFile(path+encoding.extension, out.Bytes(), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// webFiles serves the module's encoded representation at its original URL
// and leaves other files to http.FileServer.
func webFiles(dir string) http.Handler {
	root := http.Dir(dir)
	files := http.FileServer(root)
	var config webApp
	if data, err := os.ReadFile(dir + "/nsgo.json"); err == nil {
		_ = json.Unmarshal(data, &config)
	}
	base := config.Base
	if parsed, err := url.Parse(base); err == nil {
		base = parsed.Path
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if base != "" && base != "/" && strings.HasPrefix(r.URL.Path, base) {
			clone := r.Clone(r.Context())
			u := *r.URL
			u.Path, u.RawPath = "/"+strings.TrimPrefix(r.URL.Path, base), ""
			clone.URL = &u
			r = clone
		}
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && strings.Contains(r.Header.Get("Accept"), "text/html") {
			f, err := root.Open(r.URL.Path)
			if err == nil {
				f.Close()
			} else if os.IsNotExist(err) {
				f, err = root.Open("index.html")
				if err == nil {
					defer f.Close()
					if stat, err := f.Stat(); err == nil {
						w.Header().Set("Content-Type", "text/html; charset=utf-8")
						http.ServeContent(w, r, "index.html", stat.ModTime(), f)
						return
					}
				}
			}
		}
		if strings.HasSuffix(r.URL.Path, ".wasm") && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			w.Header().Add("Vary", "Accept-Encoding")
			if serveEncodedWasm(w, r, root) {
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}

func serveEncodedWasm(w http.ResponseWriter, r *http.Request, root http.Dir) bool {
	weights := encodingWeights(strings.Join(r.Header.Values("Accept-Encoding"), ","))
	weight := func(name string) float64 {
		if q, ok := weights[name]; ok {
			return q
		}
		return weights["*"]
	}
	choices := wasmEncodings
	if weight(choices[1].name) > weight(choices[0].name) {
		choices[0], choices[1] = choices[1], choices[0]
	}
	for _, encoding := range choices {
		if q := weight(encoding.name); q <= 0 || q < weights["identity"] {
			continue
		}
		file, err := root.Open(r.URL.Path + encoding.extension)
		if err != nil {
			continue
		}
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			file.Close()
			continue
		}
		w.Header().Set("Content-Type", "application/wasm")
		w.Header().Set("Content-Encoding", encoding.name)
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
		http.ServeContent(w, r, r.URL.Path, info.ModTime(), file)
		file.Close()
		return true
	}
	return false
}

func encodingWeights(header string) map[string]float64 {
	weights := map[string]float64{}
	for value := range strings.SplitSeq(header, ",") {
		name, parameter, _ := strings.Cut(value, ";")
		name = strings.ToLower(strings.TrimSpace(name))
		q := float64(1)
		if parameter = strings.TrimSpace(parameter); parameter != "" {
			value, _ := strings.CutPrefix(parameter, "q=")
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			q = 0
			if err == nil && parsed >= 0 && parsed <= 1 {
				q = parsed
			}
		}
		weights[name] = q
	}
	return weights
}
