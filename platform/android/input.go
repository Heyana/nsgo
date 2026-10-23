//go:build android

package android

import "github.com/oneo-me/nsgo/core"

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
// autocorrection, no caret), the autofill hint and the placeholder and
// selection colors (RGBA, 0 for the platform's).
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
}
