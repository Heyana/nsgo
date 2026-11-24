---
title: Input
description: Il campo di testo della piattaforma, controllato o meno, con il focus e la tastiera software.
group: Componenti
order: 33
---

`ui.Input` è il campo di testo della piattaforma con l'aspetto di `ui`, su una sola riga a meno che non sia `Multiline`. Ha i campi di `base.Input`, come ogni campo con stile; `ui.Field` è lo stesso campo senza una dimensione propria, in un bordo che non segue il puntatore né il focus; `core.Transparent` nel suo `Background` e `core.Zero` nel suo `BorderWidth` eliminano il riquadro, per un testo che si legge come testo semplice ma si può modificare.

```go preview=input/basic
name := core.NewSignal(cx, "")
ui.Input{
	Value:       name.Get,
	OnChange:    name.Set,
	Placeholder: "Your name",
	TestID:      "name",
}
```

## Controllato e non controllato

Con `Value`, il campo è controllato: l'app detiene il valore e lo aggiorna in `OnChange`. Con `DefaultValue`, il campo mantiene un valore proprio. Un campo controllato si svuota impostandone il valore a `""`.

```go preview=input/submit
draft := core.NewSignal(cx, "")
ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "New task",
	OnSubmit: func(string) { add(); draft.Set("") }}
```

## Campi

`Multiline` va a capo, parte con un'altezza di tre righe e cresce con il testo, a meno che il suo stile non imposti `Height` o `MinHeight`. `Masked` nasconde i caratteri, `Readonly` impedisce le modifiche pur consentendo di selezionare e copiare il testo, `Disabled` disattiva il campo e `MaxLength` limita le unità UTF-16 inserite dall'utente. `Placeholder` può essere una funzione con `PlaceholderFunc`.

```go
ui.Input{Masked: true, Value: password.Get, OnChange: password.Set,
	AutoComplete: core.AutoCompletePassword}
```

`KeyboardType` sceglie una tastiera software (`KeyboardEmail`, `KeyboardNumber`, `KeyboardURL` e le altre), `ReturnKeyType` assegna l'etichetta al suo tasto Invio e `SubmitBehavior` stabilisce se Invio invia o va a capo in un campo multiriga. `AutoCapitalize`, `NoAutoCorrect` e `AutoComplete` regolano l'aiuto del sistema.

`InputSource` seleziona una sorgente di input della tastiera mentre il campo ha il focus e ripristina quella dell'utente quando lo perde, a meno che non sia impostato `KeepInputSource`: `"com.apple.keylayout.ABC"` mantiene in inglese un campo per codici o comandi su macOS. L'ID è quello della piattaforma, un ID di layout di tastiera come `"00000409"` su Windows e un metodo di input Fcitx 5 o IBus su Linux; una sorgente non abilitata, i telefoni e il web non cambiano nulla.

```go
ui.Input{Placeholder: "Bundle ID", InputSource: "com.apple.keylayout.ABC"}
```

`Style` si fonde sopra l'aspetto del campo, e `StyleFunc` sopra di esso: ciò che impostano, come l'altezza, il padding, il font o un colore, sostituisce quello di `ui`, e il resto rimane. Il bordo si scurisce sotto il puntatore e assume il colore d'accento quando ha il focus, purché nessuno dei due imposti `BorderColor` o `Hover`. Un `Padding` zero lascia quello di `ui`; `core.Zero`, una lunghezza impostata, lo rimuove (`Padding: core.All(core.Zero)`).

```go
ui.Input{Placeholder: "Filter", Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
ui.Field{DefaultValue: "Untitled", Style: core.Style{FontSize: 24, FontWeight: ui.Bold,
	Background: core.Transparent, BorderWidth: core.Zero}}
```

## Focus ed eventi

`Ref` dà e toglie il focus al campo. `OnFocus` e `OnBlur` seguono il focus, `OnSelectionChange` e `OnCaretMove` la selezione e il cursore di testo, `OnKeyPress` ogni tasto digitato, `OnPaste` ciò che viene incollato e `OnCompositionChange` la composizione di un metodo di input.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

La tastiera software copre la parte inferiore di un telefono. Metti `core.EnvKeyboard` sul padding o sul margine inferiore del contenuto, così che resti sopra la tastiera; lo spostamento si anima insieme a essa. `cx.DismissKeyboard()` chiude la tastiera e `cx.Env().KeyboardHeight()` ne legge l'altezza.

## Validazione

Le regole sono dell'app. Un signal contiene l'errore, `OnBlur` o `OnSubmit` lo imposta e un messaggio sotto il campo lo mostra ([Moduli](../forms.md#validazione)).

```go preview=input/validation
err := core.NewSignal(cx, "")
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{Value: email.Get, OnChange: email.Set, Label: "Email",
		OnSubmit: func(string) {
			if !strings.Contains(email.Get(), "@") {
				err.Set("Enter an email address.")
			} else {
				err.Set("")
			}
		}},
	ui.Textf(err.Get, 12, ui.Regular, colors.Danger),
}}
```

## Accessibilità

Il campo segnala `RoleTextField`, oppure `RoleSearchField` per una casella di ricerca quando il suo `Role` lo indica, e legge la propria `Label`; `Placeholder` non è un'etichetta, quindi assegna una `Label` a un campo privo di etichetta visibile. `Readonly` e `Disabled` vengono segnalati e `OnSubmit` è l'azione della tastiera. I campi a riga singola, mascherati e multiriga espongono il `Placeholder` corrente come placeholder o suggerimento di accessibilità nativo su macOS, iOS e Linux, aggiornandolo e cancellandolo senza una seconda tappa del focus per l'etichetta di placeholder disegnata di un campo multiriga.

## Piattaforma

iOS, Android, Windows e Linux usano i propri campi nativi; il web usa un `<input>` o una `<textarea>`. La tastiera software, il riempimento automatico, i metodi di input e la direzione del testo sono quelli della piattaforma. Focus e digitazione vengono testati allo stesso modo su ogni target.

## Correlati

- [Moduli](../forms.md): campi, validazione, scelte e tastiera.
- [Accessibilità](../accessibility.md): etichette e focus.
- [Layout](../layout.md): mantenere il contenuto sopra la tastiera.
- Le pagine Input e Form della galleria.
