---
title: Bildirimler
description: Bildirim gösterme izni isteyin; bildirimleri gösterin, zamanlayın ve kapatın; sayılarını ve bir görevin ilerlemesini uygulamanın simgesinde gösterin ve basışları dinleyin.
group: Platform özellikleri
order: 45
---

## İzin

`cx.RequestNotificationPermission` kullanıcıdan uygulamanın bildirim göstermesine izin vermesini ister; ilk seferde sistemin istemini kullanır ve bir `core.NotificationPermission` ile yanıt verir: `NotificationsGranted`, `NotificationsDenied` ya da platformun veya derlemenin hiç bildirim göstermediği durumlarda `NotificationsUnavailable`. Kullanıcı bildirim gerektiren bir şey yaptığı anda isteyin: iOS ve Android yalnızca bir kez sorar, tarayıcılar ise kullanıcının tetiklemediği bir istemi reddeder. `cx.NotificationPermission` sormadan yanıtı okur.

```go
cx.RequestNotificationPermission(func(p core.NotificationPermission) {
	if p == core.NotificationsGranted {
		cx.ShowNotification(core.Notification{ID: "chat:42", Title: "Ada", Body: "Lunch?"}, nil)
	}
})
```

## Bir bildirim gösterme

`cx.ShowNotification` uygulama öndeyken de bir bildirim gösterir ve aynı `ID`'ye sahip olanın yerini alır; `Silent` sistemin sesini ve titreşimini dışarıda bırakır. `done` işlevine sistemin bildirimi kabul edip etmediğini bildirir (izin yoksa false) ve kimliği veya başlığı olmayan bir bildirim için hiçbir `done` çağırmadan `core.ErrNotification` döndürür. `cx.CloseNotification` bir kimliğin adlandırdığı bildirimi kaldırır.

```go
err := cx.ShowNotification(core.Notification{ID: "sync", Title: "Synced", Body: "12 notes"}, func(shown bool) {})
cx.CloseNotification("sync")
```

`cx.CloseNotifications` uygulamanın gösterdiği her bildirimi kaldırır ve zamanladıklarını iptal eder. macOS, iOS, watchOS, Windows ve Android'de buna önceki çalıştırmaların Bildirim Merkezi'nde, İşlem Merkezi'nde veya bildirim panelinde bıraktıkları da dahildir; Linux'ta ve web'de yalnızca bu çalıştırmanın gösterdiklerine veya zamanladıklarına ulaşır. Uygulama öne geldiğinde ve penceresi bildirimlerin anlattığını gösterdiğinde çağırın:

```go
cx.Effect(func() {
	if cx.Env().Lifecycle() == core.LifecycleActive {
		cx.CloseNotifications()
		cx.SetBadge(0)
	}
})
```

## Bir bildirim zamanlama

`At` değeri şu andan sonra olan bir bildirim zamanlanır; `cx.ShowNotification`'ı onun `ID`'siyle yeniden çağırmak onu taşır, `cx.CloseNotification` ise gösterilmeden önce iptal eder. `Repeat` onu her gün `At`'in günün saatinde ya da her hafta ayrıca onun hafta gününde, bir sonraki böyle andan itibaren cihazın saat diliminde gösterir. `cx.Env().CanScheduleNotifications()` platformun bildirim zamanlayıp zamanlamadığını söyler: bir paket içindeki macOS, iOS ve watchOS uygulamaları, Android ve Windows bunları uygulama çalışsa da çalışmasa da yerel olarak zamanlar (Windows günlük ve haftalık tekrarları uygulama çalışırken zincirler ve açılışta yeniden kurar), Linux ve web ise (`Notification` kullanılabildiği yerde) bunları uygulama veya sayfa çalıştığı sürece tutar. Diğer yerlerde (merkezi yalnızca rozet gösteren tvOS ve mini programlar) `done` false yanıtını verir.

```go
cx.ShowNotification(core.Notification{ID: "review", Title: "Cards to review", At: due}, nil)
cx.ShowNotification(core.Notification{ID: "habit", Title: "Practice",
	At: time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local), Repeat: core.NotificationDaily}, nil)
```

Android, telefon uyku (doze) modundayken zamanlanmış bir bildirimi zamanından birkaç dakika sonra gösterebilir; çalar saat gibi bildirimi tam dakikasında isteyen bir uygulama `android.permissions` içinde ya Android 12'den itibaren kullanıcının sistem ayarlarından verdiği `android.permission.SCHEDULE_EXACT_ALARM`'ı ya da Google Play'in yalnızca alarm ve takvim uygulamalarına izin verdiği `USE_EXACT_ALARM`'ı listeler. Zorla durdurulmuş bir Android uygulamasının bildirimleri, uygulama bir sonraki başlatılışına kadar bekler. İşlem sonlandıktan sonra teslimatı test etmek için `ShowNotification`'ın true yanıtını vermesini bekleyin, ardından paketi zorla durdurmadan işlemlerini sonlandırın.

## Basışlar

`cx.OnNotificationPress`, onu ayarlayan bileşen yaşadığı sürece, kullanıcının bastığı bir bildirimin `ID`'siyle işlevini çalıştırır ve uygulamayı öne getirir. Uygulamayı başlatan bir basış, kaydedilen ilk işleyiciye ulaşır. Bir basış uygulamayı yeniden başlatabileceğinden, basışın açtığı şeyi `ID` içinde tutun (`"chat:42"`). Windows'ta bunu, yükleyicisinin Başlat menüsüne koyduğu bir uygulama için yapar; aksi halde bir çalıştırma basışları yalnızca çalışırken duyar.

```go
cx.OnNotificationPress(func(id string) { openChat(strings.TrimPrefix(id, "chat:")) })
```

Android 13'ten itibaren izin isteyen bir Android uygulaması, `nsgo.yaml`'ın `android.permissions` alanında `android.permission.POST_NOTIFICATIONS`'ı listeler.

## Rozet

`cx.SetBadge` uygulamanın simgesine bir sayı koyar: macOS'ta Dock'takine, iOS'ta bildirim izni gerektiren Ana Ekran'dakine, Windows'ta görev çubuğu düğmesine, Unity uyumlu Linux başlatıcılarına ve yüklü bir web uygulamasınınkine. 0 veya daha küçük bir sayı onu temizler. Bir macOS uygulaması, bildirim iznini denetleyene veya isteyene ya da bir bildirim gösterene kadar Dock rozeti için izin gerektirmez; o andan itibaren, o çalıştırmada ve sonrakilerde Dock rozeti yalnızca kullanıcı Sistem Ayarları'nda uygulamanın rozetlerine izin verdiği sürece gösterir; `cx.RequestNotificationPermission` bunu uyarılarla birlikte ister. Linux rozet arayüzü olmayan başlatıcılarda ve Android'de ayarlanacak bir sayı yoktur; Android başlatıcıları bildirimleri gösterilen bir uygulamayı işaretler.

```go
cx.SetBadge(len(unread.Get()))
```

## Görev ilerlemesi

`cx.SetTaskProgress` uzun bir görevin ne kadar ilerlediğini uygulamanın simgesinde gösterir: macOS'ta Dock kutucuğunun üzerinde, Windows'ta görev çubuğu düğmesinde ve Unity uyumlu Linux başlatıcılarında bir çubuk olarak. İzin gerektirmez ve rozetin yanında görünür. `TaskProgressNormal`, 0 ile 1 arasında bir kesir olan `Value`'yu gösterir; `TaskProgressPaused` ve `TaskProgressError` onu Windows'ta ve macOS'ta renklendirir; `TaskProgressIndeterminate` Windows'ta hareket eder, macOS'ta ve Linux'ta ise sabit, yarı dolu bir çubuktur; sıfır değerli `TaskProgress` onu temizler. Windows'ta kapsamın penceresinin düğmesinde görünür; uygulamanın kendi kapsamı veya bir tepsi paneli gibi kendine ait düğmesi olan bir pencerenin bulunmadığı yerde ayarlanan ilerleme, uygulamanın hiç ilerleme ayarlamayan her penceresinde görünür. iOS, Android ve web hiçbir şey göstermez.

```go
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressIndeterminate}) // no fraction yet
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressNormal, Value: .43})
cx.SetTaskProgress(core.TaskProgress{}) // done
```

## Sonraki adımlar

- [İzinler](permissions.md): mikrofon ve Erişilebilirlik gibi sistemin diğer izinleri.
- [Dokunsal geri bildirim](haptics.md): cihazın kullanıcının parmağı altında oynattığı geri bildirim.
- [Platform servisleri](services.md): uyarılar, pano, dosyalar, depolama ve ağ.
- [Canlı Etkinlikler](live-activities.md): uygulamanın Kilit Ekranı'nda ve Dynamic Island'da takip ettikleri.
- [Derleme ve dağıtım](building.md): bir paketin ihtiyaç duyduğu izinler.
