//go:build !nsgodev

package nsgo

// devApp is app: only nsgo dev's builds connect to its server.
func devApp(app App) App { return app }
