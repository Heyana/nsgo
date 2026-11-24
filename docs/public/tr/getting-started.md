---
title: Kurulum
description: NSGo'nun ihtiyaç duyduklarını kurun, bir uygulama oluşturun ve her hedefte çalıştırın.
group: Başlarken
order: 10
---

## Gereksinimler

- [Go](https://go.dev/dl/) 1.27.3 veya sonrası.
- iOS, tvOS ve watchOS için derlemek üzere Xcode yüklü bir Mac. Diğer tüm hedefler macOS'ta yalnızca Go ile derlenir; x86_64 Android emülatörününki ayrıca bir Android NDK gerektirir.
- Bir iPhone'da çalıştırmak için Xcode'un Accounts bölümünde bir Apple ID ve Developer Mode açık, güvenilir olarak işaretlenmiş bir iPhone.
- Android'de çalıştırmak için Android SDK'nın platform-tools paketinden `adb`.
- Windows'ta çalıştırmak için yalnızca Go; Windows ve web paketlerini de o derler.
- Linux'ta GTK 4.14 veya daha yenisi, Pango ve Cairo. Ağ görüntüleri libsoup 3, web görünümleri WebKitGTK 6.0, ses ve video ise GStreamer eklentileri kullanır. AppArmor kullanıcı ad alanı kısıtlamaları olan sistemlerde [WebKitGTK bir bubblewrap profiline ihtiyaç duyar](debugging.md#linux-web-views).

## Bir uygulama oluşturun

Bir modül oluşturun, NSGo'yu ve aracını ekleyin, ardından uygulamayı araca yazdırın:

```sh
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
```

`go get -tool`, `nsgo`'yu modüle bir kez ekler; böylece `go tool nsgo` hiçbir şey kurmadan onu çalıştırır. `init` şunları yazar:

| Dosya | Ne olduğu |
| --- | --- |
| `app.go` | Uygulama, bir sayaç: bir `Root` bileşeni ve `nsgo.Run(Root)` çağıran `main` |
| `app_test.go` | Düğmeye basan bir test |
| `nsgo.yaml` | Uygulamanın adı, kimliği, sürümü ve hedefleri |

`go tool nsgo init -template tsx` bunun yerine bir TSX uygulaması yazar; `-template mixed` ise TSX girişi kendi Go bileşenlerini kullanan bir Go uygulaması yazar.

### Go modüllerinden şablonlar

Şablonun modül yolunu isteğe bağlı bir sürümle birlikte verin. Sürüm belirtilmezse Go'nun modül proxy'sinin bulabildiği en son sürüm kullanılır.

```sh
go tool nsgo init -template example.com/my-template@v1.2.0 -module example.com/hello hello
```

Başlangıç şablonunu modülün `template/` dizininde yayımlayın. `nsgo.yaml` ve `main.go` dosyalarını ekleyin. Araç bu ikisini ve `.tmpl` ile biten her dosyayı Go'nun `text/template` paketiyle işler ve çıktı adlarından `.tmpl` uzantısını kaldırır. İkili varlıklar dahil diğer dosyaları değiştirmeden kopyalar.

Şablonlar `.Name` (uygulamanın görünen adı), `.ID` (üretilen uygulama kimliği), `.Slug` (küçük harfli, tireli ad) ve `.Module` (`-module` değeri veya slug) değerlerini alır. Go, iç içe `go.mod` dosyalarını modül indirmelerinin dışında tuttuğundan, üretilen uygulamanın modülünü ve bağımlılıklarını bildirmek için bir `go.mod.tmpl` gönderin. Böyle bir dosya yoksa `-module`, yerleşik bir şablon için yazdığıyla aynı en küçük `go.mod` dosyasını yazar.

## Çalıştırın

```sh
go tool nsgo dev
```

`dev` uygulamayı bu makine için derler ve açar. Her kaydetmede uygulamayı yeniden derleyip yeniden başlatır; yalnızca TSX'teki bir değişiklik ise çalışan uygulamayı yerinde günceller. Başarısız bir derleme, hatayı kopyalamak ve yeniden derlemek için düğmelerle birlikte uygulamanın üzerinde gösterilir.

`-target` ile başka bir yerde çalıştırın:

```sh
go tool nsgo dev -target web                # in the browser
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator
```

`go tool nsgo doctor` her hedefi, bu makinenin onu derleyip derlemediği ve çalıştırıp çalıştırmadığıyla birlikte listeler; yapamadıkları için de Xcode, `adb`, bağlı bir cihaz veya GTK gibi eksik olanları belirtir.

[Çalıştırma ve hata ayıklama](debugging.md) cihazları, güncellemelerin izlenmesini, performans monitörünü ve diğer dilleri anlatır.

## Bir kodlama ajanıyla

NSGo'nun becerisi (skill), Claude Code, Codex ve Cursor gibi kodlama ajanlarına bir NSGo uygulamasının nasıl yazıldığını, test edildiğini ve dağıtıldığını öğretir. Onu uygulamanın dizinine ekleyin ya da her proje için `-g` ile ekleyin:

```sh
bunx skills add nsgo-dev/nsgo
```

Belgeleri kendisi okuyan ajanlar <https://nsgo.dev/llms.txt> ile başlar; bu dosya her sayfayı `https://nsgo.dev/docs/<page>.md` adresinde Markdown olarak listeler; <https://nsgo.dev/llms-full.txt> ise hepsini içerir. Bir ajan çalışan uygulamayı da kullanabilir ([Çalıştırma ve hata ayıklama](debugging.md#agents)).

## Sonraki adımlar

- [Örnek: Yapılacaklar uygulaması](tutorials/todo.md): yaklaşık on dakikada durum, bir liste ve bir test.
- [Örnek: Not uygulaması](tutorials/notes.md): bir gezinme yığını ve cihazda saklanan notlar.
- [Örnek: Popüler GitHub depoları](tutorials/repos.md): hatalar ve yeniden deneme ile ağdan yükleme.
