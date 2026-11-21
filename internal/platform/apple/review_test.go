//go:build darwin && !ios

package apple

import "testing"

func TestStoreKitReviewControllerLoads(t *testing.T) {
	controller, command := storeReview()
	if !ReviewAvailable() || controller == 0 || command != sel("requestReview") {
		t.Fatal("StoreKit did not expose its macOS requestReview method")
	}
}
