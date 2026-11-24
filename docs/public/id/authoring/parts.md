---
title: Bagian dan status
description: "Tidak menyimpan status milik aplikasi, memberi gaya setiap bagian, dan memberi tahu teknologi asistif apa komponen tersebut."
group: Menulis komponen
order: 71
---

Komponen tanpa gaya menentukan cara sebuah kontrol berperilaku dan menyerahkan tampilannya kepada siapa pun yang menggunakannya. Halaman ini membangun `Toggle`, sebuah pil yang berada dalam kondisi nyala atau mati, dengan cara yang sama seperti `base` membangun kontrol-kontrolnya.

## Nilai terkontrol

Sebuah kontrol tidak menyimpan nilai yang dimiliki oleh aplikasi: aplikasi meneruskannya sebagai fungsi dan mengaturnya melalui pemanggilan balik (*callback*). Komponen membacanya di tempat nilai tersebut ditampilkan, sehingga hanya bagian itu yang diperbarui saat nilainya berubah.

```go
// Toggle is a pill that is on or off. Its parts are the pill and a dot
// inside it, then its children, the label.
type Toggle struct {
	On       func() bool
	Disabled func() bool
	// OnChange receives the value a press asks for, the opposite of On's.
	OnChange func(on bool)

	Class        string
	Style        core.Style
	StyleFunc    func() core.Style
	DotClass     string
	DotStyle     core.Style
	DotStyleFunc func() core.Style
	Children     []core.Node
	Label        string
}
```

Aplikasi menyimpan nilainya di dalam sebuah sinyal: `chips.Toggle{On: bold.Get, OnChange: bold.Set, Label: "Bold"}`.

## Bagian

Setiap bagian yang digambar oleh komponen menerima `Class`, `Style`, dan `StyleFunc`, yang dinamai sesuai bagiannya (`DotClass`, `DotStyle`, `DotStyleFunc`). `Style` bersifat tetap, `StyleFunc` mengikuti sinyal yang dibacanya, dan kelas berada di bawah keduanya, sehingga aplikasi dapat memakai cara mana pun yang ditulisnya.

Suatu bagian mengambil status kontrol melalui `PartStates` alih-alih melaporkan statusnya sendiri, sehingga kelas `checked:` dan gaya `Checked` berlaku padanya sementara teknologi asistif melihatnya sebagai satu kontrol tunggal:

```go
func (t Toggle) Render(*core.Scope) core.Node {
	on := func() bool { return t.On != nil && t.On() }
	off := func() bool { return t.Disabled != nil && t.Disabled() }
	shows := func() core.PartStates { return core.PartStates{Checked: on(), Disabled: off()} }
	dot := core.View{Class: t.DotClass, Style: t.DotStyle, StyleFunc: t.DotStyleFunc, PartStates: shows}
	return core.View{
		Class: t.Class, Style: t.Style, StyleFunc: t.StyleFunc,
		Role: core.RoleSwitch, Label: t.Label, Checked: on, Disabled: t.Disabled,
		OnPress: func(core.PressEvent) {
			if t.OnChange != nil {
				t.OnChange(!on())
			}
		},
		Children: append([]core.Node{dot}, t.Children...),
	}
}
```

Aplikasi dapat menatanya dengan kelas saja:

```go
chips.Toggle{
	On: bold.Get, OnChange: bold.Set, Label: "Bold",
	Class:    "flex-row items-center gap-2 rounded-full border border-zinc-300 px-3 py-1 checked:bg-blue-50",
	DotClass: "size-2 rounded-full bg-zinc-300 checked:bg-blue-500",
	Children: []core.Node{core.Text{Content: "Bold"}},
}
```

`PartStates.Invalid` juga menerapkan kelas `aria-invalid:` dan `Style.AriaInvalid` ke bagian dekoratif, termasuk gaya `group-aria-invalid:` di dalamnya, tanpa melaporkan kontrol tidak valid lain. Pasang `Invalid` pada kontrol sebenarnya atau input asli agar teknologi asistif menerima status validasinya.

## Aksesibilitas

Akar (*root*) sebuah kontrol menyatakan identitasnya: `Role`, `Label` jika teksnya belum menjelaskannya, `Hint` untuk apa yang dilakukan saat diaktifkan, serta `Checked`, `Selected`, `Disabled`, `Invalid`, atau `Expanded` sesuai kebutuhan. Bagian-bagian di dalamnya tetap diam. Kontrol yang berpindah antar-item dengan tombol panah adalah satu hentian Tab, seperti halnya daftar tab. [Aksesibilitas](../accessibility.md) mencantumkan daftar peran dan aksi.

## Apa yang perlu diteruskan

Komponen yang akarnya merupakan tampilan milik sendiri mengambil `OnLayout`, `Tooltip`, dan `TestID` dari tampilan tersebut dan meneruskannya, sehingga aplikasi dapat mengukur, menjelaskan, dan mengujinya seperti tampilan lainnya. Pemanggilan balik (*callback*) bersifat opsional: `OnChange` yang bernilai `nil` membuat kontrol menampilkan nilai tanpa mengubahnya.
