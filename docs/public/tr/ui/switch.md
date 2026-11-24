---
title: Anahtar
description: Üç boyutta, açık veya kapalı olan bir ayar.
group: Bileşenler
order: 35
---

`ui.Switch` hemen etkili olan bir ayardır. Kendi değerini tutmaz: değer uygulamaya aittir ve uygulama onu `OnChange` içinde günceller.

```go preview=switch/basic
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}
```

## Boyutlar

`ui.Switch`, `Checked`, `OnChange`, `Label` ve `Size` (`SizeDefault`, `SizeSmall` veya `SizeLarge`) alanlarını kabul eder. Tutamaç kayar ve iz, platformun kare saatine göre renk değiştirir.

```go preview=switch/small
ui.Switch{Checked: sync.Get, OnChange: sync.Set, Label: "Sync automatically", Size: ui.SizeSmall}
```

Anahtar, hemen uygulanan bir ayar içindir. Bir form içindeki bir seçim için bir [onay kutusu](checkbox.md) kullanın; birkaç seçenekten biri için bir `ui.RadioGroup` içindeki `ui.Radio`'ları veya bir [seçim kutusu](select.md) kullanın.

## Erişilebilirlik

Anahtar `RoleSwitch` rolünü ve açık ya da kapalı durumunu bildirir. Yardımcı teknolojilerin okuduğu `Label`'dır ve bir anahtarın yanındaki metin genellikle onun etiketidir. Klavye odağı ondayken Space veya bir basma onu açıp kapatır.

## Platform

Varsayılan denetim uygulama tarafından çizilir, bu yüzden her yerde aynı görünür. `Variant: ui.SwitchSystem` platformun denetimini ister: macOS'ta NSSwitch, iOS'ta UISwitch ve Android'de Material 3'ün 52 × 32 dp anahtarı. Diğer hedefler varsayılan görünümü kullanır. Basmalar, odak ve ekran okuyucu platformundur. Bir telefonun varsayılan anahtarı tasarım gereği masaüstündekiyle aynı boyuttadır; `SizeSmall` bir cihaz için değil, sık satırlar içindir.

## İlgili

- [Onay kutusu](checkbox.md): bir veya daha fazla seçim.
- [Formlar](../forms.md): bir formdaki ayarlar.
- [Erişilebilirlik](../accessibility.md): etiketler ve odak.
- Galerinin Switch sayfası.
- [Stilsiz bileşenler](../authoring/base.md#switch): kendi görünümünüze sahip bir anahtar için `base.Switch`.
