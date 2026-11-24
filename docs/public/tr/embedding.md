---
title: Gömme
description: Bir NSGo uygulamasını kendi iOS, Android, masaüstü veya web uygulamanızın içine yerleştirin.
group: Araç zinciri
order: 23
---

## Gömülebilir uygulama derleme

Gömülebilir bir uygulama, bir `Root` ve bir `main` içeren olağan bir uygulama paketidir ve barındırıcının hedefi için `-embed` ile derlenir. Her görünüm uygulamayı bir kez çalıştırır ve kendi başına bir motordur; böylece bir barındırıcı istediği kadar görünümü her biri kendi durumuna sahip olacak şekilde gösterebilir.

```sh
go tool nsgo build -embed -target ios examples/embedded
go tool nsgo build -embed -target android_arm64 examples/embedded
go tool nsgo build -target web examples/embedded
```

iOS derlemesi `<Name>.xcframework` ve `<Name>Resources` oluşturur; Android derlemesi `<Name>.aar` oluşturur; web derlemesi `nsgo.json` ve yükleyiciyi içeren bir dizin oluşturur.

## iOS

Uygulama `base/terminal` paketini içe aktardığında, derleme uygulama çerçevesinin yanına `GhosttyVT.xcframework` de yazar. Hem statik hem de `-dynamic` uygulama çerçeveleri için bu eşlikçiyi barındırıcı hedefine **Embed & Sign** ile ekleyin. Cihaz, Intel ve Apple silicon simülatörü ve Mac Catalyst kütüphanelerini içerir; `GhosttyVT.framework` adını koruyun.

Çerçeveyi uygulama hedefine statik bir kütüphane olarak (gömmeyin) ve kaynak klasörünü bir klasör referansı (folder reference) olarak ekleyin, ardından `import <Name>` yazın. Uygulama başlatılırken ilk görünümden önce `NSGoStart()` işlevini bir kez çağırın. Statik kütüphanenin kodu barındırıcı uygulamanın kendisidir; bu nedenle barındırıcının gizlilik bildirimi (privacy manifest) NSGo'nun kullandığı zorunlu gerekçeli API'leri bildirmelidir: kullanıcı varsayılanları (`CA92.1`), dosya zaman damgaları (`C617.1`) ve sistem önyükleme süresi (`35F9.1`); `-dynamic` bir çerçeve bunları kendi `PrivacyInfo.xcprivacy` dosyasında taşır.

```swift
import Embedded
import UIKit

@main
final class AppDelegate: UIResponder, UIApplicationDelegate {
	func application(_ app: UIApplication, didFinishLaunchingWithOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
		NSGoStart()
		return true
	}
}

let view = NSGoView(frame: .zero)
view.params = ["user": "Ada"]              // cx.HostParams()
view.dataDirectory = documents.path        // where storage lives
view.localeIdentifier = "de-CH"            // in place of the system's
view.setFontScale(1.3)
view.overrideUserInterfaceStyle = .dark
view.tokens = ["accent": "#cf222e"]        // over the app's theme
view.handle("time") { _, reply in reply(Date().description, nil) }
view.send("message", ["text": "Hello"])    // cx.OnHostEvent("message", …)
view.onOpenURL = { url in router.open(url) }
view.onError = { message in log(message) }
view.register(component: "host.badge") { context in Badge(context) }
```

`NSGoView` bir pencereye girdiğinde uygulamayı başlatır ve bellekten silindiğinde ya da `stop()` gönderildiğinde durdurur. Başlatma değerlerini pencereye girmeden önce ayarlayın; geri kalanı her zaman değişebilir. İşleyiciler, bileşenler ve geri çağrılar ana iş parçacığında çalışır ve bir yanıt daha sonra herhangi bir iş parçacığından gelebilir. Bir görünüm denetleyicisi, sistem çubuğu ayarlarını izlemek için görünümün `preferredStatusBarStyle`, `prefersStatusBarHidden` ve `prefersHomeIndicatorAutoHidden` özelliklerini iletebilir.
Barındırıcı ayrıca `mobile.DeferSystemGestures` davranışını iletmek için denetleyicisinden görünümün `preferredScreenEdgesDeferringSystemGestures` özelliğini döndürebilir.

## Android

`base/terminal` paketini içe aktaran bir arm64 uygulaması için AAR, `libnsgo.so` yanında `jni/arm64-v8a/libghostty-vt.so` dosyasını içerir; barındırıcı yerel kütüphane çıkarmayı etkin tutar. Android ön derlemesi ve terminal yükleyicisi [#1452](https://github.com/nsgo-dev/nsgo/issues/1452) gerektirir; x86_64 terminal kütüphaneleri henüz mevcut değildir.

AAR'ı uygulama modülüne ekleyin ve çalışma zamanının bunları bulabilmesi için yerel kütüphaneleri çıkarılmış olarak paketleyin:

```kotlin
implementation(files("libs/Embedded.aar"))

android { packaging { jniLibs { useLegacyPackaging = true } } }
```

Kodda veya bir yerleşimde `dev.oneo.nsgo.NsgoView` görünümleri oluşturun. Her biri ilk kez bir boyut aldığında uygulamayı kendi sürecinde başlatır ve `stop()` onu sonlandırır. Etkinliğin (activity) sonuçlarını her görünüme iletin ve uygulamaların yeniden başlatılmaması için yapılandırma değişikliklerini etkinlikte ele alın.

```java
NsgoView app = new NsgoView(context);
app.setParams(Map.of("user", "Ada"));
app.setStorageDirectory(dir);
app.setLocale(Locale.forLanguageTag("de-CH"));
app.setFontScale(1.3f);
app.setAppearance(NsgoView.APPEARANCE_DARK);
app.setTokens(Map.of("accent", "#cf222e"), null);
app.handle("time", (data, reply) -> reply.resolve(now()));
app.handle("tapped", (data, reply) -> status.setText("tapped " + ((Map<?, ?>) data).get("count")));
app.setOpenUrlHandler(url -> router.open(url));
app.setErrorListener(message -> log(message));
app.registerComponent("host.badge", Badge::new);
```

```java
@Override
protected void onActivityResult(int request, int result, Intent data) {
	super.onActivityResult(request, result, data);
	for (NsgoView app : apps) app.activityResult(request, result, data);
}
```

Go uygulamasını canlı tutarken yerel ağacı ayırmak için barındırıcıda bir `NsgoEngine` tutun ve görünümünü açıkça bağlayın:

```java
NsgoEngine engine = new NsgoEngine(context.getApplicationContext());
app.attachToEngine(engine);
// Before taking the view off screen:
app.detachFromEngine();
// Reuse the same view later:
app.attachToEngine(engine);
// When the host ends the app:
engine.destroy();
```

Ayırma yerel alt öğeleri serbest bırakır; yeniden bağlama, bileşenleri yeniden çalıştırmadan geçerli sinyalleri, girişi, odağı, kaydırma konumunu ve çizimleri geri yükler. Web sayfaları ve oynatıcılar uygulama yapılandırmalarından yeniden oluşturulur. `stop()` yalnızca açıkça sağlanan bir motoru ayırır. Her motor hâlâ ilk görünümünü ve Activity'sini tutar ve başka bir görünümü reddeder; bu nedenle yapılandırma değişikliklerini o Activity içinde ele alın; onu değiştirmek veya tek bir süreçte bağımsız görünümler göstermek henüz desteklenmemektedir.

Google Play güncelleme denetimleri için Android barındırıcısının Gradle bağımlılıklarına `com.google.android.play:app-update:2.1.0` ekleyin. [Çalışma zamanı küçültme kurallarını](https://github.com/nsgo-dev/nsgo/blob/main/internal/platform/android/runtime/play-review.pro) kullanarak NSGo'nun ada göre yüklediği AppUpdateManager, AppUpdateInfo, AppUpdateOptions ve Tasks API'lerini koruyun. Yukarıdaki etkinlik sonucu iletimi güncelleme iptalini de teslim eder.

Gömülü bir barındırıcıdaki Android kimlik doğrulama oturumları için, barındırıcı Activity üzerinde özel geri arama şemasını bildirin ve o Activity'nin kabul ettiği bağlantıları `onNewIntent` içinde (ve yeni bir `onCreate` içinde) `view.openLink(uri.toString())` yöntemine iletin. Eşleşen etkin bir oturum kendi geri aramasını tüketir; diğer bağlantılar `OnOpenLinks` işleyicisine ulaşır. Bağımsız `NsgoActivity` bunları kendisi iletir.

## Masaüstü

Bir Go barındırıcısı platformun olay döngüsünü çalıştırır ve her görünümün `Handle` değerini penceresine koyar. `nsgo.NewView`, tutamaç (handle) bir pencereye girdiğinde uygulamayı başlatır, bu yüzden öncesinde yapılan ayarlar ilk karede yer alır.

```go
channel := core.NewHost()
channel.SetParams(map[string]any{"user": "Ada"})
channel.Handle("time", func(c *core.HostCall) { c.Resolve(time.Now().Format("15:04:05")) })
channel.HandleOpenURL(func(url string, answer func(bool)) {
	answer(strings.HasPrefix(url, "https://example.com/orders/"))
})

view := nsgo.NewView(app.Root, nsgo.HostChannel(channel), nsgo.DataDir(dataDir))
defer view.Close()

view.SetTokens(core.Tokens{Values: map[string]string{"accent": "#cf222e"}})
view.SetLocale("de-CH")
view.SetFontScale(1.3)
view.SetAppearance(core.AppearanceDark)
handle := view.Handle() // an NSView, GtkWidget or child window
```

`view.Host()` uygulamaya olay gönderir ve çağrılarını alır, `nsgo.Component` ise barındırıcının platformu için yerel bir bileşen kaydeder.

## Web

Web derlemesi kendi başına bir sayfadır, ancak bir sayfa onu bir öğenin içine bağlayıp onun barındırıcısı olabilir. `mount(element, url, options)` bir oynatıcı döndürür ve `nsgo.json` yükleyiciyi, wasm dosyasını ve kaynakları kendisine göre adlandırır.

```html
<div id="app"></div>
<script type="module">
const url = new URL("./Embedded-0.1.0-web/nsgo.json", location.href);
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url));

const player = await mount(document.getElementById("app"), url.href, {
	config,
	params: { user: "Ada" },
	host: {
		call(name) {
			if (name === "time") return new Date().toLocaleTimeString();
			throw new Error(`no host function ${name}`);
		},
		post(name, data) { console.log(name, data); },
		openURL(url) { return url.startsWith("https://example.com/orders/"); },
	},
});
player.hostEvent("message", { text: "Hello from the page" });
</script>
```

Bir `host` seçeneği olmadan yükleyici `nsgo.yaml` içindeki `web.host` alanının adlandırdığı modülü içe aktarır ve modül dışa aktarıyorsa `createHost(player)` işlevini bekler (`await`). Bir öğe veya çerçeve içindeki uygulama, tarayıcının geçmişini ve başlığını sayfaya bırakır.

`appearance` (`"light"`, `"dark"` veya `"system"`), `locale` ve `fontScale`, tıpkı yerel bir barındırıcının ayarları gibi tarayıcınınkilerin yerini alır ve oynatıcının `setAppearance`, `setLocale` ve `setFontScale` yöntemleri bunları daha sonra değiştirir. Derlenmiş bir `WebAssembly.Module` veya bunun sözü (promise) olan `module`, uygulamayı birçok kez bağlayan bir sayfanın onu bir kez indirip derlemesini sağlar.

Yükleyici uygulama başlamadan önce bir `progress.nsgo-load-progress` öğesi gösterir. Bunu CSS ile stillendirin, bağlama seçeneklerinde `onProgress` geçirin veya uygulamanın kendi sayfası için `web.host` içinden `onProgress` dışa aktarın. Geri çağrı çubuğun yerini alır ve boyutu bilinmediğinde `total` değeri `null` olacak şekilde kodu çözülmüş bayt cinsinden `{ loaded, total }` alır. Yükleyici, UI'ını temizleyebilmesi için başlangıç bittiğinde onu `null` ile çağırır. Derlenmiş bir `module` indirme ilerlemesini atlar.

```yaml
web:
  host: host.js
```

## Barındırıcı kanalı

`core.Host`, her barındırıcının uyguladığı tek sözleşmedir: başlatma parametreleri, barındırıcının gönderdiği olaylar ve uygulamanın çağırdığı barındırıcı işlevleri. Değerler JSON gibi geçer, bu nedenle eşlemler `map[string]any`, sayılar `float64` olur ve aynı kod Swift, Java, Go ve JavaScript'te çalışır.

- `HostParams` başlatma parametrelerini başlangıçta bir kez okur.
- `cx.OnHostEvent`, `Host.Send` ve sayfanın `hostEvent` çağrılarını dinler.
- `cx.CallHost` adlandırılmış bir işleyiciyi çağırır ve yanıtıyla çözülür; `cx.PostHost` yanıt olmadan bir işleyici çağırır.
- `Host.HandleOpenURL`, `cx.OpenURL` çağrısını platformdan önce görür ve yanıtı platformun da onu açıp açmayacağına karar verir.
- `Host.SetParams`, `Send`, `Handle` ve `HandleDefault` herhangi bir goroutine'den güvenle çağrılabilir.

iOS'ta barındırıcı `params` ayarlar, `handle` ile işleyiciler kurar, `send` ile olay gönderir ve `onOpenURL` ile yanıt verir. Android'de aynısını `setParams`, `handle`, `send` ve `setOpenUrlHandler` ile yapar. Masaüstünde `core.Host` yöntemlerini kullanır ve web'de sayfanın `host` nesnesi yanıt verir.

## Yerel bileşenler

`core.Native` barındırıcının oluşturduğu bir görünümü gösterir: `Props` alanı barındırıcı bileşeninin `SetProp` yöntemine ulaşır, bileşenin yaydıkları `OnEvent`'a ulaşır ve değeri canlandırılmış bir düğüm olan bir özellik motor için iş oluşturmadan kare saati üzerinde hareket eder.

```go
core.Native{
	Component: "host.badge",
	PropsFunc: func() map[string]any { return map[string]any{"count": count.Get()} },
	OnEvent:   func(name string, data any) { if name == "tap" { count.Set(0) } },
	Style:     core.Style{MinWidth: core.Pt(120), MinHeight: core.Pt(28)},
}
```

Bileşeni barındırıcıda kaydedin: iOS'ta `app.register(component:)` ve bir `NSGoComponent`, Android'de `registerComponent` ve bir `NsgoComponent`, masaüstünde `nsgo.Component`. Web'in yerel bileşenleri yoktur, bu yüzden bilinmeyen bir ad orada boş bir kutu gösterir.

## Ayarlar

`tokens`, uygulamanın temasını her iki görünümde veya birinde rol adına (`accent`, `inset`, `raised`) göre geçersiz kılar; `cx.Palette()` bunları görür. Barındırıcı ayrıca yerel ayarı, metin boyutunu, görünümü ve veri dizinini ayarlar ve uygulamanın hatalarını dinler. `nsgo build -embed` pencere, menü veya tepsi simgesi yazmaz: bunlar barındırıcıya aittir.

Bir görünümün motor goroutine'indeki dahili bir panik o motoru durdurur, bağlı yüzeylerini yok eder ve paniği ile yığını barındırıcının hata dinleyicisine (Go'da `nsgo.OnError`) bildirir. Diğer motorlar çalışmaya devam eder; başarısız olan uygulamayı yeniden başlatmak için yeni bir görünüm oluşturun. Sıradan uygulama geri çağrısı panikleri yine önce en yakın `core.Catch`'e gider.

`examples/embed-ios`, `examples/embed-android`, `examples/embed-desktop` ve `examples/embed-web` her barındırıcı türüyle aynı uygulamadır.

## Sonraki adımlar

- [Platform servisleri](services.md): uygulamanın platformdan istedikleri.
- [Derleme ve yayınlama](building.md): `-embed` seçeneğinin oluşturduğu paketler ve bunları imzalama.
- [Örnek: Yapılacaklar uygulaması](tutorials/todo.md): bir barındırıcının gömdüğü uygulama.
