---
title: Menü
description: Bir tetikleyiciden açılan menü, bağlam menüsü ve bunların öğeleri.
group: Bileşenler
order: 42
---

`ui.Menu`, `ui.MenuItem` öğelerini bir düğmeden açar: cihaz ne olursa olsun düğmenin yanında veya `Presentation: base.PresentSheet` ile alttan açılan bir sayfa olarak. `ui.ContextMenu` aynı öğeleri işaretçinin bulunduğu yerde açar.

```go preview=menu/basic height=280
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", Icon: lucide.Pencil, OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", Shortcut: "mod+d", OnSelect: duplicate},
	ui.MenuSeparator{},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

Menü kendi kendine açılır ve kapanır; `Open` ve `OnOpenChange` uygulamanın onu tutmasını sağlar. `Trigger` düğmenin görünümü ve içeriğidir, basışlarını ise menü alır. Bir açılır pencere hiçbir zaman tetikleyicisinin yanındaki alanın dışına taşmaz: tamamen sığdığı tarafta açılır ve öğeleri bu alanı veya `MaxHeight`'ı aştığında kaydırılır.

## Öğeler

Bir `ui.MenuItem`, `Text`'i, isteğe bağlı bir `Icon`'u ve bir `Shortcut`'ı (`"mod+c"`, açılır pencerede platformun yazım biçimiyle gösterilir) gösterir ve `OnSelect`'i bildirir; `NoCloseOnSelect` olmadıkça menüyü kapatır. `Disabled` onu kapatır, `Checked` yanında bir onay işareti gösterir ve `Variant: ui.MenuItemDanger` yıkıcı bir öğeyi işaretler. `ui.MenuLabel` bir grubun başında yer alır, `ui.MenuSeparator` ise grupları ayırır. Bir `ui.Submenu` daha fazla öğe tutar ve öğenin yanında ya da bir sayfada çevresindeki öğelerin yerine açılır.

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

## Bağlam menüleri

`ui.ContextMenu`, öğelerini `Trigger`'ı üzerinde ikincil bir tıklamayla veya dokunmatik ekranlarda uzun basışla, işaretçinin bulunduğu yerde açar.

```go preview=menu/context height=220
ui.ContextMenu{Trigger: fileRow, Children: []core.Node{
	ui.MenuItem{Text: "Open", OnSelect: open},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Davranış

Escape, geri tuşu ve dışarıya bir basış menüyü kapatır. Menü, `Placement` ve `Align` ile yerleştirilen bir açılır penceredir ya da öyle sunulduğunda alttan açılan bir sayfadır. Masaüstünde Down, Up, Home, End, Page Down ve Page Up, işaretçinin üzerinde bulunduğu öğeden başlayarak etkin öğeler arasında gezinir; Right bir alt menüyü açar, Left onu kapatır ve Return seçer. Klavyeyle açılan bir menü ilk öğesini odaklar ve menü kapanırken odak tetikleyiciye geri döner.

## Erişilebilirlik

Menü `RoleMenu` rolünü, her öğe ise `RoleMenuItem` rolünü bildirir. İçeriği tetikleyiciyi adlandırmadığında `Label` onu adlandırır. Bir ekran okuyucu bir öğenin metnini ve devre dışı olup olmadığını okur.

## Platform

Tetikleyici ve öğeler uygulamanındır; sayfa, açılır pencere, odak, Escape ve geri tuşu ise platformundur. Açılır pencereyi veya sayfayı uygulama seçer; NSGo cihaza göre hiçbirini seçmez.

## İlgili

- [Seçim kutusu](select.md): bir listeden bir değer.
- [İletişim kutusu](dialog.md): kalıcı (modal) bir yüzey.
- [Sekmeler](tabs.md): aynı anda gösterilen bölümler.
- Galerinin Menu, ContextMenu ve Command sayfaları.
- [Stilsiz bileşenler](../authoring/base.md#menu): kendi görünümünüze sahip bir menü için `base.Menu`.
