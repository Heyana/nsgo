---
title: Belgeler ve web sitesi
description: "Bu belgelerin bir sayfasını yazın, çevirin, web sitesini çalıştırın ve ajanların becerisini güncel tutun."
group: Katkıda bulunma
order: 83
---

## Bir sayfa

Bu belgeler `docs/public/` içindeki Markdown dosyalarıdır ve oldukları gibi GitHub'da okunabilirler. `en/` ve `zh/` yol yol birbirini yansıtır; Çince sürümü eksik olan bir sayfa bir uyarıyla İngilizce sürümü gösterir. Her sayfa ön meta verilerle (front matter) başlar:

```yaml
---
title: Testing
description: Test an app the way a user uses it, headless or in a native window.
group: Toolchain
order: 21
---
```

- `group`, kenar çubuğunda sayfanın kendi dilindeki grubudur ve `order` onun sırasını belirler. Gruplar bir okuyucunun giriş yolunu izler: öğreticiler, araç zinciri, bileşenler, temel kavramlar, ardından bileşen yazma ve katkıda bulunma. Giriş gibi grubu olmayan bir sayfa bunlardan önce gelir.
- Düzen başlığı ve açıklamayı gösterdiği için gövde `##` ile başlar.

Sayfalar arasında göreli `.md` bağlantılarıyla (`[Test etme](testing.md#run-on-a-platform)`) bağlantı verin; site bunları kendi URL'lerine dönüştürür. Her sayfa tek bir soruyu yanıtlar; NSGo'nun içeride nasıl çalıştığı `docs/architecture/` içinde kalır.

## Web sitesi

`website/` içindeki web sitesi, `website/site.config.ts` içindeki ayarlardan bir Astro tümleştirmesi olan [nsgo-website](https://github.com/nsgo-dev/nsgo-website) tarafından derlenir. [Bun](https://bun.com) 1.4 veya üzerini gerektirir:

```sh
cd website
bun install
bun run dev    # at http://localhost:4321/
bun run build  # type-check and build, as the deployment does
```

`website/README.md` sitenin nasıl derlenip yayınlandığını anlatır.

## Ajanlar için

Web sitesi İngilizce sayfaları dil modelleri için de yayınlar: `/llms.txt` bunları listeler, `/llms-full.txt` hepsini barındırır ve `/docs/<page>.md` ise mutlak bağlantılarla ve kod bloklarının işaretleri olmadan tek bir sayfayı tutar (`src/lib/llms.ts`). Bunlar kendilerine ait ek bir adım olmadan sayfaları izler.

`skills/nsgo/`, ajanların `bunx skills add nsgo-dev/nsgo` ile kurduğu beceridir: bir uygulamanın izlediği kuralları içeren `SKILL.md` ve bir görev gerektirdiğinde okunan `references/`. Bir uygulamanın yazdıkları, aracın komutları, test etme veya paketleme değiştiğinde bunu belgelerle aynı değişiklikte güncelleyin; burayı bir ajanın uyması gerekenlerle sınırlı tutun ve geri kalanı için belgelere bağlantı verin. `go tool nsgo test -run TestSkills .` ön meta verilerini, bağlantılarını ve bahsettiği her NSGo adını denetler.
