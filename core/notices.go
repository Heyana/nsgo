package core

var bundledThirdPartyNotices string

// ThirdPartyNotices returns the license notices collected by nsgo build.
// It returns an empty string when the app was built without a notice file.
func ThirdPartyNotices() string { return bundledThirdPartyNotices }

// SetBundledThirdPartyNotices records the notices during initialization.
// Only the nsgo tool's generated main package calls it.
func SetBundledThirdPartyNotices(text string) { bundledThirdPartyNotices = text }
