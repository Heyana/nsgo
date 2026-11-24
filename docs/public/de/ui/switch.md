---
title: Schalter
description: Eine Einstellung, die an oder aus ist, in drei Größen.
group: Komponenten
order: 35
---

`ui.Switch` ist eine Einstellung, die sofort wirkt. Er hält keinen Wert: Die App besitzt ihn und aktualisiert ihn in `OnChange`.

```go preview=switch/basic
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}
```

## Größen

`ui.Switch` akzeptiert `Checked`, `OnChange`, `Label` und `Size` (`SizeDefault`, `SizeSmall` oder `SizeLarge`). Der Knopf gleitet, und die Spur wechselt ihre Farbe im Takt der Frame-Uhr der Plattform.

```go preview=switch/small
ui.Switch{Checked: sync.Get, OnChange: sync.Set, Label: "Sync automatically", Size: ui.SizeSmall}
```

Ein Schalter ist für eine Einstellung gedacht, die sofort gilt. Für eine Auswahl innerhalb eines Formulars verwenden Sie eine [Checkbox](checkbox.md); für eine von mehreren Optionen `ui.Radio`s in einer `ui.RadioGroup` oder ein [Select](select.md).

## Barrierefreiheit

Der Schalter meldet `RoleSwitch` und seinen An- oder Aus-Zustand. `Label` ist das, was assistive Technologien vorlesen, und der Text neben einem Schalter ist meist sein Label. Die Leertaste oder ein Drücken schaltet ihn um, wenn er den Tastaturfokus hat.

## Plattform

Das Standard-Steuerelement wird von der App gezeichnet und sieht daher überall gleich aus. `Variant: ui.SwitchSystem` fordert das Steuerelement der Plattform an: NSSwitch unter macOS, UISwitch unter iOS und den 52 × 32 dp großen Schalter von Material 3 unter Android. Andere Zielplattformen verwenden das Standardaussehen. Drücken, Fokus und der Screenreader stammen von der Plattform. Der Standardschalter eines Telefons ist bewusst genauso groß wie der eines Desktops; `SizeSmall` ist für dichte Zeilen gedacht, nicht für ein bestimmtes Gerät.

## Siehe auch

- [Checkbox](checkbox.md): eine oder mehrere Auswahlmöglichkeiten.
- [Formulare](../forms.md): Einstellungen in einem Formular.
- [Barrierefreiheit](../accessibility.md): Labels und Fokus.
- Die Seite Switch der Gallery.
- [Ungestylte Komponenten](../authoring/base.md#switch): `base.Switch`, für einen Schalter mit eigenem Aussehen.
