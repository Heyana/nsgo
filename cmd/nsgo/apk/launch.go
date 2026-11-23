package apk

import "encoding/binary"

// LaunchScreen holds the launcher activity's splash screen and its night overrides. The framework theme can pin the selected appearance.
type LaunchScreen struct {
	Light LaunchAppearance
	Dark  LaunchAppearance
}

// LaunchAppearance holds an opaque ARGB color and a PNG for the splash screen. Zero and nil leave the default or corresponding light entry in effect.
type LaunchAppearance struct {
	Background uint32
	Image      []byte
}

const (
	launchBackgroundAttr = 0x0101062c // windowSplashScreenBackground
	launchIconAttr       = 0x0101062d // windowSplashScreenAnimatedIcon
	launchBehaviorAttr   = 0x01010668 // windowSplashScreenBehavior
	typeColorRGB8        = 0x1d
	launchStyleName      = "NsgoLaunchScreen"
	launchImageName      = "nsgo_launch_image"
	launchImageFile      = "res/drawable-xxxhdpi-v31/" + launchImageName + ".png"
	launchNightImageFile = "res/drawable-night-xxxhdpi-v31/" + launchImageName + ".png"
	configUIMode         = 0x1000
	configVersion        = 0x0400
)

type resStyle struct {
	parent uint32
	items  []resStyleItem
}

type resStyleItem struct {
	key  uint32
	kind uint8
	data uint32
}

// launchStyleID follows the other resource types, the widgets' layouts last, leaving their IDs unchanged.
func launchStyleID(m Manifest) uint32 {
	id := layoutResourceID(m)
	if len(m.layoutResources()) > 0 {
		id += 1 << 16
	}
	return id
}

func launchAppearances(m Manifest) (light, night LaunchAppearance) {
	light, night = m.Launch.Light, m.Launch.Dark
	switch styles[m.Theme] {
	case deviceDarkTheme:
		if night.Background != 0 {
			light.Background = night.Background
		}
		if len(night.Image) > 0 {
			light.Image = night.Image
		}
		fallthrough
	case deviceLightTheme:
		night = LaunchAppearance{}
	}
	return light, night
}

func launchFiles(m Manifest) []File {
	if m.Launch == nil {
		return nil
	}
	light, night := launchAppearances(m)
	var files []File
	if len(light.Image) > 0 {
		files = append(files, File{Name: launchImageFile, Data: light.Image, Stored: true})
	}
	if len(night.Image) > 0 {
		files = append(files, File{Name: launchNightImageFile, Data: night.Image, Stored: true})
	}
	return files
}

type launchConfig struct {
	sdk     uint16
	density uint16
	night   bool
}

func (c launchConfig) binary() []byte {
	b := newConfig()
	binary.LittleEndian.PutUint16(b[14:], c.density)
	binary.LittleEndian.PutUint16(b[24:], c.sdk)
	if c.night {
		b[29] = 0x20 // UI_MODE_NIGHT_YES
	}
	return b
}

func (c launchConfig) proto() []byte {
	var b []byte
	if c.night {
		b = pbVarint(b, 17, 1) // UI_MODE_NIGHT_NIGHT
	}
	if c.density != 0 {
		b = pbVarint(b, 18, uint64(c.density))
	}
	if c.sdk != 0 {
		b = pbVarint(b, 24, uint64(c.sdk))
	}
	return b
}

type launchVariant struct {
	config launchConfig
	style  resStyle
}

type launchDrawable struct {
	config launchConfig
	path   string
}

func launchValues(m Manifest) ([]launchVariant, []launchDrawable) {
	light, night := launchAppearances(m)
	var drawables []launchDrawable
	var iconID uint32
	if len(light.Image) > 0 {
		iconID = launchStyleID(m) + 1<<16
		drawables = append(drawables, launchDrawable{launchConfig{sdk: 31, density: densityXXXHigh}, launchImageFile})
		if len(night.Image) > 0 {
			drawables = append(drawables, launchDrawable{launchConfig{sdk: 31, density: densityXXXHigh, night: true}, launchNightImageFile})
		}
	}

	parent := styles[m.Theme]
	if parent == 0 {
		parent = deviceDarkTheme
	}
	// Older Android releases inherit the existing theme. The new attributes only appear in configurations for the releases that introduced them.
	variants := []launchVariant{{style: resStyle{parent: parent}}}
	if m.Theme == "" {
		parent = deviceLightTheme
	}
	variants = appendLaunchVariants(variants, parent, light.Background, iconID, false)
	if m.Theme == "" {
		background := night.Background
		if background == 0 {
			background = light.Background
		}
		variants = appendLaunchVariants(variants, deviceDarkTheme, background, iconID, true)
	}
	return variants, drawables
}

func appendLaunchVariants(variants []launchVariant, parent, background, iconID uint32, night bool) []launchVariant {
	style := resStyle{parent: parent}
	if background != 0 {
		style.items = append(style.items, resStyleItem{launchBackgroundAttr, typeColorRGB8, background})
	}
	if iconID != 0 {
		style.items = append(style.items, resStyleItem{launchIconAttr, typeReference, iconID})
	}
	variants = append(variants, launchVariant{launchConfig{sdk: 31, night: night}, style})
	if iconID != 0 {
		// Request the custom icon even for starts where Android 13 and later would normally show only the background.
		style.items = append(style.items, resStyleItem{launchBehaviorAttr, typeInt, 1}) // icon_preferred
		variants = append(variants, launchVariant{launchConfig{sdk: 33, night: night}, style})
	}
	return variants
}

func launchBinaryTypes(m Manifest) []resType {
	variants, drawables := launchValues(m)
	style := resEntry{key: launchStyleName}
	for _, v := range variants {
		style.values = append(style.values, resValue{config: v.config.binary(), style: &v.style})
	}
	varies := uint32(configVersion)
	if m.Theme == "" {
		varies |= configUIMode
	}
	types := []resType{{name: "style", varies: varies, entries: []resEntry{style}}}
	if len(drawables) != 0 {
		image := resEntry{key: launchImageName}
		for _, d := range drawables {
			image.values = append(image.values, resValue{config: d.config.binary(), str: d.path})
		}
		types = append(types, resType{name: "drawable", varies: varies | configDensity, entries: []resEntry{image}})
	}
	return types
}

func launchProtoTypes(m Manifest) [][]byte {
	variants, drawables := launchValues(m)
	id := uint64(launchStyleID(m) >> 16 & 0xff)
	var styles [][2][]byte
	for _, v := range variants {
		style := pbBytes(nil, 1, protoReference(v.style.parent))
		for _, attr := range v.style.items {
			entry := pbBytes(nil, 3, protoReference(attr.key))
			var item []byte
			switch attr.kind {
			case typeReference:
				item = pbBytes(nil, 1, protoReference(attr.data))
			case typeColorRGB8:
				item = pbBytes(nil, 7, pbVarint(nil, 10, uint64(attr.data)))
			case typeInt:
				item = pbBytes(nil, 7, pbVarint(nil, 6, uint64(attr.data)))
			}
			entry = pbBytes(entry, 4, item)
			style = pbBytes(style, 3, entry)
		}
		compound := pbBytes(nil, 5, pbBytes(nil, 2, style))
		styles = append(styles, [2][]byte{v.config.proto(), compound})
	}
	types := [][]byte{protoValueType(id, "style", launchStyleName, styles)}
	if len(drawables) != 0 {
		var files [][2][]byte
		for _, d := range drawables {
			png := pbVarint(pbString(nil, 1, d.path), 2, 1)
			files = append(files, [2][]byte{d.config.proto(), pbBytes(nil, 5, png)})
		}
		types = append(types, protoType(id+1, "drawable", launchImageName, files))
	}
	return types
}

func protoReference(id uint32) []byte {
	return pbVarint(nil, 2, uint64(id))
}
