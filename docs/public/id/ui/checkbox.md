---
title: Checkbox
description: Kotak centang, tercentang, tidak tercentang, atau tak tentu.
group: Komponen
order: 34
---

`ui.Checkbox` adalah kotak centang dengan tampilan `ui`, dengan label di sampingnya. Komponen ini tidak menyimpan nilai: aplikasi menentukan apakah kotak tercentang dan memperbaruinya di `OnChange`.

```go preview=checkbox/basic
done := core.NewSignal(cx, false)
ui.Checkbox{Checked: done.Get, OnChange: done.Set, Text: "Remember me"}
```

## Konten

`ui.Checkbox` menampilkan `Text` atau `Children` milik aplikasi di samping kotaknya. `Label` menggantikan keduanya untuk teknologi asistif, dan `Hint` menjelaskan kegunaannya.

## Status

`Indeterminate` bukan tercentang maupun tidak tercentang, untuk "pilih semua" atas pilihan sebagian; tekanan kemudian meminta status tercentang. `Disabled` menghentikan tekanan dan fokus.

```go preview=checkbox/indeterminate height=220
ui.Checkbox{
	Checked:       func() bool { return chosen.Get() == len(files) },
	Indeterminate: func() bool { return chosen.Get() > 0 && chosen.Get() < len(files) },
	OnChange:      chooseAll,
	Text:          "Select all",
}
```

## Modifier tekanan

Gunakan `OnChangeEvent` sebagai ganti `OnChange` ketika sebuah daftar membutuhkan pemilihan rentang dengan Shift-tekan:

```go
ui.Checkbox{Checked: done.Get, Text: "Remember me", OnChangeEvent: func(next bool, press core.PressEvent) {
	if press.Modifiers.Shift {
		chooseRange(next)
		return
	}
	done.Set(next)
}}
```

Jika diatur, callback ini menggantikan `OnChange`; aplikasi tetap memperbarui nilainya. Aktivasi dengan papan ketik di desktop dan web mempertahankan modifier setelah pintasan aplikasi; di Windows, Alt dengan Space atau Return justru menuju menu jendela. `onChangeEvent` di TSX menerima `{ checked, press }`.

## Aksesibilitas

Kotak centang melaporkan `RoleCheckbox` beserta status tercentang atau tak tentunya. Namanya adalah `Text`, `Label`, atau teks dari anak-anaknya. Space mengubah statusnya saat kotak memegang fokus papan ketik, dan pembaca layar mengumumkan status barunya.

## Platform

Kotak digambar oleh aplikasi, bukan kontrol platform, sehingga tampak sama di mana pun. Tekanan, cincin fokus, dan pembaca layar adalah milik platform. Komponen ini diuji berdasarkan peran dan label di setiap target.

## Terkait

- [Switch](switch.md): pengaturan yang langsung berlaku.
- [Formulir](../forms.md): pilihan dalam formulir.
- [List](list.md): kotak centang di dalam baris.
- Halaman Checkbox di galeri.
- [Komponen tanpa gaya](../authoring/base.md#checkbox): `base.Checkbox`, untuk kotak dengan tampilan Anda sendiri.
