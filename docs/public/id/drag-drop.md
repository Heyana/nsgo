---
title: Seret dan lepas
description: Pindahkan nilai aplikasi antar view dalam satu jendela.
group: Konsep inti
order: 57
---

`base/dragdrop` menghubungkan sumber dan target di dalam `Area` yang sama, dalam satu jendela. Aplikasi tetap mempertahankan view, gaya, seleksi, dan aksi yang menangani nilai tersebut. Sumber mulai menyeret setelah empat poin perpindahan secara bawaan, sehingga penekanan biasa tetap utuh.

```go
import "github.com/nsgo-dev/nsgo/base/dragdrop"

dragdrop.Area{Children: []core.Node{
	dragdrop.Source{
		View: core.View{
			Label: "Film", TestID: "poster",
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Poster"}},
		},
		Data: func() any { return filmID },
		Preview: func(any) core.Node { return core.Text{Content: "Film"} },
	},
	dragdrop.Target{
		View: core.View{
			TestID: "playlist",
			Style: core.Style{Width: core.Pt(160), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Playlist"}},
		},
		Accept: func(value any) bool { _, ok := value.(string); return ok },
		OnDrop: func(e dragdrop.Event) { addToPlaylist(e.Data.(string)) },
	},
}}
```

`Data` berjalan sekali saat seretan dimulai. Kembalikan slice untuk membawa beberapa item terpilih sekaligus; salin data yang dapat berubah di sana jika harus tetap berupa snapshot. `Data` bernilai nil menonaktifkan penyeretan. `Threshold` mengatur perpindahan yang dibutuhkan. `Activation: dragdrop.AfterLongPress` pertama-tama menunggu `View.DelayLongPress` (500 ms secara bawaan), lalu perpindahan; pilih opsi ini secara eksplisit di tempat usapan harus menggulir terlebih dahulu.

`View` mempertahankan props biasanya, termasuk handler tekan dan aksesibilitas. Sumber memiliki `View.Responder`; biarkan kosong. Anak-anaknya dapat menyertakan tombol dengan aksinya yang normal. `Preview` mengikuti penunjuk tanpa menerima input atau memasang ulang sumber; tanpanya, label sumber yang ditampilkan. Berikan label atau pratinjau pada sumber. Area bersarang beroperasi secara independen.

Target membutuhkan `OnDrop` untuk menerima nilai. Target terdalam mendapat kesempatan pertama; ketika `Accept` menolaknya, leluhur yang menerima dapat mengambilnya. `OnEnter`, `OnMove`, dan `OnLeave` dapat memperbarui tampilan target. Setiap event membawa `Data`, `Position` di target, dan `PagePosition` di permukaan. Menonaktifkan atau menghapus target aktif akan membersihkan status masuknya; pelepasan memeriksa penerimaan lagi. Target di belakang view lain, terpotong, dinonaktifkan, atau di bawah modal tidak dapat menerima pelepasan.

`OnStart` dan `OnEnd` mendeskripsikan sesi sumber. `OnEnd` menerima `Result{Data, Dropped, Target}`. Escape, pembatalan sentuhan native, menonaktifkan atau menghapus sumber, atau melepas di luar target yang menerima akan berakhir tanpa pelepasan. Penempatan target menggunakan geometri yang terakhir di-commit; animasi native di antara commit tidak diambil sampelnya. Mekanisme ini memindahkan nilai di dalam jendela; file sistem tetap menggunakan `OnFileDrop`.

Pertahankan menu, perintah keyboard, atau aksi aksesibel yang melakukan operasi yang sama. Penyeretan adalah pintasan menuju aksi tersebut. Pengujian menggunakan `app.DragTo("id:poster", "id:playlist")`, atau `DragTo` milik permukaan terpilih; pengujian TSX menggunakan `await app.dragTo("id:poster", "id:playlist")`. Kedua view harus sudah muat di jendela yang terlihat; langkah ini mengirim urutan sentuhan melalui responder tanpa menyintesis pelepasan atau menggulir selama penyeretan. Lihat [Pengujian](testing.md).
