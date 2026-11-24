---
title: Input
description: Das Textfeld der Plattform, kontrolliert oder unkontrolliert, mit Fokus und Bildschirmtastatur.
group: Komponenten
order: 33
---

`ui.Input` ist das Textfeld der Plattform im Erscheinungsbild von `ui`, einzeilig, sofern nicht `Multiline` gesetzt ist. Es besitzt die Felder von `base.Input`, wie jedes gestaltete Feld; `ui.Field` ist dasselbe Feld ohne eigene Größe, in einem Rahmen, der weder dem Zeiger noch dem Fokus folgt; `core.Transparent` in seinem `Background` und `core.Zero` in seiner `BorderWidth` schalten den Kasten ab – für Text, der wie gewöhnlicher Text wirkt, sich aber bearbeiten lässt.

```go preview=input/basic
name := core.NewSignal(cx, "")
ui.Input{
	Value:       name.Get,
	OnChange:    name.Set,
	Placeholder: "Your name",
	TestID:      "name",
}
```

## Kontrolliert und unkontrolliert

Mit `Value` ist das Feld kontrolliert: Die App hält den Wert und aktualisiert ihn in `OnChange`. Mit `DefaultValue` verwaltet das Feld seinen eigenen Wert. Ein kontrolliertes Feld lässt sich leeren, indem sein Wert auf `""` gesetzt wird.

```go preview=input/submit
draft := core.NewSignal(cx, "")
ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "New task",
	OnSubmit: func(string) { add(); draft.Set("") }}
```

## Felder

`Multiline` bricht um, beginnt drei Zeilen hoch und wächst mit seinem Text, sofern sein Style nicht `Height` oder `MinHeight` setzt. `Masked` verbirgt die Zeichen, `Readonly` unterbindet Bearbeitungen, während sich der Text weiterhin markieren und kopieren lässt, `Disabled` schaltet das Feld ab, und `MaxLength` begrenzt die UTF-16-Einheiten, die die Nutzerin oder der Nutzer eingibt. `Placeholder` kann mit `PlaceholderFunc` eine Funktion sein.

```go
ui.Input{Masked: true, Value: password.Get, OnChange: password.Set,
	AutoComplete: core.AutoCompletePassword}
```

`KeyboardType` wählt eine Bildschirmtastatur (`KeyboardEmail`, `KeyboardNumber`, `KeyboardURL` und weitere), `ReturnKeyType` beschriftet ihre Eingabetaste, und `SubmitBehavior` legt fest, ob die Eingabetaste absendet oder in einem mehrzeiligen Feld eine neue Zeile beginnt. `AutoCapitalize`, `NoAutoCorrect` und `AutoComplete` steuern die Hilfen des Systems.

`InputSource` wählt eine Tastatur-Eingabequelle, solange das Feld den Fokus hat, und stellt die der Nutzerin oder des Nutzers wieder her, wenn es ihn verliert, sofern nicht `KeepInputSource` gesetzt ist: `"com.apple.keylayout.ABC"` hält ein Feld für Codes oder Befehle unter macOS auf Englisch. Die ID ist die der Plattform – unter Windows eine Tastaturlayout-ID wie `"00000409"` und unter Linux eine Eingabemethode von Fcitx 5 oder IBus; eine nicht aktivierte Quelle, Smartphones und das Web ändern nichts.

```go
ui.Input{Placeholder: "Bundle ID", InputSource: "com.apple.keylayout.ABC"}
```

`Style` wird über das Erscheinungsbild des Feldes gelegt und `StyleFunc` darüber: Was sie setzen, etwa Höhe, Innenabstand, Schrift oder eine Farbe, ersetzt die Werte von `ui`, der Rest bleibt erhalten. Der Rahmen dunkelt unter dem Zeiger ab und nimmt im Fokus die Akzentfarbe an, solange keines von beiden `BorderColor` oder `Hover` setzt. Ein `Padding` von null belässt den Wert von `ui`; `core.Zero`, eine gesetzte Länge, entfernt ihn (`Padding: core.All(core.Zero)`).

```go
ui.Input{Placeholder: "Filter", Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
ui.Field{DefaultValue: "Untitled", Style: core.Style{FontSize: 24, FontWeight: ui.Bold,
	Background: core.Transparent, BorderWidth: core.Zero}}
```

## Fokus und Ereignisse

`Ref` fokussiert das Feld und hebt den Fokus wieder auf. `OnFocus` und `OnBlur` folgen dem Fokus, `OnSelectionChange` und `OnCaretMove` der Auswahl und der Einfügemarke, `OnKeyPress` jeder getippten Taste, `OnPaste` dem Eingefügten und `OnCompositionChange` einer Eingabemethode beim Zusammensetzen.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Die Bildschirmtastatur verdeckt den unteren Teil eines Smartphones. Setzen Sie `core.EnvKeyboard` auf den unteren Innen- oder Außenabstand des Inhalts, damit er oberhalb der Tastatur bleibt; die Verschiebung wird mit ihr animiert. `cx.DismissKeyboard()` schließt die Tastatur, und `cx.Env().KeyboardHeight()` liest ihre Höhe.

## Validierung

Die Regeln gehören der App. Ein Signal hält den Fehler, `OnBlur` oder `OnSubmit` setzt ihn, und eine Meldung unter dem Feld zeigt ihn an ([Formulare](../forms.md#validierung)).

```go preview=input/validation
err := core.NewSignal(cx, "")
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{Value: email.Get, OnChange: email.Set, Label: "Email",
		OnSubmit: func(string) {
			if !strings.Contains(email.Get(), "@") {
				err.Set("Enter an email address.")
			} else {
				err.Set("")
			}
		}},
	ui.Textf(err.Get, 12, ui.Regular, colors.Danger),
}}
```

## Barrierefreiheit

Das Feld meldet `RoleTextField` – oder `RoleSearchField` für ein Suchfeld, wenn seine `Role` dies angibt – und liest sein `Label` vor; `Placeholder` ist kein Label, geben Sie einem Feld ohne sichtbare Beschriftung also ein `Label`. `Readonly` und `Disabled` werden gemeldet, und `OnSubmit` ist die Aktion der Tastatur. Einzeilige, maskierte und mehrzeilige Felder stellen den aktuellen `Placeholder` unter macOS, iOS und Linux als nativen Barrierefreiheits-Platzhalter oder -Hinweis bereit, der sich aktualisiert und leert, ohne dass das gezeichnete Platzhalter-Label eines mehrzeiligen Feldes einen zweiten Fokusstopp erzeugt.

## Plattform

iOS, Android, Windows und Linux verwenden ihre nativen Felder; das Web verwendet ein `<input>` oder `<textarea>`. Bildschirmtastatur, automatisches Ausfüllen, Eingabemethoden und Textrichtung stammen von der Plattform. Fokus und Eingabe werden auf jeder Zielplattform auf dieselbe Weise getestet.

## Siehe auch

- [Formulare](../forms.md): Felder, Validierung, Auswahlmöglichkeiten und die Tastatur.
- [Barrierefreiheit](../accessibility.md): Labels und Fokus.
- [Layout](../layout.md): Inhalt oberhalb der Tastatur halten.
- Die Seiten Input und Form der Galerie.
