package apk

import "encoding/xml"

const (
	networkSecurityName = "network_security_config"
	networkSecurityFile = "res/xml/" + networkSecurityName + ".xml"
	cleartextAttribute  = "cleartextTrafficPermitted"
)

func (m Manifest) networkPolicy() bool { return m.Cleartext || m.LoopbackCleartext }

// networkSecurityConfig allows HTTP to all hosts when opted in, otherwise only the exact loopback names Android clients pass, including both IPv6 forms.
func networkSecurityConfig(cleartext bool) *node {
	base := &node{name: xml.Name{Local: "base-config"}}
	setAttr(base, "", cleartextAttribute, "false")
	if cleartext {
		setAttr(base, "", cleartextAttribute, "true")
		return &node{name: xml.Name{Local: "network-security-config"}, kids: []*node{base}}
	}
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
