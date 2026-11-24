---
title: Kod tabanı
description: "NSGo'nun her bir parçasının nerede yaşadığı ve nedenini açıklayan belgeler."
group: Katkıda bulunma
order: 81
---

NSGo tek bir Go modülüdür. Uygulamalar paketlerini yukarıdan içe aktarır; her platformun arka ucu en altta yer alır ve değişenleri oluşturulacak, güncellenecek ve kaldırılacak yerel görünümler olarak alır.

## Paketler

| Paket | İçeriği |
| --- | --- |
| `nsgo` (kök) | Derleme etiketleriyle bir arka uç seçen `nsgo.Run` ve `nsgo.NewView` |
| `core` | Uygulamanın API'si: öğeler, stiller, sınıflar, temalar, sinyaller ve servisler; yerleşim ve metin hazırlığı |
| `base` | Stilsiz bileşenler, her biri birer dosya, ve karmaşık olanlar paket olarak (`base/dock`, `base/editor`) |
| `ui` | Görünüme sahip `base` bileşenleri ve `ui/chat` gibi paketler |
| `mobile`, `desktop` | Yalnızca telefonlarda veya yalnızca masaüstlerinde bulunanlar |
| `tsx` | TSX uygulamaları, paketleyicisi ve derleyicisi ve içe aktardıkları modüller |
| `i18n`, `*/locales` | İleti katalogları ve bunlardan üretilen kodlar |
| `nsgotest` | Başsız olarak veya bir platform üzerinde kullanıcı gibi davranan testler |
| `internal/engine`, `internal/driver`, `internal/layout` | Motor goroutine'i, bağlama protokolü ve yerleşim |
| `internal/platform/<target>` | Bir arka uç: tek bir hedef için yerel görünümler, metin ve giriş |
| `cmd/nsgo` | `nsgo` aracı |
| `examples/` | Galeri ve diğer örnek uygulamalar |

Paketler yalnızca aşağıya doğru içe aktarım yapar; `internal/moduletest/layering_test.go` her hedefte grafiği denetler.

## Tasarım belgeleri

[ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) paketleri, iş parçacıklarını ve bir işlemeden (commit) ekrana kadar olan akışları haritalar ve `docs/architecture/` altındaki konuları listeler: yerleşim, metin, kaydırma, temalar veya araç gibi her parça için bir tane. Bir parçayı değiştirmeden önce onun konusunu okuyun; o parçanın nasıl çalıştığını ve NSGo'nun tasarımını taşıdığı [Native Shell](https://github.com/longbridge/nsh) projesinden nerelerde ayrıldığını kaydeder.

[AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md), hem insanlar hem de kodlama ajanları için her değişikliğin izlediği kuralları içerir. Birkaçı çoğu değişikliği şekillendirir:

- Her API'nin tek bir yuvası vardır: öğeler ve ortam için `core`, denetimler için `base`, görünümleri için `ui`, tek bir cihaz türü için `mobile` ve `desktop`.
- `base` bir görünüme sahip değildir; bir `ui` bileşeni onun görünümlü biçimidir.
- Cihaza göre örtük olarak hiçbir şey değişmez: `core` veya `base` içinde masaüstlerinde daha büyük veya telefonlarda daha küçük denetimler yoktur.
- Her yerde yerel görünümler, her platformda aynı: bir platformun yerel olarak sahip olduğunu diğerleri taklit eder.
- Elle yazılmış C yok ve x86_64 Android emülatörü dışında her hedefi derlemek için macOS, Xcode ve Go'dan başka bir şey yok.

Platform platform nelerin eksik olduğu [yol haritasında](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md) yer alır.
