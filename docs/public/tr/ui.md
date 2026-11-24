---
title: Bileşen başvurusu
description: En çok kullanılan bileşenler, varyantları, durumları ve erişilebilirlikleri.
group: Bileşenler
order: 30
---

Bunlar `ui`'ın bileşenleridir; NSGo'nun görünümündedir ve temayı izler. Bu başvuru, bir uygulamanın ilk başvurduğu bileşenlerle başlar; her sayfa en kısa örnekle başlar, ardından bileşenin parçalarını, durumlarını ve erişilebilirliğini ele alır. Galeri, burada henüz sayfası olmayanlar dahil her bileşeni kapsar.

[Galeri](https://nsgo.dev/gallery/) her bileşenin bir demosunu tarayıcıda çalıştırır.

## Eylemler

- [Button](ui/button.md): düz veya vurgu renginde bir basma düğmesi ve simge düğmesi.

## Metin ve girdi

- [Text](ui/text.md): paragraflar, aralıklar, bağlantılar, yazı tipleri ve belge metni.
- [Input](ui/input.md): platformun metin alanı, denetimli veya denetimsiz.
- [Checkbox](ui/checkbox.md): işaretli, işaretsiz veya belirsiz bir onay kutusu.
- [Switch](ui/switch.md): açık ya da kapalı bir ayar.
- [Select](ui/select.md): bir listeden bir değer, açılır pencere ya da sayfa (sheet) olarak.
- [Slider](ui/slider.md): bir iz boyunca tek bir değer veya bir aralık.

## Koleksiyonlar

- [List](ui/list.md): herhangi sayıda satır; yalnızca görünürken oluşturulur, seçim ve etkinleştirme destekler.
- [Scroll](ui/scroll.md): sayfalama, yaslanma, yakınlaştırma ve yenileme ile platformun kaydırma görünümü.

## Gezinme

- [Tabs](ui/tabs.md): bir sekme listesi, bir bölümlü denetim ve sistem sekme çubuğu.

## Katmanlar

- [Dialog](ui/dialog.md): kipli bir yüzey, bir sayfa (sheet) ve sistemin uyarısı.
- [Menu](ui/menu.md): bir tetikleyiciden açılan menü, bağlam menüsü ve bunların öğeleri.

## Daha fazlası

Galeri geri kalanını kapsar: tablolar, grafikler, kod düzenleyici, zengin metin, terminal, dock, komut paleti, renk, tarih ve emoji seçiciler (`ui.EmojiPicker`), bildirim baloncukları (toast) ve daha fazlası. Her birinin orada bir demosu vardır.

`base/terminal.View`, mevcut yerel yolları URL'leri açtığı gibi Command+tıklama ile (Windows ve Linux'ta Control+tıklama) açar. Göreli yollar kabuğun OSC 7 çalışma dizinini kullanır; henüz bir çalışma dizini bildirmemiş yerel bir kabuk `terminal.Options.Dir` değerini, o da ayarlı değilse ana dizini kullanır. Boşluk içeren dosya adlarını tırnak içine alın. Uzak dizin yetkileri ve yerel dizin bildirimi olmayan bağlantılar yerel dosyalara göre çözümlenmez. Dosyalar ve klasörler `Scope.OpenFile` kullanır; URL bağlantıları `Scope.OpenURL` ve gömen barındırıcının URL işleyicisi üzerinden ilerlemeye devam eder.

`base/terminal.View`, programların pano okuma ve yazmalarını varsayılan olarak reddeder. `AllowClipboardRead` ve `AllowClipboardWrite` değerlerini yalnızca uygulama bunlara izin verdiğinde ayarlayın; isteyen görünümün `AccessClipboard` ayarı yine geçerlidir. Aynı özellikler TSX'in `Terminal`'inde de bulunur ve oturumu yeniden başlatmadan değişebilir. Onay arayüzünü uygulamalar sağlar. Kullanıcının açıkça yaptığı yapıştırma, bundan bağımsız olarak Kitty pano protokolünü (mod 5522) kullanan programlara bir metin/PNG anlık görüntüsü sunar; tek kullanımlık parolası yalnızca o anlık görüntüyü okuyabilir. Bu modu kullanmayan programlar sıradan köşeli ayraçlı (bracketed) metin alır. Pano görüntüsünün kullanılabilirliği platformun pano hizmetlerine bağlıdır; X11 birincil seçimi kullanıma sunulmaz.

## Sonraki adımlar

- [Bileşenler ve durum](components.md): bir bileşenin öğeleri nasıl döndürdüğü ve sinyallerin onları nasıl güncellediği.
- [Stiller ve temalar](styles.md): bileşenlerin renklerini aldığı tema.
- [Stilsiz bileşenler](authoring/base.md): kendi görünümünüz için, görünümü olmayan aynı denetimler.
- [Erişilebilirlik](accessibility.md): her denetim için etiketler, roller ve değerler.
