---
title: Barrierefreiheit
description: "Labels, Rollen, Werte, Aktionen und Fokus für Screenreader und Tastaturen."
group: Kernkonzepte
order: 61
---

## Was ein Screenreader vorliest

Das `Label` einer View ist das, was assistive Technologien vorlesen, `AccessibilityDescription` liefert weitere Informationen dazu (die nach dem Label vorgelesen werden), `Hint` beschreibt, was das Aktivieren bewirkt, und `Role` gibt an, um welches Element es sich handelt: `core.RoleButton`, `RoleLink`, `RoleHeader`, `RoleImage`, `RoleCheckbox`, `RoleSwitch`, `RoleRadio`, `RoleTab`, `RoleTextField`, `RoleSearchField`, `RoleStatus`, `RoleMenu`, `RoleMenuItem`, `RoleDialog`, `RoleAlertDialog`, `RoleAlert`, `RoleAdjustable`, `RoleProgressBar`, `RoleTable` und so weiter. Ein Status ist standardmäßig eine höfliche Live-Region (`polite`); ein Suchfeld ist ein Textfeld, das als Suchfeld angesagt wird. Ein Dialog wird beim Öffnen mit seiner Rolle und seinem Label vorgelesen und anschließend mit seiner Beschreibung – genau wie Titel und Beschreibung bei `ui.Dialog`. Ein Text ohne Label wird anhand seines Inhalts vorgelesen. `LabelFunc`, `HintFunc` und `AccessibilityDescriptionFunc` binden veränderliche Barrierefreiheitstexte an dieselbe View, sodass Steuerelemente bei einem Sprachwechsel ihren Fokus und Zustand behalten. Core-Elemente mit einem Label akzeptieren alle `LabelFunc`, und `View` sowie `Text` akzeptieren `HintFunc`.

Verwenden Sie `TextFunc` für veränderliche Texte auf `ui.Button`, `ui.Checkbox`, `ui.MenuItem` und `ui.AlertAction`. Ihre barrierefreien Namen folgen diesem Text. Buttons und Checkboxen akzeptieren zusätzlich eine separate `LabelFunc`. Schieberegler, Tabs, Schalter, Fortschrittsbalken und Popup-Auslöser akzeptieren `LabelFunc` für einen veränderlichen barrierefreien Namen. `ui.Option.LabelFunc` liefert den Optionstext in Tabs, Selects und Comboboxen. Zahlen- und Code-Eingabefelder, Fortschrittsringe sowie beide Spinner-Varianten akzeptieren ebenfalls `LabelFunc`.

```go
core.View{
	Role:  core.RoleButton,
	Label: "Add to cart",
	Hint:  "Adds the item and opens the cart",
	OnPress: add,
	Children: []core.Node{core.Icon{SVG: lucide.ShoppingCart}},
}
```

`AccessibilityValue` liefert einen Bereich oder einen Textwert: Minimum, Maximum und aktuellen Wert eines Schiebereglers oder einen Text wie „3 Artikel“. `Accessible` fasst eine View zu einem einzigen Element zusammen, sodass Name und Preis einer Karte gemeinsam statt als zwei separate Stopps vorgelesen werden; ohne Label liest die View den Text ihrer Nachkommen vor. `AccessibilityElementsHidden` blendet Dekorationen wie einen Winkelpfeil oder eine Trennlinie aus, während `ImportantForAccessibility` einen ganzen Teilbaum ausblendet.

```go
core.View{Accessible: true, Label: "Apples, $1.20 a kilo", Children: []core.Node{
	core.Text{Content: "Apples"},
	core.Text{Content: "$1.20 a kilo"},
}}
```

## Sichtbare Labels

Übergeben Sie einem `core.Text` oder einer `core.View` die `Ref` des Steuerelements als `LabelFor`. Ein Druck auf das Label fokussiert das Steuerelement und führt dessen Druckaktion aus, beispielsweise das Umschalten einer Checkbox. Der aktuell sichtbare Text benennt das Steuerelement, ohne dass ein `Label`-String wiederholt werden muss, und übernimmt die Zustände `Disabled` und `Invalid` des Steuerelements für Barrierefreiheit und Styles. Ein Label erhält weder eine Button-Rolle noch einen Tab-Stopp.

```go
field := new(core.Ref)
return core.View{Children: []core.Node{
	core.Text{Content: "Email", LabelFor: field},
	base.Input{Ref: field},
}}
```

`LabelledBy: []*core.Ref{label, requirement}` benennt ein Steuerelement anhand mehrerer sichtbarer Elemente in der angegebenen Reihenfolge – mit Vorrang vor `Label`; Drücke werden dabei nicht weitergeleitet. Andernfalls hat `Label` Vorrang vor impliziten `LabelFor`-Namen, deren Reihenfolge dem Baum folgt. Textaktualisierungen, Verschiebungen, Ausblenden, Entfernen und erneutes Einbinden aktualisieren die Zuordnung. Fehlende, wiederholte, zyklische und ausgeblendete Referenzen werden ignoriert; Zuordnungen bleiben innerhalb einer Oberfläche und eines Eingrenzungsbereichs (`confinement scope`). Ein Label kann kein Steuerelement hinter einem modalen Dialog aktivieren.

Buttons, Checkboxen, Radiobuttons, Schalter, Selects und Zahleneingaben aus `base` und `ui` reichen `Ref` und `LabelledBy` an ihr Steuerelement durch; Schieberegler stellen `ThumbRef` und `StartThumbRef` bereit. Verwenden Sie in TSX `labelFor={field()}` und `labelledBy={[label()]}` mit Node-Signals, die über `ref={node => field.set(node)}` befüllt werden, damit ein Label vor seinem Feld aktualisiert wird, sobald dieses eingehängt wird.

## Aktionen

`AccessibilityActions` listet auf, was der Screenreader auf einem Element anbietet: die Standardaktionen (`core.ActionActivate`, `ActionIncrement`, `ActionDecrement`, `ActionEscape`, `ActionMagicTap`) sowie eigene Aktionen der App, jeweils mit `Name` und `Label`. `OnAccessibilityAction` empfängt den Namen. Verwenden Sie `AccessibilityActionsFunc` auf `core.View` oder `core.Input`, um die verfügbaren Aktionen zu ändern oder ihre Labels zu übersetzen, während das Element eingehängt bleibt.

```go
core.View{
	Role: core.RoleAdjustable,
	Label: "Quantity",
	AccessibilityValue: func() core.AccessibilityValue {
		return core.AccessibilityValue{Max: 10, Now: float32(n.Get()), Text: fmt.Sprintf("%d items", n.Get())}
	},
	AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
	OnAccessibilityAction: func(name string) {
		if name == core.ActionIncrement {
			n.Update(func(v int) int { return v + 1 })
		} else {
			n.Update(func(v int) int { return v - 1 })
		}
	},
	Children: quantityControls,
}
```

`MultiSelectable` kennzeichnet ein Grid, eine Listbox oder einen Baum, in dem mehr als ein Eintrag ausgewählt werden kann. `base.DataTable` setzt dies bei mehrzeiliger Auswahl; das Web, Windows und Linux stellen das entsprechende Barrierefreiheits-Flag bereit.

`Expanded` gibt an, ob ein Aufklappbereich oder Menü-Button geöffnet ist, `Busy`, dass Inhalte geladen werden, `Invalid`, dass die App den Inhalt eines Felds oder den Wert eines Steuerelements abgelehnt hat (gestylt mit `aria-invalid:`), und `Checked`, `Selected` sowie `Pressed` melden den Zustand eines Steuerelements. Die Teile eines Steuerelements können dessen Zustand über `PartStates` übernehmen, sodass ein Indikator seiner Checkbox folgt, ohne selbst ein eigener Tab-Stopp zu sein.

`SelectionItem` kennzeichnet eine View als Element in einer auswählbaren Sammlung, unabhängig von ihrem aktuellen `Selected`-Wert. `base.DataTable` setzt es auf Zeilen oder Zellen, wenn die Tabelle diese auswählt, und `base.Tree` setzt es auf jede Zeile; Windows behält das SelectionItem-Pattern bei und das Web schreibt `aria-selected="true"` oder `"false"`. Eine Tabelle ohne Auswahl lässt die Zeilen ohne beide Markierungen.

## Live-Änderungen

`LiveRegion` veranlasst Screenreader dazu, das Label einer View und den darin enthaltenen Text anzusagen, sobald sie sich ändern: `core.LiveRegionPolite` wartet, bis die nutzende Person inaktiv ist, `LiveRegionAssertive` unterbricht sofort. `cx.Announce` sagt eine Nachricht einmalig an.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("%d items in the basket", count.Get()) },
	LiveRegion: core.LiveRegionPolite}
```

## Fokus und Tastatur

Tooltips beschreiben ihren Auslöser, bevor dieser den Fokus erhält. `View.Tooltip`, `base.Tooltip` und `ui.Tooltip` verwenden ihren aktuellen Text als Barrierefreiheitsbeschreibung des Steuerelements, sofern er nicht den Namen des Steuerelements wiederholt, die App `AccessibilityDescription` angibt oder der Tooltip `Disabled` ist. Ein Wrapper gibt die Beschreibung an seine Steuerelemente weiter. Die Sprechblase und ihre Kinder sind für Screenreader ausgeblendet und lösen niemals eine Warnmeldung aus; der Standard-Tooltip von `ui.IconButton` wiederholt daher nicht sein Label.

Eine `base.DataTable` stellt ihr benanntes Grid als Fokus-Stopp für Tastatur und Barrierefreiheit bereit; die Trefferebene für Zeiger ist vor assistiven Technologien verborgen.

Auf Desktop-Systemen, im Web und auf Apple TV sind Views, die Drücke entgegennehmen, neben Textfeldern ebenfalls Tab-Stopps; `NoFocus` schließt eine View davon aus und `Focusable` macht eine View ohne Druckbehandlung zu einem Stopp. `Style.FocusVisible` zeichnet den Fokusring nur, wenn die Tastatur den Fokus bewegt hat (`focus-visible:`), `Style.Focus` bei jedem Fokus und `Style.FocusWithin` bei Fokus innerhalb einer View; `OnFocus` und `OnBlur` empfangen diese Ereignisse (`OnBlur` auch, wenn die fokussierte View den Baum verlässt), und `Ref.Focus()` sowie `Ref.SetAccessibilityFocus()` verschieben den Fokus; `core.FocusedRef(cx)` ist eine Ref auf die aktuell fokussierte View, sodass etwa eine über ein Tastaturkürzel geöffnete Suchleiste beim Schließen den Fokus zurückgeben kann. Eingabetaste und Leertaste betätigen die fokussierte View, und Pfeiltasten navigieren durch Tab-Leisten, Radiogruppen und Menüs. Der `Tooltip` einer View erscheint sofort, wenn die Tastatur den Fokus darauf bewegt, bis der Fokus sie verlässt oder Escape gedrückt wird. `OnOpen` eines Popups läuft, sobald sein Inhalt vorhanden ist, sodass ein `Focus` dort den Fokus hineinbewegen kann, wie es bei einem nicht-modalen Dialog sein sollte; der Inhalt eines Popovers folgt seinem Auslöser in der Tab-Reihenfolge, und beim Schließen des Popups kehrt der Fokus zum Auslöser zurück.

Auf Android, iPhone und iPad können `Ref.Focus()` und das `AutoFocus` eines Buttons eine aktivierte, drückbare oder mit `Focusable` versehene View für eine Hardware-Tastatur fokussieren. Das Berühren eines Steuerelements verschiebt diesen Fokus nicht. Leertaste und Eingabetaste aktivieren das fokussierte drückbare Element nach der Tastenzuordnung (`keymap`) und erhalten Control, Alt, Shift sowie die Plattformtaste in jeder Druckphase. `NoFocus`, deaktivierte oder ausgeblendete Steuerelemente sowie Steuerelemente hinter einem modalen Dialog können diesen Fokus nicht annehmen. Das Fokussieren einer View öffnet keine Bildschirmtastatur; native Felder behalten ihr Bearbeitungsverhalten. watchOS und Mini-Programme haben keinen Aktivierungspfad für physische Tastaturen.

`cx.Env().ScreenReader()` gibt an, ob ein Screenreader aktiv ist, `cx.Env().ReduceMotion()`, ob weniger Bewegung gewünscht ist, `cx.Env().IncreaseContrast()`, ob mehr Kontrast gewünscht ist (Kontrast erhöhen auf macOS, iOS und watchOS, ein Windows-Kontrastdesign, GNOME Hoher Kontrast, die Kontrasteinstellung von GTK 4.20 oder ein kontrastreiches GTK-Design, die Kontrasteinstellung oberhalb von Standard ab Android 14, `prefers-contrast: more` oder `forced-colors: active` im Web), `cx.Env().FontScale()` die Textgröße und `cx.Env().RTL()` die Layout-Richtung. Texte folgen der Textgröße, sofern nicht `NoFontScaling` gesetzt ist, und ein Layout von rechts nach links spiegelt seine physischen Kanten, wenn es logische Kanten verwendet.

## Weiter

- [Styles und Themes](styles.md): Fokus- und Deaktivierungszustände.
- [Formulare](forms.md): Labels und Werte für Felder.
- [Canvas und Shader](canvas.md): Teile einer Zeichnung für die Barrierefreiheit bereitstellen.
