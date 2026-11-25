//go:build !nsgoautomation && !nsgotest && !nsgocheck

package core

import (
	"context"
	"encoding/json"
	"errors"
)

type automationToolRegistry struct{}

// RegisterAutomationTool is automation's registration, inactive in this build.
func RegisterAutomationTool(*Scope, AutomationTool) error { return nil }

// CallAutomationTool is automation's call, unavailable in this build.
func CallAutomationTool(*Scope, context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, errors.ErrUnsupported
}

// DecodeAutomationResult is automation's result decoder, unavailable in this build.
func DecodeAutomationResult(json.RawMessage) (AutomationResult, bool, error) {
	return AutomationResult{}, false, errors.ErrUnsupported
}
