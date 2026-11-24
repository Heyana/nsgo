---
title: Switch
description: Un'impostazione attiva o disattiva, in tre dimensioni.
group: Componenti
order: 35
---

`ui.Switch` è un'impostazione che ha effetto immediato. Non contiene alcun valore: il valore appartiene all'app, che lo aggiorna in `OnChange`.

```go preview=switch/basic
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}
```

## Dimensioni

`ui.Switch` accetta `Checked`, `OnChange`, `Label` e `Size` (`SizeDefault`, `SizeSmall` o `SizeLarge`). Il cursore scorre e la traccia cambia colore seguendo il frame clock della piattaforma.

```go preview=switch/small
ui.Switch{Checked: sync.Get, OnChange: sync.Set, Label: "Sync automatically", Size: ui.SizeSmall}
```

Uno switch serve per un'impostazione che si applica subito. Per una scelta all'interno di un modulo, usa una [checkbox](checkbox.md); per una tra più opzioni, usa dei `ui.Radio` in un `ui.RadioGroup` oppure una [select](select.md).

## Accessibilità

Lo switch segnala `RoleSwitch` e il suo stato attivo o disattivo. `Label` è ciò che legge la tecnologia assistiva, e il testo accanto a uno switch ne è di solito l'etichetta. La barra spaziatrice o una pressione lo commutano quando ha il focus della tastiera.

## Piattaforma

Il controllo predefinito è disegnato dall'app, quindi ha lo stesso aspetto ovunque. `Variant: ui.SwitchSystem` richiede il controllo della piattaforma: NSSwitch su macOS, UISwitch su iOS e lo switch da 52 × 32 dp di Material 3 su Android. Gli altri target usano l'aspetto predefinito. Pressioni, focus e screen reader sono quelli della piattaforma. Lo switch predefinito di un telefono ha di proposito le stesse dimensioni di quello desktop; `SizeSmall` è pensato per righe dense, non per un tipo di dispositivo.

## Correlati

- [Checkbox](checkbox.md): una o più scelte.
- [Moduli](../forms.md): impostazioni in un modulo.
- [Accessibilità](../accessibility.md): etichette e focus.
- La pagina Switch della galleria.
- [Componenti senza stile](../authoring/base.md#switch): `base.Switch`, per uno switch con un aspetto personalizzato.
