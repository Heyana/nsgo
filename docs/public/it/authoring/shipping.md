---
title: Parole, test e pubblicazione
description: Traduci ciò che dice un componente, testalo come farebbe un utente e offrilo alle app Go e TSX.
group: Scrivere componenti
order: 74
---

## Parole

Un componente pronuncia le proprie parole, come il nome di un pulsante per gli screen reader, tramite i cataloghi del proprio pacchetto anziché tramite stringhe letterali. Inserisci un pacchetto `locales/` nella libreria con il catalogo della lingua sorgente e un file che genera il codice:

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

`go generate ./locales` scrive una funzione per messaggio, che il componente chiama con il proprio scope: `locales.Clear(cx)` lo pronuncia nella lingua mostrata dall'app. Effettua il commit del file `catalog_gen.go` generato, così le app compilano con il solo Go. Aggiungi un file per ogni lingua verso cui traduci (`zh-Hans.yaml`); un'app fornisce le altre nei propri cataloghi, sotto una chiave che indica il percorso di importazione del tuo pacchetto, come per [le parole proprie di NSGo](../localization.md#le-parole-di-nsgo).

## Test

`nsgotest` esegue un componente nel modo in cui lo usa un utente, in modalità headless. Montalo in una piccola radice e interagisci con esso per ruolo e testo:

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

Dalla radice della libreria, `go tool nsgo test` esegue ogni pacchetto, mentre `-target macos` o un altro target esegue gli stessi test in una finestra nativa. Testa entrambi gli aspetti e una pseudo-localizzazione (`-lang en-XA`) per individuare colori e parole codificati staticamente. [Test](../testing.md) copre il resto.

## Pubblicazione

Una libreria di componenti è un modulo Go: le app la scaricano con `go get` e importano i suoi pacchetti. Mantieni le forme senza stile e con stile in pacchetti separati quando esistono entrambe, così un'app con un proprio aspetto non importa stili che non usa.

La pagina [Componenti](https://nsgo.dev/components/) del sito elenca le librerie che altri possono usare; aggiungi un documento e uno screenshot in `docs/public/components/` per proporre la tua.

Le app TSX raggiungono i componenti Go una volta registrati, in una funzione `init`:

```go
func init() {
	tsx.Register("example.com/chips", map[string]core.Node{"Toggle": Toggle{}})
}
```

`import { Toggle } from "example.com/chips"` accetta quindi i campi come prop ([App TSX](../tsx.md#go-da-tsx)).
