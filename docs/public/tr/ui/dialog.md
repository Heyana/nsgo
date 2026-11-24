---
title: İletişim kutusu
description: Kalıcı (modal) bir yüzey, bir alt sayfa ve bir uyarı.
group: Bileşenler
order: 41
---

`ui.Dialog`, karartılmış uygulamanın üzerinde duran kalıcı (modal) bir yüzeydir; dokunmatik ekranlarda alttan açılan bir sayfadır. `ui.Sheet` alttan açılan bir paneldir, `ui.AlertDialog` ise kullanıcıdan onay ister.

```go preview=dialog/basic height=360
open := core.NewSignal(cx, false)
ui.Dialog{
	Open:         open.Get,
	OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
	Title:        "Rename the file",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { open.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

## Açma ve kapatma

İletişim kutusunun açık olup olmadığını uygulama tutar. `OnOpenChange`, iletişim kutusunun istediği her değişikliği, dışarıya bir basışı Escape'ten veya geri tuşundan ayıran bir `base.Reason` ile birlikte duyar; `OnClose` kutu kapandıktan sonra bir kez çalışır. `NoCloseOnEscape` ve `NoCloseOnBackdropPress`, Escape'in ve dışarıya bir basışın onu kapatmasını engeller.

`Title` ve `Description` yüzeyin başında yer alır; açıkken değiştiklerinde `TitleFunc` ve `DescriptionFunc` kullanılır, `Footer` ise düğmelerini tutar. `Bare`, uygulamanın kendi yerleşimine sahip bir iletişim kutusu için yüzeyin tamamını `Children`'a bırakır; masaüstleri bunu yüzeyin kenarı içinde, `Style` ve `StyleFunc`'ın verdiği köşelere göre kırpar. `Inset` ise masaüstlerinde pencerenin kenarlarına olan en küçük uzaklığını ayarlar.

Dokunmatik sayfalarda `Bare` artık varsayılan tutamacı ve içerik dolgusunu kaldırır; böylece `Children` paneli doldurur. `NoGrabber` yalnızca tutamacı gizler, `ContentStyle` ve `ContentStyleFunc` içerik sarmalayıcısını, `TitleStyle` ve `TitleStyleFunc` ise başlığı özelleştirir. `Leading` ve `Trailing`, İptal ve Bitti düğmeleri gibi öğeleri başlık satırının uçlarına yerleştirir; başlık bunların arasında ortalanır. `StartingStyle` ve `EndingStyle` sayfa geçişini özelleştirir; özel bir zamanlamaya ihtiyacınız olduğunda bunlara bir `Transition` ekleyin.

`Style`, yüzeyin görünümünün üzerine birleştirilir. `StyleFunc` ondan sonra birleştirilir ve okuduğu sinyalleri, görünüm (appearance) dahil, iletişim kutusu açıkken ve yeniden açıldığında izler. Örneğin `colors := ui.ColorsOf(cx)` iken `StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }` temayı izler. Her iki alan `ui.AlertDialog` üzerinde de çalışır. Sıfır bir renk mevcut rengi yerinde bırakır; temizlemek için `core.RGBA(0xFFFFFF, 0)` gibi sıfır olmayan saydam bir renk kullanın.

## Alt sayfalar

`ui.Sheet`, karartılmış bir arka planın üzerinde altta duran bir paneldir; arka plana bir basış, Escape veya geri tuşu `OnClose`'u çağırır.

Erişilebilir iletişim kutusu adı için `Label` veya `LabelFunc`, testlerde paneli bulmak için `TestID` ayarlayın. `LabelFunc`, açık içeriği değiştirmeden sinyallerini izler. `CloseOnSwipe: true`, aşağı doğru bir kaydırma kapanma eşiğini geçtikten sonra da `OnClose`'u çağırır; kapatmak için bu geri çağırmada `Open`'ı güncelleyin.

Tutamacı ve güvenli alan işlemeyi koruyarak yüzeyin arka planını, köşe yarıçaplarını ve dolgusunu özelleştirmek için `Style` veya `StyleFunc` ayarlayın. `StyleFunc` okuduğu sinyalleri izler. `CornerRadii` olmadan verilen bir `CornerRadius`, varsayılan yuvarlatılmış üst kısmın yerine her köşeyi yuvarlatır.

```go preview=dialog/sheet height=320
ui.Sheet{Open: filters.Get, OnClose: func() { filters.Set(false) }, Children: filterControls}
```

## Uyarılar

`ui.AlertDialog`, kullanıcıdan bir şeyi onaylamasını veya kabul etmesini ister. Her eylem onu kapatır, dışarıya bir basış ise kapatmaz.

```go preview=dialog/alert height=320
ui.AlertDialog{
	Open:         confirm.Get,
	OnOpenChange: func(v bool, _ base.Reason) { confirm.Set(v) },
	Title:        "Delete the note?",
	Description:  "This cannot be undone.",
	Actions: []ui.AlertAction{
		{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
		{Text: "Delete", Color: colors.Danger, Role: core.AlertDestructive, Default: true, OnPress: remove},
	},
}
```

`Role: core.AlertCancel` olan bir eylem Escape veya geri tuşuyla çalışır, ardından `OnOpenChange(false, base.ReasonCancel)` gelir. `Default: true`, uyarı masaüstlerinde, web'de ve Apple TV'de açıldığında bir eyleme klavye odağını verir ve Return'ün onu seçmesini sağlar; bir eyleme basış `OnPress`'i, ardından `OnOpenChange(false, base.ReasonSelect)`'i çalıştırır. Return, klavye odağı başka bir eylemde veya alanda değilken varsayılan eylemi çalıştırır: odak başka bir eyleme taşındığında Return o eyleme basar, uyarının üzerindeki bir iletişim kutusu ise Return'ü kendisinde tutar. Bir eylem çalıştıktan sonra, `OnOpenChange` ne yaparsa yapsın uyarı kapanır ve `Open` yanlış, sonra yeniden doğru olana kadar başka bir eylem çalıştırmaz. Birden fazla eylem aynı işareti taşıyorsa ilki kazanır. İşaretler olmadan Escape bir eylem çalıştırmadan kapatır ve Return'ün varsayılan bir eylemi olmaz. `Role: core.AlertDestructive` bir eylemi tanımlar; görünümünü `Variant` veya `Color` seçer.

Sistemin kendi uyarısı veya eylem sayfası için `cx.ShowAlert` kullanın ([Platform hizmetleri](../services.md#uyarılar-ve-pano)).

## Davranış

Kalıcı bir yüzey arkasındaki uygulamayı engeller, klavye odağını alır ve aksi söylenmedikçe Escape ve geri tuşuyla kapanır. Dokunmatik ekranlarda bir iletişim kutusu, aşağı doğru bir sürüklemeyi izleyen ve bir eşiği geçince kapanan bir sayfadır.

## Erişilebilirlik

`ui.Dialog`, `RoleDialog` rolünü veya açıkça verilen `Role`'ünü, `ui.AlertDialog` ise masaüstlerinde ve dokunmatik ekranlarda `RoleAlertDialog` rolünü bildirir. Her ikisi de başlığını, ya da başlık yetmediğinde `Label`'ı, ardından açıklamayı okur. Bir iletişim kutusu açıldığında odak onun içine taşınır ve kapandığında geri döner; arka plan bir durak değildir. Alt bilginin düğmeleri rollerini ve etiketlerini korur.

## Platform

Android'de ve dokunmatik ekranlarda bir iletişim kutusu alttan açılan bir sayfadır; masaüstlerinde ise arka planı olan ortalanmış bir yüzeydir. İçerik aynıdır; yalnızca sunum değişir.

## İlgili

- [Gezinme](../navigation.md): sayfalar, sekmeler ve kalıcı yüzeyler bir arada.
- [Menü](menu.md): sabitlenmiş komutlar ve bağlam menüleri.
- [Erişilebilirlik](../accessibility.md): odak ve iletişim kutusu rolleri.
- Galerinin Dialog, Sheet ve AlertDialog sayfaları.
- [Stilsiz bileşenler](../authoring/base.md#dialog): iOS'un sunumları ve kendi hareketinizle `base.Dialog` ve `base.Sheet`.
