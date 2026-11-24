---
title: Navigation
description: "Native Navigationsstapel, System-Header, Modals, Menüs und Tab-Leisten."
group: Kernkonzepte
order: 56
---

## Ein Stapel

`base.NewNav` hält einen Stapel von Routen eines Typs: `Push` fügt eine Seite hinzu, `Pop` entfernt die oberste Seite, `PopToRoot` kehrt zur ersten zurück und `Replace` tauscht die oberste aus. Sein `Stack` rendert jede Route als Seite und hält die darunterliegenden Seiten mitsamt ihrem Zustand eingehängt, während die oberste angezeigt wird. `ui.NavPage` gibt jeder Seite den Hintergrund von `ui` und einen `ui.NavHeader` mit ihrem Titel, Zurück-Button und Buttons.

watchOS hat keine System-Navigationsleiste, keine Tab-Leiste und keinen Schalter, daher zeichnet `ui.NavPage` dort seinen `ui.NavHeader` samt Zurück-Button über der Seite, `ui.TabShell` zeichnet seine eigene Tab-Leiste und `ui.SwitchSystem` ist der eigene Schalter von `ui`.

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

Auf iOS und Android zeichnet die Plattform den Header des Systems aus dem Titel der Seite und den Buttons des Headers, mit seinem Zurück-Button und seinem Übergang; auf macOS, Windows, Linux und im Web zeichnet `ui` ihn.

Android verwendet eine Material-3-Top-App-Bar. Eine reguläre Leiste ist 64 dp hoch und hat einen Titel mit 22 sp. `HeaderLargeTitle` ergibt eine große Leiste mit 152 dp und einem Titel mit 28 sp, die zur regulären Leiste zusammenklappt, wenn die Scroll-View oben auf der Seite ihren Inhalt darunter scrollt, und sich wieder ausklappt, wenn der Inhalt an den Anfang zurückkehrt.

Setzen Sie `HeaderTitleStyle` und `HeaderLargeTitleStyle`, um die Schriften des nativen kompakten und ausgeklappten Titels auf iOS und Android zu gestalten: `FontSize`, `FontWeight`, `FontFamily` und `FontStyle`. Nicht gesetzte Felder behalten die Standardwerte der jeweiligen Plattform; `HeaderTitleStyleFunc` und `HeaderLargeTitleStyleFunc` folgen Änderungen. `HeaderStyle` liefert weiterhin den Hintergrund, die Titelfarbe und die Button-Tönung. In TSX sind dies die Style-Objekte oder -Arrays `headerTitleStyle` und `headerLargeTitleStyle`.

Ein eigener `HeaderTitle` ersetzt auf Android nur den kompakten Titel; `Title` bleibt der ausgeklappte große Titel. Setzen Sie `Android: core.AndroidScreenProps{HeaderRightWithLargeTitle: true}`, um `HeaderRight` neben diesen großen Titel zu stellen. Seine Steuerelemente wandern in die angeheftete Toolbar, wenn der Titel zusammenklappt, und bleiben durchgehend drückbar. TSX verwendet `android={{ headerRightWithLargeTitle: true }}`. Das große Band wächst, wenn seine Schrift mehr Platz braucht. Eigenständige Android-Builds aktivieren das native RTL-Layout; ein einbettender Host muss `android:supportsRtl="true"` für seine Application setzen, damit native Header der Richtung der App folgen.

Geben Sie einer Seite einen eigenen `Header`, um `ui.NavHeader` zu ersetzen. Auf iOS und Android wird er anstelle von `HeaderTitle` zur Titel-View des System-Headers, neben `HeaderLeft`, `HeaderRight` und dem Zurück-Button der Seite; er füllt den Platz zwischen diesen Enden aus, wenn seine Views wachsen oder eine prozentuale Breite einnehmen, etwa ein Suchfeld, und nimmt andernfalls die Größe seines Inhalts an. An anderer Stelle wird er über dem Inhalt gezeichnet. `ui.NavHeader.Scroll` verbindet weiterhin das Scrollen einer iOS- oder Android-Seite mit der Unschärfe ihres Headers.

`NoPopOnBack` behält eine Seite, wenn Zurück gedrückt wird, `OnBackPress` erfährt von dem Druck und `NoPopOnSwipe` schaltet das Zurückwischen von iOS ab. `cx.OnBackPress(func() bool)` gibt der App einen Handler vor dem Stapel, etwa für ungespeicherte Änderungen. `OnDismiss` meldet, dass der Nutzer die Seite entfernt hat, und der Stapel entfernt sie.

## Immersiver Vollbildmodus

Rufen Sie `mobile.Immersive(cx, true)` auf, um für das Fenster den Vollbildmodus anzufordern, und `mobile.Immersive(cx, false)`, um zu seinen regulären Einstellungen der Systemleisten zurückzukehren. Für eine Seite oder einen Dialog verwenden Sie `StatusBar: core.StatusBarSetting{Immersive: core.ToggleOn}`, um ihn anzufordern, solange diese Ebene angezeigt wird. In TSX verwenden Sie `statusBar={{ immersive: true }}` auf einer Seite oder einem Dialog oder `status_bar: { immersive: true }` in `nsgo.yaml` als Standard für das Fenster.

Auf Android zeigt ein Wischen vom Rand kurz die Status- und Navigationsleiste an. Auf HarmonyOS bittet das Fenster das System, die Statusleiste, die Drei-Tasten-Navigationsleiste und den Navigationsindikator auszublenden. Auf iOS wird die Statusleiste ausgeblendet, und UIKit entscheidet, wann der Home-Indikator aus- oder eingeblendet wird. `cx.Env().SafeArea()` meldet weiterhin die Abstände, die das System liefert, einschließlich Display-Aussparungen. Auf HarmonyOS mit Drei-Tasten-Navigation im Querformat meldet das System den Bereich der Tasten am unteren Rand, während es sie an einer Seite zeichnet, sodass der sichere Bereich unten Platz lassen kann, während die Tasten die Seite verdecken.

## Modals

`ui.Dialog` zeigt eine Fläche über einer abgedunkelten App, auf Touchscreens ein Sheet von unten, und `ui.Sheet` ein Panel von unten. Beide schließen sich bei Escape und einem Druck auf ihren Hintergrund, sofern nicht anders angewiesen.

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

`ui.AlertDialog` bittet den Nutzer um eine Bestätigung, und `cx.ShowAlert` zeigt den eigenen Alert oder das eigene Action Sheet des Systems ([Dialog](ui/dialog.md)).

## Menüs

`ui.Menu` öffnet `ui.MenuItem`s von einem Button aus: daneben oder, wenn seine `Presentation` es verlangt, auf jedem Gerät als Sheet von unten. `ui.ContextMenu` tut dasselbe für einen Sekundärklick oder ein langes Drücken, und `ui.Popover`, `ui.HoverCard` und `ui.Tooltip` zeigen andere verankerte Inhalte.

```go
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", OnSelect: duplicate},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Tabs

`ui.Tabs[T]` ist ein Segmented Control oder eine Tab-Liste mit einem Indikator, der zur gewählten Option gleitet; `TabStyle` wird über den Style jedes Tabs gelegt, sein `Selected` über den des gewählten Tabs und `IndicatorStyle` über den des Indikators. Unterstrichene Tabs können `IndicatorColor` setzen und die Linie mit `IndicatorFit: TabsIndicatorContent` an ihren Inhalt anpassen. `ui.Sidebar` enthält Abschnitte und Zeilen für eine Desktop-Seitenleiste, und `ui.TabShell` hält Abschnitte der obersten Ebene bei jeder Fensterbreite neben einer Tab-Leiste (standardmäßig unten oder mit `Edge` / `EdgeFunc` vertikal am führenden oder nachfolgenden Rand, einschließlich `mobile.TabBarAutomatic`, das `cx.Env().VerticalBarEdge()` folgt). Sie verwendet die Leiste des Systems, wenn jeder Tab ein `SystemIcon` oder ein SVG-`Icon` hat, auf iOS an jedem Rand und auf Android am unteren Rand, und zeigt das SVG überall dort, wo `SystemIcon` auf iOS kein SF Symbol bzw. auf Android kein Drawable benennt; andernfalls zeichnet sie die Leiste mit den SVG-Icons. Setzen Sie `BarHidden`, solange eine gepushte Detailseite sichtbar ist, um eine der beiden Leisten auszublenden und den sicheren Bereich des Abschnitts wiederherzustellen; `TabBarInset` wird 0, solange sie ausgeblendet oder vertikal platziert ist.

`mobile.TabBar` ist die Tab-Leiste des Systems auf iOS und Android, mit ihrem Liquid-Glass- bzw. Material-Erscheinungsbild; sie nimmt `Items` vom Typ `mobile.TabBarItem{Title, Icon, SVG}`, den ausgewählten Index, `OnChange` und optional einen `Edge` (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` oder `mobile.TabBarAutomatic`, das auf iOS 27.1 `cx.Env().VerticalBarEdge()` folgt und anderswo auf den unteren Rand zurückfällt) oder ein reaktives `EdgeFunc`. Die Palettenrollen `tabBarBackground`, `tabBarIdle`, `tabBarSelected` und `tabBarIndicator` von Android legen den Hintergrund der Leiste, die inaktiven Icons und Beschriftungen, das ausgewählte Icon und die ausgewählte Beschriftung sowie die ausgewählte Pille fest. Eigene Paletten fallen jeweils auf `Surface`, `TextMuted`, `Accent` und `accentContainer` zurück, mit einer Pille aus 12 % Akzentfarbe, wenn `accentContainer` nicht gesetzt ist. Die Standardpalette behält den Hintergrund und die inaktiven Farben von Material, wobei der ausgewählte Tab mit der Akzentfarbe getönt wird. An anderer Stelle nimmt sie keinen Platz ein, daher zeigt eine App dort `ui.Tabs`. `mobile.IsAvailable(cx)` gibt an, ob die Leiste angezeigt wird. `Icon` benennt ein Plattform-Icon; `SVG`, etwa `lucide.House()`, wird auf iOS und Android überall dort angezeigt, wo `Icon` kein SF Symbol bzw. kein Drawable benennt, und von der Leiste wie ein System-Icon getönt.

Setzen Sie `TabBarItem.Badge` auf `mobile.TabBadge{Text: "3"}` für Text oder auf `mobile.TabBadge{Dot: true}` für einen Punkt. Aktualisieren Sie es über `ItemsFunc` und geben Sie den Nullwert zurück, um es zu entfernen. `ui.ShellTab.Badge` nimmt `func() mobile.TabBadge` für die Leiste des Systems ebenso wie für die gezeichnete Leiste. Text hat Vorrang, wenn beide Felder gesetzt sind. UIKit zeigt für einen Punkt ein leeres natives Badge. Assistive Technologien lesen den Badge-Text oder das lokalisierte Wort für neue Inhalte vor. TSX nimmt `badge: { text: "3" }` oder `badge: { dot: true }` auf einem nativen Element und eine Funktion, die diesen Wert zurückgibt, auf einem `TabShell`-Element.

```go
mobile.TabBar{
	Items: []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
	Selected: func() int { return tab.Get() },
	OnChange: func(i int) { tab.Set(i) },
}
```

## Weiter

- [Formulare](forms.md): was eine Seite eines Stapels enthält.
- [Animation](animation.md): wie sich Seiten und Modals bewegen.
- [Layout](layout.md): der sichere Bereich um einen Header.
