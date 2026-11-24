---
title: Componenti senza stile
description: I controlli di NSGo privi di aspetto grafico, da stilizzare parte per parte o su cui costruire una libreria con stile.
group: Scrivere componenti
order: 73
---

`base` contiene ogni controllo presente in `ui`, con il proprio comportamento, stato e accessibilità, ma senza alcun aspetto grafico. Ogni parte accetta una `Class`, uno `Style` e una `StyleFunc` ([Parti e stati](parts.md)), così una libreria o un'app con un proprio design li usa al posto di `ui`, oppure li avvolge come fa `ui` ([Aspetto e temi](looks.md)). Questa pagina tratta i controlli del [riferimento ai componenti](../ui.md); la galleria mostra ciascuno accanto alla sua forma `ui`.

Controlli come `Button`, `Checkbox`, `Select`, `Slider` e `NumberInput` accettano `Invalid: rejected.Get` per seguire un segnale di validazione. Stilizza quello stato con `Class: "aria-invalid:border-red-500"` o `Style.AriaInvalid`. Aggiorna accessibilità e stili senza disabilitare il controllo. I campi composti lo trasmettono al proprio input reale e alle parti decorative; `ColorPicker` lo condivide con i propri controlli.

Per i valori dell'applicazione trascinati tra viste, usa [`base/dragdrop`](../drag-drop.md): un'area collega sorgenti e destinazioni mentre l'app fornisce contenuto, anteprima e aspetto.

## Button

`base.Button` ricava il proprio aspetto da `Style` e il proprio contenuto da `Children`. `OnPress` viene eseguito a pressione completata; `OnPressDown` e `OnPressUp` seguono l'inizio e la fine di una pressione, per gestire un feedback personalizzato.

```go
base.Button{
	Label:   "Save",
	OnPress: func(core.PressEvent) { save() },
	Style: core.Style{Background: core.RGB(0x007aff), Padding: core.Symmetric(8, 14), CornerRadius: 8, AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.85)}},
	Children: []core.Node{core.Text{Content: "Save", Style: core.Style{Color: core.RGB(0xffffff)}}},
}
```

`Disabled`, `Busy`, `Selected` ed `Expanded` segnalano uno stato all'app, allo stile e alla tecnologia assistiva: un'azione ancora in esecuzione usa `Busy` (stilizzata con `aria-busy:`), un selettore o l'elemento corrente di un elenco usa `Selected`, un pannello espandibile `Expanded`. `NoFocus` esclude un pulsante dai punti di tabulazione quando una riga o un elenco lo raggiunge in altro modo.

## Checkbox

`base.Checkbox` accetta uno stile per il riquadro, uno per l'indicatore e il tuo segno al suo interno, mostrato in base al valore posseduto dall'app.

```go
base.Checkbox{
	Checked:        done.Get,
	OnChange:       done.Set,
	Label:          "Remember me",
	Style:          core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), BorderWidth: 1, CornerRadius: 5},
	Indicator:      core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14}},
	Children:       []core.Node{core.Text{Content: "Remember me"}},
}
```

L'indicatore eredita il proprio stato dal riquadro tramite `PartStates`, così le classi `checked:` lo stilizzano senza renderlo un secondo punto di tabulazione.

Per i modificatori, imposta `OnChangeEvent func(checked bool, press core.PressEvent)`. Sostituisce `OnChange` e lascia il valore all'app; `press.Modifiers.Shift` consente a un elenco di selezionare un intervallo. L'attivazione tramite Spazio o Invio su desktop, web, Android e iOS/tvOS preserva i modificatori a meno che una scorciatoia dell'app non gestisca prima il tasto; su Windows, Alt con Spazio o Invio apre invece il menu della finestra. In TSX, `onChangeEvent` riceve `{ checked, press }`.

## Switch

`base.Switch` è composto da una traccia, un cursore al suo interno e infine i suoi figli come etichetta. Una pressione segnala il valore richiesto, l'opposto di `Checked`.

```go
base.Switch{
	Checked:    dark.Get,
	OnChange:   dark.Set,
	Label:      "Dark appearance",
	Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	TrackStyle: core.Style{Width: core.Pt(38), Height: core.Pt(22), CornerRadius: 11},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

La traccia e il cursore accettano proprie `StyleFunc`, e i loro stili di stato seguono `Checked`.

## Slider

`base.Slider` accetta un `base.SliderValue`: `base.SingleValue(v)` o `base.RangeValue(low, high)`. Stilizza separatamente traccia, indicatore, intervallo e cursori.

```go
level := core.NewSignal(cx, 3.0)
base.Slider{
	Value:      func() base.SliderValue { return base.SingleValue(level.Get()) },
	OnChange:   func(v base.SliderValue) { level.Set(v.End) },
	Min:        func() float64 { return 0 },
	Max:        func() float64 { return 10 },
	Step:       func() float64 { return 1 },
	Label:      "Level",
	Style:      core.Style{Height: core.Pt(28)},
	TrackStyle: core.Style{Height: core.Pt(4), CornerRadius: 2},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

`Min`, `Max` e `Step` sono getter reattivi: modificare i loro segnali aggiorna lo slider montato e il suo intervallo accessibile, anche durante un trascinamento. `Axis` lo rende verticale e `Inverted` posiziona il minimo all'estremità opposta, come fa `base.Progress`; `Marks` posiziona i tuoi indicatori graduati sulla barra e `OnChangeEnd` segnala il valore una volta terminato il trascinamento.

## Select

`base.Select` accetta valori `base.SelectItem` e un valore `*string`, così «nessuno» è `nil`. Stilizzi tu il trigger, il popup, le sue righe e i suoi gruppi.

```go
choice := core.NewSignal[*string](cx, nil)
base.Select{
	Items: func() []base.SelectItem {
		return []base.SelectItem{{Value: "s", Title: "Small"}, {Value: "m", Title: "Medium"}}
	},
	Value:      choice.Get,
	OnChange:   choice.Set,
	Label:      "Size",
	Style:      core.Style{Height: core.Pt(28), Padding: core.Symmetric(4, 10), BorderWidth: 1, CornerRadius: 6},
	PopupStyle: core.Style{Padding: core.All(4), CornerRadius: 8},
	ItemStyle:  core.Style{Padding: core.Symmetric(4, 8), CornerRadius: 4},
}
```

Gli elementi adiacenti con lo stesso `Group` condividono un'intestazione, la ricerca incrementale (typeahead) corrisponde ai titoli degli elementi, `SearchField` disegna un tuo campo di ricerca e `Cleanable` mostra un pulsante per svuotare. `OnCancel` ascolta Esc, Indietro e una pressione all'esterno.

## Tabs

`base.Tabs` contiene elementi `base.Tab` e disegna l'indicatore da `IndicatorStyle`; l'app mantiene traccia della scheda selezionata. `base.Tab.OnPress` viene eseguito a ogni pressione, inclusa quella sulla scheda già selezionata.

```go
base.Tabs{Label: "Sections", IndicatorStyle: core.Style{Height: core.Pt(2), Background: core.RGB(0x007aff)}, Children: []core.Node{
	base.Tab{Label: "Overview", Selected: func() bool { return section.Get() == "overview" },
		OnPress: func(core.PressEvent) { section.Set("overview") }, Children: []core.Node{core.Text{Content: "Overview"}}},
	base.Tab{Label: "Activity", Selected: func() bool { return section.Get() == "activity" },
		OnPress: func(core.PressEvent) { section.Set("activity") }, Children: []core.Node{core.Text{Content: "Activity"}}},
}}
```

## Dialog

`base.Dialog` accetta gli stili per sfondo scuro (`backdrop`), livello e popup, una `Animation` o una `TransitionShader` e i tuoi figli. `Open`, `DefaultOpen` e `OnOpenChange` lo controllano; `OnOpen`, `OnClose` e `OnCancel` segnalano quanto accaduto. `OnConfirm` è l'azione predefinita: Invio la esegue, a meno che il focus della tastiera non sia su un pulsante o un campo che gestisce Invio da sé oppure un'altra modale non copra la finestra di dialogo, poi il dialogo si chiude con `ReasonSelect`. `Role` e `AccessibilityDescription` sono ciò che gli screen reader annunciano, `RoleAlertDialog` per una conferma. `NoCloseOnSwipe` impedisce allo scorrimento su iOS di chiuderlo, `Presentation` sceglie la presentazione iOS e `StatusBar` modifica la barra di stato mentre è aperto.

`base.Sheet` è un pannello su qualsiasi bordo, con uno sfondo e il tuo contenuto; con `CloseOnSwipe` un pannello inferiore segue un trascinamento verso il basso e si chiude oltre una soglia. Accetta `OnConfirm`, `Role` e `AccessibilityDescription` come `base.Dialog`. `base.ShowAlertDialog` mostra l'avviso o il foglio azioni di sistema con i pulsanti per ruolo:

```go
base.ShowAlertDialog(cx, base.AlertDialog{
	Title: "Delete the note?",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(label string) {
	if label == "Delete" {
		remove()
	}
})
```

## Menu

`base.Menu` apre elementi `base.MenuItem` da un trigger definito da te, e `base.ContextMenu` in corrispondenza del puntatore. Una voce segnala `OnSelect` e chiude il menu a meno che non sia impostato `NoCloseOnSelect`; `Checked` la rende una voce selezionabile, `Indicator` disegna il tuo segno prima del contenuto e `Role` sceglie cosa sente la tecnologia assistiva: una voce di menu, una casella di controllo o un pulsante radio. Un `base.Submenu` contiene ulteriori voci.

```go
base.Menu{Trigger: core.Text{Content: "Actions"}, Children: []core.Node{
	base.MenuItem{Label: "Show sidebar", Checked: sidebar.Get, OnSelect: func() { sidebar.Set(!sidebar.Get()) }},
	base.Submenu{Label: "Share", Children: []core.Node{
		base.MenuItem{Label: "Copy link", OnSelect: copyLink},
		base.MenuItem{Label: "Email", OnSelect: emailLink},
	}},
}}
```

`Placement`, `Align`, `Offset` e `Margin` posizionano il popup su desktop, mentre `PopupStyle` e `BackdropStyle` lo stilizzano.

## EmojiPicker

`base.EmojiPicker` cerca ogni emoji Unicode in base ai nomi CLDR localizzati, alle parole chiave e agli shortcode nelle lingue dell'app (o tramite `Languages` / `LanguagesFunc` espliciti), con schede di categoria, voci `Recent` opzionali, selezione della tonalità della pelle, navigazione da tastiera e una griglia virtualizzata. Tutte le etichette predefinite e i nomi delle categorie provengono da `base/locales`, e il dataset delle emoji viene collegato solo nelle app che usano `EmojiPicker` o i suoi helper (`base.SearchEmojis`, `base.EmojisInCategory`, `base.EmojiByString`).

```go
base.EmojiPicker{
	Recent:   recent.Get,
	OnSelect: func(e base.Emoji) { insert(e.Emoji) },
	Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
}
```

`Category` e `DefaultCategory` scelgono il gruppo da sfogliare. Le schede di categoria e le scelte della tonalità della pelle costituiscono ciascuna un unico punto di tabulazione con navigazione tramite frecce. `EmojiPickerRef` fornisce `Focus`, `Query`, `SetQuery`, `SetCategory` e `MatchedCount`; il selettore con stile usa lo stesso stato di ricerca. `LabelFunc`, `SearchLabelFunc` e `PlaceholderFunc` aggiornano le parole sul posto. `Emoji.WithSkinTone` applica la tonalità scelta a entrambe le persone in una coppia, mentre la tonalità predefinita ripristina la forma neutra; la ricerca tramite un'emoji ne conserva la sequenza esatta.

Usa `ui.EmojiPicker` per il selettore a tema con icone di categoria Lucide, un campo di ricerca svuotabile, anteprima dell'emoji evidenziata e campioni di tonalità della pelle.
