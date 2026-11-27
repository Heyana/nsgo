//go:build !nsgodev

package core

type devKeepState struct{}

// DevKeep preserves sig's value across nsgo dev restarts under key as JSON.
// Outside nsgo dev builds it does nothing.
func DevKeep[T any](*Scope, string, *Signal[T]) {}

// DevKeepFunc preserves state across nsgo dev restarts under key as JSON.
// Outside nsgo dev builds it does nothing.
func DevKeepFunc[T any](*Scope, string, func() T, func(T)) {}

// DevKeepNav is base.NewNav's dev-restart stack persistence.
// Outside nsgo dev builds it does nothing.
func DevKeepNav[R any](*Scope, func() []R, func([]R)) {}
