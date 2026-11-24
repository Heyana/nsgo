---
title: Stilsiz bileşenler
description: Parça parça stillendirilebilen veya üzerine stilli bir kütüphane kurulabilen, görünümü olmayan NSGo denetimleri.
group: Bileşen yazma
order: 73
---

`base`, `ui` paketinin sahip olduğu her denetimi davranışı, durumu ve erişilebilirliğiyle ancak bir görünümü olmadan tutar. Her parça bir `Class`, bir `Style` ve bir `StyleFunc` alır ([Parçalar ve durumlar](parts.md)); bu nedenle kendi tasarımına sahip bir kütüphane veya uygulama `ui` yerine bunları kullanır ya da `ui`'nin yaptığı gibi bunları sarmalar ([Görünümler ve temalar](looks.md)). Bu sayfa [bileşen başvurusundaki](../ui.md) denetimleri kapsar; galeri her birini `ui` biçiminin yanında gösterir.

`Button`, `Checkbox`, `Select`, `Slider` ve `NumberInput` gibi denetimler bir doğrulama sinyalini izlemek için `Invalid: rejected.Get` kabul eder. Bu durumu `Class: "aria-invalid:border-red-500"` veya `Style.AriaInvalid` ile stillendirin. Denetimi devre dışı bırakmadan erişilebilirliği ve stilleri günceller. Bileşik alanlar bunu gerçek girişlerine ve dekoratif parçalarına iletir; `ColorPicker` bunu kendi denetimleriyle paylaşır.

Görünümler arasında sürüklenen uygulama değerleri için [`base/dragdrop`](../drag-drop.md) kullanın: bir alan kaynakları ve hedefleri bağlarken uygulama içeriklerini, önizlemelerini ve görünümlerini sağlar.

## Button

`base.Button` görünümünü `Style`'dan, içeriğini ise `Children`'dan alır. `OnPress` tamamlanan bir basmada çalışır; `OnPressDown` ve `OnPressUp` kendi geri bildiriminiz için başlayan ve biten bir basmayı izler.

```go
base.Button{
	Label:   "Save",
	OnPress: func(core.PressEvent) { save() },
	Style: core.Style{Background: core.RGB(0x007aff), Padding: core.Symmetric(8, 14), CornerRadius: 8, AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.85)}},
	Children: []core.Node{core.Text{Content: "Save", Style: core.Style{Color: core.RGB(0xffffff)}}},
}
```

`Disabled`, `Busy`, `Selected` ve `Expanded` uygulamaya, stile ve yardımcı teknolojiye bir durum bildirir: hala çalışan bir eylem `Busy` (`aria-busy:` ile stillendirilir), bir anahtarlama düğmesi veya bir listenin geçerli öğesi `Selected`, bir açılır bölüm `Expanded` kullanır. Bir satır veya liste başka bir yolla ulaştığında `NoFocus` bir düğmeyi Tab duraklarından çıkarır.

## Checkbox

`base.Checkbox` kutu için bir stil, gösterge için bir stil ve içinde uygulamanın sahip olduğu değerden gösterilen kendi işaretinizi alır.

```go
base.Checkbox{
	Checked:        done.Get,
	OnChange:       done.Set,
	Label:          "Remember me",
	Style:          core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), BorderWidth: 1, CornerRadius: 5},
	Indicator:      core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14}},
	Children:       []core.Node{core.Text{Content: "Remember me"}},
}
```

Gösterge kendi durumunu kutudan `PartStates` aracılığıyla alır; böylece `checked:` sınıfları onu ikinci bir durak haline getirmeden stillendirir.

Değiştirici tuşlar için `OnChangeEvent func(checked bool, press core.PressEvent)` ayarlayın. `OnChange`'in yerini alır ve değeri uygulamada bırakır; `press.Modifiers.Shift` bir listenin aralık seçmesine olanak tanır. Masaüstlerinde, web'de, Android ve iOS/tvOS'ta Space/Return etkinleştirmesi bir uygulama kısayolu tuşu önce işlemediği sürece değiştirici tuşları korur; Windows'ta Space veya Return ile Alt bunun yerine pencere menüsüne gider. TSX'te `onChangeEvent` `{ checked, press }` alır.

## Switch

`base.Switch` bir kanal, içinde bir tutamaç ve ardından etiket olarak alt öğelerinden oluşur. Bir basma, `Checked`'ın tersi olan istediği değeri bildirir.

```go
base.Switch{
	Checked:    dark.Get,
	OnChange:   dark.Set,
	Label:      "Dark appearance",
	Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	TrackStyle: core.Style{Width: core.Pt(38), Height: core.Pt(22), CornerRadius: 11},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

Kanal ve tutamaç kendi `StyleFunc`'larını alır ve durum stilleri `Checked`'ı izler.

## Slider

`base.Slider` bir `base.SliderValue` alır: `base.SingleValue(v)` veya `base.RangeValue(low, high)`. Kanalı, göstergeyi, aralığı ve tutamaçları ayrı ayrı stillendirir.

```go
level := core.NewSignal(cx, 3.0)
base.Slider{
	Value:      func() base.SliderValue { return base.SingleValue(level.Get()) },
	OnChange:   func(v base.SliderValue) { level.Set(v.End) },
	Min:        func() float64 { return 0 },
	Max:        func() float64 { return 10 },
	Step:       func() float64 { return 1 },
	Label:      "Level",
	Style:      core.Style{Height: core.Pt(28)},
	TrackStyle: core.Style{Height: core.Pt(4), CornerRadius: 2},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

`Min`, `Max` ve `Step` reaktif alıcılardır (getter): sinyallerini değiştirmek, sürükleme sırasında da dahil olmak üzere bağlı kaydırıcıyı ve erişilebilir aralığını günceller. `Axis` onu dikey yapar ve `Inverted` en küçük değerini uzak uca koyar (tıpkı `base.Progress` gibi); `Marks` gösterge üzerine kendi çentiklerinizi yerleştirir ve `OnChangeEnd` sürükleme durduğunda değeri bildirir.

## Select

`base.Select`, `base.SelectItem` öğeleri ve bir `*string` değeri alır; dolayısıyla “hiçbiri” `nil`'dir. Tetikleyiciyi, açılır pencereyi, satırlarını ve gruplarını siz stillendirirsiniz.

```go
choice := core.NewSignal[*string](cx, nil)
base.Select{
	Items: func() []base.SelectItem {
		return []base.SelectItem{{Value: "s", Title: "Small"}, {Value: "m", Title: "Medium"}}
	},
	Value:      choice.Get,
	OnChange:   choice.Set,
	Label:      "Size",
	Style:      core.Style{Height: core.Pt(28), Padding: core.Symmetric(4, 10), BorderWidth: 1, CornerRadius: 6},
	PopupStyle: core.Style{Padding: core.All(4), CornerRadius: 8},
	ItemStyle:  core.Style{Padding: core.Symmetric(4, 8), CornerRadius: 4},
}
```

Aynı `Group`'a sahip bitişik öğeler bir başlığı paylaşır, yazarak arama (typeahead) öğelerin başlıklarıyla eşleşir, `SearchField` kendi arama alanınızı çizer ve `Cleanable` bir temizleme düğmesi gösterir. `OnCancel` Escape, geri ve dışarıya basma eylemlerini dinler.

## Tabs

`base.Tabs`, `base.Tab` öğelerini tutar ve göstergeyi `IndicatorStyle`'dan çizer; uygulama hangi sekmenin seçili olduğunu tutar. `base.Tab.OnPress` seçili olan dahil her basmada çalışır.

```go
base.Tabs{Label: "Sections", IndicatorStyle: core.Style{Height: core.Pt(2), Background: core.RGB(0x007aff)}, Children: []core.Node{
	base.Tab{Label: "Overview", Selected: func() bool { return section.Get() == "overview" },
		OnPress: func(core.PressEvent) { section.Set("overview") }, Children: []core.Node{core.Text{Content: "Overview"}}},
	base.Tab{Label: "Activity", Selected: func() bool { return section.Get() == "activity" },
		OnPress: func(core.PressEvent) { section.Set("activity") }, Children: []core.Node{core.Text{Content: "Activity"}}},
}}
```

## Dialog

`base.Dialog` arka plan katmanını, katmanı ve açılır pencere stillerini, bir `Animation` veya `TransitionShader` ile alt öğelerinizi alır. `Open`, `DefaultOpen` ve `OnOpenChange` onu denetler; `OnOpen`, `OnClose` ve `OnCancel` ne olduğunu bildirir. `OnConfirm` varsayılan eylemdir: klavye odağı Return'ü kendisi alan bir düğme veya alanda olmadığı ya da başka bir modal iletişim kutusunu örtmediği sürece Return onu çalıştırır, ardından iletişim kutusu `ReasonSelect` ile kapanır. `Role` ve `AccessibilityDescription` ekran okuyucuların duyurduğu şeylerdir; bir onay için `RoleAlertDialog` kullanılır. `NoCloseOnSwipe` iOS'un kaydırmasının onu kapatmasını engeller, `Presentation` iOS sunumunu seçer ve `StatusBar` açıkken durum çubuğunu değiştirir.

`base.Sheet` bir arka plan katmanı ve içeriğinizle herhangi bir kenardaki bir paneldir; `CloseOnSwipe` ile bir alt sayfa aşağı doğru sürüklemeyi izler ve bir eşiğin ötesinde kapanır. Tıpkı `base.Dialog` gibi `OnConfirm`, `Role` ve `AccessibilityDescription` alır. `base.ShowAlertDialog` role göre düğmelerle sistemin uyarısını veya eylem sayfasını gösterir:

```go
base.ShowAlertDialog(cx, base.AlertDialog{
	Title: "Delete the note?",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(label string) {
	if label == "Delete" {
		remove()
	}
})
```

## Menu

`base.Menu` kendi tetikleyicinizden `base.MenuItem` öğelerini açar, `base.ContextMenu` ise işaretçi konumunda açılır. Bir öğe `OnSelect` bildirir ve `NoCloseOnSelect` ayarlanmadıkça menüyü kapatır; `Checked` onu işaretlenebilir bir öğe yapar, `Indicator` içeriğinden önce işaretinizi çizer ve `Role` yardımcı teknolojilerin ne duyacağını seçer: bir menü öğesi, onay kutusu veya radyo öğesi. `base.Submenu` daha fazla öğe tutar.

```go
base.Menu{Trigger: core.Text{Content: "Actions"}, Children: []core.Node{
	base.MenuItem{Label: "Show sidebar", Checked: sidebar.Get, OnSelect: func() { sidebar.Set(!sidebar.Get()) }},
	base.Submenu{Label: "Share", Children: []core.Node{
		base.MenuItem{Label: "Copy link", OnSelect: copyLink},
		base.MenuItem{Label: "Email", OnSelect: emailLink},
	}},
}}
```

`Placement`, `Align`, `Offset` ve `Margin` masaüstlerinde açılır pencereyi yerleştirir; `PopupStyle` ve `BackdropStyle` ise onu stillendirir.

## EmojiPicker

`base.EmojiPicker` her Unicode emojisini uygulamanın dillerindeki (veya açık `Languages` / `LanguagesFunc`) yerelleştirilmiş CLDR adlarına, anahtar sözcüklerine ve kısa kodlarına göre arar; kategori sekmeleri, isteğe bağlı `Recent` girdileri, ten rengi seçimi, klavye gezinmesi ve sanallaştırılmış bir ızgara sunar. Tüm varsayılan etiketler ve kategori adları `base/locales` içinden gelir ve emoji veri kümesi yalnızca `EmojiPicker` veya yardımcılarını (`base.SearchEmojis`, `base.EmojisInCategory`, `base.EmojiByString`) kullanan uygulamalara bağlanır.

```go
base.EmojiPicker{
	Recent:   recent.Get,
	OnSelect: func(e base.Emoji) { insert(e.Emoji) },
	Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
}
```

`Category` ve `DefaultCategory` göz atma grubunu seçer. Kategori sekmeleri ve ten rengi seçeneklerinin her biri ok tuşlarıyla gezinilen tek bir Tab durağıdır. `EmojiPickerRef` `Focus`, `Query`, `SetQuery`, `SetCategory` ve `MatchedCount` sağlar; stilli seçici de aynı sorgu durumunu kullanır. `LabelFunc`, `SearchLabelFunc` ve `PlaceholderFunc` sözcükleri yerinde günceller. `Emoji.WithSkinTone` seçilen tonu bir çiftteki her iki kişiye de uygular ve varsayılan ton nötr biçimini geri yükler; bir emoji ile arama yapmak onun tam dizisini korur.

Lucide kategori simgeleri, temizlenebilir arama alanı, vurgulanmış emoji önizlemesi ve ten rengi örnekleri içeren temalı seçici için `ui.EmojiPicker` kullanın.
