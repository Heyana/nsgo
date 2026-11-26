---
title: Yerel kütüphaneler
description: Önceden derlenmiş paylaşımlı kütüphaneleri ve Apple framework'lerini bir Go paketiyle, NSGo'nun olağan paketleme, lisans ve imzalama adımları üzerinden dağıtın.
group: Araç zinciri
order: 23
---

Bir Go paketi, purego aracılığıyla yüklediği paylaşımlı kütüphaneleri, paket dizinine `nsgo-natives.yaml` koyarak birlikte dağıtabilir; sürüm 2'den itibaren iOS ve tvOS için framework'lerini de. Bir uygulama paketi içe aktardığında NSGo, hedef için paketin bildirimlerini seçer, dosyaları doğrular ve imzalama ile yükleyici oluşturmadan önce bunları dahil eder. Uygulamanın ek bir yapılandırmaya veya paketleme komutuna ihtiyacı yoktur.

Masaüstü uygulama derlemeleri ve hedef testleri, uygulama kodu için cgo'yu devre dışı bırakır. Yerel kod purego'yu ve `nsgo-natives.yaml` içinde bildirilen bir kütüphaneyi kullanmalıdır. NSGo, cgo dosyaları içeren uygulama paketlerini derleme başlamadan reddeder.

## Kütüphaneleri bildirme

```yaml
version: 2
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
  ios:
    - name: Example.framework
      url: https://downloads.example.org/v1.0/ios/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  ios_simulator:
    - name: Example.framework
      path: lib/ios-simulator/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  tvos: []
  tvos_simulator: []
```

Sürüm 2'nin desteklediği anahtarlar `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64`, `linux_x64`, `ios`, `ios_simulator`, `tvos` ve `tvos_simulator`'dır. Sürüm 1 yalnızca altı masaüstü anahtarını destekler; bir dosya yalnızca kendi sürümünün anahtarları için okunur, bu yüzden iOS ve tvOS derlemeleri sürüm 1 dosyasından hiçbir şey paketlemez. Go paketinin seçildiği her hedefi bildirin; `[]` o hedefin paketlenmiş bir kütüphaneye ihtiyaç duymadığını açıkça belirtir. Bunun yerine Go derleme etiketleri, paketi desteklemediği hedeflerin dışında tutabilir. `macos_universal` iki macOS girdisini, bir terminal hedefi ise kendi masaüstününkileri alır: `tui_macos_arm64`, `macos_arm64` girdilerini alır.

Her kütüphanede şunlar bulunur:

- `name`: kurulu dosya adı; macOS'ta `.dylib`, Windows'ta `.dll`, Linux'ta ise `.so` veya sürümlü bir `.so.*` ile biter. iOS ve tvOS'ta bir framework'ün adıdır, `<Name>.framework`; burada `Name` ASCII harflerinden, rakamlardan, `.` ve `-` karakterlerinden oluşur, bir harf veya rakamla başlayıp biter.
- `path` (bu paket dizinine göreli bir dosya) veya `url` (kütüphanenin kendisi için bir HTTP(S) URL'si) alanlarından tam olarak biri. Sürüm 2 yalnızca https URL'lerini kabul eder ve bir yönlendirme https'ten ayrıldığında indirme başarısız olur. Arşiv URL'leri yerine çıkarılmış kütüphaneleri, bir framework için ise yalnızca ikili dosyasını yayımlayın. URL'ler gömülü kullanıcı adları ve parolalar içermez.
- `sha256`: özgün dosyanın SHA-256 değeri; her iki kaynak için de zorunludur. Örneğin macOS'ta `shasum -a 256 lib/macos-arm64/libexample.dylib` bunu yazdırır.
- `licenses`: bu paket dizinine göreli olarak, özgün lisans ve bildirim dosyalarına giden boş olmayan yollar. Bu metinler `THIRD_PARTY_NOTICES.txt` içinde dağıtılır ve sürüm derlemelerinde `core.ThirdPartyNotices()`'a ulaşır. Uygulamanın mevcut [lisans politikası](building.md#lisans-bildirimleri), tam modül istisnaları da dahil olmak üzere, yerel lisanslara bağımsız olarak uygulanır.

Yerel yollar ve sembolik bağlantılar bildiren paketin içinde kalır. NSGo bilinmeyen alanları, sağlama toplamı uyuşmazlıklarını, başka bir işletim sistemi veya mimariye ait dosyaları ve çakışan kurulu adları reddeder. İki paket, aynı sağlama toplamına sahip aynı adlı bir dosyayı paylaşabilir. Bir dosya 1 GiB ile sınırlıdır.

## Paketleme ve yükleme

| Hedef | Kütüphane dizini |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows, Linux ve terminal hedefleri | Yürütülebilir dosyanın yanında |
| iOS ve tvOS | `<app>.app/Frameworks/<Name>.framework/<Name>` |

Paket, kütüphanesini çalışma dizini yerine yürütülebilir dosyanın konumunu kullanarak o dizinden yükler. Bağımlılıkları, eşlerinin birlikte dağıtıldıkları dizinden yüklenmesi için önceden derleyin. Gerekli her eş dosyayı ve yükleyicinin ihtiyaç duyduğu her dosya adını bildirin.

NSGo önbelleğe alınmış indirmeleri yeniden kullanmadan önce tekrar denetler ve eşzamanlı derlemeleri bir dosya kilidiyle eşgüdümler. İçerik önbelleği, işletim sisteminin kullanıcı önbelleği dizini altında `nsgo/natives` içinde bulunur. Yerel dosyalar kopyalanırken doğrulanır.

Evrensel bir macOS paketi, her iki mimari için aynı kütüphane adlarını gerektirir. NSGo dilimleri `lipo` ile birleştirir; bir yayıncının zaten evrensel olan kütüphanesi her iki girdi için de kullanılabilir. Kütüphaneler uygulamadan önce imzalanır, ardından uygulamanın olağan disk görüntüsü, imzalama ve noter onayı (notarization) adımlarından geçer. Windows imzalaması, `build.installers: false` olsa bile bildirilen DLL'leri kapsar. Linux yükleyicileri, yürütülebilir dosya dizinindekiyle aynı dosyaları ve bildirimleri içerir. Terminal hedefleri macOS kütüphanelerini ad hoc imzalar.

Geliştirme, `-run` ve hedef testleri, masaüstlerinde olduğu gibi iOS ve tvOS'ta da aynı paket seçimi ve kopyalama adımlarını kullanır. Bildirimler, yerel değiştirmeler ve vendor'a alınmış paketler dahil olmak üzere derlemenin hedefi, etiketleri ve oluşturulan Go katmanı (overlay) ile seçilir. Android, HarmonyOS, watchOS, web ve mini programlar bu dosyadan hiçbir şey paketlemez.

## iOS ve tvOS framework'leri

NSGo her framework'ün ikili dosyasını denetler ve aşağıdakileri sağlamıyorsa nedenini belirterek derlemeyi durdurur:

- `LC_BUILD_VERSION`'ının belirttiği gibi hedefin platformu, cihazlar veya simülatör, için derlenmiş bir arm64 Mach-O dinamik kütüphanesi olmalıdır; evrensel bir dosyada arm64 dilimi kullanılır;
- en düşük işletim sistemi sürümü uygulamanınkinden, yani `apple.info.MinimumOSVersion` ya da bu yoksa iOS 15.0 ve tvOS 17.0 sürümünden yeni olmamalıdır ([Derleme ve dağıtım](building.md#nsgoyaml));
- kurulum adı `@rpath/<Name>.framework/<Name>` olmalıdır;
- `@rpath` üzerinden yalnızca uygulamanın paketlerinin aynı hedef için bildirdiği framework'leri yüklemelidir.

NSGo framework'ün `Info.plist` dosyasını yazar; bunun `CFBundleIdentifier` değeri uygulamanın kimliği, bir nokta ve küçük harfli `Name`'den oluşur ve platformu ile uygulamanın en düşük işletim sistemi sürümünü taşır. Simülatör derlemeleri framework'ü ad hoc imzalar. Cihaz derlemesi onu uygulamanın imzalamasına bırakır: `sign.profile` onu uygulamadan önce uygulamanın kimliğiyle imzalar; Xcode da bir cihazda `nsgo build -run` ve `nsgo dev` için aynısını yapar. Eş framework'leri yükleyen bir framework'e `@loader_path/..` çalışma yolunu verin; böylece onları `Frameworks` içinde bulur.

Uygulama iOS framework'leri bildiren bir paketi bağladığında `nsgo build -embed` paketi adlandırarak durur: bir ana uygulama için XCFramework bunları henüz taşıyamaz.

## Vendor'a alırken dosyaları koruma

Go'nun vendor işlemi, bir paket dizininin altındaki dosyaları korumak için gömülü veriye ihtiyaç duyar. Bir paket, baytlarını çalışma zamanında kullanmadan bildirim dosyasını ve lisanslarını dahil edilmek üzere işaretleyebilir:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

`path` kullanırken yerel kütüphane yollarını da, örneğin `lib/*`, bu embed yönergesine ekleyin. URL indirmeleri, Go paketinde yalnızca bildirim ve lisans dosyalarına ihtiyaç duyar.
