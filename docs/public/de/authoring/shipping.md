---
title: "Texte, Tests und Veröffentlichung"
description: "Texte einer Komponente übersetzen, sie wie Nutzende testen und für Go- sowie TSX-Apps bereitstellen."
group: Komponenten schreiben
order: 74
---

## Texte

Eine Komponente gibt eigene Texte – wie etwa den Namen eines Buttons für Screenreader – über die Kataloge ihres Pakets aus, statt über String-Literale. Legen Sie in der Bibliothek ein `locales/`-Paket mit dem Katalog der Ausgangssprache sowie eine Datei an, die den Code generiert:

```yaml
# locales/en.yaml
clear:
  message: Clear
  description: What screen readers say of the button that empties a chip input.
```

```go
// Package locales says the chips' words in the language the app shows.
package locales

//go:generate go tool nsgo i18n
```

`go generate ./locales` erzeugt eine Funktion pro Meldung, die die Komponente mit ihrem Scope aufruft: `locales.Clear(cx)` gibt den Text in der Sprache aus, die die App anzeigt. Committen Sie die generierte `catalog_gen.go`, damit Apps allein mit Go bauen können. Fügen Sie für jede Sprache, in die Sie übersetzen, eine Datei hinzu (`zh-Hans.yaml`); eine App liefert weitere Sprachen in ihren eigenen Katalogen unter einem Schlüssel, der den Importpfad Ihres Pakets benennt – genau wie bei [NSGos eigenen Texten](../localization.md#die-eigenen-texte-von-nsgo).

## Tests

`nsgotest` führt eine Komponente headless so aus, wie sie von Nutzenden bedient wird. Hängen Sie sie in eine kleine Wurzel ein und interagieren Sie über Rolle und Text mit ihr:

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestToggle(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		return chips.Toggle{On: on.Get, OnChange: on.Set, Label: "Bold"}
	})
	app.Press("role:switch Bold")
	if !app.Find("role:switch Bold").Props.Checked {
		t.Fatal("the toggle did not turn on")
	}
}
```

Im Wurzelverzeichnis der Bibliothek erfasst `go tool nsgo test` jedes Paket, und `-target macos` oder eine andere Zielplattform führt dieselben Tests in einem nativen Fenster aus. Testen Sie beide Erscheinungsbilder und eine Pseudo-Locale (`-lang en-XA`), um hart codierte Farben und Texte aufzuspüren. [Testen](../testing.md) behandelt alles Weitere.

## Veröffentlichung

Eine Komponentenbibliothek ist ein Go-Modul: Apps laden es per `go get` und importieren seine Pakete. Trennen Sie die ungestylte und die gestylte Form in verschiedene Pakete, sofern beide existieren, sodass eine App mit eigenem Aussehen keine ungenutzten Styles importiert.

Die Seite [Komponenten](https://nsgo.dev/components/) der Website listet Bibliotheken auf, die andere verwenden können; fügen Sie ein Dokument und einen Screenshot unter `docs/public/components/` hinzu, um Ihre anzubieten.

TSX-Apps greifen auf Go-Komponenten zu, sobald diese in einer `init`-Funktion registriert sind:

```go
func init() {
	tsx.Register("example.com/chips", map[string]core.Node{"Toggle": Toggle{}})
}
```

`import { Toggle } from "example.com/chips"` übernimmt dann die Felder als Props ([TSX-Apps](../tsx.md#go-aus-tsx)).
