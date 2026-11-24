---
title: Indirizzi dell'applicazione
description: Leggere e aggiornare l'indirizzo di un'app web tramite la cronologia del browser.
group: Concetti chiave
order: 59
---

`cx.AppURL()` legge l'indirizzo dell'applicazione come valore tracciato. Sul web l'URL iniziale della pagina è disponibile prima del rendering della radice, così un ricaricamento o un link condiviso possono scegliere la stessa vista. È l'app a decidere come l'URL mappa alle proprie viste.

```go
func Root(cx *core.Scope) core.Node {
    return core.View{Children: []core.Node{
        core.Text{ContentFunc: cx.AppURL},
        base.Button{Label: "Open film", OnPress: func(core.PressEvent) {
            if err := cx.SetAppURL("/films/42", core.AppURLOptions{
                History: core.HistoryPush,
            }); err != nil {
                // Show the error in the app's usual way.
            }
        }},
    }}
}
```

`SetAppURL` aggiorna immediatamente l'indirizzo tracciato senza richiedere una pagina o navigare uno stack. Per impostazione predefinita sostituisce la voce corrente del browser; `HistoryPush` ne richiede una nuova. Un push di navigazione o di una modale nello stesso commit fornisce invece quella voce, così una singola azione non crea due passaggi indietro. Più scritture in un commit mantengono l'ultimo indirizzo e preservano qualsiasi richiesta di push.

Gli indirizzi relativi vengono risolti rispetto all'indirizzo corrente o a un base HTML esplicito. Un indirizzo del browser deve rimanere sull'origine della pagina. Indirizzi non validi, spazi bianchi, caratteri di controllo, barre rovesciate e indirizzi superiori a 16 KiB restituiscono un errore senza modificarlo.

`cx.OnAppURLChange(func(change core.AppURLChange) { ... })` osserva gli spostamenti nella cronologia del browser e le modifiche all'hash. `change.Reason` vale `"back"`, `"forward"` o `"address"`; la funzione restituita annulla la sottoscrizione e lo smaltimento dello scope rimuove il listener. Leggi `AppURL` per ottenere l'indirizzo iniziale. Le scritture effettuate dall'app stessa aggiornano i binding ma non inviano alcuna notifica di modifica. Uno spostamento che mantiene lo stesso indirizzo non invia alcuna modifica.

Il pulsante Indietro del browser chiude comunque una modale o estrae una schermata dallo stack di navigazione. Un ritorno rifiutato dall'app ripristina la voce esistente e il relativo indirizzo. Un gestore di chiusura può impostare il proprio indirizzo sostitutivo. Una rimozione programmatica aggiorna l'indirizzo dopo lo spostamento asincrono nella cronologia, e le scritture che arrivano durante tale spostamento attendono che si concluda. Tornare indietro oltre le voci di questo player esce dall'app, incluse le voci conservate da un caricamento precedente.

Per percorsi come `/films/42`, imposta la directory di distribuzione dell'app e configura il server affinché serva il suo `index.html` per le rotte dell'applicazione:

```yaml
web:
  base: /        # /my-app/ when deployed below that directory
```

`web.base` scrive un base HTML in modo che script, stili, Wasm e risorse continuino a caricarsi anche quando un indirizzo profondo viene aperto direttamente. Senza di esso vengono mantenuti i percorsi relativi esistenti degli asset; gli indirizzi con query o hash possono usarli senza modifiche. Le regole di fallback del server devono lasciare invariate le richieste di asset reali. Una Content Security Policy rigorosa deve consentire questo base sulla stessa origine (`base-uri 'self'`).

Un'app incorporata in un'altra pagina mantiene il proprio indirizzo internamente e lascia invariata la cronologia della pagina contenitore. Un iframe della stessa origine può aderire esplicitamente tramite `data-nsgo-history`. I sottoalberi confinati non possono leggere né modificare l'indirizzo dell'applicazione.

Sugli altri target, `AppURL` conserva l'ultimo link aperto con l'app o il valore memorizzato da `SetAppURL`; impostarlo non modifica alcuna finestra nativa né il browser di sistema. Anche i mini program mantengono questo valore, senza cambiare le rotte del proprio host.

TSX utilizza lo stesso stato:

```tsx
import { UrlScheme } from "nsgo";

UrlScheme.url();
UrlScheme.setUrl("/films/42", { history: "push" });
const stop = UrlScheme.on("change", ({ url, reason }) => {
  // Update the app's route from the browser's choice.
});
```
