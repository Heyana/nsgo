---
title: Resim İçinde Resim
description: Videoyu iOS, tvOS, macOS, Android ve tarayıcıdaki Resim İçinde Resim'de oynatın ya da iOS'ta özel bir ağaç gösterin.
group: Platform özellikleri
order: 45
---

iOS'ta Resim İçinde Resim'i kullanmadan önce arka planda oynatmayı bildirin; macOS bir arka plan modu gerektirmez:

```yaml
apple:
  info:
    UIBackgroundModes: [audio]
```

Bir video için `PictureInPicture`'ı etkinleştirin, bir `core.Ref` tutun ve onu bir uygulama denetiminden başlatıp durdurun. `Possible` true olduğunda o denetimi etkinleştirmek için durum geri çağırmasını izleyin:

```go
ref := &core.Ref{}
state := core.NewSignal(cx, core.PictureInPictureState{})
video := core.Video{
    Source: "asset:clip.mp4", Ref: ref,
    PictureInPicture: true, AutoPictureInPicture: true,
    OnPictureInPicture: state.Set,
}
start := ui.Button{
    Text: "Picture in Picture", OnPress: ref.StartPictureInPicture,
    Disabled: func() bool { return !state.Get().Possible },
}
```

`AutoPictureInPicture`, bir iOS uygulaması arka plana geçtiğinde otomatik girişi etkinleştirir; tvOS ve macOS bu seçeneği yok sayar. `Controls` yine yerel oynatma denetimlerini gösterir; uygulamanın PiP düğmesi, denetimli bir video için aynı yönetilen oturumu kullanır. `OnPictureInPictureRestore`, kullanıcı geri döndüğünde uygulamanın oynatıcı sayfasını göstermesini sağlar. PiP etkinken videoyu bağlı tutun: kapsamını silmek oynatmayı ve PiP'i durdurur.

Android'de paketlenmiş etkinlik PiP desteğini zaten bildirir. Bir video, uygulamanın arayüzünün geri kalanı gizlenmiş olarak etkinliğin PiP penceresine girer ve geri döndüğünde oynatıcısını ve konumunu korur. `StopPictureInPicture` aynı etkinliği genişletir. Pencerenin oynat/duraklat denetimleri `VideoPauseControls` ile birlikte `OnPlay` ve `OnPause`'u bildirir; `Ref.Seek` aynı oynatıcıyı kullanır. Otomatik giriş, açık ayarı ve hazırlanmış, oynayan bir videoyu gerektirir. Bir başlatma, sistemin o anki durumu nedeniyle yine de başarısız olabilir; `Possible` true olsa bile `Error`'ı ele alın.

Gömülü bir Android ana bilgisayarı `android:supportsPictureInPicture="true"` bildirmeli ve `orientation|screenSize|screenLayout|smallestScreenSize` yapılandırma değişikliklerini ele almalıdır. Etkinliğin `onPictureInPictureModeChanged` ve `onUserLeaveHint` çağrılarını `NsgoView` üzerindeki aynı yöntemlere iletin; Android 15+ sürümünde ayrıca `PictureInPictureUiState.isTransitioningToPip()` değerini `NsgoView.onPictureInPictureTransition`'a iletin. Desteği olmayan bir ana bilgisayar veya cihaz `Supported: false` bildirir.

Web'de, standart video PiP API'sini sunan bir tarayıcıda kullanıcının bir basışından başlatın. `Supported` ayrıca sayfanın Permissions Policy'sini yansıtır ve `Possible` yüklenmiş video meta verisi gerektirir. Tarayıcının kendi PiP denetimleri aynı etkin durumu bildirir. Kaynağı durdurmak, değiştirmek veya temizlemek ya da videoyu kaldırmak oturumunu sonlandırır; bekleyen bir başlatma önceki kaynağı yeniden açamaz. Web PiP yalnızca videoyu gösterir; özel NSGo içeriği, otomatik arka plan girişi veya `OnPictureInPictureRestore` yoktur. Desteklenmeyen tarayıcılar `Supported: false` bildirir.

iOS'ta `mobile.PictureInPicture`, kendi düzenine sahip `Children` alır ve varsayılan boyutu 320 × 180 noktadır. Uygulamada yer kaplamaz. `Ref`'i aynı başlatma ve durdurma yöntemlerine sahiptir; `OnState`, `OnRestore`, `OnPlay`, `OnPause` ve `OnSkip` ise sistem eylemlerini iletir. `PausedFunc` ve `PositionFunc`'ı bu geri çağırmaların güncellediği oynatma durumuna bağlayın; `Duration: 0` canlı içeriği tanımlar. Askıya alınmış bir uygulama son işlenen kareyi korur.

Kod çözülmüş karelerini uygulamanın çizdiği altyazılar veya başka katmanlarla birleştirmek için özel ağaca bir `core.Video` koyun. Oturumu dıştaki `Ref` ile başlatın. Video ile bir duraklatma sinyali paylaşın, özel oturumun `PositionFunc`'ını `Video.OnProgress`'ten besleyin ve `OnSkip`'i videonun `Ref.Seek`'ine iletin. Videonun sesi kendi oynatıcısı üzerinden çalar.

Özel ağaç ayrıca metin, görünümler, görüntüler ve Canvas işler. WebView, Metal gölgelendiricileri ve arka plan efektleri yakalanan karelerde kullanılamaz. Animasyonlu görüntüler ve yerel katman geçişleri model durumlarını gösterir. Hareket için açık animasyonlu değerler kullanın. App Store incelemesi videoyla ilgisi olmayan kullanımları reddedebilir.

`Supported: false`, kullanılamayan bir platformu, desteklenen ama hazır olmayan ya da `Error` değeri bir başarısızlığı açıklayan bir oturumdan ayırır. Video; AVKit cihazın PiP desteklediğini bildirdiğinde iOS ve tvOS'u, macOS'u, Android'i ve standart API'yi sunan web tarayıcılarını destekler; özel ağaçlar yalnızca iOS'u destekler. iOS ve tvOS'ta paylaşılan ses oturumu, PiP etkin olduğu sürece (duraklatılmışken de dahil) oynatma kategorisinde kalır ve videonun `AudioSession` değeri `AudioSessionMixWithOthers` olduğunda diğer uygulamaların sesiyle karışır.

Başsız (headless) testlerde `nsgotest.PictureInPictureSupport(supported, possible)` bir iOS, macOS veya Android `nsgotest.Environment` altında sistemin sunduklarını ayarlar; görünümün `PiPState`, `PiPStarts` ve `PiPStops` değerleri oturumu kaydeder; `app.ReportPictureInPicture` sistemin bildireceği bir durumu bildirir ve `app.PictureInPictureControl` pencerenin `play`, `pause`, `skip` ve `restore` eylemlerini gönderir. Videonun PiP penceresi videonun kendi oynatıcısını oynatır ve duraklatır; bunu `Video.OnPlay` ve `Video.OnPause` bildirir, duraklatma için `VideoPauseControls` ile; testlerde `app.PlayVideo` ve `app.PauseVideo` bunları bildirir.

Her iki yol için `go tool nsgo dev -target ios examples/pip`, video PiP için ise `-target macos` veya `-target android` çalıştırın. Yerel Mac testleri, yerel denetimlerle ve denetimler olmadan giriş, çıkış ve tekrarlanan komutları, ayrıca etkin bir denetimli kaynağın değiştirilmesini doğrular. Denetimli videoda tam ekran ve otomatik giriş dahil iPhone kabul testleri henüz beklemededir; gerçekleştirim şu anda yerel delegate/kare testlerine ve başsız uygulama kapsamına sahiptir.

tvOS videosu için `go tool nsgo dev -target tvos_simulator examples/pip` çalıştırın; NSGo oynatıcıyı yerel bir Now Playing oturumuyla ilişkilendirir. tvOS 27 simülatörü videoları yerel denetimlerle ve denetimler olmadan yükler, ancak AVKit orada PiP'in desteklenmediğini bildirir. Gerçek PiP girişi, durdurma ve geri yükleme hâlâ fiziksel bir Apple TV'de doğrulama gerektirir.

Web hedefinin regresyon testi, Chromium'da tarayıcının gerçek PiP oturumunu giriş, çıkış, kaynak değiştirme ve silme boyunca doğrulamak için kaydedilmiş yerel bir klip ve güvenilir uygulama düğmesi basışları kullanır. Diğer tarayıcı motorları hâlâ kabul testine ihtiyaç duyar.
