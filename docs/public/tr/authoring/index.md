---
title: Bileşen yazma
description: Başkalarının uygulamalarında kullanabileceği bileşenleri, NSGo'nun kendi base ve ui paketlerinin yapıldığı gibi oluşturun.
group: Bileşen yazma
order: 70
---

Bu sayfalar bileşen yazarları içindir: bir denetim kütüphanesi, bir tasarım sistemi veya uygulamaların bir Go paketi olarak içe aktardığı karmaşık bir görünüm. [Bileşenler ve durum](../components.md) ile [Stiller ve temalar](../styles.md) konularının bilindiği varsayılır.

## Bir bileşen

Bir bileşen, tıpkı bir uygulamada olduğu gibi `Render` yöntemine sahip bir türdür. Alanları uygulamanın ayarladıklarıdır; uygulamanın ayarlamadığı her şey sıfır değerini alır, bu nedenle her alanın sıfır değeri olağan durum olmalıdır.

```go
package chips

import "github.com/nsgo-dev/nsgo/core"

// Chip is a short label in a pill.
type Chip struct {
	Text string
}

// Render implements core.Node.
func (c Chip) Render(*core.Scope) core.Node {
	return core.View{Class: "self-start rounded-full px-3 py-1", Children: []core.Node{
		core.Text{Content: c.Text, Class: "text-sm"},
	}}
}
```

Bir uygulama bunu herhangi bir öğe gibi kullanır: `chips.Chip{Text: "New"}`.

## Davranış ve görünüm

NSGo her denetimi iki kez tutar. `base` davranışını, durumunu ve erişilebilirliğini görünümü olmadan tutar ve her stili uygulamadan parça parça alır; `ui` ise `base` biçimine bir görünüm giydirir, böylece davranış tek bir yerde yaşar. Bir kütüphane de aynı ayrımı izleyebilir:

- **Stilsiz** bir bileşen basmaları, klavyeyi, odağı ve ekran okuyucuların duyduklarını sahiplenir ve tıpkı `base.Switch`'in yaptığı gibi her parçanın stilini sunar. Uygulamalar ve diğer kütüphaneler ona kendi görünümlerini verir ([Parçalar ve durumlar](parts.md)).
- **Stilli** bir bileşen stilsiz bir bileşeni sarmalar ve tıpkı `ui.Switch`'in yaptığı gibi yalnızca yürürlükteki temadan stiller seçer ([Görünümler ve temalar](looks.md)).

Rozet (badge) gibi paylaşmaya değer bir davranışı olmayan bir bileşen tek başına stilli olabilir.

## Bir paket

- Her dosyada, adını aldığı tek bir bileşeni (`chip.go`) ve yalnızca onun kullandığı türleri tutun; birden çok bileşenin paylaştıkları kendi dosyasına gider.
- Karmaşık bir bileşene, `base/dock`'taki `dock.Area` gibi API'si bileşenin adını düşüren kendi paketini verin.
- Bir bileşenin söylediği sözcükleri paketin `locales/` dizinine koyun ([Sözcükler, testler ve yayınlama](shipping.md)).
- Cihaza göre örtük olarak hiçbir şey yapmayın: telefonda veya masaüstünde değişen bir boyut ya da biçim, uygulamanın ayarladığı bir alandır.

## Sonraki adımlar

- [Parçalar ve durumlar](parts.md): denetlenen değerler, stilli parçalar ve erişilebilirlik.
- [Görünümler ve temalar](looks.md): temadan gelen renkler, varyantlar ve boyutlar.
- [Stilsiz bileşenler](base.md): stillendirmek veya üzerine inşa etmek için NSGo'nun kendi görünümsüz denetimleri.
- [Sözcükler, testler ve yayınlama](shipping.md): çeviriler, testler ve TSX.
