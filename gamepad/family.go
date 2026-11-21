package gamepad

import (
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Family identifies a controller's physical button-label family so games can
// show matching on-screen prompts (Xbox A/B/X/Y, PlayStation Cross/Circle/Square/Triangle,
// Nintendo B/A/Y/X, or a generic fallback).
type Family uint8

const (
	FamilyGeneric     Family = Family(driver.GamepadFamilyGeneric)
	FamilyXbox        Family = Family(driver.GamepadFamilyXbox)
	FamilyPlayStation Family = Family(driver.GamepadFamilyPlayStation)
	FamilyNintendo    Family = Family(driver.GamepadFamilyNintendo)
)

// String returns the family's lowercase name: generic, xbox, playstation or nintendo.
func (f Family) String() string {
	return driver.GamepadFamily(f).String()
}

// Known USB Vendor IDs for first-party and common third-party game controllers.
const (
	vendorMicrosoft uint16 = 0x045E
	vendorSony      uint16 = 0x054C
	vendorNintendo  uint16 = 0x057E
	vendorLogitech  uint16 = 0x046D
	vendorPDP       uint16 = 0x0E6F
	vendorHarmonix  uint16 = 0x1BAD
	vendorPowerA    uint16 = 0x24C6
)

// DetectFamily classifies a controller from its USB vendor/product IDs and
// reported name or product category string. First-party USB vendor IDs take
// precedence, followed by controller name patterns, known third-party vendor
// IDs, and finally FamilyGeneric.
func DetectFamily(vendorID, productID uint16, name string) Family {
	switch vendorID {
	case vendorMicrosoft:
		return FamilyXbox
	case vendorSony:
		return FamilyPlayStation
	case vendorNintendo:
		return FamilyNintendo
	}

	if f := detectFamilyFromName(name); f != FamilyGeneric {
		return f
	}

	switch vendorID {
	case vendorPDP, vendorHarmonix, vendorPowerA:
		return FamilyXbox
	case vendorLogitech:
		// Logitech XInput gamepads (F310, F510, F710, ChillStream).
		switch productID {
		case 0xC21D, 0xC21E, 0xC21F, 0xC242:
			return FamilyXbox
		}
	}

	return FamilyGeneric
}

func detectFamilyFromName(name string) Family {
	if name == "" {
		return FamilyGeneric
	}
	lower := strings.ToLower(name)

	switch {
	case strings.Contains(lower, "dualsense"),
		strings.Contains(lower, "dualshock"),
		strings.Contains(lower, "playstation"),
		strings.Contains(lower, "sixaxis"),
		strings.Contains(lower, "ps5"),
		strings.Contains(lower, "ps4"),
		strings.Contains(lower, "ps3"),
		strings.Contains(lower, "sony"):
		return FamilyPlayStation

	case strings.Contains(lower, "xbox"),
		strings.Contains(lower, "x-box"),
		strings.Contains(lower, "xinput"),
		strings.Contains(lower, "x360"),
		strings.Contains(lower, "360 controller"),
		strings.Contains(lower, "microsoft"):
		return FamilyXbox

	case strings.Contains(lower, "nintendo"),
		strings.Contains(lower, "switch"),
		strings.Contains(lower, "joy-con"),
		strings.Contains(lower, "joycon"),
		strings.Contains(lower, "pro controller"),
		strings.Contains(lower, "gamecube"),
		strings.Contains(lower, "wii"):
		return FamilyNintendo
	}
	return FamilyGeneric
}

// parseWebGamepadID extracts USB vendor and product IDs from a browser
// Gamepad.id string when present. It recognizes Chromium's
// "Name (STANDARD GAMEPAD Vendor: xxxx Product: yyyy)" format and Firefox's
// "xxxx-yyyy-Name" prefix format (including shorter hex values without leading
// zeros). Returns (0, 0) if no vendor/product IDs are present.
func parseWebGamepadID(id string) (vendorID, productID uint16) {
	if id == "" {
		return 0, 0
	}
	// Chromium / Edge format: "... Vendor: 045e Product: 0b13)"
	if vid, ok := parseLabeledHex16(id, "vendor:"); ok {
		pid, _ := parseLabeledHex16(id, "product:")
		return vid, pid
	}
	// Firefox format: "045e-0b13-Xbox Wireless Controller" or "54c-ce6-DualSense..."
	if first := strings.IndexByte(id, '-'); first > 0 && first <= 4 {
		rest := id[first+1:]
		if second := strings.IndexByte(rest, '-'); second > 0 && second <= 4 {
			vid, ok1 := parseHex16(id[:first])
			pid, ok2 := parseHex16(rest[:second])
			if ok1 && ok2 {
				return vid, pid
			}
		}
	}
	return 0, 0
}

func parseLabeledHex16(s, label string) (uint16, bool) {
	lower := strings.ToLower(s)
	idx := strings.Index(lower, label)
	if idx < 0 {
		return 0, false
	}
	rest := strings.TrimLeft(s[idx+len(label):], " \t")
	if strings.HasPrefix(rest, "0x") || strings.HasPrefix(rest, "0X") {
		rest = rest[2:]
	}
	end := 0
	for end < len(rest) && end < 4 && isHexDigit(rest[end]) {
		end++
	}
	if end == 0 {
		return 0, false
	}
	// Ensure we didn't stop mid-token on a longer hex string.
	if end < len(rest) && isHexDigit(rest[end]) {
		return 0, false
	}
	return parseHex16(rest[:end])
}

func parseHex16(s string) (uint16, bool) {
	if len(s) == 0 || len(s) > 4 {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if !isHexDigit(s[i]) {
			return 0, false
		}
	}
	v, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return 0, false
	}
	return uint16(v), true
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}
