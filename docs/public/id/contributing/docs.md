---
title: Dokumentasi dan situs web
description: Tulis halaman dokumentasi ini, terjemahkan, jalankan situs web, dan perbarui skill agen.
group: Berkontribusi
order: 83
---

## Sebuah halaman

Dokumentasi ini adalah Markdown di `docs/public/`, yang dapat dibaca langsung di GitHub apa adanya. `en/` dan `zh/` saling mencerminkan jalur demi jalur; halaman yang belum ada dalam bahasa Mandarin akan menampilkan versi bahasa Inggris dengan pemberitahuan. Setiap halaman dimulai dengan front matter:

```yaml
---
title: Testing
description: Test an app the way a user uses it, headless or in a native window.
group: Toolchain
order: 21
---
```

- `group` adalah grup halaman di sidebar, dalam bahasa halaman tersebut, dan `order` menentukan posisinya. Grup disusun mengikuti alur masuk pembaca: tutorial, perangkat bantu, komponen, konsep inti, lalu menulis komponen dan berkontribusi. Halaman tanpa grup, seperti pengantar, muncul sebelum grup-grup tersebut.
- Tata letak menampilkan judul dan deskripsi, sehingga isi halaman dimulai dari `##`.

Tautkan antarhalaman menggunakan tautan `.md` relatif (`[Pengujian](testing.md#run-on-a-platform)`); situs akan mengubahnya menjadi URL-nya. Setiap halaman menjawab satu pertanyaan; bagaimana NSGo bekerja di dalamnya tetap berada di `docs/architecture/`.

## Situs web

Situs web di `website/` dibangun oleh [nsgo-website](https://github.com/nsgo-dev/nsgo-website), sebuah integrasi Astro, dari pengaturan di `website/site.config.ts`. Situs ini membutuhkan [Bun](https://bun.com) 1.4 atau yang lebih baru:

```sh
cd website
bun install
bun run dev    # at http://localhost:4321/
bun run build  # type-check and build, as the deployment does
```

`website/README.md` menjelaskan bagaimana situs ini dibangun dan diterbitkan.

## Untuk agen

Situs web juga menerbitkan halaman berbahasa Inggris untuk model bahasa: `/llms.txt` mencantumkannya, `/llms-full.txt` memuat semuanya, dan `/docs/<page>.md` memuat satu halaman, dengan tautan absolut dan tanpa tanda blok kode (`src/lib/llms.ts`). Halaman-halaman tersebut mengikuti halaman dokumentasi tanpa langkah tambahan tersendiri.

`skills/nsgo/` adalah skill yang dipasang agen dengan `bunx skills add nsgo-dev/nsgo`: `SKILL.md` berisi aturan yang diikuti aplikasi, dan `references/` dibaca saat suatu tugas membutuhkannya. Ubah skill tersebut bersamaan dengan dokumentasi apabila cara penulisan aplikasi, perintah alat, pengujian, atau pengemasan berubah; batasi isinya pada hal yang wajib diikuti agen dan tautkan ke dokumentasi untuk selebihnya. `go tool nsgo test -run TestSkills .` memeriksa front matter-nya, tautan-tautannya, dan setiap nama milik NSGo yang disebutkannya.
