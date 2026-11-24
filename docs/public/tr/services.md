---
title: Platform hizmetleri
description: Uyarılar, pano, dosyalar, fotoğraflar, paylaşım, depolama, gizli bilgiler, konuşma, metin seslendirme, ağ, oturum açma, uygulama içi satın alımlar ve uygulamanın yaşam döngüsü.
group: Platform özellikleri
order: 44
---

## Uyarılar ve pano

`cx.ShowAlert` platformun uyarısını veya eylem sayfasını gösterir. `Actions` sonuçları dizinle geri döner ve bir rol, iptal ve yıkıcı düğmeleri platformun kendi biçiminde çizer. macOS'ta uyarılar ve dosya seçiciler, pencereleri görünür olduğu sürece bir sayfa (sheet) kullanır. Gizli, simge durumuna küçültülmüş veya ekran dışındaki bir pencere ya da penceresi olmayan bir tepsi uygulaması ise iletişim kutusunu ayrı olarak açar. İsteği yapan pencereyi kapatmak onun iletişim kutularını iptal eder.

```go
cx.ShowAlert(core.AlertConfig{
	Title:   "Delete the file?",
	Message: "This cannot be undone.",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(index int) {
	if index == 1 {
		remove()
	}
})
```

`SetClipboard` metin, `SetClipboardImage` ise bir görüntü yazar. `ReadClipboard`, `ReadClipboardImage` ve `ReadClipboardFiles` eşzamansız yanıt verir; `HasClipboardText` ve `HasClipboardImage` ise panonun ne tuttuğunu bir yapıştırma bildirimi göstermeden okur. `core.FileFromURL`, `ReadClipboardFiles` ve `View.OnFileDrop`'un verdiği URL'leri bir `core.File` olarak açar: `file:` URL'leri ve web'de `blob:` URL'leri.

tvOS'ta herkese açık bir pano yoktur. Panoya yazmalar yoksayılır, okumalar içerik döndürmez ve varlık denetimleri false döndürür.

```go
cx.ReadClipboard(func(text string) { pasted.Set(text) })
cx.SetClipboard("Hello")
```

macOS'ta ve Windows'ta `ReadClipboardItems`, 48 MiB'a kadar mevcut tüm bayt temsillerinin bir anlık görüntüsünü verir. Her öğe, yerel bir `Type` ve ham `Data` içeren sıralı `ClipboardRepresentation` değerleri taşır; verisi boş olan, yalnızca varlığı bildirilen biçimler de buna dahildir. `ClipboardFormats` yerel biçim adlarını yüklerini kopyalamadan listeler. Bir anlık görüntüyü geri yüklemek için onu, isteğe bağlı olarak yazma hatasını alan bir geri çağırmayla birlikte `WriteClipboardItems`'a verin. Boş bir dilim panoyu temizler. Kopyalanan metin aynı olsa bile yeni bir kopyalamayı algılamak için `ClipboardChangeCount` değerlerini eşitlik açısından karşılaştırın.

macOS öğe sınırlarını ve pano türü adlarını korur. Windows tek bir öğe kullanır ve `CF_HDROP` gibi bayt tabanlı Win32 adlarını ve `Rich Text Format` gibi kayıtlı biçimleri korur. Yazarken farklı biçimlere sahip öğeleri birleştirir ve boş bir yük için sıfır baytı kullanır. GDI tanıtıcıları, sahip tarafından görüntülenen biçimler ve özel sayısal biçim kimlikleri atlanır. Diğer platformlarda okuma ve yazma geri çağırmaları `CodeUnavailable`, sayaç geri çağırması ise `-1` alır.

## Bağlantıları ve dosyaları açma

`cx.OpenURL` bir URL'yi sistemin tarayıcısında veya uygulamasında açar; `core.CheckURL` ise neyi reddettiğini söyler: şeması olmayan, 16 KiB'tan uzun, boşluk içeren, ana makinesi olmayan bir web adresi ve `javascript:`, `data:`, `blob:`, `file:`, `about:` ve `asset:`.

```go
if err := core.CheckURL(url); err == nil {
	cx.OpenURL(url, func(opened bool) {
		if !opened {
			status.Set("No app opens that link.")
		}
	})
}
```

`cx.OpenFile` uygulamanın sahip olduğu bir dosyayı türüyle ilişkilendirilmiş uygulamada açar; `cx.Share` ise macOS, iOS, Android, Windows, Linux (masaüstü portalı veya dosya yöneticisi aracılığıyla) ve web'de (`navigator.share`) dosyalar, metin veya her ikisi için sistemin paylaşım sayfasını gösterir. Bir `core.SharedFile` diskteki bir yoldur (ya da web'de `Share` için ve bir dosyanın bellekte tutulduğu her yerde onun `Data` baytlarıdır); diğer uygulamanın gördüğü ad ve türle birlikte. Masaüstlerinde `desktop.RevealFile` bir dosyayı veya klasörü Finder'da, Explorer'da ya da dosya yöneticisinde seçili olarak gösterir; `desktop.TrashFile` ise onu Çöp Sepeti'ne veya Geri Dönüşüm Kutusu'na taşır. Hiçbir zaman kalıcı olarak silmez: birimin çöp kutusu olmadığı ya da çöp kutusunun onu tutmayacağı yerde (Geri Dönüşüm Kutusu'ndan büyük bir dosya gibi) `core.ErrNoTrash` yanıtını verir ve öğeyi yerinde bırakır.

```go
cx.Share(core.ShareOptions{
	Files: []core.SharedFile{{Path: path, Name: "Notes.txt"}},
	Text:  "A note from NSGo",
}, func(shown bool, err error) {})
```

`mobile.OnShared(cx, func(items []mobile.SharedItem) { ... })`, [`android.share_types`](building.md#androidde-gelen-paylaşımlar) veya [`apple.share_types`](building.md#iosta-gelen-paylaşımlar) için metin ve dosyalar alır. Android paylaşımları uygulamayı başlatabilir; iOS ise bunları uygulama etkin olana veya kullanıcı onu bir sonraki açışına kadar bir App Group içinde kuyruğa alır. Bekleyen paylaşımlar ilk işleyiciyi bekler. iOS gelen kutusu grubunu geri çağırma dönene kadar tutar; teslimat bir süreç çıkışı boyunca en az bir kezdir. Metin `Text` içinde gelir; dosyalar ise `Path`, `Name` ve `MIME` ile okunabilir önbellek kopyaları olarak gelir. Bir dosyanın önbellekten daha uzun yaşaması gerekiyorsa onu kalıcı depolamaya taşıyın veya kopyalayın.

`desktop.LookUp`, macOS'ta üç parmakla dokunmanın yaptığı gibi, bir metin için sistemin sözlük panelini bir görünümdeki bir kutunun üzerine çizerek gösterir: bir seçimin metni ve kutusu, onun `Selectable` görünümünün `Ref` değeriyle. `desktop.CanLookUp` macOS'ta bir pencerede bunu yapıp yapamayacağını söyler; Windows, Linux ve web'de böyle bir panel yoktur. Testlerde `nsgotest.App.LookUps`, `nsgotest.Environment`'ın `CanLookUp` ayarladığı yerde gösterilen panelleri listeler.

```go
var area core.Ref
var selected core.TextSelection
core.View{Ref: &area, Selectable: true, OnTextSelection: func(s core.TextSelection) { selected = s }, Children: subtitles}
// From a menu item or a shortcut:
if desktop.CanLookUp(cx) {
	desktop.LookUp(cx, selected.Text, &area, selected.Bounds)
}
```

`desktop.QuickLook` bir dizinden başlayarak dosya önizlemeleri ister ve panel açıkken yeniden çağrıldığında dosyaları günceller. `desktop.CanQuickLook` kullanılabilirliği macOS'ta ve Windows'ta uygulamanın kendi penceresinde, Linux'ta ise oturum GNOME Sushi'nin çalışan veya etkinleştirilebilir D-Bus hizmetini sunduğunda bildirir. Windows dosya türünün kayıtlı önizleme işleyicisini kullanır; desteklenen biçimler yüklü işleyicilere veya önizleyiciye bağlıdır. True sonucu platformdan istendiği anlamına gelir, dosyayı işlediği anlamına gelmez.

Boşluk veya Escape paneli kapatır. Ok tuşları Windows'ta ve X11'de seçimi ile sahipliği bildiren Sushi sürümlerinde istekteki dosyaları seçer. Wayland'da Linux şu anda üst öğesi olmayan bir önizleme açar; sahip kapanınca otomatik kapanma ve ok tuşlarıyla seçim kullanılamaz. KDE'ye özgü önizleyiciler ve web desteklenmez.

```go
if desktop.CanQuickLook(cx) {
	desktop.QuickLook(cx, []string{reportPath, sourcePath}, 0)
}
```

## Yerel menü simgeleri

`desktop.MenuItem.Icon`, macOS, Windows ve Linux'ta uygulama, Dock, tepsi ve açılır menülerde bir öğenin yanında simge gösterir. Bir `desktop.MenuIcon`; ayrıştırılmış bir vektör simgesi (`lucide.Folder()` gibi `SVG`), kodlanmış görüntü baytları (`Data`, PNG, JPEG, GIF, WebP veya SVG) ya da yerel/asset/piksel bir `core.ImageSource` (`Source`) alır. `Template` ayarlamak tek renkli bir görüntüyü açık ve koyu görünümü (ve macOS'ta vurgu durumunu) izleyerek menünün metin rengiyle renklendirir; tek renkli `SVG` simgeleri varsayılan olarak şablon kabul edilir.

```go
desktop.SetApplicationMenu(cx, []desktop.MenuItem{{
	Label: "File",
	Items: []desktop.MenuItem{
		{Label: "Open Folder…", Icon: desktop.MenuIcon{SVG: lucide.Folder()}, OnSelect: openFolder},
		{Label: "Export", Icon: desktop.MenuIcon{Data: exportPNG, Template: true}, OnSelect: exportFile},
	},
}})
```

Simgeler yerel kaynaklardan eşzamanlı olarak yüklenir; uzak URL'ler ve geçersiz görüntüler öğeyi simgesiz bırakır. Menü etiketleri, kısayollar ve onay/radyo göstergeleri yerel davranışlarını korur.

Testlerde `nsgotest.MenuItem.Icon` her öğenin simgesini `app.Menu`, `app.DockMenu` ve `app.PopupMenu` içinde kaydeder.

## Fotoğraflar ve dosyalar

`cx.PickImages` sistemin fotoğraf seçicisini gösterir ve uygulamanın baytlarını, adını, türünü ve piksel boyutunu okuduğu `core.PickedFile` değerleriyle yanıt verir. `cx.TakePhoto` platformda kamera olan yerde kamerayı gösterir; `cx.Env().CanTakePhoto` olup olmadığını söyler. macOS, Windows ve Linux'ta tarayıcının kamera önizlemesini bir Fotoğraf Çek düğmesiyle gösterir. `cx.ScanCode` bir kamera önizlemesi açar ve okuduğu ilk kodla yanıt verir: `ScanOptions.Kinds` başkalarını istemedikçe bir QR kodu, kullanıcı kapatırsa nil. `cx.Env().CanScanCode` platformun bir tarayıcısı olup olmadığını söyler: kameralı macOS, iOS ve Android cihazlarda, kamerası olan Windows makinelerinde, Camera portalı ve GStreamer `pipewiresrc` bulunan Linux masaüstlerinde ve `getUserMedia` ile `BarcodeDetector` destekleyen tarayıcılarda. İsteği yapan pencere görünürken macOS bir sayfa, Windows ve Linux pencerenin üzerinde bir modal, web ise sayfanın üzerinde bir katman gösterir. macOS ve iOS için `apple.info` içine `NSCameraUsageDescription`, Android için `android.permissions` içine `CAMERA` koyun. Android, Windows ve Linux PDF417 dışındaki her türü okur. Kodun metnini güvenilmeyen girdi olarak ele alın.

tvOS'ta `PickImages` hiç görüntü olmadan yanıt verir ve `TakePhoto` `core.ErrNoCamera` döndürür; NSGo, Continuity Camera seçicisini sağlamaz.

Testlerde `nsgotest.App.PickImages`, `nsgotest.App.TakePhoto` ve `nsgotest.App.ScanCode`, seçimler ve çekimler için `nsgotest.ImagePick` kayıtlarını, taramalar için `nsgotest.CodeScan` kayıtlarını alır ve sistem arayüzünün yerine yanıt verir. Bu kancalar başsız (headless) olarak ve desteklenen `nsgo test -target` ana makinelerinde çalışır. `nsgotest.Environment(core.Environment{Camera: true, ScanCode: true})` kamerası olmayan bir hedefe kamera verir.

```go
cx.PickImages(core.PickOptions{Limit: 1, MaxDimension: 2048}, func(files []core.PickedFile) {
	if len(files) == 1 {
		avatar.Set(files[0].Data)
	}
})
```

`cx.PickFiles` her türden dosya seçer, `cx.PickDirectories` klasörleri bir `fs.FS` olarak seçer ve `cx.SaveFile` bir dosyayı veya baytları sistemin kaydetme iletişim kutusuyla kaydeder. Her iki seçici de bir `Title`, bir `Button` etiketi ve bir `Start` klasörü alır; `PickFilesOptions.TypesName` ise tür menüsünü adlandırır, böylece bir uygulama bunları kendi dilinde gösterebilir. Bunlar ipuçlarıdır: macOS, Windows ve Linux bunlara uyar, iOS yalnızca `Start`'a uyar, Android ve web ise hiç uymaz. `cx.PickScreenColor`, `cx.Env().CanPickScreenColor()` true olduğu yerde ekranın herhangi bir yerinden bir renk seçer: macOS, Windows, masaüstü Screenshot portalı olan Linux ve `EyeDropper` destekleyen tarayıcılar.

Kaydetme iletişim kutusunu macOS, Windows, Linux ve iOS'ta orada açmak için `core.SaveOptions.Start`'ı var olan mutlak bir klasöre ayarlayın. Boş veya kullanılamaz bir ipucuyla ya da diğer hedeflerde klasörü sistem seçer.

Yazmadan önce seçilen hedefi doğrulamak için `core.SaveOptions.Accept`'i ayarlayın. Motorun goroutine'inde çalışır. Bir hata döndürmek hedefe dokunmadan bırakır. macOS'ta, Windows'ta ve Linux'taki süreç içi GTK seçicisinde iletişim kutusu nedeni gösterir ve kullanıcının başka bir yol seçmesine izin verir. Bir retten sonra iptal etmek `saved == false` değerini, reddedilen yolu ve hatayı bildirir. Android'de ve Linux masaüstü portalında bir ret, kaydetmeyi bu sonuçla tamamlar. Android kullanıcının bir klasör seçmesine izin verir ve istenen dosya adını `name` sorgu parametresi olarak içeren bir ağaç URI'si sağlar. Belgeyi onaydan sonra oluşturur ve başarı durumunda yeni belgenin URI'sini bildirir. Denetim beş saniye içinde yanıt vermezse platform yazmayı bırakır. iOS ve HarmonyOS denetim gerektiren kaydetmeler için bir hata bildirir. Web'in hedef yolu yoktur ve olağan kaydetme akışını kullanır.

Başsız testlerde `App.PickFiles`, `App.PickDirectories` ve `App.SaveFile`; `nsgotest.FilePick`, `nsgotest.DirectoryPick` ve `nsgotest.FileSave` kayıtlarını alır. Aynı kayıtlar `app.FilePicks`, `app.DirectoryPicks` ve `app.Saves` içindedir.

### Paketlenmiş varlıklar

Dosyaları uygulamanın `resources/` dizinine koyun ve Go'dan okumak için `cx.ReadAsset("asset:maps/level.bin", done)` kullanın. Geri çağırma, okumadan sonra motorun goroutine'inde `([]byte, error)` alır ve sahibi ortadan kalkarsa veya yeniden çalışırsa atılır. Eksik dosyaları tanımlamak için `errors.Is(err, core.ErrAssetNotFound)` kullanın. Okuma dosyanın tamamını belleğe yükler.

## PCM sesi çekme

Uygulamanızın çözdüğü veya sentezlediği iç içe geçmiş (interleaved) Float32 PCM'i çalmak için `audio.NewOutput(cx, audio.OutputConfig{SampleRate: 48000, Channels: 2, Render: render})` kullanın. `Start` örnekleri çekmeye başlar, `Pause` çekmeleri durdurur ve sahip kapsam giderken çıkışı kapatır. macOS, iOS, Android, Windows, Linux ve web bu akışı destekler. Diğer hedefler `audio.ErrUnsupported` döndürür.

`Render` doldurduğu kare sayısını döndürür. Ses iş parçacığında çalışır ve engellemeden, bellek ayırmadan ve grafiğe erişmekten kaçınmalıdır. Eksik bir doldurma, kalan kısım için sessizlik çalar ve `Underruns` değerini artırır. `Position` monoton kare sayısını ve o karedeki ana makine zamanını döndürür. Bekleyen kareleri atmak için `Flush`, arabellek gecikmesini okumak için `Latency` kullanın. Yönlendirme değişiklikleri ve kesintiler çıkışı duraklatır ve motorda `OnInterrupt`'ı çağırır. Uygulama onu `Start` ile sürdürebilir.

## Depolama

`cx.Storage()` uygulamanın cihazda tutulan ve uygulamanın her grafiği tarafından paylaşılan anahtar-değer depolamasıdır. Okumalar eşzamanlıdır ve bir hesaplama içinde yapılan okumalar onu değişikliklere abone eder; `SetItem` kota aşıldığında (`core.StorageQuota`, varsayılan olarak 5 MiB) bir `core.StorageFullError` döndürür. Bu, kullanıcının herhangi bir programının okuyabileceği düz bir dosyadır; bu nedenle belirteçleri ve parolaları bunun yerine [gizli bilgiler](#gizli-bilgiler) içinde tutun.

`cx.Store()`, `core.RootOptions.Storage` ile başka bir kök oluşturan bir ana makine için (örneğin kendi goroutine'inde bir eklenti çalıştıran bir ana makine) temeldeki depoyu döndürür: o zaman her iki kök de uygulamanın girdilerini ve cihazda tutulanları paylaşır. Bu, `cx.ProvideStorage`'ın uygulamanın yazmadığı koda verdiği bölüm değil, deponun tamamıdır.

Kendi kalıcılığını sağlayan bir ana makine `core.NewStore(backend)`'i bir `core.StorageBackend` ile çağırır: `Load() (map[string]string, error)` bir kez yükler ve `Write(core.StorageChange)` değişiklikleri sırayla alır. `change.Op` üzerinde dallanın (`core.StorageSet`, `core.StorageRemove`, `core.StorageClear`) ve `change.Key` ile `change.Value`'yu kullanın. Yalnızca bellekte tutulan depolama için `nil` verin.

```go
store := cx.Storage()
if tab, ok := store.GetItem("tab"); ok {
	selected.Set(tab)
}
store.SetItem("tab", "library")
```

`store.Keys()` anahtarları listeler, `RemoveItem` birini siler ve `Clear` depolamayı boşaltır. `cx.DataDir()` ve `cx.CacheDir()`, anahtar-değer depolamasının yetmediği dosyalar için uygulamanın klasörlerini verir; `cx.LogsDir()` platformun günlük dosyaları dizinini verir ve bir `Scope.ProvideStorage` bir alt ağaca kendine ait bir bölüm verir.

Bir iOS App Clip ve tam uygulaması, `app_clip` ve açık bir `apple.app_group` yapılandırıldığında bu veri ve depolama konumlarını paylaşır; önbellekleri ayrı kalır. Grubu etkinleştirmek mevcut sandbox verilerini taşımaz. Yapılandırma ve imzalama [App Clip depolaması](building.md#app-clips) bölümünde anlatılır.

HarmonyOS'ta `os.ReadFile`, `os.WriteFile` ve `os.Rename` gibi Go dosya işlemleri bu dizinler ve `os.TempDir()` altında çalışır. Göreli yollar files dizininde başlar. Tavsiye niteliğindeki dosya kilitleri ve sembolik bağlantılar kullanılamaz.

### Kullanıcının klasörleri

`desktop.UserDir(cx, desktop.DownloadsDir)` kullanıcının klasörlerinden birinin yolunu döndürür: `DocumentsDir`, `DownloadsDir`, `DesktopDir`, `PicturesDir`, `MusicDir` veya `VideosDir`. Her seferinde sisteme sorar, böylece kullanıcının taşıdığı bir klasör izlenir: macOS'ta ev dizinindeki klasörler (videolar için `Movies`), Windows'ta OneDrive'ınkiler dahil bilinen klasörler ve Linux'ta XDG kullanıcı dizinleri. Klasör henüz var olmayabilir. Telefonlarda, web'de ve HarmonyOS'ta bir uygulamanın yol üzerinden ulaştığı böyle klasörler yoktur; `user-dirs.dirs` dosyasının dışarıda bıraktığı bir klasör için Linux'ta da yoktur: `UserDir` `core.ErrUserDirUnavailable` döndürür ve uygulama bunun yerine kullanıcıya `cx.PickDirectories` veya `cx.SaveFile` ile sorar.

```go
dir, err := desktop.UserDir(cx, desktop.DownloadsDir)
if err != nil {
	dir = cx.DataDir()
}
```

Testler kullanıcının klasörlerini asla almaz: her biri testin geçici dizini içinde boş bir dizindir ve `nsgotest.UserDirs(map[core.StandardDir]string{…})` bunları sabitler; dışarıda bırakılan bir klasör kullanılamaz.

### Çökme raporları

Yerel sürüm derlemeleri Go çökme izlerini `cx.LogsDir()/crashes` altında tutar. Apple yakalanmamış Objective-C istisnalarını, Android yakalanmamış Java istisnalarını, Windows ise en üst düzey filtresine ulaşan SEH istisnalarını kaydeder. Bekleyen raporları bir sonraki açılışta `cx.CrashReports()` ile listeleyin. Her rapor, onu üreten sürümü, derlemeyi ve platformu tanımlar. Tanılama metnini okumak için `Open()`'ı çağırın, okuyucuyu kapatın, ardından uygulama raporu işlediğinde silmek için `Acknowledge()`'ı çağırın. Bu dosya işlemlerini bir goroutine'de çalıştırın.

## Disk alanı

`cx.DiskSpace(func(space core.DiskSpace, err error) { … })`, `cx.DataDir()`'i içeren birimi eşzamansız olarak sorgular ve motor goroutine'inde geri çağırır. `Total` sistemin bildirdiği kapasitedir, `Available` uygulamanın kullanıcının istediği bir işlem için kullanabileceği alandır ve `Opportunistic` önbellekler ve önceden getirme için kullanabileceği alandır. Üçü de bayt cinsindendir. `Available`, sistemin bu rakamı bildirdiği yerde talep üzerine geri kazandığı alanı içerir. Tek bir boş alan rakamı olan platformlar onu her iki alanda döndürür. Web'de `Total` tarayıcının kaynak (origin) için tahmini depolama kotası, `Available` ise kullanılmayan kısmıdır. Hizmet kullanılamadığında `core.ErrDiskSpaceUnavailable` döndürür.

Apple derlemeleri, `nsgo.yaml` içindeki `apple.privacy.accessed_apis`'in `NSPrivacyAccessedAPICategoryDiskSpace`'i bildirmesini gerektirir. Yazmadan önce alanı denetlemek veya alan azaldığında kullanıcının gözlemleyebileceği bir değişiklikle dosyaları boşaltmak için `E174.1` bildirin. Kullanıcıya kullanılabilir alanı göstermek için `85F4.1` bildirin. Uygulama, okumaları kullanma ve paylaşma konusunda Apple'ın kısıtlamalarına uymalıdır. Kategori olmadan yapılan bir derleme `core.ErrNotDeclared` döndürür. [Apple'ın onaylı nedenleri](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)

Bir testin yanıtını `nsgotest.DiskSpace(space, err)` ile ayarlayın.

## Bulut depolama

`cloud` paketi küçük anahtar-değer girdilerini (`cloud.Get`, `cloud.Set`, `cloud.Delete`, `cloud.Keys`) ve daha büyük dosyaları veya kayıt anlık görüntülerini (`cloud.ReadFile`, `cloud.WriteFile`, `cloud.DeleteFile`, `cloud.ListFiles`) cihazlar arasında eşitler: macOS ve iOS'ta iCloud Key-Value Store ve ubiquity belge kapsayıcıları, Android'de Google Play Games Saved Games.

Her işlem geri çağırmasını motor goroutine'inde iletir ve `cx` atılmışsa onu atlar. `cx.Env().CanUseCloudStorage()` bulut depolamanın kullanılabilir olup olmadığını bildirir; kullanıcı oturumu kapattığında, yetkiler (entitlements) bulunmadığında veya platformun yerel bir bulut arka ucu olmadığında (Windows, Linux ve web) her çağrı `cloud.ErrUnavailable` döndürür ve yazmaları hiçbir zaman sessizce düşürmez.

```go
if cx.Env().CanUseCloudStorage() {
	cloud.WriteFile(cx, "saves/slot1.sav", saveBytes, func(err error) {
		if err != nil {
			status.Set(err.Error())
		}
	})
}

cloud.WatchChanges(cx, func(ev cloud.ChangeEvent) {
	reload(ev.Keys, ev.Files)
})

cloud.OnConflict(cx, func(c cloud.Conflict) {
	_ = c.Resolve(mergeSaves(c.Local, c.Remote))
})
```

`cloud.WatchChanges` ve `cloud.OnConflict` dışarıdan gelen çok cihazlı değişiklikleri ve sürüm çakışmalarını motor goroutine'inde iletir ve `cx` atıldığında kendilerini otomatik olarak temizler. Çakışan bir dosyayı okumak bir `*cloud.ConflictError` döndürür (`errors.Is(err, cloud.ErrConflict)` ile eşleşir).

`nsgo build` içinde Apple iCloud yetkilerini etkinleştirmek için `nsgo.yaml`'a bir `cloud:` (veya `icloud:`) bölümü ekleyin:

```yaml
cloud:
  kv_store: true
  containers:
    - iCloud.com.example.myapp
```

İmzalama profili, istediğiniz iCloud yeteneklerini vermelidir. macOS'ta yalnızca Mac App Store derlemeleri (`sign.macos_profile`) iCloud taşır, çünkü macOS bunu yalnızca bir provisioning profiliyle verir; bir Developer ID derlemesi onu bir uyarıyla dışarıda bırakır. Android'de bulut depolama, uygulamada Play Games Services v2 SDK'sını ve oturum açmış bir oyuncuyu gerektirir.

Steam gibi masaüstü mağazalarında Steam Auto-Cloud, hiç ek kod gerektirmeden `cx.DataDir()`'i doğrudan eşitler: yerel kayıt dosyalarını oraya yazmaya devam edin ve dizini Steam'in eşitlemesine izin verin.

Testlerde `nsgotest` her uygulamaya yalıtılmış, bellek içi bir bulut deposu verir: kullanılabilirliği `nsgotest.CloudAvailable` veya `app.SetCloudAvailable` ile açıp kapatın, dışarıdan gelen çok cihazlı güncellemeleri `app.SimulateCloudChange` ile taklit edin ve çakışma çözümünü `app.SimulateCloudConflict` ile test edin.

## Gizli bilgiler

`cx.Secrets()` yenileme belirteçleri, API anahtarları ve parolalar için uygulamanın güvenli deposudur: platformun koruduğu bir dizeyi hesaba göre `Get`, `Set` ve `Delete` ile okuyun, yazın ve silin. Eksik bir hesap `("", false, nil)` olarak okunur; web'de olduğu gibi platformun güvenli bir deposu olmadığı yerde her çağrı `core.ErrSecretsUnavailable` döndürür; NSGo hiçbir zaman düz depolamaya geri dönmez. Sistem, kullanıcıya soran bir anahtar zincirinin yaptığı gibi bir çağrıyı bekletebilir; bu nedenle bunları motorun goroutine'i dışında çağırın ve sonucu `cx.Post` ile geri verin.

```go
secrets := cx.Secrets()
go func() {
	token, ok, err := secrets.Get("refresh-token")
	cx.Post(func() {
		if err == nil && ok {
			login(token)
		}
	})
}()
```

| Platform | Gizli bilginin tutulduğu yer |
| --- | --- |
| macOS | Oturum açma anahtar zincirinin genel bir parolası: `kSecAttrService` hizmet, `kSecAttrAccount` hesaptır |
| iOS, tvOS, watchOS | Anahtar zincirinin genel bir parolası, aynı şekilde |
| Windows | Hedefi `service/account` olan, en fazla 2560 baytlık bir Credential Manager genel kimlik bilgisi |
| Linux | `service` ve `account` özniteliklerine sahip bir Secret Service (GNOME Keyring, KWallet) öğesi; oturumun hiçbirini çalıştırmadığı yerde kullanılamaz |
| Android | Android Keystore'un bir AES anahtarıyla şifrelenmiş olarak, uygulamanın özel dosyalarında, yedeklerin dışında; okumalar ayrıca `expo-secure-store`'un kendi `SecureStore` `SharedPreferences`'ı içine şifrelediği değerlere de geri döner |
| WeChat mini programları | Platform tarafından şifrelenmiş depolama (`encrypt: true` ile `wx.setStorage` / `wx.getStorage`, temel kitaplık 2.21.3+); daha eski temel kitaplıklarda kullanılamaz |
| Web, diğer mini programlar | Kullanılamaz (`core.ErrSecretsUnavailable`): tarayıcıların işletim sistemi tarafından korunan bir istemci gizli bilgi deposu yoktur (web oturumlarını sunucunun ayarladığı `HttpOnly` çerezlerinde tutun) ve QQ, Alipay ile Douyin mini programlarında şifrelenmiş depolama yoktur |

Hizmet varsayılan olarak uygulamanın kimliğidir (`core.App().ID`). Başka bir çatıdan taşınan bir uygulama, o çatının yazdığı öğeleri adlandırarak kullanıcılarının oturumunu açık tutar: `Service` ve Apple platformlarında `AccessGroup` ile `Accessible`. Android'de `expo-secure-store`'un `SharedPreferences` içine şifrelediği değerleri okumak için `Service`'i onun `keychainService` değerine (varsayılan olarak `"key_v1"`) ayarlayın; yazmak veya silmek taşınan girdiyi temizler. iOS'ta `core.SecretAfterFirstUnlock`, telefon kilitliyken arka plan işinin bir öğeyi okumasına izin verir ve bir `ThisDeviceOnly` erişilebilirliği onu yedeklerin dışında tutar. macOS'ta bir `AccessGroup`, öğeleri veri koruma anahtar zincirine taşır; bu, `keychain-access-groups` yetkisi grubu listeleyen bir ekip tarafından imzalanmış bir derleme gerektirir ve hem Mac App Store hem de Developer ID derlemeleri için `nsgo.yaml` içinde `macos.entitlements` altında yapılandırılır.

```go
s := cx.Secrets()
s.Service = "app:no-auth" // the service an earlier build used
s.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
err := s.Set("refresh-token", token)
```

`nsgotest` bir uygulamanın gizli bilgilerini bellekte, `App.Secrets` içinde tutar ve `nsgotest.Secrets` bir testi bazı gizli bilgilerle başlatır; böylece bir test hiçbir zaman makinenin anahtar zincirine dokunmaz. Bir TSX paketi depoya kendine ait bir hizmet altında `secrets` yeteneğiyle ulaşır.

## Ağ

`cx.Env().Network()` sistemin kullanılabilir bir rotası olup olmadığını (`Online`) ve bu rotayı ölçülü veya pahalı kabul edip etmediğini (`Expensive`) bildirir. Bağlamalar bunu diğer her ortam değeri gibi izler. `cx.OnNetworkChange(func(core.Network))`, kapsamı yaşadığı sürece her değişiklik için işleyiciyi motorun goroutine'inde çağırır. Sistemin rotası, belirli bir sunucunun yanıt verip vermediği hakkında hiçbir şey söylemez. Bağlantıyı bildiremeyen bir hedef varsayılan olarak çevrimiçidir ve bilinmeyen maliyet varsayılan olarak false'tur.

Çevrimdışı davranışı test etmek için `core.Environment{Network: &core.Network{Online: false}}` değerini `nsgotest.Environment` veya `App.SetEnvironment` aracılığıyla ayarlayın.

Go'nun kendi `net/http` paketi, sistemin TLS ve proxy ayarlarıyla her hedefte istek yapar. Sinyaller motorun goroutine'ine aittir; bu nedenle başka bir goroutine'den gelen bir yanıt onlara `cx.Post` aracılığıyla ulaşır.

```go
go func() {
	resp, err := http.Get("https://api.example.com/items")
	if err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	defer resp.Body.Close()
	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	cx.Post(func() { list.Set(items) })
}()
```

`cx.Post` işlevini, sinyallerin ayarlanabildiği grafiğin goroutine'inde çalıştırır. Android'de manifest internet iznini ister ve görüntü indirmeleri kendilerini diskte önbelleğe alır.

`core.NewResource` aynı şeyi bir kaynaktan yüklenen bir değer için yapar. Getiricisi kendi goroutine'inde çalışır ve kaynak değiştiğinde yeniden yükler:

```go
items := core.NewResource(cx, func() (string, bool) { return query.Get(), query.Get() != "" },
	func(ctx context.Context, q string, _ core.ResourceInfo) ([]Item, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/items?q="+url.QueryEscape(q), nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var list []Item
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return nil, err
		}
		return list, nil
	})
```

`items.Get()`, `items.Err()` ve `items.Loading()` bağlamaların izlediği sinyallerdir. Yeni bir sorgu hâlâ çalışan isteği `ctx` aracılığıyla iptal eder; böylece geç gelen bir yanıt hiçbir zaman daha yenisinin yerini almaz. `items.Refetch()` yeniden dener ve yüklenirken son listeyi korur; `items.Mutate` ise iyimser bir güncelleme için bir değeri hemen ayarlar.

`core.Suspense`, içeriğinin okuduğu kaynaklar ilk değerlerini yüklerken bir yedek gösterir; böylece bir ekranın kendine ait bir yükleme bayrağına ihtiyacı olmaz:

```go
core.Suspense{
	Content:  func(cx *core.Scope) core.Node { return itemList(cx, items) },
	Fallback: func(*core.Scope) core.Node { return ui.Spinner{Label: "Loading"} },
}
```

İçerik beklerken bağlı ama gizli kalır; böylece değerler geldiğinde hemen görünür. Yeniden getirme, onu son değerle görünür tutar.

### Yerel hizmetleri keşfetme

macOS, iOS, Android, Windows ve Linux'ta `cx.AdvertiseService` dinlenen bir bağlantı noktasını ve TXT özniteliklerini yayımlar. `cx.BrowseServices` çözümlenen hizmetleri ana makineleri, bağlantı noktaları ve TXT öznitelikleriyle bildirir. `Host` bir DNS ana makine adı veya bir IP adresi olabilir. Bir hizmet ayrıldığında geri çağırma onu `Removed` ayarlanmış olarak yeniden alır. Platform desteği için `cx.Env().CanDiscoverServices()`'i denetleyin. Her iki çağrı da bir durdurma işlevi döndürür ve kapsamları atıldığında sona erer. Bir platform hatası motor goroutine'inde eşzamansız olarak bildirilir. Linux Avahi kullanır ve arka plan hizmeti (daemon) kullanılamıyorsa bir hata bildirir.

Apple platformlarında Bonjour için hizmet türlerini bildirin ve yerel ağ erişimini `nsgo.yaml` içinde açıklayın. Sistem izin istemini yönetir ve kullanıcının seçimini hatırlar.

```yaml
apple:
  info:
    NSLocalNetworkUsageDescription: Finds nearby devices to sync with.
    NSBonjourServices:
      - _example._tcp
```

Android `NsdManager` kullanır. Android 12 ve öncesi ile T Extensions 7'den önceki Android 13, Wi-Fi keşfi için `CHANGE_WIFI_MULTICAST_STATE` gerektirir. Bağımsız derlemeler bu izni bildirir. NSGo'yu bir Android ana uygulamasına gömerseniz, bu cihazlar için izni ana uygulamanın manifestine ekleyin. Android'in ham TXT API'si (API 37 veya T Extensions 22) olmayan cihazlarda yayımlanan TXT değerleri geçerli UTF-8 olmalıdır. Tarama ham TXT değerlerini korur. Bir Android ana uygulaması API 37 veya üstünü hedefliyorsa, Android 17 veya sonrasında `ACCESS_LOCAL_NETWORK` iznini bildirmeli ve istemelidir.

Başsız bir testte tarama sonuçları sağlamak için `nsgotest.LocalServices` kullanın.

## Şimdi Oynatılıyor

Uygulamanın geçerli medyasını macOS'un Denetim Merkezi'nde ve menü çubuğunda göstermek için `cx.SetNowPlaying(core.NowPlaying{...})` kullanın. `Title`, `Artist`, `Album` ve isteğe bağlı olarak `*core.ImageSource` biçiminde `Artwork` verin. Sıfır bir `Duration` canlı yayın anlamına gelir. Geçerli `Position`'ı ve oynatma `Rate`'ini (duraklatılmışken sıfır, normal hızda bir) verin. Parça, konum veya hız değiştiğinde yeniden yayımlayın. Sistem çağrılar arasında konumu ilerletir.

Desteklenen aktarım denetimlerini `Commands` içinde listeleyin; örneğin `core.MediaPlay`, `core.MediaPause`, `core.MediaTogglePlayPause` ve `core.MediaSeek`. İstekleri `cx.OnMediaCommand` ile işleyin. Geri çağırma komutun `Kind` değerini ve bir sarma (seek) için istenen `Position`'ı alır. Komutları en yeni canlı işleyici alır. Uygulama oynatmayı bitirdiğinde `cx.ClearNowPlaying()`'i çağırın. Uygulama elle yayımlarken macOS'taki denetimli videolar kendi yayımlarını askıya alır. Platform desteği için `cx.Env().CanShowNowPlaying()`'i denetleyin.

## Arka plan indirmeleri

`cx.Download` ve `cx.OnDownloadUpdate`'i bağımsız iOS ve macOS uygulamalarında ve Android'de kullanın. Gömülü Apple görünümlerinin aktarım ana makinesi yoktur. Android uygulamaları `nsgo.yaml` içinde `android.background_downloads: true`'yu etkinleştirir. Uygulama görünürken bir kullanıcı eyleminden bir indirme başlatın. Uygulamanın yazabileceği, üst dizini var olan mutlak bir hedef kullanın.

```go
cx.OnDownloadUpdate(func(p core.DownloadProgress) {
    progress.Set(p)
})
task, err := cx.Download(core.DownloadRequest{URL: url}, path, core.DownloadOptions{
    CertificatePEM: serverCertificate,
})
```

Aktarım GET kullanır ve isteğe bağlı istek üst bilgilerini kabul eder. `CertificatePEM` sunucunun yaprak sertifikasını sabitlemedikçe normal HTTPS güven deposunu kullanır. Sabitleme ayrıca eşleşen bir ana makine adı ve geçerli bir sertifika gerektirir. Hizmet o URL'den başarılı bir yanıt gerektirdiğinden son URL'yi kullanın.

Apple'da, herkese açık bir ana makine adındaki kendinden imzalı bir sertifika, `apple.info` içinde o ana makine için bir App Transport Security istisnası gerektirir. Aktarım yine de tam sertifika sabitlemesini, ana makine adını ve geçerliliği denetler. Yerel bir NAS bağlantısı ayrıca erişimi açıklayan bir `NSLocalNetworkUsageDescription` gerektirir; nitelenmemiş veya `.local` ana makineler `NSAllowsLocalNetworking` kullanır. [Apple'ın ATS ayarları](https://developer.apple.com/documentation/security/preventing-insecure-network-connections)

```yaml
apple:
  info:
    NSAppTransportSecurity:
      NSExceptionDomains:
        files.example.com:
          NSExceptionAllowsInsecureHTTPLoads: true
```

Android ön plan hizmeti, etkinlik ekrandan ayrıldıktan sonra da devam eder. Android'in bildirim iznine bağlı olarak bir ilerleme bildirimi gönderir. İlerleme bayt sayılarını kullanır ve bilinene kadar `Total`'ı -1 olarak ayarlar. Başarı, hedefi atomik olarak değiştirir. `task.Pause()` kısmi dosyayı korur, `task.Resume()` sunucu bir doğrulayıcı sağladığında bir HTTP aralığı kullanır ve `task.Cancel()` kısmi dosyayı kaldırır.

iOS, uygulama askıya alınmışken aktarımları sürdüren bir sistem arka plan URLSession'ı kullanır. Uygulama açılışta oturumuna yeniden bağlanır ve arka plan tamamlanmasını onaylamadan önce dosyaları yayımlar. macOS aktarımları süreç içinde çalışır. Apple platformları duraklatılmış indirmeler için yerel sürdürme verilerini kullanır ve son bir sonuç kaydedilene kadar istek meta verilerini uygulamanın özel depolamasında tutar. Sürdüremeyen bir sunucu aktarımı yeniden başlatır.

Etkin görevleri ve kaydedilmiş sonuçları görmek için uygulama başladığında `OnDownloadUpdate`'i yeniden kaydedin. Son bir sonucu işledikten sonra `p.Task.Forget()` kullanın. Android hizmeti durdurabilir; örneğin ön plan aktarım süresi dolduğunda. Uygulama o zaman `DownloadInterrupted` alır. Windows, Linux, tvOS, watchOS ve web'de `Download`, `ErrDownloadUnavailable` döndürür.

## Konuşma ve yaşam döngüsü

`cx.StartSpeech`, `cx.Env().CanRecognizeSpeech()`'in var olduğunu söylediği yerde (macOS, iOS, Android, Windows ve `SpeechRecognition` veya `webkitSpeechRecognition` sunan tarayıcılar) konuşmayı sistemin tanıyıcısıyla tanır ve döküm değiştikçe onunla yanıt verir. Apple platformlarında `apple.info` içinde `NSMicrophoneUsageDescription` ve `NSSpeechRecognitionUsageDescription` bildirin; Android'de `android.permissions`'a `RECORD_AUDIO` ekleyin. Başsız bir testte, tanıma etkinken `app.Speech` bir `nsgotest.SpeechSession` tutar. Sonuçları iletmek için `app.HearSpeech` ve `app.EndSpeech` kullanın.

```go
session, err := cx.StartSpeech(core.SpeechOptions{
	Lang:    "en-US",
	Interim: true,
	OnResult: func(text string, final bool) {
		transcript.Set(text)
	},
})
if err == nil {
	defer session.Stop()
}
```

`cx.Env().Lifecycle()` uygulamanın etkin, etkin olmayan veya arka planda olduğunu söyler: masaüstlerinde `cx`'in içinde bulunduğu pencere için (böylece başka bir pencere görünürken simge durumuna küçültülmüş bir pencerenin içeriği duraklayabilir), uygulamanın kendi kapsamında ise tüm pencereleri birlikte. `desktop.Minimized(cx)` o pencerenin simge durumuna küçültülüp küçültülmediğini söyler; `desktop.FrontmostApp(cx)` ise bir masaüstü evcil hayvanının veya bir zaman izleyicinin izlediği gibi, masaüstünde hangi uygulamanın önde olduğunu (uygulamanın kendisi de dahil) ve penceresinin tam ekranda olup olmadığını söyler; `desktop.CanReadFrontmostApp(cx)` bunun izlenip izlenmediğini söyler: macOS, Windows, X11 ve desteklenen Wayland bileşimcilerinde. `cx.OnQuit`, uygulama nasıl kapanırsa kapansın kapanırken temizlik yapar ve işi için 200 ms'si vardır. `desktop.OnQuitRequest` bir masaüstü Çıkış menü öğesini veya `desktop.RequestQuit(cx)`'i iptal edebilir ya da erteleyebilir. Windows ve Linux'ta, bir pencereyi kapatmak uygulamayı sonlandıracaksa `OnClosing`'den sonra da çalışır. `desktop.RequestRestart(cx)` yeniden başlatmadan önce aynı işleyicilere sorar; `desktop.Restart(cx)` ise anında olmaya devam eder. Linux ve Windows'taki güncelleyici yeniden başlatmaları da sorar; böylece kaydetme onları iptal edebilir veya erteleyebilir. Sistem oturum kapatma, yeniden başlatma veya kapatma bildirdiğinde `q.Forced()` true olur ve uygulama `OnQuit` üzerinden devam eder. Kullanıcıdan bir karar istemeden önce `Forced()`'ı denetleyin. `cx.SetAppearance` açık veya koyu görünümü zorlar, `cx.SetLocale` dili seçer, `cx.OnKey` bir klavye kısayolu bağlar. Bu kısayol, sahibinin gösterdiği şey gizliyken veya bir iletişim kutusunun altındayken, `core.KeyOptions{OverModals: true}` onu korumadıkça dinlenir; tuşunu uygulama menüsünden önce alır, meğer ki `YieldsToMenu` onu odağın bulunduğu yerde aynı kısayolu alan etkin bir menü öğesine bıraksın (bir metin alanı veya web görünümü odaktayken bir `MenuNotEditing` öğesi tuşu bağlamaya bırakır ve aynı kısayola sahip sonraki öğelerden de esirger; Linux'un menüsü ise Control, Alt veya bir logo tuşu olmadan basılan hiçbir tuşu almaz, bu nedenle bağlama orada böyle bir tuşu korur); `AllSurfaces` uygulama çapında bir kısayol için onu uygulamanın her penceresinde ve panelinde (sonradan açılanlar da dahil) bağlamadıkça `cx`'in içinde bulunduğu pencerede çalışır; bu durumda sahibinin gösterdiği şey bağlandığı pencerede gizliyken hepsinde dinlenir; ve `core.KeyAlways` veya `core.KeyNotEditing` ile bağlanmadıkça odaklanmış bir denetime kullandığı tuşları bırakır: bir düğmeye Return ve Space tuşlarını, bir kaydırıcıya, sekme çubuğuna veya radyo grubuna ok tuşlarını. `cx.OnOpenFiles` macOS, Windows, Linux ve iOS'ta açılan belgeleri duyar. iOS normal dosyaları uygulamaya ait geçici depolamaya aktarır; onları saklamak için `cx.DataDir()`'e taşıyın. macOS'ta uygulama menüsünü sağlayan pencereyi kapatmak, hayatta kalan en son pencerenin menüsünü ya da hiçbiri kalmadığında varsayılanı geri yükler.

Wayland'da en öndeki uygulamayı izlemek, bileşimcinin wlr foreign-toplevel protokolünün 2. veya sonraki sürümünü gerektirir; diğer bileşimciler `CanReadFrontmostApp` için false bildirir. Kimliği Wayland uygulama kimliğidir, adı varsa masaüstü girdisinden gelir ve PID'si 0'dır. Paketlenmiş Windows uygulamaları bir çerçeve ana makinesi yolu yerine AUMID'lerini kullanır. macOS, tam ekranı pencere boyutundan çıkarmak yerine AppKit'in tam ekran sunum durumunu izler.

Uygulama menüsündeki bir düzenleme rolü, `OnSelect` yedeğinden önce odaklanmış düzenleyiciyi alır. macOS'ta Geri Al ve Yinele o düzenleyicinin geçmişini, Kes ise seçimini izler; uygulama etkin değilken de dahil. Düzenleyici yoksa yedek etkin kalır.

iOS'ta, bağlama tetiklenebilirken kısayolu sistemin klavye menüsünde sunmak için `cx.OnKeyWith`'e bir `core.KeyOptions.Title` işlevi verin. `Group`, iPadOS 26 ve sonrasında menüsünü adlandırır. İşlevler uygulamanın kataloğunu okuyabilir; böylece kısayol başlıkları dili izler. Geçerli listeyi incelemek için testlerde `app.KeyCommands()` kullanın.

```go
cx.OnKey("mod+k", func() { palette.Set(true) })
cx.SetLocale("de")
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })
```

`cx.Env().DeviceName()` kullanıcının cihaza verdiği addır, `cx.Env().DeviceModel()` ise `Mac15,7`, `iPhone15,2` veya `Google Pixel 8` gibi donanım modelidir; böylece bir uygulama eşleştirilmiş iki cihazı birbirinden ayırt edebilir. macOS, Android, Windows ve Linux her ikisini de verir; iOS modeli verir, adı ise yalnızca Apple'ın kullanıcı tarafından atanan cihaz adı yetkisine sahip bir uygulamaya verir. watchOS donanım tanımlayıcısını, HarmonyOS pazar adını veya ürün modelini, mini programlar ise platformun markasını ve modelini verir; bu hedefler kullanıcının atadığı adı boş bırakır. Web ikisini de vermez. Testler bunları `nsgotest.Environment` ile ayarlar.

`cx.Env().ThermalState()`, Apple platformlarında ve Android API 29 veya sonrasında `core.ThermalNominal`, `ThermalFair`, `ThermalSerious` veya `ThermalCritical` bildirir. `cx.Env().LowPowerMode()`, Apple platformlarında, Android'de, Windows'ta ve power-profiles-daemon bulunan Linux oturumlarında `core.LowPowerOn` veya `LowPowerOff` bildirir. Sistemin değer vermediği yerde okumalar `ThermalUnknown` ve `LowPowerUnknown` döndürür. Her okuma değişiklikleri bağımsız olarak izler; böylece bir uygulama çalışırken iş yükünü ayarlayabilir. Testlerde `ThermalState` ve `LowPowerMode`'u `core.Environment` üzerinde `nsgotest.Environment` veya `app.SetEnvironment` aracılığıyla ayarlayın.

`desktop.CursorPoint(cx, func(p core.Point, available bool) { ... })` macOS, Windows ve X11'de işaretçiyi uygulamanın pencerelerinin dışında bile okur. Her çağrıda bir örnek alır. Nokta, `desktop.Monitors(cx)` ve `Window.ShowAt` ile aynı ekran koordinatlarındadır. Wayland'da ve diğer desteklenmeyen hedeflerde ya da kapsamda `core.AccessCursorPoint` yoksa `available` false olur.

`desktop.SetWindowConstraints` içerik boyutunu sınırlar ve yeniden boyutlandırmayı, simge durumuna küçültmeyi ve ekranı kaplamayı denetler; bir pencere açarken `WindowOptions.Constraints`'i ayarlayın veya ana pencere için `nsgo.WindowConstraints` kullanın. Linux, X11'de sabit olmayan en büyük değerleri uygular ve yasaklanan küçültme/ekranı kaplama düğmelerini X11 ve Wayland'da GTK'nın yerel başlık çubuğundan kaldırır. Wayland şu anda üst sınır için yalnızca sabit boyutları destekler.

`desktop.SetWindowLevel` bir pencereyi diğerlerinin üzerinde yüzdürür; `desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll)` veya `WindowOptions.Workspaces` ise onu bir masaüstü evcil hayvanı, bir HUD veya bir zamanlayıcı gibi her çalışma alanında tutar: macOS'ta her Space'te ve diğer uygulamaların tam ekran alanlarının üzerinde, Cmd-` dışında; Linux'ta ise bir X11 pencere yöneticisinin her sanal masaüstünde. Windows ve Wayland onu açıldığı alanda bırakır; `desktop.CanSetWindowWorkspaces` etkili olup olmadığını söyler: macOS'ta ve `_NET_WM_DESKTOP` destekleyen bir X11 pencere yöneticisi altında true. Bir ana pencere, içeriğinden `desktop.SetWindowWorkspaces` çağrılarak her çalışma alanına yerleşir. Testlerde `nsgotest.App.Window` istekleri listeler ve `nsgotest.Environment` `CanSetWindowWorkspaces`'i ayarlar.

`desktop.RegisterHotKey`, macOS'ta, Linux'ta ve `nsgo.Run(App)` ile çalıştırılan bir Windows uygulamasında, herhangi bir uygulama en öndeyken çalışan genel bir kısayol bağlar. Tanıtıcısının `State` değeri sistemin onu alıp almadığını söyler: sistem yanıt verene kadar `HotKeyPending`, ardından `HotKeyRegistered`, başka bir kayıt bu bileşimi tutuyorsa `HotKeyTaken` veya genel kısayolları olmayan bir tuş ya da platform için `HotKeyUnsupported`; böylece bir kısayol kaydedici başka bir bileşim isteyebilir. macOS'ta bir bileşim yalnızca uygulama onu zaten tutuyorsa veya başka bir uygulama onu özel olarak kaydetmişse alınmış sayılır. Wayland'da bileşimci kullanıcıdan kısayolu bağlamasını ister: kullanıcı bağladığında kaydedilir, kullanıcı veya bileşimci onu dışarıda bıraktığında alınmış olur. Testlerde `nsgotest.HotKeyTaken` ve `nsgotest.HotKeyUnsupported` bileşimlerin böyle yanıt vermesini sağlar.

`desktop.TitleBarHidden` ile Windows ve Linux, tek başına basılıp bırakılan Alt veya F10 ile uygulama menüsünü bir açılır pencere olarak sunar. Alt ile birlikte üst düzey bir etiketin ilk harfi veya rakamı, etiketin yazı sisteminde, o alt menüyü açar veya o komutu seçer. Uygulama bağlamaları tuşlarını korur; Shift+F10 bağlam menüsü tuşu olarak kalır ve diğer Alt bileşimleri sistemde kalır. Escape açılır pencereyi kapatır ve klavye odağını geri yükler.

```go
shortcut := desktop.RegisterHotKey(cx, "alt+space", launcher.Toggle)
warning := core.Text{ContentFunc: func() string {
	if shortcut.State() == desktop.HotKeyTaken {
		return "Another app uses Alt+Space"
	}
	return ""
}}
```

macOS, Windows ve Linux'ta masaüstü yeniden boyutlandırmaları çizmeden önce yeni düzen için 16 ms'ye kadar bekler. Düzenin bu süreye yetişebilmesi için tepkisel hesaplamaları ve olay işleyicilerini hızlı tutun; yavaş G/Ç işlerini bir goroutine'de yapın ve sonuçları `cx.Post` aracılığıyla gönderin. Meşgul bir uygulama yeniden boyutlandırmaya devam eder ve yetiştiğinde düzenini gösterir.

`desktop.SetWindowFrame`, `WindowFrame.Move` true olduğunda bir pencereyi, ana ekranın sol üst köşesinden nokta cinsinden `X` ve `Y` kullanarak taşır. Ayarlanmamış boyutlar mevcut değerlerini korur. `desktop.CanPlaceWindows`'u denetleyin: macOS, Windows ve X11'deki Linux konumlandırmayı destekler; sıradan bir Wayland üst düzey penceresi bileşimcinin konumunu korur. `desktop.Frame` bu masaüstlerinde yerel pencerenin konumunu izler.

macOS'ta ve Windows'ta `WindowFrame{Animated: true}`, görünür bir paneli, yerleşik (docked) bir panelin boyutu da dahil olmak üzere, yeni çerçevesine yumuşakça geçirir. Gizli paneller ve azaltılmış hareket yeni çerçeveyi hemen uygular. Sıradan macOS pencereleri AppKit'in animasyonunu kullanır.

Üst pencere kapandığında her ikisini de kapatmak için `desktop.WindowOptions.Parent`'ı uygulamanın açık bir penceresine ayarlayın. Masaüstlerinde alt pencere üst penceresinin üzerinde kalır ve üst pencere gizlendiğinde veya simge durumuna küçültüldüğünde onu izler; açıkça gizlenmiş bir alt pencere, üst pencere geri döndüğünde gizli kalır. macOS'ta ayrıca `Frame` ayarlanmamışsa üst penceresinin üzerinde ortalanır ve onu tam ekrana izler.

`WindowOptions.NoShadow` ve `desktop.SetWindowShadow(cx, false)`, başlık çubuğu gizli olan ve kendi kenar süslemesini çizen bir Windows penceresinin gölgesini de kaldırır; sistemin başlık çubuğuna sahip bir pencere çerçevesini ve gölgesini korur; pencere yöneticisinin süslediği bir Linux penceresi de öyle; orada `NoShadow` yalnızca GTK'nın kendi süslemelerinin gölgesini temizler.

`desktop.SetWindowPointerEvents`, işaretçinin bir pencereden geçip altındaki, herhangi bir uygulamaya ait pencerelere ulaşmasını sağlar: bir ekran üstü göstergenin (heads-up display) yaptığı gibi her yerde `WindowPointerNone`; kendi üzerindeki basmaları, sürüklemeleri ve sağ tıklamaları yine de alan bir masaüstü evcil hayvanının çevresinde olduğu gibi, hiçbir görünümün bir şey çizmediği veya işaretçiyi duymadığı her yerde `WindowPointerPainted`; sistemin çizdiği başlık çubuğu hiçbir şey çizilmemiş sayılır. `WindowOptions.PointerEvents` ve `PanelOptions.PointerEvents` bir pencereyi bu şekilde açar. Bir `Desktop.WindowDrag` bölgesi, basılabilir bir üst öğenin içinde de sürüklemeye devam eder; daha yakın basılabilir veya düzenleme denetimi pencereyi sürüklemek yerine kendi girdisini korur. İşaretçi almayan bir pencere hiçbir basmayı duymaz; bu nedenle uygulama onu bir kısayoldan, bir tepsi simgesinden veya başka bir pencereden geri verir. Bir `Access`'in tuttuğu kod `core.AccessOverlay` gerektirir.

```go
desktop.OpenPanel(cx, desktop.PanelOptions{Width: 600, Height: 400, PointerEvents: desktop.WindowPointerPainted}, Pet)
```

macOS'ta ve Windows'ta, boyanmış bir penceredeki bir görüntü `Image.HitTestAlpha`'yı (TSX `hitTestAlpha`) 0'ın üzerinde ve en fazla 1 olan bir değere ayarlayabilir. Örneğin 0.5, en az yarı opak olan resim piksellerinde işaretçiyi alır. Eşik görüntünün `OnPress`'ine de uygulanır. Bir görünüm arka planı veya kenarlığı yine boyanmış sayılır. Sıfır, Linux'taki görüntülerde olduğu gibi tüm görüntü görünümünü kullanır.

Boyanmış bir macOS penceresinde `Video.HitTestAlpha` (TSX `hitTestAlpha`) aynı eşiği çözülmüş video karelerine uygular. `Controls` kapalıyken ve dönüştürülmemiş bir video izinde çalışır; örneğin bir masaüstü evcil hayvanı için alfa kanallı HEVC. Sıfır, kullanılamayan bir kare ve diğer masaüstü ana makineleri video görünümünün sınırlarını kullanır.

`desktop.NewTray`, macOS'ta ve Windows'ta `TrayOptions.View` ile canlı bir simge çizebilir. Windows görünümün saydam oluşturmasını bildirim alanının simge boyutuna sığdırır ve görünüm değiştiğinde veya bir görüntünün yüklenmesi bittiğinde onu günceller; eylemleri için `TrayOptions.OnClick`, `Panel` ve `Menu` kullanın. Linux şu anda PNG simgeleri gösterir; tepsi görünümünü çizmek hâlâ bir hedeftir.

`desktop.OpenPanel`, böyle bir kısayolun veya tepsi simgesinin gösterdiği bir paneli açar: diğerlerinin üzerinde kenarlıksız bir pencere. `desktop.OpenWindow`, `desktop.NewTray`, `mobile.OpenOverlay`, `audio.NewPlayer`, `audio.LoadSound` ve `core.NewHolder` gibi, çalışan işleyicinin sahibine değil doğrudan verdiğiniz `cx`'e aittir; bu nedenle bir düğmenin `OnPress`'inden uygulamanın kapsamını vermek, o düğmenin kapsamı gittikten sonra pencereyi, paneli veya oynatıcıyı canlı tutar; tepkisel temel öğeler (`Effect`, `Cleanup`, `AfterFunc`, `OnKey`) ise `cx.Run`'ın değiştirdiği çalışan sahibe aittir. Bir açılır kart kendini `Transparent` ile bir panel üzerinde çizer; bu panelin kendine ait arka planı, çerçevesi veya gölgesi yoktur, bu nedenle yalnızca kartın yuvarlatılmış köşeleri ve `BoxShadow`'u görünür ve gölge için panelin içinde yer bırakılır; `FitContent` paneli ilk gösterilmeden önce ve içerik değiştikçe `Width` ve `Height`'e kadar içeriğine göre boyutlandırır. `desktop.Transparent`, panelin şu anda kendine ait hiçbir şey çizmediğini (saydam bir pencerede olduğu gibi) söyler; `desktop.CanMakeWindowsTransparent` ise yalnızca bir pencerenin saydam olup olamayacağını söyler: false olduğu yerde, örneğin bileşimcisi olmayan Linux'ta, içeriğe bir arka plan verin.

macOS'ta `Transparent` olmayan bir panel, boyutlarından biri 24 noktanın altındayken kare köşeler kullanır ve normal boyutlarda yuvarlatılmış köşelerini geri yükler.

```go
lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, Transparent: true, FitContent: true, HideOnBlur: true}, Card)
desktop.RegisterHotKey(cx, "alt+space", lookup.Toggle)
```

`desktop.NewTray`, macOS'ta `TrayOptions.Title`'ı görüntüsünün veya çizilen görünümünün yanında gösterir. Özel bir açılır pencere için `OnClick` veya `Menu` içinde `tray.ClickBounds()`'u okuyun ve kullanılabilir dikdörtgeni `panel.ShowAt(bounds, core.PlaceBottom)`'a verin. `Env.Displays` ile aynı ekran noktalarını kullanır ve başlığı içerir. Sınırlar ilk tıklamadan önce, kaldırıldıktan sonra, Linux'ta ve Windows kabuğu bunları sağlayamadığında kullanılamaz. Her tıklama önceki sınırların yerini alır; böylece kullanılamayan bir sonuç hiçbir zaman eski bir dikdörtgeni yeniden kullanmaz. `TrayOptions.Panel`'i ayarlamak bunun yerine yerleşik tıkla-aç/kapa davranışını korur.

`PanelOptions.Dock`, bir paneli `Show`, `Focus`, `Toggle` ve içerik yeniden boyutlandırması boyunca bir ekranın kullanılabilir kenarı boyunca ortalanmış tutar. `Edge`'i `core.PlaceTop`, `PlaceBottom`, `PlaceLeft` veya `PlaceRight` olarak, `Display`'i bir `Env.Displays` kimliğine (birincil ekran için boş) ve `Margin`'i o kenardan nokta cinsinden uzaklığa ayarlayın. Yerleşik bir panel `ShowAt` veya `MoveTo` kullanamaz; `SetWindowFrame` yalnızca boyutunu değiştirir. Bir tepsi simgesi onu simgenin altında değil, yerleştiği yerde gösterir.

```go
panel := desktop.OpenPanel(cx, desktop.PanelOptions{
	Width: 320, Height: 80, NoActivate: true,
	Dock: desktop.PanelDock{Edge: core.PlaceBottom, Margin: 12},
}, Candidates)
panel.OnPlacementError(cx, func(err error) { /* offer another presentation */ })
panel.Show()
```

`desktop.CanDockPanels(cx)` yerleştirme desteğini rastgele pencere konumlandırmadan ayrı olarak bildirir. macOS, Windows ve X11 bunu destekler; Wayland ise isteğe bağlı çalışma zamanı kitaplığı `libgtk4-layer-shell.so.0`'ı ve `zwlr_layer_shell_v1`'i uygulayan bir bileşimciyi gerektirir. NSGo bu kitaplığı GTK'dan önce yükler; Wayland'ı zaten yüklemiş olan bir gömme ana makinesi, ana makineyi başlatmadan önce gtk4-layer-shell'i önceden yüklemelidir. Eksik bir kitaplık, protokol veya seçili ekran paneli gizli bırakır ve `OnPlacementError`'ı çağırır. Bir Wayland yerleşimi, çalışma alanı ayırmadan üst katmanı kullanır: `NoActivate` klavyeyi geçerli pencerede bırakır, onsuz bir panel ise gizlenene kadar özel klavye odağını alır. Sıradan Wayland pencereleri bileşimcinin olağan konumlandırmasını korur.

Bir kısayol kaydedici, kullanıcının böyle bir tuşu seçmesini sağlar. `Focusable` bir görünümün `OnKeyDown`'ı, odak kendisindeyken basılan her tuşu, uygulamanın bağlamalarından önce, basılı tutulan değiştiricilerle birlikte duyar; tek başına basılan bir değiştirici de dahil. `true` döndürmek tuşu bağlamalardan esirger; böylece Command+S'yi kaydetmek kaydetme yapmaz. `KeyEvent.Shortcut` basışı `OnKey` ve `RegisterHotKey`'in aldığı biçimde yazar; her değiştiriciyi adıyla ("ctrl+shift+k", "alt+space", Command+Shift+K için "shift+meta+k") ve tek başına bir değiştirici için "" olarak. `OnKeyUp`'ı ise bu tuşların her birinin bırakılışını, `OnKeyDown` onu tüketmiş veya odak taşınmış olsa bile, hâlâ basılı tutulan değiştiricilerle birlikte duyar. Her ikisi de macOS, Windows, Linux, web (tarayıcının hiçbir sayfaya vermediği kısayolları kendine sakladığı yer), Apple TV ve `nsgotest` üzerinde çalışır; Android donanım klavyesi olaylarını odaklanmış görünümlere eşzamansız olarak iletir, bu nedenle onların `OnKeyDown` dönüş değeri tuşu tüketemez. iPhone ve iPad donanım tuşlarını yoklamayı destekler. Uygulamanın genel olarak kaydettiği bir kısayol tuşunu görünümden önce alır; bu nedenle bir kaydedici kayıt yaparken onun kaydını kaldırır.

```go
core.View{
	Focusable: func() bool { return true },
	OnKeyDown: func(e core.KeyEvent) bool {
		if s := e.Shortcut(); s != "" {
			shortcut.Set(s)
		}
		return true
	},
	OnKeyUp: func(core.KeyEvent) { recording.Set(false) },
	Children: []core.Node{core.Text{ContentFunc: shortcut.Get}},
}
```

Bir oyun veya bir tuval ise tuşları yoklar. `core.KeysDown(cx)` pencerede basılı tutulan tuşları listeler ve `core.KeyHeld(cx, "left")` birinin basılı olup olmadığını söyler: bunlar bir `RequestAnimationFrame` döngüsünün de okuduğu izlenen okumalardır. İlk okuma, platformun o andan itibaren her tuşu bildirmesini sağlar; bu nedenle zaten basılı tutulan bir tuş yeniden basılana kadar bilinmez kalır; bir alana veya web görünümüne ya da bir giriş yöntemi oluştururken yazılan tuşlar hiçbir zaman basılı sayılmaz ve pencere odağı kaybettiğinde hâlâ basılı olan bir tuş o anda bırakılmış olur. `OnKeyUp`'ın çalıştığı yerlerde ve donanım klavyeli iPhone ile iPad'de çalışırlar.

Fare düğmeleri için `View.OnMouseDown` ve `View.OnMouseUp`, macOS, Windows, Linux, web ve `nsgotest` üzerinde her düğme (`core.MouseButtonLeft`, `core.MouseButtonRight`, `core.MouseButtonMiddle`, `core.MouseButtonBack`, `core.MouseButtonForward`) için görünümü bir klavye odak durağı yapmadan bir `core.MouseButtonEvent` iletir. `core.MouseButtonsDown(cx)` ve `core.MouseButtonHeld(cx, core.MouseButtonRight)`, pencerede basılı tutulan düğmelerin izlenen okumalarıdır ve pencere odağı kaybettiğinde otomatik olarak temizlenir.

Fiziksel bir konuma bağlı bir denetim için `KeyEvent.Code`, `core.KeyCodesDown(cx)` veya `core.KeyCodeHeld(cx, "KeyW")` okuyun. Kod, `KeyboardEvent.code` adlarını kullanır: AZERTY'de QWERTY'nin W konumu `Code: "KeyW"` ve `Key: "z"` değerlerine sahiptir. `core.KeyCodeLabel(cx, "KeyW")` bir istem için etkin düzenin görüntü etiketini verir. iOS, Android, HarmonyOS ve düzen haritası olmayan tarayıcılarda o konumda bildirilen son tuş adını kullanır ve bir tuş görülene kadar boş bir dize döndürür. HarmonyOS, ArkUI hiçbir tarama kodu bildirmediğinden hem konumu hem de tuşu InputKit'in `KeyCode`'undan adlandırır; bu nedenle orada tuş adı henüz QWERTY olmayan bir düzeni izlemez.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if core.KeyHeld(cx, "left") {
		x.Update(func(x float32) float32 { return x - 4 })
	}
	if core.KeyHeld(cx, "right") {
		x.Update(func(x float32) float32 { return x + 4 })
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

## Ekranı açık tutma

`cx.KeepAwake("Playing video", core.KeepAwakeOptions{Display: true})`, döndürülen serbest bırakma işlevi çalışana veya geçerli kapsam atılana kadar ekranı açık tutar. Android'de bu tutma uygulama görünürken geçerlidir. Web'de Screen Wake Lock desteğine sahip güvenli bir bağlamda çalışır ve sayfa görünür hale geldiğinde kilidi yeniden ister. Safari ilk isteği kabul etmeden önce bir basma veya tuş gerektirebilir. Yalnızca bilgisayara yönelik tutmaların bu iki platformda etkisi yoktur.

## İşaretçi kilidi

Bir oyun kamerasında veya ekran kenarında devam etmesi gereken bir sürüklemede, ref'i tutan görünüm üzerindeki bir basmadan `ref.LockPointer()`'ı çağırın. Görünüm `OnPointerDelta(dx, dy)` içinde göreli hareket alırken işaretçi yerinde kalır ve gizlenir. Pozitif değerler sağa ve aşağıya hareket eder.

Kilidi serbest bırakmak için döndürülen işlevi çağırın. Escape, pencerenin odağı kaybetmesi ya da görünümün gizlenmesi veya kaldırılması da onu serbest bırakır. Yüzeyin bunu destekleyip desteklemediğini görmek için `cx.Env().CanLockPointer()`'ı denetleyin. Web, basma gibi bir kullanıcı hareketi gerektirir. Wayland'daki Linux, işaretçi kısıtlamalarını ve göreli hareketi destekleyen bir bileşimci gerektirir.

## Ses sınırları

Motorun sınırlarını değiştirmek için sesleri yüklemeden önce `audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: 200, MaxDuration: 20 * time.Second})` kullanın. `MaxEncodedBytes`, `MaxDecodedBytes` ve `MaxPlaybacks` de ayarlanabilir. Sıfır alanlar varsayılanları kullanır: ses başına 8 MiB kodlanmış ve 5 saniye, 64 kaynak, tüm seslerde toplam 16 MiB çözülmüş ve çıkış başına 32 örtüşen oynatma. Mevcut sesler yüklü kalır ve yeni sınırlara dahil edilir. Kısıtlı kapsamlar varsayılan sınırları motorun bütçesi içinde tutar: imzalı bir TSX paketinin `nsgo/audio` sesleri, Go uygulaması ne ayarlamış olursa olsun bunlara takılır; uygulamanın kendi TSX'i ise Go uygulamasının sınırlarını alır. Android'in SoundPool'u, sınırlar ne olursa olsun aynı anda en fazla 32 ses çalar ve her birinin en fazla 1 MiB'ını çözer.

## Tam ekran

`desktop.SetFullScreen(cx, true, displayID)`, macOS, Windows ve Linux'ta `cx.Env().Displays()` tarafından adlandırılan ekranı doldurur. Pencerenin geçerli ekranını kullanmak için kimliği atlayın. Bilinmeyen bir kimlik pencereyi olduğu yerde bırakır. Aynı isteği yinelemenin hiçbir etkisi yoktur. `desktop.SetFullScreen(cx, false)` tam ekrandan çıkar ve pencerenin önceki çerçevesini geri yükler. Web'de bir basma işleyicisinden girin. Ekranı tarayıcı seçer ve `cx.Env().FullScreen()` Esc dahil giriş ve çıkışı izler.

## Ekranlar

`cx.Env().Display()` pencerenin geçerli `core.DisplayInfo` değerini döndürür ve pencere başka bir ekrana taşındığında bağlamaları günceller. `cx.Env().Displays()` platformun bildirdiği ekranları listeler. `RefreshRate` ve `MaxRefreshRate` Hz cinsindendir; kullanılamayan bir hız için sıfırdır. iOS panelin en yüksek değerini bildirir. HarmonyOS pencerenin ekranını, geçerli hızını ve desteklenen en yüksek hızını bildirir. Web geçerli ekranı bildirir ve kare geri çağırma ölçümünü `RefreshRateEstimated` ile işaretler. Testlerde `Displays` ve `DisplayID`'yi `nsgotest.Environment` aracılığıyla ayarlayın.

## Metin seslendirme

`cx.Speak`, `cx.Env().CanSpeak()`'in platformun konuştuğunu söylediği yerde bir dil için metni sistemin sesiyle yüksek sesle okur: macOS, iOS, tvOS, Android, Windows, web ve speech-dispatcher bulunan Linux. Aynı anda tek bir söyleyiş konuşur: başka bir `Speak` konuşmakta olanı keser; `cx.StopSpeaking` ve onu söyleyen kapsamın sonu da keser. `done` nasıl bittiğini bir kez duyar: bitti, durduruldu veya bir `core.SpeakError` ile başarısız oldu; örneğin yüklü hiçbir ses dili konuşmadığında `core.SpeakLanguageUnavailable`.

```go
cx.Speak("Guten Morgen", core.Voice{Lang: "de-DE", Rate: 0.8}, func(stopped bool, err error) {
	speaking.Set(false)
})
```

`Rate` ve `Pitch` 0.5'ten 2'ye kadar gider; 1 sesin kendi değeridir. `cx.Voices("ja", ...)` bir dil için yüklü sesleri listeler; böylece uygulama kullanıcıdan eksik olan bir sesi yüklemesini isteyebilir veya `ID`'si `core.Voice` içine giden bir sesi seçmesine izin verebilir. Tarayıcılar yalnızca kullanıcı sayfada bir şeye bastıktan sonra konuşur ve öncesinde `core.SpeakNotAllowed` ile başarısız olur. Bir TSX paketinin Go bileşenleri yalnızca `text_to_speech` yeteneğiyle konuşur.

## Döngüdeki ses

Bir ses oynatmasını `Stop` veya `Close`'a kadar yinelemek için `audio.Voice.Loop`'u ayarlayın. Akışla gelen müzik için `audio.PlayerOptions.LoopStart`'ı ayarlayın ve `SetLoop(true)`'yu çağırın. İlk geçiş girişi içerir. Sonraki geçişler ofsette başlar. Sıfır bir ofset tüm parçayı yineler.

macOS 14 ve iOS 17 veya sonrasında sonlu bir parça, geçerli yineleme bitmeden her yinelemeyi hazırlamak için yerel kuyruğu kullanır. Daha eski Apple sürümleri, geçerli geçiş bittiğinde kısa bir boşlukla döngü başlangıcına sarar. Linux'ta ve web'de döngüdeki bir oynatıcı, 32 MiB ve 5 dakikaya kadar olan bir parçayı belleğe çözer (dakikada yaklaşık 23 MB) ve boşluksuz yineler. Linux'ta daha büyük veya daha uzun bir parça kendi çözücüsü üzerinden akar ve o da boşluksuz yinelenir; web'de ise sonunda kısa bir boşlukla geri sarar. Windows'ta ve Android'de başlangıcı olan bir döngü, sonunda kısa bir boşlukla ona geri sarar. Web'de 1 dışında bir hızdaki döngüdeki bir oynatıcı da sonunda geri sarar, çünkü tarayıcının boşluksuz oynatması perdeyi hızla birlikte kaydırırdı. Web'de döngüdeki bir oynatıcı, bir ses gibi, yalnızca kullanıcı sayfada bir şeye bastıktan sonra çalar ve öncesinde `NotAllowed` ile başarısız olur.

## Biyometri

`cx.BiometricAvailability`, Face ID, Touch ID veya bir cihaz parmak izinin kayıtlı olup olmadığını bildirir; Android hangi türün kayıtlı olduğunu söylemez, bu nedenle orada kayıtlı bir parmak izi varsa parmak izi, yoksa cihazda yüz donanımı varsa yüz bildirir. `cx.AuthenticateBiometric` sistemden kullanıcıyı boş olmaması gereken bir gerekçeyle doğrulamasını ister ve başarı, başarısızlık, iptal veya kullanılamazlık bildirir. Uygulama için bir parola yedeği seçmez.

```go
cx.BiometricAvailability(func(kind core.BiometricKind) {
	if kind != core.BiometricNone {
		cx.AuthenticateBiometric("Confirm deleting this account", func(result core.BiometricResult) {
			if result == core.BiometricSuccess {
				deleteAccount()
			}
		})
	}
})
```

Face ID uygulamaları `apple.info` altında `NSFaceIDUsageDescription` bildirir; Android uygulamaları `android.permissions` altında `USE_BIOMETRIC` listeler.

## Oturum açma

`cx.AuthSession`, kullanıcının oturumunu bir web görünümü yerine sistemin kimlik doğrulama oturumunda bir OAuth sağlayıcısıyla açar: macOS, iOS ve watchOS'ta `ASWebAuthenticationSession`, Android'de bir Custom Tab veya tarayıcı, Windows'ta, Linux'ta ve şeması olmayan bir Mac'te varsayılan tarayıcı ve web'de bir açılır pencere. Web'de yönlendirme sayfası geri çağırmasını aynı kaynaklı bir `BroadcastChannel` üzerinden de gönderir; böylece sağlayıcının `Cross-Origin-Opener-Policy`'si açılır pencereyi sayfadan kopardığında da oturum açma tamamlanır. Böyle bir açılır pencere sayfaya kapalı görünür; bu nedenle sağlayıcının sayfasını gösterdikten sonra bir açılır pencereyi kapatmak oturumu ancak 5 dakika sonra iptal edilmiş olarak sonlandırır; oturuma bir iptal düğmesi verin. Protokol uygulamada kalır: uygulama PKCE doğrulayıcısını ve state değerini oluşturur, kendisine verilen yönlendirme için yetkilendirme URL'sini kurar ve kodunu takas etmeden önce geri çağırmayı denetler. İptal etmek başarısız olmak değildir: `errors.Is(err, core.AuthCancelled)` kullanıcının oturumu kapattığını veya döndürülen `cancel`'ın onu sonlandırdığını söyler.

```go
verifier, challenge, state := newPKCE() // the app's own
cx.AuthSession(func(redirect string) string {
	q := url.Values{"client_id": {clientID(redirect)}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email"},
		"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	return "https://accounts.example.com/authorize?" + q.Encode()
}, core.AuthSessionOptions{Scheme: "com.example.app", Path: "/oauth2redirect", Timeout: 5 * time.Minute}, func(callback string, err error) {
	if errors.Is(err, core.AuthCancelled) {
		return
	}
	if err != nil {
		showError(err)
		return
	}
	u, _ := url.Parse(callback)
	if u.Query().Get("state") == state {
		exchangeCode(u.Query().Get("code"), verifier)
	}
})
```

Yönlendirme, bir şema ayarlandığında `com.example.app:/oauth2redirect`'tir (Windows, Linux ve Android, işletim sisteminin geri çağırmayı çalışan uygulamaya vermesi için şemayı `nsgo.yaml`'ın `url_schemes` alanında gerektirir; macOS'un, iOS'un ve watchOS'un sistem oturumu bu bildirim olmadan onu yakalar), masaüstlerinin geri döngüsünde (loopback) ise `http://127.0.0.1:<port>/oauth2redirect`'tir; bu nedenle her ikisini de sağlayıcıya, çoğu zaman kendilerine ait istemciler olarak kaydedin. Yalnızca geri döngü için `Scheme`'i boş bırakın ya da iOS 17.4, macOS 14.4 ve watchOS 10.4'ün yakaladığı uygulamanın ilişkili alan adı için onu `Host` ile birlikte `https` olarak ayarlayın. watchOS bir şema veya ilişkili HTTPS alan adı gerektirir; geri döngü yedeği yoktur ve NSGo'nun mevcut watchOS 26 alt sınırı yine geçerlidir. Kapatılmış bir tarayıcı sekmesi bir geri döngü oturumuna hiçbir şey söylemez; bu nedenle ona bir `Timeout` veya bir iptal düğmesi verin. `Ephemeral`, macOS, iOS ve watchOS'tan tarayıcının çerezlerini paylaşmamalarını ister ve Android'de tarayıcının desteklediği yerde özel bir Custom Tab ister; Android'de çerez yalıtımı olduğunu varsaymayın. tvOS ve mini programlar şimdilik `core.AuthUnavailable` yanıtını verir. TSX'te `AuthSession.start(url, options)` `auth_session` yeteneğini gerektirir ve `result`'ı geri çağırmayla ya da iptal edildiğinde null ile çözülen bir oturum verir.

Android, eşleşen bir geri çağırmayı olağan `OnOpenLinks`'ten önce tüketir ve geri çağırma olmadan dönmek oturumu iptal edilmiş olarak sonlandırır. Geri çağırması uygulamaya ait özel bir şema kullanmalıdır; boş bir şema, HTTPS geri çağırması veya bildirilmemiş bir şema, oturum açma oturumlarından önce derlenmiş bir Android çalışma zamanında olduğu gibi `core.AuthUnavailable` döndürür.

Bir tarayıcı oturumu yalnızca eşleşen geri çağırma bağlantısını tüketir; ilgisiz ve geç gelen bağlantılar yine `OnOpenLinks`'e ulaşır. Aynı geri çağırma URI'si için başka bir tarayıcı oturumu başlatmak öncekini iptal eder; farklı URI'ler farklı pencerelerde bekleyebilir.

Yetkilendirme URL'si işlevine verilen yönlendirmeyi aynen kullanın. Web ve masaüstü geri döngü yönlendirmeleri, işlev onları almadan önce Unicode yolları kodlar ve nokta bölümlerini çözümler; bir geri döngü `Path`'i kodu çözülmüş metindir, bu nedenle düz bir `%` da kaçışlanır.

Mini programlar bunun yerine `cx.SignInMiniProgram(core.MiniProgramSignInOptions{}, done)` kullanır. Geri çağırma, `Platform`'u (`MiniProgramWeChat`, `MiniProgramQQ`, `MiniProgramDouyin` veya `MiniProgramAlipay`) ve boş olmayan, kısa ömürlü bir `Code` içeren bir `core.MiniProgramCredential` alır. Ana platformun kod takası için onu sunucunuza gönderin; uygulama gizli anahtarını orada tutun. Bu, profil bilgisi istemeden `wx.login`/`qq.login`/`tt.login` ya da Alipay'in temel `auth_base` yetkilendirmesidir. Bir URL açmaz veya bir OAuth URL oluşturucusu çalıştırmaz.

`cx.Env().CanSignInMiniProgram()` ana platform API'sinin kullanılabilirliğini bildirir. Diğer hedefler `core.MiniProgramSignInUnavailable` yanıtını verir; `core.AccessMiniProgramSignIn` olmayan kod `AccessDenied` alır. Douyin varsayılan olarak sessizce sorar: oturum açılmamış bir ana platform `MiniProgramSignInNotSignedIn` döndürür ve `MiniProgramSignInOptions{Prompt: true}` ana platform hesabının oturum açma penceresine izin verir. Bu pencereyi kapatmak `MiniProgramSignInCancelled` döndürür. Anonim bir cihaz kodu hiçbir zaman oturum açmış bir kullanıcı sayılmaz. Testler `nsgotest.Environment(core.Environment{MiniProgramSignIn: true})` ve `app.MiniProgramSignIn` ayarlar; `app.MiniProgramSignIns` istekleri kaydeder. TSX mini programlarda çalışmaz.

Web görünümlerinin, kullanıcının başka bir yerde (örneğin kendi tarayıcısında) zaten açık olduğu oturumla açılması gereken bir uygulama, o oturumun çerezlerini, sayfa betiklerinin ayarlayamadığı HttpOnly çerezler de dahil olmak üzere `cx.SetWebCookies` ile yazar, ardından sayfaları yükler: done, depo onları tuttuğunda çalışır. Depo, uygulamanın her web görünümünün paylaştığı kalıcı depodur; bu nedenle `Expires` değeri olan çerezler uygulamadan daha uzun yaşar; Gizli (Incognito) görünümler onları görmez, uygulama başına tek bir depo tutan Android hariç. HarmonyOS, web, tvOS ve watchOS `Unavailable` yanıtını verir.

```go
cx.SetWebCookies([]core.WebCookie{
	{Name: "SID", Value: sid, Domain: ".example.com", Expires: expires, Secure: true, HTTPOnly: true},
}, func(err error) {
	if err == nil {
		page.Set(core.WebSrc("https://www.example.com/"))
	}
})
```

Bir çerezi kaldırmak için onu aynı ad, alan adı ve yolla ve geçmişte kalan bir `Expires` ile yeniden yazın; sıfır bir son kullanma bir oturum çerezi oluşturur. Kaldırmayı gözlemlemesi gereken bir sayfayı yüklemeden önce geri çağırmayı bekleyin.

Tersi yönde, `cx.WebCookies` deponun çerezlerini, `document.cookie`'nin okuyamadığı HttpOnly çerezler ve onların alan adı, yolu, son kullanma tarihi ve bayrakları dahil, sitenin bir sayfası gösteriliyor olsun ya da olmasın listeler: bir `core.WebCookieFilter`, bir `URL`'ye yapılan isteğin göndereceklerini veya bir `Domain`'in ve altındaki her ana makinenin çerezlerini seçer; sıfır filtre ise hepsini seçer. Siteyi Go'dan kullanıcının oturumuyla çağıran veya kullanıcının nerede oturum açtığını gösteren bir uygulama onları bu şekilde okur. `cx.ClearWebsiteData` kullanıcının her siteden oturumunu kapatır: deponun çerezlerini, yerel ve oturum depolamasını, IndexedDB'yi, service worker'ları ve önbellekleri kaldırır; done, bunlar gittiğinde çalışır. Android'in `CookieManager`'ı yalnızca bir URL'nin göndereceği adları ve değerleri yanıtlar; bu nedenle orada yalnızca bir `URL` filtresi listeler, `Domain`'i ana makinesiyle doldurur ve diğer alanları boş bırakır; bir `Domain` filtresi veya sıfır filtre `Unavailable` yanıtını verir; orada temizleme HTTP önbelleğini bırakır. HarmonyOS, web, tvOS ve watchOS her ikisine de `Unavailable` yanıtını verir.

```go
cx.WebCookies(core.WebCookieFilter{Domain: "example.com"}, func(cookies []core.WebCookie, err error) {
	signedIn.Set(err == nil && slices.ContainsFunc(cookies, func(c core.WebCookie) bool { return c.Name == "SID" }))
})
```

## Apple ile Giriş Yap

`cx.SignInWithApple`, `cx.Env().CanSignInWithApple()`'ın true olduğu macOS, iOS ve tvOS'ta uygulamanın kendi düğmesinden sistemin Apple ile Giriş Yap sayfasını gösterir. Kimlik bilgisiyle ya da bir hatayla bir kez yanıt verir: kullanıcı sayfayı kapattığında `core.AppleSignInCancelled` (bu bir başarısızlık değildir), aksi halde `AppleSignInFailed` veya diğer platformlarda `AppleSignInUnavailable`. Apple e-postayı ve adı yalnızca kullanıcının uygulamaya ilk oturum açışında verir; bu nedenle onları o anda saklayın. `Nonce`, isteğe kimlik belirtecinin `nonce` talebinin tuttuğu küçük harfli onaltılık SHA-256 değeri olarak girer: sunucu uygulamaya verdiği nonce'u özetler ve karşılaştırır. `cx.AppleCredentialState` kullanıcının o zamandan beri onu iptal edip etmediğini söyler. Başsız testlerde `app.AppleSignIn` bir `nsgotest.AppleSignInRequest` alır ve kimlik bilgisini veya hatayı sağlar.

```go
nonce := rand.Text() // also sent to the server, which checks the token's claim against its SHA-256
cx.SignInWithApple(core.AppleSignInOptions{FullName: true, Email: true, Nonce: nonce}, func(c core.AppleCredential, err error) {
	switch {
	case errors.Is(err, core.AppleSignInCancelled):
	case err != nil:
		showError(err)
	default:
		signInOnServer(c.IdentityToken, c.AuthorizationCode, nonce, c.GivenName, c.FamilyName, c.Email)
	}
})
cx.AppleCredentialState(savedUser, func(s core.AppleCredentialState, err error) {
	if err == nil && s == core.AppleCredentialRevoked {
		signOut()
	}
})
```

Uygulama, `nsgo.yaml` istediğinde `build`'in eklediği Apple ile Giriş Yap yetkisiyle imzalanmalıdır; o olmadan sayfa başarısız olur:

```yaml
apple:
  sign_in_with_apple: true  # com.apple.developer.applesignin, in iOS and tvOS packages and Mac App Store builds
```

Xcode, `nsgo dev`'den gelen cihaz derlemeleri için yeteneği kaydeder; `sign.profile` veya `sign.macos_profile` içindeki bir provisioning profili, Apple ile Giriş Yap'ın açık olduğu bir App ID'den gelmelidir. macOS bunu yalnızca bir profille verir; bu nedenle bir disk görüntüsü veya bir Mac'te `nsgo dev` çalıştırması oturum açamaz, derlemeleri hiçbir geliştirici yetkisi talep etmeyen simülatör de açamaz.

## Uygulama içi satın alımlar

`cx.Products`, `cx.Purchase` ve yanlarındaki çağrılar, `nsgo.yaml`'ı StoreKit isteyen bir derlemede, `cx.Env().CanPurchase()`'ın true olduğu macOS, iOS ve tvOS'ta App Store üzerinden satış yapar:

```yaml
apple:
  purchases: true  # StoreKit's library, in macOS, iOS and tvOS packages; no entitlement is needed
```

Uygulamanın kendi fiyatlarını değil, her zaman mağazanın verdiği fiyatları (`DisplayPrice`) gösterin: mağaza her vitrini (storefront) kendisi fiyatlandırır. Bir satın alma, işlemle ya da bir hatayla bir kez yanıt verir: kullanıcı sayfayı kapattığında `core.PurchaseCancelled`, Satın Almak İçin Sor'da olduğu gibi onay beklerken `core.PurchasePending`, aksi halde mağazanın ifadeleriyle `core.PurchaseFailed` veya başka yerlerde `PurchasesUnavailable`. Doğrulamak için işlemin `Signed` değerini (mağazanın JWS'si) uygulamanın sunucusuna gönderin, satın alınanı teslim edin, ardından işlemi bitirin; mağaza bitmemiş bir işlemi her açılışta uygulamaya yeniden verir.

`cx.ManageSubscriptions` Apple'ın abonelik yönetimi arayüzünü açar (iOS'ta sistem sayfası, macOS ve tvOS'ta App Store abonelikler sayfası). `cx.RequestRefund` bir işlem için bir iade isteği başlatır ve `core.RefundSuccess` veya `core.RefundCancelled` yanıtını verir.

```go
cx.OnPurchaseUpdate(func(t core.Transaction) { // renewals, approved purchases, other devices', refunds, and unfinished ones at launch
	verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
})
cx.Products([]string{"com.example.pro.monthly"}, func(products []core.Product, err error) {
	if err == nil && len(products) > 0 {
		price.Set(products[0].DisplayPrice)
	}
})
cx.Purchase("com.example.pro.monthly", core.PurchaseOptions{AccountToken: userUUID}, func(t core.Transaction, err error) {
	switch {
	case errors.Is(err, core.PurchaseCancelled):
	case errors.Is(err, core.PurchasePending):
		showWaiting() // its transaction comes through OnPurchaseUpdate
	case err != nil:
		showError(err)
	default:
		verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
	}
})
```

`FinishTransaction`, `CurrentEntitlements`'ın döndürdüğü biri de dahil olmak üzere zaten bitmiş bir işlem için `PurchaseNotFound` döndürür; bitmiş bir tüketilemez ürün sahiplikte kalır.

`OnPurchaseUpdate`'i uygulama başlarken kaydedin: herhangi bir işleyiciden önce gelen güncellemeler ilkini bekler. `cx.CurrentEntitlements` açılışta kilidini açmak için kullanıcının şu anda neye sahip olduğunu yanıtlar ve bir Satın Alımları Geri Yükle düğmesi, kullanıcıdan oturum açmasını isteyebilecek olan `cx.SyncPurchases`'ı çağırır. `AccountToken`, işlemin ve App Store'un sunucu bildirimlerinin taşıdığı, uygulamaya ait bir UUID'dir. macOS yalnızca bir Mac App Store derlemesinde satış yapar ve bir App Clip hiçbir şey satmaz; Android, watchOS, Windows, Linux, web ve mini programlar şimdilik `PurchasesUnavailable` yanıtını verir. Satın alımları, `Products`, `PurchaseAnswer`, `UpdatePurchase` ve `UnfinishedTransactions` değerleri mağazanın yerini tutan `nsgotest` ile test edin. `app.PurchaseRequests` mağaza işlemlerini `nsgotest.PurchaseRequest` değerleri olarak kaydeder. TSX'te `Purchases` modülü `purchases` yeteneğini gerektirir: `purchase`, `{ status: "purchased", transaction }`, `"pending"` veya `"cancelled"` ile çözülür ve `on("update", listener)` güncellemeleri duyar.

## Arayüz yönü

iOS, Android, HarmonyOS veya tarayıcıdan yatay yön istemek için tam ekran bir oynatıcının kapsamında `cx.RequestOrientation(core.OrientationLandscape)`'i çağırın. Kapsamı canlı olan en son istek geçerli olur. O kapsam sona erdiğinde önceki istek yeniden geçerli olur veya uygulama olağan yön politikasına döner.

iOS 16 ve sonrasında istek arayüzü hemen döndürebilir. Daha eski iOS sürümlerinde bir sonraki sistem dönüşünü kısıtlar. Sunulan bir modalın `IOS.SupportedOrientations` değeri önceliklidir. Android'de ve HarmonyOS'ta, sistemin temsil edemediği dikey ve yatay yönlerin bir karışımı, zaten gösterilen tercih edilerek tek bir izin verilen yöne indirgenir. Android büyük ekran veya çoklu pencere modlarında yön isteklerini dikkate almayabilir.

Bir web uygulamasında, oynatıcının kapsamını açan bir basma işleyicisinden `desktop.ToggleFullScreen(cx)`'i çağırın. Tarayıcı basmadan tam ekrana girer, ardından NSGo oynatıcının yön isteğini uygular. Tam ekrandan çıkmak kilidi serbest bırakır. Tarayıcı konsolu desteklenmeyen işlemleri ve retleri bildirir.

`nsgotest.App.Orientation` istenen en son maskeyi kaydeder; olağan yön politikası için sıfırdır.

## Mağaza değerlendirmeleri

`cx.RequestReview()`, macOS'ta ve iOS'ta App Store'dan, Android'de Google Play'den uygulama içi bir puanlama istemi ister. `cx.Env().CanRequestReview()` hizmetin kullanılabilir olup olmadığını bildirir. İstemin gösterilip gösterilmeyeceğine ve ne zaman gösterileceğine mağaza karar verir ve hiçbir sonuç bildirmez. Apple istemini test etmek için bir geliştirme derlemesi kullanın. TestFlight derlemeleri onu bastırır.

Bağımsız Android paketleri Play Review SDK'sını, onun iletişim kutusu etkinliğini ve Play Store görünürlük sorgusunu içerir; bunları derlemek Android SDK'sı veya JDK gerektirmez. Etkin bir Play Store'u olmayan bir cihaz hizmeti kullanılamaz olarak bildirir. Yayımlanmış bir paketi [Internal App Sharing](https://developer.android.com/guide/playcore/in-app-review/test) kullanarak test edin.

NSGo'yu gömerken Gradle ana uygulamasına `com.google.android.play:review:2.0.2` ekleyin. Uygulamanın Play Store'u görebilmesi için ana uygulamanın manifestinde `<queries>` altına `<package android:name="com.android.vending" />` ekleyin. Ana uygulama kodunu küçültüyorsa, NSGo onları adıyla yüklediğinden Play Review ve Tasks API'lerini aşağıdaki kurallarla koruyun.

```proguard
-keep class com.google.android.play.core.review.ReviewManagerFactory { *; }
-keep interface com.google.android.play.core.review.ReviewManager { *; }
-keep class com.google.android.play.core.review.ReviewInfo { *; }
-keep class com.google.android.gms.tasks.Task { *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
```

Testlerde `nsgotest.Environment` içinde `CanRequestReview`'ı ayarlayın ve `nsgotest.App.ReviewRequests`'i denetleyin.

## Sistem kenar hareketleri

Ekranın kenarında dokunmatik denetimleri olan bir oyun, denetimler etkinken `mobile.DeferSystemGestures`'ı çağırabilir. İsteği erken serbest bırakmak için döndürülen işlevi kullanın. İstek, kapsamı temizlendiğinde de sona erer. Aynı penceredeki birden çok kapsamdan gelen istekler birleşir.

```go
release := mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{
	IOSEdges: mobile.GestureEdgeBottom,
	AndroidRects: []core.Bounds{{
		Origin: core.Point{Y: 120},
		Size:   core.Size{Width: 24, Height: 96},
	}},
})
```

iOS fiziksel ekran kenarlarını kullanır. Android 10 ve sonrası, sol üstten ölçülen yüzey noktalarında dikdörtgenler kullanır. Dikdörtgenleri onlara ihtiyaç duyan denetimlerin çevresinde tutun. Android ana ekran hareketini kendine ayırır ve bir uygulamanın Geri hareketinden hariç tutabileceği alanı sınırlar. `nsgotest.App.SystemGestures` testlere ana yüzeylerindeki geçerli birleşik isteği verir. Başka bir pencere için `Surface.SystemGestures()` kullanın.

## Android'de katmanlar

`mobile.OpenOverlay`, Android'de uygulamanın bir görünümünü diğer uygulamaların üzerinde gösterir: uygulamanın sinyallerini paylaşan, kendine ait bir ağaç; gösterilirken bir ön plan hizmeti uygulamayı çalışır durumda tutar. Uygulama `android.permissions` altında `SYSTEM_ALERT_WINDOW`'u listeler ve kullanıcı bunu, `mobile.RequestOverlayPermission`'ın açtığı sistem ayarlarında verir; bu izin olmadan katman hemen sona erer ve `OnEnd` nedenini duyar. Dokunmaları alır, ancak `Focusable` veya `Passive` olmadıkça tuşları ve çevresindeki dokunmaları öndeki uygulamaya bırakır ve içeriğinden `desktop.SetWindowFrame` ile taşınır. Diğer platformlar hiçbirini açmaz.

```go
mobile.RequestOverlayPermission(cx, func(p core.OverlayPermission) {
	if p == mobile.OverlayGranted {
		mobile.OpenOverlay(cx, mobile.OverlayOptions{Width: 200, Height: 80}, Score)
	}
})
```

## Konum ve hareket

`cx.Locate` tek bir konum belirlemesi alır, `cx.WatchLocation` konum belirlemelerini akış olarak iletir ve `cx.WatchMotion` ivmeölçer, jiroskop, manyetometre ve birleştirilmiş cihaz hareketi örneklerini akış olarak iletir. Her biri, `Stop`'u onu iptal eden bir `*core.SensorStream` döndürür; onu başlatan kapsamın sona ermesi de aynısını yapar; geri çağırmalar motor goroutine'inde çalışır. İzinler `core.PermissionLocation`, `PermissionBackgroundLocation` ve `PermissionMotion`'dır ([izinler](permissions.md)): bir akışı başlatmak hiçbir zaman izin istemez ve izni olmayan bir akış `core.SensorNotAllowed` ile başarısız olur.

```go
cx.RequestPermission(core.PermissionLocation, func(s core.PermissionStatus) {
	if !s.Allowed() {
		return
	}
	cx.Locate(core.LocationOptions{}, func(f core.LocationFix, err error) {
		if err == nil {
			showPosition(f.Latitude, f.Longitude, f.Approximate)
		}
	})
})
```

`Locate` yeni bir konum belirlemesini varsayılan olarak 30 saniye (`Timeout`) bekler; yalnızca `MaxAge` önbelleğe alınmış birini almasına izin verir ve bu, `Cached` ile `Age` aracılığıyla bunu belirtir. `Approximate`, kullanıcının yalnızca yaklaşık olarak izin verdiği bir konum belirlemesini belirtir; Android böyle bir konum belirlemesini en fazla on dakikada bir yeniler, bu nedenle orada `Locate`'e bunun için bir `MaxAge` verin. Koordinatlar WGS 84'tür; sağlayıcının sahip olmadığı bir ölçümün `Valid` değeri false'tur ve `Altitude` elipsoidaldir, `MSLAltitude` ise ortalama deniz seviyesinin üzerindedir. Hareket, cihazın doğal eksenlerini kullanır: m/s² cinsinden ivme (yüzü yukarı yatarken z'de +9.81), rad/s cinsinden dönme ve µT cinsinden manyetik alan; varsayılan olarak her 20 ms'de bir, 10 ms–1 s aralığında tutulur. Yük altında bir akış, sensör başına en yeni örneği tutar ve yerini aldığı örnekleri `Dropped` içinde sayar.

Akışlar uygulama etkin değilken devam eder ve arka planda duraklar; ardından gelen ilk örneği `Gap` ile işaretler; bekleyen bir `Locate` zaman aşımının geri kalanını korur. `LocationOptions.Background`, `PermissionBackgroundLocation` altında bir konum akışını arka planda sürdürür: Apple uygulamaları `location` arka plan modunu ekler; Android'de ise bir ön plan hizmeti `NotificationTitle` ve `NotificationBody` ile bir bildirim gösterir ve etkinliği sona erdikten sonra uygulamayı çalışır durumda tutar. Sistemin sonlandırdığı bir süreci hiçbir şey başlatmaz.

```yaml
apple:
  info:
    NSLocationWhenInUseUsageDescription: Shows where you are on the map.
    NSLocationUsageDescription: Shows where you are on the map.  # macOS
    NSMotionUsageDescription: Counts your reps.
    UIBackgroundModes: [location]                                # background streams
android:
  permissions: [ACCESS_COARSE_LOCATION, ACCESS_FINE_LOCATION, FOREGROUND_SERVICE, FOREGROUND_SERVICE_LOCATION]
```

iOS ve watchOS konum ve hareket, macOS konum, tvOS her seferinde tek bir konum belirlemesi verir; Android ise her ikisini de çatının kendi sağlayıcıları aracılığıyla verir. Linux konum belirlemelerini ve akışları sistem veri yolundaki GeoClue 2'den alır; GeoClue'nun aracısı, uygulamanın ilk isteği başlarken kullanıcıya sorar; GeoClue'ya istem göstermeden sorulamadığından, izin o çalıştırmada uygulama sorana kadar sorulmadı olarak okunur ve arka planda konum kullanılamaz. Windows bunları WinRT'nin `Geolocator`'ından alır; izin, uygulama için konum kapalıyken reddedildi, uygulama o çalıştırmada sorup izin aldıktan sonra verildi, aksi halde sorulmadı olarak okunur ve orada da arka planda konum kullanılamaz. Güvenli bir tarayıcı coğrafi konum belirlemeleri ve mobilde ivmeölçer, jiroskop ve birleştirilmiş hareket okumaları sağlar. Safari'de `cx.RequestPermission(core.PermissionMotion, done)`'u bir basma işleyicisinden çağırın. Tarayıcı, `navigator.geolocation` olmadan konumu, masaüstlerinde ve manyetometre akışları için hareketi kullanılamaz olarak bildirir. TSX'te `Sensors`, `location`, `backgroundLocation` ve `motion` paket yetenekleri altında, milisaniye cinsinden `permission`, `requestPermission`, `locate`, `watchLocation` ve `watchMotion`'a sahiptir.

## Ses

`audio` paketi, `cx.Env().CanPlayAudio()`'nun true olduğu yerde belleğe bir kez çözülen kısa sesleri (`audio.LoadSound`) ve akışla gelen uzun sesleri (`audio.NewPlayer`) çalar; başka yerlerde çağrılar `audio.Unavailable` ile başarısız olur.

```go
audio.LoadSound(cx, audio.Source{URI: "asset:sounds/click.wav"}, func(click *audio.Sound, err error) {
	if err != nil {
		return
	}
	click.Play(audio.Voice{})                                  // center, full volume, normal rate
	click.Play(audio.Voice{Volume: 0.5, Rate: 1.2, Pan: -0.5}) // overlaps, panned left
})
```

`audio.Voice` oynatma başına `Volume` (0'dan 1'e, 0 1 olarak), `Rate` (0.5'ten 2'ye, 0 1 olarak) ve `Pan` (tamamen sol için -1'den tamamen sağ için 1'e, 0 ortada) ayarlar; bu aralıkların dışındaki değerler sınırlanır ve NaN varsayılanı alır. `Sound.Stop` sesin tüm oynatmalarını durdurur ve `Close` onu serbest bırakır; sahip kapsamı atıldığında da aynısını yapar. `Player`; `Play`, `Preload`, `Pause`, `Seek`, `SetVolume`, `SetRate`, `SetLoop` ve `Close`'u destekler. Bugün macOS, iOS, Windows, Linux, Android ve web ses çalar. Apple, Windows ve tarayıcı arka uçları WAV, AAC ve MP3'ü destekler. Linux GStreamer kodeklerini kullanır ve akış oynatıcısı 1× hızda çalar. Android, görünümü ekrandan ayrıldığında bir oynatıcıyı duraklatır ve her çözülmüş sesi 1 MiB ile sınırlar. tvOS, watchOS, HarmonyOS ve mini programlar hâlâ `CanPlayAudio` için false bildirir.

Tarayıcılar duyulabilir oynatma için bir kullanıcı hareketi gerektirir. Kısa sesleri önceden yükleyin, ardından bir tıklama, dokunma veya tuş basışından sonra `Sound.Play`'i çağırın. Bir hareketten önce çalınan bir ses sessizdir. Tarayıcı oynatmayı engellerse bir oynatıcının `Play` geri çağırması `audio.NotAllowed` alır. Bir kullanıcı hareketinden yeniden deneyin.

`Sound.Play`, macOS, iOS, Windows, Linux ve web'de kendi ses düzeyini ve hızını ayarlayabilen veya bağımsız olarak durabilen bir `audio.Playback` döndürür. `Ended()` tepkiseldir ve o oynatma bittiğinde, durduğunda, ses sınırı tarafından yerinden edildiğinde veya başlayamadığında true olur. Tek tek denetimleri olmayan arka uçlar bitmiş bir tanıtıcı döndürür.

iOS'ta, `audio` ve `Video` dışında çalan, bir kitaplığın kendine ait ses motoru (örneğin bir `AudioQueue`), çalarken uygulamanın tek ses oturumunu `audio.HoldSession(core.AudioSessionPlayback)` ile tutar ve durduğunda döndürülen serbest bırakma işlevini çağırır. Bir oynatıcının `AudioSession`'ının aldığı düzeyleri, uygulamanın videoları, oynatıcıları ve kayıtlarının yanında alır; böylece `AudioSessionPlayback` ve `AudioSessionMixWithOthers`, Zil/Sessiz anahtarı sessizdeyken de çalmaya devam eder. Başka yerlerde hiçbir şey tutmaz.

Bir efekti sessiz başlatmak için `audio.Voice{Muted: true}`, etkin bir efekti susturmak için `playback.SetVolume(0)` kullanın. Uzun sesler için `audio.NewPlayer` bir kaynağı akış olarak iletir ve `player.SetMuted(true)` oynatma devam ederken çıkışı susturur. Sesi açmak oynatıcının yapılandırılmış ses düzeyini geri yükler. `Voice.Volume` ve `Player.SetVolume`, varsayılan tam ses düzeyi için sıfırı kullanır.

TSX'te `nsgo/audio`, `audio` paket yeteneğiyle denetlenen `supported()` ve `loadSound(uri)`'yi sunar. `loadSound`, `play({ volume, rate, pan, loop, muted })`'u `setVolume`, `setRate`, `stop` ve tepkisel bir `ended` içeren bir oynatma döndüren bir sesle çözülür; orada 0 ses düzeyi sessizliktir. Reddedilen bir yükleme, `reason`'ı ses hatasının adı (örneğin `quota-exceeded`) olan ve `code`'u yetenek olmadan `ERR_ACCESS_DENIED`, bir sınır aşıldığında `ERR_NSH_QUOTA_EXCEEDED` ve hiçbir şeyin ses çalmadığı yerde `ERR_FEATURE_UNAVAILABLE_ON_PLATFORM` olan bir `Error` ile reddedilir. Bir ses yalnızca `close()` ile veya görünümü gittiğinde serbest bırakılır; bir sandbox'ta her yükleme, başarısız olana veya sesi kapanana kadar `Limits.Sounds`'a (varsayılan olarak 256) sayılır.

## Oyun kumandaları

`gamepad` paketi, bağlı oyun kumandalarını W3C Standard Gamepad düzeninde okur (`ButtonSouth`/`ButtonA`'dan `ButtonHome`'a, `AxisLeftX`/`AxisLeftY`/`AxisRightX`/`AxisRightY`). `cx.Env().SupportsGamepads()` hedefin kumandaları destekleyip desteklemediğini bildirir (macOS, iOS, Windows, Linux, Android ve Gamepad API'sine sahip tarayıcılar).

`gamepad.Pads(cx)`, kumandalar bağlandığında veya bağlantısı kesildiğinde kapsamları yeniden oluşturan, ancak düğme veya çubuk değişikliklerinde yeniden oluşturmayan izlenen bir tepkisel okumadır. Her `gamepad.Pad`; `Family` değerini (`FamilyXbox`, `FamilyPlayStation`, `FamilyNintendo` veya `FamilyGeneric`; `gamepad.DetectFamily` ile de sınıflandırılır), platformun bildirdiği yerde USB `VendorID` ve `ProductID` değerlerini ve dokunsal motor desteği için `CanRumble`'ı sunar. Bir `RequestAnimationFrame` döngüsü içinde `gamepad.State(cx, id)`, ekran karesi başına bir kez anlık görüntüsü alınan kumandanın düğmelerini (`[0, 1]`, `>= 0.5` değerinde `Pad.Pressed` true) ve eksenlerini (`[-1, 1]`, negatif sol/yukarı ve pozitif sağ/aşağı) döndürür.

`gamepad.Rumble(cx, id, lowFreq, highFreq, duration)`, düşük frekanslı (ağır/sol) ve yüksek frekanslı (hafif/sağ) titreşim motorlarını `[0, 1]` aralığına sınırlanmış yoğunluklarla çalıştırır; o kumandada çalışan herhangi bir titreşimin yerini alır ve `duration` sonunda, uygulama arka plana geçtiğinde veya odağı kaybettiğinde (`cx.Env().Lifecycle() != core.LifecycleActive`), `cx` atıldığında ya da kumandanın bağlantısı kesildiğinde otomatik olarak durur. `gamepad.StopRumble(cx, id)` titreşimi erken durdurur; `CanRumble == false` olan kumandalar titreşim çağrılarını sorunsuzca yoksayar.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if pad, ok := gamepad.State(cx, 0); ok {
		move(pad.Axes[gamepad.AxisLeftX], pad.Axes[gamepad.AxisLeftY], pad.Pressed(gamepad.ButtonA))
		if hit {
			gamepad.Rumble(cx, pad.ID, 0.8, 0.3, 180*time.Millisecond)
		}
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

Yerel odak duraklarında bir kumandayla gezinmeyi etkinleştirmek için `gamepad.EnableFocusNavigation(cx)`'i çağırın (D-pad ve sol çubuk odağı taşır, `ButtonSouth` odaklanmış denetimi etkinleştirir ve `ButtonEast` geri gitmeyi tetikler). Etkin oynanış görünümleri, bağlı oldukları sürece odak gezinmesini duraklatmak için `gamepad.CaptureInput(cx)`'i çağırır. TSX'te `nsgo/gamepad`, `gamepad` paket yeteneğiyle denetlenen `supported()`, `pads()`, `state(id)`, `Button` ve `Axis`'i sunar.

## Ses zamanlaması

Zamanlanmış oynatma için saat olarak `audio.Now(cx)`'i kullanın. `audio.CanSchedule(cx)` true olduğunda `audio.Voice.StartAt`'i gelecekteki bir zamana ayarlayın. Etkin çıkış rotası için `audio.OutputLatency(cx, func(latency time.Duration, known bool) { ... })`'i çağırın. macOS, iOS, Linux ve web zamanlanmış başlatmaları destekler; Windows'ta ve Android'de `CanSchedule` false'tur ve bir `StartAt` hemen çalar. Geri çağırmanın `known` değeri, rotanın bir gecikme bildirip bildirmediğini söyler; Windows ve Android bildirmez.

## Oyun hizmetleri

`gameservices` paketi, `cx.Env().CanUseGameServices()`'in true olduğu yerde oyuncu oturum açma, başarımlar, liderlik tabloları ve yerel Game Center / Play Games katmanları için sağlayıcıdan bağımsız bir API sağlar. Her geri çağırma motor goroutine'inde çalışır; böylece kareleri engellemeden sinyalleri doğrudan ayarlayabilir.

```go
if cx.Env().CanUseGameServices() {
	gameservices.Authenticate(cx, func(p gameservices.Player, err error) {
		if err == nil && p.Authenticated {
			playerName.Set(p.DisplayName)
			gameservices.ReportAchievement(cx, "first-run", 100, nil)
			gameservices.SubmitScore(cx, "high-scores", 1250, nil)
		}
	})
}
```

`gameservices.ReportAchievement` ilerlemeyi `0..100` aralığına sınırlar (`100` başarımın kilidini açar). `gameservices.LoadScores`, `AllTime`, `Week` veya `Today` süresi boyunca `Global` ya da `Friends` için sıralı `gameservices.ScoreEntry` satırlarını okur; `gameservices.ShowAchievements` ve `gameservices.ShowLeaderboard` ise platformun yerel katmanlarını açar. macOS, iOS ve tvOS GameKit'e purego aracılığıyla ulaşır; Android, Play Games Services'e henüz çalışma zamanında bulunmayan bir Java çalışma zamanı kancası aracılığıyla ulaşacaktır, bu nedenle şimdilik `CanUseGameServices() == false` bildirir; Windows, Linux, web, watchOS ve mini programlar `CanUseGameServices() == false` bildirir ve `gameservices.ErrUnsupported` yanıtını verir.

Testlerde oyun hizmetlerini `nsgotest.Environment(core.Environment{CanUseGameServices: true})` ile etkinleştirin ve her uygulamanın yalıtılmış sahte uygulamasını `App.SetGameServicesPlayer`, `App.SetGameServicesFriends`, `App.SeedLeaderboardScores`, `App.SetGameServicesError`, `App.GameServicesAchievements`, `App.GameServicesScores`, `App.GameServicesShownAchievements` ve `App.GameServicesShownLeaderboards` ile yönetin.

## Sonraki adımlar

- [Bildirimler](notifications.md): izin, rozetler ve basmalar.
- [Formlar](forms.md): metin alanları ve klavye.
- [Gömme](embedding.md): bir ana uygulamanın gömülü uygulamasına verdiği hizmetler.
- [Derleme ve dağıtım](building.md): bir paketin ihtiyaç duyduğu izinler ve dosyalar.

### Fitness, bölgeler ve sınırlı hareket

`cx.WatchSteps`, `cx.WatchActivity` ve `cx.WatchPressure` kapsamlı akışlar döndürür. Adımlardan/etkinlikten önce ve Apple'da basınçtan önce `core.PermissionActivityRecognition` isteyin. Apple üçünü de Core Motion aracılığıyla sağlar; Android adım sayaçlarını ve basıncı sağlar, etkinlik sınıflandırmasını ise kullanılamaz olarak bildirir. Sayımlar örneğin `Since` penceresine aittir; sürdürülen pencereler `Gap` taşır. Basınç paskal cinsindendir ve isteğe bağlı olarak metre cinsinden göreli yükseklik içerir. Eksik mesafelerin, katların ve yüksekliğin `Valid` değeri false'tur.

`cx.WatchGeofences`, bir ile yirmi arasında farklı `GeofenceRegion` dairesi (WGS 84 koordinatları ve metre cinsinden yarıçap) içeren `core.GeofenceOptions` alır. Her bölge için `GeofenceInside`, `GeofenceOutside` veya `GeofenceUnknown` bildirir. `Background`, arka planda konum erişimi/bildirimleri ve Apple'ın Her Zaman yetkilendirmesini gerektirir; Android ayrıca bir bildirim başlığı gerektirir. Durdurma veya atma kayıtlarını kaldırır; hiçbir abonelik sürecin ölümünden sonra hayatta kalmaz.

`cx.RequestPreciseLocation("route", done)` açıkça hassas yetkilendirme ister ve hiçbir zaman bir konum değil, bir boolean ile yanıt verir. `apple.info` içinde `NSLocationTemporaryUsageDescriptionDictionary: {route: Explains why this feature needs precision.}` bildirin. Android hem yaklaşık hem de hassas izinleri gerektirir. False bir yanıt, yaklaşık konumu yetkilendirilmiş bırakabilir.

`cx.WatchBackgroundMotion(core.BackgroundMotionOptions{Motion: core.MotionOptions{}, Duration: 20 * time.Second, NotificationTitle: "Recording motion"}, sample, failed)` sonlu bir canlı akış ister. `AccessMotion` ve `AccessBackgroundMotion` gerektirir; TSX paketleri `motion` ve `backgroundMotion` bildirir. iOS yürütme süresini daha erken sona erdirebilir. Android uygulamaları `android.background_motion: true` ayarlar ve `FOREGROUND_SERVICE` bildirir; yapışkan olmayan hizmet, istenen süre daha uzun olsa bile üç dakika içinde durur. Sürenin veya işletim sisteminin sona erdirmesi `SensorTimedOut` bildirir. Durdurma/atma sağlayıcıyı ve süreyi sona erdirir; sonlandırmadan sonra hiçbir şey kaydedilmez. Diğer hedefler kullanılamaz bildirir.

TSX `Sensors` bu altı yöntemi yansıtır ve milisaniye kullanır. Başsız testler `FeedSteps`, `FeedActivity`, `FeedPressure` ve `FeedGeofence` ile veri besler ve doğruluk istekleri için `PreciseLocationAnswer` ayarlar. Fiziksel fitness, bölge uyandırması ve askıya alınmış arka plan davranışı hâlâ cihaz üzerinde kabul testi gerektirir.
