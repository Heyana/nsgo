//go:build !gallery_group || gallery_inputs

package main

import (
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func init() {
	type m = locales.Messages
	registerGroup("components", 2, "Inputs", m.GroupInputs, []demo{
		{"Input", m.DemoInput, m.DemoInputDetail, lucide.TextCursorInput, inputDemo},
		{"Form", m.DemoForm, m.DemoFormDetail, lucide.ClipboardList, cardsDemo(formFieldCard, formSectionCard)},
		{"Checkbox", m.DemoCheckbox, m.DemoCheckboxDetail, lucide.SquareCheck, cardsDemo(checkboxCard, radioCard, switchCard)},
		{"Select", m.DemoSelect, m.DemoSelectDetail, lucide.ChevronsUpDown, cardsDemo(selectPlayground, comboboxPlayground)},
		{"DatePicker", m.DemoDatePicker, m.DemoDatePickerDetail, lucide.Calendar, cardsDemo(datePickerPlayground, calendarPlayground, timeFieldPlayground)},
		{"ColorPicker", m.DemoColorPicker, m.DemoColorPickerDetail, lucide.Palette, cardsDemo(colorPickerPlayground, colorWellPlayground)},
		{"EmojiPicker", m.DemoEmojiPicker, m.DemoEmojiPickerDetail, lucide.FaceSlightlySmiling, cardsDemo(emojiPickerPlayground)},
		{"Slider", m.DemoSlider, m.DemoSliderDetail, lucide.SlidersHorizontal, cardsDemo(sliderPlayground, ratingCard)},
	})
}
