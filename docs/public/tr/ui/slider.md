---
title: Kaydırıcı
description: Bir iz boyunca klavye ve işaretçiyle seçilen tek bir değer veya bir aralık.
group: Bileşenler
order: 37
---

`ui.Slider` bir iz boyunca sürükleme, adım ve klavyeyle bir değer seçer. Tek bir değer tek bir tutamaçtır; `High` ise iki tutamaçlı bir aralık oluşturur.

```go preview=slider/basic
volume := core.NewSignal(cx, 40.0)
ui.Slider{Value: volume.Get, OnChange: volume.Set, Min: 0, Max: 100, Step: 5, Label: "Volume"}
```

## Bir aralık

İki değer seçmek için `High` ve `OnHighChange` ayarlayın. `Value` alt uçtur. `LowLabel` ve `HighLabel` yardımcı teknolojiler için tutamaçları adlandırır.

```go preview=slider/range
price := core.NewSignal(cx, 20.0)
ceiling := core.NewSignal(cx, 80.0)
ui.Slider{
	Value: price.Get, OnChange: price.Set,
	High: ceiling.Get, OnHighChange: ceiling.Set,
	Min: 0, Max: 100, Step: 1,
	Label: "Price", LowLabel: "Minimum", HighLabel: "Maximum",
}
```

## Adımlar ve değerler

`Step` değeri adımlara oturtur; 0 adımı herhangi bir değeri kabul eder. `ValueText` yardımcı teknolojilerin değer olarak okuduğu metni değiştirir, `Disabled` ise kaydırıcıyı devre dışı bırakır.

```go preview=slider/format
ui.Slider{Value: speed.Get, OnChange: speed.Set, Min: 0.5, Max: 2, Step: 0.25, Label: "Speed",
	ValueText: func(v float64) string { return fmt.Sprintf("%g×", v) }}
```

Aralık veya adım değiştiğinde, örneğin bir video süresini bildirdiğinde, `MinFunc`, `MaxFunc` ve `StepFunc` kullanın. Bunlar statik alanlarının yerine geçer ve bağlı kaydırıcıyı odağını ve süren sürüklemeyi koruyarak günceller. 0 döndüren açık bir `MaxFunc`, süre gelene kadar daraltılmış bir aralığa izin verir; uygulamanın değeri korunur ve geçerli sınırlar içinde gösterilir.

```go
ui.Slider{Value: position.Get, OnChange: position.Set, MaxFunc: duration.Get, Label: "Position"}
```

TSX'te `min={minimum()}`, `max={duration()}` ve `step={step()}` sinyallerini yerinde izler; `max={0}`, 0 döndüren bir `MaxFunc` gibi aralığı daraltır.

## Yön

`Axis: base.Vertical` kaydırıcıyı dikey hale getirir ve en küçük değeri alta koyar; ona `Style` içinde bir yükseklik verin. `Inverted` en küçük değeri öbür uca koyar: dikey bir kaydırıcının üstüne, yatay bir kaydırıcının sonuna. İz boyunca ok tuşları bu yönü izler. `ui.Progress` aynı `Axis` ve `Inverted` alanlarını kabul eder.

```go
ui.Slider{Value: level.Get, OnChange: level.Set, Axis: base.Vertical, Label: "Level", Style: core.Style{Height: core.Pt(160)}}
```

## Davranış

Sürükleme işaretçiyi izler ve hareket ederken `OnChange` bildirir. Ok tuşları odaktaki tutamacı bir adım kaydırır, Home ve End uçlara gider, Page Up ve Page Down ise daha uzağa taşır. `Step` 0'ın üzerindeyken `ui.Slider` izin altında adımları en fazla 60 işaretle gösterir.

## Erişilebilirlik

Kaydırıcı `RoleAdjustable` rolünü, `Label` değerini ve değerini bildirir; bir aralığın iki tutamacı da kendi etiketleriyle bulunur. Sayı yerine `ValueText` okunur. Klavye ve ekran okuyucunun artırma ve azaltma eylemleri değeri değiştirir.

## Platform

İz uygulama tarafından çizilir ve her yerde aynı boyuttadır. Basmalar, odak ve ekran okuyucu platformundur; dokunarak sürükleme, aynı adımla işaretçi sürüklemesi gibi davranır.

## İlgili

- [Formlar](../forms.md): bir formdaki kaydırıcılar.
- [Erişilebilirlik](../accessibility.md): değerler ve ayarlanabilir roller.
- Galerinin Slider, RangeSlider ve Rating sayfaları.
- [Stilsiz bileşenler](../authoring/base.md#slider): dikey ve kendi işaretlerinizle `base.Slider`.
