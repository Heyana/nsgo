---
title: Yerleşim
description: Güvenli alan ve klavye yerleşime dahil edilmiş halde flexbox, grid, boyutlar ve konumlandırma.
group: Temel kavramlar
order: 51
---

## Kutular

Her öğe, motor tarafından yerleştirilen bir kutudur. Varsayılanlar React Native'inkilerdir: sütun yönlü bir flexbox, büyümedikçe veya küçülmedikçe boyutunu koruyan alt öğeler, çapraz eksen boyunca gerilen öğeler ve dolgu ile kenarlıkları içeren boyutlar.

```go
core.View{
	Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.All(16), AlignItems: core.AlignCenter},
	Children: []core.Node{
		core.Text{Content: "One"},
		core.Text{Content: "Two"},
	},
}
```

`Column`, `Row`, `ColumnReverse` ve `RowReverse` ana ekseni belirler; `Wrap` ve `WrapReverse` alt öğelerin birden fazla satıra akmasını sağlar. `JustifyContent` onları bu eksen boyunca, `AlignItems` veya tek bir alt öğede `AlignSelf` ise onun karşısında yerleştirir. `Gap`, `RowGap` ve `ColumnGap` onları birbirinden ayırır; `style` dolgusu ve kenar boşluğu ise `core.All`, `core.Symmetric` veya `core.Edges` kullanır.

## Boyutlar

`Width`, `Height`, `MinWidth`, `MaxWidth` ve `FlexBasis`, nokta için `core.Pt` veya üst öğenin yüzdesi için `core.Pct` alır. `FlexGrow` boş alanı paylaştırır, `FlexShrink` ise bir kutunun bir kısmını geri vermesine izin verir; ikisi de olmayan bir kutu boyutunu korur. `AspectRatio` genişliğin yüksekliğe bölümüdür. `AutoMargin` adlandırdığı kenarlardaki boş alanı alır; böylece bir kenar, kutuyu diğer kenara iter.

```go
core.View{Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{
	core.View{Style: core.Style{Width: core.Pt(220), FlexShrink: 0}},
	core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
}}
```

Bir genişlik veya yükseklik aksini söyleyene kadar kutunun boyutunu içeriği belirler. `core.MinContent`, `core.MaxContent` ve `core.FitContent`, CSS'in adlandırdığı boyutları alır: bir metnin min-content genişliği en geniş sözcüğüdür. Bir satır içindeki metin yalnızca `FlexShrink: 1` ile küçülür ve kaydırılır; kalan alanı alması gerektiğinde `FlexGrow: 1` ekleyin. Ayarlanmamış bir minimum, React Native'de olduğu gibi zaten 0'dır; bu nedenle `MinWidth: core.Pt(0)` hiçbir şeyi değiştirmez ve küçülen bir metin, satır en geniş sözcüğünden dar olduğunda sözcüklerini böler. `MinWidth: core.AutoMinimum`, CSS'in otomatik minimumunun yaptığı gibi onu en geniş sözcüğünde durdurur.

Pencereyi dolduran bir ekran, `FlexGrow: 1` ve `FlexShrink: 1` alır; onunla kaydırılması gereken bir liste veya kaydırma görünümü arasındaki her kutu da öyle. Yalnızca `FlexGrow` ile bir kutu içeriği kadar büyür; bu yüzden uzun bir liste kaydırmak yerine onu pencerenin ötesine uzatır.

## Grid

`DisplayGrid`, alt öğeleri CSS grid ile yerleştirir. `GridTemplateColumns` ve `GridTemplateRows` CSS iz sözdizimini alır; `GridRow` ve `GridColumn` ise bir öğeyi çizgiye veya açıklığa (span) göre yerleştirir.

```go
core.View{Style: core.Style{
	Display: core.DisplayGrid,
	GridTemplateColumns: "repeat(3, 1fr)",
	GridAutoRows: "44px",
	Gap: 12,
}, Children: []core.Node{
	core.View{Style: core.Style{GridColumn: core.GridLines{Start: core.GridSpan(2)}}},
}}
```

`repeat(auto-fill, minmax(160px, 1fr))` sığabildiği kadar sütun oluşturur. `GridFlow` grid'in hangi sırayla doldurulacağını seçer; `JustifyItems`, `JustifySelf` ve `AlignItems` ise öğeleri hücrelerine yerleştirir.

## Konum

`Relative` varsayılandır ve bir kutuyu akışta tutar; `Top`, `Right`, `Bottom` ve `Left` onu kaydırır. `Absolute` onu akıştan çıkarır ve konumlandırılmış en yakın atasına yerleştirir. `Fixed` onu, neyin kaydırıldığından bağımsız olarak kök görünüme veya onun modal penceresine yerleştirir; `Sticky` ise onu en yakın kaydırma görünümünde üst öğesinin içine sabitler: `Top` ile üstten ya da `Top` otomatik olduğunda `Bottom` ile alttan (yatayda `Left` ve `Right`). `Position: core.Sticky, Bottom: core.Pt(0)` olan bir alt bilgi, uzun içerik kaydırılırken altta kalır ve kısa içerikten sonra akar. `ZIndex` kutuları kardeşleri arasında üst üste dizer.

```go
core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Right: core.Pt(8)}}
```

`Static` bir kutuyu akışta tutar ve iç kaydırmalarını yok sayar; böylece mutlak konumlandırılmış alt öğeler onun üstündeki bir atayı bulur. `Overflow` alt öğeleri bir kutuya kırpar ve `DisplayNone` bir kutuyu yerleşimden çıkarır.

Kutunun kendi dış gölgeleri ve dış çizgisi, CSS'te olduğu gibi kırpma alanının dışında kalır; bu Android, Windows, Linux, web ve Apple'da geçerlidir; Apple'da farklı yarıçaplı köşeler ve yakınlaştırılmış kaydırma içeriği dahil her görünüm türü için. Bu fiziksel bir iPhone'da doğrulanana kadar `ui.Dialog`'un `Bare` öğesi kırpmayı hâlâ aynı köşelere sahip, kenarlık kadar içeri çekilmiş bir iç görünüme koyar; bu da onları her hedefte korur:

```go
core.View{Style: core.Style{CornerRadius: 16, Shadows: []core.BoxShadow{ui.Shadow(0, 8, 24, -6, ui.Black(0.18))}},
	Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, CornerRadius: 16, Overflow: core.Hidden}, Children: content}}}
```

## Yön ve ortam

`LayoutDirection`, uygulamanın yönünün üzerine, bir alt ağacı soldan sağa veya sağdan sola yerleştirir: `DirectionRTL` ve `DirectionLTR` onu ayarlar, varsayılan ise devralır. Kenar boşluklarında, dolgularda, kenarlıklarda ve iç kaydırmalarda sol ve sağ yönle birlikte yer değiştirir; böylece React Native'de olduğu gibi başlangıç ve bitiş olurlar. `PhysicalEdges` ve `PhysicalBorders` adlandırılmış kenarları oldukları yerde tutar. `WritingDirection`, karışık yönlü metin parçalarını sıralar ve `WritingAuto` yönü ilk güçlü harften alır.

`PaddingEnv` ve `MarginEnv` bir kenara ortamı ekler. `core.EnvSafeArea` o kenardaki güvenli alanın iç boşluğudur ve `core.EnvKeyboard` yazılım klavyesinin yüksekliğidir; böylece içerik klavyenin üstünde kalır ve hareketi klavyeyle birlikte canlandırılır.

```go
core.View{Style: core.Style{
	Padding: core.All(16),
	PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea},
}}
```

`core.EnvAll(core.EnvSafeArea)` her kenarı adlandırır; `cx.Env().SafeArea()` ve `cx.Env().KeyboardHeight()` ise değerleri kodda okur.

Katlanabilir veya çift ekranlı bir telefonda `cx.Env().Geometry()`, bir katlama veya menteşenin pencereyi böldüğü bölgeleri, her birinin içine düşen güvenli alanla birlikte, ve aralarındaki katlamaları listeler; `State` ve `Angle` değerleri, kapak ekranında veya düz açılmış bir ekranda olduğu gibi pencereyi hiçbir katlama kesmediğinde bile cihazın ne kadar açık olduğunu söyler (`core.HingeClosed`, `HingeHalfOpen` veya `HingeOpen`). Diğer cihazlar tek bir bölge ve `HingeUnknown` bildirir. iOS 27.1'de `cx.Env().VerticalBarEdge()`, sistemin dikey bir çubuk için seçtiği kenarı bildirir (`core.VerticalBarLeading`, `core.VerticalBarTrailing` veya `core.VerticalBarUnspecified`); `Edge` değeri `mobile.TabBarAutomatic` olduğunda `mobile.TabBar` ve `ui.TabShell` buna uyar. Hiçbir şey geometriye göre örtük olarak yerleştirilmez: uygulama kendi bölmelerini kendisi seçer.

## Duyarlı sınıflar

`Class`, Tailwind CSS sınıflarını alır: `flex`, `flex-col`, `flex-wrap`, `items-center`, `justify-between`, `gap-3`, `p-4`, `w-full`, `grid`, `grid-cols-3`, `col-span-2` ve diğerleri. Kesme noktaları pencereyi izler; ekranın görünümü, yön ve azaltılmış hareket ise ortamı izler:

```go
core.View{Class: "flex flex-col gap-3 p-4 md:flex-row md:gap-6 dark:bg-zinc-900 rtl:flex-row-reverse"}
```

`sm:`, `md:`, `lg:`, `xl:` ve `2xl:` sırasıyla 40, 48, 64, 80 ve 96 rem genişlikten itibaren uygulanır; `max-md:` birinin altında uygulanır. `portrait:`, `dark:`, `rtl:`, `motion-reduce:`, `contrast-more:`, `pointer-fine:`, `pointer-coarse:`, `ios:`, `android:` ve `web:` ortamın söylediğini izler. Sınıflar bir `Style` altında birleşir ve bir durumun sınıfı (`hover:`) bir durum stiline dönüşür; böylece ikisi karıştırılabilir.

## Sonraki adımlar

- [Stiller ve temalar](styles.md): renkler, köşeler, gölgeler ve bir alt ağacın görünümü.
- [Metin](text.md): metnin boyutunu nasıl aldığı ve nasıl kaydırıldığı.
- [Listeler ve kaydırma](lists.md): kaydırılan içerik ve yeniden kullanılan satırlar.
