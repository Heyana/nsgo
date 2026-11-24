---
title: Sekmeler
description: Bir sekme listesi, bir bölümlü denetim ve sistem sekme çubuğu.
group: Bileşenler
order: 40
---

`ui.Tabs[T]` birkaç değerden birini bir bölümlü denetim (segmented control) veya bir sekme listesi olarak seçer; uygulama seçilen değerin içeriğini yanında gösterir. Bir sekme listesi tek bir Tab durağıdır: oklar sekmeler arasında dolaşır.

```go preview=tabs/basic
section := core.NewSignal(cx, "overview")
ui.Tabs[string]{
	Options: []ui.Option[string]{
		{Value: "overview", Label: "Overview"},
		{Value: "activity", Label: "Activity"},
		{Value: "settings", Label: "Settings"},
	},
	Value:    section.Get,
	OnChange: section.Set,
	Label:    "Sections",
}
```

## Varyantlar ve boyutlar

`Variant`, `TabsSegmented` (varsayılan; çukur bir iz içinde kabarık bir başparmak) veya `TabsUnderline` (seçili sekmenin altında bir vurgu çizgisi) olur. Alt çizgi `IndicatorColor` ayarlayabilir ve `IndicatorFit: TabsIndicatorContent` ile sekmenin tamamı yerine etiketini ve simgesini izleyebilir; TSX'te bunlar `indicatorColor` ve `indicatorFit="content"`'tir. `Size`, `SizeDefault`, `SizeSmall` veya `SizeLarge` olur; `Equal` ise her sekmeye etiketine uydurmak yerine aynı genişliği verir.

```go preview=tabs/underline
ui.Tabs[string]{Variant: ui.TabsUnderline, Equal: true, Size: ui.SizeSmall, Options: tabs, Value: section.Get, OnChange: section.Set}
```

Bir seçenek bir `Icon`, bir `Disabled` işlevi, bir `Content` düğümü ve bir `TestID` taşıyabilir. `Content` etiketin yerini alır ve sekmenin metin stilini devralır.

Sekmeler bir sinyali izlediğinde `OptionsFunc` kullanın. NSGo satırları değerlerine göre yerinde günceller; seçili değer sunulmaya devam ettiği sürece seçili sekmeyi ve klavye odağını korur. Kaybolan bir değer, uygulama onu değiştirene kadar seçilmemiş kalır.

## Sekme çubukları

`ui.TabShell` üst düzey bölümleri her pencere genişliğinde bir sekme çubuğunun yanında tutar: varsayılan olarak altta, `Edge` (veya `EdgeFunc`) `mobile.TabBarLeading`, `mobile.TabBarTrailing` ya da `mobile.TabBarAutomatic` (`cx.Env().VerticalBarEdge()`'i izler) olduğunda ise baş veya son kenar boyunca dikey olarak. Her sekme `SystemIcon` veya bir SVG `Icon` sağladığında sistemin çubuğunu kullanır; iOS'ta herhangi bir kenarda, Android'de alt kenarda. iOS'ta bir SF Symbol'ü veya Android'de bir drawable'ı adlandıran bir `SystemIcon` önceliklidir, aksi halde SVG gösterilir. Aksi takdirde çubuğu verilen SVG simgeleriyle kendisi çizer. Kendi yerini kaplayan bir çubuk, kenarının güvenli alanından kendisi uzak durur; bu yüzden yanındaki bir bölümün, bir `base.Nav` sayfasınınki dahil, o kenardaki `core.EnvSafeArea` değeri 0'dır. iOS'un yüzen alt sistem çubuğunun altında içeriği `ui.TabBarInset` ile uzak tutun.

Yerel ayarı veya uygulama durumunu izleyen sekme başlıkları için `ShellTab.TextFunc`, çubuğun erişilebilir adı için `TabShell.LabelFunc` ayarlayın. Bir başlığı değiştirmek, simgesi aynı kaldığı sürece seçili bölümü ve mevcut sekmeyi korur.

`mobile.TabBar`, iOS ve Android'de sistemin sekme çubuğudur; `Items`, `Selected`, `OnChange` ve isteğe bağlı `Edge` (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` veya `mobile.TabBarAutomatic`) / `EdgeFunc` alanlarını alır. Her `mobile.TabBarItem` bir `Title` ve `Icon` içinde bir platform simge adı alır; `SVG: lucide.House()`, `Icon` iOS'ta bir SF Symbol'ü veya Android'de bir drawable'ı adlandırmadığı her yerde gösterilir; 24 noktaya sığdırılır ve seçime göre renklendirilir. TSX'te `svg` bir SVG belgesi, `edge` ise `"bottom"`, `"leading"`, `"trailing"` veya `"automatic"` alır. Diğer yerlerde `mobile.TabBar` yer kaplamaz, bu yüzden orada `ui.Tabs` gösterin.

`ui.SidebarShell`, kenar çubuğunun, alt bilgisinin, başlıklarının, içeriğinin ve sağ panelinin tuttuklarını sol ve sağ güvenli alandan kendisi uzak tutar, arka planları ise pencerenin kenarlarına kadar uzanır; bu yüzden onların içinde o kenarlardaki `core.EnvSafeArea` değeri 0'dır ve `ui.NavHeader` yan güvenli alanı dolgusuna ekler. Bir `ui.NavPage`'in veya bir `TabShell` bölümünün içeriği `PaddingEnv` ile güvenli alandan kendisi uzak durur; böylece tam taşan bir görüntü gibi kenarlara kadar uzanması gerekenler uzanabilir.

## Erişilebilirlik

Liste `RoleTabList`, her sekme `RoleTab` rolünü, seçili sekme ise durumunu bildirir. Liste tek bir Tab durağıdır; Left ve Up, Right ve Down, Home ve End etkin sekmeler arasında dolaşır ve ulaşılan sekme bir basma bildirir. `Label` listeyi adlandırır.

## Platform

iOS ve Android sistemin sekme çubuğunu kendi görünümleri ve hareketleriyle gösterir; macOS, Windows, Linux ve web `ui.Tabs` gösterir. Uygulamanın içeriği her hedefte aynıdır.

## İlgili

- [Gezinme](../navigation.md): bir uygulamadaki sayfalar ve sekme çubukları.
- [Menü](menu.md): bölümler yerine komutlar.
- [Seçim kutusu](select.md): bir listedeki birçok seçenek.
- Galerinin Tabs ve NavStack sayfaları.
- [Stilsiz bileşenler](../authoring/base.md#tabs): kendi görünümünüze sahip sekmeler için `base.Tabs`.
