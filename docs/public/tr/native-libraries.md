---
title: Yerel kütüphaneler
description: Önceden derlenmiş paylaşımlı kütüphaneleri bir Go paketiyle, NSGo'nun olağan paketleme, lisans ve imzalama adımları üzerinden dağıtın.
group: Araç zinciri
order: 23
---

Bir Go paketi, purego aracılığıyla yüklediği paylaşımlı kütüphaneleri, paket dizinine `nsgo-natives.yaml` koyarak birlikte dağıtabilir. Bir uygulama paketi içe aktardığında NSGo, hedef için paketin bildirimlerini seçer, dosyaları doğrular ve imzalama ile yükleyici oluşturmadan önce bunları dahil eder. Uygulamanın ek bir yapılandırmaya veya paketleme komutuna ihtiyacı yoktur.

## Kütüphaneleri bildirme

```yaml
version: 1
libraries:
  macos_arm64:
    - name: libexample.dylib
      path: lib/macos-arm64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  macos_x64:
    - name: libexample.dylib
      url: https://downloads.example.org/v1.0/macos-x64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  windows_arm64: []
  windows_x64: []
  linux_arm64: []
  linux_x64: []
```

Desteklenen anahtarlar `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64` ve `linux_x64`'tür. Go paketinin seçildiği her hedefi bildirin; `[]` o hedefin paketlenmiş bir kütüphaneye ihtiyaç duymadığını açıkça belirtir. Bunun yerine Go derleme etiketleri, paketi desteklemediği hedeflerin dışında tutabilir. `macos_universal` iki macOS girdisini alır.

Her kütüphanede şunlar bulunur:

- `name`: kurulu dosya adı; macOS'ta `.dylib`, Windows'ta `.dll`, Linux'ta ise `.so` veya sürümlü bir `.so.*` ile biter.
- `path` (bu paket dizinine göreli bir dosya) veya `url` (kütüphanenin kendisi için bir HTTP(S) URL'si) alanlarından tam olarak biri. Arşiv URL'leri yerine çıkarılmış kütüphaneleri yayımlayın. URL'ler gömülü kullanıcı adları ve parolalar içermez.
- `sha256`: özgün dosyanın SHA-256 değeri; her iki kaynak için de zorunludur. Örneğin macOS'ta `shasum -a 256 lib/macos-arm64/libexample.dylib` bunu yazdırır.
- `licenses`: bu paket dizinine göreli olarak, özgün lisans ve bildirim dosyalarına giden boş olmayan yollar. Bu metinler `THIRD_PARTY_NOTICES.txt` içinde dağıtılır ve sürüm derlemelerinde `core.ThirdPartyNotices()`'a ulaşır. Uygulamanın mevcut [lisans politikası](building.md#lisans-bildirimleri), tam modül istisnaları da dahil olmak üzere, yerel lisanslara bağımsız olarak uygulanır.

Yerel yollar ve sembolik bağlantılar bildiren paketin içinde kalır. NSGo bilinmeyen alanları, sağlama toplamı uyuşmazlıklarını, başka bir işletim sistemi veya mimariye ait dosyaları ve çakışan kurulu adları reddeder. İki paket, aynı sağlama toplamına sahip aynı adlı bir dosyayı paylaşabilir. Bir dosya 1 GiB ile sınırlıdır.

## Paketleme ve yükleme

| Hedef | Kütüphane dizini |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows ve Linux | Yürütülebilir dosyanın yanında |

Paket, kütüphanesini çalışma dizini yerine yürütülebilir dosyanın konumunu kullanarak o dizinden yükler. Bağımlılıkları, eşlerinin birlikte dağıtıldıkları dizinden yüklenmesi için önceden derleyin. Gerekli her eş dosyayı ve yükleyicinin ihtiyaç duyduğu her dosya adını bildirin.

NSGo önbelleğe alınmış indirmeleri yeniden kullanmadan önce tekrar denetler ve eşzamanlı derlemeleri bir dosya kilidiyle eşgüdümler. İçerik önbelleği, işletim sisteminin kullanıcı önbelleği dizini altında `nsgo/natives` içinde bulunur. Yerel dosyalar kopyalanırken doğrulanır.

Evrensel bir macOS paketi, her iki mimari için aynı kütüphane adlarını gerektirir. NSGo dilimleri `lipo` ile birleştirir; bir yayıncının zaten evrensel olan kütüphanesi her iki girdi için de kullanılabilir. Kütüphaneler uygulamadan önce imzalanır, ardından uygulamanın olağan disk görüntüsü, imzalama ve noter onayı (notarization) adımlarından geçer. Windows imzalaması, `build.installers: false` olsa bile bildirilen DLL'leri kapsar. Linux yükleyicileri, yürütülebilir dosya dizinindekiyle aynı dosyaları ve bildirimleri içerir.

Geliştirme ve yerel masaüstü hedefi testleri aynı paket seçimi ve kopyalama adımlarını kullanır. Bildirimler, yerel değiştirmeler ve vendor'a alınmış paketler dahil olmak üzere derlemenin hedefi, etiketleri ve oluşturulan Go katmanı (overlay) ile seçilir. Diğer hedefler şu anda bu dosyadan hiçbir kütüphane paketlemez.

## Vendor'a alırken dosyaları koruma

Go'nun vendor işlemi, bir paket dizininin altındaki dosyaları korumak için gömülü veriye ihtiyaç duyar. Bir paket, baytlarını çalışma zamanında kullanmadan bildirim dosyasını ve lisanslarını dahil edilmek üzere işaretleyebilir:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

`path` kullanırken yerel kütüphane yollarını da, örneğin `lib/*`, bu embed yönergesine ekleyin. URL indirmeleri, Go paketinde yalnızca bildirim ve lisans dosyalarına ihtiyaç duyar.
