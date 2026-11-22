package library

import _ "embed"

// Keep the prebuilt library's notices when apps vendor this package.
//
//go:embed LICENSE
var _ string
