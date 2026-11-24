---
title: Switch
description: Pengaturan yang aktif atau nonaktif, dalam tiga ukuran.
group: Komponen
order: 35
---

`ui.Switch` adalah pengaturan yang langsung berlaku. Ia tidak menyimpan nilai: aplikasi yang memilikinya dan memperbaruinya di `OnChange`.

```go preview=switch/basic
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}
```

## Ukuran

`ui.Switch` menerima `Checked`, `OnChange`, `Label`, dan `Size` (`SizeDefault`, `SizeSmall`, atau `SizeLarge`). Kenop bergeser dan trek berganti warna mengikuti jam frame milik platform.

```go preview=switch/small
ui.Switch{Checked: sync.Get, OnChange: sync.Set, Label: "Sync automatically", Size: ui.SizeSmall}
```

Switch digunakan untuk pengaturan yang langsung berlaku. Untuk pilihan di dalam formulir, gunakan [checkbox](checkbox.md); untuk satu dari beberapa opsi, gunakan `ui.Radio` di dalam `ui.RadioGroup` atau sebuah [select](select.md).

## Aksesibilitas

Switch melaporkan `RoleSwitch` dan state aktif atau nonaktifnya. `Label` adalah apa yang dibaca teknologi bantu, dan teks di samping switch biasanya adalah labelnya. Space atau sebuah tekanan mengubah state-nya ketika ia memegang fokus keyboard.

## Platform

Kontrol bawaan digambar oleh aplikasi, sehingga tampak sama di mana pun. `Variant: ui.SwitchSystem` meminta kontrol milik platform: NSSwitch di macOS, UISwitch di iOS, dan switch 52 × 32 dp milik Material 3 di Android. Target lain menggunakan tampilan bawaan. Tekanan, fokus, dan pembaca layar berasal dari platform. Switch bawaan di ponsel memang sengaja berukuran sama dengan di desktop; `SizeSmall` ditujukan untuk baris yang padat, bukan untuk jenis perangkat tertentu.

## Terkait

- [Checkbox](checkbox.md): satu pilihan atau lebih.
- [Formulir](../forms.md): pengaturan di dalam formulir.
- [Aksesibilitas](../accessibility.md): label dan fokus.
- Halaman Switch di galeri.
- [Komponen tanpa gaya](../authoring/base.md#switch): `base.Switch`, untuk switch dengan tampilan Anda sendiri.
