// Package knownfolder reads Windows' known folders, for the backends that
// run there.
package knownfolder

// Path is SHGetKnownFolderPath's path of the FOLDERID id
// ("FDD39AD0-238F-46AF-ADB4-6C85480369C7"), which follows a folder the
// user or a policy moved, such as to OneDrive; "" when it has none, and
// off Windows.
func Path(id string) string { return path(id) }
