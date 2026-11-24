---
title: Komponenten ohne Design
description: "NSGos eigene Steuerelemente ohne eigenes Aussehen – zum teilweisen Stylen oder als Grundlage für eigene Komponentenbibliotheken."
group: Komponenten schreiben
order: 73
---

`base` enthält jedes Steuerelement, das auch `ui` besitzt – mit seinem Verhalten, seinem Zustand und seiner Barrierefreiheit, aber ohne Aussehen. Jeder Teil akzeptiert `Class`, `Style` und `StyleFunc` ([Teile und Zustände](parts.md)), sodass eine Bibliothek oder eine App mit eigenem Design diese statt `ui` verwendet oder sie wie `ui` umhüllt ([Aussehen und Themes](looks.md)). Diese Seite behandelt die Steuerelemente der [Komponentenreferenz](../ui.md); die Galerie zeigt jedes davon neben seiner `ui`-Variante.

Steuerelemente wie `Button`, `Checkbox`, `Select`, `Slider` und `NumberInput` akzeptieren `Invalid: rejected.Get`, um einem Validierungs-Signal zu folgen. Stylen Sie diesen Zustand mit `Class: "aria-invalid:border-red-500"` oder `Style.AriaInvalid`. Das aktualisiert Barrierefreiheit und Styles, ohne das Steuerelement zu deaktivieren. Zusammengesetzte Felder geben den Zustand an ihr echtes Eingabefeld und dekorative Teile weiter; `ColorPicker` teilt ihn mit seinen Steuerelementen.

Für Anwendungswerte, die zwischen Views gezogen werden, verwenden Sie [`base/dragdrop`](../drag-drop.md): Eine Area verbindet Quellen und Ziele, während die App Inhalt, Vorschau und Erscheinungsbild liefert.

## Button

`base.Button` bezieht sein Aussehen aus `Style` und seinen Inhalt aus `Children`. `OnPress` wird bei einem abgeschlossenen Druck ausgeführt; `OnPressDown` und `OnPressUp` verfolgen Beginn und Ende eines Drucks für eigenes Feedback.

```go
base.Button{
	Label:   "Save",
	OnPress: func(core.PressEvent) { save() },
	Style: core.Style{Background: core.RGB(0x007aff), Padding: core.Symmetric(8, 14), CornerRadius: 8, AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.85)}},
	Children: []core.Node{core.Text{Content: "Save", Style: core.Style{Color: core.RGB(0xffffff)}}},
}
```

`Disabled`, `Busy`, `Selected` und `Expanded` melden einen Zustand an App, Style und assistive Technologien: Eine noch laufende Aktion verwendet `Busy` (gestylt mit `aria-busy:`), ein Umschalter oder der aktuelle Eintrag einer Liste `Selected` und ein Aufklappbereich `Expanded`. `NoFocus` nimmt einen Button aus den Tab-Stopps heraus, wenn eine Zeile oder Liste ihn auf anderem Weg erreicht.

## Checkbox

`base.Checkbox` erhält einen Style für das Kästchen, einen für den Indikator und Ihre Markierung darin, die anhand des von der App verwalteten Werts angezeigt wird.

```go
base.Checkbox{
	Checked:        done.Get,
	OnChange:       done.Set,
	Label:          "Remember me",
	Style:          core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), BorderWidth: 1, CornerRadius: 5},
	Indicator:      core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14}},
	Children:       []core.Node{core.Text{Content: "Remember me"}},
}
```

Der Indikator übernimmt seinen Zustand vom Kästchen über `PartStates`, sodass `checked:`-Klassen ihn stylen, ohne ihn zu einem zweiten Tab-Stopp zu machen.

Für Modifikatortasten setzen Sie `OnChangeEvent func(checked bool, press core.PressEvent)`. Es ersetzt `OnChange` und belässt den Wert bei der App; `press.Modifiers.Shift` ermöglicht es einer Liste, einen Bereich auszuwählen. Die Aktivierung per Leertaste/Eingabetaste auf Desktop-Systemen, im Web, auf Android sowie iOS/tvOS erhält Modifikatortasten, sofern nicht ein App-Tastaturkürzel die Taste zuerst abfängt; unter Windows öffnet Alt mit Leertaste oder Eingabetaste stattdessen das Fenstermenü. In TSX empfängt `onChangeEvent` `{ checked, press }`.

## Switch

`base.Switch` besteht aus einer Schiene (`Track`), einem Regler (`Thumb`) darin und anschließend seinen Kindern als Label. Ein Druck meldet den angeforderten Wert, also das Gegenteil von `Checked`.

```go
base.Switch{
	Checked:    dark.Get,
	OnChange:   dark.Set,
	Label:      "Dark appearance",
	Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	TrackStyle: core.Style{Width: core.Pt(38), Height: core.Pt(22), CornerRadius: 11},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

Schiene und Regler akzeptieren eigene `StyleFunc`s, und ihre Zustands-Styles folgen `Checked`.

## Slider

`base.Slider` nimmt einen `base.SliderValue`: `base.SingleValue(v)` oder `base.RangeValue(low, high)`. Er stylt Schiene, Indikator, Bereich und die Anfasser getrennt voneinander.

```go
level := core.NewSignal(cx, 3.0)
base.Slider{
	Value:      func() base.SliderValue { return base.SingleValue(level.Get()) },
	OnChange:   func(v base.SliderValue) { level.Set(v.End) },
	Min:        func() float64 { return 0 },
	Max:        func() float64 { return 10 },
	Step:       func() float64 { return 1 },
	Label:      "Level",
	Style:      core.Style{Height: core.Pt(28)},
	TrackStyle: core.Style{Height: core.Pt(4), CornerRadius: 2},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

`Min`, `Max` und `Step` sind reaktive Getter: Änderungen ihrer Signals aktualisieren den eingehängten Schieberegler und seinen barrierefreien Bereich, auch während eines Ziehvorgangs. `Axis` richtet ihn vertikal aus und `Inverted` legt das Minimum ans andere Ende, genau wie bei `base.Progress`; `Marks` platziert eigene Markierungen auf dem Indikator, und `OnChangeEnd` meldet den Wert, sobald das Ziehen endet.

## Select

`base.Select` nimmt `base.SelectItem`s und einen `*string`-Wert, sodass „nichts“ `nil` ist. Sie stylen Auslöser, Popup, dessen Zeilen und Gruppen.

```go
choice := core.NewSignal[*string](cx, nil)
base.Select{
	Items: func() []base.SelectItem {
		return []base.SelectItem{{Value: "s", Title: "Small"}, {Value: "m", Title: "Medium"}}
	},
	Value:      choice.Get,
	OnChange:   choice.Set,
	Label:      "Size",
	Style:      core.Style{Height: core.Pt(28), Padding: core.Symmetric(4, 10), BorderWidth: 1, CornerRadius: 6},
	PopupStyle: core.Style{Padding: core.All(4), CornerRadius: 8},
	ItemStyle:  core.Style{Padding: core.Symmetric(4, 8), CornerRadius: 4},
}
```

Benachbarte Einträge mit demselben `Group` teilen sich eine Überschrift, Typeahead gleicht die Titel der Einträge ab, `SearchField` zeichnet ein eigenes Suchfeld, und `Cleanable` zeigt eine Löschen-Schaltfläche. `OnCancel` reagiert auf Escape, Zurück und einen Druck außerhalb.

## Tabs

`base.Tabs` enthält `base.Tab`s und zeichnet den Indikator anhand von `IndicatorStyle`; die App verwaltet, welcher Tab ausgewählt ist. `base.Tab.OnPress` läuft bei jedem Druck – auch beim bereits ausgewählten Tab.

```go
base.Tabs{Label: "Sections", IndicatorStyle: core.Style{Height: core.Pt(2), Background: core.RGB(0x007aff)}, Children: []core.Node{
	base.Tab{Label: "Overview", Selected: func() bool { return section.Get() == "overview" },
		OnPress: func(core.PressEvent) { section.Set("overview") }, Children: []core.Node{core.Text{Content: "Overview"}}},
	base.Tab{Label: "Activity", Selected: func() bool { return section.Get() == "activity" },
		OnPress: func(core.PressEvent) { section.Set("activity") }, Children: []core.Node{core.Text{Content: "Activity"}}},
}}
```

## Dialog

`base.Dialog` übernimmt Styles für Hintergrundabdunklung (`Backdrop`), Ebene (`Layer`) und Popup, eine `Animation` oder einen `TransitionShader` sowie Ihre Kinder. `Open`, `DefaultOpen` und `OnOpenChange` steuern ihn; `OnOpen`, `OnClose` und `OnCancel` melden, was passiert ist. `OnConfirm` ist die Standardaktion: Die Eingabetaste führt sie aus – es sei denn, der Tastaturfokus liegt auf einem Button oder einem Feld, das die Eingabetaste selbst verarbeitet, oder ein anderer modaler Dialog verdeckt diesen Dialog –, woraufhin der Dialog mit `ReasonSelect` schließt. `Role` und `AccessibilityDescription` bestimmen, was Screenreader ansagen (`RoleAlertDialog` für eine Bestätigung). `NoCloseOnSwipe` verhindert das Schließen durch Wischen unter iOS, `Presentation` wählt die iOS-Darstellung und `StatusBar` passt die Statusleiste an, solange der Dialog geöffnet ist.

`base.Sheet` ist ein Panel an einer beliebigen Kante mit Hintergrundabdunklung und Ihrem Inhalt; mit `CloseOnSwipe` folgt ein unteres Sheet einer Abwärts-Ziehbewegung und schließt jenseits eines Schwellenwerts. Es akzeptiert `OnConfirm`, `Role` und `AccessibilityDescription` wie `base.Dialog`. `base.ShowAlertDialog` zeigt den System-Hinweisdialog oder das Action-Sheet mit Buttons nach Rolle an:

```go
base.ShowAlertDialog(cx, base.AlertDialog{
	Title: "Delete the note?",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(label string) {
	if label == "Delete" {
		remove()
	}
})
```

## Menu

`base.Menu` öffnet `base.MenuItem`s über einen eigenen Auslöser, und `base.ContextMenu` öffnet an der Zeigerposition. Ein Eintrag meldet `OnSelect` und schließt das Menü, sofern nicht `NoCloseOnSelect` gesetzt ist; `Checked` macht ihn zu einem abhakbaren Eintrag, `Indicator` zeichnet Ihre Markierung vor dem Inhalt und `Role` bestimmt, was assistive Technologien vorlesen: einen Menüeintrag, eine Checkbox oder einen Radio-Eintrag. Ein `base.Submenu` enthält weitere Einträge.

```go
base.Menu{Trigger: core.Text{Content: "Actions"}, Children: []core.Node{
	base.MenuItem{Label: "Show sidebar", Checked: sidebar.Get, OnSelect: func() { sidebar.Set(!sidebar.Get()) }},
	base.Submenu{Label: "Share", Children: []core.Node{
		base.MenuItem{Label: "Copy link", OnSelect: copyLink},
		base.MenuItem{Label: "Email", OnSelect: emailLink},
	}},
}}
```

`Placement`, `Align`, `Offset` und `Margin` platzieren das Popup auf Desktop-Systemen, und `PopupStyle` sowie `BackdropStyle` stylen es.

## EmojiPicker

`base.EmojiPicker` durchsucht jedes Unicode-Emoji nach seinen lokalisierten CLDR-Namen, Schlüsselwörtern und Shortcodes in den Sprachen der App (oder expliziten `Languages` / `LanguagesFunc`) – mit Kategorie-Tabs, optionalen `Recent`-Einträgen, Hauttonauswahl, Tastaturnavigation und einem virtualisierten Raster. Alle Standard-Labels und Kategorienamen stammen aus `base/locales`, und der Emoji-Datensatz wird nur in Apps verlinkt, die `EmojiPicker` oder seine Hilfsfunktionen (`base.SearchEmojis`, `base.EmojisInCategory`, `base.EmojiByString`) verwenden.

```go
base.EmojiPicker{
	Recent:   recent.Get,
	OnSelect: func(e base.Emoji) { insert(e.Emoji) },
	Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
}
```

`Category` und `DefaultCategory` wählen die angezeigte Gruppe. Kategorie-Tabs und Hauttonauswahl sind jeweils ein Tab-Stopp mit Pfeilnavigation. `EmojiPickerRef` bietet `Focus`, `Query`, `SetQuery`, `SetCategory` und `MatchedCount`; der gestylte Picker verwendet denselben Suchzustand. `LabelFunc`, `SearchLabelFunc` und `PlaceholderFunc` aktualisieren Texte direkt an Ort und Stelle. `Emoji.WithSkinTone` wendet den gewählten Hautton auf beide Personen eines Paares an, und der Standardton stellt die neutrale Form wieder her; die Suche nach einem Emoji behält seine exakte Sequenz bei.

Verwenden Sie `ui.EmojiPicker` für den an das Theme angepassten Picker mit Lucide-Kategorie-Icons, einem löschbaren Suchfeld, hervorgehobener Emoji-Vorschau und Hautton-Feldern.
