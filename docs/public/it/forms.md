---
title: Moduli
description: Campi di testo, focus, tastiera software, interruttori, selettori e validazione.
group: Concetti chiave
order: 57
---

## Campi di testo

`ui.Input` è il campo di testo della piattaforma nello stile di `ui`, con i campi di `base.Input`. Con `Value` è controllato: l'app mantiene il valore e lo aggiorna in `OnChange`; con `DefaultValue` gestisce il proprio stato internamente.

```go
name, email := core.NewSignal(cx, ""), &core.Ref{}

ui.Input{
	Value:         name.Get,
	OnChange:      name.Set,
	Placeholder:   "Your name",
	ReturnKeyType: core.ReturnKeyNext,
	OnSubmit:      func(string) { email.Focus() },
}
```

`Masked` nasconde i caratteri, `Readonly` impedisce le modifiche consentendo comunque di selezionare e copiare il testo (nei mini program solo in un campo a riga singola di WeChat: un campo multiriga di WeChat e i campi di Alipay si disabilitano come fa `Disabled`, poiché tali host non possono annullare una modifica), `Disabled` disabilita il campo e `Multiline` manda a capo il testo crescendo insieme ad esso a meno che il suo stile non ne fissi le dimensioni. Le righe di un campo multiriga, e quelle del suo segnaposto, sono alte quanto il `LineHeight` del suo stile, con il font centrato in ciascuna come in un `core.Text`; un campo a riga singola mantiene la propria riga centrata nella sua altezza. `KeyboardType` sceglie la tastiera software (`core.KeyboardEmail`, `core.KeyboardNumber`, `core.KeyboardURL` e così via), `AutoComplete` indica al sistema cosa contiene il campo per il completamento automatico e `MaxLength` limita le unità UTF-16 inserite dall'utente. `AutoCapitalize` e `NoAutoCorrect` regolano il comportamento della tastiera.

`OnSelectionChange` e `OnCaretMove` segnalano la selezione e la posizione del cursore, ad esempio per un menu che si apre in quel punto. `Suggest`, un `core.Suggest`, segue una query dopo un trigger come `@`, e un `ui.SuggestList` accanto al campo propone righe in corrispondenza del cursore, selezionabili con Su, Giù, Invio o un clic, come fa il `Suggest` di `chat.Composer` per le menzioni. `OnKeyPress` intercetta ogni tasto, `OnPaste` ciò che l'utente incolla e `OnCompositionChange` segue un metodo di input.

Su iOS e Android, `cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, handler)` intercetta un tasto Invio della tastiera software prima che il campo invii, perda il focus o inserisca una nuova riga. `KeyAuto` e `KeyNotEditing` lasciano la gestione a `SubmitBehavior`; su iOS, un Invio che conferma il testo in composizione rimane al metodo di input.

## Focus e tastiera

`Ref.Focus()` assegna il focus a un campo oppure, su desktop, web e Apple TV, a una vista cliccabile o `Focusable` abilitata. `Ref.Blur()` rimuove il focus solo dalla propria vista, e `cx.DismissKeyboard()` chiude la tastiera software. `OnFocus` e `OnBlur` seguono il focus, mentre `View.OnFocus`/`OnBlur` fanno lo stesso per qualsiasi vista focalizzabile. Una vista con il focus che lascia l'albero, quando il suo `Show` si chiude o la sua riga scompare, riceve `OnBlur` una sola volta, dopo il commit che l'ha rimossa oppure, quando la piattaforma segnala prima il focus successivo, prima dell'`OnFocus` di quella vista, mentre le viste circostanti che restano ricevono `OnFocusOut`.

Le richieste di focus, blur, scorrimento e zoom di un `Ref` non hanno effetto finché il relativo elemento si trova nel contenuto di un `Holder` non mostrato in alcun punto, e quelle ancora in coda quando l'elemento lascia l'albero vengono scartate. Mostrare nuovamente il contenuto non le riesegue: richiedile di nuovo una volta che è posizionato.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Le tastiere software coprono la parte inferiore del telefono. `core.EnvKeyboard` usato come padding o margine inferiore mantiene il contenuto al di sopra, e il suo spostamento è animato insieme alla tastiera:

```go
core.View{Style: core.Style{
	Padding:    core.All(16),
	PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard},
	Gap:        12,
}, Children: form}
```

Una vista scroll attorno a un modulo può chiudere la tastiera al trascinamento e decidere cosa fa un tocco mentre la tastiera è visibile:

```go
core.Scroll{Behavior: core.ScrollBehavior{
	KeyboardDismissMode:       core.KeyboardDismissOnDrag,
	KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
}, Children: []core.Node{form}}
```

`cx.Env().KeyboardHeight()` e `cx.Env().KeyboardDuration()` leggono da codice dimensioni e durata dell'animazione della tastiera, mentre `cx.Env().SafeArea()` legge gli inset attorno allo schermo.

## Scelte

`ui.Switch` è un'impostazione che ha effetto immediato, `ui.Checkbox` può essere selezionata, deselezionata o indeterminata, e `ui.Radio` è una delle opzioni di un `ui.RadioGroup`. `ui.Select[T]` apre un elenco di opzioni sotto il proprio trigger, o come sheet quando richiesto da `Presentation`, e `ui.Tabs[T]` ne mostra alcune insieme.

```go
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}

size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options:  []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:    size.Get,
	OnChange: size.Set,
	Label:    "Size",
}
```

`ui.NumberInput` e `base.NumberInput` accettano `LabelFunc` per un nome accessibile dinamico. Un `Format` personalizzato segue i signal che legge mentre il campo non è in modifica; un cambio di lingua preserva la bozza attiva fino al termine della modifica. `ui.NumberInput` incrementa e regola un numero entro determinati limiti, `ui.DatePicker` e `ui.TimeField` modificano date e ore, `ui.ColorPicker` modifica un colore e `ui.TagInput` modifica un elenco di tag.

`ui.OtpInput` accetta un codice monouso suddiviso in celle. Sia questo componente sia `base.OtpInput` accettano `LabelFunc` senza sostituire il campo con focus o il relativo codice. `Size` imposta l'altezza e la larghezza predefinite, pari a 36 punti su desktop e TV e 44 sui dispositivi touch. `CellWidth` e `CellHeight` sovrascrivono indipendentemente ciascuna dimensione. `TextStyle` sovrascrive la tipografia della cella, `CornerRadius` ne modifica gli angoli e `CaretHeight` sovrascrive l'altezza derivata dall'interlinea.

```go
quantity := core.NewSignal(cx, 1.0)
ui.NumberInput{Value: quantity.Get, OnChange: quantity.Set, Min: 1, Max: 99, Step: 1, Label: "Quantity"}
```

## Righe raggruppate

`ui.FormSection` dispone le proprie righe su una card, con il titolo sopra e il piè di pagina sotto. Imposta `CardStyle` per lo sfondo, il bordo o gli angoli della card. `CardStyleFunc` si fonde sopra di esso e segue i signal che legge. `DividerInset` riceve ciascuna riga figlia, incluse le righe create da `core.Each`: restituisci un inset e `true` per sovrascriverne il divisore, oppure `false` per mantenere l'allineamento automatico al titolo (12 punti per gli altri nodi). Un inset pari a zero crea un divisore a tutta larghezza.

`ui.FormRow` posiziona `Leading` o un riquadro icona prima del titolo. Imposta `LeadingWidth` per riservare spazio al tuo nodo e allineare al titolo il divisore sottostante. Lo slot è di 24 punti per impostazione predefinita. Imposta `TileSize` per scalare insieme il riquadro icona, il glifo e gli angoli. `TitleStyle` e `DetailStyle` sovrascrivono la tipografia di titolo e dettaglio. `TitleMaxLines` limita le righe del titolo (zero significa illimitato), `MinHeight` sovrascrive l'altezza minima della riga in punti, e `Below` posiziona un nodo dopo il titolo e il dettaglio nella stessa colonna di testo. Un valore zero per `MinHeight` mantiene il predefinito: 40 punti, oppure 48 sui dispositivi touch.

Per un insieme a scelta singola, racchiudi la sezione in `base.RadioGroup` e imposta su ciascuna riga `Role: core.RoleRadio`, `Checked` e `OnPress`. La riga utilizza `base.Radio` per i propri stati di accessibilità selezionato e spuntato, un solo Tab stop e la selezione con i tasti freccia; premere una riga già spuntata non fa nulla. L'app gestisce il valore scelto.

`ui.FormSection.TitleFunc` e `FooterFunc`, così come `ui.FormRow.TitleFunc`, `DetailFunc` e `LabelFunc`, aggiornano i testi senza ricostruire la riga. Un dettaglio, un titolo di sezione o un piè di pagina vuoti rimuovono quel testo opzionale; un'etichetta di riga vuota utilizza il titolo corrente. Un `ui.Select` con ricerca associa `SearchPlaceholderFunc` ed `EmptyFunc` mantenendo aperte la ricerca e la selezione correnti.

## Validazione

Le regole spettano all'app: un signal conserva l'errore, `OnBlur` o `OnSubmit` lo imposta, e un `core.Show` aggiunge un messaggio sotto il campo quando ce n'è uno.

```go
address := core.NewSignal(cx, "")
err := core.NewSignal(cx, "")
colors := ui.ColorsOf(cx)
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{
		Value:    address.Get,
		OnChange: func(v string) { address.Set(v); err.Set("") },
		Label:    "Email",
		OnSubmit: func(string) {
			if !strings.Contains(address.Get(), "@") {
				err.Set("Enter an email address.")
			}
		},
	},
	core.Show{
		When: func() bool { return err.Get() != "" },
		Then: func(*core.Scope) core.Node { return ui.Textf(err.Get, 12, ui.Regular, colors.Danger) },
	},
}}
```

`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` anima un messaggio che compare, e `OnSubmit` con `ReturnKeyNext` sposta la tastiera al campo successivo.

## Prossimi passi

- [Navigazione](navigation.md): dove vive un modulo.
- [Testo](text.md): come è definito lo stile del testo di un campo.
- [Accessibilità](accessibility.md): etichette e valori necessari agli screen reader.
