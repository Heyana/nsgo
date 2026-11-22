//go:build ios || android

package chat

// Mobile component tests need the platform host as well as TestMain: otherwise simulator library tests can run headless and Android refuses to start them.
import _ "github.com/nsgo-dev/nsgo"
