package main

import (
	"fmt"
	"image"
	"path"
	"strings"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/pri"
)

const msixAppDisplayName = "ms-resource:AppDisplayName"

// Windows' visual asset scales, with names independent of a user's DPI.
var msixLogoScales = []int{100, 125, 150, 200, 400}

func msixResourceFiles(p *project, img image.Image) ([]msixFile, error) {
	names, err := msixResourceNames(p)
	if err != nil {
		return nil, err
	}
	var candidates []pri.Candidate
	for _, lang := range names.languages() {
		candidates = append(candidates, pri.Candidate{Language: lang, Value: names.names[lang]})
	}
	resources := []pri.Resource{{Name: "Resources/AppDisplayName", Candidates: candidates}}
	var files []msixFile
	for _, logo := range msixLogos {
		// Keep a 100% physical fallback at the manifest's unqualified path.
		files = append(files, msixFile{"Assets/" + logo.name, scaledPNG(img, logo.size)})
		r := pri.Resource{Name: "Files/Assets/" + logo.name, Path: true}
		for _, scale := range msixLogoScales {
			name := strings.TrimSuffix(logo.name, path.Ext(logo.name)) + fmt.Sprintf(".scale-%d.png", scale)
			file := "Assets/" + name
			files = append(files, msixFile{file, scaledPNG(img, (logo.size*scale+99)/100)})
			r.Candidates = append(r.Candidates, pri.Candidate{Scale: scale, Value: strings.ReplaceAll(file, "/", `\`)})
		}
		resources = append(resources, r)
	}
	index, err := pri.Write(msixIdentityName(p), names.source, resources)
	if err != nil {
		return nil, err
	}
	return append(files, msixFile{"resources.pri", index}), nil
}

func msixIdentityName(p *project) string {
	if p.Windows.Store.Identity != "" {
		return p.Windows.Store.Identity
	}
	name := strings.Trim(msixIdentityPattern.ReplaceAllString(p.ID, "-"), ".-")
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}

// Every declared app locale has a name candidate, including locales whose untranslated appName falls back to the source catalog.
func msixResourceNames(p *project) (*appNames, error) {
	if p.I18n.Dir == "" || !hasCatalogs(p.I18n.Dir) {
		return &appNames{source: "en-US", names: map[string]string{"en-US": p.Name}}, nil
	}
	c, ds := loadCatalog(p.I18n.Dir, p.I18n.Source)
	if err := ds.err(); err != nil {
		return nil, err
	}
	n, err := catalogAppNames(c)
	if err != nil {
		return nil, err
	}
	if n == nil {
		n = &appNames{source: c.source, names: map[string]string{c.source: p.Name}}
	}
	for lang := range c.files {
		if _, ok := n.names[lang]; !ok {
			n.names[lang] = n.names[n.source]
		}
	}
	return n, nil
}
