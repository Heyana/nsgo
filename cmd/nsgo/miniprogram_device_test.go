package main

import (
	"fmt"
	"testing"

	"github.com/nsgo-dev/goja"
)

// TestMiniProgramDeviceModel exercises the actual runtime's modern and
// legacy host APIs, including models that already contain their brand.
func TestMiniProgramDeviceModel(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, tc := range []struct{ brand, model, want string }{
			{"Google", "Pixel 8", "Google Pixel 8"},
			{"Apple", "iPhone 15 Pro", "Apple iPhone 15 Pro"},
			{"HUAWEI", "HUAWEI Mate 80", "HUAWEI Mate 80"},
			{" Samsung ", " samsung SM-S9280 ", "samsung SM-S9280"},
			{"", "Phone", "Phone"},
			{"Google", "", ""},
			{"", "", ""},
		} {
			t.Run(fmt.Sprintf("legacy=%v/%s/%s", legacy, tc.brand, tc.model), func(t *testing.T) {
				vm := goja.New()
				vm.Set("module", vm.NewObject())
				vm.Set("require", func(string) *goja.Object { return vm.NewObject() })
				src, err := miniprogramRuntime.ReadFile("miniprogram/common/nsgo/runtime.js")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := vm.RunString(string(src)); err != nil {
					t.Fatal(err)
				}
				api := fmt.Sprintf(`(() => {
const info = {brand: %q, model: %q, windowWidth: 300, windowHeight: 500, screenHeight: 500};
const api = {getSystemInfoSync: () => info};
if (!%v) {
  api.getWindowInfo = () => info;
  api.getAppBaseInfo = () => info;
  api.getDeviceInfo = () => info;
}
return module.exports.device(api).deviceModel;
})()`, tc.brand, tc.model, legacy)
				got, err := vm.RunString(api)
				if err != nil {
					t.Fatal(err)
				}
				if got.String() != tc.want {
					t.Errorf("device model = %q, want %q", got.String(), tc.want)
				}
			})
		}
	}
}
