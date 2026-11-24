---
title: Menu
description: Menu dari pemicu, menu konteks, dan item-itemnya.
group: Komponen
order: 42
---

`ui.Menu` membuka `ui.MenuItem` dari sebuah tombol: di sampingnya, atau sebagai sheet dari bawah dengan `Presentation: base.PresentSheet`, apa pun perangkatnya. `ui.ContextMenu` membuka item yang sama di posisi penunjuk.

```go preview=menu/basic height=280
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", Icon: lucide.Pencil, OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", Shortcut: "mod+d", OnSelect: duplicate},
	ui.MenuSeparator{},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

Menu membuka dan menutup dirinya sendiri; `Open` dan `OnOpenChange` memungkinkan aplikasi memegang statusnya. `Trigger` adalah tampilan dan konten tombol, dan menu menerima tekanannya. Popup tidak pernah melampaui ruang di samping pemicunya: popup terbuka di sisi tempat ia muat seluruhnya, dan item-itemnya menggulir melewati ruang tersebut, atau melewati `MaxHeight`.

## Item

`ui.MenuItem` menampilkan `Text`, `Icon` opsional, dan `Shortcut` (`"mod+c"`, ditulis dengan cara platform di dalam popup), dan melaporkan `OnSelect`, menutup menu kecuali `NoCloseOnSelect`. `Disabled` menonaktifkannya, `Checked` menampilkan tanda centang di sampingnya, dan `Variant: ui.MenuItemDanger` menandai item yang destruktif. `ui.MenuLabel` mengepalai sebuah grup dan `ui.MenuSeparator` memisahkan grup-grup. `ui.Submenu` menampung item lain dan terbuka di samping item tersebut atau, di dalam sheet, menggantikan item-item di sekitarnya.

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

## Menu konteks

`ui.ContextMenu` membuka item-itemnya saat klik sekunder, atau tekan lama di layar sentuh, di posisi penunjuk di atas `Trigger`-nya.

```go preview=menu/context height=220
ui.ContextMenu{Trigger: fileRow, Children: []core.Node{
	ui.MenuItem{Text: "Open", OnSelect: open},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Perilaku

Escape, kembali, dan tekanan di luar menutup menu. Menu adalah popup yang ditempatkan oleh `Placement` dan `Align`, atau sheet dari bawah ketika disajikan demikian. Di desktop, Down, Up, Home, End, Page Down, dan Page Up bergerak melalui item yang aktif, dimulai dari item yang sedang berada di bawah penunjuk, Right membuka submenu dan Left menutupnya, dan Return memilih. Menu yang dibuka dengan papan ketik memfokuskan item pertamanya, dan fokus kembali ke pemicu saat menu tertutup.

## Aksesibilitas

Menu melaporkan `RoleMenu` dan setiap item `RoleMenuItem`. `Label` menamai pemicu ketika kontennya tidak menamainya. Pembaca layar membacakan teks item dan apakah item tersebut dinonaktifkan.

## Platform

Pemicu dan item adalah milik aplikasi; sheet, popup, fokus, Escape, dan kembali adalah milik platform. Aplikasi memilih popup atau sheet; NSGo tidak memilih keduanya berdasarkan perangkat.

## Terkait

- [Select](select.md): sebuah nilai dari daftar.
- [Dialog](dialog.md): permukaan modal.
- [Tabs](tabs.md): bagian-bagian yang ditampilkan sekaligus.
- Halaman Menu, ContextMenu, dan Command di galeri.
- [Komponen tanpa gaya](../authoring/base.md#menu): `base.Menu`, untuk menu dengan tampilan Anda sendiri.
