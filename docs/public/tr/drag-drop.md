---
title: Sürükle ve bırak
description: Uygulama değerlerini tek bir pencerede görünümler arasında aktarın.
group: Temel kavramlar
order: 57
---

`base/dragdrop`, tek bir pencerede aynı `Area` içindeki bir kaynağı ve bir hedefi birbirine bağlar. Uygulama görünümlerini, stillerini, seçimini ve değeri işleyen eylemi korur. Bir kaynak varsayılan olarak dört nokta hareketten sonra başlar ve sıradan bir basışı bozmadan bırakır.

```go
import "github.com/nsgo-dev/nsgo/base/dragdrop"

dragdrop.Area{Children: []core.Node{
	dragdrop.Source{
		View: core.View{
			Label: "Film", TestID: "poster",
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Poster"}},
		},
		Data: func() any { return filmID },
		Preview: func(any) core.Node { return core.Text{Content: "Film"} },
	},
	dragdrop.Target{
		View: core.View{
			TestID: "playlist",
			Style: core.Style{Width: core.Pt(160), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Playlist"}},
		},
		Accept: func(value any) bool { _, ok := value.(string); return ok },
		OnDrop: func(e dragdrop.Event) { addToPlaylist(e.Data.(string)) },
	},
}}
```

`Data` sürükleme başladığında bir kez çalışır. Birkaç seçili öğeyi birlikte taşımak için bir dilim döndürün; anlık görüntü olarak kalması gerekiyorsa değişebilir veriyi orada kopyalayın. Nil bir `Data` sürüklemeyi devre dışı bırakır. `Threshold` gereken hareketi ayarlar. `Activation: dragdrop.AfterLongPress` önce `View.DelayLongPress` (varsayılan olarak 500 ms) süresini, ardından hareketi bekler; bir kaydırma hareketinin önce sayfayı kaydırması gereken yerlerde bunu açıkça seçin.

`View`, basma işleyicileri ve erişilebilirlik dahil olağan özelliklerini korur. Bir kaynak `View.Responder` alanının sahibidir; onu ayarlamadan bırakın. Alt öğeleri normal eylemine sahip bir düğme içerebilir. `Preview` giriş almadan veya kaynağı yeniden bağlamadan işaretçiyi izler; bir önizleme yoksa kaynağın etiketi gösterilir. Bir kaynağa etiket veya önizleme verin. İç içe alanlar birbirinden bağımsız çalışır.

Hedeflerin bir değeri kabul etmesi için `OnDrop` gerekir. İlk şansı en içteki hedef alır; `Accept` bunu reddettiğinde kabul eden bir ata öğe alabilir. `OnEnter`, `OnMove` ve `OnLeave` hedefin görünümünü güncelleyebilir. Her olay `Data`, hedefteki `Position` ve yüzeydeki `PagePosition` alanlarını taşır. Etkin hedefi devre dışı bırakmak veya kaldırmak girilmiş durumunu temizler; bırakma işlemi kabulü yeniden denetler. Başka bir görünümün arkasında kalan, kırpılmış, devre dışı bırakılmış veya bir modalın altındaki hedef bırakmayı alamaz.

`OnStart` ve `OnEnd` kaynağın oturumunu tanımlar. `OnEnd`, `Result{Data, Dropped, Target}` alır. Escape, yerel dokunma iptali, kaynağı devre dışı bırakmak veya kaldırmak ya da kabul eden bir hedefin dışına bırakmak bırakma gerçekleşmeden sonlandırır. Hedef yerleşimi son işlenen geometriyi kullanır; işlemeler arasındaki yerel animasyonlar örneklenmez. Bu özellik değerleri pencere içinde aktarır; sistem dosyaları hâlâ `OnFileDrop` kullanır.

Aynı işlemi gerçekleştiren bir menü, klavye komutu veya erişilebilir eylem bulundurun. Sürükleme bu eyleme giden bir kısayoldur. Testler `app.DragTo("id:poster", "id:playlist")` veya seçili bir yüzeyin `DragTo` yöntemini kullanır; TSX testleri `await app.dragTo("id:poster", "id:playlist")` kullanır. Her iki görünüm de halihazırda görünür pencereye sığmalıdır; adım, bırakma sentezlemeden veya sürükleme sırasında kaydırma yapmadan yanıtlayıcı üzerinden bir dokunma dizisi gönderir. Bkz. [Test etme](testing.md).
