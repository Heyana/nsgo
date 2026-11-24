---
title: Slider
description: Satu nilai, atau sebuah rentang, di sepanjang trek dengan papan ketik dan pointer.
group: Komponen
order: 37
---

`ui.Slider` memilih sebuah nilai di sepanjang trek, dengan seretan, langkah, dan papan ketik. Satu nilai berarti satu kenop; `High` membuat rentang dengan dua kenop.

```go preview=slider/basic
volume := core.NewSignal(cx, 40.0)
ui.Slider{Value: volume.Get, OnChange: volume.Set, Min: 0, Max: 100, Step: 5, Label: "Volume"}
```

## Sebuah rentang

Atur `High` dan `OnHighChange` untuk memilih dua nilai. `Value` adalah ujung bawahnya. `LowLabel` dan `HighLabel` menamai kenop-kenop itu untuk teknologi bantu.

```go preview=slider/range
price := core.NewSignal(cx, 20.0)
ceiling := core.NewSignal(cx, 80.0)
ui.Slider{
	Value: price.Get, OnChange: price.Set,
	High: ceiling.Get, OnHighChange: ceiling.Set,
	Min: 0, Max: 100, Step: 1,
	Label: "Price", LowLabel: "Minimum", HighLabel: "Maximum",
}
```

## Langkah dan nilai

`Step` menempelkan nilai ke kelipatan langkah; langkah 0 menerima nilai apa pun. `ValueText` mengubah apa yang dibaca teknologi bantu sebagai nilainya, dan `Disabled` menonaktifkannya.

```go preview=slider/format
ui.Slider{Value: speed.Get, OnChange: speed.Set, Min: 0.5, Max: 2, Step: 0.25, Label: "Speed",
	ValueText: func(v float64) string { return fmt.Sprintf("%g×", v) }}
```

Gunakan `MinFunc`, `MaxFunc`, dan `StepFunc` ketika rentang atau langkah berubah, misalnya saat sebuah video melaporkan durasinya. Fungsi-fungsi ini menggantikan field statisnya dan memperbarui slider yang sudah terpasang, dengan tetap mempertahankan fokusnya dan seretan yang sedang berlangsung. `MaxFunc` eksplisit yang mengembalikan 0 mengizinkan rentang yang menyusut hingga durasinya tiba; nilai milik aplikasi tetap dipertahankan dan ditampilkan di dalam batas saat ini.

```go
ui.Slider{Value: position.Get, OnChange: position.Set, MaxFunc: duration.Get, Label: "Position"}
```

Di TSX, `min={minimum()}`, `max={duration()}`, dan `step={step()}` mengikuti signal-nya di tempat; `max={0}` menyusutkan rentang, sebagaimana `MaxFunc` yang mengembalikan 0.

## Arah

`Axis: base.Vertical` menegakkan slider, dengan nilai minimumnya di bawah; beri tinggi di `Style`. `Inverted` menempatkan nilai minimum di ujung yang lain: di atas pada slider vertikal, di akhir pada slider horizontal. Tombol panah di sepanjang trek mengikuti arahnya. `ui.Progress` menerima `Axis` dan `Inverted` yang sama.

```go
ui.Slider{Value: level.Get, OnChange: level.Set, Axis: base.Vertical, Label: "Level", Style: core.Style{Height: core.Pt(160)}}
```

## Perilaku

Seretan mengikuti pointer dan melaporkan `OnChange` selama bergerak. Tombol panah menggeser kenop yang sedang difokus sebesar satu langkah, Home dan End menuju ke ujung-ujungnya, dan Page Up serta Page Down menggeser lebih jauh. `ui.Slider` menandai langkah-langkah di bawah trek ketika `Step` lebih dari 0, hingga 60 tanda.

## Aksesibilitas

Slider melaporkan `RoleAdjustable`, `Label`-nya, dan nilainya; sebuah rentang memiliki kedua kenop dengan labelnya masing-masing. `ValueText` dibaca sebagai pengganti angka. Papan ketik serta aksi menaikkan dan menurunkan milik pembaca layar mengubah nilainya.

## Platform

Trek digambar oleh aplikasi dan berukuran sama di mana pun. Tekanan, fokus, dan pembaca layar berasal dari platform; seretan sentuh berperilaku seperti seretan pointer dengan langkah yang sama.

## Terkait

- [Formulir](../forms.md): slider di dalam formulir.
- [Aksesibilitas](../accessibility.md): nilai dan peran adjustable.
- Halaman Slider, RangeSlider, dan Rating di galeri.
- [Komponen tanpa gaya](../authoring/base.md#slider): `base.Slider`, vertikal dan dengan tanda buatan Anda sendiri.
