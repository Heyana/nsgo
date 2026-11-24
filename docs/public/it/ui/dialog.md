---
title: Dialog
description: Una superficie modale, un pannello inferiore e un avviso.
group: Componenti
order: 41
---

`ui.Dialog` è una superficie modale sopra l'app oscurata, un pannello dal basso sui touch screen. `ui.Sheet` è un pannello dal basso e `ui.AlertDialog` chiede all'utente di confermare.

```go preview=dialog/basic height=360
open := core.NewSignal(cx, false)
ui.Dialog{
	Open:         open.Get,
	OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
	Title:        "Rename the file",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { open.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

## Apertura e chiusura

È l'app a stabilire se la finestra di dialogo è aperta. `OnOpenChange` riceve ogni cambiamento richiesto dalla finestra di dialogo, con un `base.Reason` che distingue una pressione all'esterno da Esc o dal tasto indietro; `OnClose` viene eseguito una volta che si è chiusa. `NoCloseOnEscape` e `NoCloseOnBackdropPress` impediscono che Esc e una pressione all'esterno la chiudano.

`Title` e `Description` stanno in cima alla superficie, `TitleFunc` e `DescriptionFunc` quando cambiano mentre è aperta, e `Footer` ne contiene i pulsanti. `Bare` lascia l'intera superficie ai `Children`, per una finestra di dialogo con un layout proprio dell'app, che sui desktop viene ritagliata all'interno del bordo della superficie secondo gli angoli assegnati da `Style` e `StyleFunc`, e `Inset` imposta la distanza minima dai bordi della finestra sui desktop.

Sui pannelli touch, `Bare` ora rimuove la maniglia predefinita e il padding del contenuto, così i `Children` riempiono il pannello. `NoGrabber` nasconde solo la maniglia, `ContentStyle` e `ContentStyleFunc` personalizzano il contenitore del contenuto e `TitleStyle` e `TitleStyleFunc` personalizzano il titolo. `Leading` e `Trailing` collocano dei nodi, come i pulsanti Annulla e Fine, alle estremità della riga del titolo, con il titolo centrato tra loro. `StartingStyle` e `EndingStyle` personalizzano la transizione del pannello; includi una `Transition` al loro interno quando ti serve una temporizzazione personalizzata.

`Style` si fonde sopra l'aspetto della superficie. `StyleFunc` si fonde dopo di esso e segue i signal che legge, incluso l'aspetto chiaro o scuro, mentre la finestra di dialogo è aperta e quando viene riaperta. Ad esempio, `StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }` segue il tema, dove `colors := ui.ColorsOf(cx)`. Entrambi i campi funzionano anche su `ui.AlertDialog`. Un colore zero lascia invariato il colore esistente; usa un colore trasparente diverso da zero come `core.RGBA(0xFFFFFF, 0)` per azzerarlo.

## Pannelli inferiori

`ui.Sheet` è un pannello in basso sopra uno sfondo oscurato; una pressione sullo sfondo, Esc o il tasto indietro chiamano `OnClose`.

Imposta `Label` o `LabelFunc` per il suo nome accessibile di finestra di dialogo, e `TestID` per trovare il pannello nei test. `LabelFunc` segue i propri signal senza sostituire il contenuto aperto. `CloseOnSwipe: true` chiama `OnClose` anche dopo uno scorrimento verso il basso che supera la soglia di chiusura; aggiorna `Open` in quella callback per chiuderlo.

Imposta `Style` o `StyleFunc` per personalizzare lo sfondo, i raggi degli angoli e il padding della superficie mantenendo la maniglia e la gestione dell'area sicura. `StyleFunc` segue i signal che legge. Un `CornerRadius` senza `CornerRadii` arrotonda ogni angolo al posto della parte superiore arrotondata predefinita.

```go preview=dialog/sheet height=320
ui.Sheet{Open: filters.Get, OnClose: func() { filters.Set(false) }, Children: filterControls}
```

## Avvisi

`ui.AlertDialog` chiede all'utente di confermare o di prendere atto di qualcosa. Ogni azione lo chiude, mentre una pressione all'esterno no.

```go preview=dialog/alert height=320
ui.AlertDialog{
	Open:         confirm.Get,
	OnOpenChange: func(v bool, _ base.Reason) { confirm.Set(v) },
	Title:        "Delete the note?",
	Description:  "This cannot be undone.",
	Actions: []ui.AlertAction{
		{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
		{Text: "Delete", Color: colors.Danger, Role: core.AlertDestructive, Default: true, OnPress: remove},
	},
}
```

Un'azione con `Role: core.AlertCancel` viene eseguita con Esc o il tasto indietro, seguita da `OnOpenChange(false, base.ReasonCancel)`. `Default: true` dà a un'azione il focus della tastiera quando l'avviso si apre su desktop, web e Apple TV, e fa sì che Invio la scelga; la pressione di un'azione esegue `OnPress`, poi `OnOpenChange(false, base.ReasonSelect)`. Invio esegue l'azione predefinita finché il focus della tastiera non si trova su un'altra azione o su un campo: con il focus spostato su un'altra azione, Invio preme quella, e una finestra di dialogo sopra l'avviso si tiene Invio. Una volta eseguita un'azione, l'avviso si chiude, qualunque cosa faccia `OnOpenChange`, e non ne esegue altre finché `Open` non torna falso e poi di nuovo vero. Se più azioni portano lo stesso marcatore, vince la prima. Senza marcatori, Esc chiude senza eseguire alcuna azione e Invio non ha un'azione predefinita. `Role: core.AlertDestructive` descrive un'azione; `Variant` o `Color` ne sceglie l'aspetto.

Per l'avviso o l'action sheet propri del sistema, usa `cx.ShowAlert` ([Servizi di piattaforma](../services.md#alerts-and-the-clipboard)).

## Comportamento

Una finestra modale blocca l'app dietro di sé, prende il focus della tastiera e si chiude con Esc e il tasto indietro, a meno che non le sia indicato diversamente. Sui touch screen una finestra di dialogo è un pannello che segue un trascinamento verso il basso e si chiude oltre una soglia.

## Accessibilità

`ui.Dialog` segnala `RoleDialog`, oppure il suo `Role` esplicito, e `ui.AlertDialog` segnala `RoleAlertDialog` su desktop e touch screen. Entrambi leggono il proprio titolo, oppure `Label` quando il titolo non basta, seguito dalla descrizione. Il focus entra in una finestra di dialogo quando si apre e torna indietro quando si chiude; lo sfondo non è una tappa del focus. I pulsanti del footer mantengono i propri ruoli ed etichette.

## Piattaforma

Su Android e sui touch screen una finestra di dialogo è un pannello dal basso; sui desktop è una superficie centrata con uno sfondo. Il contenuto è lo stesso; cambia solo la presentazione.

## Correlati

- [Navigazione](../navigation.md): pagine, schede e finestre modali insieme.
- [Menu](menu.md): comandi ancorati e menu contestuali.
- [Accessibilità](../accessibility.md): focus e ruoli delle finestre di dialogo.
- Le pagine Dialog, Sheet e AlertDialog della galleria.
- [Componenti senza stile](../authoring/base.md#dialog): `base.Dialog` e `base.Sheet`, con le presentazioni di iOS e animazioni tutte tue.
