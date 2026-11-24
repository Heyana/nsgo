---
title: Membuat perubahan
description: Apa yang dibutuhkan pull request ke NSGo, mulai dari perubahan yang sama di setiap backend hingga pengujian, dokumentasi, dan baris changelog.
group: Berkontribusi
order: 82
---

## Sebelum Anda mulai

Buka issue atau diskusi untuk apa pun yang lebih besar daripada perbaikan kecil, agar desainnya disepakati sebelum menulis kode. Pertanyaan yang masih dibiarkan terbuka oleh desain tercantum di [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md#open-questions); jawabannya diputuskan di sana terlebih dahulu.

## Setiap backend

Perbaikan atau fitur di satu backend biasanya juga perlu ada di backend lainnya. Buat perubahan yang sama pada setiap target yang membutuhkannya, atau catat apa yang masih kurang di [peta jalan](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md). Komponen harus terlihat dan berperilaku sama di setiap platform.

## Pemeriksaan

Gunakan alat `nsgo` alih-alih `go vet`, `go test`, atau `go build`:

```sh
go tool nsgo check   # vet every package
go tool nsgo test    # every package's tests
```

Tambahkan pengujian untuk apa yang Anda ubah: `nsgotest` untuk apa yang dilihat pengguna, dan pengujian paket itu sendiri untuk sisanya. Jalankan galeri pada platform yang Anda sentuh ([Pengaturan pengembangan](../contributing.md#the-gallery)).

Format file Go dengan `gofmt` dari versi Go di `go.mod`; `gofmt -l .` mencantumkan file yang perlu diformat. CI memeriksa setiap file Go yang dilacak dan gagal dengan menampilkan nama file yang belum diformat.

## Pull request

Beri judul sebagai Conventional Commit, seperti `fix(linux): keep the caret after a paste`: squash merge menjadikan judul itu sebagai subjek commit, dan pemeriksaan akan gagal hingga formatnya sesuai. Templat meminta target yang disentuh perubahan, tangkapan layar sebelum dan sesudah untuk apa pun yang terlihat, serta pemeriksaan yang telah Anda jalankan.

## Dokumentasi dan changelog

- Perbarui dokumen yang faktanya berubah: topik di `docs/architecture/` untuk cara kerja NSGo, dan halaman di `docs/public/` untuk cara aplikasi dan komponen menggunakannya ([Dokumentasi dan situs web](docs.md)).
- Jaga agar `README.md` dan `README.zh-CN.md` tetap setara; semua dokumen lainnya berbahasa Inggris, dengan halaman berbahasa Mandarin di `docs/public/zh/`.
- Catat perubahan yang dapat dilihat pengguna dalam satu baris, dengan referensi seperti `(#12 by @you)`, pada file tersendiri di `changelog.d/` di bawah judul jenisnya, seperti `### Fixed`, alih-alih di `CHANGELOG.md`, yang diisi saat rilis dari file-file tersebut ([format](https://github.com/nsgo-dev/nsgo/blob/main/changelog.d/README.md)).
