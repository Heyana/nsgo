---
title: Aspetto e temi
description: Prendi i colori dal tema in vigore e dota un componente di varianti e dimensioni.
group: Scrivere componenti
order: 72
---

Un componente con stile sceglie gli stili e nient'altro: avvolge un componente senza stile, in un pacchetto dedicato come `chipsui`, e mantiene lì il comportamento. I suoi colori provengono dal tema, così segue il tema dell'app, il tema proprio di un pannello e l'aspetto chiaro o scuro senza dover essere ricostruito.

## Colori dal tema

Leggi i colori all'interno di una `StyleFunc`: viene rieseguita quando ciò che ha letto cambia, quindi un cambio di tema o di aspetto ristilizza il componente sul posto. `cx.Palette()` restituisce la tavolozza mostrata dove si trova il componente e `ui.ColorsOf(cx)` assegna un nome ai ruoli usati da `ui`.

```go
// Toggle is chips.Toggle in the theme's colors.
type Toggle struct {
	On       func() bool
	OnChange func(bool)
	Label    string
}

func (t Toggle) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	on := func() bool { return t.On != nil && t.On() }
	return chips.Toggle{On: t.On, OnChange: t.OnChange, Label: t.Label,
		Class: "flex-row items-center gap-2 rounded-full px-3 py-1",
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.Accent()}
			}
			return core.Style{Background: colors.Inset()}
		},
		DotClass: "size-2 rounded-full",
		DotStyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.OnAccent()}
			}
			return core.Style{Background: colors.Faint()}
		},
		Children: []core.Node{ui.Text(t.Label, 13, ui.Medium, colors.Text)},
	}
}
```

I colori codificati staticamente si rompono in modalità scura e sotto il tema di un'app; ricava ogni colore da un ruolo.

## Ruoli personalizzati

Un colore per cui la tavolozza non prevede un ruolo va in `Palette.Colors` sotto un nome scelto da te, impostato in entrambi gli aspetti. Le app lo sovrascrivono da codice o nel proprio file del tema, sotto `light` e `dark`:

```go
theme := core.DefaultTheme()
theme.Light.SetColor("chip", core.RGB(0xeef5ff))
theme.Dark.SetColor("chip", core.RGB(0x1b2a3d))
cx.SetTheme(theme)
```

Un componente lo legge con `cx.Palette().Color("chip")`, che segnala se il tema lo contiene o meno, così il componente può ripiegare su un ruolo predefinito quando manca. Gradienti, ombre e altre finiture non sono colori: tienili nel codice, scelti tramite `cx.Dark()`.

## Varianti

Un componente con più aspetti grafici accetta `Variant`, un tipo dedicato il cui valore zero corrisponde all'aspetto predefinito. Esso nomina esclusivamente gli aspetti che il componente disegna, colore incluso, anziché campi booleani o un tipo colore condiviso:

```go
// ToggleVariant is a toggle's look.
type ToggleVariant uint8

const (
	// ToggleSolid fills the pill with the accent when on.
	ToggleSolid ToggleVariant = iota
	// ToggleOutline draws its edge in the accent instead.
	ToggleOutline
)
```

## Dimensioni

La maggior parte dei componenti richiede una sola dimensione, il che mantiene i controlli allineati senza che l'app debba scegliere. Un componente che ne richiede di più accetta `Size`, un `ui.Size` tra predefinito, piccolo o grande, in modo da corrispondere ai 28, 24 e 32 punti di `ui.Button`; `Size.Height()` fornisce l'altezza con cui allineare gli altri controlli. Un componente che mantiene la stessa forma a qualsiasi scala, come un avatar, accetta invece `Size` in punti.
