---
title: Diğer çerçevelerle karşılaştırma
description: "NSGo'nun NSH, MyGo ve diğer UI çerçevelerinin yanındaki yeri ve nelerden ödün verdiği."
order: 2
---

NSGo'nun UI çerçeveleri arasındaki yeri üç tercihe dayanır: uygulamalar Go ile (veya onun üzerinde TSX ile) yazılır, her platform kendi görünümleriyle çizer ve tek bir kod tabanı telefonları, masaüstlerini ve web'i kapsar. Diğer çerçeveler başka tercihler yapar ve her tercihin bir bedeli vardır.

## Bir bakışta

| Çerçeve | Dil | Çizim yöntemi | Hedefler |
| --- | --- | --- | --- |
| NSGo | Go, TSX | Her platformun kendi görünümleri | iOS, App Clips, tvOS, watchOS, Android, macOS, Windows, Linux, web; deneysel olarak mini programlar |
| [NSH](https://nsh.dev) | Rust, TSX | Her platformun kendi görünümleri | iOS, Android, macOS, Windows, Linux, web, GPUI uygulamaları, terminaller; topluluk platformları üzerinden HarmonyOS ve WeChat mini programları |
| [MyGo](https://github.com/egoist/mygo) | Go, web sayfaları | Kendi GPU işleyicisi veya sistem web görünümü | macOS, Windows, Linux, web |
| [Fyne](https://fyne.io), [Gio](https://gioui.org) | Go | Kendi işleyicileri | Masaüstleri, telefonlar, web |
| [Wails](https://wails.io) | Go, web sayfaları | Sistem web görünümü | Masaüstleri |
| [Flutter](https://flutter.dev) | Dart | Kendi işleyicisi | Telefonlar, masaüstleri, web |
| [Compose Multiplatform](https://www.jetbrains.com/compose-multiplatform/) | Kotlin | Kendi işleyicisi; Android'in Compose'u | Telefonlar, masaüstleri, web |
| [React Native](https://reactnative.dev) | JavaScript, TSX | Her platformun kendi görünümleri | iOS, Android; topluluk platformları üzerinden diğerleri |
| [.NET MAUI](https://dotnet.microsoft.com/apps/maui) | C# | Her platformun kendi denetimleri | iOS, Android, macOS, Windows |
| [Avalonia](https://avaloniaui.net) | C# | Kendi işleyicisi | Masaüstleri, telefonlar, web |
| [Qt](https://www.qt.io) | C++, QML | Kendi işleyicisi | Masaüstleri, telefonlar, gömülü cihazlar |
| [Electron](https://www.electronjs.org), [Tauri](https://tauri.app) | Web sayfaları; Tauri için Rust | Pakete dahil Chromium; Tauri için sistem web görünümü | Masaüstleri; Tauri için telefonlar |

## Güçlü yönler

- **Yerel görünümler.** Metin, giriş yöntemleri, seçim, kaydırma, erişilebilirlik ve menüler platformun kendi bileşenleridir; böylece bir uygulama kullanıcıların o platformda beklediği gibi davranır, oysa kendi çizimini yapan bir çerçeve bunların her birini baştan kurar. Yalnızca tek bir platformda bulunan özellikleri — örneğin iOS'un Liquid Glass'ı — diğerleri atmak yerine aynı şekilde taklit eder.
- **İnce taneli güncellemeler.** Bileşenler bir kez çalışır ve sinyaller yalnızca kendilerini okuyan kısımları günceller; fark alınacak bir sanal DOM veya kare başına çalışan bir görünüm işlevi yoktur; animasyonlar platformun kare saati üzerinde çalışır.
- **En küçük paketler ve en hızlı başlangıç.** [Karşılaştırmalarda](https://nsgo.dev/benchmarks/), aynı sayaç uygulaması macOS, Windows ve Linux üzerinde yaklaşık 8 MB boyutunda paketlenir; MyGo, GPUI ve NSH'den (NSH için 33 ila 45 MB) daha küçüktür. Ayrıca en hızlı şekilde başlar ve MyGo ile GPUI gibi GPU üzerinde çizim yapan çerçevelerden daha az bellek tüketir.
- **Yalnızca Go.** Motor, yerleşim ve arka uçlar Go'dur; Go uygulamaları için C, cgo ve JavaScript motoru yoktur.
- **Tek bir Mac'ten her platform.** Go ve Xcode kurulu bir Mac her hedefi çapraz derler: NDK, Android SDK, JDK, GTK veya Windows araç zinciri gerekmez.
- **Tek komuttan her sürüm dosyası.** `nsgo build`; macOS için imzalanmış ve noter onaylı bir disk görüntüsü, imzalı bir Windows yükleyicisi, deb ve Arch paketleri ile bir AppImage, Mac App Store, Microsoft Store, Google Play ve Flathub paketleri ve GitHub sürümlerinden kurulum yapan betikler üretir ([derleme](building.md)).
- **Her platformun kendi güncellemeleri.** macOS'ta Sparkle, Windows'ta WinSparkle bir sonraki sürümü indirir, doğrular ve kurar; Linux ve Android'de uygulama yeni sürümü bulup sürüm sayfasını açar; iOS'ta ise App Store günceller. Bir mağazadan kurulan uygulama o mağazaya bırakılır ([güncellemeler](building.md#güncellemeler)).
- **Her platform.** Telefonlar, tabletler, TV'ler, saatler, masaüstleri ve web; iOS App Clips, iPhone ve Mac widget'ları ve deneysel olarak WeChat, QQ, Alipay ve Douyin mini programlarıyla birlikte tek bir motoru ve tek bir API'yi paylaşır.
- **Gömme.** Bir NSGo uygulaması mevcut bir iOS, Android, Go masaüstü veya web uygulamasının içinde bir görünüm olarak yer alabilir ve uygulamanın ekran ekran taşınmasını sağlar ([gömme](embedding.md)).
- **Hızlı geliştirme döngüsü.** Bir Go dosyasını kaydetmek uygulamayı yeniden derler ve başlatır; bir TSX değişikliği çalışan uygulamayı yerinde günceller ve durumunu korur; derleme hataları ve işleme sırasındaki panikler uygulamanın üzerinde gösterilir ([çalıştırma ve hata ayıklama](debugging.md)).
- **Her yerde tek bir test kümesi.** Testler denetimleri kullanıcılar gibi metin veya role göre bulup kullanır; penceresiz olarak hızla çalışabileceği gibi, her hedefte gerçek pencerelerde platformun kendi girdisiyle değişiklik yapmadan çalışıp ekran görüntüsü de alabilir ([test etme](testing.md)).
- **Yerleşik yerelleştirme.** Bir uygulamanın ve bileşenlerinin sözcükleri kataloglarda tutulur; sahte yerel ayarlar çevrilmemiş, kesilmiş veya sağdan sola yazıma hazır olmayan yerleri bulur ([yerelleştirme](localization.md)).
- **Go ile gölgelendiriciler.** `core.Shader` her platformda Go sözdizimiyle veya WGSL ile yazılmış GPU gölgelendiricilerini çalıştırır ([tuval](canvas.md#gölgelendiriciler-shaders)).
- **İstendiğinde TSX.** Ekranlar, Go bileşenleriyle aynı motor üzerinde Tailwind sınıflarıyla TSX olarak yazılabilir.

## Zayıf yönler

- **Erken aşama.** 0.1.0 ilk sürümdür ve API henüz kararlı değildir; [yol haritası](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md), çalışma zamanında dağıtılan TSX için yığın sınırı gibi eksik olanları listeler.
- **Küçük bir ekosistem.** Flutter, React Native veya Qt'nin yanında çevrimiçi ortamda az sayıda üçüncü taraf bileşen, örnek veya yanıt bulunur.
- **Platformlar farklıdır.** Yerel görünümler platformlar arasında küçük farklar anlamına gelir; bu yüzden bir uygulamanın her birinde kontrol edilmesi gerekir. Kendi çizimini yapan bir çerçeve her yerde aynı görünür.
- **Kendi köprüleri.** Android'de Go programı Java çalışma zamanının yanında çalışır ve onunla borular (pipes) üzerinden haberleşir; Linux'ta ise kullanıcıların GTK 4.14 veya daha yenisine ihtiyacı vardır.
- **TSX yorumlanır.** TSX, Go ile yazılmış bir JavaScript yorumlayıcısı olan goja içinde çalışır; bu da React Native veya Electron'un kullandığı motorlardan daha yavaştır.

## NSH

NSGo, NSH'nin tasarımını taşır: mimari, arka uçlar, API adları ve `base` paketindeki stilsiz bileşenler NSH'ye aittir ([durum](index.md#durum)).

- **NSGo'yu seçin:** Uygulamaları Rust veya TSX yerine Go ile yazmak, Go ve Xcode kurulu bir Mac'ten her hedefi derlemek ya da NSGo'nun eklediklerinden yararlanmak için: Go ile yazılmış GPU gölgelendiricileri, vektör simgeler, bir terminal, kendi pencerelerini, panellerini ve tepsi simgelerini açabilen uygulamalar ve `ui` içindeki stilli bileşenler.
- **NSH'yi seçin:** Daha olgun bir proje, bayt kodu ve yığın sınırlarıyla QuickJS tarafından çalıştırılan TSX, GPUI arka ucu veya 32 bit Android desteği için.

## MyGo

İkisi de cgo olmadan Go ile yazılmış UI çerçeveleridir; ancak MyGo yerel UI'ını kendisi GPU üzerinde çizer ve her karede bir görünüm işlevinden yeniden oluştururken, NSGo yerel görünümlere yalnızca değişen kısımları iletir.

- **MyGo'yu seçin:** Her masaüstünde aynı görünen masaüstü uygulamaları, Go'ya tür güvenli çağrılar yapan web sayfası pencereleri ve masaüstü API'leri ile dağıtım araçları (tepsi simgeleri, dosya ilişkilendirmeleri, yükleyiciler, noter onayı ve imzalı delta güncellemeleri) için.
- **NSGo'yu seçin:** Telefonlara da ulaşmak, her platformun kendi metin, giriş ve erişilebilirlik özelliklerini kullanmak ya da uygulamanın verisinden yeniden çizmek yerine durumu sinyallerde tutmak için.

## Diğer Go araç takımları

- **Fyne ve Gio** Tıpkı MyGo gibi her şeyi kendileri çizer ve ikisi de telefonlara ve web'e ulaşır. Fyne kendi görünümüne sahip eksiksiz bir pencere öğesi seti sunar; Gio ise anlık modlu (immediate mode), düşük seviyeli ve esnektir. İkisinin de yayınlanmış sürümleri ve kullanıcıları vardır; hiçbiri yerel görünmez veya yerel hissettirmez.
- **Wails** Arkasında Go bulunan sistem web görünümünün içine bir web ön ucu yerleştirir: her web kütüphanesi çalışır, ancak arayüz bir web sayfasıdır ve yalnızca masaüstüne yöneliktir.

## Diğer dillerdeki çerçeveler

- **Flutter ve Compose Multiplatform** Her pikseli kendileri çizerler: her platformda birebir aynıdırlar ve büyük ekosistemleriyle olgundurlar, ancak metin, kaydırma ve giriş yerel olmak yerine yeniden inşa edilmiştir ve Dart ya da Kotlin öğrenmeyi gerektirir.
- **React Native** Yerel görünümleri süren tepkisel bir çekirdek modelini NSGo ile paylaşır ve türünün en büyük ekosistemine sahiptir. iOS ve Android merkezlidir ve uygulama mantığını JavaScript'te çalıştırır; NSGo ise bunu Go'da çalıştırır ve masaüstleri ile web'i birinci sınıf hedef olarak görür.
- **.NET MAUI** Linux ve web olmadan her platformun denetimlerini C# ile sarmalar. **Avalonia** WPF'ye yakın şekilde kendi çizimini yapar ve her masaüstünde, telefonlarda ve web'de çalışır.
- **Qt** Olgundur, hızlıdır ve gömülü cihazlara ulaşır; ancak kendi pencere öğelerini çizer, C++ veya QML ister ve LGPL ya da ticari lisans altındadır.
- **Electron ve Tauri** Web sayfaları gösterir: bir tarayıcının bellek maliyeti (özellikle Electron için) ve web sayfası gibi hissettiren bir arayüz pahasına web'in tüm araçlarını ve kütüphanelerini eksiksiz sunarlar.

## Seçim yapma

Erken aşama bir proje kabul edilebilir olduğunda, tek bir Go kod tabanından telefonlarda, masaüstlerinde ve web'de yerel hissettirmesi gereken bir uygulama için NSGo'yu seçin. Yakında kararlı bir API üzerinde yayınlanması gereken bir uygulama için yukarıdaki tablodan olgun bir çerçeve alın: telefonlar için Flutter veya React Native; yalnızca masaüstleri için MyGo, Wails veya Tauri.
