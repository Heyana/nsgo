---
title: Seçim kutusu
description: Bir listeden, açılır pencere veya alt sayfa olarak tek bir değer.
group: Bileşenler
order: 36
---

`ui.Select[T]`, karşılaştırılabilir herhangi bir türdeki değerlerin listesinden birini seçer. Liste her cihazda tetikleyicinin altında açılır; `Presentation: base.PresentSheet` ise bir telefon uygulamasının isteyebileceği gibi onu bunun yerine alttan açılan bir sayfa olarak açar.

```go preview=select/basic height=280
size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options: []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:   size.Get,
	OnChange: size.Set,
	Label:   "Size",
}
```

## Seçenekler ve arama

`Options`, her biri bir `Value`, bir `Label` ve isteğe bağlı bir `Group`, `Icon`, `Disabled` işlevi veya uygulamanın kendi `Content`'i olan `ui.Option[T]` değerleridir. Boş olmayan aynı `Group`'a ait bitişik seçenekler, bir `Select` veya `Combobox` içinde o grubun başlığı altında gösterilir; ok tuşları ve yazma bu başlığı atlar, ekran okuyucular ise onu bir başlık olarak okur. Seçenekler bir sinyali izlediğinde `OptionsFunc` kullanın; böylece açık liste yerinde güncellenir ve alanın değerini ve odağını korur. `Placeholder`, hiçbir seçenek seçilmemişken gösterilir. `Searchable` listeyi yazarak filtreler, `OnClear` bir temizleme düğmesi gösterir ve `Disabled` onu durdurur.

```go preview=select/searchable height=400
ui.Select[string]{
	Options:  languages,
	Value:    language.Get,
	OnChange: language.Set,
	Searchable: true,
	SearchPlaceholder: "Search languages",
	OnClear:  func() { language.Set("") },
	Placeholder: "Choose a language",
}
```

`Style` seçim kutusunu yerleştirir; bir genişlik ayarlamadıkça 200 nokta genişliğindedir. Yüksekliği, dolgusu ve yazı biçimi, bir [Input'unkiler](input.md) gibi alana ulaşır; böylece bir seçim kutusu `SizeSmall` bir düğmeyle hizalanır:

```go
ui.Select[string]{Options: sizes, Value: size.Get, OnChange: size.Set, Label: "Size",
	Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
```

## Davranış

Up, Down, Return ve Space, odaklanmış tetikleyiciden listeyi açar; oklar vurguyu taşır ve önden yazma (typeahead) öğelerin başlıklarına göre atlar. Bir öğeyi seçmek listeyi kapatır ve değerini bildirir. Escape, geri tuşu ve dışarıya bir basış onu bir değişiklik yapmadan kapatır. `Empty`, arama hiçbir şey bulamadığında gösterilen metindir.

## Erişilebilirlik

`Label`, alanı yardımcı teknolojiler için adlandırır. Sinyalleri değiştikçe bu adı yerinde güncellemek için `LabelFunc` kullanın. Boş bir etiket `Placeholder`'ı kullanır. Tetikleyici ve liste seçim kutusunun rollerini taşır ve seçili öğe bildirilir. Klavye tetikleyiciye Tab ile ulaşır ve onu Return veya Space ile açar.

## Platform

macOS, Windows ve Linux'ta liste, tetikleyicinin yanında bir açılır penceredir. iOS ve Android'de bir sayfadır ve seçili satır alanda kalır. Aynı seçenekler ve değerler her hedefte çalışır.

## İlgili

- [Formlar](../forms.md): bir formdaki seçim kutuları.
- [Menü](menu.md): bir değer yerine komutlar.
- [Sekmeler](tabs.md): aynı anda gösterilen birkaç seçenek.
- Galerinin Select ve Combobox sayfaları.
- [Stilsiz bileşenler](../authoring/base.md#select): gruplar ve kendi tetikleyicinizle `base.Select`.
