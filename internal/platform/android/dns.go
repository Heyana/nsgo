//go:build (android || nsgoandroidhost) && (nsgo_android_dns || nsgodev || nsgotest)

package android

import (
	"context"
	"errors"
	"net"
)

// useDNS sends the app's DNS queries to the servers the runtime reports. Android has no resolv.conf, so Go without cgo would ask localhost. Linked only into builds that use Go's net package (cmd/nsgo adds nsgo_android_dns when the app imports net).
func useDNS() {
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			servers := dnsServers.Load()
			if servers == nil || len(*servers) == 0 {
				return nil, errors.New("android: no DNS servers")
			}
			// Go tries its default servers in turn, 127.0.0.1 first: map that one to our first and the rest to our second.
			i := 0
			if address != "127.0.0.1:53" {
				i = 1
			}
			var d net.Dialer
			return d.DialContext(ctx, network, net.JoinHostPort((*servers)[i%len(*servers)], "53"))
		},
	}
}
