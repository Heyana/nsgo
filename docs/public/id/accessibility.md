---
title: Aksesibilitas
description: "Label, peran, nilai, aksi, dan fokus untuk pembaca layar dan papan ketik."
group: Konsep inti
order: 61
---

## Apa yang dibaca pembaca layar

`Label` suatu tampilan adalah apa yang diucapkan oleh teknologi asistif, `AccessibilityDescription` memberikan informasi lebih lanjut tentangnya yang dibacakan setelah label, `Hint` menjelaskan apa yang terjadi saat mengaktifkannya, dan `Role` menunjukkan jenis elemennya: `core.RoleButton`, `RoleLink`, `RoleHeader`, `RoleImage`, `RoleCheckbox`, `RoleSwitch`, `RoleRadio`, `RoleTab`, `RoleTextField`, `RoleSearchField`, `RoleStatus`, `RoleMenu`, `RoleMenuItem`, `RoleDialog`, `RoleAlertDialog`, `RoleAlert`, `RoleAdjustable`, `RoleProgressBar`, `RoleTable`, dan lainnya. Secara default, status adalah wilayah langsung (*live region*) yang sopan; kolom pencarian adalah kolom teks yang diumumkan sebagai kotak pencarian. Dialog dibacakan beserta peran dan labelnya saat terbuka, lalu deskripsinya setelah itu, sebagaimana judul dan deskripsi `ui.Dialog`. Teks tanpa label dibacakan sesuai kontennya. `LabelFunc`, `HintFunc`, dan `AccessibilityDescriptionFunc` mengikat teks aksesibilitas yang dapat berubah pada tampilan yang sama, sehingga kontrol tetap mempertahankan fokus dan statusnya saat bahasa diganti. Semua elemen inti yang memiliki label menerima `LabelFunc`, dan `View` serta `Text` menerima `HintFunc`.

Gunakan `TextFunc` untuk teks yang dapat berubah pada `ui.Button`, `ui.Checkbox`, `ui.MenuItem`, dan `ui.AlertAction`. Nama aksesibelnya mengikuti teks tersebut. Tombol dan kotak centang juga menerima `LabelFunc` terpisah. Slider, tab, sakelar (*switch*), bilah progres, dan pemicu popup menerima `LabelFunc` untuk nama aksesibel yang dapat berubah. `ui.Option.LabelFunc` menyediakan teks opsi pada tab, select, dan combobox. Input angka dan kode, cincin progres, serta kedua varian spinner juga menerima `LabelFunc`.

```go
core.View{
	Role:  core.RoleButton,
	Label: "Add to cart",
	Hint:  "Adds the item and opens the cart",
	OnPress: add,
	Children: []core.Node{core.Icon{SVG: lucide.ShoppingCart}},
}
```

`AccessibilityValue` memberikan rentang atau nilai tekstual: nilai minimum, maksimum, dan angka saat ini pada slider, atau teks seperti "3 items". `Accessible` menjadikan satu tampilan sebagai satu elemen tunggal, sehingga nama dan harga pada kartu dibacakan bersama alih-alih sebagai dua hentian terpisah, dan tanpa label, tampilan tersebut akan membacakan teks turunannya. `AccessibilityElementsHidden` menyembunyikan dekorasi, seperti ikon panah (*chevron*) atau pemisah, sedangkan `ImportantForAccessibility` menyembunyikan seluruh subpohon.

```go
core.View{Accessible: true, Label: "Apples, $1.20 a kilo", Children: []core.Node{
	core.Text{Content: "Apples"},
	core.Text{Content: "$1.20 a kilo"},
}}
```

## Label yang terlihat

Berikan `Ref` milik kontrol sebagai `LabelFor` pada `core.Text` atau `core.View`. Menekan label akan memfokuskan kontrol dan menjalankan aksi tekannya, seperti mengalihkan kotak centang. Teks yang sedang terlihat menjadi nama kontrol tanpa perlu mengulang string `Label`, dan label mengikuti status `Disabled` serta `Invalid` milik kontrol untuk aksesibilitas dan gaya. Label tidak memperoleh peran tombol maupun hentian Tab.

```go
field := new(core.Ref)
return core.View{Children: []core.Node{
	core.Text{Content: "Email", LabelFor: field},
	base.Input{Ref: field},
}}
```

`LabelledBy: []*core.Ref{label, requirement}` memberi nama kontrol dari beberapa elemen yang terlihat sesuai urutan yang diberikan, mendahului `Label`; ini tidak meneruskan penekanan. Selain itu, `Label` lebih diutamakan daripada nama implisit dari `LabelFor`, yang urutannya mengikuti pohon tampilan. Pembaruan teks, pemindahan, penyembunyian, penghapusan, dan pemasangan ulang akan memperbarui relasi tersebut. Referensi yang hilang, berulang, siklik, dan tersembunyi diabaikan; asosiasi tetap berada di dalam satu permukaan (*surface*) dan cakupan pembatasan (*confinement scope*). Label tidak dapat mengaktifkan kontrol di balik modal.

Tombol, kotak centang, radio, sakelar, select, dan input angka pada `base` serta `ui` meneruskan `Ref` dan `LabelledBy` ke kontrolnya; slider mengekspos `ThumbRef` dan `StartThumbRef`. Di TSX, gunakan `labelFor={field()}` dan `labelledBy={[label()]}` dengan sinyal node yang diisi melalui `ref={node => field.set(node)}`, sehingga label yang berada sebelum kolomnya tetap diperbarui ketika kolom tersebut dipasang.

## Aksi

`AccessibilityActions` mencantumkan apa yang ditawarkan pembaca layar pada suatu elemen: aksi standar (`core.ActionActivate`, `ActionIncrement`, `ActionDecrement`, `ActionEscape`, `ActionMagicTap`) dan aksi milik aplikasi sendiri, masing-masing dengan `Name` dan `Label`. `OnAccessibilityAction` menerima namanya. Gunakan `AccessibilityActionsFunc` pada `core.View` atau `core.Input` untuk mengubah aksi yang tersedia atau menerjemahkan labelnya sementara elemen tetap terpasang.

```go
core.View{
	Role: core.RoleAdjustable,
	Label: "Quantity",
	AccessibilityValue: func() core.AccessibilityValue {
		return core.AccessibilityValue{Max: 10, Now: float32(n.Get()), Text: fmt.Sprintf("%d items", n.Get())}
	},
	AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
	OnAccessibilityAction: func(name string) {
		if name == core.ActionIncrement {
			n.Update(func(v int) int { return v + 1 })
		} else {
			n.Update(func(v int) int { return v - 1 })
		}
	},
	Children: quantityControls,
}
```

`MultiSelectable` menandai grid, listbox, atau tree yang mengizinkan lebih dari satu item dipilih. `base.DataTable` mengaturnya untuk pemilihan multi-baris; web, Windows, dan Linux mengekspos flag aksesibilitas yang sesuai.

`Expanded` menyatakan apakah tombol penyingkap (*disclosure*) atau tombol menu sedang terbuka, `Busy` menandakan konten sedang dimuat, `Invalid` menandakan aplikasi menolak isi kolom atau nilai kontrol (diberi gaya dengan `aria-invalid:`), serta `Checked`, `Selected`, dan `Pressed` melaporkan status kontrol. Bagian-bagian suatu kontrol dapat mengambil statusnya melalui `PartStates`, sehingga indikator mengikuti kotak centangnya tanpa menjadi hentian fokus tersendiri.

`SelectionItem` menandai suatu tampilan sebagai item dalam koleksi yang dapat dipilih secara terpisah dari nilai `Selected`-nya saat ini. `base.DataTable` mengaturnya pada baris atau sel ketika tabel memilihnya, dan `base.Tree` mengaturnya pada setiap baris; Windows mempertahankan pola SelectionItem dan web menulis `aria-selected="true"` atau `"false"`. Tabel tanpa pemilihan membiarkan baris tanpa kedua penanda tersebut.

## Perubahan langsung

`LiveRegion` membuat pembaca layar mengumumkan label tampilan dan teks di dalamnya saat berubah: `core.LiveRegionPolite` menunggu hingga pengguna selesai berinteraksi, sedangkan `LiveRegionAssertive` langsung menyela. `cx.Announce` mengucapkan satu pesan satu kali.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("%d items in the basket", count.Get()) },
	LiveRegion: core.LiveRegionPolite}
```

## Fokus dan papan ketik

Tooltip mendeskripsikan pemicunya sebelum pemicu tersebut menerima fokus. `View.Tooltip`, `base.Tooltip`, dan `ui.Tooltip` menggunakan teksnya saat ini sebagai deskripsi aksesibilitas kontrol, kecuali jika mengulang nama kontrol, aplikasi menyediakan `AccessibilityDescription`, atau tooltip berstatus `Disabled`. Pembungkus (*wrapper*) meneruskan deskripsi tersebut ke kontrol-kontrolnya. Gelembung tooltip dan turunannya disembunyikan dari pembaca layar dan tidak pernah mengumumkan peringatan; oleh karena itu tooltip bawaan `ui.IconButton` tidak mengulang labelnya.

`base.DataTable` mengekspos grid bernamanya sebagai hentian fokus papan ketik dan aksesibilitas; lapisan penangkap klik pointer (*pointer hit layer*) disembunyikan dari teknologi asistif.

Di desktop, web, dan Apple TV, tampilan yang menerima penekanan menjadi hentian Tab di samping kolom teks; `NoFocus` mengecualikannya dan `Focusable` menjadikan tampilan tanpa penekanan sebagai hentian fokus. `Style.FocusVisible` menggambar cincin fokus hanya ketika papan ketik yang memindahkan fokus (`focus-visible:`), `Style.Focus` untuk segala fokus, dan `Style.FocusWithin` untuk fokus di dalam suatu tampilan; `OnFocus` dan `OnBlur` mendengarnya—`OnBlur` juga berjalan saat tampilan yang berfokus meninggalkan pohon—serta `Ref.Focus()` dan `Ref.SetAccessibilityFocus()` memindahkannya. `core.FocusedRef(cx)` adalah ref ke tampilan yang sedang memilikinya, berguna bagi elemen yang terbuka tanpa pemicu—seperti bilah pencarian dari pintasan tombol—agar dapat mengembalikan fokus saat ditutup. Return dan Space menekan tampilan yang berfokus, dan tombol panah berpindah melalui daftar tab, grup radio, dan menu. `Tooltip` suatu tampilan langsung muncul saat papan ketik memindahkan fokus ke tampilan tersebut, hingga fokus berpindah atau tombol Escape ditekan. `OnOpen` milik popup berjalan setelah kontennya tersedia, sehingga pemanggilan `Focus` di sana memindahkan fokus ke dalam popup, sebagaimana seharusnya pada dialog non-modal; konten popover mengikuti pemicunya dalam urutan Tab, dan fokus kembali ke pemicu saat popup ditutup.

Di Android, iPhone, dan iPad, `Ref.Focus()` serta `AutoFocus` pada tombol dapat memfokuskan tampilan yang dapat ditekan atau berproperti `Focusable` (selama aktif) untuk papan ketik perangkat keras. Menyentuh kontrol tidak memindahkan fokus tersebut. Space dan Return mengaktifkan elemen tekan yang berfokus setelah peta tombol (*keymap*), dengan tetap mempertahankan Control, Alt, Shift, dan tombol platform pada setiap fase penekanan. `NoFocus`, kontrol yang dinonaktifkan atau tersembunyi, serta kontrol di balik modal tidak dapat mengambil fokus tersebut. Memfokuskan tampilan tidak membuka papan ketik perangkat lunak; kolom teks native mempertahankan perilaku penyuntingannya. watchOS dan program mini tidak memiliki jalur aktivasi papan ketik fisik.

`cx.Env().ScreenReader()` menyatakan apakah pembaca layar sedang berjalan, `cx.Env().ReduceMotion()` apakah pengguna meminta pengurangan gerak, `cx.Env().IncreaseContrast()` apakah pengguna meminta kontras lebih tinggi (Increase Contrast di macOS, iOS, dan watchOS; tema kontras Windows; High Contrast GNOME, pengaturan kontras GTK 4.20 atau tema GTK kontras tinggi; pengaturan kontras Android 14 di atas standar; `prefers-contrast: more` atau `forced-colors: active` di web), `cx.Env().FontScale()` skala ukuran teks, dan `cx.Env().RTL()` arah tata letak. Teks mengikuti ukuran teks kecuali `NoFontScaling` diatur, dan tata letak kanan-ke-kiri mencerminkan tepi fisiknya saat menggunakan tepi logis.

## Berikutnya

- [Gaya dan tema](styles.md): status fokus dan nonaktif.
- [Formulir](forms.md): label dan nilai untuk kolom.
- [Kanvas dan shader](canvas.md): mengekspos bagian-bagian gambar.
