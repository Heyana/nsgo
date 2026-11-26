package main

import "github.com/nsgo-dev/nsgo/examples/gallery/locales"

// groupRegistration is one group, added by the file that declares its
// demos. Each group's file carries a build tag, so a build named with
// gallery_group and gallery_<group> carries that group alone.
type groupRegistration struct {
	tab   string
	order int
	name  string
	title func(locales.Messages) string
	demos []demo
}

var groupRegistrations []groupRegistration

func registerGroup(tab string, order int, name string, title func(locales.Messages) string, demos []demo) {
	groupRegistrations = append(groupRegistrations, groupRegistration{tab, order, name, title, demos})
}
