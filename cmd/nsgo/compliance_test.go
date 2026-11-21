package main

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestStoreChecklistReminders(t *testing.T) {
	var p project
	if err := yaml.Unmarshal([]byte(`
compliance:
  app_store: {age_rating: false}
  google_play: {data_safety: false}
  steam: {content_survey: false}
`), &p); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		channel, target, want string
	}{
		{channelStore, "ios", "app_store.age_rating"},
		{channelStore, "android_arm64", "google_play.data_safety"},
		{channelSteam, "linux_x64", "steam.content_survey"},
		{channelStore, "ios_simulator", ""},
		{channelStore, "windows_x64", ""},
		{channelDirect, "android_arm64", ""},
	} {
		p.Updates.Channel = tc.channel
		out := captureStderr(t, func() {
			if err := p.checkStoreCompliance([]string{tc.target}); err != nil {
				t.Error(err)
			}
		})
		if tc.want == "" && out != "" || tc.want != "" &&
			(!strings.Contains(out, "compliance."+tc.want) || strings.Count(out, "compliance.") != 1) {
			t.Errorf("%s %s: got %q, want reminder for %q", tc.channel, tc.target, out, tc.want)
		}
	}
}

func TestAppleExportComplianceDeclaration(t *testing.T) {
	const declaration = "ITSAppUsesNonExemptEncryption"
	const code = "ITSEncryptionExportComplianceCode"
	p := project{}
	p.Updates.Channel = channelStore
	for _, tc := range []struct {
		name string
		info map[string]any
		want string
	}{
		{name: "questionnaire at upload"},
		{name: "exempt", info: map[string]any{declaration: false}},
		{name: "review pending", info: map[string]any{declaration: true}},
		{name: "approved", info: map[string]any{declaration: true, code: "example"}},
		{name: "quoted boolean", info: map[string]any{declaration: "false"}, want: declaration},
		{name: "blank code", info: map[string]any{code: "  "}, want: code},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p.Apple.Info = tc.info
			err := p.checkStoreCompliance([]string{"macos_arm64"})
			if tc.want == "" && err != nil || tc.want != "" &&
				(err == nil || !strings.Contains(err.Error(), "apple.info."+tc.want)) {
				t.Errorf("checkStoreCompliance = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestStoreComplianceChecksUniversalAppleTarget(t *testing.T) {
	const declaration = "ITSAppUsesNonExemptEncryption"
	p := project{}
	p.Updates.Channel = channelStore
	p.Apple.Info = map[string]any{declaration: "false"}
	if err := p.checkStoreCompliance([]string{"macos_universal"}); err == nil || !strings.Contains(err.Error(), "apple.info."+declaration) {
		t.Fatalf("checkStoreCompliance(macos_universal) = %v, want the export declaration error", err)
	}
}
