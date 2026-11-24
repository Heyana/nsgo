---
title: Riferimento dei componenti
description: I componenti più usati, le loro varianti, i loro stati e la loro accessibilità.
group: Componenti
order: 30
---

Questi sono i componenti di `ui`, con l'aspetto di NSGo e conformi al tema. Questo riferimento parte dai componenti a cui un'app ricorre per primi; ogni pagina inizia con l'esempio più breve, poi ne descrive le parti, gli stati e l'accessibilità. La galleria copre ogni componente, compresi quelli che qui non hanno ancora una pagina.

La [galleria](https://nsgo.dev/gallery/) esegue nel browser una demo di ogni componente.

## Azioni

- [Button](ui/button.md): un pulsante, semplice o nel colore di accento, e il pulsante a icona.

## Testo e input

- [Text](ui/text.md): paragrafi, span, link, font e testo di documento.
- [Input](ui/input.md): il campo di testo della piattaforma, controllato o no.
- [Checkbox](ui/checkbox.md): una casella di spunta, selezionata, deselezionata o indeterminata.
- [Switch](ui/switch.md): un'impostazione attiva o disattiva.
- [Select](ui/select.md): un valore da un elenco, come popup o come foglio.
- [Slider](ui/slider.md): un valore o un intervallo lungo una traccia.

## Collezioni

- [List](ui/list.md): righe in qualsiasi numero, costruite solo quando visibili, con selezione e attivazione.
- [Scroll](ui/scroll.md): la vista di scorrimento della piattaforma, con paginazione, aggancio, zoom e aggiornamento.

## Navigazione

- [Tabs](ui/tabs.md): un elenco di schede, un controllo segmentato e la barra a schede di sistema.

## Overlay

- [Dialog](ui/dialog.md): una superficie modale, un foglio e l'avviso di sistema.
- [Menu](ui/menu.md): un menu da un attivatore, un menu contestuale e le loro voci.

## Altro

La galleria copre il resto: tabelle, grafici, l'editor di codice, il testo formattato, il terminale, il dock, la palette dei comandi, i selettori di colore, data ed emoji (`ui.EmojiPicker`), i toast e altro ancora. Ognuno vi ha una demo.

`base/terminal.View` apre i percorsi locali esistenti con Command+clic (Control+clic su Windows e Linux), così come apre gli URL. I percorsi relativi usano la directory di lavoro OSC 7 della shell; una shell locale che non ne ha segnalata una usa `terminal.Options.Dir`, oppure la home directory se non è impostata. Metti tra virgolette i nomi di file che contengono spazi. Le autorità di directory remote e le connessioni senza una directory locale segnalata non vengono risolte rispetto ai file locali. File e cartelle usano `Scope.OpenFile`; i link URL continuano a passare per `Scope.OpenURL` e per il gestore di URL dell'host che integra l'app.

`base/terminal.View` nega per impostazione predefinita le letture e le scritture degli appunti da parte dei programmi. Imposta `AllowClipboardRead` e `AllowClipboardWrite` solo quando l'app le consente; si applica comunque `AccessClipboard` della vista richiedente. Le stesse prop sono disponibili sul `Terminal` di TSX e possono cambiare senza riavviarne la sessione. Le app forniscono l'interfaccia per il consenso. Un incolla esplicito da parte dell'utente offre in modo indipendente un'istantanea testo/PNG ai programmi che usano il protocollo degli appunti di Kitty (modalità 5522); la sua password monouso può leggere solo quell'istantanea. I programmi senza tale modalità ricevono normale testo in bracketed paste. La disponibilità delle immagini negli appunti segue i servizi degli appunti della piattaforma; la selezione primaria di X11 non è esposta.

## Prossimi passi

- [Componenti e stato](components.md): come un componente restituisce elementi e come i signal li aggiornano.
- [Stili e temi](styles.md): il tema da cui i componenti prendono i propri colori.
- [Componenti senza stile](authoring/base.md): gli stessi controlli privi di aspetto, per un aspetto tutto tuo.
- [Accessibilità](accessibility.md): etichette, ruoli e valori per ogni controllo.
