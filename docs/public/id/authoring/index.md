---
title: Menulis komponen
description: "Membuat komponen yang dapat dipasang orang lain di aplikasi mereka, seperti cara base dan ui milik NSGo dibuat."
group: Menulis komponen
order: 70
---

Halaman-halaman ini ditujukan bagi pembuat komponen: pustaka kontrol, sistem desain, atau satu tampilan kompleks yang diimpor aplikasi sebagai paket Go. Halaman ini mengasumsikan pemahaman terhadap [Komponen dan status](../components.md) serta [Gaya dan tema](../styles.md).

## Sebuah komponen

Komponen adalah sebuah tipe dengan metode `Render`, seperti di dalam aplikasi. Field-fieldnya adalah hal yang diatur oleh aplikasi; apa pun yang tidak diatur oleh aplikasi akan mengambil nilai nol (*zero value*), sehingga nilai nol dari setiap field sebaiknya menjadi kasus yang paling umum.

```go
package chips

import "github.com/nsgo-dev/nsgo/core"

// Chip is a short label in a pill.
type Chip struct {
	Text string
}

// Render implements core.Node.
func (c Chip) Render(*core.Scope) core.Node {
	return core.View{Class: "self-start rounded-full px-3 py-1", Children: []core.Node{
		core.Text{Content: c.Text, Class: "text-sm"},
	}}
}
```

Aplikasi menggunakannya seperti elemen lainnya: `chips.Chip{Text: "New"}`.

## Perilaku dan tampilan

NSGo menyimpan setiap kontrol dalam dua bentuk. `base` menyimpan perilaku, status, dan aksesibilitasnya tanpa tampilan, dan menerima setiap gaya dari aplikasi bagian demi bagian; `ui` memasang tampilan pada bentuk `base`, sehingga perilaku tetap berada di satu tempat. Pustaka dapat mengikuti pemisahan yang sama:

- Komponen **tanpa gaya** (*unstyled*) memegang penekanan, papan ketik, fokus, dan apa yang didengar pembaca layar, serta mengekspos gaya setiap bagian, seperti `base.Switch`. Aplikasi dan pustaka lain memberinya tampilan masing-masing ([Bagian dan status](parts.md)).
- Komponen **bergaya** (*styled*) membungkus komponen tanpa gaya dan hanya memilih gaya dari tema yang berlaku, seperti `ui.Switch` ([Tampilan dan tema](looks.md)).

Komponen tanpa perilaku yang perlu dibagikan, seperti lencana (*badge*), dapat dibuat bergaya secara langsung.

## Sebuah paket

- Simpan satu komponen per file, dinamai sesuai komponennya (`chip.go`), bersama tipe-tipe yang hanya digunakannya; apa yang dipakai bersama beberapa komponen mendapat filenya sendiri.
- Beri paket tersendiri untuk komponen yang kompleks, dengan API yang menghilangkan nama komponennya, seperti `dock.Area` pada `base/dock`.
- Letakkan kata-kata yang diucapkan komponen di dalam `locales/` milik paket ([Kata, pengujian, dan publikasi](shipping.md)).
- Jangan melakukan apa pun secara implisit berdasarkan perangkat: ukuran atau bentuk yang berubah antara ponsel dan desktop harus menjadi field yang diatur oleh aplikasi.

## Berikutnya

- [Bagian dan status](parts.md): nilai terkontrol, bagian yang diberi gaya, dan aksesibilitas.
- [Tampilan dan tema](looks.md): warna dari tema, varian, dan ukuran.
- [Komponen tanpa gaya](base.md): kontrol milik NSGo sendiri tanpa tampilan, untuk diberi gaya atau dijadikan dasar.
- [Kata, pengujian, dan publikasi](shipping.md): terjemahan, pengujian, dan TSX.
