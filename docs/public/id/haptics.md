---
title: Haptik
description: Putar umpan balik haptik bawaan atau rangkaian pulsa berwaktu.
group: Fitur platform
order: 46
---

## Memutar satu umpan balik

`mobile.PlayHaptic` memutar umpan balik di bawah jari pengguna pada iOS dan Android, dan tidak melakukan apa pun di platform lain, jadi pasangkan dengan apa yang dilihat pengguna. `HapticSelection` menandai perubahan nilai, langkah pada picker, segmen, atau slider; `HapticLight` hingga `HapticRigid` menandai benturan antar-elemen, berdasarkan bobot dan kelenturannya; dan `HapticSuccess`, `HapticWarning`, serta `HapticError` menandai tugas yang berakhir sesuai hasil tersebut.

```go
mobile.PlayHaptic(cx, mobile.HapticSelection)
```

## Memutar pola

`cx.Env().CanPlayHapticPattern()` melaporkan apakah haptik berwaktu tersedia. Sebuah pola berisi pulsa dengan waktu mulai `At` yang relatif terhadap pola tersebut. `Duration` bernilai nol menghasilkan getaran sesaat (*transient*), dan durasi positif mempertahankan pulsa hingga 30 detik. `Intensity` dan `Sharpness` berkisar dari 0 hingga 1. Pola baru menggantikan pola sebelumnya. Berikan pola kosong untuk menghentikan pemutaran. Pola berhenti saat aplikasi berpindah ke latar belakang.

```go
mobile.PlayHapticPattern(cx, mobile.HapticPattern{
    {Intensity: 0.25, Sharpness: 0.4},
    {At: 100 * time.Millisecond, Intensity: 0.55, Sharpness: 0.6},
    {At: 200 * time.Millisecond, Intensity: 1, Sharpness: 0.8},
})
```

iOS memutar event tersebut melalui Core Haptics pada perangkat keras yang mendukung. Android mengikuti pengaturannya dan menggunakan intensitas apabila vibrator mendukung kontrol amplitudo. Browser Android dengan `navigator.vibrate` mengikuti waktu nyala dan mati setelah adanya interaksi pengguna. Android dan web memutar getaran sesaat sebagai pulsa 20 ms, dan ketajaman (*sharpness*) tersedia di iOS. Pada platform yang tidak mendukung, pemanggilan ini tidak berpengaruh.

Dalam pengujian, atur `HapticPatterns: true` melalui `nsgotest.Environment` lalu baca `app.HapticPatterns`. Riwayatnya menyertakan entri nil saat aplikasi membatalkan pemutaran.

## Kapan digunakan

Putar saat gestur terjadi, bukan saat respons tiba: pilihan saat nilai terlewati, tumbukan saat tampilan mendarat, hasil saat tugas selesai. Putar untuk perubahan yang disebabkan oleh pengguna, dan jangan putar untuk hal yang dilakukan aplikasi dengan sendirinya.

## Selanjutnya

- [Notifikasi](notifications.md): notifikasi dan lencana (*badge*) yang menyampaikan kabar yang sama.
- [Layanan platform](services.md): fitur lain yang ditawarkan oleh sistem.
- [Aksesibilitas](accessibility.md): pengumuman yang didengar pengguna yang tidak dapat merasakan getaran.
