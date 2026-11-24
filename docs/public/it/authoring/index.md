---
title: Scrivere componenti
description: Crea componenti che altri inseriscono nelle proprie app, proprio come sono fatti base e ui di NSGo.
group: Scrivere componenti
order: 70
---

Queste pagine sono pensate per gli autori di componenti: una libreria di controlli, un design system o una singola vista complessa che le app importano come pacchetto Go. Presuppongono la conoscenza di [Componenti e stato](../components.md) e [Stili e temi](../styles.md).

## Un componente

Un componente è un tipo dotato di un metodo `Render`, proprio come in un'app. I suoi campi sono ciò che l'app imposta; tutto ciò che l'app non imposta assume il valore zero, quindi il valore zero di ciascun campo dovrebbe rappresentare il caso abituale.

```go
package chips

import "github.com/nsgo-dev/nsgo/core"

// Chip is a short label in a pill.
type Chip struct {
	Text string
}

// Render implements core.Node.
func (c Chip) Render(*core.Scope) core.Node {
	return core.View{Class: "self-start rounded-full px-3 py-1", Children: []core.Node{
		core.Text{Content: c.Text, Class: "text-sm"},
	}}
}
```

Un'app lo usa come qualsiasi altro elemento: `chips.Chip{Text: "New"}`.

## Comportamento e aspetto

NSGo mantiene ogni controllo in due forme. `base` ne custodisce comportamento, stato e accessibilità senza alcun aspetto grafico, accettando ogni stile dall'app parte per parte; `ui` applica un aspetto alla forma `base`, così il comportamento vive in un unico posto. Una libreria può seguire la stessa separazione:

- Un componente **senza stile** gestisce pressioni, tastiera, focus e ciò che leggono gli screen reader, ed espone lo stile di ogni parte, come fa `base.Switch`. Le app e le altre librerie gli danno il proprio aspetto ([Parti e stati](parts.md)).
- Un componente **con stile** avvolge un componente senza stile e sceglie soltanto gli stili, dal tema in vigore, come fa `ui.Switch` ([Aspetto e temi](looks.md)).

Un componente privo di comportamento condivisibile, come un badge, può essere creato direttamente con il solo stile.

## Un pacchetto

- Tieni un componente per file, chiamandolo con il suo nome (`chip.go`), insieme ai tipi che usa soltanto lui; ciò che più componenti condividono va in un file dedicato.
- Dai a un componente complesso un pacchetto proprio, la cui API ometta il nome del componente, come `dock.Area` di `base/dock`.
- Inserisci le parole pronunciate da un componente in `locales/` del pacchetto ([Parole, test e pubblicazione](shipping.md)).
- Non fare nulla implicitamente in base al dispositivo: una dimensione o una forma che cambia su un telefono o su un desktop è un campo che imposta l'app.

## Prossimi passi

- [Parti e stati](parts.md): valori controllati, parti stilizzate e accessibilità.
- [Aspetto e temi](looks.md): colori dal tema, varianti e dimensioni.
- [Componenti senza stile](base.md): i controlli di NSGo privi di aspetto grafico, da stilizzare o su cui costruire.
- [Parole, test e pubblicazione](shipping.md): traduzioni, test e TSX.
