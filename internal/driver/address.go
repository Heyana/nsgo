package driver

// PageAddress accompanies ItemHistory without changing its navigation depth.
type PageAddress struct {
	URL  string
	Push bool
	// Set distinguishes an explicit application write from the address
	// carried passively while acknowledging a navigation depth.
	Set bool
}

// PageAddressChange is EventOpenLinks' browser traversal payload. Depth is
// the navigation depth of the landed entry; an empty Reason updates quietly.
type PageAddressChange struct {
	URL, Reason string
	Depth       int
}

// ResolvePageAddress is the browser's URL parser/serializer when linked.
// Native and headless graphs use core's URL implementation instead.
var ResolvePageAddress func(raw, page, base string) (string, error)
