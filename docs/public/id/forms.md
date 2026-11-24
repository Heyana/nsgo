---
title: Formulir
description: Kolom teks, fokus, keyboard perangkat lunak, switch, pemilih, dan validasi.
group: Konsep inti
order: 57
---

## Kolom teks

`ui.Input` adalah kolom teks platform dalam tampilan `ui`, dengan field milik `base.Input`. Dengan `Value` kolom bersifat terkontrol: aplikasi memegang nilainya dan memperbaruinya di `OnChange`; dengan `DefaultValue` kolom menyimpan nilainya sendiri.

```go
name, email := core.NewSignal(cx, ""), &core.Ref{}

ui.Input{
	Value:         name.Get,
	OnChange:      name.Set,
	Placeholder:   "Your name",
	ReturnKeyType: core.ReturnKeyNext,
	OnSubmit:      func(string) { email.Focus() },
}
```

`Masked` menyembunyikan karakter, `Readonly` menghentikan pengeditan sementara teks tetap dapat dipilih dan disalin (di mini program hanya pada kolom satu baris WeChat: kolom multibaris WeChat dan kolom Alipay mati seperti `Disabled`, karena host tersebut tidak dapat membatalkan suntingan), `Disabled` menonaktifkan kolom, dan `Multiline` membungkus teks dan membesar bersamanya kecuali gayanya menentukan ukurannya. Baris kolom multibaris, beserta placeholder-nya, setinggi `LineHeight` gayanya, dengan font berada di tengah masing-masingnya seperti pada `core.Text`; kolom satu baris menjaga barisnya tetap di tengah tingginya. `KeyboardType` memilih keyboard perangkat lunak (`core.KeyboardEmail`, `core.KeyboardNumber`, `core.KeyboardURL`, dan lainnya), `AutoComplete` memberi tahu sistem apa yang diisi kolom untuk autofill, dan `MaxLength` membatasi unit UTF-16 yang dimasukkan pengguna. `AutoCapitalize` dan `NoAutoCorrect` menyetel perilaku keyboard.

`OnSelectionChange` dan `OnCaretMove` melaporkan seleksi dan posisi kursor sisip (caret), untuk menu yang terbuka di sana. `Suggest`, sebuah `core.Suggest`, mengikuti kueri setelah pemicu seperti `@`, dan `ui.SuggestList` di samping kolom menawarkan baris pilihan di caret, yang dipilih dengan Up, Down, Return, dan tekanan, sebagaimana `Suggest` milik `chat.Composer` untuk mention. `OnKeyPress` mendengar setiap tombol, `OnPaste` apa yang ditempel pengguna, dan `OnCompositionChange` mengikuti metode input.

Di iOS dan Android, `cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, handler)` mengambil tombol Return perangkat lunak sebelum kolom mengirimkan, mengaburkan fokus, atau menyisipkan baris baru. `KeyAuto` dan `KeyNotEditing` menyerahkannya ke `SubmitBehavior`; di iOS, Return yang mengonfirmasi teks bertanda tetap ditangani oleh metode input.

## Fokus dan keyboard

`Ref.Focus()` memfokuskan kolom atau, di desktop, web, dan Apple TV, view yang dapat ditekan yang aktif atau view `Focusable`. `Ref.Blur()` hanya menghapus fokus view-nya sendiri, dan `cx.DismissKeyboard()` menutup keyboard perangkat lunak. `OnFocus` dan `OnBlur` mengikuti fokus, dan `View.OnFocus`/`OnBlur` melakukan hal yang sama untuk view yang dapat difokuskan mana pun. View terfokus yang keluar dari pohon, saat `Show`-nya menutup atau barisnya hilang, mendengar `OnBlur` sekali, setelah commit yang menghapusnya atau, ketika platform melaporkan fokus berikutnya terlebih dahulu, sebelum `OnFocus` view tersebut, dan view di sekitarnya yang tetap ada mendengar `OnFocusOut`.

Permintaan fokus, blur, gulir, dan zoom dari `Ref` tidak melakukan apa pun selama elemennya berada di dalam konten `Holder` yang tidak ditampilkan di tempat mana pun, dan permintaan yang masih antre saat elemen keluar dari pohon akan dibatalkan. Menampilkan konten lagi tidak memutar ulang permintaan tersebut: mintalah lagi setelah ditempatkan.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Keyboard perangkat lunak menutupi bagian bawah ponsel. `core.EnvKeyboard` sebagai padding atau margin bawah menjaga konten tetap di atasnya, dan perpindahannya dianimasikan bersama keyboard:

```go
core.View{Style: core.Style{
	Padding:    core.All(16),
	PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard},
	Gap:        12,
}, Children: form}
```

Scroll view di sekitar formulir dapat menutup keyboard saat diseret dan menentukan apa yang dilakukan ketukan saat keyboard tampil:

```go
core.Scroll{Behavior: core.ScrollBehavior{
	KeyboardDismissMode:       core.KeyboardDismissOnDrag,
	KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
}, Children: []core.Node{form}}
```

`cx.Env().KeyboardHeight()` dan `cx.Env().KeyboardDuration()` membaca ukuran dan durasi keyboard dalam kode, dan `cx.Env().SafeArea()` membaca inset di sekitar layar.

## Pilihan

`ui.Switch` adalah pengaturan yang langsung berlaku, `ui.Checkbox` dapat dicentang, tidak dicentang, atau tak tentu (indeterminate), dan `ui.Radio` adalah satu pilihan dari `ui.RadioGroup`. `ui.Select[T]` membuka daftar opsi di bawah pemicunya, atau sebagai sheet bila diminta oleh `Presentation`-nya, dan `ui.Tabs[T]` menampilkan beberapa sekaligus.

```go
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}

size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options:  []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:    size.Get,
	OnChange: size.Set,
	Label:    "Size",
}
```

`ui.NumberInput` dan `base.NumberInput` menerima `LabelFunc` untuk nama aksesibel yang berubah. `Format` kustom mengikuti signal yang dibacanya saat kolom tidak sedang diedit; pergantian bahasa mempertahankan draf aktif hingga pengeditan selesai. `ui.NumberInput` menaikturunkan dan menggeser angka dengan batas, `ui.DatePicker` dan `ui.TimeField` mengedit tanggal dan waktu, `ui.ColorPicker` mengedit warna, dan `ui.TagInput` mengedit daftar tag.

`ui.OtpInput` menerima kode sekali pakai dalam sel-sel. Komponen ini dan `base.OtpInput` menerima `LabelFunc` tanpa mengganti kolom yang terfokus atau kodenya. `Size` mengatur tinggi dan lebar bawaan, 36 poin di desktop dan TV serta 44 di perangkat sentuh. `CellWidth` dan `CellHeight` menimpa masing-masing dimensi secara independen. `TextStyle` menimpa tipografi sel, `CornerRadius` mengubah sudutnya, dan `CaretHeight` menimpa tinggi yang diturunkan dari tinggi baris.

```go
quantity := core.NewSignal(cx, 1.0)
ui.NumberInput{Value: quantity.Get, OnChange: quantity.Set, Min: 1, Max: 99, Step: 1, Label: "Quantity"}
```

## Baris berkelompok

`ui.FormSection` menempatkan baris-barisnya di atas kartu, dengan judul di atas dan footer di bawah. Atur `CardStyle` untuk latar belakang, batas, atau sudut kartu. `CardStyleFunc` bergabung di atasnya dan mengikuti signal yang dibacanya. `DividerInset` menerima setiap baris anak, termasuk baris yang dibuat oleh `core.Each`: kembalikan inset dan `true` untuk menimpa pembatasnya, atau `false` untuk mempertahankan perataan judul otomatis (12 poin untuk node lainnya). Inset nol membuat pembatas selebar penuh.

`ui.FormRow` menempatkan `Leading` atau ubin ikon sebelum judul. Atur `LeadingWidth` untuk mencadangkan ruang bagi node Anda dan meratakan pembatas di bawahnya dengan judul. Slot tersebut secara bawaan berukuran 24 poin. Atur `TileSize` untuk menskalakan ubin ikon, glif, dan sudut bersama-sama. `TitleStyle` dan `DetailStyle` menimpa tipografi judul dan detail. `TitleMaxLines` membatasi baris judul (nol berarti tidak terbatas), `MinHeight` menimpa tinggi minimum baris dalam poin, dan `Below` menempatkan node setelah judul dan detail di kolom teksnya. `MinHeight` nol mempertahankan nilai bawaan: 40 poin, atau 48 di perangkat sentuh.

Untuk kumpulan pilihan tunggal, bungkus bagian tersebut dalam `base.RadioGroup` dan atur `Role: core.RoleRadio`, `Checked`, dan `OnPress` masing-masing baris. Baris tersebut menggunakan `base.Radio` untuk status aksesibilitas tercentang dan terpilihnya, satu pemberhentian Tab, dan pemilihan tombol panah; menekan baris yang sudah tercentang tidak melakukan apa pun. Aplikasi memegang nilai yang dipilih.

`ui.FormSection.TitleFunc` dan `FooterFunc`, serta `ui.FormRow.TitleFunc`, `DetailFunc`, dan `LabelFunc`, memperbarui kata-katanya tanpa membangun ulang baris. Detail, judul bagian, atau footer yang kosong akan menghapus teks opsional tersebut; label baris yang kosong menggunakan judulnya saat ini. `ui.Select` yang dapat dicari mengikat `SearchPlaceholderFunc` dan `EmptyFunc` sambil mempertahankan pencarian yang terbuka dan pilihannya.

## Validasi

Aplikasi memiliki aturannya: sebuah signal menyimpan error, `OnBlur` atau `OnSubmit` mengaturnya, dan `core.Show` menambahkan pesan di bawah kolom selama error tersebut ada.

```go
address := core.NewSignal(cx, "")
err := core.NewSignal(cx, "")
colors := ui.ColorsOf(cx)
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{
		Value:    address.Get,
		OnChange: func(v string) { address.Set(v); err.Set("") },
		Label:    "Email",
		OnSubmit: func(string) {
			if !strings.Contains(address.Get(), "@") {
				err.Set("Enter an email address.")
			}
		},
	},
	core.Show{
		When: func() bool { return err.Get() != "" },
		Then: func(*core.Scope) core.Node { return ui.Textf(err.Get, 12, ui.Regular, colors.Danger) },
	},
}}
```

`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` menganimasikan pesan yang muncul, dan `OnSubmit` dengan `ReturnKeyNext` memindahkan keyboard ke kolom berikutnya.

## Berikutnya

- [Navigasi](navigation.md): tempat formulir berada.
- [Teks](text.md): bagaimana teks kolom diberi gaya.
- [Aksesibilitas](accessibility.md): label dan nilai yang dibutuhkan pembaca layar.
