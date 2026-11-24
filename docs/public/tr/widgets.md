---
title: Widget'lar
description: Uygulamanın görünümlerini iPhone ve iPad'lerin Ana Ekranı'nda ve Kilit Ekranı'nda ve Mac'in masaüstünde gösterin.
group: Platform özellikleri
order: 47
---

## Bir widget türü

Widget'lar, uygulama çalışsa da çalışmasa da ona bir bakış sunar: iPhone ve iPad'lerin Ana Ekranı'nda ve Kilit Ekranı'nda, Mac'in masaüstünde ve Bildirim Merkezi'nde ve Android'in başlatıcısında. Bir tür, sistemin galerisinin sunduğu tek bir widget'tır ve geldiği ailelerle birlikte tanımlanır:

```yaml
widgets:
  - kind: counter
    name: Counter                 # shown in the system's gallery of widgets
    description: Shows the count.
    families: [small, medium, circular]  # also large, extra_large, rectangular and inline
# apple: { app_group: group.dev.example.hello }  # the shared App Group, group.<id> by default
```

Apple platformlarında onu bir widget uzantısı çizer; `build` ve `dev` bu uzantıyı Xcode'un Swift derleyicisiyle derler ([Derleme ve dağıtım](building.md#widgets-and-live-activities)). Android'de önceden derlenmiş Java çalışma zamanı aynı zaman çizelgesini App Widgets olarak çizer. İkisi de uygulama durduğunda yayınlanan içeriği göstermeye devam eder.

Korumalı alanda çalışan (sandboxed) bir Mac App Store derlemesi için `apple.app_group` ayarlayın ve macOS hazırlama profillerini `sign.macos_profile` ve `sign.widgets_profile` üzerinden sağlayın. Hem uygulama hem de `<id>.widgets` aynı ekibe ait olmalı ve o gruba yetkili olmalıdır. Zaman çizelgeleri ve görüntüler grubun paylaşılan kapsayıcısına gider; doğrudan ve geliştirme macOS derlemeleri mevcut ana dizin konumlarını korur. [Mağaza imzalama](building.md#store-packages) derleme seçeneklerini açıklar.

`name` ve `description` uygulamanın kataloglarındaki iletileri adlandırabilir. Bunu yaptıklarında widget galerisi ve yer tutucu kullanıcının dilini kullanır; bir çeviri eksikse kaynak kataloğun metnine geri döner. Katalog anahtarı olmayan bir değer olduğu gibi gösterilir.

## Bir zaman çizelgesi

`widget.Update`, bir türün widget'larının bundan sonra ne göstereceğini girdilerden oluşan bir zaman çizelgesi olarak yazar: sistem, tarihi en son geçmiş olan girdiyi, ardından sonraki her birini kendi tarihinde, uygulama olmadan gösterir. Sıfır bir `Date` şimdi demektir. Girdi yoksa widget'lar türün adını ve açıklamasını gösterir. Apple platformlarında ön plandaki bir uygulama widget'larını istediği sıklıkta, arka plandaki bir uygulama ise günde birkaç düzine kez günceller.

```go
err := widget.Update("counter", widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Count", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Text{Content: strconv.Itoa(n), Style: widget.Style{FontSize: 44, FontWeight: core.FontWeightBold}},
	}},
})
```

`Entry.Families` bir aileye `Entry.View` yerine kendine ait bir görünüm gösterir, `Entry.Background` widget'ı arkasından, sistemin koruduğu kenar boşluklarının ötesine kadar doldurur ve `Entry.URL` widget'a dokunulduğunda uygulamayla birlikte açılır. `widget.Update`, widget'lar olmadan derlenmiş bir uygulamada `widget.ErrNoWidgets`, macOS, iOS ve Android dışındaki yerlerde `widget.ErrUnsupported` bildirir; `widget.Available` ise bir yazmadan önce hangisi olduğunu söyler. Herhangi bir goroutine üzerinde, `nsgo.Run`'dan önce de çalışabilir.

## Görünümler

Bir widget'ın görünümleri core'un öğeleri değildir: SwiftUI'daki gibi yerleşirler; `Style` boyutlandırmadıkça veya doldurtmadıkça bir görünüm içeriği kadar büyüktür ve `AlignItems` ile `Justify` aksini söylemedikçe bir kapsayıcı tuttuğu şeyi ortalar. `Text`, `Symbol` ve baytlardan ya da `Update`'in okuduğu bir dosyadan gelen `Image` içerik gösterir; `Column`, `Row`, `Stack`, `Spacer` ve `Divider` onu düzenler; `Rectangle`, `Progress` ve `Gauge` çizer; `Date` ise bir zamanı sistemin biçimleriyle gösterir ve `Relative`, `Offset` ve `Timer` her saniye için bir girdi olmadan kendi kendilerine saymaya devam eder.

```go
widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Next hour in", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Date{Time: hour, Format: widget.Timer, Style: widget.Style{TextStyle: widget.Title2, FontVariant: core.FontTabularNums}},
		widget.Progress{Value: 0.4, Style: widget.Style{Color: widget.Accent}},
	}},
}
```

`Style` bir boşluk, dolgu, bir boyut veya doldurma, hizalama, yuvarlatılmış köşeli bir arka plan, sistemin metin stillerini veya bir yazı tipi boyutu, kalınlığı, tasarımı ve varyantını, satır sınırlarını, uygulamayla birlikte açılan bir `Link`'i ve bir görünümü sistemin renklendirdiği görünümler arasına koyan `Accented`'i alır. `widget.Primary`, `Secondary` ve `Accent` görünümü (açık/koyu) izler; `RGB` ve `RGBA` ise `Dark` ile koyu modda başka bir renk alır.

## Düğmeler

`widget.Button`, iOS 17 ve macOS 14'ten itibaren uygulamayı açmadan `widget.OnAction`'ı çalıştırır. Daha eski sistemler yalnızca alt öğelerini gösterir. İşleyiciyi `main` içinde, `nsgo.Run`'dan önce kaydedin: sistem uygulamayı arka planda görünümleri olmadan başlatabilir. Bir widget basmasında `Activity == nil`'dir; `Action` ise düğmenin değeridir.

```go
widget.Button{Action: "refresh", Children: []widget.View{
    widget.Text{Content: "Refresh"},
}}
```

Eylemi işleyin ve işleyici dönmeden önce `widget.Update` çağırın; böylece sistemin otomatik zaman çizelgesi yeniden yüklemesi sonucu görür. İşleyici görünümlerin goroutine'i dışında çalışır; görünümleri değiştirmek için `core.Scope.Post` kullanın. Bu, [Canlı Etkinlikler](live-activities.md) ile aynı işleyiciyi ve intent'i kullanır ve `nsgo.yaml` içinde `live_activities` olmadan yalnızca `widgets` gerektirir.

## Android

Widget'ı başlatıcının widget seçicisinden ekleyin ve orada yeniden boyutlandırın. İşleyici (renderer) sınırlarına göre izin verilen bir aile seçer: her iki boyutta 240 noktanın altında small, enine 240'tan itibaren medium, boyuna 240'tan itibaren large ve enine 500'den itibaren extra-large; o aile listede yoksa listedeki ilk aileyi kullanır. `Entry.URL` ve `Style.Link`, uygulamayı `url_schemes` içinde bildirdiği bir şema üzerinden açar. İç içe bir görünümdeki bağlantı, kapsayıcısının bağlantısına göre önceliklidir.

Gelecekteki zaman çizelgesi girdileri Android'in kesin olmayan alarmlarını kullanır; sistem uygulama arka plandayken bunları geciktirebilir. Zamanlayıcı görünümleri yerel kronometreleri kullanır ve uygulama olmadan işler; şu anda boyutu ve rengi uygular, özel yazı tipi kalınlığı, tasarımı ve italik stil ise hâlâ hedefler arasındadır. Göreli tarihler sistemin programına göre yenilenir. Genel işleyici metni, görüntüleri, yerleşimi, şekilleri, çubukları, halkaları ve yaygın sembol glif kümesini destekler; iki platformda da aynı görünmesi gereken çizimler için `Image` kullanın. Rastgele SF Symbol adları Android için açık bir hedef olarak kalır. Android widget sağlayıcıları bağımsız APK'lar ve AAB'ler için oluşturulur; gömülü barındırıcılar hiç almaz.

## Canlı Etkinlikler

Aynı uzantı, uygulamanın Canlı Etkinliklerini Kilit Ekranı'nda ve Dynamic Island'da çizer ([Canlı Etkinlikler](live-activities.md)).

## Sonraki

- [Canlı Etkinlikler](live-activities.md): uygulamanın olurken izlediği şeyler.
- [Bildirimler](notifications.md): bir widget'ın yanındaki bildirimler ve rozetler.
- [Derleme ve dağıtım](building.md): uzantının paylaştığı uygulama grubu.
