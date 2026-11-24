---
title: Alamat aplikasi
description: Membaca dan memperbarui alamat aplikasi web melalui riwayat peramban.
group: Konsep inti
order: 59
---

`cx.AppURL()` membaca alamat aplikasi sebagai nilai yang dilacak. Di web, URL halaman awal tersedia sebelum root dirender, sehingga muat ulang atau tautan yang dibagikan dapat memilih tampilan yang sama. Aplikasi menentukan bagaimana URL dipetakan ke tampilannya.

```go
func Root(cx *core.Scope) core.Node {
    return core.View{Children: []core.Node{
        core.Text{ContentFunc: cx.AppURL},
        base.Button{Label: "Open film", OnPress: func(core.PressEvent) {
            if err := cx.SetAppURL("/films/42", core.AppURLOptions{
                History: core.HistoryPush,
            }); err != nil {
                // Show the error in the app's usual way.
            }
        }},
    }}
}
```

`SetAppURL` langsung memperbarui alamat yang dilacak tanpa meminta halaman atau menavigasi tumpukan (*stack*). Secara default, ini menggantikan entri peramban saat ini; `HistoryPush` meminta entri baru. Dorongan navigasi atau modal dalam komit yang sama akan menyediakan entri tersebut sebagai gantinya, sehingga satu aksi tidak membuat dua langkah kembali. Beberapa penulisan dalam satu komit menyimpan alamat terakhir dan mempertahankan permintaan push apa pun.

Alamat relatif diselesaikan terhadap alamat saat ini, atau HTML base eksplisit. Alamat peramban harus tetap berada di origin halaman. Alamat yang tidak valid, spasi kosong, karakter kontrol, garis miring terbalik (*backslash*), dan alamat di atas 16 KiB mengembalikan galat tanpa mengubahnya.

`cx.OnAppURLChange(func(change core.AppURLChange) { ... })` mengamati penelusuran riwayat peramban dan suntingan hash. `change.Reason` bernilai `"back"`, `"forward"`, atau `"address"`; fungsi yang dikembalikan berhenti berlangganan, dan pembuangan (*disposal*) cakupan menghapus pendengar tersebut. Baca `AppURL` untuk alamat awal. Penulisan dari aplikasi sendiri memperbarui ikatan (*binding*), tetapi tidak mengirim notifikasi perubahan. Penelusuran yang tetap mempertahankan alamat yang sama tidak mengirim perubahan apa pun.

Tombol kembali peramban tetap menutup modal atau mengeluarkan layar navigasi. Aksi kembali yang ditolak aplikasi akan memulihkan entri yang ada beserta alamatnya. Penangan penutupan (*dismissal handler*) dapat menetapkan alamat penggantinya sendiri. Pop terprogram memperbarui alamat setelah penelusuran riwayat asinkron selesai, dan penulisan yang datang selama penelusuran tersebut akan menunggunya. Kembali melewati entri pemutar (*player*) ini akan keluar dari aplikasi, termasuk entri yang tersimpan dari pemuatan sebelumnya.

Untuk jalur seperti `/films/42`, tetapkan direktori aplikasi yang di-deploy dan atur server agar menyajikan `index.html` untuk rute-rute aplikasi:

```yaml
web:
  base: /        # /my-app/ when deployed below that directory
```

`web.base` menulis HTML base agar skrip, gaya, Wasm, dan sumber daya tetap termuat ketika alamat dalam (*deep address*) dibuka secara langsung. Tanpanya, jalur aset relatif yang ada tetap dipertahankan; alamat query atau hash dapat menggunakannya tanpa perubahan. Aturan fallback server harus membiarkan permintaan aset asli apa adanya. Content Security Policy yang ketat harus mengizinkan base same-origin ini (`base-uri 'self'`).

Aplikasi yang disematkan di dalam halaman lain menyimpan alamatnya secara internal dan membiarkan riwayat halaman induk tidak berubah. Iframe same-origin dapat memilih ikut secara eksplisit dengan `data-nsgo-history`. Subpohon yang dibatasi (*confined subtrees*) tidak dapat membaca maupun mengubah alamat aplikasi.

Pada target lain, `AppURL` menyimpan tautan terakhir yang dibuka bersama aplikasi atau nilai yang disimpan `SetAppURL`; mengaturnya tidak mengubah jendela native maupun peramban sistem. Program mini juga menyimpan nilai ini tanpa mengubah rute host-nya.

TSX menggunakan status yang sama:

```tsx
import { UrlScheme } from "nsgo";

UrlScheme.url();
UrlScheme.setUrl("/films/42", { history: "push" });
const stop = UrlScheme.on("change", ({ url, reason }) => {
  // Update the app's route from the browser's choice.
});
```
