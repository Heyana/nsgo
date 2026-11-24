---
title: Onay kutusu
description: İşaretli, işaretsiz veya belirsiz bir onay kutusu.
group: Bileşenler
order: 34
---

`ui.Checkbox`, `ui`'nin görünümünde, yanında bir etiket bulunan bir onay kutusudur. Hiçbir değer tutmaz: işaretli olup olmadığını uygulama söyler ve bunu `OnChange` içinde günceller.

```go preview=checkbox/basic
done := core.NewSignal(cx, false)
ui.Checkbox{Checked: done.Get, OnChange: done.Set, Text: "Remember me"}
```

## İçerik

`ui.Checkbox`, kutusunun yanında `Text` değerini veya uygulamanın `Children` öğelerini gösterir. `Label` yardımcı teknolojiler için ikisinin de yerini alır, `Hint` ise ne işe yaradığını söyler.

## Durumlar

`Indeterminate` ne işaretli ne de işaretsizdir; kısmi bir seçim üzerindeki "tümünü seç" içindir. Bu durumda bir basış işaretli durumu ister. `Disabled` basışları ve odağı durdurur.

```go preview=checkbox/indeterminate height=220
ui.Checkbox{
	Checked:       func() bool { return chosen.Get() == len(files) },
	Indeterminate: func() bool { return chosen.Get() > 0 && chosen.Get() < len(files) },
	OnChange:      chooseAll,
	Text:          "Select all",
}
```

## Basma değiştiricileri

Bir liste Shift ile basarak aralık seçimine ihtiyaç duyduğunda `OnChange` yerine `OnChangeEvent` kullanın:

```go
ui.Checkbox{Checked: done.Get, Text: "Remember me", OnChangeEvent: func(next bool, press core.PressEvent) {
	if press.Modifiers.Shift {
		chooseRange(next)
		return
	}
	done.Set(next)
}}
```

Ayarlandığında bu geri çağırma `OnChange`'in yerini alır; değeri yine uygulama günceller. Masaüstünde ve web'de klavyeyle etkinleştirme, uygulama kısayollarından sonra değiştiricileri korur; Windows'ta Space veya Return ile birlikte Alt bunun yerine pencere menüsüne gider. TSX'in `onChangeEvent` işlevi `{ checked, press }` alır.

## Erişilebilirlik

Onay kutusu `RoleCheckbox` rolünü ve işaretli ya da belirsiz durumunu bildirir. Adı `Text`, `Label` veya alt öğelerin metnidir. Klavye odağı üzerindeyken Space onu açıp kapatır ve bir ekran okuyucu yeni durumu duyurur.

## Platform

Kutu bir platform denetimi tarafından değil, uygulama tarafından çizilir; bu yüzden her yerde aynı görünür. Basış, odak halkası ve ekran okuyucu platformundur. Her hedefte rol ve etiketle test edilir.

## İlgili

- [Anahtar](switch.md): hemen etkili olan bir ayar.
- [Formlar](../forms.md): bir formdaki seçimler.
- [Liste](list.md): satırlardaki onay kutuları.
- Galerinin Checkbox sayfası.
- [Stilsiz bileşenler](../authoring/base.md#checkbox): kendi görünümünüze sahip bir kutu için `base.Checkbox`.
