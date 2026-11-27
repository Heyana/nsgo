package main

import "fmt"

// urlSchemesPlist is the Info.plist entry that registers the app's URL schemes.
func (p *project) urlSchemesPlist() string {
	if len(p.URLSchemes) == 0 {
		return ""
	}
	return fmt.Sprintf("\t<key>CFBundleURLTypes</key>\n\t<array>\n\t\t<dict>\n\t\t\t<key>CFBundleURLName</key><string>%s</string>\n\t\t\t<key>CFBundleURLSchemes</key>%s\n\t\t</dict>\n\t</array>",
		xmlText(p.ID), plistStrings(p.URLSchemes))
}

// iosURLSchemesPlist is urlSchemesPlist for sys's app: an iPhone's or iPad's registers the schemes, whose links its scene delegate hands to OnOpenLinks; an Apple TV's and an App Clip's, which takes no URL schemes, register none.
func (p *project) iosURLSchemesPlist(sys *appleOS) string {
	if sys != iOS || p.clipParent != "" {
		return ""
	}
	return p.urlSchemesPlist()
}
