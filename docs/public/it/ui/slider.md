---
title: Slider
description: Un valore, o un intervallo, lungo una traccia, con la tastiera e il puntatore.
group: Componenti
order: 37
---

`ui.Slider` sceglie un valore lungo una traccia, con il trascinamento, il passo e la tastiera. Un singolo valore è un solo cursore; `High` crea un intervallo di due.

```go preview=slider/basic
volume := core.NewSignal(cx, 40.0)
ui.Slider{Value: volume.Get, OnChange: volume.Set, Min: 0, Max: 100, Step: 5, Label: "Volume"}
```

## Un intervallo

Imposta `High` e `OnHighChange` per scegliere due valori. `Value` è l'estremo inferiore. `LowLabel` e `HighLabel` danno un nome ai cursori per la tecnologia assistiva.

```go preview=slider/range
price := core.NewSignal(cx, 20.0)
ceiling := core.NewSignal(cx, 80.0)
ui.Slider{
	Value: price.Get, OnChange: price.Set,
	High: ceiling.Get, OnHighChange: ceiling.Set,
	Min: 0, Max: 100, Step: 1,
	Label: "Price", LowLabel: "Minimum", HighLabel: "Maximum",
}
```

## Passi e valori

`Step` aggancia il valore ai passi; un passo pari a 0 accetta qualsiasi valore. `ValueText` cambia ciò che la tecnologia assistiva legge come valore, e `Disabled` blocca lo slider.

```go preview=slider/format
ui.Slider{Value: speed.Get, OnChange: speed.Set, Min: 0.5, Max: 2, Step: 0.25, Label: "Speed",
	ValueText: func(v float64) string { return fmt.Sprintf("%g×", v) }}
```

Usa `MinFunc`, `MaxFunc` e `StepFunc` quando l'intervallo o il passo cambiano, ad esempio quando un video comunica la propria durata. Hanno la precedenza sui rispettivi campi statici e aggiornano lo slider montato, mantenendone il focus e qualsiasi trascinamento in corso. Una `MaxFunc` esplicita che restituisce 0 consente un intervallo collassato finché non arriva la durata; il valore dell'app viene preservato e mostrato entro i limiti correnti.

```go
ui.Slider{Value: position.Get, OnChange: position.Set, MaxFunc: duration.Get, Label: "Position"}
```

In TSX, `min={minimum()}`, `max={duration()}` e `step={step()}` seguono i rispettivi signal sul posto; `max={0}` collassa l'intervallo, come fa una `MaxFunc` che restituisce 0.

## Direzione

`Axis: base.Vertical` mette lo slider in verticale, con il minimo in basso; assegnagli un'altezza in `Style`. `Inverted` porta il minimo all'estremità opposta: in alto in uno slider verticale, alla fine in uno orizzontale. I tasti freccia lungo la traccia seguono la direzione. `ui.Progress` accetta gli stessi `Axis` e `Inverted`.

```go
ui.Slider{Value: level.Get, OnChange: level.Set, Axis: base.Vertical, Label: "Level", Style: core.Style{Height: core.Pt(160)}}
```

## Comportamento

Un trascinamento segue il puntatore e segnala `OnChange` durante il movimento. I tasti freccia spostano di un passo il cursore con il focus, Home e Fine portano alle estremità, e Pagina su e Pagina giù spostano di più. `ui.Slider` segna i passi sotto la traccia quando `Step` è maggiore di 0, fino a 60 tacche.

## Accessibilità

Lo slider segnala `RoleAdjustable`, la sua `Label` e il suo valore; un intervallo ha entrambi i cursori, ciascuno con la propria etichetta. `ValueText` viene letto al posto del numero. La tastiera e le azioni di incremento e decremento di uno screen reader lo modificano.

## Piattaforma

La traccia è disegnata dall'app e ha le stesse dimensioni ovunque. Pressioni, focus e screen reader sono quelli della piattaforma; un trascinamento al tocco si comporta come un trascinamento del puntatore, con lo stesso passo.

## Correlati

- [Moduli](../forms.md): slider in un modulo.
- [Accessibilità](../accessibility.md): valori e ruoli regolabili.
- Le pagine Slider, RangeSlider e Rating della galleria.
- [Componenti senza stile](../authoring/base.md#slider): `base.Slider`, verticale e con tacche personalizzate.
