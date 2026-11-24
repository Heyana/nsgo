---
title: Dialog
description: Eine modale Oberfläche, ein Sheet und eine Warnmeldung.
group: Komponenten
order: 41
---

`ui.Dialog` ist eine modale Oberfläche über einer abgedunkelten App, auf Touchscreens ein Sheet von unten. `ui.Sheet` ist ein Panel von unten, und `ui.AlertDialog` bittet die Nutzerin oder den Nutzer um eine Bestätigung.

```go preview=dialog/basic height=360
open := core.NewSignal(cx, false)
ui.Dialog{
	Open:         open.Get,
	OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
	Title:        "Rename the file",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { open.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

## Öffnen und Schließen

Die App hält fest, ob der Dialog geöffnet ist. `OnOpenChange` erfährt jede Änderung, die der Dialog anfordert, zusammen mit einem `base.Reason`, der ein Drücken außerhalb von Escape oder Zurück unterscheidet; `OnClose` läuft, sobald er geschlossen ist. `NoCloseOnEscape` und `NoCloseOnBackdropPress` verhindern, dass Escape beziehungsweise ein Drücken außerhalb ihn schließen.

`Title` und `Description` stehen oben auf der Oberfläche – `TitleFunc` und `DescriptionFunc`, wenn sie sich ändern, während er geöffnet ist –, und `Footer` enthält seine Buttons. `Bare` überlässt die gesamte Oberfläche den `Children`, für einen Dialog mit eigenem Layout der App, den Desktops innerhalb des Oberflächenrands auf die Ecken zuschneiden, die `Style` und `StyleFunc` ihm geben; `Inset` legt auf Desktops seinen Mindestabstand zu den Fensterrändern fest.

Auf Touch-Sheets entfernt `Bare` nun den Standard-Greifer und den Innenabstand des Inhalts, sodass die `Children` das Panel ausfüllen. `NoGrabber` blendet nur den Greifer aus, `ContentStyle` und `ContentStyleFunc` passen den Inhaltscontainer an, und `TitleStyle` und `TitleStyleFunc` passen den Titel an. `Leading` und `Trailing` platzieren Nodes, etwa Buttons für Abbrechen und Fertig, an den Enden der Titelzeile, wobei der Titel zwischen ihnen zentriert ist. `StartingStyle` und `EndingStyle` passen den Übergang des Sheets an; nehmen Sie eine `Transition` darin auf, wenn Sie ein eigenes Timing benötigen.

`Style` wird über das Erscheinungsbild der Oberfläche gelegt. `StyleFunc` wird danach angewendet und folgt den Signals, die es liest – einschließlich der Darstellung –, solange der Dialog geöffnet ist und wenn er erneut geöffnet wird. Zum Beispiel folgt `StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }` dem Theme, wobei `colors := ui.ColorsOf(cx)`. Beide Felder funktionieren auch bei `ui.AlertDialog`. Eine Nullfarbe lässt die vorhandene Farbe unverändert; verwenden Sie eine von null verschiedene transparente Farbe wie `core.RGBA(0xFFFFFF, 0)`, um sie zu entfernen.

## Sheets

`ui.Sheet` ist ein Panel am unteren Rand über einem abgedunkelten Hintergrund; ein Drücken auf den Hintergrund, Escape oder Zurück ruft `OnClose` auf.

Setzen Sie `Label` oder `LabelFunc` für seinen barrierefreien Dialognamen und `TestID`, um das Panel in Tests zu finden. `LabelFunc` folgt seinen Signals, ohne den geöffneten Inhalt zu ersetzen. `CloseOnSwipe: true` ruft `OnClose` zusätzlich auf, nachdem ein Wischen nach unten die Schließschwelle überschritten hat; aktualisieren Sie `Open` in diesem Callback, um es zu schließen.

Setzen Sie `Style` oder `StyleFunc`, um Hintergrund, Eckradien und Innenabstand der Oberfläche anzupassen, während Greifer und Safe-Area-Behandlung erhalten bleiben. `StyleFunc` folgt den Signals, die es liest. Ein `CornerRadius` ohne `CornerRadii` rundet jede Ecke anstelle der standardmäßig abgerundeten Oberkante.

```go preview=dialog/sheet height=320
ui.Sheet{Open: filters.Get, OnClose: func() { filters.Set(false) }, Children: filterControls}
```

## Warnmeldungen

`ui.AlertDialog` bittet die Nutzerin oder den Nutzer, etwas zu bestätigen oder zur Kenntnis zu nehmen. Jede Aktion schließt ihn, ein Drücken außerhalb dagegen nicht.

```go preview=dialog/alert height=320
ui.AlertDialog{
	Open:         confirm.Get,
	OnOpenChange: func(v bool, _ base.Reason) { confirm.Set(v) },
	Title:        "Delete the note?",
	Description:  "This cannot be undone.",
	Actions: []ui.AlertAction{
		{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
		{Text: "Delete", Color: colors.Danger, Role: core.AlertDestructive, Default: true, OnPress: remove},
	},
}
```

Eine Aktion mit `Role: core.AlertCancel` läuft bei Escape oder Zurück, gefolgt von `OnOpenChange(false, base.ReasonCancel)`. `Default: true` gibt einer Aktion den Tastaturfokus, wenn sich die Warnmeldung auf Desktops, im Web und auf Apple TV öffnet, und lässt die Eingabetaste sie wählen; das Drücken einer Aktion führt `OnPress` aus, dann `OnOpenChange(false, base.ReasonSelect)`. Die Eingabetaste führt die Standardaktion aus, solange der Tastaturfokus auf keiner anderen Aktion und keinem Feld liegt: Liegt der Fokus auf einer anderen Aktion, drückt die Eingabetaste diese, und ein Dialog über der Warnmeldung behält die Eingabetaste für sich. Sobald eine Aktion ausgeführt wurde, schließt sich die Warnmeldung, unabhängig davon, was `OnOpenChange` tut, und führt keine weitere aus, bis `Open` wieder false und dann true wird. Tragen mehrere Aktionen dieselbe Markierung, gewinnt die erste. Ohne Markierungen schließt Escape, ohne eine Aktion auszuführen, und die Eingabetaste hat keine Standardaktion. `Role: core.AlertDestructive` beschreibt eine Aktion; `Variant` oder `Color` wählt ihr Erscheinungsbild.

Für die systemeigene Warnmeldung oder das systemeigene Aktionsblatt verwenden Sie `cx.ShowAlert` ([Plattformdienste](../services.md#hinweisdialoge-und-die-zwischenablage)).

## Verhalten

Ein Modal blockiert die App dahinter, übernimmt den Tastaturfokus und schließt sich bei Escape und Zurück, sofern nicht anders festgelegt. Auf Touchscreens ist ein Dialog ein Sheet, das einem Ziehen nach unten folgt und sich jenseits einer Schwelle schließt.

## Barrierefreiheit

`ui.Dialog` meldet `RoleDialog` oder seine explizit gesetzte `Role`, und `ui.AlertDialog` meldet auf Desktops und Touchscreens `RoleAlertDialog`. Beide lesen ihren Titel vor – oder `Label`, wenn der Titel nicht ausreicht –, gefolgt von der Beschreibung. Der Fokus wechselt beim Öffnen in einen Dialog und kehrt beim Schließen zurück; der Hintergrund ist kein Fokusstopp. Die Buttons im Footer behalten ihre Rollen und Labels.

## Plattform

Unter Android und auf Touchscreens ist ein Dialog ein Sheet von unten; auf Desktops ist er eine zentrierte Oberfläche mit Hintergrund. Der Inhalt ist derselbe; nur die Darstellung ändert sich.

## Siehe auch

- [Navigation](../navigation.md): Seiten, Tabs und Modals im Zusammenspiel.
- [Menu](menu.md): verankerte Befehle und Kontextmenüs.
- [Barrierefreiheit](../accessibility.md): Fokus und Dialogrollen.
- Die Seiten Dialog, Sheet und AlertDialog der Galerie.
- [Komponenten ohne Design](../authoring/base.md#dialog): `base.Dialog` und `base.Sheet`, mit den Darstellungsweisen von iOS und eigener Bewegung.
