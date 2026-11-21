package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"sort"
	"strconv"
)

// SetsInfo reports whether apple.info has key, even with an empty value, which writePlist writes in place of the package's own.
func (p *project) SetsInfo(key string) bool {
	_, ok := p.Apple.Info[key]
	return ok
}

// InfoValue writes the types apple.info can express in an Info.plist.
// Keep strings as strings: YAML booleans and numbers have their own
// native types. Reject unsupported values rather than stringify them.
func (*project) InfoValue(value any) (string, error) {
	var out bytes.Buffer
	var write func(any) error
	write = func(v any) error {
		switch v := v.(type) {
		case string:
			out.WriteString("<string>")
			if err := xml.EscapeText(&out, []byte(v)); err != nil {
				return err
			}
			out.WriteString("</string>")
		case bool:
			if v {
				out.WriteString("<true/>")
			} else {
				out.WriteString("<false/>")
			}
		case int:
			fmt.Fprintf(&out, "<integer>%d</integer>", v)
		case int64:
			fmt.Fprintf(&out, "<integer>%d</integer>", v)
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("apple.info: real values must be finite")
			}
			out.WriteString("<real>" + strconv.FormatFloat(v, 'g', -1, 64) + "</real>")
		case map[string]any:
			out.WriteString("<dict>")
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				out.WriteString("<key>")
				if err := xml.EscapeText(&out, []byte(key)); err != nil {
					return err
				}
				out.WriteString("</key>")
				if err := write(v[key]); err != nil {
					return err
				}
			}
			out.WriteString("</dict>")
		case []string:
			out.WriteString("<array>")
			for _, s := range v {
				if err := write(s); err != nil {
					return err
				}
			}
			out.WriteString("</array>")
		case []any:
			out.WriteString("<array>")
			for _, s := range v {
				if err := write(s); err != nil {
					return err
				}
			}
			out.WriteString("</array>")
		default:
			return fmt.Errorf("apple.info: unsupported property list value %T", v)
		}
		return nil
	}
	err := write(value)
	return out.String(), err
}
