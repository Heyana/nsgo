package main

import (
	"fmt"
	"slices"
)

const associatedDomainsEntitlement = "com.apple.developer.associated-domains"

// appLinkDomains are full-app universal link entitlements. App Clips keep their separate appclips: domains and never inherit these claims.
func (p *project) appLinkDomains() []any {
	if p.clipParent != "" {
		return nil
	}
	var domains []any
	for _, host := range p.AppLinks {
		domains = append(domains, "applinks:"+host)
	}
	return domains
}

// claimAppLinks restricts profile-provided Associated Domains to the full app's explicit declarations. Xcode-managed builds claim these same values when requesting the automatically provisioned profile.
func (p *project) claimAppLinks(entitlements, granted map[string]any) error {
	domains := p.appLinkDomains()
	if len(domains) == 0 {
		return nil
	}
	allowed, _ := granted[associatedDomainsEntitlement].([]any)
	for _, domain := range domains {
		if !slices.Contains(allowed, any("*")) && !slices.Contains(allowed, domain) {
			return fmt.Errorf("the provisioning profile does not grant Associated Domains for %s; enable the capability for %s and renew the profile", domain, p.ID)
		}
	}
	entitlements[associatedDomainsEntitlement] = domains
	return nil
}
