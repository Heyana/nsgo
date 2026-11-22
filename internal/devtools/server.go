//go:build nsgodev

package devtools

import (
	"os"
	"sync"
)

// ServerToken is the run's token for nsgo dev's server, which the dev client sends with every request (Fetch). Token fills it.
var ServerToken = ""

// tokenEnv carries the run's token to the app: the tool sets it for the apps it starts, the simulator's through SIMCTL_CHILD_, and for targets that pass the app no environment, Android's, iPhones' and mini programs', the build adds a file to the app's main package that sets it as main initializes, after this package has, so that no command line, which every user of the machine may read, carries it.
const tokenEnv = "NSGO_DEV_TOKEN"

var takeToken sync.Once

func init() {
	// An app started with the token takes it out of its environment at once, so the programs it starts never inherit it.
	if _, ok := os.LookupEnv(tokenEnv); ok {
		Token()
	}
}

// Token returns ServerToken, which it takes the first time from NSGO_DEV_TOKEN, removing it from the environment, or in a web page from the page (pageToken).
func Token() string {
	takeToken.Do(func() {
		if t := os.Getenv(tokenEnv); t != "" {
			ServerToken = t
		}
		os.Unsetenv(tokenEnv)
		if ServerToken == "" {
			ServerToken = pageToken()
		}
	})
	return ServerToken
}
