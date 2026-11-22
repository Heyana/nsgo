package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// appNameKey is the message of the app's catalogs that names the app in
// each language, which packages list it under.
const appNameKey = "appName"

// appNames are the app's name in the languages of its catalogs.
type appNames struct {
	source string            // the source language
	names  map[string]string // by language, the source's included
}

// appNames reads the app's names from its catalogs (nsgo.yaml's i18n):
// their appName message, which says no argument. It returns nil when the
// catalogs have none.
func (p *project) appNames() (*appNames, error) {
	if p.I18n.Dir == "" || !hasCatalogs(p.I18n.Dir) {
		return nil, nil
	}
	c, ds := loadCatalog(p.I18n.Dir, p.I18n.Source)
	if err := ds.err(); err != nil {
		return nil, err
	}
	return catalogAppNames(c)
}

// catalogAppNames reads only explicit appName translations from a parsed catalog.
func catalogAppNames(c *catalog) (*appNames, error) {
	src := c.files[c.source]
	if src == nil || src.msgs[appNameKey] == nil {
		return nil, nil
	}
	if len(c.params[appNameKey]) > 0 {
		return nil, fmt.Errorf("%s: %s names the app, so it takes no arguments", rel(src.path), appNameKey)
	}
	n := &appNames{source: c.source, names: map[string]string{}}
	for lang, f := range c.files {
		if m := f.msgs[appNameKey]; m != nil {
			n.names[lang] = constantText(m.parts)
		}
	}
	return n, nil
}

// languages returns the languages the app is named in, the source's
// first.
func (n *appNames) languages() []string {
	langs := []string{n.source}
	for lang := range n.names {
		if lang != n.source {
			langs = append(langs, lang)
		}
	}
	slices.Sort(langs[1:])
	return langs
}

// writeAppleNames writes an InfoPlist.strings per language into the .lproj directories of an Apple bundle's resource directory, with the app's names and its localized apple.info strings, and returns the Info.plist entries of the source language.
func writeAppleNames(p *project, resources string) (string, error) {
	localizations, err := p.appleLocalizations()
	if err != nil {
		return "", err
	}
	n, err := p.appNames()
	if err != nil {
		return "", err
	}
	info, err := p.appleInfoStrings()
	if err != nil {
		return "", err
	}
	if n == nil && info == nil {
		return localizations, nil
	}
	langs := map[string]bool{}
	if n != nil {
		for lang := range n.names {
			langs[lang] = true
		}
	}
	if info != nil {
		for lang := range info.texts {
			langs[lang] = true
		}
	}
	for lang := range langs {
		var strs strings.Builder
		named := false
		if n != nil {
			if name, ok := n.names[lang]; ok {
				name := appleString(name)
				fmt.Fprintf(&strs, "\"CFBundleDisplayName\" = %s;\n\"CFBundleName\" = %s;\n", name, name)
				named = true
			}
		}
		if info != nil {
			texts := info.texts[lang]
			for _, key := range slices.Sorted(maps.Keys(texts)) {
				// appName's entries name the app; a message of the same key would contradict them.
				if named && (key == "CFBundleDisplayName" || key == "CFBundleName") {
					continue
				}
				fmt.Fprintf(&strs, "%s = %s;\n", appleString(key), appleString(texts[key]))
			}
		}
		dir := filepath.Join(resources, lang+".lproj")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, "InfoPlist.strings"), []byte(strs.String()), 0o644); err != nil {
			return "", err
		}
	}
	if n == nil {
		// Without it the system falls back to English's .lproj, which an app written in another language lacks.
		return localizations + fmt.Sprintf("<key>CFBundleDevelopmentRegion</key><string>%s</string>", xmlEscape(info.source)), nil
	}
	return localizations + fmt.Sprintf("<key>CFBundleDevelopmentRegion</key><string>%s</string>\n\t<key>CFBundleDisplayName</key><string>%s</string>",
		xmlEscape(n.source), xmlEscape(n.names[n.source])), nil
}

// appleInfo is the text of the string apple.info entries the app's catalogs have a message of the same key for.
type appleInfo struct {
	source string                       // the source language
	texts  map[string]map[string]string // by language, then key
}

// appleInfoStrings reads apple.info's localized strings from the app's catalogs: each string entry whose key is a message of theirs, which says no argument, in every language of the catalogs, the source's message in a language that does not translate it, as the catalogs' functions say it. It returns nil when no entry has a message. Other entries keep only their Info.plist value.
func (p *project) appleInfoStrings() (*appleInfo, error) {
	if len(p.Apple.Info) == 0 || p.I18n.Dir == "" || !hasCatalogs(p.I18n.Dir) {
		return nil, nil
	}
	c, ds := loadCatalog(p.I18n.Dir, p.I18n.Source)
	if err := ds.err(); err != nil {
		return nil, err
	}
	src := c.files[c.source]
	info := &appleInfo{source: c.source, texts: map[string]map[string]string{}}
	for _, key := range slices.Sorted(maps.Keys(p.Apple.Info)) {
		if _, ok := p.Apple.Info[key].(string); !ok || src.msgs[key] == nil {
			continue
		}
		if len(c.params[key]) > 0 {
			return nil, fmt.Errorf("%s: %s is apple.info's, so it takes no arguments", rel(src.path), key)
		}
		for lang, f := range c.files {
			m := f.msgs[key]
			if m == nil {
				m = src.msgs[key]
			}
			if info.texts[lang] == nil {
				info.texts[lang] = map[string]string{}
			}
			info.texts[lang][key] = constantText(m.parts)
		}
	}
	if len(info.texts) == 0 {
		return nil, nil
	}
	return info, nil
}

// appleLocalizations returns the Info.plist entry listing the languages of
// the app's catalogs, the source's first, in which the system says what it
// adds to the app, such as AppKit's menu items and panels; "" without
// catalogs.
func (p *project) appleLocalizations() (string, error) {
	if p.I18n.Dir == "" || !hasCatalogs(p.I18n.Dir) {
		return "", nil
	}
	c, ds := loadCatalog(p.I18n.Dir, p.I18n.Source)
	if err := ds.err(); err != nil {
		return "", err
	}
	langs := make([]string, 0, len(c.files))
	for lang := range c.files {
		if lang != c.source {
			langs = append(langs, lang)
		}
	}
	slices.Sort(langs)
	var b strings.Builder
	b.WriteString("<key>CFBundleLocalizations</key><array>")
	for _, lang := range append([]string{c.source}, langs...) {
		b.WriteString("<string>" + xmlEscape(lang) + "</string>")
	}
	b.WriteString("</array>\n\t")
	return b.String(), nil
}

// appleString quotes s for a .strings file.
func appleString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// desktopLocale is a language's locale in a desktop entry's keys, as
// Name[pt_BR]: a language and its region, Chinese's scripts by their
// usual regions.
func desktopLocale(lang string) string {
	switch lang {
	case "zh-Hans":
		return "zh_CN"
	case "zh-Hant":
		return "zh_TW"
	}
	parts := strings.Split(lang, "-")
	if len(parts) > 1 && len(parts[len(parts)-1]) == 2 {
		return parts[0] + "_" + parts[len(parts)-1]
	}
	return parts[0]
}

// android returns the names by language for an APK's table, the source's
// also as the default; nil without names.
func (n *appNames) android() map[string]string {
	if n == nil {
		return nil
	}
	out := map[string]string{"": n.names[n.source]}
	for lang, name := range n.names {
		out[lang] = name
	}
	return out
}
