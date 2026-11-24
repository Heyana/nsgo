---
title: Curseur
description: Une valeur, ou une plage, le long d’une piste au clavier et au pointeur.
group: Composants
order: 37
---

`ui.Slider` choisit une valeur le long d’une piste, par glisser, par palier et au clavier. Une valeur correspond à un seul bouton ; `High` forme une plage à deux.

```go preview=slider/basic
volume := core.NewSignal(cx, 40.0)
ui.Slider{Value: volume.Get, OnChange: volume.Set, Min: 0, Max: 100, Step: 5, Label: "Volume"}
```

## Une plage

Définissez `High` et `OnHighChange` pour choisir deux valeurs. `Value` est la borne inférieure. `LowLabel` et `HighLabel` nomment les boutons pour les technologies d’assistance.

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

## Paliers et valeurs

`Step` cale la valeur sur les paliers ; un palier de 0 accepte n’importe quelle valeur. `ValueText` modifie ce que les technologies d’assistance lisent comme valeur, et `Disabled` désactive le contrôle.

```go preview=slider/format
ui.Slider{Value: speed.Get, OnChange: speed.Set, Min: 0.5, Max: 2, Step: 0.25, Label: "Speed",
	ValueText: func(v float64) string { return fmt.Sprintf("%g×", v) }}
```

Utilisez `MinFunc`, `MaxFunc` et `StepFunc` lorsque la plage ou le palier change, par exemple lorsqu’une vidéo indique sa durée. Ils remplacent leurs champs statiques et mettent à jour le curseur monté, tout en conservant son focus et tout glissement en cours. Un `MaxFunc` explicite renvoyant 0 autorise une plage réduite en attendant que la durée arrive ; la valeur de l’application est conservée et affichée dans les bornes actuelles.

```go
ui.Slider{Value: position.Get, OnChange: position.Set, MaxFunc: duration.Get, Label: "Position"}
```

En TSX, `min={minimum()}`, `max={duration()}` et `step={step()}` suivent leurs signaux sur place ; `max={0}` réduit la plage, comme le fait un `MaxFunc` renvoyant 0.

## Orientation

`Axis: base.Vertical` redresse le curseur verticalement, avec son minimum en bas ; donnez-lui une hauteur dans `Style`. `Inverted` place le minimum à l’autre extrémité : en haut pour un curseur vertical, à la fin pour un curseur horizontal. Les touches fléchées le long de la piste suivent cette orientation. `ui.Progress` accepte les mêmes `Axis` et `Inverted`.

```go
ui.Slider{Value: level.Get, OnChange: level.Set, Axis: base.Vertical, Label: "Level", Style: core.Style{Height: core.Pt(160)}}
```

## Comportement

Un glissement suit le pointeur et signale `OnChange` pendant son déplacement. Les touches fléchées déplacent le bouton ayant le focus d’un palier, Début et Fin vont aux extrémités, et Page précédente et Page suivante avancent davantage. `ui.Slider` marque les paliers sous la piste lorsque `Step` est supérieur à 0, jusqu’à 60 repères.

## Accessibilité

Le curseur signale `RoleAdjustable`, son `Label` et sa valeur ; une plage possède les deux boutons avec leurs propres libellés. `ValueText` est lu à la place du nombre. Le clavier et les actions d’incrémentation et de décrémentation d’un lecteur d’écran le modifient.

## Plateforme

La piste est dessinée par l’application et a la même taille partout. Les appuis, le focus et le lecteur d’écran relèvent de la plateforme ; un glissement tactile se comporte comme un glissement au pointeur avec le même palier.

## Voir aussi

- [Formulaires](../forms.md) : les curseurs dans un formulaire.
- [Accessibilité](../accessibility.md) : valeurs et rôles ajustables.
- Les pages Slider, RangeSlider et Rating de la galerie.
- [Composants sans style](../authoring/base.md#slider) : `base.Slider`, vertical et avec vos propres repères.
