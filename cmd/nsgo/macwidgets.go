package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// signMacStoreWidgets provisions the extension's sandbox for the same shared group and team as the containing Mac App Store app. The home directory exception used by ad hoc widgets grants no access here.
func signMacStoreWidgets(ctx context.Context, p *project, app, team string) error {
	data, err := os.ReadFile(p.Sign.WidgetsProfile)
	if err != nil {
		return err
	}
	profile, err := readProfile(ctx, data)
	if err != nil {
		return fmt.Errorf("sign.widgets_profile: %w", err)
	}
	granted, _ := profile["Entitlements"].(map[string]any)
	id := p.ID + ".widgets"
	appID, _ := granted["com.apple.application-identifier"].(string)
	if !profileAllows(appID, id) {
		return fmt.Errorf("sign.widgets_profile is for %s, not %s", appID, id)
	}
	widgetTeam, _ := granted["com.apple.developer.team-identifier"].(string)
	if widgetTeam == "" || widgetTeam != team {
		return fmt.Errorf("sign.widgets_profile must belong to the app profile's team %s", team)
	}
	groups, _ := granted["com.apple.security.application-groups"].([]any)
	if !slices.Contains(groups, any(p.appGroup())) {
		return fmt.Errorf("sign.widgets_profile does not grant app group %s", p.appGroup())
	}
	identity, err := profileIdentity(ctx, profile)
	if err != nil {
		return err
	}
	ents, err := writeEntitlements(map[string]any{
		"com.apple.security.app-sandbox":        true,
		"com.apple.application-identifier":      signedAppID(appID, id),
		"com.apple.developer.team-identifier":   team,
		"com.apple.security.application-groups": []any{p.appGroup()},
	})
	if err != nil {
		return err
	}
	defer os.Remove(ents)
	appex := macWidgetsAppex(app)
	if err := os.WriteFile(filepath.Join(appex, "Contents", "embedded.provisionprofile"), data, 0o644); err != nil {
		return err
	}
	return run(ctx, "codesign", "--force", "--sign", identity, "--entitlements", ents, "--generate-entitlement-der", appex)
}
