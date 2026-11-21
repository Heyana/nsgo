package driver

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// LocalService is a DNS-SD instance on the local network.
type LocalService struct {
	// Name is the instance label and Type is a service type such as _example._tcp.
	Name, Type string
	// Host is the resolved DNS hostname or IP address. It is set when browsing.
	Host string
	// Port is the listening port in host byte order.
	Port int
	// TXT holds the service's attributes. Keys are case-insensitive.
	TXT map[string]string
}

// DiscoveryOp selects a local service discovery operation.
type DiscoveryOp uint8

const (
	DiscoveryAdvertise DiscoveryOp = iota + 1
	DiscoveryBrowse
	DiscoveryStop
)

// DiscoveryRequest starts or stops an operation identified by the item's Count.
type DiscoveryRequest struct {
	Op      DiscoveryOp
	Service LocalService
}

// DiscoveryReply reports a resolved service, its removal, or an operation error.
type DiscoveryReply struct {
	Service LocalService
	Removed bool
	Error   string
}

// DiscoveryTXT encodes DNS-SD's length-prefixed TXT strings in key order.
func DiscoveryTXT(records map[string]string) ([]byte, error) {
	var data []byte
	seen := make(map[string]bool, len(records))
	for _, key := range slices.Sorted(maps.Keys(records)) {
		if !discoveryTXTKey(key) || seen[strings.ToLower(key)] {
			return nil, fmt.Errorf("invalid or repeated TXT key %q", key)
		}
		seen[strings.ToLower(key)] = true
		value := records[key]
		size := len(key) + 1 + len(value)
		if size > 255 || len(data)+1+size > 65535 {
			return nil, fmt.Errorf("TXT records exceed DNS-SD's size limit")
		}
		data = append(data, byte(size))
		data = append(data, key...)
		data = append(data, '=')
		data = append(data, value...)
	}
	return data, nil
}

// ParseDiscoveryTXT reads DNS-SD attributes, preserving the first value for each key.
func ParseDiscoveryTXT(data []byte) map[string]string {
	var records map[string]string
	for len(data) != 0 {
		size := int(data[0])
		data = data[1:]
		if len(data) < size {
			break
		}
		key, value, _ := strings.Cut(string(data[:size]), "=")
		data = data[size:]
		if !discoveryTXTKey(key) {
			continue
		}
		key = strings.ToLower(key)
		if _, seen := records[key]; !seen {
			if records == nil {
				records = map[string]string{}
			}
			records[key] = value
		}
	}
	return records
}

func discoveryTXTKey(key string) bool {
	if key == "" {
		return false
	}
	for i := range len(key) {
		if key[i] < 0x20 || key[i] > 0x7e || key[i] == '=' {
			return false
		}
	}
	return true
}
