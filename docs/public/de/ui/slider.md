---
title: Slider
description: Ein Wert oder ein Bereich entlang einer Spur, mit der Tastatur und dem Zeiger.
group: Komponenten
order: 37
---

`ui.Slider` wählt einen Wert entlang einer Spur, per Ziehen, in Schritten und mit der Tastatur. Ein einzelner Wert ist ein einzelner Regler; `High` macht daraus einen Bereich mit zwei Reglern.

```go preview=slider/basic
volume := core.NewSignal(cx, 40.0)
ui.Slider{Value: volume.Get, OnChange: volume.Set, Min: 0, Max: 100, Step: 5, Label: "Volume"}
```

## Ein Bereich

Setzen Sie `High` und `OnHighChange`, um zwei Werte zu wählen. `Value` ist das untere Ende. `LowLabel` und `HighLabel` benennen die Regler für assistive Technologien.

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

## Schritte und Werte

`Step` rastet den Wert ein; ein Schritt von 0 lässt jeden Wert zu. `ValueText` ändert, was assistive Technologien als Wert vorlesen, und `Disabled` deaktiviert den Slider.

```go preview=slider/format
ui.Slider{Value: speed.Get, OnChange: speed.Set, Min: 0.5, Max: 2, Step: 0.25, Label: "Speed",
	ValueText: func(v float64) string { return fmt.Sprintf("%g×", v) }}
```

Verwenden Sie `MinFunc`, `MaxFunc` und `StepFunc`, wenn sich der Bereich oder der Schritt ändert, etwa wenn ein Video seine Dauer meldet. Sie haben Vorrang vor ihren statischen Feldern und aktualisieren den eingehängten Slider, wobei sein Fokus und ein laufender Ziehvorgang erhalten bleiben. Ein explizites `MaxFunc`, das 0 zurückgibt, erlaubt einen zusammengefallenen Bereich, bis die Dauer eintrifft; der Wert der App bleibt erhalten und wird innerhalb der aktuellen Grenzen angezeigt.

```go
ui.Slider{Value: position.Get, OnChange: position.Set, MaxFunc: duration.Get, Label: "Position"}
```

In TSX folgen `min={minimum()}`, `max={duration()}` und `step={step()}` ihren Signals an Ort und Stelle; `max={0}` lässt den Bereich zusammenfallen, wie es ein `MaxFunc` tut, das 0 zurückgibt.

## Richtung

`Axis: base.Vertical` stellt den Slider auf, mit seinem Minimum unten; geben Sie ihm in `Style` eine Höhe. `Inverted` legt das Minimum an das andere Ende: nach oben bei einem vertikalen Slider, ans Ende bei einem horizontalen. Die Pfeiltasten entlang der Spur folgen der Richtung. `ui.Progress` akzeptiert dieselben Felder `Axis` und `Inverted`.

```go
ui.Slider{Value: level.Get, OnChange: level.Set, Axis: base.Vertical, Label: "Level", Style: core.Style{Height: core.Pt(160)}}
```

## Verhalten

Ein Ziehvorgang folgt dem Zeiger und meldet `OnChange`, während er sich bewegt. Die Pfeiltasten bewegen einen fokussierten Regler um einen Schritt, Pos1 und Ende springen an die Enden, und Bild auf und Bild ab bewegen ihn weiter. `ui.Slider` markiert die Schritte unter der Spur, wenn `Step` größer als 0 ist, mit bis zu 60 Markierungen.

## Barrierefreiheit

Der Slider meldet `RoleAdjustable`, sein `Label` und seinen Wert; ein Bereich hat beide Regler mit eigenen Labels. `ValueText` wird anstelle der Zahl vorgelesen. Die Tastatur sowie die Erhöhen- und Verringern-Aktionen eines Screenreaders ändern ihn.

## Plattform

Die Spur wird von der App gezeichnet und ist überall gleich groß. Drücken, Fokus und der Screenreader stammen von der Plattform; ein Ziehen per Touch verhält sich wie ein Ziehen mit dem Zeiger, mit demselben Schritt.

## Siehe auch

- [Formulare](../forms.md): Slider in einem Formular.
- [Barrierefreiheit](../accessibility.md): Werte und einstellbare Rollen.
- Die Seiten Slider, RangeSlider und Rating der Gallery.
- [Ungestylte Komponenten](../authoring/base.md#slider): `base.Slider`, vertikal und mit eigenen Markierungen.
