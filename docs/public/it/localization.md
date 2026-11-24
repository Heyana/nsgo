---
title: Localizzazione
description: Traduci i messaggi di un'app e segui la lingua che mostra.
group: Concetti chiave
order: 62
---

## Messaggi

`locales/`, accanto a `nsgo.yaml`, contiene i messaggi dell'app, un file YAML per lingua (`en.yaml`, `de.yaml`) in formato ICU MessageFormat:

```yaml
greeting: Hello, {name}!
appName:
  message: Hello
  description: The app's name, for translators.
```

Un messaggio è una stringa, oppure `message` con una `description` e dei `placeholders` per i traduttori.

Il pacchetto importa `locales/`. `dev`, `build` e `test` controllano i cataloghi e scrivono accanto a essi una funzione Go per ogni messaggio, come `locales.Greeting(cx, "Ada")`, che lo pronuncia nella lingua mostrata dall'app e la segue quando cambia; `go tool nsgo i18n` scrive soltanto le funzioni. I messaggi formattano numeri e date (`{d, date, medium}`), e `appName` dà il nome all'app in ciascuna lingua nei suoi pacchetti, così come un messaggio con una chiave `apple.info`, come `NSCameraUsageDescription`, traduce quella voce di Info.plist ([permessi](permissions.md#usage-descriptions)).

## La lingua

`cx.SetLocale("de")` sceglie la lingua, e il layout segue la direzione in cui essa si scrive. In un'app standalone, anche ciò che il sistema mostra di suo, come i menu di selezione del testo, le finestre di dialogo dei file e i widget di GTK, segue la lingua forzata (immediatamente su Android 13+, Windows e Linux, e dal successivo avvio su macOS e iOS); `cx.SetLocale("")` torna a seguire la lingua dell'utente.

```sh
go tool nsgo dev -lang en-XA
```

esegue l'app in una pseudo-locale che evidenzia il testo non tradotto e quello troncato. Qualsiasi build la accetta all'avvio: `--nsgo-lang=en-XA`.

## Settimane, orari e unità di misura

`i18n.Regional(cx)` indica come l'utente legge settimane, orari e unità di misura, indipendentemente dalla lingua: il giorno in cui iniziano le settimane, l'orario a 12 o 24 ore, il sistema di misura e l'unità di temperatura, come li ha impostati sulla piattaforma, altrimenti come li usa la sua regione. Calendari e selettori di data iniziano le settimane da quel giorno e i campi orari mostrano quelle ore, a meno che l'app non imposti `FirstDayOfWeek` o `HourCycle`, e gli orari scritti dai messaggi di un catalogo seguono le ore dell'utente.

## Le parole di NSGo

I componenti di NSGo pronunciano le proprie parole nelle lingue in cui NSGo è tradotto. Per un'altra lingua, una chiave che nomina un pacchetto, come `github.com/nsgo-dev/nsgo/base/locales`, fornisce i messaggi di quel pacchetto nella lingua del file. I menu delle piattaforme, e i ruoli e gli stati annunciati dagli screen reader di Android e Windows dove il sistema non ha una parola propria, come una scheda o una casella di ricerca, usano le parole di `github.com/nsgo-dev/nsgo/desktop/locales`.
