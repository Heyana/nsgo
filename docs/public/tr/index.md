---
title: Giriş
description: NSGo'nun ne olduğu, neyin üzerinde çalıştığı ve nasıl çalıştığı.
order: 1
---

NSGo, Go ile yazılmış, platformlar arası yerel bir UI çatısıdır. Bir uygulamayı Go veya TSX ile bir kez yazarsınız ve uygulama iOS ve onun App Clip'leri, tvOS, watchOS, Android, macOS, Windows, Linux ve web üzerinde, her platformun kendi görünümleriyle çizilerek çalışır. Go uygulamaları deneysel olarak WeChat, QQ, Alipay ve Douyin mini programlarında da çalışır: bu platformlar bir uygulamayı 2 MB ile sınırlar ve daha büyük uygulamalar bu sınırı aşar. HarmonyOS, Go uygulamalarını ArkUI ile çizerek önizleme olarak çalıştırır.

## Ne yazarsınız

Bir uygulama bir Go main paketidir. Bileşenleri öğeler döndürür: görünümler, metinler, görüntüler, girişler ve `ui` içindeki denetimler. Durum sinyallerde yaşar ve bir sinyal değiştiğinde yalnızca onu okuyan şey güncellenir.

```go
func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
	}}
}
```

Aynı uygulama TSX ile yazılıp Tailwind sınıflarıyla stillendirilebilir ([TSX uygulamaları](tsx.md)).

## Nasıl çalışır

Uygulamanın sahibi bir Go motorudur: reaktif grafik, stiller, flexbox ve grid yerleşimi ve metin hazırlığı. Her güncelleme tek bir değişiklik grubu üretir ve ince bir arka uç bunu platformun yerel görünümlerine uygular. Animasyon platformun kare saatinde çalışır, böylece hiçbir şey her karede bir kez işlenmez.

| Platform | Çizim aracı |
| --- | --- |
| iOS | UIKit ve CoreText |
| Android | Android Views |
| macOS | AppKit ve CoreText |
| Windows | Direct2D ve DirectWrite |
| Linux | GTK 4, Pango ve Cairo |
| Web | WebAssembly ve DOM öğeleri |

Her hedef, Go ile bir Mac üzerinde derlenir; iOS ayrıca Xcode gerektirir. `nsgo` aracı uygulamaları oluşturur, çalıştırır, test eder ve paketler.

## Durum

NSGo erken aşamadadır: 0.1.0 ilk sürümüdür, API'si henüz kararlı değildir ve eksik olanlar [yol haritasında](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md) yer alır. [Native Shell](https://github.com/longbridge/nsh)'in tasarımını Go'ya taşır.

## Belgeler nasıl okunur

Her sayfa tek bir soruyu yanıtlar. Sayfa aksini belirtmedikçe bir kod örneği bir bileşenin parçasıdır: `cx`, bir bileşenin aldığı `*core.Scope` değeridir. Uygulamalar, bir görünümü olan `ui` bileşenlerini kullanır; `base` ise aynı denetimleri görünümsüz olarak bileşen yazarları için tutar.

- **Başlarken**: [kurulum](getting-started.md), ardından adım adım oluşturulan örnek uygulamalar: [bir yapılacaklar uygulaması](tutorials/todo.md), [bir not uygulaması](tutorials/notes.md) ve [popüler GitHub depoları](tutorials/repos.md).
- **Araç zinciri**: [çalıştırma ve hata ayıklama](debugging.md), [test etme](testing.md), [derleme ve dağıtım](building.md) ve [gömme](embedding.md).
- **Bileşenler**: [en çok kullanılan bileşenler](ui.md), varyantları, durumları ve erişilebilirlikleri.
- **Temel kavramlar**: [bileşenler ve durum](components.md), [yerleşim](layout.md), [stiller](styles.md), [metin](text.md), [listeler](lists.md), [gezinme](navigation.md), [formlar](forms.md), [yerelleştirme](localization.md), [TSX](tsx.md) ve diğerleri.
- **Platform özellikleri**: [platform hizmetleri](services.md), [bildirimler](notifications.md), [dokunsal geri bildirim](haptics.md), [widget'lar](widgets.md), [Live Activities](live-activities.md) ve [izinler](permissions.md).
- **Bileşen yazma**: [başkalarının uygulamaları için bileşenler](authoring/index.md), stilsiz ve stilli.
- **Katkıda bulunma**: [NSGo'yu kaynaktan derleyin](contributing.md), içinde yolunuzu bulun ve bir değişiklik gönderin.
- [Canlı demo](https://nsgo.dev/gallery/): her bileşen, tarayıcınızda.
