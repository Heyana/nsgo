package nsgotest

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// AnimatedValues records the bound properties last evaluated by the animator.
type AnimatedValues struct {
	// Set holds a bit for each bound AnimatedProp.
	Set                    uint16
	Opacity                float32
	TranslateX, TranslateY core.Points
	Scale, ScaleX, ScaleY  float32
	Rotate                 float32
	Background             core.Color
	BorderColor            core.Color
	ShadowColor            core.Color
	ShadowRadius           core.Points
	ShadowOpacity          float32
	// TextColor colors a text's whole content.
	TextColor        core.Color
	RotateX, RotateY float32
}

func animatedValues(v driver.AnimatedValues) AnimatedValues {
	return AnimatedValues{
		Set:           v.Set,
		Opacity:       v.Opacity,
		TranslateX:    v.TranslateX,
		TranslateY:    v.TranslateY,
		Scale:         v.Scale,
		ScaleX:        v.ScaleX,
		ScaleY:        v.ScaleY,
		Rotate:        v.Rotate,
		Background:    v.Background,
		BorderColor:   v.BorderColor,
		ShadowColor:   v.ShadowColor,
		ShadowRadius:  v.ShadowRadius,
		ShadowOpacity: v.ShadowOpacity,
		TextColor:     v.TextColor,
		RotateX:       v.RotateX,
		RotateY:       v.RotateY,
	}
}

func (v AnimatedValues) driverValues() driver.AnimatedValues {
	return driver.AnimatedValues{
		Set:           v.Set,
		Opacity:       v.Opacity,
		TranslateX:    v.TranslateX,
		TranslateY:    v.TranslateY,
		Scale:         v.Scale,
		ScaleX:        v.ScaleX,
		ScaleY:        v.ScaleY,
		Rotate:        v.Rotate,
		Background:    v.Background,
		BorderColor:   v.BorderColor,
		ShadowColor:   v.ShadowColor,
		ShadowRadius:  v.ShadowRadius,
		ShadowOpacity: v.ShadowOpacity,
		TextColor:     v.TextColor,
		RotateX:       v.RotateX,
		RotateY:       v.RotateY,
	}
}

// Has reports whether prop is bound.
func (v AnimatedValues) Has(prop AnimatedProp) bool { return v.driverValues().Has(prop) }

// Apply returns style with its bound properties replaced by the recorded values.
func (v AnimatedValues) Apply(style ViewStyle) ViewStyle {
	return viewStyle(v.driverValues().Apply(style.driverStyle()))
}
