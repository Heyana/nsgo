---
title: Animasyon
description: Platformun kare saatinde çalışan geçişler, animasyonlu değerler, yerleşim animasyonları ve hareket ön ayarları.
group: Temel kavramlar
order: 58
---

## Animasyonlu değerler

Bir animasyon platformun ana iş parçacığında kendi kare saatiyle adımlanarak çalışır; dolayısıyla kare başına motora hiçbir maliyeti yoktur. `AnimatedValue` platformun hareket ettirdiği bir sayıdır; `Style.Animated` bir görünümün opaklığını, ötelemesini, ölçeğini, döndürmesini, arka planını, kenarlık rengini, gölgesini ve metin rengini buna bağlar.

```go
fade := core.NewAnimatedValue(cx, 0)
cx.StartAnimation(core.Timing(fade, 1, core.TimingConfig{
	Duration: 300 * time.Millisecond,
	Easing:   core.EaseOut,
}), nil)

core.View{Style: core.Style{Animated: &core.AnimatedStyle{Opacity: fade}}}
```

`Timing`, `Spring` ve `Decay` tek bir değeri hareket ettirir; `Sequence`, `Parallel`, `Stagger`, `Delay` ve `Loop` ise bunları birleştirir. `cx.StartAnimation`, `Stop` yöntemi çalıştırmayı bulunduğu yerde durduran ve `Reset` yöntemi değerlerini geri yükleyen bir `AnimationRun` döndürür. Bir çalıştırma değerinin bulunduğu yerden başlar ve bir değerin yeni animasyonu eskiyi sonlandırır.

```go
scale := core.NewAnimatedValue(cx, 1)
ui.Spring(cx, scale, 1.1, core.SpringConfig{Stiffness: 220, Damping: 14})
```

Türetilmiş düğümler bir değeri izler: `core.Interpolate` kendi yumuşatma (easing) ve ekstrapolasyonuyla sayıları veya renkleri eşler; `core.Add`, `Sub`, `Mul`, `Div`, `Mod`, `DiffClamp` ve `SnappingDiffClamp` ise ondan başka değerler hesaplar. `ui.Lerp` kısa bir enterpolasyondur.

```go
angle := core.Interpolate(scroll, core.Interpolation{
	Input:  []float64{0, 240},
	Output: []float64{0, 90},
	Easing: core.EaseOut,
})
```

`Scope.RequestAnimationFrame` bir işlevi bir sonraki ekran karesinde bir kez çalıştırır ve `Scope.Animate` bu saat üzerinde bir süreyi adımlar. Her ikisi de grafiğin goroutine'inde çalışır, bu nedenle sinyal ayarlayabilirler. İşlev, macOS, iOS, Android, web, Linux, Windows, HarmonyOS ve watchOS'ta ekranın kendi kare zamanını alır (HarmonyOS'ta derleme makinesinde OpenHarmony SDK varsa; onsuz paketlenen bir uygulama motorun saatini alır); böylece öğeleri iki kare arasındaki süreye göre hareket ettiren bir döngü ekranın hızını izler; bir `nsgotest` testi test saatinin zamanını verir.

`cx.SetPreferredFrameRate(core.FrameRateRange{Preferred: 30})` ile bir pencerenin geri çağrı hızını ayarlayın. Bir ProMotion ekranında `{Minimum: 80, Maximum: 120, Preferred: 120}` gibi ekran bağlantısına bir aralık vermek için `Minimum` ve `Maximum` geçirin. Bunu pencerenin kendi kapsamından çağırın. Tercih değişene kadar `RequestAnimationFrame` ve `Animate` için geçerli olur. Sıfır aralığı ekranın normal hızını geri yükler. Geri çağrı zamanı gerçek ekran zamanı olarak kalır. Ekran ve işletim sistemi kullanılabilir yenileme hızını seçer ve NSGo geri çağrıları hızlandırmak/ayarlamak için ekran karelerini atlayabilir.


`Scope.Tween` bir animasyonlu değerin ulaşamadığı şeyler (örneğin bir metinde yukarı sayan bir rakam) için o saatte bir sayıyı adımlar: tıpkı `Timing`'in bir değeri hareket ettirmesi gibi bir işlevi bir sayıdan diğerine her sayı ile ve son değerle bir kez daha çağırır. Bunu başlatan bir `nsgotest` adımı bir animasyonda olduğu gibi son değeri bekler ve azaltılmış hareket altında doğrudan son değeri verir. TSX'teki `Animated.tween` aynıdır.

```go
freed := core.NewSignal(cx, 0.0)
cx.Tween(0, 18.7, core.TimingConfig{Duration: 600 * time.Millisecond, Easing: core.BezierEasing(core.EaseOut)}, freed.Set, nil)

core.Text{ContentFunc: func() string { return fmt.Sprintf("%.1f GB can be freed", freed.Get()) }}
```

## Geçişler

`Transition`, bir görünümün stili veya sınıfları değiştiğinde bunları platform üzerinde, motor için kare başına iş yükü olmadan canlandırır. `Properties` neyin hareket edeceğini söyler; 0, çerçeve dışındaki her özellik anlamına gelir.

```go
core.View{Style: core.Style{
	Background:  core.RGB(0xffffff),
	CornerRadius: 12,
	Transition:  core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionBackground | core.TransitionTransform},
	Transform:   core.Transform{Scale: 1},
	Hover:       &core.Style{Transform: core.Transform{Scale: 1.02}, Background: core.RGB(0xf5f5f5)},
}}
```

`TransitionLayout`, kendi stili ona yeni bir çerçeve verdiğinde bir görünümü göründüğü yerden taşır ve yeniden boyutlandırır. Yerleşim animasyonu ise React Native'in `LayoutAnimation`'ı gibi tüm bir işlemeyi (commit) canlandırır: `cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` bir sonraki toplu işlemin taşımalarını, eklemelerini ve kaldırmalarını canlandırır; ön ayarlar yumuşatma (easing), doğrusal (linear) ve yaylı (spring) seçeneklerini kapsar. Bir `LayoutSpring` geçişi `Response` (yayın sönümsüz doğal periyodu) ve `DampingRatio` (`< 1` az sönümlü, `1` kritik sönümlü, `> 1` aşırı sönümlü) alır, sabit bir süre yerine durulana kadar çalışır ve uçuş ortasında yeniden hedeflendiğinde konumunu ve hızını yeni hedefe taşır.

```go
cx.AnimateLayout(core.LayoutAnimation{
	Update: &core.LayoutTransition{
		Curve:        core.LayoutSpring,
		Response:     280 * time.Millisecond,
		DampingRatio: 0.85,
	},
})
open.Set(!open.Get())
```

`View.OnTransitionEnd`, bir şey hareket etmiş olsun ya da olmasın, bir değişiklik geçişini tamamladığında çalışır. Azaltılmış hareket (reduced motion) geçişleri ve yerleşim animasyonlarını kapatır; bu nedenle son durumu ulaşılabilir tutarak platformun ayarına saygı gösterin.

## Hareket (Motion)

`core.Motion`, uygulamanın kendi `MotionSpec`'inden veya hazır bir `MotionPreset`'ten (girişler, çıkışlar, dikkat çekiciler ve döngüler) bir görünümün opaklığını, ötelemesini, ölçeğini ve döndürmesini anahtar kareler boyunca birlikte hareket ettirir. Stilini bağlamak görünümü durağan hale getirir; `Play` bir belirtimi (spec) çalıştırır ve `Exit` bir görünümün bir `core.Presence` içinde kendi hareketiyle çıkmasını sağlar.

```go
enter := core.SlideInUp.Spec()
motion := core.NewMotion(cx, &enter)
motion.Exit(core.FadeOut.Spec())

core.View{Style: core.Style{Animated: motion.Style()}, Children: content}
```

`core.Presence{When, Then, Else}` kaydettiği her çıkış bitene kadar bir dalı korur; böylece bir alt sayfa veya bildirim (toast) onu ne kaldırırsa kaldırsın hareketiyle ayrılır. `core.PresenceOf` aynı davranışı karşılaştırılabilir bir anahtar ve içerik işlevine uygular ve `MaxLeaving` bir değer hızla değişirken eski dalları sınırlar. `MotionPresets()` her ön ayarı listeler ve `Kind()` bunun giriş, çıkış, dikkat çekici veya döngü olup olmadığını söyler.

## Anahtar kareler (Keyframes)

`KeyframeAnimation` platformda anahtar kareleri tekrarlar: bir süre, gecikme, yumuşatma ve yineleme sayısı ile her ofsette opaklık, ölçek, döndürme ve dikey öteleme. `KeyframesSpin`, `KeyframesPing`, `KeyframesPulse` ve `KeyframesBounce` Tailwind'in anahtar kareleridir.

```go
core.View{Style: core.Style{Animation: core.KeyframeAnimation{
	Keyframes: core.KeyframesPulse,
	Duration:  2 * time.Second,
	Iterations: 0, // forever
}}}
```

`ValueKeyframes`, ofsetler, parça başına yumuşatma, gecikme, yinelemeler ve yön ile Web Animations'ın yaptığı gibi bir animasyonlu değeri duraklar boyunca hareket ettirir. Hareketler (gestures) de değerleri sürebilir: `View.PanValues`, `PinchValue` ve `RotationValue` hareketin başladığı yerden itibaren bunları ana iş parçacığında hareket ettirir.

`core.PanEvent`, her kaydırma (pan) aşamasında görünüm içindeki `Position` ve sayfa içindeki `PagePosition` değerlerini içerir; bu nedenle sürükleme araçlarının parmak konumunu kurtarmak için ayrı bir basma olayına ihtiyacı yoktur.

## Sonraki adımlar

- [Stiller ve temalar](styles.md): bir geçişin hareket ettirdiği özellikler.
- [Listeler ve kaydırma](lists.md): kaydırma görünümünü izleyen değerler.
- [Yerleşim](layout.md): animasyonlu çerçevelerin ne olduğu.
