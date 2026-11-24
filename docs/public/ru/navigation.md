---
title: Навигация
description: Нативные стеки навигации, системные заголовки, модальные окна, меню и панели вкладок.
group: Основные концепции
order: 56
---

## Стек

`base.NewNav` хранит стек маршрутов одного типа: `Push` добавляет страницу, `Pop` удаляет верхнюю страницу, `PopToRoot` возвращает к первой, а `Replace` заменяет верхнюю. Его `Stack` отображает каждый маршрут как страницу, сохраняя нижние страницы смонтированными вместе с их состоянием, пока показывается верхняя. `ui.NavPage` даёт каждой странице фон `ui` и `ui.NavHeader` с её заголовком, кнопкой «Назад» и кнопками.

В watchOS нет системной панели навигации, панели вкладок и переключателя, поэтому там `ui.NavPage` рисует свой `ui.NavHeader` над страницей, включая кнопку «Назад», `ui.TabShell` рисует собственную панель вкладок, а `ui.SwitchSystem` — это собственный переключатель `ui`.

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

На iOS и Android платформа рисует системный заголовок по заголовку страницы и кнопкам заголовка, со своей кнопкой «Назад» и своим переходом; на macOS, Windows, Linux и в вебе его рисует `ui`.

Android использует верхнюю панель приложения Material 3. Обычная панель имеет высоту 64 dp с заголовком 22 sp. `HeaderLargeTitle` даёт большую панель высотой 152 dp с заголовком 28 sp, которая сворачивается в обычную панель, когда область прокрутки в верхней части страницы прокручивает под ней своё содержимое, и разворачивается, когда содержимое возвращается к началу.

Задайте `HeaderTitleStyle` и `HeaderLargeTitleStyle`, чтобы оформить шрифты нативного компактного и развёрнутого заголовков на iOS и Android: `FontSize`, `FontWeight`, `FontFamily` и `FontStyle`. Незаданные поля сохраняют значения по умолчанию каждой платформы; `HeaderTitleStyleFunc` и `HeaderLargeTitleStyleFunc` следуют за изменениями. `HeaderStyle` по-прежнему задаёт фон, цвет заголовка и оттенок кнопок. В TSX это объекты или массивы стилей `headerTitleStyle` и `headerLargeTitleStyle`.

Пользовательский `HeaderTitle` заменяет на Android только компактный заголовок; `Title` остаётся развёрнутым большим заголовком. Задайте `Android: core.AndroidScreenProps{HeaderRightWithLargeTitle: true}`, чтобы разместить `HeaderRight` рядом с этим большим заголовком. Его элементы управления переходят на закреплённую панель инструментов, когда заголовок сворачивается, и всё время остаются доступными для нажатия. В TSX используется `android={{ headerRightWithLargeTitle: true }}`. Большая полоса увеличивается, когда её шрифту нужно больше места. Автономные сборки Android включают нативную раскладку RTL; встраивающий хост должен задать `android:supportsRtl="true"` для своего приложения, чтобы нативные заголовки следовали направлению приложения.

Задайте странице собственный `Header`, чтобы заменить `ui.NavHeader`. На iOS и Android он становится представлением заголовка в системном заголовке вместо `HeaderTitle`, рядом с `HeaderLeft`, `HeaderRight` страницы и кнопкой «Назад»; он заполняет пространство между этими краями, когда его представления растягиваются или занимают процентную ширину, как поле поиска, а в остальных случаях принимает размер своего содержимого. На других платформах он рисуется над содержимым. `ui.NavHeader.Scroll` по-прежнему связывает прокрутку страницы на iOS или Android с размытием её заголовка.

`NoPopOnBack` сохраняет страницу при нажатии «Назад», `OnBackPress` получает это нажатие, а `NoPopOnSwipe` отключает жест смахивания назад в iOS. `cx.OnBackPress(func() bool)` даёт приложению обработчик, срабатывающий раньше стека, — для несохранённых изменений. `OnDismiss` сообщает, что пользователь закрыл страницу, и стек удаляет её.

## Полноэкранный иммерсивный режим

Вызовите `mobile.Immersive(cx, true)`, чтобы запросить полноэкранный режим для окна, и `mobile.Immersive(cx, false)`, чтобы вернуть обычные настройки системных панелей. Для страницы или диалога используйте `StatusBar: core.StatusBarSetting{Immersive: core.ToggleOn}`, чтобы запросить его, пока этот слой показан. В TSX используйте `statusBar={{ immersive: true }}` на странице или в диалоге либо `status_bar: { immersive: true }` в `nsgo.yaml` как значение по умолчанию для окна.

На Android смахивание от края ненадолго показывает строку состояния и панель навигации. На HarmonyOS окно просит систему скрыть строку состояния, трёхкнопочную панель навигации и индикатор навигации. На iOS строка состояния скрывается, а UIKit решает, когда скрывать или показывать индикатор «Домой». `cx.Env().SafeArea()` продолжает сообщать отступы, которые предоставляет система, включая вырезы дисплея. На HarmonyOS с трёхкнопочной навигацией в альбомной ориентации система сообщает область кнопок вдоль нижнего края, хотя рисует их вдоль боковой стороны, поэтому безопасная область может оставлять место внизу, пока кнопки перекрывают боковую сторону.

## Модальные окна

`ui.Dialog` показывает поверхность над затемнённым приложением, а на сенсорных экранах — лист снизу, а `ui.Sheet` — панель снизу. Каждый из них закрывается по Escape и по нажатию на фон, если не указано иное.

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

`ui.AlertDialog` просит пользователя подтвердить действие, а `cx.ShowAlert` показывает собственное системное оповещение или лист действий ([Dialog](ui/dialog.md)).

## Меню

`ui.Menu` открывает `ui.MenuItem` по нажатию кнопки: рядом с ней или в виде листа снизу, если этого требует его `Presentation`, на любом устройстве. `ui.ContextMenu` делает то же самое по вторичному щелчку или долгому нажатию, а `ui.Popover`, `ui.HoverCard` и `ui.Tooltip` показывают другое привязанное содержимое.

```go
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", OnSelect: duplicate},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Вкладки

`ui.Tabs[T]` — это сегментированный элемент управления или список вкладок с индикатором, который перемещается к выбранному варианту; `TabStyle` накладывается на стиль каждой вкладки, его `Selected` — на стиль выбранной вкладки, а `IndicatorStyle` — на стиль индикатора. Вкладки с подчёркиванием могут задать `IndicatorColor` и подогнать линию под содержимое с помощью `IndicatorFit: TabsIndicatorContent`. `ui.Sidebar` содержит разделы и строки для боковой панели настольного приложения, а `ui.TabShell` держит разделы верхнего уровня рядом с панелью вкладок (по умолчанию внизу или вертикально у начального или конечного края с `Edge` / `EdgeFunc`, включая `mobile.TabBarAutomatic`, чтобы следовать `cx.Env().VerticalBarEdge()`) при любой ширине окна. Он использует системную панель, когда у каждой вкладки есть `SystemIcon` или SVG-иконка `Icon`, — у любого края на iOS и у нижнего края на Android, показывая SVG там, где `SystemIcon` не называет SF Symbol на iOS или drawable на Android; в противном случае он рисует панель с SVG-иконками. Задайте `BarHidden`, пока видна открытая поверх страница с подробностями, чтобы скрыть любую из панелей и восстановить безопасную область раздела; `TabBarInset` становится равным 0, пока панель скрыта или размещена вертикально.

`mobile.TabBar` — это системная панель вкладок на iOS и Android с оформлением Liquid Glass или Material; она принимает `Items` из `mobile.TabBarItem{Title, Icon, SVG}`, выбранный индекс, `OnChange` и необязательный `Edge` (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` или `mobile.TabBarAutomatic`, который следует `cx.Env().VerticalBarEdge()` на iOS 27.1 и в остальных случаях возвращается к нижнему краю) либо реактивный `EdgeFunc`. Роли палитры Android `tabBarBackground`, `tabBarIdle`, `tabBarSelected` и `tabBarIndicator` задают фон панели, неактивные иконки и подписи, выбранную иконку и подпись и выделяющую плашку выбранной вкладки. Пользовательские палитры по умолчанию используют соответственно `Surface`, `TextMuted`, `Accent` и `accentContainer`, а если `accentContainer` не задан — плашку с 12% акцентного цвета. Палитра по умолчанию сохраняет фон и неактивные цвета Material, а выбранная вкладка окрашивается в акцентный цвет. На других платформах она не занимает места, поэтому приложение показывает там `ui.Tabs`. `mobile.IsAvailable(cx)` сообщает, показывается ли панель. `Icon` называет иконку платформы; `SVG`, например `lucide.House()`, показывается на iOS и Android там, где `Icon` не называет SF Symbol или drawable, и окрашивается панелью так же, как системная иконка.

Задайте `TabBarItem.Badge` значение `mobile.TabBadge{Text: "3"}` для текста или `mobile.TabBadge{Dot: true}` для точки. Обновляйте его через `ItemsFunc` и возвращайте нулевое значение, чтобы убрать его. `ui.ShellTab.Badge` принимает `func() mobile.TabBadge` как для системной, так и для отрисованной панели. Если заданы оба поля, приоритет у текста. Для точки UIKit показывает пустой нативный значок. Вспомогательные технологии озвучивают текст значка или локализованное слово, обозначающее новое содержимое. TSX принимает `badge: { text: "3" }` или `badge: { dot: true }` у нативного элемента и функцию, возвращающую это значение, у элемента `TabShell`.

```go
mobile.TabBar{
	Items: []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
	Selected: func() int { return tab.Get() },
	OnChange: func(i int) { tab.Set(i) },
}
```

## Далее

- [Формы](forms.md): что содержит страница стека.
- [Анимация](animation.md): как перемещаются страницы и модальные окна.
- [Раскладка](layout.md): безопасная область вокруг заголовка.
