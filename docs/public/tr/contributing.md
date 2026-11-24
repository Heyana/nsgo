---
title: Geliştirme kurulumu
description: "NSGo'yu kaynaktan derleyin, galeriyi çalıştırın ve bir değişikliği test edin."
group: Katkıda bulunma
order: 80
---

## Gereksinimler

Xcode ve Go 1.27.3 veya üzeri kurulu macOS, Android APK'sı dahil arm64 için her hedefi derler. Cihazlarda çalıştırmak [Kurulum](getting-started.md#requirements) sayfasında listelenenleri gerektirir ve x86_64 Android emülatörünün hedefi olan `android_x64` ise Go yalnızca cgo ile android/amd64 bağlantısı kurduğundan bir Android NDK gerektirir. Windows kurulum ve kaldırma programları yalnızca Go ile derlenir.

```sh
git clone https://github.com/nsgo-dev/nsgo.git
cd nsgo
```

Kod değiştirmeden önce [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) ve [AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) belgelerini okuyun.

## Galeri

Galeride her öğenin, `base` ve `ui` bileşeninin ve servisin bir demosu bulunur. Geniş bir pencere demoları kenar çubuğundaki sekmelerin altında gösterir; dar bir pencere ise sekmeleri ve ayarları alt çubukta gösterir ve demolar onların üzerine itilir. Depo kökünden çalıştırın:

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev examples/gallery-tsx               # the gallery's components and apps, written in TSX
go tool nsgo dev -target web examples/gallery       # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # on the connected iPhone, signed by Xcode
go tool nsgo dev -target android examples/gallery   # on the connected device or emulator, over adb
go tool nsgo build -target all examples/gallery     # every platform's packages in examples/gallery/dist, what to hand out in dist/releases
```

## Denetleme ve test etme

```sh
go tool nsgo check                 # vet every package
go tool nsgo check -target all     # vet every available Go/Xcode target instead
go tool nsgo test                  # every package's tests
go tool nsgo test -bench . ./core  # core's benchmarks
(cd benchmark && go run ./runner)  # measure against other frameworks
# capacity curve for NSGo's 600-cell grid, without rebuilding it
(cd benchmark && go run ./runner -frameworks nsgo -cases grid -grid-cols 20 -grid-rows 15 -out /tmp/grid.json)
```

`-target`, ana bilgisayar yerine adlandırdığı hedefleri ve NSGo'nun deposunda geliştirici istemcisini (yalın `check` bunu denetler) denetler (`go vet`). Bir değişikliğin dokunduğu platformları denetlemek için birden fazla başlatmak yerine hepsini tek bir denetlemede adlandırın, örneğin `check -target web,android -jobs 2`: `-jobs n` aynı anda en fazla n hedefi, her `go vet` işlemini n derleme çalışanıyla denetler; varsayılan ise Go'nun varsayılanıyla aynı anda bir hedeftir ve aynı modülde bir diğeri çalışırken başlatılan bir denetleme onu bekler ve bunu bildirir. CI'daki `check.yml` her hedefi denetler.

Depo kökünde veya `internal/dev` içinde çalıştırıldığında, başsız komut `internal/dev` paketini geliştirici istemcisinin dosyalarının ihtiyaç duyduğu `nsgodev` derleme etiketiyle ikinci kez test eder; böylece testleri yerel olarak ve CI'da çalışır. Her iki çalıştırma da aynı filtreleri ve bayrakları alır ve tek bir sayımı ve JUnit raporunu paylaşır; burada ikinci çalıştırmanın paket adı `internal/dev [nsgodev]` olur.

`check -target all`, herhangi bir ana bilgisayarda macOS, Windows, Linux, Android arm64 ve web hedeflerini denetler. Xcode SDK'ları mevcut olduğunda iOS, tvOS ve watchOS'u da denetler ve atladıklarını listeler. `check -target ios` gibi açıkça adlandırılmış bir hedef, `all` ile birleştirildiğinde bile araçları mevcut değilse başarısız olur. Başarısız olan bir hedef kalan hedeflerin denetlenmesini engellemez ve gerekli herhangi bir denetleme başarısız olursa komut başarısız olur. `build -target all` hâlâ Xcode'lu macOS gerektirir. NSGo'nun kendi deposunda adlandırılan her hedef platform test barındırıcılarını `nsgotest` ile de denetler; CI, `all` dışında olduğu için önizleme `harmonyos` hedefini açıkça adlandırır.

Linux CI işi, HarmonyOS Go arka ucunun `js/wasm` testlerini Node.js ve Go'nun Wasm başlatıcısı ile çalıştırır. Galeri testini yerel olarak `hdc` üzerinden bağlı bir HarmonyOS emülatörü ile çalıştırın. [HarmonyOS barındırıcı şablonlarını](building.md#harmonyos) derlemek OpenHarmony SDK API 26 gerektirir.

```sh
go tool nsgo test -env GOOS=js -env GOARCH=wasm ./internal/platform/harmonyos \
  -- "-exec=env -i \"PATH=$PATH\" \"$(go env GOROOT)/lib/wasm/go_js_wasm_exec\"" -count=1
go tool nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery
```

Bir Linux masaüstünde Wayland oturumunda gerçek genel kısayol portalı kabul denetimini `NSGO_TEST_GLOBAL_SHORTCUT_PORTAL=<installed-desktop-id> go tool nsgo test -run TestGlobalShortcutPortalWithPermission ./internal/platform/linux` ile çalıştırın. Kimliğin bir `.desktop` girdisi olmalıdır. Birleştiricinin isteminde Ctrl+Alt+K'ye izin verin, ardından başka bir uygulama odaktayken bu tuşlara basın; denetim hem kaydı hem de etkinleştirmeyi bekler. Ortam değişkeni ayarlanmadıkça atlanır.

Bir Linux ana bilgisayarında Docker mevcut olduğunda, `go tool nsgo test -run '^TestLinuxPackagesNative$' -env NSGO_TEST_LINUX_PACKAGES=1 ./cmd/nsgo -- -timeout=20m` tek kullanımlık Debian, Ubuntu ve Arch kapsayıcılarında gerçek kurulumları denetler. Resmi imajlarını indirir ve içlerine GTK bağımlılıklarını kurar, her paket veritabanını ve kurulu dosyaları doğrular, ardından paketi kaldırır. Ana bilgisayarın paket veritabanı asla kullanılmaz.

## Sürümler

NSGo, alt akış bir proje etiketlenmiş bir sürüme ihtiyaç duyduğunda veya bakımcılar gönderilmeye hazır değişikliklere sahip olduğunda talep üzerine sürüm yayınlar. Bir kütüphane için sürüm talep ederken bağımlı çalışmayı bağlayın ve ihtiyaç duyduğu son NSGo değişikliğini içeren commit'i adlandırın. Bu commit'i içeren ilk sürüm etiketi, kütüphanenin gerektirebileceği sürümdür.

Birleştirilmiş bir değişikliği etiketlenmeden önce test etmek için bağımlı modülden `go get github.com/nsgo-dev/nsgo@main` çalıştırın. Doğruladığınız revizyonu seçmek için `main` yerine bir commit kimliği kullanabilirsiniz. Go elde edilen sözde sürümü `go.mod` dosyasına kaydeder.

Bir bakımcı değişikliklere `CHANGELOG.md` içinde sürüm ve tarih verir, yeni bir Unreleased bölümü bırakır ve `main` üzerindeki ilgili commit'i etiketler. [Sürüm iş akışı](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/release.yml) etiketi denetler ve test iş akışı geçtikten sonra yayınlar; yayınlanan notlar daha sonra yalnızca yayınlanmamış değişiklikleri tutan `CHANGELOG.md` dosyasından [Sürümler](https://github.com/nsgo-dev/nsgo/releases) sayfasına geçer.

## NSH

NSGo tasarım referansı olarak [Native Shell](https://github.com/longbridge/nsh) projesini okur; testlerinin hiçbiri ona ihtiyaç duymaz. Okumak için deponun yanına klonlayın ve asla `main` değil, en son sürüm etiketini alın:

```sh
git clone https://github.com/longbridge/nsh.git ../nsh
git -C ../nsh checkout "$(git -C ../nsh describe --tags --abbrev=0)"
```

## Sonraki adımlar

- [Kod tabanı](contributing/codebase.md): NSGo'nun her bir parçasının nerede yaşadığı.
- [Değişiklik yapma](contributing/changes.md): bir çekme isteğinin gerektirdikleri.
