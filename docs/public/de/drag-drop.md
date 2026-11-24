---
title: Drag and Drop
description: Anwendungswerte zwischen Views innerhalb eines Fensters übertragen.
group: Kernkonzepte
order: 57
---

`base/dragdrop` verbindet eine Quelle und ein Ziel in derselben `Area` innerhalb eines Fensters. Die App behält ihre Views, Styles, Auswahl und die Aktion, die den Wert verarbeitet. Eine Quelle startet standardmäßig nach vier Punkten Bewegung, sodass ein normaler Druck erhalten bleibt.

```go
import "github.com/nsgo-dev/nsgo/base/dragdrop"

dragdrop.Area{Children: []core.Node{
	dragdrop.Source{
		View: core.View{
			Label: "Film", TestID: "poster",
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Poster"}},
		},
		Data: func() any { return filmID },
		Preview: func(any) core.Node { return core.Text{Content: "Film"} },
	},
	dragdrop.Target{
		View: core.View{
			TestID: "playlist",
			Style: core.Style{Width: core.Pt(160), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Playlist"}},
		},
		Accept: func(value any) bool { _, ok := value.(string); return ok },
		OnDrop: func(e dragdrop.Event) { addToPlaylist(e.Data.(string)) },
	},
}}
```

`Data` wird einmal ausgeführt, wenn der Ziehvorgang beginnt. Geben Sie ein Slice zurück, um mehrere ausgewählte Einträge gemeinsam zu transportieren; kopieren Sie dort veränderliche Daten, falls sie ein Schnappschuss bleiben müssen. Ein `nil`-Rückgabewert von `Data` deaktiviert das Ziehen. `Threshold` legt die erforderliche Bewegung fest. `Activation: dragdrop.AfterLongPress` wartet zuerst `View.DelayLongPress` (standardmäßig 500 ms) ab und dann auf Bewegung; wählen Sie dies ausdrücklich dort, wo ein Wischen zuerst scrollen soll.

`View` behält seine gewöhnlichen Props bei, einschließlich Press-Handlern und Barrierefreiheit. Eine Quelle besitzt `View.Responder`; lassen Sie es ungesetzt. Ihre Kind-Elemente können einen Button mit seiner normalen Aktion enthalten. `Preview` folgt dem Zeiger, ohne Eingaben entgegenzunehmen oder die Quelle neu zu mounten; ohne Vorschau wird das Label der Quelle angezeigt. Geben Sie einer Quelle ein Label oder eine Vorschau. Verschachtelte Areas arbeiten unabhängig voneinander.

Ziele benötigen `OnDrop`, um einen Wert anzunehmen. Das innerste Ziel erhält die erste Chance; wenn `Accept` den Wert ablehnt, kann ihn ein akzeptierender Vorfahre übernehmen. `OnEnter`, `OnMove` und `OnLeave` können das Erscheinungsbild des Ziels aktualisieren. Jedes Event enthält `Data`, `Position` im Ziel und `PagePosition` auf der Oberfläche. Das Deaktivieren oder Entfernen des aktiven Ziels löscht seinen betretenen Zustand; beim Loslassen wird die Akzeptanz erneut geprüft. Ein Ziel hinter einem anderen View, beschnitten, deaktiviert oder unter einem Modal kann keinen Drop empfangen.

`OnStart` und `OnEnd` beschreiben die Sitzung der Quelle. `OnEnd` erhält `Result{Data, Dropped, Target}`. Escape, ein nativer Touch-Abbruch, das Deaktivieren oder Entfernen der Quelle oder das Loslassen außerhalb eines akzeptierenden Ziels beendet den Vorgang ohne Drop. Die Zielplatzierung nutzt die zuletzt committete Geometrie; native Animationen zwischen Commits werden nicht abgetastet. Dies überträgt Werte innerhalb des Fensters; Systemdateien verwenden weiterhin `OnFileDrop`.

Behalten Sie ein Menü, einen Tastaturbefehl oder eine Aktion für Barrierefreiheit bei, die denselben Vorgang ausführt. Ziehen ist eine Abkürzung zu dieser Aktion. Tests verwenden `app.DragTo("id:poster", "id:playlist")` oder `DragTo` einer ausgewählten Oberfläche; TSX-Tests verwenden `await app.dragTo("id:poster", "id:playlist")`. Beide Views müssen bereits in das sichtbare Fenster passen; der Schritt sendet eine Touch-Sequenz durch den Responder, ohne einen Drop künstlich zu synthetisieren oder während des Ziehens zu scrollen. Siehe [Tests](testing.md).
