package targets

import (
	"slices"
	"testing"
)

// Expand replaces the groups with the targets they mean and refuses a name the tool does not know (#2259).
func TestExpand(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want []string
	}{
		{[]string{"all"}, GoAndXcode()},
		{[]string{"tui"}, []string{"tui_macos_arm64", "tui_macos_x64", "tui_windows_arm64", "tui_windows_x64", "tui_linux_arm64", "tui_linux_x64"}},
		{[]string{"miniprogram"}, []string{"miniprogram_wechat", "miniprogram_qq", "miniprogram_alipay", "miniprogram_douyin"}},
		{[]string{"windows"}, []string{"windows_arm64", "windows_x64"}},
		{[]string{"android"}, []string{"android_arm64"}},
		{[]string{"macos"}, []string{"macos_arm64", "macos_x64"}},
		{[]string{"linux"}, []string{"linux_arm64", "linux_x64"}},
		{[]string{"web", "web"}, []string{"web"}},
	} {
		got, err := Expand(tc.in)
		if err != nil {
			t.Fatalf("Expand(%v) = %v", tc.in, err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("Expand(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
	if _, err := Expand([]string{"nonesuch"}); err == nil {
		t.Fatal("Expand accepted a name the tool does not know")
	}
	names := Names()
	names[0] = "changed"
	if Names()[0] == "changed" {
		t.Fatal("Names returned the table itself")
	}
}
