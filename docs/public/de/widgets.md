---
title: Widgets
description: Die Views der App auf dem Home-Bildschirm und dem Sperrbildschirm von iPhones und iPads sowie auf dem Schreibtisch des Mac anzeigen.
group: Plattformfunktionen
order: 47
---

## Eine Widget-Art

Widgets zeigen einen Blick auf die App, ob sie läuft oder nicht: auf dem Home-Bildschirm und dem Sperrbildschirm von iPhones und iPads, auf dem Schreibtisch des Mac und in der Mitteilungszentrale sowie im Launcher von Android. Eine Art ist ein Widget, das die Galerie des Systems anbietet, mit den Familien, in denen es erhältlich ist:

```yaml
widgets:
  - kind: counter
    name: Counter                 # shown in the system's gallery of widgets
    description: Shows the count.
    families: [small, medium, circular]  # also large, extra_large, rectangular and inline
# apple: { app_group: group.dev.example.hello }  # the shared App Group, group.<id> by default
```

Auf Apple-Plattformen zeichnet es eine Widget-Erweiterung, die `build` und `dev` mit Xcodes Swift-Compiler kompilieren ([Bauen und ausliefern](building.md#widgets-und-live-aktivitäten)). Unter Android zeichnet die vorgebaute Java-Laufzeit dieselbe Zeitachse als App-Widgets. Beide zeigen den veröffentlichten Inhalt weiter an, wenn die App beendet wird.

Für einen Mac-App-Store-Build in der Sandbox setzen Sie `apple.app_group` und liefern macOS-Provisioning-Profile über `sign.macos_profile` und `sign.widgets_profile`. Sowohl die App als auch `<id>.widgets` müssen zum selben Team gehören und für diese Gruppe berechtigt sein. Zeitachsen und Bilder liegen in ihrem gemeinsamen Container; direkte und Entwicklungs-Builds für macOS behalten ihren bisherigen Ort im Home-Verzeichnis. [Store-Signierung](building.md#store-pakete) beschreibt die Build-Optionen.

`name` und `description` können Nachrichten in den Katalogen der App benennen. In diesem Fall verwenden die Widget-Galerie und der Platzhalter die Sprache der Nutzerin bzw. des Nutzers und greifen auf den Text des Quellkatalogs zurück, wenn eine Übersetzung fehlt. Ein Wert, der kein Katalogschlüssel ist, wird wörtlich angezeigt.

## Eine Zeitachse

`widget.Update` schreibt, was die Widgets einer Art von nun an zeigen, als Zeitachse aus Einträgen: Das System zeigt den Eintrag, dessen Datum zuletzt verstrichen ist, und dann jeden späteren zu seinem Datum, ohne die App. Ein `Date` mit Nullwert bedeutet jetzt. Ohne Einträge zeigen die Widgets den Namen und die Beschreibung der Art. Auf Apple-Plattformen aktualisiert eine App im Vordergrund ihre Widgets so oft sie will, eine im Hintergrund einige Dutzend Mal am Tag.

```go
err := widget.Update("counter", widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Count", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Text{Content: strconv.Itoa(n), Style: widget.Style{FontSize: 44, FontWeight: core.FontWeightBold}},
	}},
})
```

`Entry.Families` zeigt einer Familie anstelle von `Entry.View` eine eigene View, `Entry.Background` füllt das Widget dahinter aus, über die Ränder hinaus, die das System freihält, und `Entry.URL` wird mit der App geöffnet, wenn das Widget angetippt wird. `widget.Update` meldet `widget.ErrNoWidgets` in einer App, die ohne Widgets gebaut wurde, und `widget.ErrUnsupported` außerhalb von macOS, iOS und Android; `widget.Available` sagt vor einem Schreibvorgang, welcher Fall vorliegt. Es kann auf jeder Goroutine laufen, auch vor `nsgo.Run`.

## Views

Die Views eines Widgets sind keine Elemente von core: Sie werden wie in SwiftUI angeordnet – eine View ist so groß wie ihr Inhalt, sofern `Style` sie nicht dimensioniert oder füllen lässt, und ein Container zentriert seinen Inhalt, sofern `AlignItems` und `Justify` nichts anderes vorgeben. `Text`, `Symbol` und `Image` – aus Bytes oder aus einer Datei, die `Update` liest – zeigen Inhalte an; `Column`, `Row`, `Stack`, `Spacer` und `Divider` ordnen sie an; `Rectangle`, `Progress` und `Gauge` zeichnen; und `Date` zeigt eine Zeit in den Formaten des Systems, die `Relative`, `Offset` und `Timer` selbstständig weiterzählen lassen, ohne einen Eintrag für jede Sekunde.

```go
widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Next hour in", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Date{Time: hour, Format: widget.Timer, Style: widget.Style{TextStyle: widget.Title2, FontVariant: core.FontTabularNums}},
		widget.Progress{Value: 0.4, Style: widget.Style{Color: widget.Accent}},
	}},
}
```

`Style` akzeptiert einen Abstand, Padding, eine Größe oder Füllung, Ausrichtung, einen Hintergrund mit abgerundeten Ecken, die Textstile des Systems oder eine Schriftgröße, -stärke, -gestaltung und -variante, Zeilenbegrenzungen, einen `Link`, der mit der App geöffnet wird, und `Accented`, das eine View zu denen hinzufügt, die das System einfärbt. `widget.Primary`, `Secondary` und `Accent` folgen dem Erscheinungsbild, und `RGB` und `RGBA` nehmen mit `Dark` eine andere Farbe im Dunkelmodus an.

## Buttons

`widget.Button` führt ab iOS 17 und macOS 14 `widget.OnAction` aus, ohne die App zu öffnen. Ältere Systeme zeigen nur seine Kinder. Registrieren Sie den Handler in `main`, vor `nsgo.Run`: Das System kann die App ohne ihre Views im Hintergrund starten. Ein Widget-Druck hat `Activity == nil`; seine `Action` ist der Wert des Buttons.

```go
widget.Button{Action: "refresh", Children: []widget.View{
    widget.Text{Content: "Refresh"},
}}
```

Behandeln Sie die Aktion und rufen Sie `widget.Update` auf, bevor der Handler zurückkehrt, damit das automatische Neuladen der Zeitachse durch das System das Ergebnis sieht. Der Handler läuft außerhalb der Goroutine der Views; verwenden Sie `core.Scope.Post`, um Views zu ändern. Dies nutzt denselben Handler und denselben Intent wie [Live-Aktivitäten](live-activities.md) und benötigt nur `widgets` in `nsgo.yaml`, ohne `live_activities`.

## Android

Fügen Sie das Widget über die Widget-Auswahl des Launchers hinzu und ändern Sie dort seine Größe. Der Renderer wählt anhand seiner Abmessungen eine zulässige Familie: small unter 240 Punkten in beiden Dimensionen, medium ab 240 in der Breite, large ab 240 in der Höhe und extra-large ab 500 in der Breite; ist diese Familie nicht aufgeführt, verwendet er die erste aufgeführte Familie. `Entry.URL` und `Style.Link` öffnen die App über ein Schema, das sie in `url_schemes` deklariert. Ein Link auf einer verschachtelten View hat Vorrang vor dem Link ihres Containers.

Künftige Einträge der Zeitachse verwenden die ungenauen Alarme von Android, die das System verzögern kann, solange die App im Hintergrund ist. Timer-Views verwenden native Chronometer und ticken ohne die App; sie berücksichtigen derzeit Größe und Farbe, während eigene Schriftstärke, Schriftgestaltung und Kursivstil noch Ziele sind. Relative Datumsangaben werden nach dem Zeitplan des Systems aktualisiert. Der generische Renderer unterstützt Text, Bilder, Layout, Formen, Balken, Ringe und eine Reihe gängiger Symbol-Glyphen; verwenden Sie `Image` für Grafiken, die auf beiden Plattformen übereinstimmen müssen. Beliebige SF-Symbol-Namen bleiben ein offenes Ziel für Android. Android-Widget-Provider werden für eigenständige APKs und AABs erzeugt; eingebettete Hosts erhalten keine.

## Live-Aktivitäten

Dieselbe Erweiterung zeichnet die Live-Aktivitäten der App auf dem Sperrbildschirm und in der Dynamic Island ([Live-Aktivitäten](live-activities.md)).

## Weiter

- [Live-Aktivitäten](live-activities.md): was die App verfolgt, während es geschieht.
- [Mitteilungen](notifications.md): die Mitteilungen und Badges neben einem Widget.
- [Bauen und ausliefern](building.md): die App Group, die die Erweiterung teilt.
