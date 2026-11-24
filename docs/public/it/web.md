---
title: Pubblicare sul web
description: Pubblica un'app NSGo con semantica del documento, metadati di pagina e asset su CDN.
group: Strumenti
order: 24
---

## Compilare l'app

```sh
go tool nsgo build -target web
```

Servi la directory generata tramite HTTP. Gli asset di release hanno hash del contenuto nei loro nomi, quindi memorizzali in cache come immutabili; rivalida `index.html`, `nsgo.json` e `manifest.webmanifest`. L'app web mantiene il layout, lo scorrimento, il focus e il comportamento del puntatore di NSGo.

## Intestazioni e link

Una `core.View` o un `core.Text` con `Role: core.RoleHeader` diventa un elemento da `h1` a `h6`, scelto da `AccessibilityLevel`; senza un livello è `h2`. Le intestazioni Markdown di `base/textview` conservano i propri livelli. I margini e i font delle intestazioni del browser non modificano lo stile dell'app.

`base.Link` e `ui.Link` pubblicano il proprio `Href` come ancora. Un clic normale esegue `OnOpen` e `OnPress` dell'app; un clic centrale o un clic con un tasto modificatore usa l'azione del browser per i link. Il browser fornisce il proprio menu contestuale per i link, a meno che l'app non gestisca `OnContextMenu`. Un link disabilitato non ha un indirizzo attivo. Per una vista personalizzata con ruolo di link, imposta `core.View.Web` su `core.WebViewProps{Href: "/notes"}` oppure collega `HrefFunc`. TSX usa `web={{href: "/notes"}}`; le intestazioni usano `accessibilityLevel={1}`. Gli schemi URL eseguibili non sicuri vengono rifiutati. Il contenuto con restrizioni mantiene la navigazione nei propri gestori di pressione circoscritti e non espone alcun indirizzo al browser.

## Metadati di pagina

Importa `github.com/nsgo-dev/nsgo/web` e colloca un `web.Head` accanto al contenuto di una pagina:

```go
core.Group{
	web.Head{Title: "Notes", Description: "Your saved notes"},
	pageContent,
}
```

`TitleFunc` e `DescriptionFunc` seguono i signal. Gli head visibili successivi hanno la precedenza sui campi che impostano ed ereditano gli altri. Nascondere o rimuovere una pagina ripristina gli head precedenti; rimuovere tutti gli head ripristina il documento originale. Un campo statico vuoto eredita; una funzione che restituisce una stringa vuota lo azzera. Un head non aggiunge alcun nodo di layout. I target nativi, le app integrate e il contenuto con restrizioni lasciano invariato il documento del proprio host.

In TSX, importa `Head` da `nsgo/web` e scrivi `<Head title={title()} description="Your saved notes" />`. Per le anteprime HTML statiche, esegui il prerendering di ogni URL come parte del deployment del sito; `Head` aggiorna il documento in esecuzione.

## Servire gli asset da una CDN

Il loader risolve tutti i file dell'app in modo relativo a `nsgo.json`. Una pagina può caricare l'app da un'altra origine:

```js
const url = new URL("https://cdn.example.com/notes/nsgo.json");
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url).href);
await mount(null, url.href, { config });
```

Consenti all'origine della pagina di recuperare i file della CDN tramite CORS, e servi il WebAssembly come `application/wasm`. Servi l'HTML di ingresso dell'app per tutte le route profonde usate dalla tua app. [Integrazione](embedding.md#web) mostra come montarla invece in un elemento.
