---
title: Menu
description: Un menu da un trigger, un menu contestuale e le loro voci.
group: Componenti
order: 42
---

`ui.Menu` apre delle `ui.MenuItem` da un pulsante: accanto a esso, oppure come pannello dal basso con `Presentation: base.PresentSheet`, qualunque sia il dispositivo. `ui.ContextMenu` apre le stesse voci in corrispondenza del puntatore.

```go preview=menu/basic height=280
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", Icon: lucide.Pencil, OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", Shortcut: "mod+d", OnSelect: duplicate},
	ui.MenuSeparator{},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

Il menu si apre e si chiude da solo; `Open` e `OnOpenChange` permettono all'app di gestirlo. `Trigger` è l'aspetto e il contenuto del pulsante, e il menu ne riceve le pressioni. Un popup non va mai oltre lo spazio disponibile accanto al suo trigger: si apre sul lato in cui entra per intero e le sue voci scorrono oltre quello spazio, o oltre `MaxHeight`.

## Voci

Una `ui.MenuItem` mostra `Text`, un'`Icon` facoltativa e una `Shortcut` (`"mod+c"`, scritta nel modo della piattaforma in un popup), e segnala `OnSelect`, chiudendo il menu a meno che non sia impostato `NoCloseOnSelect`. `Disabled` la disattiva, `Checked` mostra un segno di spunta accanto a essa e `Variant: ui.MenuItemDanger` ne contrassegna una distruttiva. `ui.MenuLabel` fa da intestazione a un gruppo e `ui.MenuSeparator` separa i gruppi. Un `ui.Submenu` contiene altre voci e si apre accanto alla voce oppure, in un pannello, al posto delle voci circostanti.

```go preview=menu/checked height=320
ui.Menu{Trigger: ui.Button{Text: "View", Variant: ui.ButtonGhost}, Children: []core.Node{
	ui.MenuLabel{Text: "Sort by"},
	ui.MenuItem{Text: "Name", Checked: func() bool { return sort.Get() == "name" }, OnSelect: func() { sort.Set("name") }},
	ui.MenuItem{Text: "Date", Checked: func() bool { return sort.Get() == "date" }, OnSelect: func() { sort.Set("date") }},
	ui.MenuSeparator{},
	ui.Submenu{Text: "Share", Children: []core.Node{
		ui.MenuItem{Text: "Copy link", OnSelect: copyLink},
		ui.MenuItem{Text: "Email", OnSelect: emailLink},
	}},
}}
```

## Menu contestuali

`ui.ContextMenu` apre le proprie voci con un clic secondario, o con una pressione prolungata sui touch screen, in corrispondenza del puntatore sopra il suo `Trigger`.

```go preview=menu/context height=220
ui.ContextMenu{Trigger: fileRow, Children: []core.Node{
	ui.MenuItem{Text: "Open", OnSelect: open},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Comportamento

Esc, il tasto indietro e una pressione all'esterno chiudono un menu. Il menu è un popup posizionato da `Placement` e `Align`, oppure un pannello dal basso quando viene presentato così. Su desktop, Giù, Su, Home, Fine, Pagina giù e Pagina su si spostano tra le voci abilitate, partendo da quella sotto il puntatore, Destra apre un sottomenu e Sinistra lo chiude, e Invio sceglie. Un menu aperto con la tastiera dà il focus alla sua prima voce, e il focus torna al trigger quando il menu si chiude.

## Accessibilità

Il menu segnala `RoleMenu` e ogni voce `RoleMenuItem`. `Label` dà un nome al trigger quando il suo contenuto non lo fa. Uno screen reader legge il testo di una voce e se è disabilitata.

## Piattaforma

Il trigger e le voci sono dell'app; il pannello, il popup, il focus, Esc e il tasto indietro sono della piattaforma. È l'app a scegliere il popup o il pannello; NSGo non sceglie nessuno dei due in base al dispositivo.

## Correlati

- [Select](select.md): un valore da un elenco.
- [Dialog](dialog.md): una superficie modale.
- [Tabs](tabs.md): sezioni mostrate contemporaneamente.
- Le pagine Menu, ContextMenu e Command della galleria.
- [Componenti senza stile](../authoring/base.md#menu): `base.Menu`, per un menu con un aspetto tutto tuo.
