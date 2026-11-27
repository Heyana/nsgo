package main

import (
	"bytes"
	"fmt"
	"github.com/nsgo-dev/nsgo/cmd/nsgo/apk"
)

func (p *project) androidWidgets() ([]apk.Widget, error) {
	l, err := p.widgetLocalizations()
	if err != nil {
		return nil, err
	}
	text := func(value string) map[string]string {
		out := map[string]string{"": value}
		if resolved := l.resolve(value); resolved.key != "" {
			out[""] = l.texts[l.source][resolved.key]
			for lang, entries := range l.texts {
				out[lang] = entries[resolved.key]
			}
		}
		return out
	}
	var out []apk.Widget
	for _, w := range p.Widgets {
		names := map[string]string{"": w.Kind}
		if w.Name != "" {
			names = text(w.Name)
		}
		out = append(out, apk.Widget{Kind: w.Kind, Names: names, Descriptions: text(w.Description), Families: widgetKindFamilies(w)})
	}
	return out, nil
}

// androidWidgetRuntime refuses a package whose generated factory cannot load.
// Runtime binaries are refreshed separately from Java-source pull requests.
func (p *project) androidWidgetRuntime(classes []byte) error {
	if len(p.Widgets) > 0 && !bytes.Contains(classes, []byte("Ldev/oneo/nsgo/NsgoComponents;")) {
		return fmt.Errorf("widgets: the bundled Android runtime predates App Widgets; update NSGo to a version with the rebuilt Android runtime")
	}
	return nil
}
