//go:build ios || tvos || android

package base_test

import (
	"testing"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }
