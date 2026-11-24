---
title: Schede
description: Un elenco di schede, un controllo segmentato e la barra delle schede di sistema.
group: Componenti
order: 40
---

`ui.Tabs[T]` sceglie uno tra pochi valori, come controllo segmentato o come elenco di schede; l'app mostra accanto il contenuto del valore scelto. Un elenco di schede è un'unica fermata del tasto Tab: le frecce si spostano tra le schede.

```go preview=tabs/basic
section := core.NewSignal(cx, "overview")
ui.Tabs[string]{
	Options: []ui.Option[string]{
		{Value: "overview", Label: "Overview"},
		{Value: "activity", Label: "Activity"},
		{Value: "settings", Label: "Settings"},
	},
	Value:    section.Get,
	OnChange: section.Set,
	Label:    "Sections",
}
```

## Varianti e dimensioni

`Variant` è `TabsSegmented` (il valore predefinito, un indicatore in rilievo in una traccia incassata) oppure `TabsUnderline` (una linea di accento sotto la scheda selezionata). Una sottolineatura può impostare `IndicatorColor` e usare `IndicatorFit: TabsIndicatorContent` per seguire la propria etichetta e la propria icona anziché l'intera scheda; in TSX sono `indicatorColor` e `indicatorFit="content"`. `Size` è `SizeDefault`, `SizeSmall` o `SizeLarge`, e `Equal` assegna a ogni scheda la stessa larghezza invece di adattarla alla sua etichetta.

```go preview=tabs/underline
ui.Tabs[string]{Variant: ui.TabsUnderline, Equal: true, Size: ui.SizeSmall, Options: tabs, Value: section.Get, OnChange: section.Set}
```

Un'opzione può avere un `Icon`, una funzione `Disabled`, un nodo `Content` e un `TestID`. `Content` sostituisce l'etichetta ed eredita lo stile del testo della scheda.

Usa `OptionsFunc` quando le schede seguono un signal. NSGo aggiorna le righe sul posto in base ai loro valori, mantenendo la scheda selezionata e il focus della tastiera finché quel valore resta tra quelli offerti; un valore che scompare resta non selezionato finché l'app non lo cambia.

## Barre delle schede

`ui.TabShell` mantiene le sezioni di primo livello accanto a una barra delle schede a qualsiasi larghezza della finestra: in basso per impostazione predefinita, oppure in verticale lungo il bordo iniziale o finale quando `Edge` (o `EdgeFunc`) vale `mobile.TabBarLeading`, `mobile.TabBarTrailing` o `mobile.TabBarAutomatic` (che segue `cx.Env().VerticalBarEdge()`). Usa la barra di sistema quando ogni scheda fornisce un `SystemIcon` o un `Icon` SVG, su qualsiasi bordo su iOS e sul bordo inferiore su Android; un `SystemIcon` che indica un SF Symbol su iOS o un drawable su Android ha la precedenza, altrimenti viene mostrato l'SVG. In caso contrario disegna la barra con le icone SVG fornite. Una barra che occupa uno spazio proprio si tiene da sola lontana dall'area sicura del suo bordo, perciò `core.EnvSafeArea` su quel bordo di una sezione accanto ad essa, inclusa una pagina `base.Nav`, vale 0; sotto la barra di sistema inferiore fluttuante di iOS, tieni il contenuto a distanza con `ui.TabBarInset`.

Imposta `ShellTab.TextFunc` per titoli delle schede che seguono la lingua o lo stato dell'applicazione e `TabShell.LabelFunc` per il nome accessibile della barra. La modifica di un titolo preserva la sezione selezionata e la scheda esistente, purché la sua icona resti la stessa.

`mobile.TabBar` è la barra delle schede di sistema su iOS e Android, con `Items`, `Selected`, `OnChange` e, facoltativamente, `Edge` (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` o `mobile.TabBarAutomatic`) / `EdgeFunc`. Ogni `mobile.TabBarItem` accetta un `Title` e il nome di un'icona della piattaforma in `Icon`; `SVG: lucide.House()` viene mostrato ovunque `Icon` non indichi un SF Symbol su iOS o un drawable su Android, adattato a 24 punti e colorato in base alla selezione. In TSX, `svg` accetta un documento SVG ed `edge` accetta `"bottom"`, `"leading"`, `"trailing"` o `"automatic"`. Altrove `mobile.TabBar` non occupa spazio, quindi lì mostra `ui.Tabs`.

`ui.SidebarShell` tiene da sola lontano dall'area sicura sinistra e destra ciò che contengono la barra laterale, il piè di pagina, le intestazioni, il contenuto e il pannello destro, con gli sfondi che arrivano fino ai bordi della finestra, perciò al loro interno `core.EnvSafeArea` su quei lati vale 0, e `ui.NavHeader` aggiunge l'area sicura laterale al proprio padding. Il contenuto di una `ui.NavPage` o di una sezione di `TabShell` si tiene da solo lontano dall'area sicura, con `PaddingEnv`, così ciò che deve arrivare fino ai bordi, come un'immagine a tutta pagina, può farlo.

## Accessibilità

L'elenco segnala `RoleTabList`, ogni scheda `RoleTab` e la scheda selezionata il proprio stato. L'elenco è un'unica fermata del tasto Tab; Sinistra e Su, Destra e Giù, Home e Fine spostano tra le schede abilitate, e la scheda raggiunta segnala una pressione. `Label` dà un nome all'elenco.

## Piattaforma

iOS e Android mostrano la barra delle schede di sistema con il proprio aspetto e i propri gesti; macOS, Windows, Linux e il web mostrano `ui.Tabs`. Il contenuto dell'app è lo stesso su ogni target.

## Correlati

- [Navigazione](../navigation.md): pagine e barre delle schede in un'app.
- [Menu](menu.md): comandi anziché sezioni.
- [Select](select.md): molte opzioni in un elenco.
- Le pagine Tabs e NavStack della galleria.
- [Componenti senza stile](../authoring/base.md#tabs): `base.Tabs`, per schede con un aspetto personalizzato.
