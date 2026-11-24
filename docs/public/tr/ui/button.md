---
title: Button
description: Bir basma düğmesi, varyantları ve boyutları ve simge düğmesi.
group: Bileşenler
order: 31
---

`ui.Button`, `ui`'ın görünümünde, bir pencerenin ihtiyaç duyduğu varyant ve boyutlara sahip bir basma düğmesidir. `OnPress` tamamlanmış bir basışta çalışır.

```go preview=button/basic
ui.Button{Text: "Save", OnPress: save}
ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: cancel}
```

## Varyantlar ve boyutlar

`ui.Button.Variant`; `ButtonPrimary` (varsayılan), `ButtonSecondary`, `ButtonOutline`, `ButtonGhost`, `ButtonTinted` veya `ButtonDanger` olabilir. Tinted ve danger, temanın vurgu veya tehlike rengini yumuşak bir dolgu üzerinde kullanır. Masaüstü ve TV kalıplarında `Size`; `SizeDefault` (28 punto), `SizeSmall` (24) veya `SizeLarge` (32) olur. Dokunmatik kalıplarda aynı boyutlar 44, 40 ve 48 puntodur; böylece düğme yeniden stillendirilmeden kullanılabilir.

```go preview=button/variants
ui.Button{Text: "Delete", Variant: ui.ButtonOutline, OnPress: remove}
ui.Button{Text: "Continue", Size: ui.SizeLarge, OnPress: next}
ui.Button{Text: "Save", Icon: lucide.Check, Flex: 1, OnPress: save}
```

`Color` düğmeyi bir varyantın rengi yerine uygulamanın bir rengiyle doldurur, `Tint` ise simgesini renklendirir. `Flex` ona bir satırda bir pay verirken `Style` onu yerleştirir: düğmenin kapsayıcısını doldurmasını sağlayabilir, uygulamanın belirlediği bir boyut ayarlayabilir, kenar boşlukları ekleyebilir veya köşelerini yuvarlayabilir; `TestID` ise onu testler için adlandırır.

```go
ui.Button{Text: "Continue", Size: ui.SizeLarge,
    Style: core.Style{Width: core.Pct(100), Height: core.Pt(50)}, OnPress: next}
```

Yalnızca simgeden oluşan bir düğme `ui.IconButton`'dır: `Variant` olarak `IconButtonGhost`, `IconButtonRaised` veya `IconButtonFilled` (temanın iç rengiyle veya `Fill` ile düz bir daire), punto cinsinden bir `Size` ve yardımcı teknolojiler için bir `Label` alır. Masaüstlerinde etiket, işaretçi düğmenin üzerinde durduğunda bir araç ipucu olarak da görünür; `Tooltip` orada başka bir şey söyler ve `""` döndüren bir `Tooltip` hiçbir şey göstermez. `Icon` içeren ve `Text` içermeyen bir `ui.Button` da karedir ve `Label` gerektirir.

```go preview=button/icon
ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost, OnPress: remove}
```

## İçerik ve durumlar

`ui.Button`, `Text`'i, isteğe bağlı önde bir `Icon`'u veya ikisini birden gösterir. Etiketin yazı tipi boyutunu, kalınlığını veya diğer metin stillerini özelleştirmek için `TextStyle` kullanın.

```go
ui.Button{Text: "Saving…", Disabled: saving.Get, OnPress: save}
```

`Disabled` bir işlev alır; böylece düğme bir sinyali izler: devre dışı bir düğme ne basışları ne de klavye odağını alır ve görünümü, uygulamanın seçtiği bir renk değil, devre dışı durumudur.

`Busy` hâlâ süren bir eylem içindir. Doğru (true) döndürdüğü sürece, içeriğin üzerinde düğmenin metin renginde bir yükleme göstergesi görünür; böylece birincil bir düğmenin vurgu rengi üzerinde de görünür, basışlar hiçbir şey yapmaz ve yardımcı teknolojiler düğmenin meşgul olduğunu duyar. Varsayılan olarak içerik, göstergenin altında yerini korur. Düğme klavye odağını korur. `BusyLabel` veya `BusyLabelFunc` göstergenin yanında bir ileti gösterir ve meşgulken bunu düğmenin erişilebilir adı olarak bildirir. Düğme en az durağan genişliğini korur.

```go preview=button/busy
ui.Button{Text: "Save", Busy: saving.Get, BusyLabel: "Saving…", OnPress: save}
```

`ui.Spinner` ve `ui.ProgressRing`, vurgu renklerinin kaybolacağı başka herhangi bir yüzey için kendi `Tint`'lerini alır.

## Bağlantılar

`ui.Link{Text: "Docs", Href: "/docs"}` bağlantı rolüyle bir hedefi açar. Metni ve üzerine gelindiğindeki alt çizgisi, `Style.Color` başka bir renk ayarlamadıkça vurgu rengini kullanır; `Style.Hover.Color` üzerine gelme rengini ayarlar.

## Erişilebilirlik

Düğme `RoleButton` bildirir ve adı olarak metnini kullanır. `Label`, yardımcı teknolojiler için metnin yerini alır. `Label` içermeyen yalnızca simgeli bir düğme okunamaz; bu yüzden onu adlandırın.

Masaüstlerinde, web'de ve Apple TV'de Return ve Space odaklanmış bir düğmeye basar; bir telefonun ekran okuyucusu ona tıklama olarak basar.

## Platform

Düğme bir platform düğmesi değil, her yerde aynı şekilde çizilen bir görünümdür; basışlar, odak ve erişilebilirlik platformdan gelir. Dokunmatik kalıplar yukarıda anlatıldığı gibi daha büyük yerleşim hedefleri kullanır.

## İlgili

- [Formlar](../forms.md): bir formdaki düğmeler ve doğrulama.
- [Stiller ve temalar](../styles.md): bir düğmenin renklerini aldığı tema.
- [Erişilebilirlik](../accessibility.md): roller, etiketler ve odak.
- Galerinin Button sayfası.
- [Stilsiz bileşenler](../authoring/base.md#button): kendi görünümünüze sahip bir düğme için `base.Button`.
