//go:build android

package android

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/oneo-me/nsgo/core"
)

// autofillHints are Android's autofill hints of each AutoComplete, as
// NSH's: "" turns autofill off and "-" leaves it to the system.
var autofillHints = map[core.AutoComplete]string{
	core.AutoCompleteDefault: "-", core.AutoCompleteOff: "",
	core.AutoCompleteUsername: "username", core.AutoCompletePassword: "password",
	core.AutoCompleteNewPassword: "newPassword", core.AutoCompleteOneTimeCode: "smsOTPCode",
	core.AutoCompleteEmail: "emailAddress", core.AutoCompleteTel: "phone", core.AutoCompleteName: "personName",
	core.AutoCompleteGivenName: "personGivenName", core.AutoCompleteFamilyName: "personFamilyName",
	core.AutoCompleteMiddleName: "personMiddleName", core.AutoCompleteNickname: "nickname",
	core.AutoCompleteOrganization: "organization", core.AutoCompleteStreetAddress: "postalAddress",
	core.AutoCompleteAddressLine1: "streetAddress", core.AutoCompleteAddressLine2: "extendedAddress",
	core.AutoCompleteCity: "addressLocality", core.AutoCompleteRegion: "addressRegion",
	core.AutoCompletePostalCode: "postalCode", core.AutoCompleteCountry: "addressCountry",
	core.AutoCompleteCreditCardNumber: "creditCardNumber", core.AutoCompleteCreditCardExpiration: "creditCardExpirationDate",
	core.AutoCompleteCreditCardSecurityCode: "creditCardSecurityCode", core.AutoCompleteCreditCardName: "personName",
	core.AutoCompleteURL: "url", core.AutoCompleteBirthdate: "birthDateFull",
}

// writeEditing appends ItemInput's editing props, which the runtime's
// InputField reads after the accessory: multiline, maxLength, what Return
// does (InputConfig.Submit), the return key, capitalization, flags (no
// autocorrection, no caret), the autofill hint, the placeholder and
// selection colors (RGBA, 0 for the platform's) and the PasteBehavior.
func writeEditing(w *writer, in *core.InputConfig) {
	w.flags(in.Multiline)
	w.u32(uint32(max(in.MaxLength, 0)))
	w.byte(byte(in.Submit()))
	w.byte(byte(in.ReturnKey))
	w.byte(byte(in.AutoCapitalize))
	w.flags(in.NoAutoCorrect, in.HideCaret)
	hint, ok := autofillHints[in.AutoComplete]
	if !ok {
		hint = "-"
	}
	w.str(hint)
	w.u32(uint32(in.PlaceholderColor))
	w.u32(uint32(in.SelectionColor))
	w.byte(byte(in.PasteBehavior))
}

// readPaste reads the paste an EventPaste carries in its data: lines of
// "uri\t" and a file: URL, or "file\t", a MIME type, a tab and the
// bytes in base64.
func readPaste(text, data string) *core.PasteEvent {
	p := &core.PasteEvent{Text: text}
	for _, line := range strings.Split(data, "\n") {
		kind, rest, _ := strings.Cut(line, "\t")
		switch kind {
		case "uri":
			p.URIs = append(p.URIs, rest)
		case "file":
			mime, b64, _ := strings.Cut(rest, "\t")
			if b, err := base64.StdEncoding.DecodeString(b64); err == nil {
				p.Files = append(p.Files, core.PastedFile{MIME: mime, Data: b})
			}
		}
	}
	return p
}

// readTouches reads the fingers of an EventTouch the runtime sends: each
// "id x y changed", in points, joined by ";".
func readTouches(s string) []core.Touch {
	var out []core.Touch
	for _, f := range strings.Split(s, ";") {
		p := strings.Fields(f)
		if len(p) != 4 {
			continue
		}
		id, _ := strconv.Atoi(p[0])
		x, _ := strconv.ParseFloat(p[1], 32)
		y, _ := strconv.ParseFloat(p[2], 32)
		out = append(out, core.Touch{ID: id, PagePosition: core.Point{X: core.Points(x), Y: core.Points(y)}, Changed: p[3] == "1"})
	}
	return out
}
