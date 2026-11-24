---
title: Accessibilità
description: Etichette, ruoli, valori, azioni e focus per screen reader e tastiere.
group: Concetti chiave
order: 61
---

## Cosa legge uno screen reader

La proprietà `Label` di una vista è ciò che la tecnologia assistiva pronuncia, `AccessibilityDescription` fornisce ulteriori dettagli letti dopo l'etichetta, `Hint` indica cosa fa l'attivazione e `Role` ne specifica il tipo: `core.RoleButton`, `RoleLink`, `RoleHeader`, `RoleImage`, `RoleCheckbox`, `RoleSwitch`, `RoleRadio`, `RoleTab`, `RoleTextField`, `RoleSearchField`, `RoleStatus`, `RoleMenu`, `RoleMenuItem`, `RoleDialog`, `RoleAlertDialog`, `RoleAlert`, `RoleAdjustable`, `RoleProgressBar`, `RoleTable` e gli altri ruoli. Per impostazione predefinita, uno status è una live region cortese (`polite`); un campo di ricerca è un campo di testo annunciato come casella di ricerca. Una finestra di dialogo viene letta con il suo ruolo e la sua etichetta all'apertura, seguiti dalla descrizione, proprio come avviene per il titolo e la descrizione di `ui.Dialog`. Un testo privo di etichetta viene letto tramite il suo contenuto. `LabelFunc`, `HintFunc` e `AccessibilityDescriptionFunc` associano testi di accessibilità dinamici sulla stessa vista, così i controlli mantengono focus e stato anche durante un cambio di lingua. Tutti gli elementi `core` dotati di etichetta accettano `LabelFunc`, mentre `View` e `Text` accettano `HintFunc`.

Usa `TextFunc` per il testo dinamico su `ui.Button`, `ui.Checkbox`, `ui.MenuItem` e `ui.AlertAction`. I loro nomi accessibili seguono quel testo. Pulsanti e checkbox accettano anche una `LabelFunc` separata. Slider, schede, switch, barre di avanzamento e trigger di popup accettano `LabelFunc` per un nome accessibile variabile. `ui.Option.LabelFunc` fornisce il testo dell'opzione in schede, select e combobox. Anche i campi numerici e di codice, gli anelli di avanzamento ed entrambe le varianti di spinner accettano `LabelFunc`.

```go
core.View{
	Role:  core.RoleButton,
	Label: "Add to cart",
	Hint:  "Adds the item and opens the cart",
	OnPress: add,
	Children: []core.Node{core.Icon{SVG: lucide.ShoppingCart}},
}
```

`AccessibilityValue` fornisce un intervallo o un valore testuale: il minimo, il massimo e il valore corrente di uno slider, oppure un testo come «3 elementi». `Accessible` raggruppa una vista in un unico elemento, in modo che il nome e il prezzo di una scheda vengano letti insieme anziché in due passaggi separati e, in assenza di etichetta, la vista legga il testo dei suoi discendenti. `AccessibilityElementsHidden` nasconde gli elementi decorativi, come una freccia o un separatore, mentre `ImportantForAccessibility` nasconde un intero sottoalbero.

```go
core.View{Accessible: true, Label: "Apples, $1.20 a kilo", Children: []core.Node{
	core.Text{Content: "Apples"},
	core.Text{Content: "$1.20 a kilo"},
}}
```

## Etichette visibili

Assegna a un `core.Text` o a un `core.View` il `Ref` del controllo come `LabelFor`. Premendo l'etichetta si sposta il focus sul controllo e si esegue la sua azione di pressione, ad esempio attivando o disattivando un checkbox. Il suo testo attualmente visibile dà il nome al controllo senza dover ripetere una stringa `Label` e segue gli stati `Disabled` e `Invalid` del controllo per l'accessibilità e gli stili. Un'etichetta non acquisisce alcun ruolo di pulsante né diventa un punto di tabulazione.

```go
field := new(core.Ref)
return core.View{Children: []core.Node{
	core.Text{Content: "Email", LabelFor: field},
	base.Input{Ref: field},
}}
```

`LabelledBy: []*core.Ref{label, requirement}` assegna il nome a un controllo a partire da più elementi visibili nell'ordine indicato, con precedenza su `Label`; non inoltra invece le pressioni. Negli altri casi `Label` prevale sui nomi impliciti di `LabelFor`, il cui ordine segue quello dell'albero. Aggiornamenti del testo, spostamenti, occultamenti, rimozioni e rimontaggi aggiornano la relazione. I riferimenti mancanti, ripetuti, ciclici o nascosti vengono ignorati; le associazioni restano confinate all'interno di una stessa superficie e dello stesso ambito di confinamento. Un'etichetta non può attivare un controllo dietro una finestra modale.

Pulsanti, checkbox, pulsanti radio, switch, select e campi numerici di `base` e `ui` passano `Ref` e `LabelledBy` al proprio controllo; gli slider espongono `ThumbRef` e `StartThumbRef`. In TSX, usa `labelFor={field()}` e `labelledBy={[label()]}` con segnali di nodo popolati tramite `ref={node => field.set(node)}`, così un'etichetta posizionata prima del relativo campo si aggiorna quando quel campo viene montato.

## Azioni

`AccessibilityActions` elenca ciò che lo screen reader offre su un elemento: le azioni standard (`core.ActionActivate`, `ActionIncrement`, `ActionDecrement`, `ActionEscape`, `ActionMagicTap`) e quelle definite dall'app, ciascuna con un `Name` e una `Label`. `OnAccessibilityAction` riceve il nome. Usa `AccessibilityActionsFunc` su `core.View` o `core.Input` per modificare le azioni disponibili o tradurne le etichette mentre l'elemento resta montato.

```go
core.View{
	Role: core.RoleAdjustable,
	Label: "Quantity",
	AccessibilityValue: func() core.AccessibilityValue {
		return core.AccessibilityValue{Max: 10, Now: float32(n.Get()), Text: fmt.Sprintf("%d items", n.Get())}
	},
	AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
	OnAccessibilityAction: func(name string) {
		if name == core.ActionIncrement {
			n.Update(func(v int) int { return v + 1 })
		} else {
			n.Update(func(v int) int { return v - 1 })
		}
	},
	Children: quantityControls,
}
```

`MultiSelectable` contrassegna una griglia, una listbox o una vista ad albero che consente la selezione di più elementi. `base.DataTable` lo imposta per la selezione multiriga; il web, Windows e Linux espongono il corrispondente flag di accessibilità.

`Expanded` indica se un pannello espandibile o un pulsante di menu è aperto, `Busy` che il contenuto è in caricamento, `Invalid` che l'app ha rifiutato il contenuto di un campo o il valore di un controllo (stilizzato con `aria-invalid:`), mentre `Checked`, `Selected` e `Pressed` riportano lo stato di un controllo. Le parti di un controllo possono ereditarne lo stato tramite `PartStates`, così un indicatore segue il proprio checkbox senza diventare un elemento navigabile a sé stante.

`SelectionItem` contrassegna una vista come elemento di una raccolta selezionabile indipendentemente dal suo valore corrente di `Selected`. `base.DataTable` lo imposta sulle righe o sulle celle quando la tabella le seleziona, e `base.Tree` lo imposta su ogni riga; Windows mantiene il pattern SelectionItem e il web scrive `aria-selected="true"` o `"false"`. Una tabella senza selezione lascia le righe prive di entrambi i marcatori.

## Modifiche in tempo reale

`LiveRegion` fa sì che gli screen reader annuncino l'etichetta di una vista e il testo al suo interno quando cambiano: `core.LiveRegionPolite` attende che l'utente sia inattivo, `LiveRegionAssertive` interrompe la lettura corrente. `cx.Announce` pronuncia un messaggio una sola volta.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("%d items in the basket", count.Get()) },
	LiveRegion: core.LiveRegionPolite}
```

## Focus e tastiera

I tooltip descrivono il proprio trigger prima che riceva il focus. `View.Tooltip`, `base.Tooltip` e `ui.Tooltip` usano il loro testo corrente come descrizione di accessibilità del controllo, a meno che non ripeta il nome del controllo, l'app fornisca `AccessibilityDescription` o il tooltip sia `Disabled`. Un wrapper trasmette la descrizione ai propri controlli. Il riquadro del tooltip e i suoi figli sono nascosti agli screen reader e non annunciano mai un avviso; pertanto il tooltip predefinito di `ui.IconButton` non ripete la sua etichetta.

Un `base.DataTable` espone la sua griglia con nome come punto di focus per la tastiera e l'accessibilità; il livello di intercettazione del puntatore è nascosto alla tecnologia assistiva.

Su desktop, web e Apple TV, le viste che accettano pressioni sono punti di tabulazione insieme ai campi di testo; `NoFocus` ne esclude una e `Focusable` rende punto di tabulazione una vista priva di gestori di pressione. `Style.FocusVisible` disegna l'anello di focus solo quando il focus è stato spostato da tastiera (`focus-visible:`), `Style.Focus` per qualsiasi focus e `Style.FocusWithin` per il focus all'interno di una vista; `OnFocus` e `OnBlur` lo rilevano, `OnBlur` anche quando la vista con il focus esce dall'albero, e `Ref.Focus()` e `Ref.SetAccessibilityFocus()` lo spostano; `core.FocusedRef(cx)` è un riferimento alla vista che detiene il focus, utile per ciò che si apre senza un trigger, come una barra di ricerca attivata da un tasto, affinché lo restituisca alla chiusura. Invio e Spazio premono la vista focalizzata, mentre i tasti freccia si spostano tra elenchi di schede, gruppi di pulsanti radio e menu. Il `Tooltip` di una vista appare immediatamente quando la tastiera sposta il focus su di essa, finché il focus non si sposta o non viene premuto Esc. `OnOpen` di un popup viene eseguito una volta montato il suo contenuto, quindi un `Focus` al suo interno sposta lì il focus, come dovrebbe fare una finestra di dialogo non modale; il contenuto di un popover segue il proprio trigger nell'ordine di tabulazione e il focus torna al trigger quando il popup si chiude.

Su Android, iPhone e iPad, `Ref.Focus()` e `AutoFocus` di un pulsante possono assegnare il focus a una vista abilitata premibile o `Focusable` per una tastiera hardware. Toccare un controllo non sposta quel focus. Spazio e Invio attivano la vista premibile focalizzata dopo la mappatura dei tasti, preservando Control, Alt, Maiusc e il tasto di piattaforma in ogni fase della pressione. `NoFocus`, i controlli disabilitati o nascosti e i controlli dietro una modale non possono ricevere quel focus. L'assegnazione del focus a una vista non apre alcuna tastiera software; i campi nativi mantengono il loro comportamento di modifica. watchOS e i mini program non dispongono di un percorso di attivazione tramite tastiera fisica.

`cx.Env().ScreenReader()` indica se è attivo uno screen reader, `cx.Env().ReduceMotion()` se l'utente ha richiesto la riduzione del movimento, `cx.Env().IncreaseContrast()` se l'utente ha richiesto un contrasto maggiore (Aumenta contrasto su macOS, iOS e watchOS, un tema a contrasto su Windows, Contrasto elevato di GNOME, l'impostazione del contrasto di GTK 4.20 o un tema GTK ad alto contrasto, l'impostazione del contrasto sopra lo standard di Android 14, `prefers-contrast: more` o `forced-colors: active` sul web), `cx.Env().FontScale()` la dimensione del testo e `cx.Env().RTL()` la direzione del layout. I testi seguono la dimensione del testo a meno che non sia impostato `NoFontScaling`, e un layout da destra a sinistra rispecchia i propri bordi fisici quando utilizza quelli logici.

## Prossimi passi

- [Stili e temi](styles.md): stati di focus e disabilitazione.
- [Moduli](forms.md): etichette e valori per i campi.
- [Canvas e shader](canvas.md): esporre le parti di un disegno.
