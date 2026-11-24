---
title: Checkbox
description: Una casella di controllo, selezionata, non selezionata o indeterminata.
group: Componenti
order: 34
---

`ui.Checkbox` è una casella di controllo con l'aspetto di `ui`, affiancata da un'etichetta. Non contiene alcun valore: è l'app a stabilire se è selezionata e ad aggiornarlo in `OnChange`.

```go preview=checkbox/basic
done := core.NewSignal(cx, false)
ui.Checkbox{Checked: done.Get, OnChange: done.Set, Text: "Remember me"}
```

## Contenuto

`ui.Checkbox` mostra `Text` oppure i `Children` dell'app accanto alla casella. `Label` sostituisce entrambi per la tecnologia assistiva e `Hint` indica a cosa serve.

## Stati

`Indeterminate` non è né selezionato né non selezionato, per un «seleziona tutto» su una selezione parziale; una pressione richiede quindi lo stato selezionato. `Disabled` blocca le pressioni e il focus.

```go preview=checkbox/indeterminate height=220
ui.Checkbox{
	Checked:       func() bool { return chosen.Get() == len(files) },
	Indeterminate: func() bool { return chosen.Get() > 0 && chosen.Get() < len(files) },
	OnChange:      chooseAll,
	Text:          "Select all",
}
```

## Modificatori della pressione

Usa `OnChangeEvent` al posto di `OnChange` quando un elenco richiede la selezione di un intervallo con Maiusc+pressione:

```go
ui.Checkbox{Checked: done.Get, Text: "Remember me", OnChangeEvent: func(next bool, press core.PressEvent) {
	if press.Modifiers.Shift {
		chooseRange(next)
		return
	}
	done.Set(next)
}}
```

Se impostata, questa callback sostituisce `OnChange`; l'app aggiorna comunque il valore. L'attivazione da tastiera su desktop e web conserva i modificatori dopo le scorciatoie dell'app; su Windows, Alt con Spazio o Invio va invece al menu della finestra. `onChangeEvent` in TSX riceve `{ checked, press }`.

## Accessibilità

La checkbox segnala `RoleCheckbox` e il proprio stato selezionato o indeterminato. Il suo nome è `Text`, `Label` o il testo dei figli. Spazio la attiva o disattiva quando ha il focus della tastiera e uno screen reader annuncia il nuovo stato.

## Piattaforma

La casella è disegnata dall'app, non da un controllo della piattaforma, quindi ha lo stesso aspetto ovunque. La pressione, l'anello di focus e lo screen reader sono quelli della piattaforma. Viene testata per ruolo ed etichetta su ogni target.

## Correlati

- [Switch](switch.md): un'impostazione che ha effetto immediato.
- [Moduli](../forms.md): scelte in un modulo.
- [List](list.md): checkbox nelle righe.
- La pagina Checkbox della galleria.
- [Componenti senza stile](../authoring/base.md#checkbox): `base.Checkbox`, per una casella con un aspetto tutto tuo.
