---
title: Navigazione
description: Stack di navigazione nativi, header di sistema, modali, menu e barre delle schede.
group: Concetti chiave
order: 56
---

## Uno stack

`base.NewNav` mantiene uno stack di route di un unico tipo: `Push` aggiunge una pagina, `Pop` rimuove la pagina in cima, `PopToRoot` torna alla prima e `Replace` sostituisce quella in cima. Il suo `Stack` mostra ogni route come una pagina, mantenendo montate con il loro stato le pagine sottostanti mentre viene mostrata quella in cima. `ui.NavPage` assegna a ogni pagina lo sfondo di `ui` e un `ui.NavHeader` con il titolo, il pulsante Indietro e i pulsanti.

watchOS non ha una barra di navigazione, una barra delle schede o uno switch di sistema, quindi lì `ui.NavPage` disegna il proprio `ui.NavHeader` sopra la pagina, pulsante Indietro compreso, `ui.TabShell` disegna la propria barra delle schede e `ui.SwitchSystem` è lo switch di `ui`.

```go
nav := base.NewNav(cx, "inbox")

return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
	if route == "inbox" {
		return ui.NavPage(cx, base.NavPage{Title: "Inbox", Children: []core.Node{
			ui.Button{Text: "Open a message", OnPress: func() { nav.Push("message") }},
		}}, ui.NavHeader{})
	}
	return ui.NavPage(cx, base.NavPage{Title: "Message"}, ui.NavHeader{
		OnBack: func() { nav.Pop(1) }, BackLabel: "Inbox",
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Archive, Label: "Archive", Variant: ui.IconButtonGhost, OnPress: archive}},
	})
})
```

Su iOS e Android la piattaforma disegna l'header di sistema a partire dal titolo della pagina e dai pulsanti dell'header, con il proprio pulsante Indietro e la propria transizione; su macOS, Windows, Linux e il web lo disegna `ui`.

Android usa una top app bar di Material 3. Una barra normale è alta 64 dp con un titolo di 22 sp. `HeaderLargeTitle` offre una barra grande di 152 dp con un titolo di 28 sp che si comprime nella barra normale quando la vista di scorrimento in cima alla pagina fa scorrere il proprio contenuto sotto di essa, e si espande quando il contenuto torna in cima.

Imposta `HeaderTitleStyle` e `HeaderLargeTitleStyle` per definire lo stile dei font nativi del titolo compatto ed espanso su iOS e Android: `FontSize`, `FontWeight`, `FontFamily` e `FontStyle`. I campi non impostati mantengono i valori predefiniti di ciascuna piattaforma; `HeaderTitleStyleFunc` e `HeaderLargeTitleStyleFunc` seguono le modifiche. `HeaderStyle` continua a fornire lo sfondo, il colore del titolo e la tinta dei pulsanti. In TSX questi sono oggetti o array di stile `headerTitleStyle` e `headerLargeTitleStyle`.

Un `HeaderTitle` personalizzato sostituisce su Android solo il titolo compatto; `Title` resta il titolo grande espanso. Imposta `Android: core.AndroidScreenProps{HeaderRightWithLargeTitle: true}` per mettere `HeaderRight` accanto a quel titolo grande. I suoi controlli si spostano nella toolbar fissata quando il titolo si comprime e restano premibili per tutto il tempo. TSX usa `android={{ headerRightWithLargeTitle: true }}`. La fascia grande cresce quando il suo font ha bisogno di più spazio. Le build Android standalone abilitano il layout RTL nativo; un host incorporato deve impostare `android:supportsRtl="true"` sulla propria application affinché gli header nativi seguano la direzione dell'app.

Assegna a una pagina il proprio `Header` per sostituire `ui.NavHeader`. Su iOS e Android diventa la vista del titolo dell'header di sistema, al posto di `HeaderTitle`, accanto a `HeaderLeft`, `HeaderRight` della pagina e al pulsante Indietro; riempie lo spazio tra quelle estremità quando le sue viste crescono o assumono una larghezza in percentuale, come un campo di ricerca, e altrimenti assume la dimensione del suo contenuto. Altrove viene disegnato sopra il contenuto. `ui.NavHeader.Scroll` collega comunque lo scorrimento di una pagina iOS o Android alla sfocatura del suo header.

`NoPopOnBack` mantiene una pagina quando si preme Indietro, `OnBackPress` riceve la pressione e `NoPopOnSwipe` disattiva lo swipe indietro di iOS. `cx.OnBackPress(func() bool)` fornisce all'app un gestore che interviene prima dello stack, per le modifiche non salvate. `OnDismiss` segnala che l'utente ha rimosso la pagina, e lo stack la elimina.

## Schermo intero immersivo

Chiama `mobile.Immersive(cx, true)` per richiedere lo schermo intero per la finestra e `mobile.Immersive(cx, false)` per tornare alle sue impostazioni consuete delle barre di sistema. Per una pagina o una finestra di dialogo, usa `StatusBar: core.StatusBarSetting{Immersive: core.ToggleOn}` per richiederlo mentre quel livello è mostrato. In TSX, usa `statusBar={{ immersive: true }}` su una pagina o una finestra di dialogo, oppure `status_bar: { immersive: true }` in `nsgo.yaml` come impostazione predefinita della finestra.

Su Android uno swipe dal bordo mostra brevemente la barra di stato e la barra di navigazione. Su HarmonyOS la finestra chiede al sistema di nascondere la barra di stato, la barra di navigazione a tre pulsanti e l'indicatore di navigazione. Su iOS la barra di stato si nasconde e UIKit decide quando nascondere o mostrare l'indicatore Home. `cx.Env().SafeArea()` continua a segnalare i margini forniti dal sistema, compresi i ritagli del display. Su HarmonyOS con la navigazione a tre pulsanti in orizzontale, il sistema segnala l'area dei pulsanti lungo il bordo inferiore mentre li disegna lungo un lato, quindi l'area sicura può lasciare spazio in basso mentre i pulsanti coprono il lato.

## Modali

`ui.Dialog` mostra una superficie sopra l'app oscurata, un pannello dal basso sugli schermi touch, e `ui.Sheet` un pannello dal basso. Ciascuno si chiude con Esc e con una pressione sullo sfondo, a meno che non gli venga indicato diversamente.

```go
ui.Dialog{
	Open:         dialog.Get,
	OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) },
	Title:        "Rename",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { dialog.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

`ui.AlertDialog` chiede conferma all'utente, e `cx.ShowAlert` mostra l'avviso o l'action sheet propri del sistema ([Dialog](ui/dialog.md)).

## Menu

`ui.Menu` apre degli `ui.MenuItem` da un pulsante: accanto a esso, oppure come pannello dal basso quando il suo `Presentation` lo richiede, su qualsiasi dispositivo. `ui.ContextMenu` fa lo stesso per un clic secondario o una pressione prolungata, e `ui.Popover`, `ui.HoverCard` e `ui.Tooltip` mostrano altri contenuti ancorati.

```go
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", OnSelect: duplicate},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Schede

`ui.Tabs[T]` è un controllo segmentato o un elenco di schede, con un indicatore che scorre fino all'opzione scelta; `TabStyle` si sovrappone allo stile di ogni scheda, il suo `Selected` a quello della scheda scelta, e `IndicatorStyle` a quello dell'indicatore. Le schede sottolineate possono impostare `IndicatorColor` e adattare la linea al proprio contenuto con `IndicatorFit: TabsIndicatorContent`. `ui.Sidebar` contiene sezioni e righe per una barra laterale desktop, e `ui.TabShell` mantiene le sezioni di primo livello accanto a una barra delle schede (in basso per impostazione predefinita, oppure verticale sul bordo iniziale o finale con `Edge` / `EdgeFunc`, compreso `mobile.TabBarAutomatic` per seguire `cx.Env().VerticalBarEdge()`) a ogni larghezza della finestra. Usa la barra di sistema quando ogni scheda ha una `SystemIcon` o un'`Icon` SVG, su qualsiasi bordo su iOS e sul bordo inferiore su Android, mostrando l'SVG ovunque `SystemIcon` non nomini un SF Symbol su iOS o un drawable su Android; altrimenti disegna la barra con le icone SVG. Imposta `BarHidden` mentre è visibile una pagina di dettaglio aggiunta con push per nascondere l'una o l'altra barra e ripristinare l'area sicura della sezione; `TabBarInset` diventa 0 mentre la barra è nascosta o posta in verticale.

`mobile.TabBar` è la barra delle schede di sistema su iOS e Android, con il suo aspetto Liquid Glass o Material; accetta `Items` di `mobile.TabBarItem{Title, Icon, SVG}`, l'indice selezionato, `OnChange` e un `Edge` facoltativo (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` oppure `mobile.TabBarAutomatic`, che segue `cx.Env().VerticalBarEdge()` su iOS 27.1 e altrove ricade sul bordo inferiore) o un `EdgeFunc` reattivo. I ruoli della palette di Android `tabBarBackground`, `tabBarIdle`, `tabBarSelected` e `tabBarIndicator` impostano lo sfondo della barra, le icone e le etichette inattive, l'icona e l'etichetta selezionate e la pillola di selezione. Le palette personalizzate ricadono rispettivamente su `Surface`, `TextMuted`, `Accent` e `accentContainer`, con una pillola d'accento al 12% quando `accentContainer` non è impostato. La palette predefinita mantiene lo sfondo e i colori inattivi di Material, con la scheda selezionata colorata dall'accento. Altrove non occupa spazio, quindi lì un'app mostra `ui.Tabs`. `mobile.IsAvailable(cx)` indica se la barra viene mostrata. `Icon` nomina un'icona della piattaforma; `SVG`, come `lucide.House()`, viene mostrato su iOS e Android ovunque `Icon` non nomini un SF Symbol o un drawable, colorato dalla barra come avviene per un'icona di sistema.

Imposta `TabBarItem.Badge` su `mobile.TabBadge{Text: "3"}` per un testo o su `mobile.TabBadge{Dot: true}` per un punto. Aggiornalo tramite `ItemsFunc` e restituisci il valore zero per rimuoverlo. `ui.ShellTab.Badge` accetta `func() mobile.TabBadge` sia per la barra di sistema sia per quella disegnata. Il testo ha la precedenza quando sono impostati entrambi i campi. UIKit mostra un badge nativo vuoto per un punto. La tecnologia assistiva legge il testo del badge o la parola localizzata che indica nuovi contenuti. TSX accetta `badge: { text: "3" }` o `badge: { dot: true }` su un elemento nativo, e una funzione che restituisce quel valore su un elemento di `TabShell`.

```go
mobile.TabBar{
	Items: []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
	Selected: func() int { return tab.Get() },
	OnChange: func(i int) { tab.Set(i) },
}
```

## Prossimi passi

- [Moduli](forms.md): ciò che contiene una pagina di uno stack.
- [Animazioni](animation.md): come si muovono pagine e modali.
- [Layout](layout.md): l'area sicura attorno a un header.
