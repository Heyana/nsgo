---
title: Menerbitkan di web
description: Terbitkan aplikasi NSGo dengan semantik dokumen, metadata halaman, dan aset dari CDN.
group: Perangkat bantu
order: 24
---

## Membangun aplikasi

```sh
go tool nsgo build -target web
```

Sajikan direktori yang dihasilkan melalui HTTP. Aset rilis memiliki hash konten di namanya, jadi simpan di cache sebagai immutable; lakukan validasi ulang untuk `index.html`, `nsgo.json`, dan `manifest.webmanifest`. Aplikasi web mempertahankan perilaku tata letak, pengguliran, fokus, dan pointer milik NSGo.

## Judul dan tautan

`core.View` atau `core.Text` dengan `Role: core.RoleHeader` menjadi `h1` hingga `h6`, dipilih oleh `AccessibilityLevel`; tanpa level ia menjadi `h2`. Judul Markdown dari `base/textview` membawa levelnya. Margin dan font judul milik browser tidak mengubah gaya aplikasi.

`base.Link` dan `ui.Link` menerbitkan `Href`-nya sebagai anchor. Klik biasa menjalankan `OnOpen` dan `OnPress` milik aplikasi; klik tengah atau klik dengan tombol pengubah menggunakan aksi tautan milik browser. Browser menyediakan menu konteks tautannya kecuali aplikasi menangani `OnContextMenu`. Tautan yang dinonaktifkan tidak memiliki alamat aktif. Untuk view kustom dengan peran tautan, atur `core.View.Web` ke `core.WebViewProps{Href: "/notes"}` atau ikat `HrefFunc`. TSX menggunakan `web={{href: "/notes"}}`; judul menggunakan `accessibilityLevel={1}`. Skema URL yang dapat dieksekusi dan tidak aman ditolak. Konten yang dibatasi menjaga navigasi di dalam handler tekanan yang tercakup dan tidak mengumumkan alamat browser.

## Metadata halaman

Impor `github.com/nsgo-dev/nsgo/web` dan tempatkan `web.Head` di samping konten halaman:

```go
core.Group{
	web.Head{Title: "Notes", Description: "Your saved notes"},
	pageContent,
}
```

`TitleFunc` dan `DescriptionFunc` mengikuti signal. Head yang terlihat kemudian menimpa field yang diaturnya dan mewarisi yang lain. Menyembunyikan atau menghapus sebuah halaman memulihkan head sebelumnya; menghapus semua head memulihkan dokumen aslinya. Field statis yang kosong akan mewarisi; fungsi yang mengembalikan string kosong akan mengosongkannya. Head tidak menambahkan node tata letak. Target native, aplikasi tertanam, dan konten yang dibatasi membiarkan dokumen host-nya apa adanya.

Di TSX, impor `Head` dari `nsgo/web` dan tulis `<Head title={title()} description="Your saved notes" />`. Untuk pratinjau HTML statis, pra-render setiap URL sebagai bagian dari deployment situs; `Head` memperbarui dokumen yang sedang berjalan.

## Menyajikan aset dari CDN

Loader menyelesaikan semua file aplikasi relatif terhadap `nsgo.json`. Sebuah halaman dapat memuat aplikasi dari origin lain:

```js
const url = new URL("https://cdn.example.com/notes/nsgo.json");
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url).href);
await mount(null, url.href, { config });
```

Izinkan origin halaman mengambil file CDN dengan CORS, dan sajikan WebAssembly sebagai `application/wasm`. Sajikan HTML entri aplikasi untuk setiap rute dalam yang digunakan aplikasi Anda. [Penyematan](embedding.md#web) menunjukkan cara memasangnya ke dalam sebuah elemen sebagai gantinya.
