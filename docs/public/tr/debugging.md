---
title: Çalıştırma ve hata ayıklama
description: "Bir uygulamayı herhangi bir hedefte sıcak yeniden yüklemeyle çalıştırın; hatalarını, güncellemelerini, yerleşimini, hızını ve diğer dillerini görün."
group: Araç zinciri
order: 20
---

## Çalıştırma

```sh
go tool nsgo dev
```

`dev` uygulamayı bu makine için veya `nsgo.yaml` içindeki `dev.target` alanının adlandırdığı hedef için derler ve açar. Bir Go dosyası her kaydedildiğinde uygulamayı yeniden derler ve yeniden başlatır; uygulama `go.mod` veya `go.sum` dosyalarının eksik olduğu bir modülün paketini içe aktardığında önce `go mod tidy` çalıştırır; yalnızca TSX değiştiğinde ise çalışan uygulamayı durumunu koruyarak yerinde günceller. Çalıştırma uygulamayla birlikte sonlanır.

macOS ve Windows'ta yerel bir dev yeniden derlemesi, ana görünümün penceresini geçerli oturumdaki son normal konumunda ve boyutunda yeniden açar. Windows tam ekranı da geri yükler. Linux'ta GTK normal boyutunu geri yükler ve son tam ekran modunu ister. Konumu pencere yöneticisi seçer.

`-target` uygulamayı başka bir yerde çalıştırır ve `-device` iPhone, Apple TV, Apple Watch, simülatör veya Android cihazını ad, UDID ya da seri numarasıyla seçer:

```sh
go tool nsgo dev -target tui                # terminal cells; diagnostics in .nsgo/tui.log
go tool nsgo dev -target web                # in the browser, at the address it prints, on -port (8080 by default)
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target ios                # on the connected iPhone, signed by Xcode
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator, over adb
go tool nsgo dev -target miniprogram_wechat # in WeChat DevTools (also _qq, _alipay, _douyin)
```

Geliştirme sunucusu varsayılan olarak tüm arayüzlerde dinler. Android'in `adb reverse` üzerinden bağlandığı durumlar dahil sunucuyu geri döngüde (loopback) tutmak için `-host 127.0.0.1` kullanın veya yalnızca orada dinlemek için yerel bir arayüzün IP adresini belirtin. Fiziksel Apple cihazlarında geliştirici özellikleri için LAN üzerinden erişilebilen bir adres kullanın.

`-h` her bayrağı listeler.

## Go hata ayıklayıcısı

Go sürümünüzle uyumlu bir [Delve](https://github.com/go-delve/delve/blob/master/Documentation/installation/README.md) sürümünü `PATH` üzerinde `dlv` olarak bulundurun. Yerel bir masaüstü uygulamasını hata ayıklama bilgileriyle ve Go iyileştirmesi devre dışı bırakılmış olarak başlatın.

```sh
go tool nsgo dev -target native -debug
```

Delve'nin `API server listening at` satırındaki bağlantı noktasını kopyalayın. Go eklentili VS Code'da bir bağlanma (attach) yapılandırması ekleyin ve `40000` değerini o bağlantı noktasıyla değiştirin.

```json
{
  "name": "Attach to NSGo",
  "type": "go",
  "request": "attach",
  "mode": "remote",
  "debugAdapter": "dlv-dap",
  "host": "127.0.0.1",
  "port": 40000
}
```

GoLand'da `127.0.0.1` ana bilgisayarı ve aynı bağlantı noktasıyla bir **Go Remote** çalıştırma yapılandırması ekleyin. Uygulamanızın Go kodunda bir kesme noktası koyun, bağlanın ve ona ulaşan eylemi çalıştırın. Başarılı bir Go yeniden derlemesinin ardından yeni sürece kesme noktalarını göndermesi için hata ayıklayıcıyı aynı adresten yeniden bağlayın. TSX değişiklikleri hâlâ sıcak yeniden yüklemeyi kullanır.

`-debug`, `-target native` ile macOS, Windows ve Linux'ta çalışır. Hata ayıklama için bir dev derlemesi kullanın ve `-release` seçeneğini bir yayın derlemesini ölçmek için saklayın.

## Bileşen önizlemeleri

İçe aktarılabilir bir Go paketinde bileşenlerin yanına bir veya daha fazla `PreviewXxx` işlevi koyun. Bunları önizleme derleme etiketiyle korunan bir Go dosyasına kaydedin.

```go
//go:build nsgopreview

package components

import "github.com/nsgo-dev/nsgo/core"

func PreviewEmpty(cx *core.Scope) core.Node {
    return core.Text{Content: "No items"}
}
```

```sh
go tool nsgo dev -preview ./components
go tool nsgo dev -target web -preview ./components
```

Birlikte görüntülemek istediğiniz test düzeneklerini (fixtures) işaretleyin ve bir yerel ayar, tema, açık veya koyu görünüm, görünüm alanı boyutu ve yerleşim yönü seçin. Önizleme en yakın uygulamanın `nsgo.yaml` dosyasını, çeviri kataloglarını, yapılandırılmış `theme` değerini ve kaynaklarını alır. Aynı modülde farklı bir yapılandırma seçmek için bayraklardan sonra bir uygulama dizini geçirin. Bu durumda önizleme yolu uygulama dizinine görelidir.

Bir dosyayı kaydetmek önizlemeyi yeniden derler ve seçiminizi ile görüntüleme ayarlarınızı korur. `nsgopreview` etiketi test düzeneklerini olağan yayın derlemelerinin dışında tutar. Yerel bir ekran görüntüsü almak için aynı düzeneği etiketli bir `_test.go` dosyasında `nsgotest.Start(t, PreviewEmpty)` ile kullanın ve `GOFLAGS=-tags=nsgopreview go tool nsgo test -target macos ./components` komutuyla çalıştırın.

`-target ios_simulator` ile ana uygulamanın ilk kurulumundan sonra, yalnızca Go içeren bir yeniden derleme kurulu çalıştırılabilir dosyayı değiştirir ve uygulamayı yeniden başlatır. `nsgo.yaml`, paketlenmiş kaynaklar veya uygulama simgesindeki düzenlemeler yeniden kurulumu tetikler.

Daha kısa bir Go yeniden derleme döngüsüyle Android UI çalışması için `go tool nsgo dev -target android -android-host` kullanın. Araç yerel çalışma zamanını bir kez kurar ve Go'yu geliştirme makinesinde çalıştırır. Her Go yeniden derlemesi adb üzerinden aynı Activity'ye yeniden bağlanır. Paketlenmiş kaynaklar, önceden derlenmiş sınıflar, başlatıcı simgeleri veya `nsgo.yaml` değişiklikleri çalışma zamanı APK'sını yeniden kurar. Go süreci ana bilgisayarın derleme kısıtlarını, dosyalarını ve ağını kullanır. `Scope.DiskSpace`, ana bilgisayarın `DataDir` yolunu içeren birimi bildirir. `_android.go` dosyalarını ve cihaz içi Go davranışını denemek için olağan Android dev komutunu kullanın.

## Hatalar

Başarısız bir derleme, çalışan uygulamanın üzerinde hatayı kopyalama ve yeniden derleme düğmeleriyle gösterilir; dosyayı düzeltin, bir sonraki kaydetme onun yerini alır. İşlenirken panik yapan bir bileşen de aynı şekilde gösterilir. Uygulama `cx.OnKey` ile veya uygulama menüsünün etkin bir öğesi olarak tuşu kendisi bağlamadığı sürece uygulamadaki Command+R (Apple platformları dışında Control+R) hata ekranının üzerinde bile her zaman yeniden derleyip başlatır; tarayıcıda bu tuş her zamanki gibi sayfayı yeniler. Uygulamanın `log.Print` gibi kendi çıktıları terminalde aracın satırlarının altında görünür.

Çalıştırmadan önce `go tool nsgo check` uygulamayı denetler (`go vet`) ve TSX'ini tür denetiminden geçirir; `-target all` her hedef için denetler, böylece başka bir platformun derleme etiketleri arkasındaki kodlar da bir Mac'ten denetlenir.

### Paketlenmiş Apple uygulamalarının günlükleri

macOS, iOS, tvOS ve watchOS'ta varsayılan Go günlükçüsü `log.Print`, NSGo tanılamaları ve TSX konsol çıktısı dahil Apple'ın birleşik günlüğüne yazar. Console uygulamasında alt sistem (subsystem) olarak çalışan uygulamanın paket tanımlayıcısına, kategori olarak `nsgo` değerine göre filtreleyin. macOS'ta depoyu da sorgulayabilirsiniz:

```sh
log show --last 10m --predicate 'subsystem == "dev.example.hello"'
```

Kayıtlar varsayılan günlük türünü kullanır ve uyarı veya hata ön eklerini korur. Metinleri herkese açıktır; bu nedenle kimlik bilgilerini ve diğer gizli verileri günlüklerden uzak tutun. Uzun iletiler birkaç girdi haline gelir. Geliştirme sırasında terminal çıktısı kullanılmaya devam eder; özel bir günlük yazıcısı veya `slog` işleyicisi kendi çıktısını denetler.

### Linux web görünümleri

Ubuntu 24.04 ve sonrası yetkisiz kullanıcı ad alanlarını AppArmor üzerinden kısıtlayabilir. bubblewrap WebKitGTK'nın sanal alanını (sandbox) başlatamadığında NSGo hatayı günlüğe yazar ve bir yer tutucu gösterir. Bazı WebKitGTK sürümleri ilk web görünümünü açarken aksi halde `SIGTRAP` ile çıkar. [Ubuntu AppArmor belgeleri](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/)

Dağıtımınız `bwrap-userns-restrict` gibi bir bubblewrap AppArmor profili sağlıyorsa onu kullanın. Aksi halde bir yönetici, başlangıç noktası olarak [NSGo'nun Linux CI'ındaki profili](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/test.yml) kullanarak `/usr/bin/bwrap` için `userns` izni veren bir profil yükleyebilir. Profil sistemin bubblewrap'ini kullanan her uygulama için geçerli olduğundan yönetici tarafından yönetilir.

`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1`, WebKit'in alt süreç sanal alanını kapatır. Yalnızca güvenilen içerikle yerel geliştirme için kullanın.

## DevTools

```sh
go tool nsgo dev -devtools
```

`-devtools` her hedef (bir telefon uygulaması dahil) için `nsgo dev` tarafından sunulan DevTools'u tarayıcıda açar. Çalışan uygulamayı çalıştığı sırada gösterir:

- **Elements**: bir ağaç içindeki görünümler. Her biri kendisini oluşturan bileşeni, düzenleyicinizdeki satırına bir bağlantıyı, bir testin onu bulduğu seçiciyi, sınıflarını, sınırlarını, yerleşimi yapılmış dış boşluk, kenarlık ve iç boşluğunu ve ayarladığı stili gösterir. Bir satırın üzerine gelmek, tıpkı bir tarayıcının DevTools'u gibi uygulamada onun dış boşluğunu, iç boşluğunu ve içeriğini çizer ve **Pick** ağaçta bulmak için herhangi bir pencere, panel, sayfa veya gömülü görünümdeki bir görünüme basmanızı sağlar.
- **State**: kodunuzun oluşturduğu sinyaller; sahip olan bileşenlerin ve hesaplamaların altında, değerleri, memo değerleri ve her birini kaç hesaplamanın okuduğuyla birlikte. Hiçbir hesaplamanın okumadığı bir değer ekranda hiçbir şeyi değiştirmez. Dizeler, boole değerler ve sayılar düzenlenebilir; böylece kod değişikliği yapmadan bir durumu deneyebilirsiniz. NSGo'nun kendisi için oluşturduğu değerler **NSGo's own** altındadır.
- **Profiler**: her işlemenin (commit) süresi; öncesindeki kendi kodunuz, yerleşimiyle birlikte işleme ve platformun ana iş parçacığında uygulanması olarak bölünür. Ayrıca yazmaları işlemeye neden olan satırları da gösterir ve çalıştırdığı hesaplamalar üzerinde en uzundan başlayarak açılır. İkinci bir liste son bir saniyede en çok çalışan hesaplamaları onları oluşturan satıra göre adlandırır. Android'in çalışma zamanı toplu işleri kendi Java kodunda uygular, bu yüzden Android işlemelerinin platform süresi yoktur.
- **Recorder**: uygulamada bastığınız, yazdığınız, temizlediğiniz ve kaydırdığınız her şey; bir `_test.go` dosyasına kopyalanacak bir `nsgotest` test dosyası olarak yazılır. Elements içinde **Assert shown** ve **Assert text** seçili görünüm için bir denetim ekler. Bir metin denetimi metni uygulamanın gösterdiği dilde tutar, bu nedenle testlerinizin çalıştığı dilde kayıt yapın.
- **Highlight updates** tıpkı `-highlight` gibi bir işlemenin değiştirdiği her görünümün dış çizgisini çizer.
- **Layout** `-layout` bayrağının başlattığı gibi cihazdaki [yerleşim denetimini](#yerleşim) gösterir ve gizler.

Bir TSX uygulamasının durumu, hesaplamaları ve yazmaları paketlerin kaynak haritaları aracılığıyla TSX satırlarıyla adlandırılır ve uygulamanızdaki dosyalara bağlanır.

DevTools uygulamayı, telefonların ulaşabilmesi için ağınızda dinleyen `nsgo dev` sunucusu üzerinden okur. Her çalıştırma bir belirteç (token) oluşturur; `nsgo dev`'in DevTools ve web hedefi için yazdırıp açtığı adresler bunu `#` sonrasında taşır ve sunucu yalnızca bunu taşıyan isteklere yanıt verir, bu nedenle DevTools'u ve web hedefini bu adreslerden açın. Bunlardan birini verdiğiniz herkes uygulama çalışırken durumunu değiştirebilir ve Recorder'ın yazdığı teste adımlar ekleyebilir; bu yüzden kaydedilen bir testi çalıştırmadan önce okuyun. Ağınızdaki diğer kişiler ve ziyaret ettiğiniz diğer sitelerin web sayfaları bunların hiçbirine erişemez.

## Güncellemeler

```sh
go tool nsgo dev -highlight
```

`-highlight`, bir işlemenin değiştirdiği her görünümün dış çizgisini yarım saniye boyunca çizer; böylece gerektiğinden fazlasını yeniden çizen bir güncelleme uygulamayı kullanırken ekranda görünür. Dış çizgiler her pencere, panel, modal, iOS sayfası (sheet) ve gömülü görünüm (`nsgo.NewView`) üzerinde çizilir. Uygulama çalışırken DevTools bunları açıp kapatır.

```sh
go tool nsgo dev -trace
```

`-trace`, çalışan uygulamanın her işlemesini yazdırır: numarası, süresi ve gönderdiği bağlama (mount) öğeleri; böylece bir güncellemenin hangi görünümleri oluşturduğunu, değiştirdiğini veya kaldırdığını görürsünüz. Öğelerin üstünde işlemenin neden kaynaklandığını adlandırır: son işlemeden bu yana sinyal yazan kod satırlarınız (en çok yazandan başlayarak) ve kaç bağlama, etki ve memo çalıştığı.

```text
── commit 12: 40 items, commit 1.65 ms
   after app/market.go:473 ×4, app/market.go:487 ×4, app/market.go:384
   ran 118 bindings, 18 effects, 5 memos
```

Çok kez yazan bir satır veya tek bir yazma için çalışan çok sayıda bağlama, yazmaları toplulaştırmanız (`cx.Batch`) ya da bir bağlamanın okuduğunu daraltmanız gereken yerdir. `go tool nsgo test -trace` bir testin her adımının altında aynısını yazdırır.

## Çalışma zamanı profilleri

`nsgo dev` ile çalışan bir uygulamanın Go CPU etkinliğini, yığınını (heap) veya yürütme izini yakalayın. Uygulama yakalamayı dev sunucusuna gönderir ve sunucu bunu komutu çalıştıran makineye kaydeder.

```sh
go tool nsgo dev -target android -profile cpu -profile-duration 10s
go tool pprof -top .nsgo/profiles/cpu.pprof
```

CPU örneği bir masaüstünde, bir iPhone'da veya bir iOS simülatöründe de çalışır. Bir iPhone'da sistem isterse Yerel Ağ erişimine izin verin. Yığın bir anlık görüntüdür ve web'de de çalışır. Zamanlayıcı ve goroutine olayları için `go tool nsgo dev -profile trace` çalıştırın ve `.nsgo/profiles/execution.trace` dosyasını `go tool trace` ile açın. CPU ve yürütme izleri varsayılan olarak 10 saniyedir. `-profile-output` yerel dosyanın yolunu ayarlar. Bileşen önizlemeleri için varsayılan ve göreli yollar uygulama dizinini kullanır. Bir dev çalıştırması ilk tam yakalamasını kaydeder, ardından uygulamayı çalıştırmaya devam eder.

```sh
go tool nsgo dev -target web -profile heap
go tool pprof -top .nsgo/profiles/heap.pprof
```

Go'nun tarayıcı çalışma zamanı yığın profillerini ve yürütme izlerini destekler. Tarayıcı CPU örnekleri için tarayıcının Performance araçlarını kullanın. `-profile` veya `-release` seçeneklerinden birini seçin. `nsgo build` ile yayınlamak dev yakalama istemcisini dışarıda bırakır. [Go çalışma zamanı tanılamaları](https://go.dev/doc/diagnostics)

## Yerleşim

```sh
go tool nsgo dev -layout
go tool nsgo dev -layout=bounds,grid=4
```

`-layout` uygulamayı her pencere, panel, iOS sayfası ve gömülü görünüm üzerinde DevTools'un yerleşim denetimiyle başlatır; bu da her görünümün dış çizgisini çizer: kenarlık kutusu mavi, iç boşluğunun iç kenarı yeşil ve dış boşluğunun dış kenarı turuncu; her biri bir kaydırma görünümünün veya kırpan bir görünümün gizlediği yerde kesilir. `grid` 8 noktalı bir ızgara çizer, `grid=N` ise en az 4 olmak üzere N noktalı bir ızgara çizer. Değeri `=` sonrasında verin: bayrak tek başına `bounds` anlamına gelir.

Alt köşedeki bir okuma alanı, kutuları, ızgarayı ve cetveli açıp kapatan düğmelerle birlikte pencerenin nokta cinsinden boyutunu gösterir. Apple TV ve yalnızca uzaktan kumandalı diğer cihazlarda odak okuma alanının düğmelerine ulaşır, böylece uygulamadan ayrılmadan kumandayla bunları değiştirebilirsiniz. Cetvel açıkken basışlar uygulamaya ulaşmak yerine iki nokta seçer ve okuma alanı bunların yatay ve dikey olarak ne kadar uzak olduğunu gösterir; üçüncü bir basış yeniden başlatır. Apple TV'de veya bir klavyeyle yön okları (veya Siri Remote dokunmatik yüzeyini kaydırmak) ekran boyunca bir artı imleci taşır ve Select, Return veya Space noktaları seçerek ikinci nokta mesafeyi kilitleyene kadar uzaklığı canlı olarak günceller.

| Tuş | Etki |
| --- | --- |
| `Cmd+Option+Shift+L`, `Ctrl+Alt+Shift+L` | `-layout` ile bir derlemede denetimi gösterir veya gizler |
| Oklar, kumanda D-pad / kaydırma | Cetvelin artı imlecini ızgara adımı kadar (varsayılan 8 pt) taşır |
| `Shift` + Oklar | Cetvelin artı imlecini 1 pt taşır |
| Select, `Return`, `Space` | Ölçüm yaparken artı imlecinin noktasını seçer |
| `Esc`, geri, Menu | Ölçümü durdurur |

Uygulamanız aynı tuşu bağlarsa uygulamanın bağlaması kazanır. Bir telefonda olduğu gibi tuş yoksa `-devtools` ile birlikte çalıştırın ve denetimi DevTools'un **Layout** seçeneğiyle gösterip gizleyin; okuma alanının altındaki bir denetime ulaşmak için gizleyin. Yerel arka uçlar ve web sunum matrislerini okur; böylece sınırlar, Pick ve denetim görüntülenen dönüşümleri, animasyonu ve kaydırmayı izler; watchOS afin çizimini izler. HarmonyOS ve mini programlar, işleme katmanları geometriyi bildirene kadar Go tarafındaki izleyiciyi korur. Bunları yeniden çizmek her güncellemede süre maliyetine yol açar; bu yüzden hızı bunlar olmadan ölçün.

## Hız

```sh
go tool nsgo dev -fps
go tool nsgo dev -release -fps
```

`-fps`, macOS, iOS, Windows, Linux, Android ve web'de uygulamanın üzerinde başarım izleyicisini gösterir. Kareleri ve işlemeleri ölçer; Android ayrıca yerel metin gidiş-dönüş süresini de gösterir. Dev derlemeleri geliştirici istemcisini taşır; `nsgo build` gibi derleyen `-release` ile ölçüm yapın.

## Diller ve görünüm

```sh
go tool nsgo dev -lang zh-Hans
go tool nsgo dev -lang en-XA   # accented and longer, to find untranslated and cut text
go tool nsgo dev -lang ar-XB   # right to left
```

`-lang` uygulamayı bir yerel ayarda, sahte yerel ayarlarda da başlatır ([Yerelleştirme](localization.md)).

## Ajanlar

```sh
go tool nsgo dev -automation
go tool nsgo dev -target android -automation
go tool nsgo dev -target web -automation
```

`-automation`, bir yapay zeka ajanının veya uçtan uca bir test çalıştırıcısının çalışan uygulamayı şimdiye kadar macOS, Windows, Linux (`-target native`), Android (`-target android`), web (`-target web`), geliştirici araçlarının simülatörlerindeki mini programlar (`-target miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` veya `miniprogram_douyin`) ve Apple cihazları ile simülatörleri (`-target ios`, `tvos`, `watchos`, `ios_simulator`, `tvos_simulator` veya `watchos_simulator`) üzerinde bir insan gibi kullanmasını sağlar: pencerelerini listeler, birinin gösterdiğini özetler, basar, yazar, tuş gönderir, kaydırır, sürükler, üzerine gelir, menü öğelerini seçer, uyarıları yanıtlar, görüneni bekler ve ekran görüntüsü alır. Ajanlar buna MCP üzerinden `go tool nsgo mcp` aracılığıyla ulaşır; bu komut dizin için çalışan `dev -automation` oturumunu kullanır veya aynı bayraklarla (örneğin `-target android`) bir tane başlatır ve istemci ayrıldığında durdurur:

```sh
claude mcp add nsgo -- go tool nsgo mcp
```

Diğer istemciler de uygulamanın dizininde çalıştırılan `tool nsgo mcp` argümanlarıyla aynı `go` komutunu alır. Görünümler [testlerin](testing.md#bir-test) seçicileriyle veya bir anlık görüntünün yazdırdığı `view:` referanslarıyla adlandırılır; bir kaydetme işlemi uygulamayı yeniden başlattığından bu referanslar eskir.

Ajan, `dev` komutunun çalıştırdığı gibi uygulamanın gerçek verileri üzerinde hareket eder. Yalnızca `dev -automation` derlemeleri bunu taşır ve yalnızca bu makinede yalnızca sizin okuyabileceğiniz `.nsgo/automation.yaml` içindeki belirtece sahip araçlar ona ulaşır.

### Uygulama tanımlı MCP araçları

Motor goroutine'i üzerinde bir Go uygulamasından veya kütüphanesinden `automation.Register` çağırın. Örneğin, bu işlev ajanlara o anda gösterilen sahnenin kimliğini verir.

```go
import (
    "context"
    "encoding/json"

    "github.com/nsgo-dev/nsgo/automation"
    "github.com/nsgo-dev/nsgo/core"
)

func registerSceneTool(cx *core.Scope, sceneID *core.Signal[string]) error {
    return automation.Register(cx, automation.Tool{
        Name:        "current_scene",
        Description: "Read the ID of the active scene",
        InputSchema: json.RawMessage(`{"type":"object"}`),
        Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
            return map[string]string{"id": sceneID.Get()}, nil
        },
    })
}
```

Ad, uygulamanın grafiğinde benzersiz olmalı ve yerleşik adlardan farklı olmalıdır. `nsgo mcp`, kapsam temizlenene kadar aracı listeler. İşleyici motor goroutine'i üzerinde çalışır, böylece sinyalleri okuyabilir veya ayarlayabilir. Bir JSON nesnesi alır, ihtiyaç duyduğu argümanları denetler ve JSON olarak kodlanabilen bir değer veya bir hata döndürür.

Bir testte uygulama başladığında kapsamı yakalayın, `app.Post` içinde `automation.Call(cx, ctx, name, args)` çağırın, ardından sonucu veya görünümü denetlemeden önce `app.Settle()` kullanın. Kayıt `dev -automation` ve testlerde etkindir, yayın derlemelerinde ise işlevsizdir (no-op).

## Her derlemenin bayrakları

Paketler dahil bir uygulamanın her derlemesi başlarken NSGo'nun kendi bayraklarını alır; bunlar `nsgo.yaml` ve uygulamanın kodunu geçersiz kılar ve uygulamanın kendi bayraklarına asla ulaşmaz:

| Bayrak | Etki |
| --- | --- |
| `--nsgo-fps` | macOS, iOS, Windows, Linux, Android ve web'de başarım izleyicisini gösterir |
| `--nsgo-lang=zh-Hans` | Uygulamayı bir yerel ayarda, sahte yerel ayarlarda da başlatır |
| `--nsgo-appearance=dark` | Uygulamayı `light`, `dark` veya `system` izleyecek şekilde başlatır |
| `--nsgo-os=ios` | Bir web derlemesini `macos`, `windows`, `linux`, `ios` veya `android` olarak çalıştırır (`web` kendisidir) |

```sh
./Hello --nsgo-fps                                   # Windows and Linux
open Hello.app --args --nsgo-fps                     # macOS
xcrun simctl launch booted dev.example.hello --nsgo-fps
adb shell am start -n dev.example.hello/dev.oneo.nsgo.NsgoActivity --ez nsgo-fps true --es nsgo-lang de
```

Web'de bunlar sayfanın sorgu dizesine gider: `?nsgo-fps&nsgo-lang=de`.

`--nsgo-os`, bir tarayıcıda başka bir platformun yerleşimini önizler: uygulama o işletim sistemini ve iOS ile Android için telefon, aksi durumda masaüstü biçimini okur ve `Env.Simulated` doğru (`true`) bildirir. Tarayıcı o platformun yerel parçalarının hiçbirine sahip değildir: `NavHeader` ve çizilen sekme çubuğu sisteminkilerin yerini alır, uygulama menüsü, yerel açılır menü (`desktop.PopupMenu`) ve pencere işlevleri yoktur ve `mobile.IsAvailable` ile `desktop.IsAvailable` yanlıştır (`false`). `ui.ContextMenu` ve `ui.Menu` gibi çizilen menüler yine çalışır. Kısayollar ziyaretçinin klavyesini izler. Diğer derlemeler bayrağı yok sayar ve bir uyarı günlüğe yazar.
