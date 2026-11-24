---
title: Kaydırma
description: Sayfalama, yaslama, yakınlaştırma ve yenileme ile platformun kaydırma görünümü.
group: Bileşenler
order: 39
---

`core.Scroll`, platformun kendi kaydırma görünümüdür. Alt öğeleri bir sütunda ya da `Horizontal` ile bir satırda dizilir ve içerik kayarken kabını doldurur.

```go preview=scroll/basic height=240
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

Dolgu kaydırma görünümüne değil içeriğe verilir; böylece kaydırma çubuğu kenara yaslı kalır. Kaydırma görünümü, stili aksini söylemedikçe üst öğesine sığacak şekilde küçülür. Bir kardeş öğe küçülürken istenen boyutunu koruması gerekiyorsa `NoShrink` ayarlayın.

## Davranış

`Behavior` nasıl kaydırılacağını seçer: `Paging`, `SnapToAlignment` ile `SnapToInterval` veya `SnapToOffsets`, bir `DecelerationRate`, `NoBounces`, `ContentInset` ve klavyenin kuralları.

```go preview=scroll/paging height=240
core.Scroll{
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: pages,
}
```

`HideIndicator` platformun kaydırma çubuğunu gizler, `Bar` ise uygulamanın kendi çubuğunu çizer. `MinZoomScale` ve `MaxZoomScale` sıkıştırma hareketlerinin yakınlaştırmasına izin verir; `NoPinchZoom` bunları yok sayar. `CenterContent`, görüntü alanından küçük içeriği ortalar.

## Yenileme ve olaylar

`OnRefresh` görünümü yenilenebilir yapar; iş sürerken `Refreshing` ayarlayın. `PullValue`, uygulamanın kendi göstergesi için içeriğin ne kadar çekildiğini izler, `Behavior.RefreshIndicator` ise onun yüksekliğini ayarlar.

```go
refreshing := core.NewSignal(cx, false)
core.Scroll{
	Refreshing: refreshing.Get,
	OnRefresh:  func() { refreshing.Set(true); reload(cx) },
	Children:   content,
}
```

`OnScroll` kaydırma konumunu her turda en fazla bir kez veya her `ScrollEventThrottle` için bir kez bildirir; `Stale`, platformun koddan gelen bir kaydırmayı uygulamasından önceki bir bildirimi işaretler. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` ve `OnScrollEnd` kullanıcının hareketlerini çevreler.

`OffsetValues`, her kare için bir commit olmadan daralan bir başlık için kaydırma konumunu ana iş parçacığında izler. `Ref`, koddan `ScrollTo`, `ScrollToEnd` ve `ZoomTo` ile kaydırır; içerideki herhangi bir görünümün ref'i ise `Reveal` ile onu görünür hale getirir ve çevresindeki her kaydırma görünümünü kaydırır:

```go
row.Reveal(core.RevealOptions{Vertical: core.ScrollAlignCenter, Animated: true})
```

`RevealBox` bunun yerine görünüm içindeki bir kutuyu, örneğin bir metnin `TextRangeBoxes` değerinden bir sözcüğün kutusunu görünür hale getirir; `Margin` ise üzerindeki bir çubuk için en dıştaki kaydırma görünümünün kenarlarında boş alan bırakır.

Klavyenin odağı taşıdığı bir görünüm kendiliğinden görünür alana kayar.

## Yerleşim

`Horizontal` yatay olarak kaydırır. `Behavior.ContentInset`, içeriğin çevresine onunla birlikte kayan boşluk koyar. Güvenli alanı dolduran bir sayfa için `PaddingEnv`'i kaydırma görünümünde değil, içerikte kullanın.

## Erişilebilirlik

`Label` kaydırma alanını adlandırır, `Role` ise bir liste kutusu gibi ne olduğunu söyler. Platformun kaydırma hareketleri, klavyeyle kaydırma ve ekran okuyucuyla kaydırma korunur; `Busy` hâlâ yüklenmekte olan bir görünümü bildirir.

## Platform

Her arka uç platformun kaydırma görünümünü kullanır; bu yüzden fırlatmalar, sekmeler, kaydırma çubukları ve dokunarak kaydırma yereldir. Kenar efektleri farklıdır: macOS ve iOS lastik bant gibi esner, Android gerilir, Windows ve Linux durur, web ise tarayıcıyı izler. `EdgeEffects` ve `EdgeBar`, uygulamanın istediği yerlerde içeriğin üzerine çubuklar ekler. iOS ve Android telefon ve tabletlerde taşan dikey ve yatay kaydırma görünümleri, etkin eksen boyunca içeriğin tamamında gezinen, dokunarak sürüklenebilen bir kaydırma tutamacı gösterir; taşmadan sığan kısa kaydırma görünümleri, yakınlaştırma yapan görünümler, `Behavior.NoScroll`, `HideIndicator` ve özel bir `Bar` veya `base.SetScrollbarTheme` kullanan görünümler onu gizler. Testlerde `nsgotest.App.ScrollHandle` onun görüntü alanı sınırlarını bildirir, `DragAt` veya `Touch` ise onu doğrudan sürükler. Windows'ta kaplama göstergesi tekerlek, dokunma, çubuk sürükleme ve bunların momentumu için görünür; uygulamanın kendi yaptığı `ScrollTo`, `ScrollBy`, görünür hale getirmeler, iç boşluklar veya animasyonlar için görünmez.

## İlgili

- [Listeler ve kaydırma](../lists.md): kaydırma ve listeler için tam kılavuz.
- [Liste](list.md): yeniden kullanılan satırlar.
- [Yerleşim](../layout.md): dolgular ve güvenli alan.
- Galerinin Scroll ve Lists sayfaları.
