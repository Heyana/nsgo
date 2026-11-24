---
title: Listeler ve kaydırma
description: Yerel hareketlere sahip kaydırma görünümleri ve istenen sayıda öğe için satırları yeniden kullanan listeler.
group: Temel kavramlar
order: 55
---

## Bir kaydırma görünümü

`core.Scroll` platformun kendi kaydırma görünümüdür; bu nedenle fırlatmalar, sekmeler, kaydırma çubuğu, yaslanma ve dokunmatik kaydırma yereldir. Alt öğeleri bir sütunda ya da `Horizontal` ile bir satırda, ihtiyaç duydukları uzunlukta yerleştirilir.

```go
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

Kaydırma görünümü kapsayıcısını doldurur ve dolgu içeriğine uygulanır; böylece kaydırma çubuğu kenara dayalı kalır. Stili aksini söylemedikçe üst öğesine sığacak şekilde küçülür. Bir kardeş öğe küçülürken istenen boyutunu koruması gerektiğinde `NoShrink` ayarlayın. `HideIndicator` platformun göstergesini gizler, `Bar` ise uygulamanın kendi göstergesini çizer.

`Behavior` ve `BehaviorFunc` nasıl kaydıracağını seçer: `SnapToAlignment` ile birlikte `Paging`, `SnapToInterval` ve `SnapToOffsets`, bir `DecelerationRate`, `NoBounces`, `ContentInset` ve klavyenin kuralları. `MinZoomScale` ve `MaxZoomScale` kıstırma hareketlerinin yakınlaştırmasını sağlar, `OnRefresh` ve `Refreshing` aşağı çekmenin yenilemesini sağlar, `OffsetValues` ise onunla birlikte hareket eden efektler için kaydırma konumunu ana iş parçacığında izler.

```go
page := core.Scroll{
	OnScroll: func(e core.ScrollEvent) { y.Set(e.Offset.Y) },
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: []core.Node{...},
}
```

`OnScroll` kaydırma konumunu her turda en fazla bir kez veya her `ScrollEventThrottle` süresinde bir kez bildirir. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` ve `OnScrollEnd` kullanıcının hareketlerinin başını ve sonunu işaretler. `Ref`, `ScrollTo`, `ScrollToEnd`, `ZoomTo` ve bir liste için `ScrollToIndex` ile koddan kaydırır.

## Bir liste

`core.List` uzun verileri yerel bir kaydırma görünümünde işler ve yalnızca ekrana yakın satırları oluşturur. `Count` kaç satır olduğunu söyler ve `Row` bir satırı oluşturur; `Key` her satıra bir kimlik verir, böylece yerinde kalan satırlar üstlerinde satırlar gelip giderken hücrelerini korur.

```go
core.List{
	Count:         func() int { return len(items.Get()) },
	Key:           func(i int) any { return items.Get()[i].ID },
	EstimatedSize: 56,
	Row: func(cx *core.Scope, r core.ListRow) core.Node {
		return core.View{Style: core.Style{Padding: core.All(12)}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return items.Get()[r.Index()].Title }},
		}}
	},
}
```

`Row` yeniden kullanılan her hücre için bir kez çalışır ve o hücre başka bir satır için yeniden kullanıldığında `r.Index()` değerini günceller; bu nedenle satırı oluştururken tek seferlik bir değeri yakalamak yerine `r.Index()` değerini ve öğe sinyallerini reaktif işlevlerin (`ContentFunc`, `StyleFunc`, `NewMemo`) içinde okuyun. Boştaki hücreler park edilir; böylece hesaplamaları, hücre yeniden canlı bir satıra bağlanana kadar bekler.

`Header`, `Footer`, `Empty` ve `Separator` satırların çevresindeki ve aralarındaki içeriği tutar. `NumColumns` satırları yan yana yerleştirir, `Gap` ve `ColumnGap` onları ayırır, `Horizontal` ise yatay kaydırır. `StickyHeaderIndices` bu satırları, bir sonraki onları dışarı itene kadar görüntü alanının başına sabitler; `StickyHeaderOffset` ise üstlerindeki bir başlığın açıkta kalmasını sağlar.

`OnViewableItemsChange` görünümdeki satırları `ViewabilityConfig` eşiklerine göre bildirir ve `OnLoadMore`, görüntü alanı sona yaklaştıkça satır sayısı başına bir kez çalışır. `OnLoadMoreAtStart` aynısını başlangıçta yapar ve `InitialScrollIndex` listeyi bir satırda açar. `Alignment: core.ListAlignEnd` bir listeyi, satırları okuma sırasında olan bir sohbetin yaptığı gibi sonunda tutar: en yeni satırında açılır, kullanıcı sondayken yeni satırları izler ve kısa bir konuşmayı altta tutar. `Inverted` bunun yerine 0. satırı sona koyar ve `MaintainVisibleContentPosition`, kullanıcı yakınındayken büyüyen bir listenin sonunu izler.

```go
core.List{Ref: list, Count: count, EstimatedSize: 64, Row: row, OnLoadMore: loadMore,
	StickyHeaderIndices: headers(cx), Footer: core.View{Style: core.Style{Height: core.Pt(24)}}}
```

`list.ScrollToIndexAt(index, 0.5, 0, true)` bir satırı görüntü alanının ortasına, `list.ScrollToEnd(true)` ise sona kaydırır. `list.ListPosition()` bir okuma konumunu korumak için görüntü alanının nerede başladığını, bir satırı ve onun ne kadar içine girildiğini okur: `list.ScrollToIndexAt(p.Index, 0, -p.Offset, false)` yeniden oluşturulan bir listeyi oraya geri getirir. Bir liste ayrıca başka içerikle birlikte tek bir sayfada da yer alabilir: `Viewport: core.ListViewportParent` ayarlayın; çevresindeki kaydırma görünümü listeyi geri kalanla birlikte kaydırır.

## Tablolar

`base/datatable`; sanallaştırılmış satır ve sütunlar, yapışkan bir başlık, diğerleri altlarından kayarken solda (`Fixed`) veya sağda (`FixedRight`) sabitlenen sütunlar, sıralama, tek veya çok satırlı seçim, sütunları yeniden boyutlandırma ve taşıma ile uygulamanın kendi hücreleriyle bir milyon satırlık bir tablo gösterir. `MultiSelect`, Command veya Control ile açıp kapatmayı, Shift ile aralıkları ve Shift'li klavye genişletmelerini destekler; `SelectedRows` ve `OnSelectedRowsChange` ise satırları uygulamanın denetlemesini sağlar. `CellTooltip` bir hücreye, hücreyi ekran okuyuculara da tanımlayan ve `Styles.CellTooltip` ile stillenen bir araç ipucu verir; `RenderCell` öğesinin çizdiği şey işaretçi almaz, bu yüzden bir hücrenin içindeki `ui.Tooltip` asla görünmez. `base.Table` daha küçük veriler için uygulamanın satırlarından oluşan sade bir tablodur ve `base/plot` grafik çizer.

Yerel ayarı veya uygulama durumunu izleyen bir tablo adı için `DataTable.LabelFunc` kullanın. `ColumnGroup.LabelFunc`, grup görünümde kaldığı sürece gruplanmış bir sütun başlığını günceller.

## Sonraki

- [Animasyon](animation.md): bir kaydırma görünümünü izleyen efektler.
- [Metin](text.md): alt satıra kayan metin satırları.
- [Gezinme](navigation.md): içeriği kayan ekranlar.
