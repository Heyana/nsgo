package driver

// MiniProgramCredential is EventMiniProgramSignIn's short-lived code. The
// application exchanges it on its server, using Platform's own login API.
type MiniProgramCredential struct {
	Platform, Code string
}
