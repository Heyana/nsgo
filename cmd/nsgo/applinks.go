package main

import (
	"fmt"
	"net"
	"strings"
)

// validateAppLinks normalizes explicit DNS hosts shared by iOS universal links and Android App Links. A URL's path, scheme and port belong in neither platform's association host declaration.
func (p *project) validateAppLinks() error {
	seen := map[string]bool{}
	for i, raw := range p.AppLinks {
		domain := strings.ToLower(raw)
		if strings.HasPrefix(domain, "*.") || !domainPattern.MatchString(domain) || len(domain) > 253 || net.ParseIP(domain) != nil {
			return fmt.Errorf("app_links[%d] %q is not an explicit DNS host; write example.com without a scheme, path, port or wildcard", i, raw)
		}
		for _, label := range strings.Split(domain, ".") {
			if len(label) > 63 {
				return fmt.Errorf("app_links[%d] %q has a DNS label longer than 63 bytes", i, raw)
			}
		}
		if seen[domain] {
			return fmt.Errorf("app_links[%d]: the host %q is listed twice", i, domain)
		}
		seen[domain] = true
		p.AppLinks[i] = domain
	}
	return nil
}
