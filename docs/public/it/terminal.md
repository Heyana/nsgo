---
title: Terminale
description: Esegui un programma o mostra una sessione remota in una griglia di terminale.
group: Concetti chiave
order: 63
---

`base/terminal` fornisce una sessione (`Terminal`) e la sua `View`. I desktop eseguono programmi locali; iOS e i browser usano `Options.Conn`, un `io.ReadWriteCloser` collegato a un programma remoto. La sessione appartiene all'app, che la chiude al termine.

```go
t, err := terminal.New(terminal.Options{CursorBlink: true})
if err != nil {
	return core.Text{Content: err.Error()}
}
cx.Cleanup(func() { t.Close() })
return terminal.View{
	Terminal: t, AutoFocus: true,
	MinimumContrast: 4.5, PaddingColor: terminal.PaddingExtend,
	Style: core.Style{FlexGrow: 1},
}
```

`Options.Cursor` sceglie la forma predefinita: blocco, barra o sottolineatura. `CursorBlink` ne abilita il lampeggio predefinito; con zero resta fisso. I programmi sovrascrivono forma e lampeggio tramite DECSCUSR o la modalità DEC 12. Un cursore senza focus è vuoto e fisso; anche la composizione e il movimento ridotto lo mantengono fisso. Il lampeggio è guidato dal clock di animazione della piattaforma, senza che il motore ridisegni il testo del terminale.

`View.Font`, `Theme` e `DarkTheme` impostano il font e la palette. La griglia disegna da sé i caratteri Unicode di disegno box e di blocco e i separatori Powerline U+E0B0–U+E0B7, allineando le celle adiacenti ai pixel del dispositivo. Il resto del testo usa i font della piattaforma e i relativi fallback.

`MinimumContrast` aumenta il contrasto del testo ordinario rispetto all'effettivo sfondo della cella, inclusi i colori invertiti, il testo attenuato e la selezione. I valori da 1 a 21 usano il rapporto di contrasto WCAG; zero preserva i colori. Grafica ed emoji mantengono i propri colori. Con `Transparent`, il contrasto usa comunque lo sfondo configurato del terminale.

`Padding` lascia spazio all'esterno della griglia, in punti. `PaddingColor` ne sceglie lo sfondo:

| Valore | Comportamento |
| --- | --- |
| `PaddingBackground` | Sfondo del terminale, comprese le modifiche OSC 11 |
| `PaddingExtend` | Sfondo della cella più vicina; sullo schermo primario, sopprime l'estensione superiore/inferiore accanto alle righe del prompt, alle righe Powerline o alle righe che contengono una cella con lo sfondo predefinito |
| `PaddingExtendAlways` | Estende ogni bordo senza quelle eccezioni |

Le app TSX importano `tsx/terminal` per il suo effetto collaterale, che registra `Terminal` in `nsgo/base`. Le prop sono `cursorBlink`, `minimumContrast` e `paddingColor`; quest'ultima accetta `background`, `extend` ed `extend-always`. Le prop dell'aspetto possono cambiare mentre la sessione prosegue.

## In un browser

L'app incorpora il modulo WebAssembly libghostty-vt in versione fissata ed esegue la stessa vista di terminale e lo stesso percorso di input. Per compilare l'app non servono Zig, l'installazione di librerie native né download a runtime.

Adatta il WebSocket del browser a `io.ReadWriteCloser`: invia byte binari e unisci o dividi i messaggi in arrivo come un flusso. Autenticazione, riconnessione ed eventuali messaggi di ridimensionamento/controllo spettano all'app e al suo server; NSGo consuma i byte del terminale. La [fixture di accettazione per browser](https://github.com/nsgo-dev/nsgo/blob/main/internal/testhost/testdata/terminalweb/socket_js_test.go) mostra un adattatore completo per flussi di byte, con la gestione della chiusura.

## Azioni e zoom

`View.Keybinds` sovrascrive le impostazioni predefinite con `Keybind{Key, Action, Amount}`. I tasti usano gli stessi nomi di `Scope.OnKey`, come `mod+=` o `ctrl+shift+c`. Le voci successive prevalgono. Le associazioni si attivano mentre l'input di questo terminale ha il focus e nessun metodo di input sta componendo.

Le azioni sono `CopyToClipboard`, `PasteFromClipboard`, `SelectAll`, `ClearScreen`, `IncreaseFontSize`, `DecreaseFontSize`, `ResetFontSize`, `SetFontSize` e `Ignore`. Le azioni di finestra e di scheda appartengono alla mappa dei tasti dell'app. `Ignore` consuma un tasto senza inviarlo al programma.

Per impostazione predefinita, `mod+=` e `mod+shift+=` aumentano il font di un punto, `mod+-` lo riduce e `mod+0` lo riporta a `View.Font.Size` (13 se non impostato). `Amount` imposta il passo per aumento/riduzione, uno se zero, oppure la dimensione per `SetFontSize`. Le dimensioni sono limitate a 1–255 punti; le richieste non finite vengono ignorate. Lo zoom mantiene la sessione e la vista esterna, aggiorna righe e colonne della griglia e ridimensiona il programma.

```go
ref := &terminal.Ref{}
view := terminal.View{
	Terminal: session, Ref: ref,
	Keybinds: []terminal.Keybind{
		{Key: "ctrl+shift+k", Action: terminal.ClearScreen},
		{Key: "mod+=", Action: terminal.IncreaseFontSize, Amount: 2},
	},
}
```

`Ref.FontSize()` è tracciato per i controlli che lo mostrano; `Ref.SetFontSize(size)` e `Ref.Perform(action, amount)` vengono eseguiti sul motore dell'app, come i gestori di eventi. `Perform` indica se un'azione è disponibile. Un ref non montato restituisce una dimensione zero e rifiuta le azioni.

## Menu contestuale

Un clic secondario apre un `base.ContextMenu` senza stile, con le voci localizzate Copia, Incolla, Seleziona tutto e Cancella. Copia richiede una selezione. Cancella rimuove il contenuto sopra il prompt ed è disabilitato sullo schermo alternativo. Gli schermi touch usano la presentazione a pressione prolungata del menu contestuale. Applica stili alla superficie con `MenuClass`/`MenuStyle` e alle voci con `MenuItemClass`/`MenuItemStyle`.

Imposta `RightClickAction` su `RightClickPaste`, `RightClickCopy`, `RightClickCopyOrPaste` (copia una selezione, altrimenti incolla) o `RightClickIgnore`. Ignore lascia l'evento a un menu dell'app che racchiude il terminale; usalo quando fornisci un `ui.ContextMenu` con stile che chiama le azioni del ref.

Importa `tsx/terminal` per registrare `<Terminal>` in `nsgo/base`. Le sue prop reattive `keybinds` e `rightClickAction` aggiornano la vista mantenendo la sessione. I valori delle azioni usano i nomi di Ghostty, come `increase_font_size` e `copy_to_clipboard`; i valori del clic destro sono `context-menu`, `paste`, `copy`, `copy-or-paste` e `ignore`. Le prop di stile del menu usano i nomi Go in lower camel case.

## Segnalazioni del mouse

I programmi che abilitano il tracciamento del mouse ricevono le pressioni, i rilasci, i trascinamenti e l'hover richiesti, codificati da libghostty-vt nel protocollo che hanno selezionato. Shift aggira la cattura di clic e trascinamenti per consentire la selezione. `View.MouseReporting` è una `func() bool` reattiva facoltativa, vera quando nil; restituisci false per mantenere la selezione con il mouse e lo scorrimento nativo dello scrollback anche mentre un programma richiede la cattura. Il clic destro di un programma ha la precedenza su un menu contestuale che lo racchiude finché le segnalazioni sono abilitate.

Sullo schermo alternativo, con la modalità di scorrimento alternativa abilitata e nessun tracciamento del mouse richiesto, l'input verticale della rotellina diventa tasti cursore, seguendo la modalità dei tasti cursore normale o applicativa. `MouseScrollMultiplier{Precision: 1, Discrete: 3}` imposta il numero predefinito di segnalazioni per touchpad e rotelline; zero usa quei valori predefiniti, e i valori positivi sono limitati a 0,01–10.000. L'input frazionario si accumula finché non è disponibile un passo completo. Lo scrollback normale mantiene lo scorrimento nativo e l'inerzia della piattaforma.

Importa `tsx/terminal` per registrare `<Terminal>` in `nsgo/base`. La sua prop `mouseReporting` è vera per impostazione predefinita; `mouseScrollPrecision` e `mouseScrollDiscrete` impostano i moltiplicatori.

