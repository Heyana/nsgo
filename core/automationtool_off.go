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

// CallAutomationToolAsync is automation's non-waiting call, unavailable in this build.
func CallAutomationToolAsync(_ *Scope, _ context.Context, _ string, _ json.RawMessage, done func(json.RawMessage, error)) {
	done(nil, errors.ErrUnsupported)
}
