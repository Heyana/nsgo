---
title: Basis kode
description: Di mana setiap bagian NSGo berada, dan dokumen yang menjelaskan alasannya.
group: Berkontribusi
order: 81
---

NSGo adalah satu modul Go. Aplikasi mengimpor paket-paketnya dari atas; backend setiap platform berada di bagian bawah dan menerima apa yang berubah sebagai view native untuk dibuat, diperbarui, dan dihapus.

## Paket

| Paket | Isinya |
| --- | --- |
| `nsgo` (root) | `nsgo.Run` dan `nsgo.NewView`, memilih backend berdasarkan build tag |
| `core` | API aplikasi: elemen, gaya, kelas, tema, signal, dan layanan; persiapan tata letak dan teks |
| `base` | Komponen tanpa gaya, masing-masing satu file, dan komponen kompleks sebagai paket (`base/dock`, `base/editor`) |
| `ui` | Komponen `base` dengan tampilan, dan paket seperti `ui/chat` |
| `mobile`, `desktop` | Fitur yang hanya dimiliki ponsel atau hanya desktop |
| `tsx` | Aplikasi TSX, bundler dan compiler-nya, serta modul yang diimpornya |
| `i18n`, `*/locales` | Katalog pesan dan kode yang dihasilkan darinya |
| `nsgotest` | Pengujian yang bertindak seperti pengguna, secara headless atau di suatu platform |
| `internal/engine`, `internal/driver`, `internal/layout` | Goroutine engine, protokol mount, dan tata letak |
| `internal/platform/<target>` | Backend: view native, teks, dan input untuk satu target |
| `cmd/nsgo` | Alat `nsgo` |
| `examples/` | Galeri dan aplikasi contoh lainnya |

Paket hanya mengimpor ke arah bawah; `internal/moduletest/layering_test.go` memeriksa graf impor pada setiap target.

## Dokumen desain

[ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) memetakan paket, thread, dan alur dari sebuah commit hingga ke layar, serta mencantumkan topik di `docs/architecture/`: satu per bagian, seperti tata letak, teks, pengguliran, tema, atau alatnya. Baca topik dari bagian yang Anda ubah sebelum mengubahnya; dokumen ini mencatat cara kerja bagian tersebut dan di mana ia berbeda dari [Native Shell](https://github.com/longbridge/nsh), yang desainnya di-porting oleh NSGo.

[AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) memuat aturan yang diikuti setiap perubahan, baik oleh manusia maupun agen pemrograman. Beberapa di antaranya membentuk sebagian besar perubahan:

- Setiap API memiliki satu rumah: `core` untuk elemen dan lingkungan, `base` untuk kontrol, `ui` untuk tampilannya, `mobile` dan `desktop` untuk satu jenis perangkat.
- `base` tidak memiliki tampilan; komponen `ui` adalah bentuk `base`-nya yang diberi tampilan.
- Tidak ada yang berubah secara implisit berdasarkan perangkat: tidak ada kontrol yang lebih besar di desktop atau lebih kecil di ponsel pada `core` atau `base`.
- View native di mana saja, sama di setiap platform: apa yang dimiliki satu platform secara native, ditiru oleh platform lain.
- Tidak ada kode C tulisan tangan, dan tidak ada yang dibutuhkan selain macOS, Xcode, dan Go untuk membangun setiap target kecuali emulator Android x86_64.

Apa yang masih kurang, platform demi platform, ada di [peta jalan](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md).
