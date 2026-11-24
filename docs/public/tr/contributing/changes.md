---
title: Değişiklik yapma
description: "Bir NSGo çekme isteğinin gerektirdikleri; her arka uçta aynı değişiklikten testlere, belgelere ve değişiklik günlüğü satırına kadar."
group: Katkıda bulunma
order: 82
---

## Başlamadan önce

Bir düzeltmeden daha büyük her şey için bir sorun (issue) veya tartışma (discussion) açın ki koddan önce tasarım üzerinde uzlaşılsın. Tasarımın açık bıraktığı sorular [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md#open-questions) içinde listelenmiştir; yanıtları önce orada kararlaştırılır.

## Her arka uç

Bir arka uçtaki bir düzeltme veya özellik genellikle diğerlerine de aittir. Gereken her hedefte aynı değişikliği yapın veya eksik olanları [yol haritasına](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md) kaydedin. Bileşenler her platformda aynı görünür ve aynı davranır.

## Denetlemeler

`go vet`, `go test` veya `go build` yerine `nsgo` aracını kullanın:

```sh
go tool nsgo check   # vet every package
go tool nsgo test    # every package's tests
```

Değiştirdiğiniz şey için bir test ekleyin: kullanıcının gördükleri için `nsgotest`, geri kalanı için paketin kendi testleri. Dokunduğunuz platformlarda galeriyi çalıştırın ([Geliştirme kurulumu](../contributing.md#the-gallery)).

Go dosyalarını `go.mod` içindeki Go sürümünün `gofmt` aracıyla biçimlendirin; `gofmt -l .` biçimlendirme gerektiren dosyaları listeler. CI izlenen her Go dosyasını denetler ve biçimlendirilmemiş olanların adlarıyla başarısız olur.

## Çekme isteği

Başlığını `fix(linux): keep the caret after a paste` gibi bir Conventional Commit olarak yazın: squash merge işlemi başlığı commit konusu yapar ve uyana kadar bir denetim başarısız olur. Şablon, değişikliğin dokunduğu hedefleri, görünür her şeyin öncesi ve sonrası ekran görüntülerini ve çalıştırdığınız denetlemeleri ister.

## Belgeler ve değişiklik günlüğü

- Olguları değişen belgeyi güncelleyin: NSGo'nun nasıl çalıştığı için `docs/architecture/` altında bir konu, uygulamaların ve bileşenlerin onu nasıl kullandığı için `docs/public/` altında bir sayfa ([Belgeler ve web sitesi](docs.md)).
- `README.md` ve `README.zh-CN.md` dosyalarını eşdeğer tutun; bunun dışındaki her şey İngilizcedir ve Çince sayfalar `docs/public/zh/` içindedir.
- Kullanıcıların görebileceği bir değişikliği `CHANGELOG.md` yerine — ki sürüm yayınlanırken bu dosyalardan doldurulur — `changelog.d/` içinde kendi dosyasına, `### Fixed` gibi türünün başlığı altına, `(#12 by @you)` şeklinde bir referansla tek satır olarak kaydedin ([biçim](https://github.com/nsgo-dev/nsgo/blob/main/changelog.d/README.md)).
