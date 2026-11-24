---
title: Input
description: Kolom teks milik platform, terkontrol atau tidak, dengan fokus dan papan ketik perangkat lunak.
group: Komponen
order: 33
---

`ui.Input` adalah kolom teks milik platform dengan tampilan `ui`, satu baris kecuali `Multiline`. Komponen ini memiliki field `base.Input`, sebagaimana setiap kolom bergaya; `ui.Field` adalah kolom yang sama tanpa ukuran sendiri, dalam tepi yang tidak mengikuti penunjuk maupun fokus; `core.Transparent` di `Background` dan `core.Zero` di `BorderWidth` menghilangkan kotaknya, untuk teks yang terbaca sebagai teks biasa tetapi dapat diedit.

```go preview=input/basic
name := core.NewSignal(cx, "")
ui.Input{
	Value:       name.Get,
	OnChange:    name.Set,
	Placeholder: "Your name",
	TestID:      "name",
}
```

## Terkontrol dan tidak terkontrol

Dengan `Value`, kolom bersifat terkontrol: aplikasi menyimpan nilainya dan memperbaruinya di `OnChange`. Dengan `DefaultValue`, kolom menyimpan nilainya sendiri. Kolom terkontrol dapat dikosongkan dengan mengatur nilainya ke `""`.

```go preview=input/submit
draft := core.NewSignal(cx, "")
ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "New task",
	OnSubmit: func(string) { add(); draft.Set("") }}
```

## Field

`Multiline` membungkus teks, dimulai setinggi tiga baris dan bertambah tinggi mengikuti teksnya kecuali gayanya mengatur `Height` atau `MinHeight`. `Masked` menyembunyikan karakter, `Readonly` menghentikan pengeditan sementara teks tetap dapat dipilih dan disalin, `Disabled` menonaktifkan kolom, dan `MaxLength` membatasi jumlah unit UTF-16 yang dimasukkan pengguna. `Placeholder` dapat berupa fungsi dengan `PlaceholderFunc`.

```go
ui.Input{Masked: true, Value: password.Get, OnChange: password.Set,
	AutoComplete: core.AutoCompletePassword}
```

`KeyboardType` memilih papan ketik perangkat lunak (`KeyboardEmail`, `KeyboardNumber`, `KeyboardURL`, dan lainnya), `ReturnKeyType` memberi label pada tombol return-nya, dan `SubmitBehavior` menentukan apakah Return mengirimkan atau memotong baris pada kolom multibaris. `AutoCapitalize`, `NoAutoCorrect`, dan `AutoComplete` menyesuaikan bantuan dari sistem.

`InputSource` memilih sumber input papan ketik selama kolom memegang fokus dan memulihkan milik pengguna saat fokus hilang, kecuali `KeepInputSource`: `"com.apple.keylayout.ABC"` menjaga kolom untuk kode atau perintah tetap dalam bahasa Inggris di macOS. ID-nya adalah milik platform, berupa ID tata letak papan ketik seperti `"00000409"` di Windows dan metode input Fcitx 5 atau IBus di Linux; sumber yang tidak diaktifkan, ponsel, dan web tidak mengubah apa pun.

```go
ui.Input{Placeholder: "Bundle ID", InputSource: "com.apple.keylayout.ABC"}
```

`Style` digabungkan di atas tampilan kolom, dan `StyleFunc` di atasnya: apa yang mereka atur, seperti tinggi, padding, font, atau warna, menggantikan milik `ui`, dan sisanya tetap. Tepinya menggelap di bawah penunjuk dan mengambil warna aksen saat difokuskan selama keduanya tidak mengatur `BorderColor` atau `Hover`. `Padding` nol membiarkan milik `ui`; `core.Zero`, panjang yang diatur, menghapusnya (`Padding: core.All(core.Zero)`).

```go
ui.Input{Placeholder: "Filter", Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
ui.Field{DefaultValue: "Untitled", Style: core.Style{FontSize: 24, FontWeight: ui.Bold,
	Background: core.Transparent, BorderWidth: core.Zero}}
```

## Fokus dan event

`Ref` memfokuskan dan melepaskan fokus kolom. `OnFocus` dan `OnBlur` mengikuti fokus, `OnSelectionChange` dan `OnCaretMove` mengikuti seleksi dan caret, `OnKeyPress` setiap tombol yang diketik, `OnPaste` apa yang ditempelkan, dan `OnCompositionChange` metode input yang sedang menyusun.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Papan ketik perangkat lunak menutupi bagian bawah ponsel. Letakkan `core.EnvKeyboard` pada padding atau margin bawah konten agar tetap berada di atas papan ketik; perpindahannya beranimasi bersama papan ketik. `cx.DismissKeyboard()` menutup papan ketik, dan `cx.Env().KeyboardHeight()` membaca tingginya.

## Validasi

Aplikasi yang memiliki aturannya. Sebuah signal menyimpan galat, `OnBlur` atau `OnSubmit` mengaturnya, dan pesan di bawah kolom menampilkannya ([Formulir](../forms.md#validation)).

```go preview=input/validation
err := core.NewSignal(cx, "")
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{Value: email.Get, OnChange: email.Set, Label: "Email",
		OnSubmit: func(string) {
			if !strings.Contains(email.Get(), "@") {
				err.Set("Enter an email address.")
			} else {
				err.Set("")
			}
		}},
	ui.Textf(err.Get, 12, ui.Regular, colors.Danger),
}}
```

## Aksesibilitas

Kolom melaporkan `RoleTextField`, atau `RoleSearchField` untuk kotak pencarian ketika `Role`-nya menyatakan demikian, dan membacakan `Label`-nya; `Placeholder` bukan label, jadi berikan `Label` pada kolom tanpa label yang terlihat. `Readonly` dan `Disabled` dilaporkan, dan `OnSubmit` adalah aksi papan ketik. Kolom satu baris, tersamar, dan multibaris mengekspos `Placeholder` saat ini sebagai placeholder atau petunjuk aksesibilitas native di macOS, iOS, dan Linux, diperbarui dan dikosongkan tanpa perhentian fokus kedua untuk label placeholder yang digambar pada kolom multibaris.

## Platform

iOS, Android, Windows, dan Linux menggunakan kolom native mereka; web menggunakan `<input>` atau `<textarea>`. Papan ketik perangkat lunak, isi otomatis, metode input, dan arah teks adalah milik platform. Fokus dan pengetikan diuji dengan cara yang sama di setiap target.

## Terkait

- [Formulir](../forms.md): kolom, validasi, pilihan, dan papan ketik.
- [Aksesibilitas](../accessibility.md): label dan fokus.
- [Tata letak](../layout.md): menjaga konten tetap di atas papan ketik.
- Halaman Input dan Form di galeri.
