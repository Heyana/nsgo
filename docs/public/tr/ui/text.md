---
title: Metin
description: Paragraflar, parçalar, bağlantılar, yazı tipleri ve belgeler.
group: Bileşenler
order: 32
---

`core.Text`, platformun metin motoruyla dizilen tek bir paragraftır. `ui.Text` ve `ui.Textf`, `ui`'ın yazı stilinde bir satır için yardımcılardır; öğe yine bir `core.Text`'tir, bu yüzden `Style` ve `Spans` alanlarını da kabul eder.

```go preview=text/basic
ui.Text("Settings", 20, ui.Bold, colors.Text)
ui.Textf(func() string { return fmt.Sprintf("%d files", n.Get()) }, 13, ui.Regular, colors.Muted)
```

## İçerik

`Content` sabittir, `ContentFunc` ise bir sinyali izler. `MaxLines` satırları sınırlar ve `EllipsizeMode` geri kalanı başından, ortasından veya sonundan keser; `Style.WhiteSpaceNoWrap` bir paragrafı tek satırda tutar. `TextAlign`, `TextTransform`, `LineHeight` ve `LetterSpacing` paragrafı stillendirir, `OnTextLayout` ise yerleşimden sonra satırlarını bildirir.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) },
	MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 15}}
```

## Parçalar ve bağlantılar

`Spans`, her biri kendi stiline sahip parçalardan bir metin oluşturur. `OnPress` içeren bir parça bir bağlantıdır; `Background` ve `CornerRadius` alanları her satırda karakterlerinin arkasına çizilir. `SpansFunc`, okuduğu şey değiştiğinde parçaları yeniden verir; vurgulanmış kod veya değişen bir cümle için.

```go preview=text/spans
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "Read the "},
	{Content: "guide", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openGuide},
	{Content: " before you start."},
}}
```

`Attachment`, parçanın içeriği yerine satıra bir görünüm koyar: bir simge, bir görüntü veya küçük bir düğme. Kendi boyutunda yerleştirilir, sözcüklerle birlikte satır kaydırır ve kendi basmalarını alır.

## Yazı tipleri ve stil

`FontFamily` bir aileyi veya bir CSS listesini adlandırır. `FontSize`, `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LetterSpacing`, `TextDecoration` ve `TextShadowColor` ile ofseti ve yarıçapı metni stillendirir. `core.FontMonospace` platformun sabit genişlikli ailesidir.

```go
core.Text{Content: "fn main() {}", Style: core.Style{FontFamily: core.FontMonospace, FontSize: 13}}
```

Paketlenmiş yazı tiplerini ilk görünümden önce `nsgo.Fonts` ile kaydedin veya uygulama çalışırken bir yazı yüzünü `cx.RegisterFont` ile kaydedin. Bir yazı tipi gelmeden önce hazırlanan metin yeniden hazırlanır.

## Belgeler

`base/textview.TextView`, Markdown veya HTML'yi aynı temel öğelerle gösterir: başlıklar, paragraflar, listeler, tablolar, kod, dipnotlar, uyarılar, görüntüler, formüller ve bağlantılar. `textview.Parse` Markdown'ı, `textview.ParseHTML` HTML'yi okur ve `Options.Streaming` hâlâ gelmekte olan bir belgeyi onarır.

```go preview=text/document height=240
textview.TextView{Markdown: "## Notes\n\nA **native** document.", Styles: ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) { open(e.URL) }}
```

`textview.NewSearch`, bir `TextView`'un gösterdiği metinde arama yapar ve eşleşmeleri vurgular. Birkaç görünüme `textview.ProvideSearch` ile aynı aramayı verin, ardından `Open`, `SetQuery`, `Next` ve `Previous` çağırın; `Count` ve `Selected` sonucu bildirir. Arama parçaları `textview.PartSearch` ve `textview.PartSearchSelected` ile stillendirilir.

## Seçim ve erişilebilirlik

`SelectableOn`, kullanıcının bir metni seçip kopyalamasına izin verir. `Selectable` içeren bir `core.View`, metinlerinin belge sırasına göre birlikte seçilmesini sağlar. `RoleHeader` gibi bir `Role`, yardımcı teknolojilere metnin ne olduğunu söyler; sözcükler etiket değilse `Label` onun yerini alır.

## Platform

Metni platformun metin motoru yerleştirir, bu yüzden satır sonları, yazı tipleri ve giriş yöntemleri onu izler. CJK satır bölme, emoji ve sağdan sola metin sistemi izler. Uygulama metin boyutunu `cx.Env().FontScale()` ile okuyabilir ve `NoFontScaling` ayarlanmadıkça metin onu izler.

## İlgili

- [Metin](../text.md): metin, parçalar, yazı tipleri ve belgeler için kapsamlı kılavuz.
- [Görüntüler ve simgeler](../images.md): satır içi görüntüler ve simgeler.
- [Erişilebilirlik](../accessibility.md): başlıklar, etiketler ve seçim.
- Galerinin Text ve TextView sayfaları.
