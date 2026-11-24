---
title: Formulare
description: Textfelder, Fokus, Bildschirmtastatur, Schalter, Auswahlfelder und Validierung.
group: Kernkonzepte
order: 57
---

## Textfelder

`ui.Input` ist das Textfeld der jeweiligen Plattform im Look von `ui`, mit den Feldern von `base.Input`. Mit `Value` ist es kontrolliert: Die App hält den Wert und aktualisiert ihn in `OnChange`; mit `DefaultValue` verwaltet das Feld ihn selbst.

```go
name, email := core.NewSignal(cx, ""), &core.Ref{}

ui.Input{
	Value:         name.Get,
	OnChange:      name.Set,
	Placeholder:   "Your name",
	ReturnKeyType: core.ReturnKeyNext,
	OnSubmit:      func(string) { email.Focus() },
}
```

`Masked` verbirgt die Zeichen, `Readonly` verhindert Bearbeitungen, während sich der Text weiterhin markieren und kopieren lässt (in Mini-Programmen nur in einem einzeiligen WeChat-Feld: ein mehrzeiliges WeChat-Feld und Alipay-Felder deaktivieren sich wie bei `Disabled`, da diese Hosts eine Eingabe nicht rückgängig machen können), `Disabled` schaltet das Feld ab, und `Multiline` bricht Text um und wächst mit ihm mit, sofern sein Style keine Größe vorgibt. Die Zeilen eines mehrzeiligen Feldes – sowie die seines Platzhalters – sind so hoch wie `LineHeight` seines Styles, wobei die Schrift darin wie in einem `core.Text` zentriert ist; ein einzeiliges Feld hält seine Zeile in seiner Höhe zentriert. `KeyboardType` wählt die Bildschirmtastatur (`core.KeyboardEmail`, `core.KeyboardNumber`, `core.KeyboardURL` usw.), `AutoComplete` teilt dem System für Autofill mit, was das Feld enthält, und `MaxLength` begrenzt die UTF-16-Einheiten, die eingetippt werden können. `AutoCapitalize` und `NoAutoCorrect` steuern das Verhalten der Tastatur genauer.

`OnSelectionChange` und `OnCaretMove` melden die Auswahl und die Position der Einfügemarke, etwa für ein Menü, das dort aufgeht. `Suggest` – ein `core.Suggest` – verfolgt eine Abfrage nach einem Auslöser wie `@`, und eine `ui.SuggestList` neben dem Feld bietet dafür Einträge an der Einfügemarke an, aus denen Hoch, Runter, Return und ein Druck auswählen, so wie `Suggest` von `chat.Composer` für Erwähnungen. `OnKeyPress` empfängt jede Taste, `OnPaste` das Eingefügte und `OnCompositionChange` verfolgt eine Eingabemethode (IME).

Unter iOS und Android fängt `cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, handler)` ein Return der Bildschirmtastatur ab, bevor das Feld absendet, den Fokus verliert oder einen Zeilenumbruch einfügt. `KeyAuto` und `KeyNotEditing` überlassen es `SubmitBehavior`; unter iOS bleibt ein Return, das markierten Text bestätigt, bei der Eingabemethode.

## Fokus und Tastatur

`Ref.Focus()` fokussiert ein Feld oder – auf Desktops, im Web und auf Apple TV – einen aktivierten drückbaren oder `Focusable`-View. `Ref.Blur()` löscht nur den Fokus seines eigenen Views, und `cx.DismissKeyboard()` schließt die Bildschirmtastatur. `OnFocus` und `OnBlur` verfolgen den Fokus, und `View.OnFocus`/`OnBlur` tun dasselbe für jeden fokussierbaren View. Ein fokussierter View, der den Baum verlässt – etwa weil sich sein `Show` schließt oder seine Zeile verschwindet –, erhält `OnBlur` einmal nach dem Commit, der ihn entfernt hat, oder – falls die Plattform zuerst den nächsten Fokus meldet – vor dem `OnFocus` jenes Views, und die umgebenden Views, die bleiben, erhalten `OnFocusOut`.

Fokus-, Blur-, Scroll- und Zoom-Anfragen eines `Ref` tun nichts, solange sich sein Element im Inhalt eines `Holder` befindet, den kein Platz anzeigt, und noch in der Warteschlange stehende Anfragen verfallen, wenn das Element den Baum verlässt. Ein erneutes Anzeigen des Inhalts spielt sie nicht nach: Fordern Sie sie erneut an, sobald er platziert ist.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Bildschirmtastaturen verdecken den unteren Bereich eines Smartphones. `core.EnvKeyboard` als unterer Innen- oder Außenabstand hält Inhalte darüber, und seine Bewegung wird gemeinsam mit der Tastatur animiert:

```go
core.View{Style: core.Style{
	Padding:    core.All(16),
	PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard},
	Gap:        12,
}, Children: form}
```

Ein Scroll-View um ein Formular herum kann die Tastatur beim Ziehen schließen und festlegen, was ein Tippen bewirkt, solange die Tastatur sichtbar ist:

```go
core.Scroll{Behavior: core.ScrollBehavior{
	KeyboardDismissMode:       core.KeyboardDismissOnDrag,
	KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
}, Children: []core.Node{form}}
```

`cx.Env().KeyboardHeight()` und `cx.Env().KeyboardDuration()` lesen Größe und Timing der Tastatur im Code aus, und `cx.Env().SafeArea()` die Abstände rund um den Bildschirm.

## Auswahl-Steuerelemente

`ui.Switch` ist eine Einstellung, die sofort wirksam wird, `ui.Checkbox` ist angehakt, nicht angehakt oder unbestimmt, und `ui.Radio` ist eine Option einer `ui.RadioGroup`. `ui.Select[T]` öffnet eine Optionsliste unter seinem Auslöser – oder als Sheet, wenn seine `Presentation` es verlangt –, und `ui.Tabs[T]` zeigt mehrere Optionen zugleich an.

```go
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}

size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options:  []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:    size.Get,
	OnChange: size.Set,
	Label:    "Size",
}
```

`ui.NumberInput` und `base.NumberInput` akzeptieren `LabelFunc` für einen wechselnden barrierefreien Namen. Ein eigenes `Format` folgt den gelesenen Signals, solange das Feld nicht bearbeitet wird; ein Sprachwechsel bewahrt den aktiven Entwurf bis zum Ende der Bearbeitung. `ui.NumberInput` schaltet und zieht eine Zahl innerhalb von Grenzen, `ui.DatePicker` und `ui.TimeField` bearbeiten Daten und Uhrzeiten, `ui.ColorPicker` bearbeitet eine Farbe, und `ui.TagInput` bearbeitet eine Liste von Tags.

`ui.OtpInput` nimmt einen Einmalcode in einzelnen Zellen entgegen. Es und `base.OtpInput` akzeptieren `LabelFunc`, ohne das fokussierte Feld oder seinen Code zu ersetzen. `Size` setzt die Standardhöhe und -breite – 36 Punkte auf Desktops und Fernsehern und 44 auf Touch-Geräten. `CellWidth` und `CellHeight` überschreiben beide Maße unabhängig voneinander. `TextStyle` überschreibt die Zelltypografie, `CornerRadius` ändert die Ecken, und `CaretHeight` überschreibt die aus der Zeilenhöhe abgeleitete Höhe.

```go
quantity := core.NewSignal(cx, 1.0)
ui.NumberInput{Value: quantity.Get, OnChange: quantity.Set, Min: 1, Max: 99, Step: 1, Label: "Quantity"}
```

## Gruppierte Zeilen

`ui.FormSection` setzt seine Zeilen auf eine Karte, mit dem Titel darüber und dem Footer darunter. Setzen Sie `CardStyle` für Hintergrund, Rahmen oder Ecken der Karte. `CardStyleFunc` legt sich darüber und folgt den gelesenen Signals. `DividerInset` erhält jede Kindzeile, einschließlich durch `core.Each` erzeugter Zeilen: Geben Sie einen Einzug und `true` zurück, um deren Trennlinie zu überschreiben, oder `false`, um die automatische Titelausrichtung beizubehalten (12 Punkte bei anderen Nodes). Ein Einzug von null ergibt eine Trennlinie über die volle Breite.

`ui.FormRow` platziert `Leading` oder eine Icon-Kachel vor dem Titel. Setzen Sie `LeadingWidth`, um Platz für Ihren Node zu reservieren und die Trennlinie darunter am Titel auszurichten. Der Slot misst standardmäßig 24 Punkte. Setzen Sie `TileSize`, um Icon-Kachel, Glyphe und Ecken gemeinsam zu skalieren. `TitleStyle` und `DetailStyle` überschreiben die Typografie von Titel und Detail. `TitleMaxLines` begrenzt die Zeilenanzahl des Titels (null ist unbegrenzt), `MinHeight` überschreibt die Mindesthöhe der Zeile in Punkten, und `Below` platziert einen Node nach Titel und Detail in deren Textspalte. Ein `MinHeight` von null behält den Standardwert bei: 40 Punkte beziehungsweise 48 auf Touch-Geräten.

Für eine Einfachauswahl-Gruppe umschließen Sie den Abschnitt mit `base.RadioGroup` und setzen bei jeder Zeile `Role: core.RoleRadio`, `Checked` und `OnPress`. Die Zeile verwendet `base.Radio` für ihre Barrierefreiheitszustände „checked“ und „selected“, genau einen Tab-Stopp sowie Auswahl per Pfeiltasten; das Drücken einer bereits ausgewählten Zeile tut nichts. Der gewählte Wert gehört der App.

`ui.FormSection.TitleFunc` und `FooterFunc` sowie `ui.FormRow.TitleFunc`, `DetailFunc` und `LabelFunc` aktualisieren ihre Texte, ohne die Zeile neu aufzubauen. Ein leeres Detail, ein leerer Abschnittstitel oder Footer entfernt diesen optionalen Text; ein leeres Zeilen-Label verwendet seinen aktuellen Titel. Ein durchsuchbares `ui.Select` bindet `SearchPlaceholderFunc` und `EmptyFunc`, während die geöffnete Suche und Auswahl erhalten bleiben.

## Validierung

Die Regeln gehören der App: Ein Signal hält den Fehler, `OnBlur` oder `OnSubmit` setzt ihn, und ein `core.Show` fügt unter dem Feld eine Meldung hinzu, solange eine vorhanden ist.

```go
address := core.NewSignal(cx, "")
err := core.NewSignal(cx, "")
colors := ui.ColorsOf(cx)
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{
		Value:    address.Get,
		OnChange: func(v string) { address.Set(v); err.Set("") },
		Label:    "Email",
		OnSubmit: func(string) {
			if !strings.Contains(address.Get(), "@") {
				err.Set("Enter an email address.")
			}
		},
	},
	core.Show{
		When: func() bool { return err.Get() != "" },
		Then: func(*core.Scope) core.Node { return ui.Textf(err.Get, 12, ui.Regular, colors.Danger) },
	},
}}
```

`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` animiert eine erscheinende Meldung, und `OnSubmit` zusammen mit `ReturnKeyNext` bewegt die Tastatur zum nächsten Feld.

## Nächste Schritte

- [Navigation](navigation.md): Wo ein Formular lebt.
- [Text](text.md): Wie der Text eines Feldes gestaltet wird.
- [Barrierefreiheit](accessibility.md): Labels und Werte, die Screenreader benötigen.
