---
title: Görünümler ve temalar
description: Yürürlükteki temadan renkler alın ve bir bileşene varyantlar ile boyutlar verin.
group: Bileşen yazma
order: 72
---

Stilli bir bileşen yalnızca stilleri seçer ve başka hiçbir şey yapmaz: `chipsui` gibi kendi paketinde stilsiz bir bileşeni sarmalar ve davranışını orada tutar. Renkleri temadan gelir; böylece uygulamanın temasını, bir panelin kendi temasını ve açık ya da koyu görünümü yeniden oluşturulmaya gerek kalmadan izler.

## Temadan gelen renkler

Renkleri bir `StyleFunc` içinde okuyun: okuduğu değer değiştiğinde yeniden çalışır; böylece bir tema veya görünüm değişikliği bileşeni yerinde yeniden stillendirir. `cx.Palette()` bileşenin bulunduğu yerde gösterilen paleti verir ve `ui.ColorsOf(cx)` ise `ui`'nin kullandığı rolleri adlandırır.

```go
// Toggle is chips.Toggle in the theme's colors.
type Toggle struct {
	On       func() bool
	OnChange func(bool)
	Label    string
}

func (t Toggle) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	on := func() bool { return t.On != nil && t.On() }
	return chips.Toggle{On: t.On, OnChange: t.OnChange, Label: t.Label,
		Class: "flex-row items-center gap-2 rounded-full px-3 py-1",
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.Accent()}
			}
			return core.Style{Background: colors.Inset()}
		},
		DotClass: "size-2 rounded-full",
		DotStyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.OnAccent()}
			}
			return core.Style{Background: colors.Faint()}
		},
		Children: []core.Node{ui.Text(t.Label, 13, ui.Medium, colors.Text)},
	}
}
```

Sabit kodlanmış (hard-coded) renkler koyu modda ve uygulamanın kendi teması altında bozulur; her rengi bir rolden alın.

## Kendi rolleriniz

Paletin bir rolü olmadığı renk, her iki görünümde de ayarlanarak sizin belirlediğiniz bir ad altında `Palette.Colors` içine konur. Uygulamalar bunu kodda veya tema dosyalarında `light` ve `dark` altında geçersiz kılar:

```go
theme := core.DefaultTheme()
theme.Light.SetColor("chip", core.RGB(0xeef5ff))
theme.Dark.SetColor("chip", core.RGB(0x1b2a3d))
cx.SetTheme(theme)
```

Bir bileşen bunu `cx.Palette().Color("chip")` ile okur; bu yöntem temada rengin bulunup bulunmadığını bildirir, böylece bileşen bulunmadığında başka bir role geri dönebilir. Gradyanlar, gölgeler ve diğer bitiş efektleri renk değildir: bunları `cx.Dark()` ile seçerek kodda tutun.

## Varyantlar

Birden fazla görünüme sahip bir bileşen, sıfır değeri varsayılan görünüm olan kendine ait bir `Variant` türü alır. Mantıksal alanlar veya paylaşılan bir renk türü yerine, rengi de dahil olmak üzere yalnızca bileşenin çizdiği görünümleri adlandırır:

```go
// ToggleVariant is a toggle's look.
type ToggleVariant uint8

const (
	// ToggleSolid fills the pill with the accent when on.
	ToggleSolid ToggleVariant = iota
	// ToggleOutline draws its edge in the accent instead.
	ToggleOutline
)
```

## Boyutlar

Çoğu bileşen yalnızca tek bir boyuta ihtiyaç duyar; bu da uygulamanın seçim yapmasına gerek kalmadan denetimlerin hizalı kalmasını sağlar. Daha fazlasına ihtiyaç duyan bir bileşen, `ui.Button`'ın 28, 24 ve 32 puntoluk boyutlarıyla eşleşmesi için varsayılan, küçük veya büyük değerlerinden oluşan bir `ui.Size` türündeki `Size`'ı alır; `Size.Height()` diğer denetimlerin hizalanacağı yüksekliği verir. Avatar gibi her ölçekte tek bir biçim olan bir bileşen ise `Size`'ı punto cinsinden alır.
