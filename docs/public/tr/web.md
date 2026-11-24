---
title: Web'de yayınlama
description: Bir NSGo uygulamasını belge anlamlarıyla, sayfa meta verileriyle ve CDN varlıklarıyla yayınlayın.
group: Araç zinciri
order: 24
---

## Uygulamayı derleyin

```sh
go tool nsgo build -target web
```

Oluşturulan dizini HTTP üzerinden sunun. Sürüm varlıklarının adlarında içerik karmaları bulunur, bu yüzden onları değişmez (immutable) olarak önbelleğe alın; `index.html`, `nsgo.json` ve `manifest.webmanifest` dosyalarını ise yeniden doğrulayın. Web uygulaması NSGo'nun yerleşim, kaydırma, odak ve işaretçi davranışını korur.

## Başlıklar ve bağlantılar

`Role: core.RoleHeader` içeren bir `core.View` veya `core.Text`, `AccessibilityLevel` ile seçilen `h1` ile `h6` arasında bir öğe olur; düzey yoksa `h2` olur. `base/textview`'dan gelen Markdown başlıkları kendi düzeylerini taşır. Tarayıcının başlık kenar boşlukları ve yazı tipleri uygulamanın stilini değiştirmez.

`base.Link` ve `ui.Link`, `Href` değerlerini bir bağlantı (anchor) olarak yayınlar. Sıradan bir tıklama uygulamanın `OnOpen` ve `OnPress` işleyicilerini çalıştırır; orta tıklama veya bir değiştirici tuşla yapılan tıklama tarayıcının bağlantı eylemini kullanır. Uygulama `OnContextMenu`'yu işlemedikçe tarayıcı kendi bağlantı bağlam menüsünü sağlar. Devre dışı bir bağlantının etkin bir adresi yoktur. Özel bir bağlantı rollü görünüm için `core.View.Web` değerini `core.WebViewProps{Href: "/notes"}` olarak ayarlayın veya `HrefFunc` bağlayın. TSX `web={{href: "/notes"}}` kullanır; başlıklar `accessibilityLevel={1}` kullanır. Güvensiz yürütülebilir URL şemaları reddedilir. Sınırlandırılmış içerik gezinmeyi kapsamlı basma işleyicilerinde tutar ve hiçbir tarayıcı adresi duyurmaz.

## Sayfa meta verileri

`github.com/nsgo-dev/nsgo/web` paketini içe aktarın ve bir sayfanın içeriğinin yanına bir `web.Head` koyun:

```go
core.Group{
	web.Head{Title: "Notes", Description: "Your saved notes"},
	pageContent,
}
```

`TitleFunc` ve `DescriptionFunc` sinyalleri izler. Sonraki görünür başlık öğeleri (head) ayarladıkları alanları geçersiz kılar, diğerlerini devralır. Bir sayfayı gizlemek veya kaldırmak önceki başlık öğelerini geri getirir; tüm başlık öğelerini kaldırmak özgün belgeyi geri getirir. Boş bir statik alan devralır; boş dize döndüren bir işlev ise alanı temizler. Bir başlık öğesi yerleşim düğümü eklemez. Yerel hedefler, gömülü uygulamalar ve sınırlandırılmış içerik barındırıcılarının belgesine dokunmaz.

TSX'te `Head`'i `nsgo/web`'den içe aktarın ve `<Head title={title()} description="Your saved notes" />` yazın. Statik HTML önizlemeleri için her URL'yi sitenin dağıtımının bir parçası olarak önceden işleyin; `Head` çalışan belgeyi günceller.

## Varlıkları bir CDN'den sunun

Yükleyici tüm uygulama dosyalarını `nsgo.json`'a göre çözümler. Bir sayfa uygulamayı başka bir kaynaktan (origin) yükleyebilir:

```js
const url = new URL("https://cdn.example.com/notes/nsgo.json");
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url).href);
await mount(null, url.href, { config });
```

Sayfanın kaynağının CDN'in dosyalarını CORS ile almasına izin verin ve WebAssembly'yi `application/wasm` olarak sunun. Uygulamanızın kullandığı tüm derin yollar için uygulamanın giriş HTML'sini sunun. [Gömme](embedding.md#web) bunun yerine bir öğeye nasıl bağlanacağını gösterir.
