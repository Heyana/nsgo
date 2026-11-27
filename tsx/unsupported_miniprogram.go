//go:build nsgominiprogram

package tsx

// targetUnsupported is true on mini programs, which evaluate no dynamic scripts
// and cannot fit goja in their 2 MB package cap.
const targetUnsupported = true
