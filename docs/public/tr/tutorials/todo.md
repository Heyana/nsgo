---
title: "Örnek: Bir yapılacaklar uygulaması"
description: Sinyaller, bir liste ve bir testle adım adım küçük bir yapılacaklar uygulaması oluşturun.
group: Başlarken
order: 11
---

Bu eğitim küçük bir yapılacaklar uygulaması oluşturur: öğe ekleyin, tamamlandı olarak işaretleyin ve silin. Yaklaşık on dakika sürer ve [kurulum](../getting-started.md) adımlarının tamamlandığını varsayar.

## Uygulamayı oluşturun

```sh
mkdir todo && cd todo
go mod init example.com/todo
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Yapılacaklar uygulamasını oluşturun

Oluşturulan uygulama bir sayaçtır. `app.go` dosyasının tamamını bu uygulamayla değiştirin:

```go program height=480
package main

import (
	"fmt"
	"strings"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type todo struct {
	id   int
	text string
	done bool
}

func main() { nsgo.Run(Root) }

func Root(cx *core.Scope) core.Node {
	items := core.NewSignalFunc(cx, []todo{}, nil)
	draft := core.NewSignal(cx, "")
	nextID := core.NewSignal(cx, 1)
	colors := ui.ColorsOf(cx)

	add := func() {
		text := strings.TrimSpace(draft.Get())
		if text == "" {
			return
		}
		id := nextID.Get()
		nextID.Set(id + 1)
		items.Update(func(list []todo) []todo {
			return append(list, todo{id: id, text: text})
		})
		draft.Set("")
	}
	setDone := func(id int, done bool) {
		items.Update(func(list []todo) []todo {
			next := append([]todo(nil), list...)
			for i := range next {
				if next[i].id == id {
					next[i].done = done
				}
			}
			return next
		})
	}
	remove := func(id int) {
		items.Update(func(list []todo) []todo {
			out := list[:0:0]
			for _, it := range list {
				if it.id != id {
					out = append(out, it)
				}
			}
			return out
		})
	}
	left := func() int {
		n := 0
		for _, it := range items.Get() {
			if !it.done {
				n++
			}
		}
		return n
	}

	return core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.All(24), PaddingEnv: core.EnvAll(core.EnvSafeArea), Gap: 16},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Background()}
		},
		Children: []core.Node{
			ui.Text("Todos", 28, ui.Bold, colors.Text),
			core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
				ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "What needs doing?",
					OnSubmit: func(string) { add() }, TestID: "draft", Style: core.Style{FlexGrow: 1}},
				ui.Button{Text: "Add", Icon: lucide.Plus, OnPress: add, TestID: "add"},
			}},
			ui.Textf(func() string { return fmt.Sprintf("%d left", left()) }, 13, ui.Regular, colors.Muted),
			core.List{
				Count:         func() int { return len(items.Get()) },
				Key:           func(i int) any { return items.Get()[i].id },
				EstimatedSize: 44,
				Style:         core.Style{FlexGrow: 1},
				Empty:         ui.Text("Nothing yet. Add one above.", 13, ui.Regular, colors.Muted),
				Row: func(cx *core.Scope, row core.ListRow) core.Node {
					index := row.Index()
					it := items.Get()[index]
					return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(8, 0)},
						Children: []core.Node{
							ui.Checkbox{
								Checked: func() bool {
									for _, x := range items.Get() {
										if x.id == it.id {
											return x.done
										}
									}
									return false
								},
								OnChange: func(done bool) { setDone(it.id, done) },
								Text:     it.text,
								TestID:   fmt.Sprintf("todo-%d", it.id),
								Style:    core.Style{FlexGrow: 1, FlexShrink: 1},
							},
							ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost,
								TestID: fmt.Sprintf("delete-%d", it.id), OnPress: func() { remove(it.id) }},
						}}
				},
			},
		},
	}
}
```

Dosyayı kaydedin, uygulama yeniden başlar. Bir görev yazın, **Add** düğmesine basın ve onu işaretleyin.

## Nasıl çalışır

**Durum sinyallerden oluşur.** `items`, `draft` ve `nextID` sinyaldir. Bir sinyal bir değer tutar ve onu neyin okuduğunu izler: `items` değiştiğinde yalnızca onu okuyan metin ve satırlar güncellenir. `core.NewSignal` değerleri `==` ile karşılaştırır; bu yüzden bir dilim, eşitlik işlevi olmadan `core.NewSignalFunc` kullanır.

**Güncellemeler dilimi değiştirir.** `items.Update` yeni bir dilim döndürür; bu yüzden ekranda neyin kalacağına listenin kimliği ve satır anahtarları karar verir. `nextID` her yapılacak öğeye bir kimlik verir ve `core.List`'in `Key`'i onu kullanır; kararlı bir anahtar olmasaydı satırlar hücrelerini başka öğeler için yeniden kullanırdı.

**Ekran bir kutu ağacıdır.** `Root`, dolgu, boşluk ve güvenli alanın iç boşluklarına sahip bir `core.View` döndürür. `FlexGrow: 1` ve `FlexShrink: 1` onun pencereyi doldurmasını ve fazlasını almamasını sağlar; böylece uzun bir liste onu pencerenin kenarının ötesine uzatmak yerine içinde kaydırılır. Giriş satırı `Direction: core.Row` ayarlar; geri kalan her şey varsayılan sütundur.

**Giriş denetimlidir.** `draft.Get` alanın değeridir ve `draft.Set` her değişikliği alır. `OnSubmit` Return tuşuna basıldığında bir görev ekler, `add` ise taslağı temizler; böylece alan kendiliğinden boşalır.

**Liste yalnızca görüneni oluşturur.** `core.List`, `Count` kadar satır ister ve bir satırı yalnızca ekrana yaklaştığında oluşturur. `Empty` hiç öğe yokken gösterilir; her satırın onay kutusu ve silme düğmesi kendi yapılacak öğesinin kimliğini taşır, böylece bir basış liste değiştikten sonra da doğru öğeyi bulur.

**Görünüm `ui`'ındır.** `ui.Text`, `ui.Input`, `ui.Button`, `ui.Checkbox` ve `ui.IconButton` renklerini temadan alır ve açık ile koyu modu otomatik olarak izler. `ui.ColorsOf(cx)` uygulamanın kendisinin çizdiği parçalar için renkleri verir ve bir `StyleFunc` tema değişikliğini yerinde izler.

## Test edin

Oluşturulan test sayacın düğmesine basar; bu yüzden `app_test.go` dosyasını yapılacaklar uygulamasının bir testiyle değiştirin:

```go
package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestAddCompleteAndDelete(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Find("text:Nothing yet. Add one above.")
	app.Type("id:draft", "Buy milk")
	app.Press("id:add")
	app.WaitFor("text:Buy milk", time.Second)
	app.Press("id:todo-1")
	app.WaitFor("text:0 left", time.Second)
	app.Press("id:delete-1")
	app.WaitFor("text:Nothing yet. Add one above.", time.Second)
}
```

Test uygulamayı başsız (headless) başlatır, öğeleri metinlerine veya test kimliklerine göre bulur ve bir kullanıcının yapacağı gibi davranır. Şununla çalıştırın:

```sh
go tool nsgo test
```

`TestMain`, testler bir hedefte çalıştığında ana iş parçacığını platforma verir ve `go tool nsgo test -target ios_simulator` aynı testi simülatörde, simülatörün yerel girdisiyle çalıştırır.

## Yayınlayın

```sh
go tool nsgo build
```

`build`, `nsgo.yaml`'ın listelediği her hedef için `dist/<name>-<version>-<target>` içine bir sürüm paketi yazar: bir `.app`, bir `.ipa`, bir `.apk`, Windows'un ve Linux'un dizinleri ya da bir web sitesi. Dağıtılacak her şey ayrıca `dist/releases` içine de düşer: disk görüntüsü, kurulum programı, Debian ve Arch paketleri, IPA'lar, APK'lar, site, mini programlar ve uygulamayı GitHub sürümlerinizden kuran betikler. [Derleme ve dağıtım](../building.md) yükleyicileri, imzalama ve noterlemeyi, yayınlamayı, simgeleri, widget'ları, App Clip'leri ve App Store'u kapsar.

## Sonraki adımlar

- [Örnek: Bir not uygulaması](notes.md) ve [Örnek: Popüler GitHub depoları](repos.md): adım adım iki uygulama daha.
- [Bileşenler ve durum](../components.md): öğeler, bileşenler, sinyaller ve bölgeler daha ayrıntılı olarak.
- [Yerleşim](../layout.md) ve [Stiller ve temalar](../styles.md): ekranın yapısı ve görünümü.
- [Listeler ve kaydırma](../lists.md): uzun listeler, yapışkan başlıklar ve daha fazlasını yükleme.
- [Test etme](../testing.md): denetimleri bulun, onlara basın ve ekran görüntüsü alın.
- [Gömme](../embedding.md): uygulamayı başka bir uygulamanın içine yerleştirin.
