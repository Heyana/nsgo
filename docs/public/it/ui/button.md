---
title: Button
description: Un pulsante, le sue varianti e dimensioni, e il pulsante a icona.
group: Componenti
order: 31
---

`ui.Button` è un pulsante con l'aspetto di `ui`, con le varianti e le dimensioni di cui una finestra ha bisogno. `OnPress` viene eseguito al completamento di una pressione.

```go preview=button/basic
ui.Button{Text: "Save", OnPress: save}
ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: cancel}
```

## Varianti e dimensioni

`ui.Button.Variant` è `ButtonPrimary` (il predefinito), `ButtonSecondary`, `ButtonOutline`, `ButtonGhost`, `ButtonTinted` o `ButtonDanger`. Tinted e danger usano il colore di accento o di pericolo del tema su un riempimento tenue. Sugli idiomi desktop e TV, `Size` è `SizeDefault` (28 punti), `SizeSmall` (24) o `SizeLarge` (32). Sugli idiomi touch, le stesse dimensioni sono 44, 40 e 48 punti, così il pulsante si può usare senza ridefinirne lo stile.

```go preview=button/variants
ui.Button{Text: "Delete", Variant: ui.ButtonOutline, OnPress: remove}
ui.Button{Text: "Continue", Size: ui.SizeLarge, OnPress: next}
ui.Button{Text: "Save", Icon: lucide.Check, Flex: 1, OnPress: save}
```

`Color` riempie il pulsante con un colore scelto dall'app anziché con quello di una variante, e `Tint` ne colora l'icona. `Flex` gli assegna una parte di una riga, mentre `Style` ne definisce il layout: può far sì che il pulsante riempia il proprio contenitore, impostare una dimensione scelta dall'app, aggiungere margini o arrotondarne gli angoli; `TestID` gli dà un nome per i test.

```go
ui.Button{Text: "Continue", Size: ui.SizeLarge,
    Style: core.Style{Width: core.Pct(100), Height: core.Pt(50)}, OnPress: next}
```

Un pulsante composto solo da un'icona è `ui.IconButton`, con `Variant` `IconButtonGhost`, `IconButtonRaised` o `IconButtonFilled`, un cerchio piatto nel colore inset del tema o in `Fill`, una `Size` in punti e un `Label` per le tecnologie assistive. Sui desktop l'etichetta compare anche come tooltip quando il puntatore si ferma sul pulsante; `Tooltip` vi mostra un testo diverso, e uno che restituisce `""` non ne mostra alcuno. Anche un `ui.Button` con `Icon` e senza `Text` è quadrato, e ha bisogno di `Label`.

```go preview=button/icon
ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost, OnPress: remove}
```

## Contenuto e stati

`ui.Button` mostra `Text`, un `Icon` iniziale facoltativo, o entrambi. Usa `TextStyle` per personalizzare la dimensione del carattere, il peso o altri stili di testo dell'etichetta.

```go
ui.Button{Text: "Saving…", Disabled: saving.Get, OnPress: save}
```

`Disabled` accetta una funzione, così il pulsante segue un signal: un pulsante disabilitato non accetta né pressioni né il focus della tastiera, e il suo aspetto è quello dello stato disabilitato, non un colore scelto dall'app.

`Busy` serve per un'azione ancora in corso. Finché restituisce true, uno spinner nel colore del testo del pulsante compare sopra il contenuto, quindi è visibile anche sull'accento di un pulsante primario, le pressioni non hanno effetto e le tecnologie assistive annunciano che è occupato. Per impostazione predefinita il contenuto conserva il proprio spazio sotto lo spinner. Il pulsante mantiene il focus della tastiera. `BusyLabel` o `BusyLabelFunc` mostra un messaggio accanto allo spinner e lo segnala come nome accessibile del pulsante finché è occupato. Il pulsante mantiene almeno la propria larghezza a riposo.

```go preview=button/busy
ui.Button{Text: "Save", Busy: saving.Get, BusyLabel: "Saving…", OnPress: save}
```

`ui.Spinner` e `ui.ProgressRing` accettano un proprio `Tint` per qualsiasi altra superficie su cui il loro accento scomparirebbe.

## Link

`ui.Link{Text: "Docs", Href: "/docs"}` apre una destinazione con il ruolo di link. Il suo testo e la sottolineatura al passaggio del puntatore usano l'accento, a meno che `Style.Color` non imposti un altro colore; `Style.Hover.Color` imposta il colore al passaggio del puntatore.

## Accessibilità

Il pulsante segnala `RoleButton` e il suo testo come nome. `Label` sostituisce il testo per le tecnologie assistive. Un pulsante con sola icona senza `Label` è illeggibile, quindi dagli un nome.

Invio e Spazio premono un pulsante con il focus sui desktop, sul web e su Apple TV; lo screen reader di un telefono lo preme come un clic.

## Piattaforma

Il pulsante è una vista disegnata allo stesso modo ovunque, non un pulsante della piattaforma; pressioni, focus e accessibilità provengono dalla piattaforma. Gli idiomi touch usano aree di layout più grandi, come descritto sopra.

## Correlati

- [Moduli](../forms.md): pulsanti in un modulo e validazione.
- [Stili e temi](../styles.md): il tema da cui un pulsante prende i propri colori.
- [Accessibilità](../accessibility.md): ruoli, etichette e focus.
- La pagina Button della galleria.
- [Componenti senza stile](../authoring/base.md#button): `base.Button`, per un pulsante con un aspetto tutto tuo.
