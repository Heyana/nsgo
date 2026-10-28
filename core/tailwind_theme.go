package core

import "time"

// tailwindTheme is what classes resolve against: Tailwind CSS 4's default
// theme (theme.css), as NSH's tailwind/theme.rs has it, in points.
type tailwindTheme struct {
	spacing     Points // one spacing step: p-1
	rem         Points
	colors      map[string]Color
	breakpoints []breakpoint // ascending
	containers  map[string]Points
	text        map[string]textSize
	weights     map[string]FontWeight
	fonts       map[string]string // "" is the system font
	tracking    map[string]float32
	leading     map[string]float32
	radius      map[string]Points
	shadows     map[string][]BoxShadow
	insetShadow map[string][]BoxShadow
	dropShadows map[string][]BoxShadow
	textShadows map[string]BoxShadow
	blur        map[string]Points
	perspective map[string]Points
	aspect      map[string]float32
	ease        map[string]Curve
	duration    time.Duration // the default transition's
	curve       Curve
}

// breakpoint is a responsive variant: sm: applies from width on.
type breakpoint struct {
	name  string
	width Points
}

// textSize is text-lg: a size and its line height as a multiple of it.
type textSize struct {
	size    Points
	leading float32
}

// shadow is a black shadow of alpha.
func shadow(x, y, blur, spread Points, alpha float32) BoxShadow {
	return BoxShadow{Offset: Point{X: x, Y: y}, Blur: blur, Spread: spread, Color: RGBA(0, alpha)}
}

var defaultTailwind = newTailwindTheme()

func newTailwindTheme() *tailwindTheme {
	const rem = 16
	colors := map[string]Color{"black": RGB(0), "white": RGB(0xffffff), "transparent": clearColor}
	for name, shades := range tailwindPalette {
		for i, c := range shades {
			colors[name+"-"+tailwindShades[i]] = c
		}
	}
	return &tailwindTheme{
		spacing: 0.25 * rem,
		rem:     rem,
		colors:  colors,
		breakpoints: []breakpoint{{"sm", 40 * rem}, {"md", 48 * rem}, {"lg", 64 * rem}, {"xl", 80 * rem},
			{"2xl", 96 * rem}},
		containers: map[string]Points{"3xs": 16 * rem, "2xs": 18 * rem, "xs": 20 * rem, "sm": 24 * rem,
			"md": 28 * rem, "lg": 32 * rem, "xl": 36 * rem, "2xl": 42 * rem, "3xl": 48 * rem, "4xl": 56 * rem,
			"5xl": 64 * rem, "6xl": 72 * rem, "7xl": 80 * rem},
		text: map[string]textSize{"xs": {0.75 * rem, 1 / 0.75}, "sm": {0.875 * rem, 1.25 / 0.875},
			"base": {rem, 1.5}, "lg": {1.125 * rem, 1.75 / 1.125}, "xl": {1.25 * rem, 1.75 / 1.25},
			"2xl": {1.5 * rem, 2 / 1.5}, "3xl": {1.875 * rem, 2.25 / 1.875}, "4xl": {2.25 * rem, 2.5 / 2.25},
			"5xl": {3 * rem, 1}, "6xl": {3.75 * rem, 1}, "7xl": {4.5 * rem, 1}, "8xl": {6 * rem, 1},
			"9xl": {8 * rem, 1}},
		weights: map[string]FontWeight{"thin": 100, "extralight": 200, "light": 300, "normal": 400,
			"medium": 500, "semibold": 600, "bold": 700, "extrabold": 800, "black": 900},
		fonts: map[string]string{"sans": "", "serif": "serif", "mono": "monospace"},
		tracking: map[string]float32{"tighter": -0.05, "tight": -0.025, "normal": 0, "wide": 0.025,
			"wider": 0.05, "widest": 0.1},
		leading: map[string]float32{"none": 1, "tight": 1.25, "snug": 1.375, "normal": 1.5, "relaxed": 1.625,
			"loose": 2},
		radius: map[string]Points{"xs": 0.125 * rem, "sm": 0.25 * rem, "md": 0.375 * rem, "lg": 0.5 * rem,
			"xl": 0.75 * rem, "2xl": rem, "3xl": 1.5 * rem, "4xl": 2 * rem},
		shadows: map[string][]BoxShadow{
			"2xs": {shadow(0, 1, 0, 0, 0.05)},
			"xs":  {shadow(0, 1, 2, 0, 0.05)},
			"sm":  {shadow(0, 1, 3, 0, 0.1), shadow(0, 1, 2, -1, 0.1)},
			"":    {shadow(0, 1, 3, 0, 0.1), shadow(0, 1, 2, -1, 0.1)},
			"md":  {shadow(0, 4, 6, -1, 0.1), shadow(0, 2, 4, -2, 0.1)},
			"lg":  {shadow(0, 10, 15, -3, 0.1), shadow(0, 4, 6, -4, 0.1)},
			"xl":  {shadow(0, 20, 25, -5, 0.1), shadow(0, 8, 10, -6, 0.1)},
			"2xl": {shadow(0, 25, 50, -12, 0.25)},
		},
		insetShadow: map[string][]BoxShadow{
			"2xs": {inset(shadow(0, 1, 0, 0, 0.05))},
			"xs":  {inset(shadow(0, 1, 1, 0, 0.05))},
			"sm":  {inset(shadow(0, 2, 4, 0, 0.05))},
		},
		dropShadows: map[string][]BoxShadow{
			"xs":  {shadow(0, 1, 1, 0, 0.05)},
			"sm":  {shadow(0, 1, 2, 0, 0.15)},
			"md":  {shadow(0, 3, 3, 0, 0.12)},
			"lg":  {shadow(0, 4, 4, 0, 0.15)},
			"xl":  {shadow(0, 9, 7, 0, 0.1)},
			"2xl": {shadow(0, 25, 25, 0, 0.15)},
			"":    {shadow(0, 1, 2, 0, 0.1), shadow(0, 1, 1, 0, 0.06)},
		},
		// Each text shadow's strongest layer: a text draws one.
		textShadows: map[string]BoxShadow{
			"2xs": shadow(0, 1, 0, 0, 0.15), "xs": shadow(0, 1, 1, 0, 0.2), "sm": shadow(0, 2, 2, 0, 0.075),
			"md": shadow(0, 2, 4, 0, 0.1), "lg": shadow(0, 4, 8, 0, 0.1),
		},
		blur: map[string]Points{"xs": 4, "sm": 8, "": 8, "md": 12, "lg": 16, "xl": 24, "2xl": 40, "3xl": 64},
		perspective: map[string]Points{"dramatic": 100, "near": 300, "normal": 500, "midrange": 800,
			"distant": 1200},
		aspect: map[string]float32{"square": 1, "video": 16.0 / 9},
		ease: map[string]Curve{"linear": {X1: 0, Y1: 0, X2: 1, Y2: 1}, "in": {X1: 0.4, Y1: 0, X2: 1, Y2: 1},
			"out": {X1: 0, Y1: 0, X2: 0.2, Y2: 1}, "in-out": {X1: 0.4, Y1: 0, X2: 0.2, Y2: 1}},
		duration: 150 * time.Millisecond,
		curve:    Curve{X1: 0.4, Y1: 0, X2: 0.2, Y2: 1},
	}
}

func inset(s BoxShadow) BoxShadow { s.Inset = true; return s }

// clearColor is transparent as a style sets it: alpha 0, but not the
// zero Color, which leaves a field unset.
const clearColor Color = 0x00000100
