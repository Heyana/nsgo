---
title: Gezinme
description: Yerel gezinme yığınları, sistem başlıkları, modallar, menüler ve sekme çubukları.
group: Temel kavramlar
order: 56
---

## Bir yığın

`base.NewNav` tek bir türden rotalardan oluşan bir yığın tutar: `Push` bir sayfa ekler, `Pop` en üstteki sayfayı kaldırır, `PopToRoot` ilk sayfaya döner ve `Replace` en üsttekini değiştirir. `Stack` her rotayı bir sayfa olarak işler; en üstteki gösterilirken alttaki sayfaları durumlarıyla birlikte bağlı tutar. `ui.NavPage` her sayfaya `ui`'nin arka planını ve başlığı, geri düğmesi ve düğmeleriyle bir `ui.NavHeader` verir.

watchOS'ta sistem gezinme çubuğu, sekme çubuğu veya anahtar yoktur; bu nedenle orada `ui.NavPage`, geri düğmesi dahil `ui.NavHeader`'ını sayfanın üstüne çizer, `ui.TabShell` kendi sekme çubuğunu çizer ve `ui.SwitchSystem` `ui`'nin kendi anahtarıdır.

```go
nav := base.NewNav(cx, "inbox")

return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
	if route == "inbox" {
		return ui.NavPage(cx, base.NavPage{Title: "Inbox", Children: []core.Node{
			ui.Button{Text: "Open a message", OnPress: func() { nav.Push("message") }},
		}}, ui.NavHeader{})
	}
	return ui.NavPage(cx, base.NavPage{Title: "Message"}, ui.NavHeader{
		OnBack: func() { nav.Pop(1) }, BackLabel: "Inbox",
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Archive, Label: "Archive", Variant: ui.IconButtonGhost, OnPress: archive}},
	})
})
```

iOS ve Android'de sistemin başlığını, sayfanın başlığından ve başlık çubuğunun düğmelerinden, kendi geri düğmesi ve geçişiyle platform çizer; macOS, Windows, Linux ve web'de ise `ui` çizer.

Android bir Material 3 üst uygulama çubuğu kullanır. Normal bir çubuk 22 sp başlıkla 64 dp yüksekliğindedir. `HeaderLargeTitle`, 28 sp başlıklı 152 dp yüksekliğinde büyük bir çubuk verir; bu çubuk, sayfanın üstündeki kaydırma görünümü içeriğini altına kaydırdıkça normal çubuğa daralır ve içerik başa döndükçe genişler.

iOS ve Android'de yerel kompakt ve genişletilmiş başlık yazı tiplerini stillemek için `HeaderTitleStyle` ve `HeaderLargeTitleStyle` ayarlayın: `FontSize`, `FontWeight`, `FontFamily` ve `FontStyle`. Ayarlanmamış alanlar her platformun varsayılanlarını korur; `HeaderTitleStyleFunc` ve `HeaderLargeTitleStyleFunc` değişiklikleri izler. `HeaderStyle` arka planı, başlık rengini ve düğme renk tonunu sağlamaya devam eder. TSX'te bunlar `headerTitleStyle` ve `headerLargeTitleStyle` stil nesneleri veya dizileridir.

Özel bir `HeaderTitle` Android'de yalnızca kompakt başlığın yerini alır; `Title` genişletilmiş büyük başlık olarak kalır. `HeaderRight`'ı o büyük başlığın yanına koymak için `Android: core.AndroidScreenProps{HeaderRightWithLargeTitle: true}` ayarlayın. Denetimleri, başlık daraldıkça sabitlenmiş araç çubuğuna taşınır ve baştan sona basılabilir kalır. TSX `android={{ headerRightWithLargeTitle: true }}` kullanır. Büyük bant, yazı tipinin daha fazla alana ihtiyacı olduğunda büyür. Bağımsız Android derlemeleri yerel RTL düzenini etkinleştirir; gömülü bir barındırıcının, yerel başlıkların uygulamanın yönünü izlemesi için uygulamasında `android:supportsRtl="true"` ayarlaması gerekir.

`ui.NavHeader` yerine geçmesi için bir sayfaya kendi `Header`'ını verin. iOS ve Android'de bu, `HeaderTitle` yerine, sayfanın `HeaderLeft`, `HeaderRight` öğeleri ve geri düğmesinin yanında sistem başlığının başlık görünümü olur; görünümleri büyüdüğünde veya bir arama alanı gibi yüzde genişlik aldığında bu uçlar arasındaki alanı doldurur, aksi takdirde içeriğinin boyutunu alır. Diğer yerlerde içeriğin üstüne çizilir. `ui.NavHeader.Scroll` bir iOS veya Android sayfasının kaydırmasını başlığının bulanıklığına bağlamaya devam eder.

`NoPopOnBack` geriye basıldığında bir sayfayı tutar, `OnBackPress` basmayı duyar ve `NoPopOnSwipe` iOS'un kaydırarak geri gitmesini kapatır. `cx.OnBackPress(func() bool)` kaydedilmemiş değişiklikler için uygulamaya yığından önce bir işleyici verir. `OnDismiss` kullanıcının sayfayı çıkardığını bildirir ve yığın onu kaldırır.

## Sürükleyici tam ekran

Pencere için tam ekran istemek üzere `mobile.Immersive(cx, true)`, normal sistem çubuğu ayarlarına dönmek için `mobile.Immersive(cx, false)` çağırın. Bir sayfa veya iletişim kutusu için, o katman gösterilirken bunu istemek üzere `StatusBar: core.StatusBarSetting{Immersive: core.ToggleOn}` kullanın. TSX'te bir sayfa veya iletişim kutusunda `statusBar={{ immersive: true }}`, pencere varsayılanı için ise `nsgo.yaml` içinde `status_bar: { immersive: true }` kullanın.

Android'de kenardan kaydırma durum ve gezinme çubuklarını kısa bir süre gösterir. HarmonyOS'ta pencere sistemden durum çubuğunu, üç düğmeli gezinme çubuğunu ve gezinme göstergesini gizlemesini ister. iOS'ta durum çubuğu gizlenir ve ana ekran göstergesinin ne zaman gizlenip gösterileceğine UIKit karar verir. `cx.Env().SafeArea()`, ekran çentikleri dahil sistemin sağladığı iç boşlukları bildirmeye devam eder. HarmonyOS'ta yatay modda üç düğmeli gezinme ile sistem, düğmeleri bir yan kenar boyunca çizerken onların alanını alt kenar boyunca bildirir; bu nedenle güvenli alan, düğmeler yan tarafı kaplarken altta boşluk bırakabilir.

## Modallar

`ui.Dialog` karartılmış bir uygulamanın üzerinde bir yüzey, dokunmatik ekranlarda ise alttan bir sayfa gösterir; `ui.Sheet` alttan bir panel gösterir. Aksi söylenmedikçe her biri Escape tuşuyla ve arka planına basılmasıyla kapanır.

```go
ui.Dialog{
	Open:         dialog.Get,
	OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) },
	Title:        "Rename",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { dialog.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

`ui.AlertDialog` kullanıcıdan onay ister, `cx.ShowAlert` ise sistemin kendi uyarısını veya eylem sayfasını gösterir ([Dialog](ui/dialog.md)).

## Menüler

`ui.Menu`, `ui.MenuItem` öğelerini bir düğmeden açar: her cihazda, düğmenin yanında ya da `Presentation` istediğinde alttan bir sayfa olarak. `ui.ContextMenu` aynısını ikincil bir tıklama veya uzun basma için yapar; `ui.Popover`, `ui.HoverCard` ve `ui.Tooltip` ise başka sabitlenmiş içerik gösterir.

```go
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", OnSelect: duplicate},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Sekmeler

`ui.Tabs[T]`, seçilen seçeneğe kayan bir göstergeyle bir bölümlü denetim veya bir sekme listesidir; `TabStyle` her sekmenin stilinin, `Selected`'ı seçilen sekmenin stilinin, `IndicatorStyle` ise göstergenin stilinin üzerine birleştirilir. Alt çizgili sekmeler `IndicatorColor` ayarlayabilir ve `IndicatorFit: TabsIndicatorContent` ile çizgiyi içeriklerine sığdırabilir. `ui.Sidebar` bir masaüstü kenar çubuğu için bölümleri ve satırları tutar; `ui.TabShell` ise her pencere genişliğinde üst düzey bölümleri bir sekme çubuğunun yanında tutar (varsayılan olarak altta veya `Edge` / `EdgeFunc` ile, `cx.Env().VerticalBarEdge()`'i izlemek için `mobile.TabBarAutomatic` dahil, baş veya son kenarda dikey). Her sekmenin bir `SystemIcon`'ı veya bir SVG `Icon`'ı olduğunda iOS'ta her kenarda, Android'de ise alt kenarda sistemin çubuğunu kullanır ve `SystemIcon` iOS'ta bir SF Symbol'ü ya da Android'de bir drawable'ı adlandırmadığı her yerde SVG'yi gösterir; aksi takdirde çubuğu SVG simgelerle kendisi çizer. İtilmiş bir ayrıntı sayfası görünürken her iki çubuğu da gizlemek ve bölümün güvenli alanını geri yüklemek için `BarHidden` ayarlayın; çubuk gizliyken veya dikey yerleştirildiğinde `TabBarInset` 0 olur.

`mobile.TabBar`, iOS ve Android'de Liquid Glass veya Material görünümüyle sistemin sekme çubuğudur; `mobile.TabBarItem{Title, Icon, SVG}` türünden `Items`, seçili dizini, `OnChange`'i ve isteğe bağlı bir `Edge` (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` veya iOS 27.1'de `cx.Env().VerticalBarEdge()`'i izleyen ve diğer yerlerde alt kenara geri dönen `mobile.TabBarAutomatic`) ya da reaktif bir `EdgeFunc` alır. Android'in `tabBarBackground`, `tabBarIdle`, `tabBarSelected` ve `tabBarIndicator` palet rolleri çubuğun arka planını, boştaki simgeleri ve etiketleri, seçili simgeyi ve etiketi ve seçili hapı ayarlar. Özel paletler sırasıyla `Surface`, `TextMuted`, `Accent` ve `accentContainer`'a geri döner; `accentContainer` ayarlanmamışsa %12 vurgu renginde bir hap kullanılır. Varsayılan palet Material'in arka plan ve boştaki renklerini korur ve seçili sekmeyi vurgu rengiyle renklendirir. Diğer yerlerde hiç yer kaplamaz; bu nedenle bir uygulama orada `ui.Tabs` gösterir. `mobile.IsAvailable(cx)` çubuğun gösterilip gösterilmediğini söyler. `Icon` bir platform simgesini adlandırır; `lucide.House()` gibi bir `SVG`, iOS ve Android'de `Icon`'ın bir SF Symbol'ü veya drawable'ı adlandırmadığı her yerde gösterilir ve bir sistem simgesi gibi çubuk tarafından renklendirilir.

Metin için `TabBarItem.Badge` değerini `mobile.TabBadge{Text: "3"}`, nokta için `mobile.TabBadge{Dot: true}` olarak ayarlayın. Bunu `ItemsFunc` aracılığıyla güncelleyin ve temizlemek için sıfır değerini döndürün. `ui.ShellTab.Badge`, sistem çubuğu veya çizilen çubuk için `func() mobile.TabBadge` alır. Her iki alan da ayarlandığında metin önceliklidir. UIKit nokta için boş bir yerel rozet gösterir. Yardımcı teknolojiler rozet metnini veya yeni içerik için yerelleştirilmiş sözcüğü okur. TSX, yerel bir öğede `badge: { text: "3" }` veya `badge: { dot: true }`, bir `TabShell` öğesinde ise bu değeri döndüren bir işlev alır.

```go
mobile.TabBar{
	Items: []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
	Selected: func() int { return tab.Get() },
	OnChange: func(i int) { tab.Set(i) },
}
```

## Sonraki

- [Formlar](forms.md): bir yığının sayfasının tuttukları.
- [Animasyon](animation.md): sayfaların ve modalların nasıl hareket ettiği.
- [Düzen](layout.md): bir başlığın çevresindeki güvenli alan.
