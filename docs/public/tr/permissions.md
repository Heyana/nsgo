---
title: İzinler
description: Sistemin izinlerini denetleyin ve isteyin, ayarlarını açın ve macOS'ta kullanıcıyı Sistem Ayarları'na yönlendirin.
group: Platform özellikleri
order: 49
---

## Denetleme ve isteme

Sistem, bir uygulamanın yapabileceklerinin bir kısmını kullanıcının vermesine bırakır: telefonlarda ve Mac'lerde mikrofon, kamera, kişiler, takvimler, anımsatıcılar, fotoğraflar, Bluetooth, medya arşivi, konum ve bildirimler; telefonlarda hareket ve arka planda konum; macOS'ta ayrıca Erişilebilirlik, Ekran Kaydı, Giriş İzleme, Tam Disk Erişimi, Uygulama Yönetimi, Geliştirici Araçları ve başka bir uygulamanın Otomasyonu ([konum ve hareket](services.md#location-and-motion), [bildirimler](notifications.md)). `core.Permissions()` bunları `core.Permission` anahtarları olarak listeler. `cx.PermissionStatus` uygulamanın birine sahip olup olmadığını sormadan yanıtlar; `cx.RequestPermission` ise sistemin bir istemi olduğu ve henüz sormadığı yerde o istemle sorar. Kullanıcı izni gerektiren bir şey yaptığı anda isteyin. Face ID ve Touch ID bunların arasında değildir: `cx.BiometricAvailability` onları bildirir ve `cx.AuthenticateBiometric` kendi istemleriyle sorar ([biyometri](services.md#biometrics)).

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

macOS'ta Otomasyon her hedef uygulama için ayrı verilir: çalışan bir hedef uygulamayı paket kimliğiyle denetlemek veya istemek için `core.PermissionAutomationOf("com.apple.finder")`, Sistem Ayarları'nda Otomasyon bölmesini açmak için `core.PermissionAutomation` verin. Çalışmayan bir hedef uygulama `PermissionUnknown` yanıtını verir.

| Durum | `String()` | Anlamı |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | Henüz sorulmadı: bir istek sistemin istemini gösterir |
| `PermissionGranted` | `granted` | Uygulamada var |
| `PermissionLimited` | `limited` | Seçili fotoğraflar veya kişiler gibi kısmi erişim |
| `PermissionDenied` | `denied` | Reddedildi; yalnızca sistemin ayarları bunu değiştirir |
| `PermissionUnknown` | `unknown` | Kısıtlı, ancak sistem veya platform durumu bildirmiyor |
| `PermissionNotGated` | `not-gated` | Linux'ta olduğu gibi, uygulamayı ondan alıkoyan hiçbir şey yok |
| `PermissionUnavailable` | `unavailable` | Platformda böyle bir şey yok |

`Allowed` verilmiş, sınırlı ve kısıtlanmamış durumlar için true'dur. macOS, istemek uygulamayı yalnızca ilgili bölmede listelediğinden, kullanıcı açana kadar Erişilebilirlik, Ekran Kaydı ve Giriş İzleme için reddedildi yanıtını verir ve bir Ekran Kaydı iznini ancak uygulama yeniden başladığında bildirir.

## Kullanım açıklamaları

macOS'ta ve iOS'ta istemek, iznin gerekçesinin uygulamanın Info.plist dosyasında bulunmasını gerektirir; bunu `nsgo.yaml`'ın `apple.info` alanı tutar. Gerekçe yoksa istek kullanılamaz yanıtını verir ve anahtarı günlüğe yazar, çünkü aksi halde sistem uygulamayı sonlandırırdı. Uygulamanın [kataloglarında](localization.md) aynı anahtara sahip bir ileti, örneğin `locales/en.yaml` içinde `NSCameraUsageDescription: Scans the codes you point it at.`, gerekçeyi çevirir ve sistem bunu kullanıcının dilinde sorar; gerekçeyi çevirmeyen bir dil kaynağınkini söyler.

```yaml
apple:
  info:
    NSMicrophoneUsageDescription: Records your voice notes.
    NSCameraUsageDescription: Scans the codes you point it at.
    NSContactsUsageDescription: Finds friends who already use the app.
    NSCalendarsFullAccessUsageDescription: Adds your bookings to your calendar.  # macOS 14 and iOS 17; NSCalendarsUsageDescription before
    NSRemindersFullAccessUsageDescription: Turns tasks into reminders.            # NSRemindersUsageDescription before
    NSPhotoLibraryUsageDescription: Imports photos from your library.
    NSBluetoothAlwaysUsageDescription: Finds your headphones.
    NSAppleMusicUsageDescription: Plays your library.                            # iOS
    NSAppleEventsUsageDescription: Reveals files in Finder.                      # macOS Automation
android:
  permissions:
    - RECORD_AUDIO
    - CAMERA
    - READ_CALENDAR
    - WRITE_CALENDAR
    - BLUETOOTH_CONNECT      # Android 12+; BLUETOOTH on Android 9–11
    - READ_MEDIA_AUDIO       # Android 13+; READ_EXTERNAL_STORAGE on Android 9–12
windows:
  device_capabilities: [microphone, webcam]  # what a store build's MSIX declares
```

Android'de her izin `nsgo.yaml`'ın `android.permissions` alanında listelenmelidir; manifestten dışarıda bırakılan bir izin `PermissionUnavailable` yanıtını verir. `nsgo dev` altında macOS, uygulamanın yaptıklarını onu başlatan terminale yükler; bu yüzden sorulan ve listelenen uygulama terminaldir. Uygulamanın kendi izinlerini test etmek için uygulamayı derleyin.

## Ayarları açma

`cx.OpenPermissionSettings` bir izni veren sayfayı açar ve bir sayfanın açılıp açılmadığını yanıtlar: macOS'ta iznin bölmesinde Gizlilik ve Güvenlik (Otomasyon, Kişiler, Fotoğraflar ve Konum Servisleri dahil) ya da Bildirimler; iOS'ta Ayarlar'daki uygulamanın sayfası (veya iOS 16'dan itibaren Bildirimler sayfası); Android'de Ayarlar'daki uygulamanın sayfası; Windows'ta mikrofonun, kameranın veya bildirimlerin sayfası.

`PermissionLocation` ve `PermissionBackgroundLocation` için macOS Konum Servisleri'ni açar; `PermissionMotion`, macOS hareket örnekleri sağlamasa da Hareket ve Fitness'ı açar. iOS ve Android üçü için de uygulamanın sayfasını açar. Ayarları açmak bir izni ne ister ne de verir.

## Kullanıcıyı yönlendirme

`desktop.GuidePermission` o sayfayı açar ve kullanıcının izni vermesini bekler. macOS'ta bir panel, odağı almadan Sistem Ayarları'nın penceresini izler ve listesine bir uygulamanın sürüklenebildiği bölmeler için uygulamayı kullanıcının oraya sürüklemesi üzere tutar. Uygulama izne sahip olduğunda, kullanıcı paneli veya Sistem Ayarları'nı kapattığında, `stop` çağrıldığında ya da bileşen kaldırıldığında sona erer ve işlevi durumu alır; yeni bir yönlendirme öncekini sona erdirir.

```go
stop := desktop.GuidePermission(cx, core.PermissionAccessibility, desktop.GuideOptions{}, func(s core.PermissionStatus) {
	trusted.Set(s == core.PermissionGranted)
})
```

Sunulan uygulama, uygulamanın paketidir, yoksa yürütülebilir dosyasıdır; `GuideOptions.App` başka birini, `Name` ise panelin ona verdiği adı belirtir. Windows sayfayı açar ve Ayarlar'ın kapanmasını bekler. Diğer yerlerde işlev durumu hemen alır.

## Platformlar

| Platform | Bildirdikleri ve sordukları |
| --- | --- |
| macOS | Her izin (`PermissionAutomationOf` ile çalışan her hedef uygulama için Otomasyon, ayrıca paketlenmiş uygulamalar için bildirimler); yalın Otomasyon, Uygulama Yönetimi, Geliştirici Araçları ve Medya ve Apple Music bilinmez; hareket kullanılamaz |
| iOS | Mikrofon, kamera, kişiler, takvimler, anımsatıcılar, fotoğraflar (`PermissionLimited` ile), Bluetooth, medya arşivi, konum, hareket ve bildirimler |
| Windows | Mikrofon, kamera ve hiçbir zaman istem göstermediği bildirimler; geri kalanı kısıtlı değildir |
| Linux | Korumalı alanların dışında kısıtlı değildir; bildirimler bir bildirim sunucusunun yanıt verip vermediğini yansıtır; Flatpak ve Snap'te kamera masaüstü portalı üzerinden, mikrofon ise portal ve PipeWire üzerinden |
| Web | Mikrofon ve kamera (Firefox'ta bilinmez), bildirimler, konum (arka planda konum değil) ve mobil tarayıcılarda hareket |
| Android | `android.permissions` bildirdiği sürece mikrofon, kamera, takvimler, Bluetooth ve medya arşivi |
| Apple Watch, mini programlar | Kendi servisleri üzerinden bildirimler, konum ve hareket; sistemin kısıtladığı geri kalanı şimdilik bilinmez |

Konum ve hareket izinleri her [platformun algılayıcılarını](services.md#location-and-motion), `PermissionNotifications` ise [bildirimleri](notifications.md) izler. Fitness, Apple basınç verisi dahil `PermissionActivityRecognition` kullanır (Android'de `ACTIVITY_RECOGNITION`, Apple'da `NSMotionUsageDescription`); ham Android hareket verisi ayrı kalır. Mobil Safari'de `PermissionMotion`'ı bir basış işleyicisinden isteyin.

## Test etme

`nsgotest` kendi modelinden yanıt verir: `app.Permissions` her durumu tutar, başlangıçta hepsi sorulmamıştır; sorulmamış birinin istenmesi onu `app.PermissionAnswers`'ın o izin için verdiği değere, yoksa verildi durumuna getirir. `PermissionNotifications`, `app.NotificationPermission` ve `app.NotificationAnswer` ile eşzamanlı kalır. `app.PermissionSettings` açılan ayarları, `app.Guide` ise gösterilen yönlendirmeyi listeler; `app.GrantPermission` ve `app.CloseGuide` bunu sona erdirir.

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## Sonraki adımlar

- [Bildirimler](notifications.md): kendi izinleri, rozetler ve basışlar.
- [Derleme ve dağıtım](building.md): bir paketin bildirdiği yetkilendirmeler ve yetenekler.
