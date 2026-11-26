---
title: Derleme ve dağıtım
description: Bir uygulamayı her platform için paketleyin, yükleyiciler oluşturun, imzalayın ve noter onayı alın, GitHub'da yayınlayın ve başka uygulamalara gömün.
group: Araç zinciri
order: 22
---

## Derleme

```sh
go tool nsgo build
```

`build`, `nsgo.yaml` dosyasının listelediği her hedef için soyulmuş (stripped) bir sürüm paketini `dist/<name>-<version>-<target>` içine yazar: bir `.app`, `.ipa`, `.apk`, Windows ve Linux dizinleri ya da sayfası bir `import()` çağrısının adlandırdığı her TSX modülünü içe aktarma çalıştığında getiren bir web sitesi. `-target` listeyi değiştirir ve `all` yalnızca Go ve Xcode gerektiren her hedef anlamına gelir; Android NDK kuruluysa `android_x64`, [mini programlar](#mini-programlar) için `miniprogram` (ya da tek başına `miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` veya `miniprogram_douyin`) ve bir [HarmonyOS](#harmonyos) önizlemesi için `harmonyos` ekleyin. `-run` bu makine için derlenen paketi başlatır ya da `-device` ile seçilen iPhone, simülatör veya Android cihaza yükler.

Dağıtılacak her şey bakılacak tek dizin olan `dist/releases` içine düşer:

| `dist/releases` içinde | Nedir |
| --- | --- |
| `*.dmg`, `*-setup.exe`, `*.deb`, `*.pkg.tar.gz`, `*.AppImage` | macOS, Windows ve Linux için [Yükleyiciler](#yükleyiciler) |
| `*.AppImage.zsync` | AppImageUpdate ve uyumlu başlatıcılar için delta güncelleme denetimleri |
| `*-ios.ipa`, `*-tvos.ipa`, `*.apk` | Mağazalar veya cihazlar için iOS, tvOS ve Android paketleri |
| `*.pkg`, `*.msix`, `*.aab`, `*.flatpak`, `*.tar.gz` ve `flatpak/` | Bir `store` derlemesinin [mağaza paketleri](#mağaza-paketleri) |
| `install.sh`, `install.ps1` | Uygulamayı [GitHub sürümlerinden](#githubda-yayınlama) yükleyen betikler |
| `appcast-<target>.xml` | Uygulamaların [güncelleme](#güncellemeler) yaparken okuduğu akışlar |
| `homebrew/<name>.rb`, `winget/` | Doğrudan sürüm [Homebrew cask ve winget bildirimleri](#kanallar) |
| `*-web.zip` | Olduğu gibi açılıp sunulacak web sitesi |
| `miniprogram/<platform>/` | Geliştirici aracından yüklenecek her mini programın projesi |

Her dosya `hello-1.2.0-windows-x64-setup.exe` gibi `<name>-<version>-<system>-<arch>` biçiminde adlandırılır, böylece kullanıcılar bir sürüm sayfasında kendi sistemlerine uygun olanı bulur ve yanında `.sha256` dosyası bulunur. Başka bir sürümün derlemesi önce dizini boşaltır; aynı sürümün (başka makinelerdeki dahil) derlemeleri dizine eklenir. Simülatörlerin ve watchOS'un paketleri kendi `dist/<name>-<version>-<target>` dizinlerinde kalır.

Web sitesi, özgün modülden daha küçük olduklarında `.wasm.br` ve `.wasm.gz` dosyalarını içerir. Bunları destekleyen sunucularda önceden sıkıştırılmış dosyaları etkinleştirin. Bunları özgün `.wasm` URL'sinde `Content-Type: application/wasm`, eşleşen `Content-Encoding` ve `Vary: Accept-Encoding` ile sunun. Özgün modül herhangi bir statik sunucuda çalışır. [Web'de yayınlama](web.md) anlamsal başlıkları, tarayıcı bağlantılarını, sayfa başına meta verileri ve CDN yüklemesini kapsar.

### Çevrimdışı web paketleri

Bir web sürümünü ilk çevrimiçi başlatışında önbelleğe almak için `nsgo.yaml` içinde `web.offline: true` ayarlayın.

```yaml
web:
  offline: true
```

Yayınlanan dizini HTTPS üzerinden sunun veya localhost üzerinde `go tool nsgo build -target web -run` kullanın. Hizmet çalışanı (service worker) sayfayı, WebAssembly'yi, betikleri ve paketlenmiş kaynakları önbelleğe alır. Çevrimiçi ziyaretler yayınlanan sürümü yükler. Kurulum başarılı olduktan ve önceki sürümün sekmeleri kapandıktan sonra en son sürüm önbelleğinden başlatılabilir. Uygulamanın çevrimiçi hizmetlerine yönelik istekler yine bir bağlantı gerektirir.

## Uçbirim (Terminal)

Uygulamayı etkileşimli bir uçbirimde `go tool nsgo dev -target tui` ile çalıştırın. `go tool nsgo test -target tui`, testlerini `nsgotest.Main` aracılığıyla orada çalıştırır (test süreci başına bir uygulama). Boyutu uçbirim belirler: bir hücre 8 x 16 yerleşim puntosunu temsil eder. Uygulamanızın stillerini açıkça seçin; bileşenler olağan biçim ve boyutlarını korur. `examples/tui` alanları, düğmeleri, kısayolları ve yeniden boyutlandırmayı gösterir.

`go tool nsgo build -target tui_macos_arm64` ya da eşleşen `tui_macos_x64`, `tui_windows_arm64`, `tui_windows_x64`, `tui_linux_arm64` veya `tui_linux_x64` ile bir konsol yürütülebilir dosyası derleyin. `tui` derleme/denetleme grubu altısını da içerir ve `all` da bunları kapsar. Her paket dizini yürütülebilir dosyayı ve kaynakları içerir; GUI yükleyicisi yoktur. Altısı da yalnızca Go ile derlenir.

Arka uç hücre metnini, arka planları, düz kenarlıkları, kırpmayı, kaydırmayı, odağı ve metin düzenlemeyi destekler. Uçbirimin yazı tipini ve renklerini kullanır; Mac dahil olmak üzere `mod` tuşu Control'dür. Ctrl+C bir düzenleyici içinde kopyalar ve hiçbir bağlama veya düzenleyici onu almadığında çıkar. Parantezli yapıştırma (bracketed paste) kısayolları çalıştırmadan metin ekler ve pano sürece özeldir. Tuval, görüntüler, medya, yerel barındırılan bileşenler, sistem menüleri/uyarıları, animasyonlar, yakınlaştırma, çift yönlü şekillendirme, baş/orta üç nokta ve sistem panosu köprüsü henüz uygulanmamıştır. `base/terminal` bir GUI uygulamasına gömülü uçbirim denetimi olmayı sürdürür.

`nsgo dev` uçbirime sahipken, araç ve uygulama tanılamaları `.nsgo/tui.log` dosyasına gider. Çıkış, kabuğa dönmeden önce ekranı, imleci ve uçbirim kiplerini geri yükler. Uçbirim test ekran görüntüleri, öykünücüyü yakalamak yerine hücreleri sabit taşınabilir bir yazı tipiyle rasterleştirir; o yazı tipinde eksik olan Unicode glifleri için metin doğrulamaları kullanın.

## nsgo.yaml

```yaml
name: Hello
id: dev.example.hello   # the bundle ID; without it, dev.nsgo. and the name's letters and digits
version: 0.1.0
build_number: 1         # CFBundleVersion and Android's versionCode; else from version
# copyright: © 2026 Example  # the Windows executable's details and installer, and the macOS About panel
# appearance: dark      # light or dark whatever the system's
# theme: theme.json     # a TSX app's theme: NSGo's or a shadcn/ui one
# i18n: { dir: locales, source: en }  # the app's catalogs and the language they are written in
icon:
  apple: assets/AppIcon.icon  # macOS, iOS and watchOS, or an .icns for macOS
  windows: assets/IconWindows.png  # optional Windows artwork
  linux: assets/IconLinux.png      # optional Linux artwork
  png: assets/Icon.png        # every other target
  tvos: assets/AppIconTV.brandassets  # the Apple TV's layered icon and top shelf art, or a wide PNG
  watchos: assets/AppIconWatch.icon   # the Apple Watch's own, drawn in circles, or a square PNG
  android:
    png: assets/IconAndroid.png  # optional bitmap; valid without adaptive layers
    foreground: assets/IconForeground.png  # Android's adaptive icon: 432 px, the logo within the center 264 px
    background: "#1F1F1F"                 # a color or a square PNG
    monochrome: assets/IconMonochrome.png  # optional: the shape Android 13's themed icons tint
dev:
  target: native        # native, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android, harmonyos or miniprogram_<platform>
  hot_reload: true      # false builds once, watching no files
  # icon: { png: assets/Icon-dev.png }  # replaces icon's files in dev's builds; defaults to assets/*-dev.*
build:
  output: dist
  tags: []              # Go build tags every build, test and check of the app takes, such as timetzdata
  ldflags: []           # the app's own linker flags, such as -X main.releaseDate=2026-11-17
  open: false           # true opens the output directory after building
  installers: true      # false leaves out the disk image, the Windows installer and the Linux packages
  jobs: 0               # how many targets build at once; 0 is half the CPUs, 1 one after another
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android_arm64, web]
# apple: { sign_in_with_apple: true }  # the Sign in with Apple entitlement, for iOS, tvOS and Mac App Store builds
# apple: { purchases: true }  # StoreKit's library, which in-app purchases go through, in macOS, iOS and tvOS packages
# apple: { privacy: { accessed_apis: { NSPrivacyAccessedAPICategoryDiskSpace: [E174.1] }, tracking: false } }  # add required-reason APIs and tracking declarations to PrivacyInfo.xcprivacy
# apple: { requires_full_screen: false }  # let an iPad run the app in Split View, Slide Over and Stage Manager, in all four orientations
# apple: { orientations: [portrait, landscape_left] }  # the orientations a full-screen app takes on an iPad
# apple: { multiple_scenes: true }  # let an iPad open more than one window of the app, each showing the main view with its own state
# apple: { info: { UIBackgroundModes: [audio] } }  # background playback for iOS Picture in Picture
# android: { permissions: [REQUEST_INSTALL_PACKAGES] }  # manifest permissions beyond the runtime's
# android: { recording: true }  # the microphone service microphone.RecordWAV records in, with RECORD_AUDIO, the foreground-service permissions and POST_NOTIFICATIONS, which RecordWAV asks for so its notification shows from Android 13; a refusal still records
# android: { uses_cleartext_traffic: true }  # allow plain HTTP to any host in the APK and App Bundle
# android: { background_downloads: true }  # dataSync foreground service for cx.Download
# android: { background_motion: true }  # explicit finite motion shortService; streams also need a notification title
# android: { loopback_cleartext: true }  # allow plain HTTP to loopback in the APK and App Bundle
# commands: [{ name: hello-cli, package: ./cmd/hello-cli }]  # command-line programs the desktop packages ship beside the app: see Commands
# windows: { command_line: true }  # add <name>.com, a console companion that forwards arguments and streams to <name>.exe; not in store builds
# windows: { path: true }  # the installer puts the app's folder on the user's Path, so consoles find its commands
# windows: { publisher: Example Ltd }  # the company the Windows executable's details, its installer and the uninstall entry name; else windows.store.publisher_name
# linux: { maintainer: "Jane Doe <jane@example.com>" }  # the Linux packages' maintainer, else the app's name
# linux: { categories: [Office, Dictionary] }  # the freedesktop menu categories of the app's desktop entry, at least one a main category such as Office, Utility or Game, else menus file it under Other; your own start with X-
# linux: { share: packaging/share }  # files the .deb and Arch package install under /usr/share and the AppImage in its usr/share, such as metainfo/<id>.metainfo.xml or bash-completion/completions/<command>
# linux: { depends: { deb: [gstreamer1.0-plugins-good], arch: [gst-plugins-good] }, recommends: { deb: [gstreamer1.0-libav], arch: ["gst-libav: H.264 and AAC video"] } }  # packages the .deb and Arch package also depend on and recommend, after GTK 4 and WebKitGTK 6, each as dpkg or pacman writes it
# release: { github: me/hello }  # the GitHub repository the install scripts and updates install from, else the Git remote origin's; tag_prefix: v by default
# updates: { key: <public key> }  # how the app updates, by platform and channel: see Updates
# store: { dir: store }           # store listings directory (YAML and Markdown per language)
# miniprogram: { appid: { wechat: wx0123456789abcdef, alipay: "2021000000000000" }, custom_navigation: false }  # the mini programs'
```

`targets` içindeki — veya `nsgo build -target` ve `nsgo check -target` içindeki — yalın bir işletim sistemi adı, o sistemin mimarilerini ifade eder (`windows` `windows_arm64` ve `windows_x64` anlamına gelir; `android` yalnızca `android_arm64` anlamındadır, çünkü x86_64 derlemesi bir NDK gerektirir). `macos` ve `all`'ın dışarıda bıraktığı `macos_universal`, her iki macOS mimarisini de derler ve kütüphaneleri ile araç takımları dahil tek bir uygulamada birleştirir. Steam kanalı evrensel uygulama paketini kullanır. Yükleyici derlemeleri bunu doğrudan kanalda tek bir Sparkle akışıyla tek bir disk görüntüsünde sarmalar. `nsgo test -target` ayrıca testlerinin çalışacağı mimarinin adını da alır: bir masaüstü işletim sistemininki bu makinenin mimarisinde ve Android'inki `arm64` üzerinde; dolayısıyla `windows_x64` bir x64 Windows makinesinde `windows` hedefini çalıştırırken, hedefin üzerinde çalışmadığı bir mimari kullanılacak adla birlikte reddedilir.

Bayraklar `-target`, `-version` veya `-output` gibi adlandırdıkları değerlerin yerini alır; `-h` tümünü listeler.

`build.ldflags`, her biri `go build`'in `-ldflags` bayrağının aldığı şekilde yazılmış uygulamanın kendi bağlayıcı bayraklarını tutar (örneğin `-X main.releaseDate=2026-11-17` veya `-X 'main.channel=beta builds'`). Uygulamayı bağlayan her derleme bunları alır: `dev` ve yeniden yüklemeleri, `-embed` ile `build` de dahil ve `-target` ile `test` de dahil, her hedefte. `build` ve `test` üzerindeki `-ldflags` bunların yerini almak yerine bunlara eklenir; daha fazlası için tekrarlanır. Aracın kendi `-X` bayraklarından önce gelirler ve bağlayıcı bir değişkenin son değerini tutar; böylece `core.App` içindeki ad, kimlik ve sürüm `nsgo.yaml` içindekiler olarak kalır. Araç go'ya kendi `-ldflags` bayrağını geçirir, bu da `GOFLAGS` içindekinin yerini alır, dolayısıyla bağlayıcı bayrakları buraya aittir; dizinden sonraki başsız `test`'in `-ldflags` bayrağı `go test`'inkidir ve tümünün yerini alır.

Aracın uygulama için çalıştırdığı her go komutu `build.tags`'i alır: `dev` ve yeniden yüklemelerinde, `-embed` ile `build`de, `-target` ile `test`te ve `check`te, her hedefte. `dev`'deki `nsgodev` veya hedef testlerdeki `nsgotest` gibi NSGo'nun kendi etiketleri bunlara eklenir; aynı şekilde aksi durumda yerini alacağı `GOFLAGS` içindeki `-tags` etiketleri de eklenir; `GOFLAGS`'in geri kalanı olduğu gibi uygulanır.

`apple.info.LSMinimumSystemVersion` macOS paketinin en düşük sürümünü, `apple.info.MinimumOSVersion` ise seçilen iOS, tvOS veya watchOS paketinin en düşük sürümünü yükseltir. `"14.2"` veya `"18.0"` gibi bir ila üç sayısal parçalı tırnak içine alınmış sürüm dizeleri yazın. Çalışma zamanı tabanları macOS 13.0 (Go 1.27), iOS 15.0, tvOS 17.0 ve watchOS 26.0'dır; daha düşük veya hatalı biçimlendirilmiş değerler başarısız olur. Mobil anahtar seçilen her mobil hedefin tabanını karşılamalıdır. App Clips kendi `app_clip.minimum` değerlerini korur. Değerler paket meta verilerine, varlık derlemesine, imzalama projelerine ve uygulamanın bağlayıcısına ulaşır; gömülü iOS çerçeveleri de bunları kullanır (Go'nun macOS 13 çalışma zamanı için Mac Catalyst en az 16.0).

`icon.windows` ve `icon.linux` uygulama paketleri, yükleyici görselleri ve mağaza logoları için kare PNG'ler seçer. Varsayılan olarak `assets/IconWindows.png` ve `assets/IconLinux.png` dosyalarını kullanır, ardından `icon.png`'ye geri dönerler; `dev.icon` aynı ayarları ve `-dev` varsayılan dosyalarını kabul eder. `icon.android.png`, uyarlanabilir katmanlar yapılandırılmadığında bile bağımsız olarak bir Android bit eşlemi seçer. Diğer hedefler paylaşılan `icon.png` geri dönüşünü korur.

Bir Apple TV simgesi kare değil, katmanlı bir karttır: `tvos`, Xcode'un `.brandassets`'ini, `App Icon` yığınlarını ve üst raf görüntülerini ya da aracın kenar rengi üzerinde karta sığacak şekilde çizip üst raf görüntülerinde ortaladığı 1280×768 gibi geniş bir görüntüyü alır. İkisi de yoksa PNG kart yüksekliğinin dörtte üçüne oturur. Bir Apple Watch simgesini bir daireye maskeler; bu nedenle `watchos` watchOS'un daireleri için çizilmiş bir `.icon` veya kare bir PNG alır; o olmadan uygulama `apple`'ı, ardından `png`'yi kullanır. Her ikisi de tıpkı `apple` ve `png`'nin yaptığı gibi `assets/AppIconTV.*` ve `assets/AppIconWatch.*` dosyalarına geri döner.

Android'de `icon.android.foreground` ve `background` başlatıcı simgesini uyarlanabilir (adaptive) yapar; böylece başlatıcılar `png`'yi kendi plakalarının üzerine küçültmek yerine kendi şekillerine göre maskeler ve istedikleri gibi hareket ettirir. Her katman 108 dp karedir (432 px'e ölçeklenmiş bir PNG); başlatıcı ortadaki 72 dp'yi (288 px) gösterir ve ortadaki 66 dp'nin (264 px) dışındaki her şeyi kesebilecek bir şekle maskeler, bu yüzden logoyu o daire içinde tutun ve arka planın tüm kareyi doldurmasını sağlayın. `background` bir PNG veya `"#1F1F1F"` gibi bir renktir. `monochrome`, yalnızca alfası önemli olan bir şekildir ve kullanıcı temalı simgeleri açtığında Android 13 ve sonraki sürümler bunu renklendirir; onsuz simge orada kendi renklerini korur. Paket, NSGo'nun üzerinde çalıştığı en eski sürüm olan Android 9 veya sonraki hiçbir başlatıcının uyarlanabilir simgeye tercih etmediği düz bit eşlem simgesi olarak `icon.android.png`'yi (yoksa `icon.png`'yi) saklar; `png` olmadan araç bunu arka plan üzerine ön plandan çizer. Uyarlanabilir katmanlar olmadan başlatıcı Android bit eşlemini veya paylaşılan `png`'yi gösterir.

`android.uses_cleartext_traffic: true`, bir APK veya App Bundle'ı herhangi bir ana bilgisayara düz HTTP için etkinleştirir ve hem bildirim bayrağını hem de ağ güvenliği ilkesini ayarlar. Bu katılım olmadan `android.loopback_cleartext: true`, yalnızca uygulamanın kendisi tarafından sunulan video gibi `localhost`, `127.0.0.1` ve `::1` üzerinden düz HTTP'ye izin verir. Yalnızca geri döngü (loopback) ilkesi diğer ana bilgisayarlara şifresiz trafiği reddeder; her iki ayar da true olduğunda tüm ana bilgisayar katılımı öncelik kazanır. Gömülü bir AAR için konak uygulamanın Android ağ güvenliği ilkesini yapılandırın.

Her derleme ayrıca başlarken `--nsgo-fps` gibi NSGo'nun kendi bayraklarını alır ([Çalıştırma ve hata ayıklama](debugging.md#her-derlemenin-bayrakları)). `nsgo dev` içinde Command/Control+R açık olan her pencereden yeniden yükleme yapar ve başka bir pencere bir uygulama işlevini çalışır durumda tuttuğunda ana görünüm kapandıktan sonra da kullanılabilir kalır.

### Başlatma ekranı

iOS ve Android 12 veya sonraki sürümlerde opak bir arka plan, ortalanmış bir PNG veya her ikisini birden göstermek için `launch_screen` ayarlayın. Görüntü yolları `nsgo.yaml`'a göredir. Görüntü 128 × 128 punto veya dp içine orantılı olarak sığar. En uzun kenarı en az 512 piksel olan saydam bir PNG, yüksek yoğunluklu ekranlarda görüntüyü keskin tutar.

```yaml
launch_screen:
  background: "#F5F1EA"
  image: assets/Launch.png
  dark:
    background: "#201C26"
    image: assets/LaunchDark.png
```

`appearance` sabitlemediği sürece görünümü sistem seçer. Karşılık gelen değeri yeniden kullanmak için `dark` alanlarından birini atlayın. Atlanan bir arka plan veya görüntü platformun varsayılanını korur. Gömülü uygulamalar başlatma ekranlarını konaktan alır.

## Android yerel sınıfları

`core.Native`'den özel bir View kullanmak için önceden derlenmiş Android sınıflarını uygulamanın `nsgo.yaml` dosyasında listeleyin.

```yaml
android:
  classes: [java/BadgeView.jar, java/Helpers.dex]
```

Mutlak olmadıkça yollar `nsgo.yaml`'a göredir. Her yol bir `.dex` dosyasını ya da D8'in kök `classes.dex`, `classes2.dex` ve sonraki parçalarını içeren bir JAR'ı adlandırır. Bu dosyaları D8 ile `--min-api 28` kullanarak hazırlayın. arm64 üzerinde NSGo, APK veya App Bundle'ı bu önceden derlenmiş dosyalardan yalnızca Go ile oluşturur. Android geliştirme oturumları listelenen bir dosya değiştiğinde yeniden derler. Gömülü bir AAR için konağın Android derlemesi kendi Java bağımlılıklarını sağlar. [D8 belgeleri](https://developer.android.com/tools/d8)

[Android yerel örneği](https://github.com/nsgo-dev/nsgo/tree/main/examples/android-native), genel bir `Context` oluşturucusuna sahip bir sınıfı kaydeder ve onu bir Go prop'u ile yapılandırır.

## Oyunlar

Bir oyun için `nsgo.yaml` içinde `game: true` ayarlayın. Android uygulamayı oyun olarak sınıflandırır ve sistemin Oyun Kipi iyileştirmelerini kullanır. macOS ve iOS Oyun Kipini etkinleştirir, macOS varsayılan olarak Games kategorisini kullanır ve Linux masaüstü girdisine `Game` ekler. Açık bir `macos.category` veya `apple.info` içindeki Oyun Kipi anahtarı öncelik kazanır.

Uygulamanız bir donanım oyun kumandasını işlediğinde `gamepad: true` ayarlayın. Android hem oyun kumandasını hem de dokunmatik ekranı isteğe bağlı olarak bildirir. macOS, iOS ve tvOS Extended Gamepad profilini, tvOS ayrıca Siri Remote'un Micro Gamepad profilini bildirir; `apple.info` içindeki denetleyici anahtarları öncelik kazanır.

## Lisans bildirimleri

`nsgo build`, bağımsız paketin kaynakları arasına `THIRD_PARTY_NOTICES.txt` dosyasını koyar. Bu dosya, söz konusu hedefin kullandığı Go modüllerinin, Go çalışma zamanının ve NSGo'nun paketlediği Java veya JavaScript çalışma zamanının lisans metinlerini içerir. Web derlemeleri, diğer kaynaklarda olduğu gibi dosyayı içeriğine göre adlandırır. Aynı metni uygulama içi bir teşekkür ekranında göstermek için `core.ThirdPartyNotices()` kullanın. İşlev geliştirme aşamasında, testlerde ve gömülü kütüphanelerin derlemelerinde boş bir dize döndürür. [Yerel kütüphane bildirimleri](native-libraries.md) kendi özgün lisans metinlerini aynı bildirimlere ekler.

Varsayılan olarak bilinmeyen veya eksik bir üçüncü taraf lisansı derlemeyi başarısız kılar ve algılanan bir copyleft lisansı uyarı verir. Uygulamanın Go çalışma alanında (`go.work`) listelenen modüller, uygulamanın kendi modülüyle birlikte birinci taraf kodu olarak ele alınır: içerdikleri tüm lisans dosyaları dahil edilir ve üçüncü taraf denetimlerini tetiklemezler. Her iki kuralı da `error` veya `warn` olarak ayarlayın. `allow` listesi tam modül yollarını her iki kuraldan da muaf tutar ve derlemenin bulduğu lisans metinlerini saklar.

```yaml
licenses:
  unknown: error
  copyleft: warn
  allow: [example.org/reviewed-module]
```

## Kütüphane ekleme

Bir kütüphaneyi, yani bir Go modülünü, planını önce gösteren bir `go get` olan `nsgo add` ile ekleyin:

```sh
nsgo add github.com/acme/chart           # en yeni sürümü; go.mod zaten içeriyorsa dokunmaz
nsgo add github.com/acme/chart@v1.3.0    # bu sürüm, go.mod'dakinden düşük olsa bile
nsgo add -dry-run github.com/acme/chart  # planı yazdırır, hiçbir şey yazmaz
```

```text
github.com/acme/chart
  module     github.com/acme/chart v1.3.0, published 2026-11-20
  owner      github.com/acme, the first module from it in go.mod
  next       import "github.com/acme/chart"

example.com/hello
  go.mod     + require github.com/acme/chart v1.3.0 // indirect
             + require example.com/units v1.0.0 // indirect
             ~ require github.com/nsgo-dev/nsgo v0.4.0 => v0.4.2, a release
  go.sum     + 6 lines
  tools      go tool nsgo then runs v0.4.2, and so does an installed nsgo here
Apply these changes? [y/N]
```

Plan her modül için sürümünü ve yayın tarihini, ayrıca sahibini belirtir: barındırıcıyı (host) ve github.com gibi kod barındırma hizmetlerinde hesabı da; `go.mod` bu sahipten henüz bir modül içermiyorsa bu işaretlenir. Geçerli olduklarında geri çekme (retract) ve kullanımdan kaldırma bildirimleri, sürüm etiketi olmayan bir sürüm, modülün bir `replace` satırı ve kök paketinin import satırı ardından gelir. Sonra `go.mod` içinde değişen satırlar, `go` ve `toolchain` dahil, başka bir sürümü çalıştıracak `go tool` komutları, kendi barındırıcılarından alınan modüller ve `go.sum`'ın sağlama toplamı veritabanının denetimi olmadan kabul ettiği modüller gelir. `go get`, henüz hiçbir paketin içe aktarmadığı bir modülü `// indirect` olarak işaretler; `go mod tidy` bir paket onu içe aktarınca bu işareti kaldırır.

Bir terminalde `add` yazmadan önce sorar, `-y` ise planı sormadan uygular; terminal ve `-y` olmadan hiçbir şey yazmaz ve 1 durum koduyla çıkar. `go.mod`'un zaten gerektirdiği bir modül, bir sürüm yazılmadıkça olduğu gibi bırakılır ve sürümü yazdırılır.

Planlama, `go.mod` ve `go.sum` kopyaları üzerinde go komutundan başka hiçbir şey çalıştırmaz:

- modülün zaten kullandığı araç zinciriyle (toolchain) ve `GOTOOLCHAIN=local` ile, böylece başka hiçbir araç zinciri indirilmez ya da çalıştırılmaz;
- `GOPROXY`'nin ilk `direct` değerinden önce listelediği modül proxy'leri üzerinden; `direct` `off` olur, böylece yalnızca `GOPRIVATE` ya da `GONOPROXY` ile işaretlenen modüller kendi barındırıcılarından alınır, `GONOSUMDB` ve `GOAUTH` olduğu gibi kalır;
- `GOWORK=off` ile, böylece bir çalışma alanında (workspace) `go.mod`'u yalnızca modülün kendisi için, `GOWORK=off go get` nasıl yazıyorsa öyle planlar ve bunu belirtir.

`-y` verilsin ya da verilmesin, `add` şunları reddeder:

- NSGo gereksinimini bir sözde sürüme (pseudo-version) ya da ön sürüme taşımak; NSGo'nun bir commit'ini gerektiren bir kütüphane bunu yapardı; `go get github.com/nsgo-dev/nsgo@<sürüm>` onu önce bilerek oraya taşır;
- modülün araç zincirinden daha yeni bir Go gerektiren bir modül; `go get go@<sürüm>` önce modülünkini yükseltir;
- bir modülü `@latest` gibi bir sorguyla düşürmek; sürümü yazmak onu düşürür ve plan buna sürüm düşürme (downgrade) der.

`go.mod` ve `go.sum` dosyalarını yalnızca planlamadan bu yana ikisi de değişmediyse, her birini üzerine yeniden adlandırılan geçici bir dosya aracılığıyla yazar; bir hata ya da kesinti olursa ikisini de eski hâline getirir. `vendor` dizini olan bir modülde plan, sonraki adım olarak `go mod vendor` komutunu gösterir.

`add`, NSGo 0.3.4'ten sonra geldi. 0.3.4'ü ya da daha eski bir sürümü sabitleyen bir modülde kurulu `nsgo`, bu komut için modülün `nsgo`'sunu çalıştırmaz; `add`'i getiren `go get github.com/nsgo-dev/nsgo@<sürüm>` komutunu yazdırır. Orada `go tool nsgo add`, `add`'i bilmeyen sabitlenmiş `nsgo`'yu çalıştırır.

## Go paketlerinden kütüphaneler

`base/terminal` paketini içe aktaran uygulamalar libghostty-vt kütüphanesini de birlikte dağıtır: Windows ve Linux paketleri kütüphaneyi yürütülebilir dosyanın yanına yerleştirir; macOS ve iOS paketleri bunu çerçevelerine (framework) imzalar. [Gömme](embedding.md) iOS konakları için eşlik eden XCFramework'ü açıklar.

Bir bağımlılık, `nsgo-natives.yaml` içinde önceden derlenmiş masaüstü paylaşılan kütüphanelerini bildirebilir. NSGo bunların SHA-256 özetlerini doğrular, imzalamadan önce uygulamaya kopyalar ve yükleyicilere dahil eder; uygulama yazarının fazladan bir adım çalıştırması gerekmez. [Yerel kütüphaneler](native-libraries.md) paket yazarının biçimini ve yükleme yollarını açıklar.

## Komutlar

Bir uygulama masaüstü yürütülebilir dosyasının yanında, örneğin uygulamanın verileri üzerinde çalışan bir CLI gibi komut satırı programları dağıtabilir. Bunları her biri bir Go main paketinden derlenecek şekilde `commands` altında listeleyin:

```yaml
commands:
  - name: hello-cli             # the file's name, hello-cli.exe on Windows
    package: ./cmd/hello-cli    # a main package, built for each desktop target with CGO_ENABLED=0 and build.tags
    ldflags: [-X main.channel=beta]  # its own linker flags, after build.ldflags
windows:
  path: true                    # the installer puts the app's folder on the user's Path
```

Bir ad harflerden, rakamlardan, `-` ve `_` karakterlerinden oluşur ve herhangi bir büyük/küçük harf biçiminde uygulamanın yürütülebilir dosyası, Linux komutu, `Uninstall`, `AppRun` veya başka bir komutun adı olamaz. `macos_universal`, tıpkı uygulamanınkini birleştirdiği gibi her komutun iki macOS derlemesini birleştirir.

Bir paket `build.ldflags` ve `-ldflags`, ardından `-X` değerleri kazanan kendi `ldflags` bayrakları ve ardından uygulamanın kimliği ile bağlanır; böylece `core.App()`, tıpkı uygulamaya verdiği gibi komuta da `nsgo.yaml` içindeki adı, kimliği, sürümü ve derleme numarasını verir. `core`'u içe aktarmayan bir komut, sürümü hem uygulamanın hem de komutlarının aldığı `build` üzerindeki `-ldflags "-X main.version=1.2.0"` gibi bir `-X` ile alır.

| Paket | Komutlar |
| --- | --- |
| macOS | `Contents/MacOS` içinde, uygulamadan önce olduğu gibi `<id>.<command>` adıyla imzalanır: geçici (ad hoc) olarak ya da `sign.identity`, güçlendirilmiş çalışma zamanı (hardened runtime) ve zaman damgasıyla (böylece uygulama [noter onayı](#noter-onayı) alır), ancak uygulamanın yetkileri olmadan. Disk görüntüsü `PATH` üzerine hiçbir şey koymaz; [Homebrew cask](#kanallar) her komutu bağlar ve bir kullanıcı `/Applications/<name>.app/Contents/MacOS/<command>` yolunu kendisi bağlayabilir |
| Windows | `<name>.exe` yanında, olduğu gibi [imzalanmış](#windows-paketlerini-imzalama), yükleyicinin kurduğu ve kaldırıcısının sildiği `<command>.exe`; `windows.path` ile yükleyici, tıpkı `windows.command_line`'ın yaptığı gibi klasörü kullanıcının `Path` değişkenine de ekler, böylece kurulumdan sonra açılan konsollar komutları bulur. Bir güncellemenin yeniden adlandırdığı veya kaldırdığı bir komut, uygulamanın artık göndermediği bir dosya gibi klasörde kalır; çünkü kaldırıcı yalnızca kendi sürümünün kurduğunu siler |
| Linux | `.deb` ve Arch paketinin sahip olduğu `/usr/bin/<command>` bağlantısıyla `/opt/<slug>` içinde yer alır; böylece paketi kaldırmak bağlantıyı da kaldırır ve dpkg ile pacman başka bir paketin üzerine kurulum yapmayı reddeder; AppImage bunları uygulamanın yanında tutar ve `PATH` üzerine hiçbir şey koymaz |

Yükleyiciler, sağlama toplamları ve [güncelleme](#güncellemeler) appcast'leri komutları uygulamanın diğer dosyalarını taşıdığı gibi taşır; bu nedenle bir güncelleme bunları uygulamayla birlikte değiştirir. Mac App Store korumalı alanı, MSIX ve Flatpak yalnızca uygulamanın kendi yürütülebilir dosyasını çalıştırdığı için [mağaza paketleri](#mağaza-paketleri) bunları dışarıda bırakır.

## Yükleyiciler

Masaüstü hedefleri için `build`, kullanıcıların uygulamayı kuracağı dosyaları her biri bir `.sha256` sağlama toplamıyla birlikte `dist/releases` içine yazar:

| Hedef | Yükleyici |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.dmg`; uygulamayı, bir oku ve sürükleyip bırakılacak Applications bağlantısını gösteren, `sign.identity` ile imzalanmış ve [noter onaylı](#noter-onayı) bir disk görüntüsü |
| Windows | `<name>-<version>-windows-<arch>-setup.exe`; kullanıcı için (yine kullanıcının daha önce seçtiği klasöre, çalışıyorsa uygulamayı kapatarak) Başlat menüsü kısayolu, isteğe bağlı masaüstü kısayolu ve yalnızca kurduğunu kaldıran bir kaldırıcı ile kurulum yapan, uygulamanın Windows paketlerini imzaladığı yerde [`sign.certificate`](#windows-paketlerini-imzalama) ile imzalanmış yerel bir Windows yükleyicisi; `windows.command_line` ayrıca `.com` eşlikçisini kurar ve uygulamanın klasörünü kullanıcının `Path` değişkenine ekler, böylece kurulumdan sonra açılan konsollar uygulamanın komutunu çalıştırırken eşlikçiyi de bulan Win+R uygulamayı başlatır ve açtığı konsolu kapatır; kaldırıcı klasörü `Path`'ten yalnızca bir yükleyici eklediyse çıkarır, sizin zaten sahip olduğunuz bir girdiyi asla çıkarmaz |
| Linux | Debian ve Ubuntu için `<name>-<version>-linux-<arch>.deb` ve Arch Linux için `<name>-<version>-linux-<arch>.pkg.tar.gz` — uygulamayı `/opt/<slug>` dizinine bir komut, [komutlarına](#komutlar) bağlantılar, masaüstü girdisi ve simgesiyle kurar (Arch paketi `pacman -Qkk` için `.MTREE` içerir) — ve kurulum olmadan herhangi bir dağıtımda çalışan `<name>-<version>-linux-<arch>.AppImage` |

Windows kurulum programı ve kaldırıcı, herhangi bir sistemde yalnızca Go ile derlenir ve yerel Windows iletişim kutularını kullanır. Uygulamanın kataloglarındaki dillerden karşılığını bildikleri her birini (Basitleştirilmiş ve Geleneksel Çince dahil) sistemin arayüz dilini seçerek konuşurlar; `/LANG=<LCID>` tek bir çalıştırma için bir dil seçer, örneğin Basitleştirilmiş Çince için `/LANG=2052`. `/S` sessizce kurar veya kaldırır, sondaki bir `/D=<folder>` klasörü (boşluklar dahil) seçer ve `/RELAUNCH` bir güncellemeden sonra uygulamayı açar. `/D` olmadan bir yükseltme, yerini aldığı kurulumun klasörüne kurar. `-installers=false` veya `build.installers: false` yalnızca paketleri derler.

Yükleyiciler uygulamanın kendi resimlerini ve simgelerini alır; aksi durumda her biri uygulamanın simgesini veya yerleşik resmi kullanır:

```yaml
macos:
  dmg_background: assets/DMGBackground.png  # the disk image's window, which takes its size in points
  dmg_icon: assets/DMGIcon.png              # the disk image's icon: an .icns or a square PNG
windows:
  installer_icon: assets/Setup.png          # the setup program's icon: an .ico or a square PNG
  uninstaller_icon: assets/Uninstall.png    # the uninstaller's
```

`DMGBackground@2x.png` gibi iki katı boyutunda bir ikizi olan arka plan PNG'si Retina ekranlarda keskin kalır. Uygulamanın simgesi soldan dörtte bir içeride, Applications sağdan dörtte bir içeride ve her birinin adı yukarıdan aşağıya ortalanmış olarak (yerleşik resmin okunu çizdiği yerde) yer alır; kendi arka planınızda bunlar için yer bırakın. Linux paketlerinin kendi resimleri yoktur; uygulamanın simgesini kurarlar.

Linux paketleri ayrıca yazılım merkezlerinin uygulamayı listelediği AppStream meta verisini (`/usr/share/metainfo/<id>.metainfo.xml`) `linux.summary`, ayarlıysa `linux.license` ve uygulamanın adlarından (`linux.categories` hiçbiri adlandırmazsa kategori olarak `Utility` ile) yazar ve kurar; AppImage bunu AppDir'inin `usr/share` dizininde, meta verinin adlandırdığı `usr/share/applications` içindeki masaüstü girdisine bir bağlantının yanında tutar. Sistemin `/usr/share` dizininden okuduğu daha zengin meta veriler, ek masaüstü girdileri, kabuk tamamlamaları veya kılavuz sayfaları gibi dosyalar `nsgo.yaml`'a göre `linux.share` tarafından adlandırılan dizine gider: `.deb` ve Arch paketi bunun ağacını `/usr/share` altına, AppImage ise AppDir'inin `usr/share` dizini altına kurar. Bunun `metainfo/<id>.metainfo.xml` dosyası NSGo'nun yazdığının yerini alır ve uygulamanın masaüstü girdisi veya simgesi gibi paketin kendisinin yazdığı bir yoldaki dosya derlemeyi başarısız kılar. Nasıl adlandırılırsa adlandırılsın `resources/` içindeki dosyalar `/opt/<slug>` içinde yürütülebilir dosyanın yanında kalır ve mağaza derlemelerinin Flatpak'leri yalnızca NSGo'nun meta verisini alır.

AppImage, çoğu dağıtımda bulunan FUSE'a ihtiyaç duyar ve tıpkı paketler gibi GTK 4'ü sistemden alır; çalışma zamanı kullanıcı önbelleğine bir kez indirilir.

AppImage'lar, `release.github` veya GitHub `origin` uzak deposunu kullanarak en son kararlı sürümü ve bu uygulamanın mimarisini seçen bir `gh-releases-zsync` konum belirleyicisi gömer. Her `.AppImage` dosyasını üretilen `.AppImage.zsync` dosyasıyla birlikte yükleyin. Başka bir sunucu için `linux.appimage_update` ayarını HTTP aralık desteğiyle sunulan kararlı bir URL olan `zsync|https://example.com/latest.AppImage.zsync` olarak yapın; ayrıca bir ön sürüm kanalı için `gh-releases-zsync|owner|repo|latest-pre|name-*-linux-x64.AppImage.zsync` biçimini de kabul eder. Açık bir boş dize gömülü keşfi devre dışı bırakır; bir GitHub deposu veya yapılandırılmış bir konum belirleyici olmadan da boş kalır. `.zsync` dosyası yine de imzalı appcast'ten ayrı olarak üretilir.

## Mağaza paketleri

`-channel store` veya `updates.channel: store`, masaüstü ve Android hedeflerini yükleyiciler ve kurulum betikleri yerine onları güncelleyen kendi mağazaları için derler:

| Hedef | Mağaza paketi |
| --- | --- |
| macOS | Mac App Store için `<name>-<version>-macos-<arch>.pkg`: App Sandbox içindeki uygulama, sağlama profilinin kimliğiyle imzalanır ve ekibinizin yükleyici kimliğiyle paketlenir |
| Windows | Yayınlama sırasında imzalayan Microsoft Store için `<name>-<version>-windows-<arch>.msix` |
| Linux | Kurulabilir `<name>-<version>-linux-<arch>.flatpak`, ayrıca GNOME'un çalışma zamanı üzerinde Flathub veya flatpak-builder için AppStream taşıyan `.tar.gz` ve `flatpak/<id>.yml` |
| Android | Google Play için yükleme anahtarı olan `sign.keystore` ile imzalanmış bir App Bundle olan `<name>-<version>-android-<arch>.aab` |

MSIX, her dilin görünen adını katalogların `appName` alanından okur ve kaynak dile geri döner. Başlat, görev çubuğu ve Mağaza logoları %100, %125, %150, %200 ve %400 varyantlarını içerir. Araç kaynak dizinini macOS'ta yazar; Windows SDK gerekmez.

`-target macos` mağaza kanalında her iki macOS mimarisini seçer ve tıpkı `-target macos_universal` gibi tek bir `<name>-<version>-macos-universal.pkg` yazar. `arm64-v8a` ve `x86_64` içeren tek bir `<name>-<version>-android-universal.aab` için `-target android_arm64,android_x64` seçin. `x86_64` dilimi bir Android NDK gerektirir.

```yaml
macos:
  category: public.app-category.productivity  # LSApplicationCategoryType, which the Mac App Store requires
  # entitlements: { com.apple.security.files.downloads.read-write: true }  # beyond the network, the files the user picks and what apple.info explains
windows:
  store:                               # as Partner Center's Product identity lists them
    identity: 12345Example.Hello
    publisher: CN=00000000-0000-0000-0000-000000000000
    publisher_name: Example
  # device_capabilities: [microphone, webcam]  # the devices the app uses, which Windows asks the user for
linux:
  summary: Says hello                  # one line for software centers
  license: MIT                         # an SPDX expression; LicenseRef-proprietary by default
  # flatpak_runtime: "51"              # GNOME's runtime version
android:
  target_sdk: 36                       # the targetSdkVersion Google Play requires
sign:
  macos_profile: assets/MacAppStore.provisionprofile  # the Mac App Store's profile
  # installer: "3rd Party Mac Developer Installer: Example (ABCDE12345)"  # by default the keychain's of the profile's team
```

`android.target_sdk` ayarlanmadığında çalışma zamanı API 36'yı hedefler. Bir Android App Bundle derlemesi, [Google Play'in 31 Ağustos 2026'dan itibaren yeni mobil uygulamalar ve güncellemeler için zorunlu kıldığı](https://support.google.com/googleplay/android-developer/answer/11926878) API 36'nın altında başarısız olur. Bağımsız bir APK derlemesi uyarı verir ve devam eder.

Mac paketini yükleme için imzalamak üzere `sign.macos_profile` ayarlayın. Araç takımları olmadan imzasız paket yine de korumalı alanda (sandbox) çalışır; bu da uygulamanın kapsayıcısına, ağa ve kullanıcının seçtiği dosyalara erişimi test etmenizi sağlar. Araç takımları içeren bir mağaza derlemesi hem `<id>` için `sign.macos_profile` hem de aynı ekipten `<id>.widgets` için `apple.app_group` (varsayılan `group.<id>`) izni veren bir macOS `sign.widgets_profile` gerektirir. Her iki paket de o paylaşılan kapsayıcıyı kullanır; hiçbir ana dizin istisnası eklenmez. `-installers=false`, `.pkg` dosyasını ve yükleyici kimliğini dışarıda bırakırken korumalı alanı ve profil imzalarını korur. `.msix` dosyasını Partner Center'a ve `.aab` dosyasını Play Console'a yükleyin. Sürüm adlandırdığı arşivleri içerdiğinde `flatpak/<id>.yml` dosyasını Flathub'a gönderin. `flatpak-builder` bunu yerel olarak da derler. Bir paketi `flatpak install --user ./<name>-<version>-linux-<arch>.flatpak` ile kurun; GNOME'un eşleşen çalışma zamanı kurulu veya yapılandırılmış bir uzak depodan erişilebilir olmalıdır. Derlemeler paketi bir Linux araç zinciri olmadan Go içinde yazar. Mağaza paketleri görsel olmadığında saydam bir simge ve `linux.categories` ayarlanmadığında Utility kullanır. Mağaza dışında kurulum için MSIX'i imzalamak üzere [`sign.certificate`](#windows-paketlerini-imzalama) ayarlayın; sertifika konusu `windows.store.publisher` ile eşleşmeli ve alıcı makine sertifikaya güvenmelidir. İmzalama ayarları olmadan yüklenen paketi Mağaza imzalar.

macOS ve iOS'ta bir kaydırma görünümünün kenar efektleri (`Scroll.EdgeEffects`), `IOS.BackdropMaterial` içermeyen macOS arka plan bulanıklaştırmaları ve arka plan renk filtreleri (tek başına veya `IOS.BackdropMaterial` içermeyen bir bulanıklaştırmanın üzerinde), hiçbir genel API yarıçapa göre bulanıklaştırmadığı, bir görünüm boyunca bulanıklaştırmayı değiştirmediği veya bir arka plan renk matrisi uygulamadığı için tıpkı AppKit ve UIKit'in kendi materyalleri ve kenar efektleri gibi Core Animation'ın özel `CABackdropLayer` ve `CAFilter` sınıfları üzerinden çizilir. App Store incelemesi özel API kullanan uygulamaları reddedebilir: NSGo bunları yalnızca kenar çubuklu kaydırma görünümleri ve söz konusu arka plan filtreleri için çağırır, ancak her macOS ve iOS derlemesi bunların adlarını içerir.

`build` tarafından yazılan her Apple paketi — uygulama, araç takımı uzantısı, App Clip ve Apple Watch uygulaması ile bir `-dynamic` `-embed` çerçevesi — NSGo'nun ve Go çalışma zamanının kullandığı gerekli nedenli API'leri bildiren bir gizlilik bildirimi (`PrivacyInfo.xcprivacy`) taşır: kullanıcı varsayılanları (`CA92.1`), dosya zaman damgaları (`C617.1`) ve sistem açılış zamanı (`35F9.1`). `apple.privacy` uygulamanın kendininkileri ekler: `accessed_apis`, her kategorinin nedenleri, `tracking`, `tracking_domains` ve `collected_data` (Apple'ın adlandırdığı biçimiyle `NSPrivacyCollectedDataTypes` girdileri); `resources/` içindeki bir `PrivacyInfo.xcprivacy` değiştirilmek yerine bunlarla birleştirilir.

`build` tarafından cihazlar için yazılan her iOS, tvOS ve watchOS paketinin (uygulamanın, araç takımı uzantısının ve App Clip'in) `Info.plist` dosyası, App Store Connect'in denetlediği ve derleyen Xcode'dan okunan SDK ve Xcode `DT` anahtarlarını ve `UIRequiredDeviceCapabilities: [arm64]` değerini taşır; simülatör paketleri bunları dışarıda bırakır ve `apple.info`'nun ayarladığı bir anahtar değerini korur. `apple.multiple_scenes: true` bir iOS uygulamasının sahne bildiriminde varsayılan olarak false olan `UIApplicationSupportsMultipleScenes` değerini ayarlar: iPad üzerinde kullanıcı böylece uygulamanın birden fazla penceresini açabilir ve her biri kullanıcı kapatana kadar kendi durumu ve uygulamanın paylaşılan sinyalleri ile depolamasıyla uygulamanın ana görünümünü yeniden gösterir.

### Google Play'de yayınlama

Google Play Developer API'yi etkinleştirin ve bir hizmet hesabına uygulamayı seçilen kanala (track) yayınlama izni verin. `NSGO_PLAY_SERVICE_ACCOUNT` değişkenini hesabın JSON anahtarına veya anahtar dosyasının yoluna ayarlayın. [Google Play API kurulumu](https://developers.google.com/android-publisher/getting_started)

```sh
go tool nsgo build -target android -channel store
go tool nsgo publish -target android -track internal
```

Komut, projenin sürümü için `build.output/releases` içinde App Bundle'ı bulur. Birden fazla derleme olduğunda bir dosya seçmek için bayraklardan sonra bir `.aab` yolu geçirin. `internal`, `production` veya kapalı bir testin kanal kimliğini kullanın. Komut paketi ve sürüm kodunu paketten okur, o kanaldaki her sürümden daha yüksek bir kod gerektirir, ardından paketi yükler ve sürümü işler (commit).

## Mağaza listelemeleri

Mağaza meta verileri (başlıklar, alt başlıklar, açıklamalar, anahtar sözcükler, sürüm notları, URL'ler, ekran görüntüleri ve önizlemeler) depoda `store/` (veya `nsgo.yaml` içindeki `store.dir`) altında düz metin YAML ve Markdown olarak yer alır:

```text
store/
  listing.yaml              # version: 1, default_locale, app_store and google_play settings
  en-US/
    listing.yaml            # version: 1, title, subtitle, short_description, keywords, URLs, media
    description.md          # full description (optional file in place of YAML description)
    release_notes.md        # what's new (optional file in place of YAML release_notes)
    promotional_text.md     # App Store promotional text (optional file)
    screenshots/
  zh-Hans/
    listing.yaml
    description.md
    release_notes.md
    screenshots/
```

Her iki `listing.yaml` dosyası da `version: 1` gerektirir ve bilinmeyen anahtarları reddeder. `zh-Hans` gibi kurallı yerel ayar dizinleri, mağaza başına `app_store.locale` veya `google_play.locale` ile geçersiz kılınmadıkça otomatik olarak her mağazanın koduna (App Store Connect'te `zh-Hans`, Google Play'de `zh-CN`) eşlenir.

```sh
go tool nsgo publish -store all -dry-run                  # validate lengths, locales and media without uploading
go tool nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
go tool nsgo publish -store play -track production        # also update release notes on the named Play track
```

Doğrulama kimlik bilgilerini okumadan veya herhangi bir HTTP isteği yapmadan önce çalışır; gerekli alanları, yerel ayar kodlarını, URL şemalarını, medya dosyalarını ve sayıları (App Store ekran türü başına en fazla 10 ekran görüntüsü ve 3 önizleme; Google Play görüntü türü başına en fazla 8 ekran görüntüsü) ve karakter sınırlarını (App Store: başlık 30, alt başlık 30, anahtar sözcükler 100, tanıtım metni 170, açıklama 4000, sürüm notları 4000; Google Play: başlık 30, kısa açıklama 80, açıklama 4000, sürüm notları 500) denetler.

Yükleme kimlik bilgileri yalnızca ortamdan gelir: App Store Connect için `NSGO_APP_STORE_KEY`, `NSGO_APP_STORE_KEY_ID` ve `NSGO_APP_STORE_ISSUER` (noter onayı anahtarı `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` ve `NSGO_NOTARY_ISSUER`'a geri döner) ve Google Play için `NSGO_PLAY_SERVICE_ACCOUNT` (hizmet hesabı JSON dosya yolu veya JSON metni).

### Mağaza uyumluluğu

Her sürüm anketinin durumunu izlemek için `nsgo.yaml` kullanın.

```yaml
compliance:
  app_store: { age_rating: false }
  google_play: { data_safety: false }
  steam: { content_survey: false }
```

Anketini tamamladıktan sonra bir girdiyi `true` olarak ayarlayın. Eşleşen bir `-channel store` veya `-channel steam` derlemesi size `false` olarak ayarlanmış girdileri hatırlatır. YAML'da izlemediğiniz herhangi bir girdiyi atlayın.

- App Store Connect'te [yaş derecelendirme anketini](https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating) ve [Uygulama Gizliliği ayrıntılarını](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy) tamamlayın.
- Apple'ın [şifreleme anketi](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation) için `apple.info.ITSAppUsesNonExemptEncryption` değerini uygulamanızın yanıtı olarak bir YAML mantıksal değeri şeklinde ayarlayın. Apple bir onay kodu gönderirse bunu dize olarak `apple.info.ITSEncryptionExportComplianceCode` içine koyun. Anahtarları dışarıda bırakıp yükleme sırasında yanıtlayabilirsiniz. Apple mağaza derlemeleri bu anahtarların türlerini ve paketin gizlilik bildiriminin yapısını denetler.
- Play Console'da [Veri güvenliği](https://support.google.com/googleplay/android-developer/answer/10787469) formunu tamamlayın. Yalnızca dahili test kanalında dağıtılan uygulamalar muaftır.
- Oyunu incelemeye göndermeden önce Steamworks'te [İçerik Anketini](https://partner.steamgames.com/doc/gettingstarted/contentsurvey) tamamlayın.
## App Store Connect

App Store Connect'te bir uygulama oluşturun ve bir iOS cihaz IPA'sını bir App Store dağıtım profiliyle veya bir macOS mağaza PKG'sini ekibinizin yükleyici kimliğiyle imzalayın. Xcode'lu bir Mac üzerinde `NSGO_APP_STORE_KEY` değişkenini App Store Connect ekip anahtarınızın `.p8` dosyasına veya metnine, `NSGO_APP_STORE_KEY_ID` değişkenini anahtar kimliğine ve `NSGO_APP_STORE_ISSUER` değişkenini veren (issuer) kimliğine ayarlayın; bunlar olmadan mağaza listelemelerinde olduğu gibi noter onayı anahtarı (`NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, `NSGO_NOTARY_ISSUER`) kullanılır. Bu anahtara uygulamaya erişim ve derleme yükleme izni verin.

```sh
go tool nsgo publish ios path/to/Hello.ipa
go tool nsgo publish macos path/to/Hello.pkg
```

Araç yüklemeden önce paketin derleme numarasını denetler, ardından TestFlight için Apple'ın işleme sonucunu bildirir. Bir sonraki yükleme için `build_number` değerini artırın. macOS derleme numaraları tüm uygulama sürümleri boyunca artarken iOS yeni bir sürüm için diziyi yeniden başlatabilir. İşlenmiş bir derlemeyi App Store Connect'te App Review için ayrı olarak gönderin. [Apple'ın dağıtım kılavuzu](https://developer.apple.com/documentation/xcode/preparing-your-app-for-distribution)

## Mağaza varlık paketleri

Büyük varlık dizinlerini `mobile.asset_packs` içinde bildirin. Bu dizinlerin içindeki dosyalar temel Android App Bundle ve iOS uygulamasından ayrı olarak teslim edilir. Paket adları küçük harfle başlar ve küçük harfler, rakamlar ve alt çizgiler içerebilir.

```yaml
mobile:
  asset_packs:
    - {name: levels, dir: resources/levels, delivery: install-time}
    - {name: voices, dir: large-assets/voices, delivery: fast-follow}
```

Android paketlerini `go tool nsgo build -channel store -target android_arm64` ile derleyin. Paketi bir cihazda test etmek için `bundletool build-apks --local-testing` ve `bundletool install-apks` kullanın. NSGo kurulu `install-time` paketlerini açar. `fast-follow` modülleri Play teslimatı için dahil edilir ve bu paketlerin Play Asset Delivery SDK'dan okunması henüz eklenmemiştir.

iOS'ta `go tool nsgo build -target ios`, hem ilk kurulumda hem de güncellemelerde `install-time`'ı Apple'ın temel (essential) ilkesine ve `fast-follow`'u ön getirmeye (prefetch) eşler. Xcode 26 veya sonrası Background Assets indiricisini derler. Paylaşılan bir `apple.app_group` veya varsayılan `group.<id>` kullanın. Uygulamanın `sign.profile` dosyası ve `<id>.assetdownloader` uzantısının `sign.asset_packs_profile` dosyası her ikisi de o gruba ihtiyaç duyar. `dist/releases` içindeki `.aar` dosyalarını App Store Connect'in varlık paketi yükleme iş akışını kullanarak IPA ile birlikte yükleyin. [Yönetilen varlık paketleri oluşturma](https://developer.apple.com/documentation/backgroundassets/creating-managed-asset-packs)

Bir goroutine'den hazır olma durumunu denetlemek için `mobile.AssetPackReady(ctx, "levels")` ve bildirilen dizine göre bir dosyayı açmak için `mobile.OpenAssetPackFile("levels", "intro.map")` çağırın. Kullandıktan sonra döndürülen okuyucuyu kapatın. [Apple varlık paketlerini yerel olarak test etme](https://developer.apple.com/documentation/backgroundassets/testing-asset-packs-locally)

## Steam depoları

```sh
go tool nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
```

Kanalı varsayılan olarak kullanmak için `updates.channel: steam` ayarlayın. macOS, Windows ve Linux hedeflerini seçin. Başlatma seçeneği olarak macOS'ta uygulama paketini, Windows ve Linux arm64'te yürütülebilir dosyayı ve Linux x64'te `steam-launch.sh` dosyasını kullanarak her `dist/<name>-<version>-<target>` dizinini kendi Steam deposuna yükleyin. Güncellemeleri Steam yönetir. Dizinler yükleyici, kendi kendine güncelleyici veya `steam_appid.txt` içermez.

Linux x64 depoları Steam Linux Runtime 3.0 ve 4.0 için GTK 4 ve bağımlılıklarını içerir. Başlatıcı bunları yalnızca sistemde GTK 4.14 veya daha yenisi olmadığında kullanır. `nsgo-gtk/` dizinini başlatıcının yanında tutun: `licenses/` dizini kütüphanelerin lisanslarını, derlendikleri kaynakları ve LGPL'nin GLib, Pango ve GTK için gerektirdiği derleme betiğini içerir. Linux arm64 çalışma zamanında GTK 4'ün mevcut olmasını gerektirir.

Steam istemcisi dışında yerel test için `nsgo.yaml` içinde `steam: {app_id: 480}` ayarlayın (480 yerine kendi Steamworks App ID'nizi yazın). `go tool nsgo dev` kimliği yerel derleme dizinindeki `steam_appid.txt` dosyasına yazar ve uygulamayı çalışma dizini olarak o dizinle başlatır. [Steamworks API](https://partner.steamgames.com/doc/api/steam_api), [SteamPipe](https://partner.steamgames.com/doc/sdk/uploading).

## GitHub'da yayınlama

Derlemede bir macOS, Windows veya Linux hedefi bulunduğunda `dist/releases` ayrıca `install.sh` ve `install.ps1` betiklerini de içerir; bunlar uygulamayı `release.github`'ın adlandırdığı GitHub deposunun (yoksa uygulamanın Git `origin` uzak deposunun) sürümlerinden yükler; ikisi de yoksa `build` hiçbirini yazmaz ve bunu bildirir. `miniprogram/` olmadan en üst düzeydeki dosyalarını `v<version>` etiketli sürüme yükleyin (`release.tag_prefix`, `v`'nin yerini alır; betikler o önekteki ayrılmış URL karakterlerini kodlar):

```sh
gh release create v1.2.0 dist/releases/*.*
```

Kullanıcılar ardından uygulamayı derlemenin yazdırdığı tek bir satırla kurar veya günceller:

```sh
curl -fsSL https://github.com/me/hello/releases/latest/download/install.sh | sh    # macOS and Linux
```

```powershell
irm https://github.com/me/hello/releases/latest/download/install.ps1 | iex         # Windows
```

macOS'ta betik uygulamayı disk görüntüsünden `/Applications` içine (oraya yazamıyorsa `~/Applications` içine) kopyalar; Linux'ta `sudo` aracılığıyla Arch paketini `pacman` ile, değilse Debian paketini `apt-get` veya `dpkg` ile kurar; Windows'ta yükleyiciyi sessizce çalıştırır ve bu da uygulamanın zaten bulunduğu yere kurulum yapar. Her biri önce dosyayı sağlama toplamına göre denetler. Büyük harflerle uygulamanın adı olan `<NAME>_VERSION` (örneğin `HELLO_VERSION=1.1.0`) başka bir sürümü kurar ve `<NAME>_DOWNLOAD_URL` bir yansıdan (mirror) indirir.

## itch.io'da yayınlama

`nsgo.yaml` içinde `itch.project` alanını itch.io oyununuzun `owner/game` kısa adına (slug) ayarlayın. Kanal adlarını geçersiz kılmak için `itch.channels` alanını derleme hedefine göre ayarlayın. Varsayılanlar web için `html5`, macOS için `osx-<arch>`, Windows için `windows-<arch>` ve Linux için `linux-<arch>` şeklindedir.

```yaml
itch:
  project: example/my-game
  channels:
    macos_arm64: osx-arm64-beta
```

CI'da `BUTLER_API_KEY` gizli değişken olarak ayarlanmışken hedefleri derleyin ve yükleyin.

```sh
go tool nsgo build -channel none -installers=false -target web,macos_arm64
go tool nsgo publish itch -target web,macos_arm64
```

Geçersiz kılarsanız her iki komutta da aynı `-output` ve `-version` değerlerini kullanın. Yükleme her hedefin paket dizinini alır ve itch.io sürümünü derlemenin sürümüne ayarlar. NSGo `PATH` üzerindeki `butler`'ı kullanır veya itch.io'dan sabitlenmiş bir kopyasını indirir. İlk web yüklemesinden sonra itch.io oyun türünü HTML olarak ayarlayın ve yüklemeyi tarayıcıda oynanabilir olarak işaretleyin. [Butler belgeleri](https://itch.io/docs/butler/pushing.html)

## Güncellemeler

`updates` uygulamanın kendisini platformunun ve kullanıcıların onu aldığı kanalın yaptığı şekilde güncel tutmasını sağlar:

```yaml
updates:
  key: zE0hx2FZwwHy6X7LbEP3GjTTE3KWfCAp5gsaBysC+vg=  # the public key nsgo keygen prints
  # channel: store                    # direct, store, steam or none; direct by default with a key, none without
  # feed: https://example.com/hello/  # where the appcasts and installers are; by default the latest GitHub release
  # automatic: false                  # check only when asked; true by default
  # automatic_downloads: true          # Sparkle downloads automatically; false by default, macOS only
```

`nsgo keygen` bir anahtar çifti yazdırır ve hiçbir dosya yazmaz: açık anahtarı `updates.key` içine koyup işleyin (commit), gizli olanı ise deponun dışında, bir parola yöneticisinde ve sürümlerin derlendiği yerde (örneğin bir GitHub Actions gizli değişkeninde) `NSGO_UPDATES_KEY` içinde tutun. Her doğrudan derleme yükleyicileri gizli anahtarla imzalar ve yanlarına uygulamanın `feed` altında okuduğu [Sparkle](https://sparkle-project.org) biçiminde bir akış olan `appcast-<target>.xml` yazar: appcast'leri yükleyicilerle birlikte yükleyin. Gizli anahtar olmadan `build` macOS veya Windows appcast'i yazmaz (çünkü bunlar yalnızca imzalı yükleyicileri kurar) ve imzasız bir appcast okuyan Linux ve Android uygulamaları paketini kurmak yerine sürümün sayfasını açar.

`-release-notes notes.md` sürümün notlarından (örneğin değişiklik günlüğü bölümünden) oluşan bir Markdown dosyasını, dosyanın kendi ham HTML'i olmadan HTML olarak işlenmiş biçimde her appcast'in içine koyar. Sparkle ve WinSparkle bunu güncelleme iletişim kutularında gösterir; Linux ve Android'deki istem ise bunu 1.500 karaktere kadar sürümün altında metin olarak gösterir; uygulamalar bunu `core.UpdateCheck.Notes` olarak okur.

| Hedef | Doğrudan bir derlemenin güncellenme biçimi |
| --- | --- |
| macOS | `build`'in uygulamaya koyduğu Sparkle günlük olarak denetler, yeni sürümü sunar, disk görüntüsünü indirir, imzasını denetler, kurar ve uygulamayı yeniden başlatır; varsayılan uygulama menüsünde *Check for Updates…* bulunur ve kendi menüsünü ayarlayan bir uygulama bunu `updater.Check` ile ekler |
| Windows | Yürütülebilir dosyanın yanındaki WinSparkle, WinSparkle'da mevcut olduğunda uygulamanın başlarken gösterdiği dilde aynı şeyi yapar ve yeni sürümü başlatan yükleyiciyi sessizce çalıştırır |
| Linux, Android | Uygulama appcast'ini okur ve kullanıcı Güncelle'yi seçtiğinde imzalı paketi doğrudan diske aktarır ve bildirilen ilerleme (`updater.Status` ve platform görev ilerleme göstergesi) ile doğrular, ardından `pkexec dpkg`, `pkexec pacman` (`.pkg.tar.gz`/`.pkg.tar.zst`), atomik AppImage değiştirme (Linux'ta yeniden başlatarak) veya Android'in paket yükleyicisi aracılığıyla kurar; bu biçimler dışındaki kurulumlar veya imzasız bir appcast bunun yerine sürümün sayfasını sunar |
| iOS | Uygulama güncellemeyi yapan App Store'a sorar ve oradaki sayfasını sunar |

Yayınlanmış bağımsız bir web uygulaması için `updater.Check(cx)` çalışan sürümü taze bir `nsgo.json` ile karşılaştırır ve onaydan sonra yeniden yüklemeyi önerir. Aynı sürüm etiketi altındaki değişmiş derleme dosyalarını da fark eder. Appcast anahtarı gerektirmez; açık `none`/`steam` kanalları yine de denetimleri devre dışı bırakır ve gömülü konaklar kendi yeniden yüklemelerini yönetir. Mini programlar platformlarının güncelleme yöneticisini kullanır ve yalnızca indirilen güncelleme hazır olduğunda yeniden başlar. Bu hedefler uygulama güncelleyiciyi çağırdığında denetim yapar; mini program indirme/denetleme zamanlaması konağa aittir. Otomatik denetim tercihi ayarlayıcıları sunmazlar. Özel bir `cx.CheckForUpdates` işleyicisi, sürüm etiketi bulunmayabilecek hazırlanmış bir güncelleme için `UpdateCheck.Restart` kullanır.

### Kanallar

Kanal uygulamayı kimin güncelleyeceğine karar verir. `-channel` tek bir derleme için `updates.channel`'ın yerini alır; böylece tek bir proje her biri için derlenir:

| Kanal | Ne için | Paketler |
| --- | --- | --- |
| `direct` | Sürümün kendi dosyalarını kuran GitHub, bir web sitesi, Homebrew cask'leri ve winget | Yukarıdaki gibi kendilerini günceller |
| `store` | App Store, Mac App Store, Microsoft Store, Google Play, Flathub ve diğer mağazalar | [Mağaza paketleridir](#mağaza-paketleri) ve kendi kendine güncelleyici taşımazlar; iOS ve macOS bir anahtar olmadan kendi App Store'larına sorar ve bir Google Play kurulumu Play'in yerel güncelleme akışını kullanabilir |
| `steam` | Steam | Steam'in güncellediği [depo dizinleri](#steam-depoları) üretir |
| `none` | Linux dağıtımları ve uygulamayı yeniden paketleyen herkes | Hiçbir şey denetlemez |

Mağazalar kendisini güncelleyen uygulamaları reddeder; bu nedenle hiçbir zaman doğrudan bir derleme değil, her zaman bir `store` derlemesi yükleyin. macOS veya Windows yükleyicileri içeren her doğrudan sürüm ayrıca bir Homebrew cask (`dist/releases/homebrew/<name>.rb`; `auto_updates true`, derlenen disk görüntüleri için sağlama toplamları ve indirme URL'leri ile uygulamanın her [komutuna](#komutlar) bir `binary` bağlantısı bildirir) ve bir winget çoklu dosya bildirimi (`dist/releases/winget/<id>.yaml`, derlenen NSIS yükleyicileri için `<id>.installer.yaml` ve `<id>.locale.en-US.yaml`) yazar. Sistemin uygulamayı kimin kurduğunu söylediği her yerde uygulama bunu da denetler: başka bir mağazadan gelen bir Android uygulaması, bir Flatpak veya Snap ve bir TestFlight derlemesi güncellemeleri o mağazaya bırakır. Bir Google Play kurulumundaki el ile `updater.Check` çağrısı, izin verildiğinde Play In-App Updates'ten anlık akışını sunmasını ister; onayı, indirmeyi, kurulumu ve yeniden başlatmayı Play yönetir. Geliştirme derlemeleri asla bu akışa girmez. Bağımsız Android paketleri SDK'yı paketler; [gömülü konaklar](embedding.md#android) bunu Gradle aracılığıyla sağlar.

### Uygulamadan denetleme

`updater.Check(cx)` bir *Güncellemeleri Denetle…* menü öğesi gibi denetler ve bulduğunu gösterir; `updater.CheckInBackground(cx)` yalnızca daha yeni bir sürümü gösterir ve `updater.CheckDaily(cx)` bunu günde en fazla bir kez yapar. `updater.Status(cx)` Linux ve Android'de canlı indirme ve kurulum ilerlemesini okur. `automatic` açıkken uygulama Linux'ta, doğrudan Android derlemelerinde ve Apple App Store derlemelerinde başlarken ve kullanıcı uygulamaya her döndüğünde bu şekilde denetler; çevrimdışı durumdaki gibi başarısız olan bir denetim bir saat sonra yeniden denenir. Geliştirme derlemeleri asla güncellenmez. Steam derlemelerinde bu üç işlev hiçbir şey yapmaz.

`updater.CapabilityOf(cx)` bir ağ isteği yapmadan bu kopyanın nasıl güncellendiğini bildirir: `ManagedSelf`, `ManagedPage`, `ManagedDevelopment`, `ManagedChannel`, `ManagedStore` (`Store` adıyla birlikte) veya `ManagedUnavailable`. Bir geliştirme derlemesi, devre dışı bırakılmış kanal veya mağaza kurulumu denetlendiğinde nedene özgü bir açıklama üretir; `core.UpdateCheck.Err` hatalı bir akış veya yüklenemeyen paketlenmiş bir güncelleyici gibi arızalar için ayrılmıştır.

`Capability`, `AutomaticChecks`, `AutomaticDownloads`, `Available`, `Notes` ve `LastCheck` değerlerini ve Linux ile Android'de bir güncellemenin `Phase`, `Loaded` ve `Total` (bir ilerleme çubuğu için `Fraction()`) değerlerini izlemek için bir bağlama içinde `updater.Status(cx)` okuyun. `updater.SetAutomaticChecks(cx, on)` kullanıcının seçimini kalıcı hale getirir ve çalışma zamanında günlük zamanlayıcıyı değiştirir. Sparkle ve WinSparkle kendi tercihlerine sahiptir; Linux, Android ve iOS seçimi özel bir depolama bölümünde tutar. `updater.SetAutomaticDownloads(cx, on)` yalnızca macOS'ta desteklenir ve varsayılan olarak `updates.automatic_downloads` değerini alır; diğer platformlar false bildirir ve ayarlayıcıyı yoksayar. Desteklenmeyen ve harici olarak yönetilen derlemeler her iki ayarı da false bildirir. WinSparkle bulunan güncelleme meta verilerini bulunan geri çağrısından sonra appcast'i okuyarak bildirir; kurulumu hala kendisi denetler. Linux başlatmada Debian veya Arch paket sahipliğini ve AppImage kimliğini çözümler; bu kopyalar `ManagedSelf` bildirirken, paketlenmemiş diğer kopyalar herhangi bir denetimden önce `ManagedPage` bildirir.

## Oturum açılışında başlatma

`desktop.SetLaunchAtLogin(cx, on)` uygulamayı platformun oturum açma mekanizmasına kaydeder ve `desktop.LaunchAtLogin(cx, done)` bunu okur. Geçerli araç tarafından derlenen bir MSIX otomatik olarak devre dışı bir başlangıç görevi içerir; etkinleştirmek kullanıcının Başlangıç uygulamaları anahtarına ve yönetici ilkesine uyar. Sıradan Windows ve Linux kurulumları Run değerini ve XDG otomatik başlatma girdisini korur. macOS paketlenmiş uygulamanın yerel oturum açma hizmetini kullanır.

Flatpak, konak yapılandırma dosyalarına erişim vermeden onay isteyebilen Arka Plan portalını ister. Sonraki bir okuma yerel yanıtı bekler. Portalın salt okunur başlangıç sorgusu yoktur: Flatpak okumaları yalnızca bu çalıştırmadaki son onaylı yanıtı (öncesinde false) döndürür ve uygulama dışındaki bir değişikliği algılayamaz veya bir sonraki çalıştırmada ayarı kurtaramaz.

Harici olarak paketlenmiş bir Snap, uygulaması üzerinde otomatik başlatma dosya adını bildirir:

```yaml
apps:
  app:
    command: bin/app
    autostart: app.desktop
```

NSGo bu dosya adını `$SNAP_USER_DATA/.config/autostart` altına yazar ve snapd bunu uygulamanın sarmalayıcısı aracılığıyla başlatır. `desktop.LaunchedAtLogin(cx)` uygulama derlenmeden önce (bir MSIX StartupTask etkinleştirmesi dahil) bir oturum açma başlatmasını ayırt eder.

## Dosya türleri

`file_types` uygulamanın açtığı belgeleri kaydeder: macOS'ta Finder, Dock ve Birlikte Aç'tan; Windows'ta Windows yükleyicisi veya MSIX aracılığıyla Gezgin'in Birlikte Aç menüsünden; Linux'ta Debian, Arch, AppImage ve Flatpak paketlerinin masaüstü girdisi üzerinden ve iOS'ta Şununla Aç üzerinden:

```yaml
file_types:
  - name: Flow File
    extensions: [flow]
    # id: dev.example.hello.flow   # <id>.<first extension> by default; another's ID imports its type
    # icon: assets/Flow.icns       # macOS document icon; or a square PNG
    # role: viewer                 # editor by default
    # conforms: [public.json]      # public.data by default
    # mime: [application/json]     # existing MIME types; otherwise inferred on Linux
```

Windows yükleyicisi, kullanıcının varsayılanını değiştirmeden uygulamayı Birlikte Aç'a ekler ve yükseltme ile kaldırma sırasında kayıtlarını siler. Linux yaygın uzantıların MIME türlerini sabit bir tablodan adlandırır ve uygulamanın kendi türünün bilinmeyen bir uzantısına paylaşılan bir MIME-info tanımıyla `application/x-<type id>` verir. `mime` bunun yerine mevcut türleri adlandırır; uzantısı tabloda bulunmayan içe aktarılmış bir tür buna ihtiyaç duyar. Bir AppImage çalıştırmak hiçbir şey kaydetmez: girdisini ve tanımlarını bir masaüstü bütünleştirme aracı kurar.

Uygulama, çalışırken daha sonraki bir Windows veya Linux başlatması ve iOS sahne belge açılışları dahil olmak üzere dosyaları `desktop.OnOpenFiles` ile dinler. Uygulamayı başlatan dosyalar ilk işleyiciyi bekler:

```go
desktop.OnOpenFiles(cx, func(paths []string) { open(paths[0]) })
```

iOS'ta normal belgeler özgün adlarıyla uygulamanın sahip olduğu geçici dosyalara aktarılır. Dosya sağlayıcı okumaları UI iş parçacığı dışında koordine edilir ve güvenlik kapsamları işleyici çalışmadan önce sona erer. Saklamak için içe aktarılan dosyayı `cx.DataDir()` içine taşıyın; düzenlemeler kopyayı etkiler. iOS uygulamanın belge simgesini kullanır; `icon` macOS'ta uygulanır. App Clips ve tvOS paketleri hiçbir belge türü kaydetmez. Dosya açılışları `OnOpenLinks`'ten ayrı kalır.

`desktop.AddRecentFile(cx, path)` kullanıcının açtığı bir belgeyi kaydeder: macOS'ta Dock menüsünde ve File › Open Recent içinde, Windows'ta Atlama Listesi'nin (Jump List) Son Kullanılanlar kategorisinde ve Linux'ta masaüstünün son kullanılan dosyalarında. Bir seçim aynı işleyiciye ulaşır. `desktop.ClearRecentFiles`, Linux'ta paylaşılan dosyaların diğer uygulamalardaki geçmişini koruyarak uygulamanın kayıtlarını unutur; Linux ayrıca GTK'nın geçmiş ayarına da uyar.

`desktop.SetDockMenu(cx, items)` uygulamanın simgesine eylemler ekler: macOS'un Dock menüsü, Windows'un Atlama Listesi görevleri ve hızlı listelerin desteklendiği Linux Ubuntu dock, Plasma ve Dash to Dock üzerindeki hızlı liste. Linux, uygulamanın `id` değeriyle eşleşen kurulu bir masaüstü girdisi gerektirir ve alt menüleri, ayırıcıları, işaretli ve devre dışı öğeleri korur. Menü onu ayarlayan kapsama aittir ve uygulamanın pencereleri genelindeki son ayarlayıcı onun yerini alır.

## Bağlantılar

`url_schemes` uygulamaya bir tarayıcıdan, başka bir uygulamadan, `open`, `start` veya `xdg-open`'dan açan `myapp://settings` gibi kendi bağlantılarını verir. macOS, iOS ve Android uygulamaları bunları kaydeder; aynı şekilde Windows yükleyicisi ve MSIX paketi ile Debian, Arch, AppImage ve Flatpak paketlerinin taşıdığı Linux masaüstü girdisi de kaydeder. Kaldırmak bunları siler.

```yaml
url_schemes: [myapp]
# app_links: [links.example.com]  # HTTPS hosts for the full app
```

Uygulama bunları çalışırken de dahil olmak üzere `desktop.OnOpenLinks` ile dinler: Windows ve Linux'ta ikinci bir başlatma bağlantısını çalışan uygulamaya teslim eder ve çıkar. Uygulamayı başlatan bir bağlantı ilk işleyiciyi bekler. Böyle bir bağlantıyı herkes yazabilir ve Windows ile Linux'ta işleyici uygulamanın başlatıldığı her URL'yi duyar; bu nedenle bir bağlantı herhangi bir şeyi değiştirmeden önce şemayı denetleyin ve kullanıcıya sorun:

```go
desktop.OnOpenLinks(cx, func(links []string) { show(links[0]) })
```

iOS ve Android'de `mobile.OnOpenLinks` başlatma sırasında ve uygulama çalışırken bağlantıları dinler. iOS simülatöründe `xcrun simctl openurl booted myapp://settings` veya Android'de `adb shell am start -a android.intent.action.VIEW -d myapp://settings` ile bir bağlantı açın. `app.OpenLinks("myapp://settings")` bir testte bir bağlantı iletir.

iOS evrensel bağlantıları için `app_links: [links.example.com]` ile açık HTTPS ana bilgisayarlarını bildirin. Her ana bilgisayar şema, yol, bağlantı noktası, IP adresi veya joker karakter içermeyen bir DNS adıdır; adlar küçük harfe çevrilir ve yinelemeler reddedilir. Araç, Xcode tarafından yönetilen imzalama dahil olmak üzere tam uygulamanın Associated Domains yetkisine `applinks:<host>` yazar; bir IPA profili buna yetki vermelidir. App Clips, `app_clip.domains` içindeki ayrı `appclips:` taleplerini korur.

Her ana bilgisayar `https://<host>/.well-known/apple-app-site-association` adresinde tam uygulamayı adlandıran bir `apple-app-site-association` dosyası sunmalıdır; paket meta verileri tek başına evrensel bir bağlantıya yetki veremez. [Apple'ın ilişkilendirme gereksinimleri](https://developer.apple.com/documentation/xcode/supporting-associated-domains) sunucu kurulumunu açıklar. Geçici (ad hoc) simülatör imzaları kısıtlı geliştirici yetkilerini dışarıda bırakır; bu nedenle bir temsilci testi sunucu ilişkilendirmesini kanıtlamaz.

iOS'ta `mobile.OpenedURL(cx)`, kök bağlanmadan önceki ilk değer dahil olmak üzere en son App Clip çağrısını, evrensel bağlantıyı veya özel şema URL'sini izler. Özel şema teslimatı yine `mobile.OnOpenLinks`'e ulaşır; bu işleyici çalıştığında çağrı değeri günceldir. Dosya açılışları `OnOpenFiles` içinde kalır ve çağrı URL'sinin yerini almaz. Android/Web çağrı desteği ayrı olarak #1475 numaralı konuda izlenmektedir.

## Android'de gelen paylaşımlar

Uygulamanın Android paylaşım sayfasından işleyeceği MIME türlerini seçin:

```yaml
android:
  share_types: [text/plain, 'image/*', application/pdf]
```

APK ve App Bundle derlemeleri bu türleri hem tekli hem de çoklu öğeler için kaydeder. Her paylaşımı almak için [`mobile.OnShared`](services.md#bağlantıları-ve-dosyaları-açma) kaydedin.

Yol tabanlı adreslere sahip bir web uygulaması için `web.base` ayarını dağıtım dizinine (`/` veya `/my-app/`) yapın ve üretim sunucunuzu uygulama rotaları için `index.html` sunacak şekilde yapılandırın. Geliştirme sunucusu HTML gezinme istekleri için bu geri dönüşü zaten sağlar. Derin bir bağlantı yeniden yüklendiğinde varlıklar bu sabit HTML tabanına göreli kalır. Bkz. [Uygulama adresleri](addresses.md).

## iOS'ta gelen paylaşımlar

Paylaşım uzantısının kabul ettiği Tekdüzen Tür Tanımlayıcılarını (UTI), URL ve metin türleri daha geniş dosya türlerinden önce gelecek şekilde bildirin:

```yaml
apple:
  share_types: [public.url, public.plain-text, public.image, com.adobe.pdf]
  # app_group: group.com.example.app  # defaults to group.<id>
sign:
  # share_profile: assets/Share.mobileprovision
```

`build` ve `dev` uygulamayla birlikte bir Go paylaşım uzantısı (`<id>.share`) paketler. Xcode bunu imzalar ve cihaz çalıştırmaları için Uygulama Grubunu kaydeder. Bir IPA için `<id>` ve `<id>.share` için aynı Uygulama Grubunu etkinleştirin; `sign.profile` ve `sign.share_profile` bunu vermelidir. App Clips, tvOS ve watchOS'ta paylaşım uzantısı yoktur.

Uzantı 128 öğeye ve 512 MiB dosyaya kadar Uygulama Grubuna kopyalar, ardından paylaşım sayfasını tamamlar. `mobile.OnShared` bu toplu işleri uygulama etkinken veya kullanıcı uygulamayı bir sonraki açışında alır; bunlar teslimattan önce uzantının ve uygulamanın çıkması durumunda bile korunur. iOS bir Paylaşım uzantısının kapsayan uygulamasını ön plana getirmesi veya başlatması için desteklenen hiçbir API sağlamaz. Bu aktarım bu nedenle uygulamayı otomatik olarak açmaz.

## Mini programlar

```sh
go tool nsgo build -target miniprogram_wechat -run
go tool nsgo dev -target miniprogram_alipay
```

Her hedef `dist/releases/miniprogram/<platform>` içinde kendi platformunun geliştirici aracı için bir proje yazar; `-run` ve `dev` bunu açar; siz kaydettikçe `dev` yeniden derler ve araç yeniden yükler:

| Hedef | Platform | Araç |
| --- | --- | --- |
| `miniprogram_wechat` | WeChat | Hizmet bağlantı noktası açık (Ayarlar, Güvenlik) [WeChat DevTools](https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html) |
| `miniprogram_qq` | QQ | macOS üzerinde [QQ geliştirici aracı](https://q.qq.com/wiki/tools/devtool/) |
| `miniprogram_alipay` | Alipay | macOS üzerinde [Alipay Mini Program Studio](https://opendocs.alipay.com/mini/ide/download) |
| `miniprogram_douyin` | Douyin ve ByteDance'ın diğer uygulamaları | macOS üzerinde [Douyin geliştirici aracı](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download) |

`miniprogram.appid` her platformda (`wechat`, `qq`, `alipay`, `douyin`) mini programın AppID'sini verir; onsuz WeChat DevTools bunu turist AppID'siyle önizler ve diğer araçlar bir AppID ister. WeChat'te `-run` ve her `dev` yeniden derlemesi CLI üzerinden projeyi kapatıp otomatik açar; böylece DevTools taze dosyaları derler. Bu davranış DevTools 2.01.2510290 ile doğrulanmıştır. Platformun gezinme çubuğu uygulamanın başlığını gösterir; `custom_navigation` ile uygulama sayfayı doldurur ve güvenli alanı onu menü düğmesinden uzak tutar. Uygulama `Env.Simulated` true olarak telefonun işletim sistemini okur, tıpkı web'de `--nsgo-os` ile olduğu gibi: sistem başlığı yerini almadığı için bir yığının sayfaları kendi `Header` bileşenlerini gösterir. QQ, Alipay ve Douyin henüz kendi araçlarında denenmemiştir ve Alipay henüz uygulama üzerinden geriyi, anahtar kareleri veya klavye yüksekliğini almamaktadır.

Mini programlar deneyseldir. Bir mini program TSX değil, yalnızca Go uygulamalarını çalıştırır ve platformlar uygulamanın sıkıştırılmış modülünü 2 MB ile sınırlar: `base` üzerinde birkaç ekranlık bir uygulama tıpkı `examples/gallery-miniprogram` gibi sığar, `ui` denetimlerinden oluşan bir uygulama ise henüz sığmaz ve sığmadığında `build` bunu bildirir. Gölgelendiriciler orada hiçbir şey çizmez; web görünümleri, videolar, yerel bileşenler ve uygulamanın yazı tipi dosyaları henüz gösterilmez.

## HarmonyOS

```sh
go tool nsgo build -target harmonyos -run
```

`harmonyos`, `dist/releases` içine `<name>-<version>-harmonyos.hap` yazar. Motor ArkWeb içinde WebAssembly olarak çalışır ve bir ArkTS konağı bağladıklarını ArkUI görünümleriyle çizer; böylece metin, alanlar, kaydırma ve iletişim kutuları sistemin kendisininkiler olur. `PATH` üzerinde `hdc` varken `-run` HAP'ı bağlı cihaza veya öykünücüye kurar ve başlatır; `nsgo test -target harmonyos` bir uygulamanın testlerini bir öykünücüde imzasız bir HAP'tan ayrı bir `.nsgotest` paket kimliği altında çalıştırır ve kurulu uygulamanın kullanılabilir kalması için çalıştırmadan sonra o test HAP'ını kaldırır.

ArkTS konağını şablonlardan derlemek ve paket adı olarak uygulamanın `id` değerini kullanmak için `PATH` üzerine `hvigorw` koyun ve `OHOS_BASE_SDK_HOME` değişkenini şablonların derleme ve hedef SDK sürümü olan API 26 (`26.0.0`) içeren bir OpenHarmony SDK'sına ayarlayın. `build.harmonyos_bundles` varsayılan `auto`, SDK gerektiren `exact` veya kimliği işlenmiş (committed) konağın sabit uzunluğuna dolduran ve SDK gerektirmeyen `padded` seçeneğini seçer. Hedef testler aynı ayarı kullanır ve hangi konağı paketlediklerini yazdırır. `harmonyos.permissions` uygulamanın istediği `MICROPHONE` ve `CAMERA` gibi `ohos.permission` adlarını listeler ve `widget.title` ana ekran kartının gösterdiği başlıktır.

HarmonyOS bir önizlemedir. Öykünücüler imzasız HAP'lar kurabilir. [Mimari notları](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/harmonyos.md) önizlemenin çizdiği ArkUI özelliklerini listeler.

### İmzalı HAP'lar

Bir HAP'ı AppGallery Connect materyallerinizle imzalamak için `harmonyos.signing` ayarlayın. Bir Huawei telefona doğrudan kurulum o cihaz için bir hata ayıklama sertifikası ve profili gerektirir. Dağıtım için sürüm materyallerini kullanın. [Huawei imzalama kılavuzu](https://developer.huawei.com/consumer/en/doc/harmonyos-guides-V2/command-line-building-app-hap-hvigor-0000001553061909-V2/)

```yaml
harmonyos:
  signing:
    keystore: signing/app.p12
    key_alias: app
    certificate: signing/app.cer
    profile: signing/app.p7b
```

Yollar `nsgo.yaml`'a göredir. Ortamda `NSGO_HARMONY_KEYSTORE_PASSWORD` ve `NSGO_HARMONY_KEY_PASSWORD` ayarlayın. İmzalama SDK'nın `hap-sign-tool.jar` aracını ve `JAVA_HOME` veya `PATH`'ten Java'yı kullanır. Derleme profille eşleşmesi için uygulamanın tam `id`'sini korur; dolayısıyla imzalama `auto` kipinde bile SDK ve `hvigorw` gerektirir. Uygulama dizininde çalıştırılan `go tool nsgo doctor`, parolalar olmadan imzalama yapılandırmasını ve profil paket adını denetler. `build -run` imzalı HAP'ı `hdc`'ye verir.

## İmzalama

```yaml
sign:
  identity: "Apple Development: you@example.com (ABCDE12345)"  # signs the macOS app, else ad hoc
  profile: assets/AppStore.mobileprovision  # signs the IPA
  keystore: assets/release.p12              # signs APKs, with key_alias
  certificate: assets/codesign.pfx          # signs the Windows app and its installer
  # widgets_profile: assets/Widgets.mobileprovision  # the iOS extension's profile; use a macOS profile for a Mac App Store build
  # share_profile: assets/Share.mobileprovision      # signs the IPA's share extension
  # clip_profile: assets/Clip.mobileprovision        # signs the IPA's App Clip
```

Anahtar deposunun parolaları `NSGO_KEYSTORE_PASSWORD` ve `NSGO_KEY_PASSWORD` değişkenlerinden gelir. `-team` iPhone paketini imzalar. `-profile`, `-widgets-profile`, `-share-profile`, `-clip-profile` ve `-macos-profile` komut satırından uygulamanın, araç takımı uzantısının, paylaşım uzantısının, App Clip'in ve Mac App Store'un sağlama profillerini değiştirir. `-installer` Mac Installer Distribution kimliğini değiştirir.

### Windows paketlerini imzalama

Authenticode bir Windows indirmesinin yayıncısını tanımlar; [SmartScreen itibarını ayrı olarak denetler](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation). Bir `.pfx` olan `sign.certificate` yürütülebilir dosyayı, komut eşlikçisini, uygulamanın kaynaklarının getirdiği programları ve kütüphaneleri, kaldırıcısıyla birlikte yükleyiciyi ve Store kanalı için derlenen MSIX'i imzalar; parolası `NSGO_CERTIFICATE_PASSWORD`'dan gelir. İmzalar Go içinde, tıpkı `signtool`'un yaptığı gibi oluşturulur; böylece bir Mac bunları Windows SDK olmadan imzalar:

```yaml
sign:
  certificate: assets/codesign.pfx
  timestamp_url: http://timestamp.digicert.com  # the RFC 3161 server that timestamps them
```

`sign.timestamp_url` varsayılan olarak DigiCert'inkini kullanır ve `none` imzaları zaman damgasız bırakır, bu durumda sertifikanın süresi dolduğunda imzalar geçerliliğini yitirir. Bir donanım belirtecindeki veya bir bulut hizmetindeki sertifika bunun yerine `%1`'in imzalanacak dosya olduğu ve derlemenin yukarıdaki her dosya için çalıştırdığı kendi komutuyla imzalar:

```yaml
sign:
  sign_command: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1"
```

Komut başarılı olur ancak uygulama yürütülebilir dosyasını, komut eşlikçisini, bir PE kaynağını veya kurulum yürütülebilir dosyasını gömülü bir Authenticode imzası olmadan bırakırsa `build` durur.

Yükleyici içindeki kaldırıcı, gömülmeden önce uygulama ve kurulum programıyla aynı `sign.certificate` veya `sign.sign_command` ile imzalanır. `-certificate`, `-sign-command` ve `-timestamp-url` bunları komut satırından ayarlar.

### Noter onayı

Bir Mac'te indirilen uygulamalar, Apple onları noter onayından geçirdikten sonra Gatekeeper uyarısı olmadan açılır. Bir Developer ID ile imzalayın ve notarytool'un anahtar zincirinizde tuttuğu kimlik bilgilerini adlandırın:

```sh
xcrun notarytool store-credentials hello --apple-id you@example.com --team-id ABCDE12345
```

```yaml
sign:
  identity: "Developer ID Application: Example Inc (ABCDE12345)"
  notarize: hello   # the keychain profile above; or -notarize hello
```

`build` ardından güçlendirilmiş çalışma zamanıyla imzalar, disk görüntüsünü gönderir, birkaç dakika Apple'ın kararını bekler ve bileti görüntüye ve uygulamaya zımbalar (staple); bir ret Apple'ın günlüğünü yazdırır. CI'da bir App Store Connect API anahtarının `.p8` dosyası, kimliği ve vereni olan `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` ve `NSGO_NOTARY_ISSUER` profilin yerini alır. Güçlendirilmiş çalışma zamanı uygulama hak talebinde bulunmadığı sürece mikrofonu, kamerayı, takvimleri ve konumu reddeder; `apple.info` içinde `NSMicrophoneUsageDescription`, `NSCameraUsageDescription`, bir takvim veya anımsatıcı kullanım açıklaması ya da bir konum kullanım açıklaması bulunduğunda `build` bunları talep eder.

## Araç takımları ve Canlı Etkinlikler

`build` ve `dev` uygulamanın araç takımlarını ve Canlı Etkinliklerini (Live Activities) çizen uzantıyı ayrı bir adım olmadan Xcode'un Swift derleyicisiyle macOS ve iOS paketlerinin içine derler. iOS'ta uygulama ve araç takımları bir uygulama grubunu paylaşır: bir iPhone üzerinde `-run` Xcode'un her ikisini de imzalamasını ve grubu kaydetmesini sağlar; bir IPA için hem `<id>` hem de `<id>.widgets` için etkinleştirin ve uzantıyı `sign.widgets_profile` ile imzalayın. Android bağımsız APK'ları ve AAB'leri `widgets` içinden App Widget sağlayıcılarını ve kaynaklarını içerir; önceden derlenmiş Java çalışma zamanı, uygulama derlemelerinin hiçbir SDK veya Java derleyicisine ihtiyaç duymadan zaman çizelgelerini çizer. Android 16, Canlı Etkinlikleri hiçbir uzantı gerektirmeyen Live Updates olarak gösterir. [Araç takımları](widgets.md) ve [Canlı Etkinlikler](live-activities.md) bunların yapılandırmasını ve API'sini kapsar.

## Android App Links

`app_links` bağlantıları tam Android uygulamasını açan açık HTTPS ana bilgisayarlarını adlandırır:

```yaml
app_links: [example.com, shop.example.com]
```

APK ve App Bundle her ana bilgisayar için `autoVerify` ile göz atılabilir bir `ACTION_VIEW` filtresi bildirir. Şema, yol, bağlantı noktası, joker karakter veya IP adresi olmadan DNS ana bilgisayarları kullanın. Uygulamanın paket kimliği ve imzalama sertifikası parmak izi ile `https://<host>/.well-known/assetlinks.json` sunun; Android bağlantıları uygulamaya yönlendirmeden önce bu ilişkilendirmeyi doğrular. Araç ilişkilendirme dosyalarını yayınlamaz; [Android'in doğrulama kılavuzu](https://developer.android.com/training/app-links/verify-applinks) bunların kurulumunu açıklar. Gömülü bir Android konağı kendi intent filtrelerini bildirir ve kabul edilen bağlantıları `NsgoView`'ına iletir.

`mobile.OpenedURL(cx)` ilk kök işlemeden önce kabul edilen çağrıyı okur ve başka bir bağlantı çalışan uygulamayı açtığında değişir. `mobile.OnOpenLinks` kabul edilen her bağlantıyı dinlemeyi sürdürür. Tam sayfa bir web uygulaması tarayıcı geçmişi ve hash değişiklikleri dahil olmak üzere geçerli sayfa adresini `mobile.OpenedURL` üzerinden okur; başka bir sayfanın içine bağlanan bir uygulama kapsayan sayfa adresini okumaz. Uygulama kendi adresini de yazdığında [`cx.AppURL`](addresses.md) kullanın.

## App Clips

Bir App Clip, bir iOS uygulamasının uygulamayı yüklemeden bir bağlantıdan, bir koddan veya bir NFC etiketinden çalışan küçük bir parçasıdır. `app_clip`, yalnızca gösterdiğini bağlaması için kendi main paketinden iOS paketinin içine bir App Clip derler:

```yaml
app_clip:
  main: ./clip              # the clip's main package, the app's own by default
  domains: [example.com]    # whose links open it
  # name: Hello Clip        # "<name> Clip" by default
  # id: dev.example.hello.Clip  # <id>.Clip by default
  # minimum: "17.0"         # 16.0 by default; 17.0 allows 100 MB when only links open it
  # notifications: true     # ask for notifications for a day
  # location: true          # let the clip confirm where it was opened
```

Klip kendisini açan bağlantıyı başka bir bağlantı onu yeniden açtıkça değişen `mobile.OpenedURL` ile okur ve `mobile.IsAppClip` kodun bir klip olarak çalışıp çalışmadığını söyler. Sistem yalnızca bağlantılar açtığında iOS 17'den itibaren 100 MB veya diğer durumlarda 15 MB'a kadar bir klip indirir ve `build` daha büyüğünü reddeder; `ui` bağlayan bir klip yaklaşık 11 MB tutar. `dev -target ios_simulator -clip https://example.com/order?table=12` klibi o bağlantıdan açılmış olarak simülatörde tek başına çalıştırır; `-target ios` bunu bir iPhone üzerinde çalıştırır ve bağlantının QR kodunu yazdırır: bağlantıyı Settings > Developer > Local Experiences altına ekleyin, ardından kamerayla tarayın. Geliştirme sembolleri klibi QR boyut sınırının üzerine çıkarırsa `-release` kullanın. Apple App Clip'leri kişisel ekipler için değil, yalnızca Apple Developer Program ekipleri için imzalar.

`dev -target ios` ve `build -run -target ios` ayrıca klibi tam uygulamanın içinde tutar: Xcode bunu ayrı bir hedef olarak imzalar ve uygulamayı imzalamadan önce içine gömer. Otomatik imzalama, Xcode'da hem uygulama hem de klip için sağlama içeren kimliği doğrulanmış ücretli ekip hesabı gerektirir. App Store için `<id>` adına App Clips'i etkinleştirin, klibi `sign.clip_profile` ile imzalayın ve alanların `apple-app-site-association` dosyasını sunun. Deponun `examples/appclip` örneği klibi masada sipariş veren bir kafedir.

Tam uygulamaya bir sipariş veya başka bir yerel veri aktarmak için `app_clip` yanında açık bir grup ayarlayın:

```yaml
apple:
  app_group: group.dev.example.hello
```

Klip ve tam uygulama bu durumda grubun kapsayıcısında `cx.DataDir()` ve `cx.Storage()` verilerini paylaşır. Tam uygulama başladığında veri dizini altındaki dosyalar ve klibin yazdığı anahtar-değer girdileri kullanılabilir olur; `cx.LogsDir()` bu dizini izlerken her paket kendi önbelleğini korur. Xcode imzalaması her iki hedef için de grubu talep eder; profil kullanırken aynı grubu her iki App ID için de etkinleştirin ve `sign.profile` ile `sign.clip_profile` içinde verin. Eksik bir izin imzalamayı reddeder ve erişilemeyen bir grup başlatmayı durdurur. Bir konağın açık veri dizini yine de öncelik kazanır.

`apple.app_group` olmadan her paket kendi korumalı alan depolamasını korur. Ayarı etkinleştirmek daha önce bu korumalı alanlarda depolanmış verileri taşımaz ve yalnızca araç takımı içeren uygulamalar mevcut depolama konumlarını korur. Paylaşılan gizli bilgileri düz depolama yerine anahtar zincirinde (keychain) tutun. Kafe örneği klibinin siparişini kalıcı hale getirir ve bunu tam uygulamada görüntüler.

Klip içinde açık bir kullanıcı eyleminden, StoreKit'ten karşılık gelen tam uygulamasını önermesini istemek için `mobile.ShowAppClipOverlay(cx, mobile.AppClipOverlayBottom)` çağırın. `AppClipOverlayBottomRaised` bir alt çubuk için yer bırakır. Döndürülen işlev bu isteği kapatır; kapsam temizliği de bunu kapatır ve daha eski bir istek daha yeni bir isteği kapatamaz. Yerel bir iOS App Clip dışındaki çağrılar hiçbir şey yapmaz. App Store öneriyi yükleyip görüntüleyebileceğine kendisi karar verir; bu nedenle çağrı ne görüntülenen bir teklif ne de bir kurulum bildirir. Tam uygulamayı kurmadan klibin görevini kullanılabilir tutun. [Apple'ın yer paylaşımı kılavuzu](https://developer.apple.com/documentation/appclip/recommending-your-app-to-app-clip-users)

## Başka bir uygulamaya gömme

`-embed` uygulamayı kendi iOS veya Android uygulamanız için paketler: `-dynamic` ile dinamik çerçevelerden oluşan bir XCFramework ya da `NSGoView` veya `NsgoView` bileşeninin gösterdiği bir AAR. Go masaüstü programları uygulamaları `nsgo.NewView` ile, web sayfaları ise ana bilgisayarı sayfanınki veya `nsgo.yaml` içindeki `web.host`'un adlandırdığı olan sitenin `nsgo.js` betiğiyle gösterir. Deponun `examples/embed-*` örnekleri her birini gösterir; [Gömme](embedding.md) konak tarafını kapsar.
