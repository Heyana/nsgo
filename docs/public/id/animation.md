---
title: Animasi
description: "Transisi, nilai teranimasi, animasi tata letak, dan preset gerak yang berjalan pada jam frame platform."
group: Konsep inti
order: 58
---

## Nilai teranimasi

Animasi berjalan pada utas utama platform dan dilangkahkan oleh jam frame (*frame clock*) milik platform sendiri, sehingga tidak membebani mesin sama sekali per frame. `AnimatedValue` adalah angka yang digerakkan platform; `Style.Animated` mengikat opasitas, translasi, skala, rotasi, latar belakang, warna batas, bayangan, dan warna teks suatu tampilan ke nilai tersebut.

```go
fade := core.NewAnimatedValue(cx, 0)
cx.StartAnimation(core.Timing(fade, 1, core.TimingConfig{
	Duration: 300 * time.Millisecond,
	Easing:   core.EaseOut,
}), nil)

core.View{Style: core.Style{Animated: &core.AnimatedStyle{Opacity: fade}}}
```

`Timing`, `Spring`, dan `Decay` menggerakkan satu nilai, sedangkan `Sequence`, `Parallel`, `Stagger`, `Delay`, dan `Loop` menggabungkannya. `cx.StartAnimation` mengembalikan `AnimationRun` yang metode `Stop`-nya menghentikan animasi di posisinya saat itu dan `Reset`-nya mengembalikan nilainya. Animasi dimulai dari posisi nilai saat ini, dan animasi baru pada nilai yang sama menghentikan animasi yang lama.

```go
scale := core.NewAnimatedValue(cx, 1)
ui.Spring(cx, scale, 1.1, core.SpringConfig{Stiffness: 220, Damping: 14})
```

Node turunan mengikuti sebuah nilai: `core.Interpolate` memetakan angka atau warna dengan pelonggaran (*easing*) dan ekstrapolasinya sendiri, sementara `core.Add`, `Sub`, `Mul`, `Div`, `Mod`, `DiffClamp`, dan `SnappingDiffClamp` menghitung nilai lain darinya. `ui.Lerp` adalah interpolasi singkat.

```go
angle := core.Interpolate(scroll, core.Interpolation{
	Input:  []float64{0, 240},
	Output: []float64{0, 90},
	Easing: core.EaseOut,
})
```

`Scope.RequestAnimationFrame` menjalankan fungsi satu kali pada frame tampilan berikutnya, dan `Scope.Animate` melangkahkan durasi pada jam tersebut. Keduanya berjalan pada goroutine grafik, sehingga dapat mengatur sinyal. Fungsi tersebut menerima waktu frame—waktu milik tampilan itu sendiri di macOS, iOS, Android, web, Linux, Windows, HarmonyOS, dan watchOS (di HarmonyOS ketika mesin build memiliki OpenHarmony SDK; aplikasi yang dipaketkan tanpanya mendapatkan jam mesin)—sehingga perulangan yang menggerakkan objek berdasarkan selisih waktu antar dua frame mengikuti laju tampilan; pengujian `nsgotest` memberikan waktu jam pengujian.

Atur laju pemanggilan balik (*callback*) jendela dengan `cx.SetPreferredFrameRate(core.FrameRateRange{Preferred: 30})`. Berikan `Minimum` dan `Maximum` untuk memberi rentang pada display link, seperti `{Minimum: 80, Maximum: 120, Preferred: 120}` pada layar ProMotion. Panggil dari cakupan milik jendela itu sendiri. Preferensi ini berlaku untuk `RequestAnimationFrame` dan `Animate` hingga diubah lagi. Rentang nol mengembalikan laju normal tampilan. Waktu pemanggilan balik tetap berupa waktu tampilan aktual. Tampilan dan OS menentukan laju penyegaran yang tersedia, dan NSGo dapat melewati frame tampilan untuk mengatur ritme pemanggilan balik.


`Scope.Tween` melangkahkan angka pada jam tersebut untuk hal-hal yang tidak dapat dijangkau nilai teranimasi, seperti angka yang menghitung naik dalam teks: fungsi dipanggil dengan setiap angka dari nilai awal ke akhir sebagaimana `Timing` menggerakkan nilai, dan sekali lagi dengan nilai terakhir. Langkah `nsgotest` yang memulainya akan menunggu hingga nilai terakhir, seperti halnya animasi, dan saat pengurangan gerak aktif fungsi langsung memberikan nilai terakhir. `Animated.tween` di TSX bekerja dengan cara yang sama.

```go
freed := core.NewSignal(cx, 0.0)
cx.Tween(0, 18.7, core.TimingConfig{Duration: 600 * time.Millisecond, Easing: core.BezierEasing(core.EaseOut)}, freed.Set, nil)

core.Text{ContentFunc: func() string { return fmt.Sprintf("%.1f GB can be freed", freed.Get()) }}
```

## Transisi

`Transition` menganimasikan gaya atau kelas suatu tampilan saat berubah, langsung di platform tanpa membebani mesin per frame. `Properties` menentukan apa yang bergerak; nilai 0 berarti semua properti selain frame.

```go
core.View{Style: core.Style{
	Background:  core.RGB(0xffffff),
	CornerRadius: 12,
	Transition:  core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionBackground | core.TransitionTransform},
	Transform:   core.Transform{Scale: 1},
	Hover:       &core.Style{Transform: core.Transform{Scale: 1.02}, Background: core.RGB(0xf5f5f5)},
}}
```

`TransitionLayout` memindahkan dan mengubah ukuran tampilan dari posisi tampilannya saat gayanya sendiri memberinya frame baru. Sebaliknya, animasi tata letak menganimasikan seluruh komit sekaligus, seperti `LayoutAnimation` di React Native: `cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` menganimasikan pemindahan, penyisipan, dan penghapusan pada batch berikutnya, dan presetnya mencakup easing, linear, serta pegas (*spring*). Transisi `LayoutSpring` menerima `Response` (periode alami pegas tanpa redaman) dan `DampingRatio` (`< 1` kurang teredam, `1` teredam kritis, `> 1` terlalu teredam), berjalan hingga tenang alih-alih untuk durasi tetap, dan membawa posisi serta kecepatannya ke target baru saat diarahkan ulang di tengah jalan.

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

`View.OnTransitionEnd` berjalan setelah perubahan menyelesaikan transisinya, baik ada sesuatu yang bergerak maupun tidak. Pengurangan gerak menonaktifkan transisi dan animasi tata letak, jadi hormati pengaturan platform dengan memastikan status akhir tetap dapat dicapai.

## Gerak

`core.Motion` menggerakkan opasitas, translasi, skala, dan rotasi satu tampilan melalui keyframe secara bersamaan, dari `MotionSpec` milik aplikasi atau `MotionPreset` siap pakai: masuk (*entrances*), keluar (*exits*), penarik perhatian (*attention seekers*), dan perulangan (*loops*). Mengikat gayanya menetapkan tampilan dalam keadaan diam; `Play` menjalankan suatu spesifikasi, dan `Exit` membiarkan tampilan keluar bersama geraknya di dalam `core.Presence`.

```go
enter := core.SlideInUp.Spec()
motion := core.NewMotion(cx, &enter)
motion.Exit(core.FadeOut.Spec())

core.View{Style: core.Style{Animated: motion.Style()}, Children: content}
```

`core.Presence{When, Then, Else}` mempertahankan sebuah cabang hingga semua animasi keluar yang didaftarkannya selesai, sehingga sheet atau toast keluar dengan geraknya apa pun yang menghapusnya. `core.PresenceOf` menerapkan perilaku yang sama untuk kunci yang dapat dibandingkan dan fungsi konten, dan `MaxLeaving` membatasi jumlah cabang lama ketika suatu nilai berubah dengan cepat. `MotionPresets()` mencantumkan semua preset dan `Kind()` menyatakan apakah preset tersebut masuk, keluar, menarik perhatian, atau berulang.

## Keyframe

`KeyframeAnimation` mengulang keyframe di suatu platform: opasitas, skala, rotasi, dan translasi vertikal pada setiap offset, dengan durasi, penundaan, easing, dan jumlah iterasi. `KeyframesSpin`, `KeyframesPing`, `KeyframesPulse`, dan `KeyframesBounce` berasal dari Tailwind.

```go
core.View{Style: core.Style{Animation: core.KeyframeAnimation{
	Keyframes: core.KeyframesPulse,
	Duration:  2 * time.Second,
	Iterations: 0, // forever
}}}
```

`ValueKeyframes` menggerakkan nilai teranimasi melalui titik-titik henti seperti Web Animations, dengan offset, easing per segmen, penundaan, iterasi, dan arah. Gestur juga dapat menggerakkan nilai: `View.PanValues`, `PinchValue`, dan `RotationValue` menggerakkannya di utas utama dari posisinya saat gestur dimulai.

`core.PanEvent` menyertakan `Position` di dalam tampilan dan `PagePosition` di dalam halaman pada setiap fase pan, sehingga alat seret (*drag tools*) tidak perlu peristiwa tekan terpisah untuk memperoleh lokasi jari.

## Berikutnya

- [Gaya dan tema](styles.md): properti yang digerakkan oleh transisi.
- [Daftar dan pengguliran](lists.md): nilai yang mengikuti tampilan gulir.
- [Tata letak](layout.md): apa itu frame yang dianimasikan.
