---
title: Button
description: Tombol tekan, varian dan ukurannya, serta tombol ikon.
group: Komponen
order: 31
---

`ui.Button` adalah tombol tekan dengan tampilan `ui`, dengan varian dan ukuran yang dibutuhkan sebuah jendela. `OnPress` berjalan saat sebuah tekanan selesai.

```go preview=button/basic
ui.Button{Text: "Save", OnPress: save}
ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: cancel}
```

## Varian dan ukuran

`ui.Button.Variant` bernilai `ButtonPrimary` (bawaan), `ButtonSecondary`, `ButtonOutline`, `ButtonGhost`, `ButtonTinted`, atau `ButtonDanger`. Tinted dan danger menggunakan warna aksen atau warna bahaya milik tema di atas isian lembut. Pada idiom desktop dan TV, `Size` bernilai `SizeDefault` (28 poin), `SizeSmall` (24), atau `SizeLarge` (32). Pada idiom sentuh, ukuran yang sama menjadi 44, 40, dan 48 poin, sehingga tombol dapat digunakan tanpa perlu diberi gaya ulang.

```go preview=button/variants
ui.Button{Text: "Delete", Variant: ui.ButtonOutline, OnPress: remove}
ui.Button{Text: "Continue", Size: ui.SizeLarge, OnPress: next}
ui.Button{Text: "Save", Icon: lucide.Check, Flex: 1, OnPress: save}
```

`Color` mengisi tombol dengan warna pilihan aplikasi alih-alih warna varian, dan `Tint` mewarnai ikonnya. `Flex` memberinya bagian dari sebuah baris, sedangkan `Style` mengatur tata letaknya: ia dapat membuat tombol mengisi kontainernya, mengatur ukuran pilihan aplikasi, menambahkan margin, atau membulatkan sudutnya; `TestID` menamainya untuk pengujian.

```go
ui.Button{Text: "Continue", Size: ui.SizeLarge,
    Style: core.Style{Width: core.Pct(100), Height: core.Pt(50)}, OnPress: next}
```

Tombol yang hanya berisi ikon adalah `ui.IconButton`, dengan `Variant` `IconButtonGhost`, `IconButtonRaised`, atau `IconButtonFilled`, sebuah lingkaran datar dalam warna inset milik tema atau `Fill`, `Size` dalam poin, dan `Label` untuk teknologi bantu. Di desktop, label juga tampil sebagai tooltip saat pointer berhenti di atas tombol; `Tooltip` menampilkan teks lain di sana, dan yang mengembalikan `""` tidak menampilkan apa pun. `ui.Button` dengan `Icon` tanpa `Text` juga berbentuk persegi, dan membutuhkan `Label`.

```go preview=button/icon
ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost, OnPress: remove}
```

## Konten dan state

`ui.Button` menampilkan `Text`, `Icon` opsional di depan, atau keduanya. Gunakan `TextStyle` untuk menyesuaikan ukuran font, ketebalan, atau gaya teks lain pada label.

```go
ui.Button{Text: "Saving…", Disabled: saving.Get, OnPress: save}
```

`Disabled` menerima sebuah fungsi, sehingga tombol mengikuti sebuah signal: tombol yang dinonaktifkan tidak menerima tekanan maupun fokus keyboard, dan tampilannya adalah state nonaktif, bukan warna yang dipilih aplikasi.

`Busy` digunakan untuk aksi yang masih berjalan. Selama ia mengembalikan true, spinner dalam warna teks tombol tampil di atas konten, sehingga tetap terlihat di atas aksen tombol primary, tekanan tidak melakukan apa pun, dan teknologi bantu mendengar bahwa tombol sedang sibuk. Secara bawaan, konten tetap mempertahankan ruangnya di bawah spinner. Tombol tetap mempertahankan fokus keyboard. `BusyLabel` atau `BusyLabelFunc` menampilkan pesan di samping spinner dan melaporkannya sebagai nama aksesibel tombol selama sibuk. Tombol mempertahankan setidaknya lebar saat diamnya.

```go preview=button/busy
ui.Button{Text: "Save", Busy: saving.Get, BusyLabel: "Saving…", OnPress: save}
```

`ui.Spinner` dan `ui.ProgressRing` menerima `Tint` sendiri untuk permukaan lain mana pun tempat aksennya akan tidak terlihat.

## Tautan

`ui.Link{Text: "Docs", Href: "/docs"}` membuka sebuah tujuan dengan peran tautan. Teks dan garis bawah saat hover menggunakan aksen kecuali `Style.Color` mengatur warna lain; `Style.Hover.Color` mengatur warna saat hover.

## Aksesibilitas

Tombol melaporkan `RoleButton` dan teksnya sebagai namanya. `Label` menggantikan teks untuk teknologi bantu. Tombol yang hanya berisi ikon tanpa `Label` tidak dapat dibaca, jadi beri nama.

Return dan Space menekan tombol yang sedang difokus di desktop, web, dan Apple TV; pembaca layar ponsel menekannya sebagai klik.

## Platform

Tombol adalah view yang digambar dengan cara yang sama di mana pun, bukan tombol platform; tekanan, fokus, dan aksesibilitas berasal dari platform. Idiom sentuh menggunakan target tata letak yang lebih besar seperti dijelaskan di atas.

## Terkait

- [Formulir](../forms.md): tombol di dalam formulir dan validasi.
- [Gaya dan tema](../styles.md): tema tempat tombol mengambil warnanya.
- [Aksesibilitas](../accessibility.md): peran, label, dan fokus.
- Halaman Button di galeri.
- [Komponen tanpa gaya](../authoring/base.md#button): `base.Button`, untuk tombol dengan tampilan Anda sendiri.
