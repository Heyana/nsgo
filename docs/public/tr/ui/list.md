---
title: Liste
description: Yalnızca görünürken oluşturulan, seçim ve etkinleştirme içeren, herhangi sayıda satır.
group: Bileşenler
order: 38
---

`ui.List`, satırları yerel bir kaydırma görünümü üzerinde işler ve seçim, etkinleştirme ve klavye desteğiyle yalnızca ekrana yakın satırları oluşturur.

```go preview=list/basic height=280
ui.List{
	Count:         func() int { return len(items) },
	Key:           func(i int) any { return items[i].ID },
	EstimatedSize: 44,
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		item := items[row.Index()]
		return ui.Text(item.Title, 13, ui.Regular, colors.Text)
	},
}
```

## Satırlar ve kimlik

`Count` kaç satır olduğunu söyler, `Row` ise bir satır oluşturur. `Key` bir satıra kimliğini verir: yerinde kalan satırlar, üstlerinde satırlar gelip giderken hücrelerini ve durumlarını korur. Satırlar eklenebiliyor, kaldırılabiliyor veya yeniden sıralanabiliyorsa dizin yerine bir kimlik (ID) kullanın; dizin, hücre başka bir satırı gösterirken onu izleyen bir sinyaldir.

```go
ui.List{Count: count, Key: func(i int) any { return items[i].ID }, Row: func(cx *core.Scope, row core.ListRow) core.Node {
	return ui.Text(items[row.Index()].Title, 13, ui.Regular, colors.Text)
}}
```

`EstimatedSize` bir satırın beklenen yüksekliğidir; 0 olduğunda 50 noktadır. Satırların yükseklikleri farklı olabilir; liste oluşturduklarını ölçer ve geri kalanını tahmin eder.

## Yerleşim

`Header`, `Footer` ve `Empty`, satırların çevresindeki ve hiç satır yokken gösterilen içeriği tutar. `Gap` satırları ayırır, `Columns` onları yan yana dizer ve listenin dolgusu içeriğin dolgusudur; böylece kaydırma çubuğu kenarda kalır. Listeye bir yükseklik verin veya üst öğesini doldurmasına izin verin.

## Seçim ve etkinleştirme

`ui.List`, `ui.NewSelection(cx, multiple)` ile oluşturulan bir `Selection` alır. Anahtarlar, etiketler, etkinleştirme ve klavye onu izler: Up, Down, Home ve End imleci taşır, Shift genişletir, Mod+A tümünü seçer, Space çoklu seçimde açıp kapatır, Enter veya çift tıklama ise bir satırı etkinleştirir.

```go preview=list/selection height=320
selected := ui.NewSelection(cx, true)
ui.List{
	Count: count, Key: func(i int) any { return items[i].ID },
	Selection: selected,
	OnActivate: func(i int) { open(items[i]) },
	RowLabel:   func(i int) string { return items[i].Title },
	Row: func(cx *core.Scope, row core.ListRow) core.Node { return rowContent(cx, items[row.Index()]) },
}
```

`OnSelectionChange`, bir basış, bir tuş veya önden yazma (typeahead) hangi satırların seçildiğini değiştirdikten sonra çalışır; uygulama `Selection`'ı kendisi ayarladığında çalışmaz. Böylece bir ayrıntı bölmesi kullanıcının seçimini izleyebilirken uygulama seçimi başka bir yoldan açılmış bir öğeye taşıyabilir.

`TapSelects`, bir telefonda dokunmanın etkinleştirmek yerine seçmesini sağlar. `RowLabel`, yardımcı teknolojilerin okuduğu ve önden yazmanın eşleştirdiği şeydir.

## Yükleme ve kaydırma

`OnLoadMore`, sona yaklaşıldıkça her satır sayısı için bir kez çalışır. `Ref`, `ScrollToIndexAt`, `ScrollToEnd` ve `ScrollToIndex` ile kaydırır. Altındaki düz liste olan `core.List`, düşük düzeyli durumlar için ayrıca `OnLoadMoreAtStart`, bir sohbet için `Inverted`, `StickyHeaderIndices`, `MaintainVisibleContentPosition`, `Viewport` ve `ViewabilityConfig` sunar ([Listeler ve kaydırma](../lists.md)).

## Erişilebilirlik

Liste, `Label` ile adlandırılan tek bir liste veya liste kutusudur (listbox); bir satırın adı, metni veya `RowLabel`'ıdır. Seçim yapılabilen bir liste imlecin bulunduğu satırı bildirir ve klavye hareket ettikçe onu duyurur. Görünümün dışına çıkan bir ekran okuyucu listeden oraya kaydırmasını ister; bu da ulaştığı satırları oluşturur.

## Platform

Yerel kaydırma görünümü ve sanallaştırma her hedefte aynıdır. Satırlar yeniden kullanılır, bu yüzden bir satırın durumu `Key`'ini izlemelidir; oluşturma durumunu bir satırda değil uygulamada tutmak olağan kuraldır.

## İlgili

- [Listeler ve kaydırma](../lists.md): yapışkan başlıklar ve daha fazla yükleme dahil tam kılavuz.
- [Kaydırma](scroll.md): düz bir kaydırma görünümü.
- [DataTable](../lists.md#tablolar): çok sayıda satır ve sütundan oluşan bir tablo.
- Galerinin List, Table ve DataTable sayfaları.
