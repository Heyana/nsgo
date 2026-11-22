package driver

import "testing"

func TestAuthRedirectURIEscapesAndNormalizesPaths(t *testing.T) {
	for _, tc := range []struct {
		request AuthRequest
		want    string
	}{
		{AuthRequest{Scheme: "com.example.app", Path: "/oauth"}, "com.example.app:/oauth"},
		{AuthRequest{Scheme: "COM.Example.App", Path: "/before/../登录/%2F"}, "com.example.app:/%E7%99%BB%E5%BD%95/%252F"},
		{AuthRequest{Scheme: "HTTPS", Host: "example.com", Path: "/a/../oauth"}, "https://example.com/oauth"},
	} {
		if got := AuthRedirectURI(tc.request); got != tc.want {
			t.Errorf("AuthRedirectURI(%+v) = %q, want %q", tc.request, got, tc.want)
		}
	}
}
