---
title: Bileşenler ve durum
description: Öğeler, bileşenler, sinyaller ve stiller.
group: Temel kavramlar
order: 50
---

## Öğeler

Ekran, Go struct değerleri olan öğelerden oluşan bir ağaçtır. `core` temel yapı taşlarını içerir: `View`, `Text`, `Image`, `Scroll`, `Input` ve daha fazlası. `ui` ise bir uygulamanın kullandığı `Button`, `Select`, `DatePicker` ve `EmojiPicker` gibi denetimleri temayı izleyen bir görünümle sunar. (`base`, [bileşen yazarları](authoring/index.md) için aynı denetimleri stilsiz olarak tutar.)

```go
core.View{
	Style: core.Style{Padding: core.All(24), Gap: 12},
	Children: []core.Node{
		core.Text{Content: "Hello"},
	},
}
```

`core.WebView` web içeriği gösterir; boş bir `WebSource`, `about:blank` yükler ve olağan yükleme ve gezinme olaylarını bildirir. Önceki sayfayı temizlemek için `SourceFunc` değerini boş bir kaynağa ayarlayın veya `Ref.Load(core.WebSource{})` çağırın. Sayfanın kendi betiklerinden önceki betik bir sinyali izlediğinde `InjectedJavaScriptBeforeContentFunc` ayarlayın: sinyal değiştiğinde, görünüm — kullanıcının veya `Ref.Load`'ın gittiği sayfa dahil — gösterdiği sayfanın yerine kaynağını yeni betikle yeniden yükler. Platformun kendi kullanıcı aracısının (user agent) sonuna veya ayarlıysa `UserAgent` değerinin sonuna `Version/26.0 Safari/605.1.15` gibi bir belirteç eklemek için `UserAgentSuffix` (TSX `userAgentSuffix`) ayarlayın; macOS ve iOS'ta platformun kendi kullanıcı aracısına eklenen sonek yalnızca görünüm ilk oluşturulduğunda etkili olur, HarmonyOS ve web ise kendi kullanıcı aracılarını korur. `Reload`, `GoBack` ve `GoForward` çağırmak için bir `core.Ref` bağlayın. Yerel hedeflerde bu komutlar uygulamanın kendi HTML ve veri kaynaklarını `core.AccessWebSchemes` kısıtından muaf tutar; yeni gezinmeler oluşturan sayfalar yine erişim politikasını izler.

tvOS üzerinde `cx.Env().HasWebView()` yanlıştır (`false`). Bir `WebView` bağlamak WebKit'i yüklemeden `OnError` üzerinden bir `*core.WebError` bildirir; o hedef için uygulama içeriği sağlayın.

macOS, iOS, Windows ve Linux'ta `OnNavigationStateChange` aynı belge içindeki URL, geçmiş ve başlık değişikliklerini de bildirir.

Bir sayfanın `getUserMedia` isteklerine izin vermek için `WebView.Media` alanını `core.WebMicrophone`, `core.WebCamera` veya her ikisine ayarlayın. Kullanım açıklamalarını `apple.info` içinde, `RECORD_AUDIO` veya `CAMERA` izinlerini ise `android.permissions` içinde bildirin; sistem yine de kullanıcıya sorar. Android'de, sistem istemi açıkken uygulama istediği cihazlardan birini kaldırırsa, sayfa ayrılırsa veya görünüm kapanırsa bekleyen istek reddedilir. Böyle bir değişiklikten sonra sayfanın yeniden erişim istemesi gerekir.

`WebView.Muted` ve `MutedFunc` (TSX `muted`), Windows ve Linux'ta Web Audio ve gömülü çerçeveler dahil tüm sayfa sesini susturur. macOS 12 ve iOS 15 veya sonrası sessize alındığında medya oynatımını askıya alır. Diğer barındırıcılar olağan oynatımlarını sürdürür. Güncellemeler geçerli sayfaya uygulanır ve gezinmeler boyunca taşınır.

TSX'te `injectedJavaScriptBeforeContentLoaded` değişikliği, tıpkı `InjectedJavaScriptBeforeContentFunc` gibi, aynı web görünümünde kaynağı yeni betikle yeniden yükler; `injectedJavaScript` değişikliği ise `userAgent`, `userAgentSuffix` veya `mobile` değişikliği gibi yeni bir web görünümü oluşturur ve kaynağı baştan yükler. HarmonyOS'ta enjekte edilen betiklerden herhangi birinin değişmesi yeni bir web görünümü oluşturur ve yeni bir yerel geçmiş başlatır. Windows ve HarmonyOS'ta enjekte edilen her betik ana çerçevede bir blok içinde çalışır; bu nedenle üst düzey `let`, `const` ve `class` bildirimleri o bloğa yerel kalır; sayfanın görmesi gereken bir genel değişken için `window` üzerine atama yapın.

Windows, macOS ve Linux'ta YouTube Shorts gibi yalnızca telefonlarda dikey kaydırmayla video değiştiren siteler için sayfayı bir telefonun gördüğü gibi göstermek üzere `Mobile` (TSX `mobile`) ayarlayın: `UserAgent` ayarlı değilse sayfa bir telefonun kullanıcı aracısını ve üzerine gelme (hover) olmayan bir dokunmatik ekranı alır; fareyle sürükleme veya kaymayan bir alan üzerindeki tekerlek ya da izleme dörtgeni kaydırması sayfaya parmak kaydırması olarak ulaşır. Windows, Chrome'un cihaz modunun yaptığı gibi bir telefonu öykünür. macOS ve Linux bunu bir betikle yapar; bu nedenle sayfalarının stil sayfaları hâlâ bir fare görür ve dokunma olayları güvenilmez (untrusted) olarak işaretlenir, ki bir sayfa bunu yok sayabilir. iOS, Android, HarmonyOS, web ve mini programlar sayfaları oldukları gibi gösterir.

macOS, iOS, Windows ve Linux'ta `OnOpenWindow`, bir web sayfasından gelen yeni pencere isteklerini alır. Görünüm açılır pencere oluşturmaz. Aynı görünümü `Ref.Load` ile yönlendirmeye, URL'yi başka bir yerde açmaya veya yok saymaya karar vermek için isteğin `URL` ve `UserInitiated` alanlarını kullanın. Kısıtlanmış bir görünüm reddedilen istekleri `OnError`'a gönderir.

Bir web görünümündeki sayfaların indirdiği dosyaları macOS, iOS, Linux, Windows ve Android'de kaydetmek için `OnDownload` ayarlayın. İndirmeler uygulamanın veri dizini içinde `downloads/<id>/<file name>` yoluna kaydedilir ve başlangıcı, bayt sayısıyla tamamlanmayı veya hatayı bildirir; `OnDownload` olmayan görünümler veya kısıtlanmış görünümler indirmeleri iptal eder.

`core.Video`, platformun oynatıcısında bir dosya veya URL oynatır. macOS, iOS ve tvOS'ta sonlu bir video klibin sesini ayrı bir dosya veya URL'den sağlamak için `Video.AudioSource` ayarlayın. Oynatıcının sesi o kaynaktan gelir. Görüntü ve ses birlikte başlar ve `Ref.Seek` ikisini aynı zaman çizelgesinde taşır. Oynatma süresini video belirler ve daha kısa olan ses bittiğinde susar.

Saydam bir klip arkasındakini macOS'ta `.mov` içinde alfa kanallı HEVC olarak (iOS henüz kontrol edilmedi) ve Linux'ta GStreamer'ın `codecalpha` eklentisiyle (`gstreamer1.0-plugins-bad`, bir Linux paketi bunu `nsgo.yaml` içinde önerebilir) WebM içinde alfa kanallı VP9 olarak gösterir. macOS, iOS, Windows ve Linux'ta alfası korunamayan yerel bir klip (Apple platformlarında WebM, Linux'ta HEVC ve Windows'ta Media Foundation'ın alfa çıktısı bir Windows makinede kontrol edilene kadar herhangi biri) videonun opak bir kutuda görünmesi yerine `OnError`'ın bunu bildirmesini sağlar: hata, `Reason` alanı `core.MediaErrorAlphaUnsupported` olan bir `*core.MediaError`'dır; böylece uygulama o klibin kodunu kendisi çözebilir. Android ve web ile her platformdaki uzak URL'ler bu tür klipleri kod çözücülerinin çıktısına göre oynatır ve bu çıktı opak olabilir.

`OnPause` ve `OnPlay`, Apple, Android, Windows, Linux ve web'deki yerleşik denetimleri izler. `OnPause`, bir denetim duraklatması için `VideoPauseControls` verir. `Paused`, bir kaynak açıldığında veya değeri değiştiğinde uygulanır. Ses düzeyi, hız ve yerleşim değişiklikleri, `Paused` doğru kalsa bile bir denetim oynatması dahil yerel bir duraklatma veya oynatmayı korur. Uygulamanız kendi oynat düğmesini gösteriyorsa `PausedFunc`'ı durumunuza bağlayın ve bu geri çağrılarda güncelleyin.

`Rate` oynatma hızını ayarlar ve `RateFunc` oynatma sırasında bunu değiştirebilir. Bir hız denetimi göstermek için `OnLoad`'dan gelen `VideoInfo.RateChangeable` değerini kullanın. Linux, GstPlay ve GStreamer'ın `scaletempo` filtresi kurulu olduğunda aranabilir (seekable) videoların hızını değiştirebilir. Yedek oynatıcı 1× kullanır.

Android'de ve Linux, macOS, iOS ve tvOS'ta MP4 veya MOV gibi doğrudan HTTP dosyaları için `Referer`, `User-Agent` veya `Cookie` gibi HTTP istek üstbilgilerini göndermek üzere `Video.Headers` ayarlayın.

```go
core.Video{
    Source:  "https://video.example/clip.mp4",
    Headers: map[string]string{"Referer": "https://video.example/player"},
}
```

Video bağlandığında eşlemi (map) kopyalar ve `SourceFunc` yeni bir URL sağlarsa aynı üstbilgileri kullanır. Apple platformlarında arama (seeking) için bilinen bir bayt uzunluğuna ve aralık (range) desteğine sahip bir kaynak kullanın. Oynatıcı yapılandırılmış üstbilgileri aynı kökenli yönlendirmeler boyunca gönderir. Aynı üstbilgiler Apple platformlarında doğrudan HTTP `AudioSource` dosyası için de geçerlidir. `.m3u` veya `.m3u8` ile biten oynatma listesi URL'leri yerel yükleyiciyi kullanır.

Linux'ta GstPlay ve GStreamer'ın `scaletempo` ve `souphttpsrc` eklentilerini kurun. `User-Agent` ayarlıyorsanız veya `souphttpsrc` eklentinizde `location-trusted` özelliği yoksa son medya URL'sini kullanın. Bu özellik ve varsayılan kullanıcı aracısı ile GStreamer aynı kökenli yönlendirmelerde uygulama üstbilgilerini korur. Uygulamanın üstbilgileri kaynak dosyaya uygulanır; uyarlanabilir medya parçaları kendi isteklerini kullanır.

## Bileşenler

Bir bileşen, bir kapsam (scope) alan ve bir öğe döndüren bir işlevdir. Bir kez çalışır; daha sonra değişenler sinyaller tarafından yönlendirilir.

```go
func Greeting(cx *core.Scope) core.Node {
	return core.Text{Content: "Hello from NSGo"}
}
```

Özelliklere (props) sahip bir bileşen, `Render` yöntemi olan bir türdür:

```go
type Badge struct{ Text string }

func (b Badge) Render(*core.Scope) core.Node {
	return core.Text{Content: b.Text}
}
```

`ui.ToastContent`, `NoClose` ile kapatma düğmesini çıkarabilir, bir `Trailing` düğümü ekleyebilir ve `OnPress` üzerinden bildirimi kapatabilir. `Toasts.NoClose` yığındaki tüm kapatma düğmelerini gizler.

`ui.Toasts.Style` ve `StyleFunc`, arka planı, kenarlığı, köşeleri ve iç boşluğu dahil her bildirim (toast) yüzeyini özelleştirir. Yığının stili temanın üzerine birleştirilir, ardından her `ToastContent.Style` ve `StyleFunc` gelir. Her `StyleFunc` karşılık gelen `Style` sonrasında çalışır ve sinyalleri değiştiğinde güncellenir. Bu stillerdeki bir `Color` başlığı, azaltılmış opaklıkla da açıklamayı renklendirir; böylece metin koyu veya renkli bir yüzeyde okunaklı kalır. Giriş, çıkış ve kaydırma aşamaları hâlâ kendi hareket stillerini uygular. TSX'te `Toasts` öğesinin `style` özelliğini ayarlayın ve tek bir bildirim için `show` işlevine `style` veya `styleFunc` geçirin.

## Sinyaller

Bir sinyal bir değer tutar. Bir öğenin çağırdığı `ContentFunc` gibi bir işlevin içinde okunması, ekranın o kısmını abone yapar ve değer değiştiğinde yalnızca o kısım güncellenir.

```go
count := core.NewSignal(cx, 0)

core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }}

ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }}
```

Tam bir dilimin (slice) sinyali kendisini okuyan her satırı günceller. `core.NewCollection` öğeleri anahtara göre, her birini kendi sinyalinde tutar; böylece bir satır kendi öğesini izler ve bir öğeyi değiştirmek yalnızca bir satırı çalıştırır:

```go
todos := core.NewCollection(cx, func(t Todo) int { return t.ID }, nil)

core.List{Count: todos.Len, Key: func(i int) any { return todos.KeyAt(i) },
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Text{ContentFunc: func() string { t, _ := todos.At(row.Index()); return t.Title }}
	}}

todos.Update(id, func(t Todo) Todo { t.Done = true; return t })
```

Diğer durumlar da aynı şekilde sinyaller üzerine kurulur:

- `core.NewSelector` bir anahtarın seçili olup olmadığını söyler ve yalnızca seçimin ayrıldığı ve ulaştığı satırları çalıştırır.
- `core.NewHistory` geri al ve yinele özellikleriyle bir değeri tutar; `CanUndo` ve `CanRedo` sinyalleri bir Düzenle menüsü içindir.
- `core.NewPersisted` tür güvenli bir değeri uygulamanın depolama alanında sürümlendirilmiş JSON olarak tutar ve eski sürümlerin yazdıklarını geçirir (migrate eder).
- `core.From`, başka bir goroutine üzerinde teslimat yapan bir abonelik gibi grafik dışındaki bir kaynağı izler.

## Stiller

`core.Style` CSS'i izler: flexbox ve grid, iç ve dış boşluklar, köşeler, kenarlıklar, gölgeler, gradyanlar, dönüşümler ve `Active` gibi durumlar. Öğeler ayrıca `Class` aracılığıyla Tailwind sınıfları da alır:

```go
core.Text{Content: "Done", Class: "self-start rounded-full bg-zinc-100 px-3 py-1 text-sm"}
```

Temalar `ui` bileşenlerine açık ve koyu modda görünümlerini verir.

## Masaüstü uygulamaları

`nsgo.Run(Root)` tek bir kök görünüm gösterir. Bir masaüstü uygulaması bunun yerine ne göstereceğini kendisi açabilir: `nsgo.Run(App)`, `func App(app *core.Scope)` işlevini bir kez çalıştırır ve o da `desktop` ile kendi sinyallerini paylaşan pencereler, paneller, tepsi simgeleri ve genel kısayollar açar. Bu macOS, Windows ve Linux'ta çalışır; macOS'ta `examples/tray` Dock'tan uzak durur ve menü çubuğunda yaşar. `nsgo.Run(App, nsgo.StartHidden())` ana pencereyi göstermeden açar; böylece bir tepsi uygulaması veya sistemin oturum açmada başlattığı (`desktop.LaunchedAtLogin`) bir uygulama pencereyi anlık olarak parlatmaz; uygulama pencereyi `app.MainWindow().Show()` ile gösterir. Telefonlar ve web uygulamayı başlatıldığı haliyle gösterir.

`desktop.SetWindowLevel`, bir zamanlayıcı veya masaüstü evcil hayvanı gibi bir pencereyi diğerlerinin üzerinde ya da altında tutar; `desktop.SetWindowFrame` ise onu taşır ve boyutlandırır. `desktop.CanSetWindowLevel` ve `desktop.CanPlaceWindows`, bir pencerenin kapsamında etkili olup olmadıklarını bildirir: macOS, Windows ve X11 altındaki Linux'ta ikisi de etkilidir; pencere düzenleyicisi (compositor) tarafından yığınlanan ve yerleştirilen sıradan bir Wayland üst düzey penceresinde ise ikisi de etkili değildir.

`desktop.WindowOptions{Transparent: true}` ile açılan bir pencerenin veya `nsgo.WindowTransparent()` ile açılan ana pencerenin arka planı veya çerçevesi yoktur; böylece uygulamanın hiçbir şey çizmediği yerlerde bir masaüstü evcil hayvanı veya widget için olduğu gibi masaüstü görünür; `Desktop.WindowDrag` içeren bir görünüm pencereyi taşır. `NoShadow` sistemin gölgesini kaldırır ve `SkipTaskbar` pencereyi Windows görev çubuğundan, Alt+Tab'dan ve X11 görev listesinden uzak tutar; `desktop.SetWindowTransparent`, `SetWindowShadow` ve `SetSkipTaskbar` bunları daha sonra değiştirir. Windows saydamlığı yalnızca gölgesiz olarak saydam açılmış bir pencerede değiştirir ve Wayland'da bir pencereyi görev listesinden uzak tutacak bir ipucu yoktur. `desktop.CanMakeWindowsTransparent` ve `desktop.CanSkipTaskbar`, bir pencerenin kapsamında bunların etkili olup olmadığını bildirir: saydamlık Windows'ta DirectComposition ve Linux'ta bir pencere düzenleyicisi (birleştiricisi olmayan X11'de yoktur) gerektirir; macOS'ta `desktop.SetDockVisible` uygulamayı Dock'tan çıkarır.

## Terminal

`base/terminal` paketini içe aktarın ve uzak bir akış için `terminal.New(terminal.Options{Conn: conn})` ile bir oturum oluşturun veya macOS, Linux ya da Windows'ta yerel bir program için `Command` ayarlayın. `terminal.View{Terminal: session}` işleyin ve oturum bittiğinde `Close` çağırın. `Options.ImageLimit`, Kitty grafik depolamasını bayt cinsinden sınırlar: sıfır 64 MiB kullanır, negatif bir değer görselleri devre dışı bırakır. Programlar RGB, RGBA veya PNG görselleri ve Unicode yer tutucu yerleşimleri gönderebilir.

Ekranda ve geri kaydırma (scrollback) alanında eşleşmeleri vurgulamak için `Search(text)`, bunları seçmek için `SearchNext` ve `SearchPrevious`, temizlemek için ise `EndSearch` çağırın. `Options.OnSearch`, `SearchStatus` bildirir; `Theme.Search`, `SearchText`, `SearchSelected` ve `SearchSelectedText` eşleşme renklerini ayarlar. OSC 133 işaretçileri yayan kabuklar, imleci taşımak için etkin komut girişi içinde Alt/Option-tıklamasını destekler. NSGo kabuk tümleştirme betikleri kurmaz.
