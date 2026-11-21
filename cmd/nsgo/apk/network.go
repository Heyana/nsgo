package apk

import "encoding/xml"

const (
	networkSecurityName = "network_security_config"
	networkSecurityFile = "res/xml/" + networkSecurityName + ".xml"
	cleartextAttribute  = "cleartextTrafficPermitted"
)

// networkSecurityConfig allows HTTP for the exact loopback host names that
// Android clients pass to the policy, including both forms of the IPv6 literal.
func networkSecurityConfig() *node {
	base := &node{name: xml.Name{Local: "base-config"}}
	setAttr(base, "", cleartextAttribute, "false")
	loopback := &node{name: xml.Name{Local: "domain-config"}}
	setAttr(loopback, "", cleartextAttribute, "true")
	for _, host := range []string{"localhost", "127.0.0.1", "::1", "[::1]"} {
		loopback.kids = append(loopback.kids, &node{name: xml.Name{Local: "domain"}, text: host})
	}
	return &node{
		name: xml.Name{Local: "network-security-config"},
		kids: []*node{base, loopback},
	}
}
