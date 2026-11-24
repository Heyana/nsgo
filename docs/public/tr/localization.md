---
title: Yerelleştirme
description: Bir uygulamanın iletilerini çevirin ve gösterdiği dili izleyin.
group: Temel kavramlar
order: 62
---

## İletiler

`nsgo.yaml` dosyasının yanındaki `locales/`, uygulamanın iletilerini ICU MessageFormat biçiminde, her dil için bir YAML dosyası (`en.yaml`, `de.yaml`) olarak tutar:

```yaml
greeting: Hello, {name}!
appName:
  message: Hello
  description: The app's name, for translators.
```

Bir ileti bir dizedir ya da çevirmenler için bir `description` ve `placeholders` içeren bir `message`'dır.

Paket `locales/` dizinini içe aktarır. `dev`, `build` ve `test` katalogları denetler ve yanlarına her ileti için `locales.Greeting(cx, "Ada")` gibi bir Go işlevi yazar; bu işlev iletiyi uygulamanın gösterdiği dilde söyler ve dil değiştikçe onu izler; `go tool nsgo i18n` yalnızca işlevleri yazar. İletiler sayıları ve tarihleri biçimlendirir (`{d, date, medium}`); `appName` uygulamayı paketlerinde her dilde adlandırır, tıpkı `NSCameraUsageDescription` gibi bir `apple.info` anahtarına sahip bir iletinin o Info.plist girdisini çevirmesi gibi ([izinler](permissions.md#kullanım-açıklamaları)).

## Dil

`cx.SetLocale("de")` dili seçer ve düzen, dilin yazıldığı yönde akar. Bağımsız bir uygulamada, sistemin kendisinin söyledikleri (metin seçim menüleri, dosya iletişim kutuları ve GTK'nin pencere öğeleri gibi) de zorlanan dili izler (Android 13+, Windows ve Linux'ta hemen, macOS ve iOS'ta ise bir sonraki açılıştan itibaren); `cx.SetLocale("")` yeniden kullanıcının dilini izler.

```sh
go tool nsgo dev -lang en-XA
```

uygulamayı, çevrilmemiş ve kesilmiş metni gösteren bir sözde yerel ayarda (pseudo locale) çalıştırır. Her derleme bunu başlarken kabul eder: `--nsgo-lang=en-XA`.

## Haftalar, saatler ve ölçüler

`i18n.Regional(cx)`, dilden bağımsız olarak kullanıcının haftaları, saatleri ve ölçüleri nasıl okuduğunu söyler: haftaların başladığı gün, 12 veya 24 saatlik zaman, ölçü sistemi ve sıcaklık birimi; bunlar kullanıcının platformda ayarladığı gibi, aksi takdirde bölgesinin kullandığı gibidir. Takvimler ve tarih seçiciler haftalarını orada başlatır ve uygulama `FirstDayOfWeek` veya `HourCycle` ayarlamadıkça saat alanları bu saatleri gösterir; bir kataloğun iletilerinin yazdığı saatler de kullanıcının saat biçimini izler.

## NSGo'nun kendi sözcükleri

NSGo'nun bileşenleri sözcüklerini NSGo'nun çevrildiği dillerde söyler. Başka bir dil için, `github.com/nsgo-dev/nsgo/base/locales` gibi bir paketi adlandıran bir anahtar, o paketin iletilerini dosyanın dilinde verir. Platformların menüleri ve sistemin kendi sözcüğü olmadığı yerlerde Android ile Windows ekran okuyucularının duyurduğu sekme veya arama kutusu gibi roller ve durumlar, `github.com/nsgo-dev/nsgo/desktop/locales` sözcüklerini söyler.
