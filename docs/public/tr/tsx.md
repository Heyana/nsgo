---
title: TSX uygulamaları
description: Tailwind sınıflarıyla TSX'te bir uygulama yazın; Node olmadan önceden derlenir.
group: Temel kavramlar
order: 63
---

## Bir TSX uygulaması

Bir TSX uygulaması, kaynaklarını gömen ve giriş noktasını çalıştıran bir Go paketidir:

```go
//go:embed *.tsx nsgo.yaml
var src embed.FS

var Root = compiler.App(src, "App.tsx")

func main() { nsgo.Run(Root) }
```

```tsx
import { mount, signal, Text, View } from "nsgo";

function App() {
  const count = signal(0);
  return (
    <View class="gap-3 p-6" onPress={() => count.update((n) => n + 1)}>
      <Text class="text-lg font-bold">Pressed {count()} times</Text>
    </View>
  );
}

mount(App);
```

`go tool nsgo init -template tsx` bir tane yazar, `examples/tsx` de başka bir örnektir. `nsgo dev` ve `nsgo build` TSX'i önceden derler, bu nedenle paketler bir derleyici taşımaz; `go run` ve `go test` ise onu uygulama başlarken derler. Her iki durumda da Node gerekmez.

## Bileşenler ve modüller

TSX temel yapı taşlarını `nsgo`'dan, `base`'in bileşenlerini `nsgo/base`'den, oyun kumandalarını `nsgo/gamepad`'den (imzalı paketlerde `gamepad` yeteneği altında), sesleri `nsgo/audio`'dan (`audio` altında) ve uygulama `tsx/ui`'ı içe aktardığında `ui`'ın bileşenlerini `nsgo/ui`'dan içe aktarır; `examples/gallery-tsx` bunları gösterir.

Mermaid diyagramları için, `tsx/textview` veya `tsx/ui` ile birlikte Go ana uygulamasında yan etkisi için `github.com/nsgo-dev/nsgo/tsx/diagram`'ı içe aktarın. Bu, diyagram ayrıştırıcısını onu kullanmayan uygulamalara bağlamadan `nsgo/base`'e `Diagram` ekler. `source`, `fontSize`, kısmi bir `theme`, `accessibilityLabel` ve olağan stil prop'larını alır. Onu doğrudan kullanın ya da bir TextView çizicisi sağlayın:

```tsx
import { Diagram, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderDiagram={(code) => <Diagram source={code.code} errorClass="font-mono" />} />
```

Aynı `renderDiagram` prop'u `nsgo/ui`'ın TextView'inde de çalışır. Mermaid olmayan bir kod bloğu olağan çizicisini korur; geçersiz bir diyagram kaynağını gösterir.

Dizilmiş matematik için, `tsx/textview` veya `tsx/ui` ile birlikte Go ana uygulamasında yan etkisi için `github.com/nsgo-dev/nsgo/tsx/formula`'yı içe aktarın. Bu, `nsgo/base`'e `Formula` ekler; paketi dışarıda bırakan uygulamalar matematik motorunu ve yazı tipini ikili dosyalarının dışında tutar. `source`, `dialect` (varsayılan olarak `tex` ya da `typst`), `display`, `accessibilityLabel` ve stil prop'larını alır. Yazı tipi boyutu ve rengi çevredeki metni izler. Onu doğrudan kullanın ya da bir TextView çizicisi sağlayın:

```tsx
import { Formula, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderMath={(math) => <Formula source={math.tex} display={math.display} />} />
```

Aynı `renderMath`, `nsgo/ui`'ın TextView'inde de çalışır. Formüller kopyalandığında TeX'lerini korur. Go `Renderer`'ı bir ana uygulama yapılandırması olarak kalır.

`UrlScheme.url()` izlenen uygulama adresini okur; `UrlScheme.setUrl(url, { history: "push" })` bir adres girdisi ekler ya da seçenekler verilmediğinde onu değiştirir. `UrlScheme.on("change", fn)` tarayıcının geri, ileri ve hash değişikliklerini gözlemler ve bir abonelikten çıkma fonksiyonu döndürür. Yönlendirme, aynı köken doğrulaması ve derin bağlantı barındırma için [Uygulama adresleri](addresses.md) sayfasına bakın.

## TSX'ten Go

TSX'in içe aktarması için Go'yu kaydedin:

```go
tsx.Register("app/native", map[string]core.Node{"Badge": Badge{}})
tsx.RegisterFunctions("app/native", map[string]any{
	"greet": func(name string) string { return "Hello, " + name },
})
```

Ardından `import { Badge, greet } from "app/native"`, `Badge`'in alanlarını prop olarak alır ve `greet`'i çağırır. `context.Context` alan fonksiyonlar promise'lere dönüşür ve `tsx.RegisterClass` Go türlerini TSX'in `new` ile oluşturduğu sınıflara dönüştürür.

`undefined` olarak iletilen isteğe bağlı bir prop varsayılanını kullanır. Varsayılanı türünün sıfır değerinden farklı olan bir getter'a sahip bir Go bileşeni kaydederken `tsx.Default("Max", float64(100))` gibi bir seçenek ekleyin; aynı varsayılan, reaktif bir prop undefined döndürdüğü sürece de bileşeni değiştirmeden uygulanır.

Belgelenmiş Go varsayılanlarına sahip veri alanları da atlanabilir: bir `Flow` düğümünün `width` ve `height` değerleri yokken JSX içeriğini ölçer, bir kenarın atlanan `width` değeri varsayılan çizgiyi kullanır ve bir kota satırının atlanan `pace` değeri tempo işaretini gizli bırakır.

Go bileşenlerini ve sınıflarını kaydetmek, dışa aktarılan alanlarını ve metotlarını uygulama çalışırken yansıma (reflection) ile bulur. Bu arama, Go bağlayıcısının hiçbir şeyin çağırmadığı dışa aktarılmış metotları atmasını engeller; bu yalnızca TSX'in kaydettikleri için değil tüm uygulama için geçerlidir; bu nedenle `tsx`'i bağlamak, sembolleri ayıklanmış bir masaüstü ikili dosyasına yaklaşık 20 MB ekler, uygulama ne kadar çok paket bağlarsa o kadar fazla.

## İmzalı paketler

Bir uygulama, paketi doğruladıktan sonra indirdiği TSX'i çalıştırabilir; örneğin yeniden teslim edilen kendi ekranlarını veya eklentileri. Paketin kodunun nelere erişebileceğini `nsgo.yaml` dosyasında bildirin; geri kalan her şey ona reddedilir:

```yaml
capabilities:
  network:
    hosts: [api.example.com, "*.cdn.example.com"]
  open_url: true # or { schemes: [https] }
  host: [search, "player.*"] # the app's handlers it may call
  # webview: { hosts: [...], input: true, media: [microphone, camera] }, clipboard, picker, speech, text_to_speech, status_bar, notifications, biometrics, sign_in_with_apple, purchases, secrets, audio, gamepad, auth_session
```

Bir kök anahtarı çevrimdışı tutun ve sürüm anahtarlarını onunla sertifikalandırın, ardından uygulamayı o dizinde imzalayın. `go tool nsgo keygen` bir anahtar çifti yazdırır ve hiçbir dosya yazmaz; onu kök için bir kez ve her sürüm anahtarı için bir kez çalıştırın. Açık anahtarlar bayraklara ve uygulamanın koduna girer; gizli anahtarlar depo dışında kalır ve araca yalnızca `NSGO_ROOT_KEY` ve `NSGO_RELEASE_KEY` aracılığıyla ulaşır:

```sh
go tool nsgo keygen # once for the root, once for the release key
NSGO_ROOT_KEY=<root secret> go tool nsgo certify -key <release public> -apps 'com.example.*'
NSGO_RELEASE_KEY=<release secret> go tool nsgo publish -cert release.cert plugin
go tool nsgo verify -root <root public> plugin/dist/Plugin-1.0.0.nsgopkg
```

Uygulama, kodunda yazılı olan kökün açık anahtarına güvenir, paketi motorun goroutine'i dışında doğrular ve onu kendi görünümlerinin yanında, korumalı alanda (sandbox) `tsx.View` içinde gösterir. Paket uygulamaya yalnızca görünümün ona verdiği ana uygulama kanalı aracılığıyla ulaşır ve uygulamanın depolamasından ve gizli bilgilerinden kendi payını korur:

```go
trust, err := pkg.OpenTrust(filepath.Join(cx.DataDir(), "trust"))
trust.AddRoot(root)
p, err := trust.Verify(data) // refuses tampered, revoked and older builds
channel := core.NewHost()
channel.Handle("search", func(c *core.HostCall) { c.Resolve(results(c.Data)) })
view := tsx.View{Package: p, Sandbox: tsx.Sandbox{Host: channel}}
core.Catch{Try: view.Render, Fallback: failed}
trust.Accept(p) // once it ran, older builds are refused
```

`Sandbox.Grant` ona bildirdiğinden daha azını verir, `Sandbox.Limits` başka zaman bütçeleri ve kotalar, `Sandbox.Modules` ise NSGo'nun kendilerinin ötesinde Go fonksiyonları ve bileşenleri verir. Klavye kısayolları, geri işleyicileri ve gezinme yığınları yalnızca klavye odağı kendi görünümündeyken etkili olur ve odağı kendisi (`focus()`, `autoFocus`, `Keyboard.dismiss`, `setAccessibilityFocus`) yalnızca odak zaten oradayken ya da kullanıcının o görünümdeki basışına yanıt olarak taşır. Ebeveyninin ona verdiği alanı dolduran, daha fazlasını değil, kendine ait bir görünümde gösterilir ve menüler, açılır pencereler ve bildirim balonları dahil olmak üzere kırpılır; bir kaydırma görünümünde, içeriğinin yüksekliğini alabilmesi için onu bir görünümün içine koyun. Ona ait bir iletişim kutusu, uyarı, bağlam menüsü veya duyuru yalnızca aynı şekilde gösterilir ve geri, onu kaldırmasa bile ona ait bir iletişim kutusunu kapatır. `View.Export`, paketin giriş modülünün dışa aktardığı bir bileşeni adlandırır; bu bileşen, girişin bağladığı kökün yerine gösterilir (girişin `mount`'u bu durumda ne çizer ne de bileşenini çalıştırır) ve ona her görünüm için kopyalanan, JSON olarak kodlanabilen bir eşlem olan `View.Props` verilir; böylece tek bir paket, aynı yetenekler ve sınırlar altında, her biri kendi durumuna sahip birkaç görünüm sağlar; eksik bir dışa aktarım `tsx.ErrNoViewExport` ile, fonksiyon olmayan bir dışa aktarım ise `tsx.ErrViewExportNotComponent` ile panic yapar. Girişi fonksiyonlar dışa aktaran bir paket, bir fonksiyon eklentisidir: `tsx.Load(cx, p, tsx.Sandbox{})` onu görünmeden ve eşzamanlı olarak çağıranın goroutine'inde, bir görünüm ağacına ait bir `cx` altında çalıştırır (ağaç yoksa `tsx.ErrNoTree` döndürür) ya da giriş modülünün yüklenirken fırlattığı şeyi döndürür; `Plugin.Call` ise bunları çağırır. `go tool nsgo dev -limits` bir uygulamayı böyle bir görünümün yapacağı gibi çalıştırır ve `go tool nsgo revoke`, `Trust.UpdateRevocations` için iptal edilmiş sürüm anahtarlarının ve paketlerin bir listesini imzalar. `examples/plugins` kullanıcıdan bir paketin yeteneklerini vermesini ister, onu kurar ve çalıştırır. Varsayılan olarak 64 MiB olan `Limits.Memory`, diziler, dizeler, arabellekler ve türlü diziler (typed arrays) için ölçülen yerel depolamayı bütçeler. Buna `concat`, `flat`, `slice`, dizi yayma (spread), dize birleştirme, şablonlar, `replace`, `replaceAll`, `TextEncoder` ve `Blob` kopyaları dahildir. `-limits` içindeki `memory=<bytes>` bütçeyi değiştirir ve `memory=0` onu kapatır. Denetimler ayırmadan önce çalışır. Bütçeyi aşan bir paket, Go toplanabilir depolamayı denetledikten sonra `ERR_NSH_QUOTA_EXCEEDED` ve `memory` kaynağıyla durur. Sıradan nesneler, kapanışlar (closure), yığınlar, küçük ayırmalar ve ölçülmeyen yerleşik öğeler ile ana uygulama API'leri de bellek ayırabilir. Tarayıcı kendi yığınını (heap) tutar. Paketleri `pkg.Build` ile ya da `go tool nsgo publish -extra <file>` ile o dosyanın baytlarıyla kendisi imzalayan bir uygulama, `pkg.Contents.Extra` içine kendi meta verisini koyabilir; örneğin bir eklentinin menülerine ne eklediği: bu paketle birlikte imzalanır ve `Verify` onu uygulamanın ayrıştırması için `p.Extra` olarak değiştirmeden döndürür; böylece uygulama onu paketin kodunu çalıştırmadan okur.

Bir uygulama, eklentilerinin bir kataloğu gibi kendi JSON belgelerini `nsgo keygen`'in yazdırdığı anahtarlarla `pkg.SignDocument` ile imzalayabilir ve `Signed.Open` ile doğrulayabilir. Her belge türüne, `app:catalog-v1` gibi sürüm içeren kendine ait bir `app:` bağlamı verin: bir belge asla başka bir tür yerine, ne de bir paket, sertifika veya iptal listesi yerine geçmez. `Open` yalnızca kendisine verilen açık anahtarlara güvenir, hiçbir sertifika veya iptal listesi kullanmaz ve içine çözümlediği türde bulunmayan gövde alanlarını reddeder. Zarf hiçbir zaman veya sıra bilgisi tutmaz, bu nedenle eski bir belge yenisi kadar iyi doğrulanır: gövdeye bir sürüm veya son kullanma tarihi koyun ve onu denetleyin.
Yerel bir korumalı alanda `JSON.stringify`, reviver ile `JSON.parse`, `Array.prototype.join`, `toString`, `toLocaleString` ve `flat` ve dolayısıyla bir dizinin `String()`'i, 1.000 düzeyden daha derine indiklerinde yakalanabilir bir `RangeError` fırlatır; iç içe çağrılar birlikte sayılır, tıpkı bir dizinin `toString`'i ve onun çağırdığı `join`'in her birinin sayılması gibi; daha derin veriler çağrıyı askıda bırakır ya da uygulamayı sonlandırırdı. Web, tarayıcının yerleşik öğelerini kullanır.

Yerel hedeflerde `DefaultLimits.Stack` 1.000 çerçevedir. 10.000 çerçeveye kadar farklı bir derinlik seçmek için `Limits.Stack` veya `-limits stack=<frames>` ayarlayın. Sıfır veya altındaki Go değerleri ve tavanın üstündeki girdiler 10.000 kullanır. Web, tarayıcının yığın sınırını kullanır.

Birbirini çağıran yerel fonksiyonlar, JS çağrılarının yaptığı gibi `Limits.Stack`'e sayılır: yerleşik bir getter, `toString` veya `valueOf`, `exec`'i kendi metotlarından bir başkası olan bir düzenli ifade, birbiri içindeki `Reflect.apply` ve `Reflect.construct` ve bağlı fonksiyon ya da proxy zincirleri, sınırı aştığında uygulamayı sonlandırmak yerine yakalanabilir bir `RangeError` fırlatır; ondan daha derin iç içe geçmiş bir eklenti değeri, onu Go'ya dönüştüren çağrıyı bir `RangeError` ile başarısız kılar. Bu, NSGo'nun gerektirdiği goja çatalı olan `github.com/nsgo-dev/goja`'dan gelir; uygulamanız onu `go.mod` dosyasında herhangi bir değişiklik yapmadan NSGo ile birlikte alır.

Yerel hedeflerde `Limits.Call`, zamanlayıcı geri çağrıları ve `await` sonrasındaki promise devamları dahil her JavaScript diliminin sınırını belirler. Bir dilim kesildiğinde, bekleyen bir `Plugin.Call` bir kez `tsx.ErrBudget` bildirir, kalan zamanlayıcılarını durdurur ve geç gelen ana uygulama yanıtlarını yok sayar. Kesintiye uğrayan dilimde kuyruğa alınmış promise tepkileri dahil bağımsız çağrılar çalışmaya devam eder. Başka bir çağrının promise'ini sonuçlandıran bir çağrı yine kendi sonucunu veya hatasını bildirir. Geri izlemeli (backtracking) düzenli ifadeler, eşleşme zaman aşımı olarak yükleme veya çağrı bütçesinden kısa olanını kullanır; işlenmeyen bir zaman aşımı da `tsx.ErrBudget` olur.

`Limits.CallTotal`, her eklenti çağrısı ve korumalı alandaki görünüm olayı için bir duvar saati son tarihi ekler. Beklemede geçen süreyi içerir ve çağrı sonuçlandıktan sonra bile zamanlayıcılarını, aralıklarını, karelerini, promise devamlarını ve ana uygulama yanıtlarını bu sınıra tabi tutar. Son tarihte bekleyen iş bırakılır ve hâlâ bekleyen bir çağrı `tsx.ErrBudget` ile başarısız olur. Giriş modülünün işi `Limits.Load` ile sınırlanır; `Load` sıfırsa `CallTotal` kullanılır. Bağımsız ana uygulama olayları, teslimden itibaren `CallTotal` kullanır. Bekleyen bir çağrının son tarihine bir tanıtıcı almak için `Plugin.CallWithBudget` kullanın. Bir ana uygulamanın `Scope.Post`'unda, yalnızca bu çağrıya daha fazla zaman vermek için o son tarihten önce `budget.Extend(duration)` çağırın.

`Plugin.Interrupt` çalışma zamanını herhangi bir goroutine'den durdurur, bekleyen zamanlayıcılarını bırakır ve mevcut ve sonraki çağrıları başarısız kılar. `Plugin.Stopped()` ve `Plugin.Err()` de herhangi bir goroutine'den okunabilir. `Err()` çalışma zamanı çalışırken nil'dir ve durduğunda ilk durma nedenini döndürür. Eklentiyi kapatmak veya sahibini elden çıkarmak normalde temizleme işlemlerini çalıştırır ve onu `tsx.ErrPluginClosed` ile durdurur. Bekleyen çağrılar başarısız olduktan sonra bu nedeni motorun goroutine'inde bir kez almak için `tsx.Load` çağırırken `Sandbox.OnStop` ayarlayın.

Bekleyen bir `Plugin.Call` dışındaki yakalanmamış zamanlayıcı, animasyon karesi, `queueMicrotask` ve `reportError` hatalarını almak için bir fonksiyon eklentisi yüklerken `Sandbox.OnError` ayarlayın. Her eklentinin işleyicisi motor goroutine'inde çalışır; böylece bir ana uygulama hataları ilişkilendirebilir ve devre kesicisi açıldığında `Plugin.Interrupt` çağırabilir. Yerel dilim bütçesi kesintileri de, çağrısı zaten sonuçlanmış bir promise işi dahil, `tsx.ErrBudget` bildirir. Sıradan geri çağrı hataları eklentiyi çalışır durumda bırakır. Yükleme ve çağrı sonucu hataları yine çağıranlarına döner ve görünümler yine `core.Catch` kullanır; bir işleyici olmadan mevcut sınır ve uygulama raporu davranışı korunur.

Her görünümün `*tsx.ViewRuntime`'ını giriş modülü çalışmadan önce motor goroutine'inde almak için `Sandbox.OnViewRuntime` ayarlayın. İşini durdurmak ve son çizilen görünümü yerinde bırakmak için herhangi bir goroutine'den `Interrupt` çağırın. `Stopped` ve `Err` son durumunu bildirir; normal elden çıkarmada `tsx.ErrViewClosed` olur. `Sandbox.OnStop` da bir görünümün son durma nedenini bir kez bildirir.

`Plugin.JSTime` ve `ViewRuntime.JSTime`, geçerli giriş dahil, yerel çalışma zamanının JavaScript girişlerinde geçen süreyi bildirir. Girişler arasındaki bekleme ve kayıtlı eşzamanlı Go fonksiyonlarının içindeki süre hariç tutulur. `ViewRuntime.EngineTime` ayrıca eşzamanlı Go çağrılarını ve hâlâ çalışmakta olanlar dahil, sahip olunan reaktif, bağlama, işleme (commit) ve yerleşim işlerini de içerir. Motor girişleri arasındaki beklemeyi, ana uygulamaya veya başka bir görünüme ait işleri, paylaşılan işleme kayıt tutmayı ve yerel çizimi hariç tutar. Her iki sayaç da herhangi bir goroutine'den güvenle okunabilir. Tarayıcının NSGo dışındaki JavaScript turları ölçülemez veya kesilemez. Orada her iki sayaç da kullanılamaz olarak bildirilir ve `Interrupt`, ana uygulama denetimi yeniden kazandığında gelecekteki NSGo geri çağrılarını durdurur.

Web henüz bir paketi sayfadan yalıtamaz: kodu, yeteneklerinin ötesinde, sayfanın kendi ağına, panosuna ve güven deposununki dahil depolamasına erişirdi. Orada `tsx.View` ve `tsx.Load`, `Sandbox.WebUnisolated` ayarlayarak onu sayfanın yetkisiyle çalıştırmadığınız sürece bir paketi `tsx.ErrUnisolated` ile reddeder; bu yalnızca uygulamanın kendi yetkisiyle güveneceğiniz paketlere uygundur.

## Düzenleyiciler ve testler

`.nsgo/tsconfig.json`'ı genişleten bir `tsconfig.json`, düzenleyicilere TSX'in türlerini verir; bunları `go tool nsgo dev` ve `go tool nsgo check` yazar. Uygulamanın yanında `nsgo/test` ile yazılmış `*.test.tsx` dosyaları, uygulamanın Go testleriyle birlikte çalışır ([Test etme](testing.md)).
