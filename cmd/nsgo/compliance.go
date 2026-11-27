package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

func appleStoreTarget(target string) bool {
	switch target {
	case "macos_arm64", "macos_x64", "macos_universal", "ios", "tvos", "watchos":
		return true
	}
	return false
}

func (p *project) checkStoreCompliance(targets []string) error {
	if p.buildEmbed {
		return nil
	}

	channel := p.channel()
	if notice := androidTargetNotice(p, targets); notice != "" {
		if channel == channelStore {
			return errors.New(notice)
		}
		warn("%s", notice)
	}
	if channel == channelStore && slices.ContainsFunc(targets, appleStoreTarget) {
		if err := p.checkExportCompliance(); err != nil {
			return err
		}
		remindChecklist(p.Compliance.AppStore.AgeRating, "app_store.age_rating", "complete the age rating questionnaire in App Store Connect")
	}
	if channel == channelStore && slices.ContainsFunc(targets, androidTarget) {
		remindChecklist(p.Compliance.GooglePlay.DataSafety, "google_play.data_safety", "complete the Data safety form in Play Console")
	}
	if channel == channelSteam {
		remindChecklist(p.Compliance.Steam.ContentSurvey, "steam.content_survey", "complete the Content Survey in Steamworks")
	}
	return nil
}

func (p *project) checkExportCompliance() error {
	const declaration = "ITSAppUsesNonExemptEncryption"
	const approvalCode = "ITSEncryptionExportComplianceCode"
	if v, exists := p.Apple.Info[declaration]; exists {
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("apple.info.%s must be a boolean (true or false)", declaration)
		}
	}
	if v, exists := p.Apple.Info[approvalCode]; exists {
		if code, ok := v.(string); !ok || strings.TrimSpace(code) == "" {
			return fmt.Errorf("apple.info.%s must be a nonempty string with the code from App Store Connect", approvalCode)
		}
	}
	return nil
}

func remindChecklist(complete *bool, key, instruction string) {
	if complete != nil && !*complete {
		warn("compliance.%s: %s", key, instruction)
	}
}
