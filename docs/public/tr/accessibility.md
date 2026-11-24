---
title: Erişilebilirlik
description: Ekran okuyucular ve klavyeler için etiketler, roller, değerler, eylemler ve odak.
group: Temel kavramlar
order: 61
---

## Ekran okuyucunun okudukları

Bir görünümün `Label` alanı yardımcı teknolojilerin söylediği adı, `AccessibilityDescription` etiketten sonra okunan ek açıklamayı, `Hint` etkinleştirildiğinde ne yapacağını ve `Role` ise ne olduğunu belirtir: `core.RoleButton`, `RoleLink`, `RoleHeader`, `RoleImage`, `RoleCheckbox`, `RoleSwitch`, `RoleRadio`, `RoleTab`, `RoleTextField`, `RoleSearchField`, `RoleStatus`, `RoleMenu`, `RoleMenuItem`, `RoleDialog`, `RoleAlertDialog`, `RoleAlert`, `RoleAdjustable`, `RoleProgressBar`, `RoleTable` ve diğerleri. Bir durum öğesi (`RoleStatus`) varsayılan olarak nazik (polite) bir canlı bölgedir; bir arama alanı ise arama kutusu olarak duyurulan bir metin alanıdır. Bir iletişim kutusu açıldığında rolü ve etiketiyle, ardından tıpkı `ui.Dialog`'un başlık ve açıklamasında olduğu gibi açıklamasıyla okunur. Etiketi olmayan bir metin kendi içeriğiyle okunur. `LabelFunc`, `HintFunc` ve `AccessibilityDescriptionFunc` değişen erişilebilirlik metinlerini aynı görünüme bağlar; böylece denetimler bir dil değişimi sırasında odaklarını ve durumlarını korur. Etiket alan tüm çekirdek öğeler `LabelFunc`'ı, `View` ve `Text` ise `HintFunc`'ı kabul eder.

`ui.Button`, `ui.Checkbox`, `ui.MenuItem` ve `ui.AlertAction` üzerinde değişen metinler için `TextFunc` kullanın. Erişilebilir adları bu metni izler. Düğmeler ve onay kutuları ayrıca ayrı bir `LabelFunc` da kabul eder. Kaydırıcılar, sekmeler, anahtarlar, ilerleme çubukları ve açılır pencere tetikleyicileri değişen erişilebilir ad için `LabelFunc` kabul eder. `ui.Option.LabelFunc` sekmeler, seçim kutuları ve birleşik kutulardaki seçeneğin metnini sağlar. Sayı ve kod girişleri, ilerleme halkaları ve her iki döndürücü (spinner) varyantı da `LabelFunc` kabul eder.

```go
core.View{
	Role:  core.RoleButton,
	Label: "Add to cart",
	Hint:  "Adds the item and opens the cart",
	OnPress: add,
	Children: []core.Node{core.Icon{SVG: lucide.ShoppingCart}},
}
```

`AccessibilityValue` bir aralık veya metinsel değer verir: bir kaydırıcının en küçük, en büyük ve geçerli sayısı ya da "3 items" gibi bir metin. `Accessible` bir görünümü tek bir öğe haline getirir; böylece bir kartın adı ve fiyatı iki ayrı durak yerine birlikte okunur ve etiketi olmayan görünüm alt öğelerinin metnini okur. `AccessibilityElementsHidden` köşeli ayraç (chevron) veya ayırıcı gibi süslemeleri gizlerken `ImportantForAccessibility` bütün bir alt ağacı gizler.

```go
core.View{Accessible: true, Label: "Apples, $1.20 a kilo", Children: []core.Node{
	core.Text{Content: "Apples"},
	core.Text{Content: "$1.20 a kilo"},
}}
```

## Görünür etiketler

Bir `core.Text` veya `core.View` öğesine denetimin `Ref` değerini `LabelFor` olarak verin. Etikete basmak denetime odaklanır ve basma eylemini (örneğin bir onay kutusunu açıp kapatmayı) çalıştırır. Geçerli görünür metni, ayrı bir `Label` dizesini tekrarlamadan denetimi adlandırır ve erişilebilirlik ile stiller için denetimin `Disabled` ve `Invalid` durumlarını izler. Bir etiket düğme rolü veya Tab durağı kazanmaz.

```go
field := new(core.Ref)
return core.View{Children: []core.Node{
	core.Text{Content: "Email", LabelFor: field},
	base.Input{Ref: field},
}}
```

`LabelledBy: []*core.Ref{label, requirement}` bir denetimi verilen sırayla birden çok görünür öğeden, `Label`'dan önce adlandırır; basmaları iletmez. Aksi takdirde `Label`, sırası ağacı izleyen örtük `LabelFor` adlarına üstün gelir. Metin güncellemeleri, taşımalar, gizleme, kaldırma ve yeniden bağlama bu ilişkiyi günceller. Eksik, tekrarlanan, döngüsel ve gizli başvurular yoksayılır; ilişkilendirmeler tek bir yüzey ve sınırlama kapsamı içinde kalır. Bir etiket, bir modalın arkasındaki denetimi etkinleştiremez.

`base` ve `ui` düğmeleri, onay kutuları, radyo düğmeleri, anahtarları, seçim kutuları ve sayı girişleri `Ref` ve `LabelledBy` değerlerini kendi denetimlerine iletir; kaydırıcılar `ThumbRef` ve `StartThumbRef` sunar. TSX'te `ref={node => field.set(node)}` ile doldurulan düğüm sinyalleriyle `labelFor={field()}` ve `labelledBy={[label()]}` kullanın; böylece alanından önce gelen bir etiket, o alan bağlandığında güncellenir.

## Eylemler

`AccessibilityActions` ekran okuyucunun bir öğe üzerinde sunduğu eylemleri listeler: standart eylemler (`core.ActionActivate`, `ActionIncrement`, `ActionDecrement`, `ActionEscape`, `ActionMagicTap`) ve uygulamanın kendi eylemleri, her biri bir `Name` ve `Label` ile. `OnAccessibilityAction` bu adı alır. Öğe bağlı kalırken kullanılabilir eylemleri değiştirmek veya etiketlerini çevirmek için `core.View` veya `core.Input` üzerinde `AccessibilityActionsFunc` kullanın.

```go
core.View{
	Role: core.RoleAdjustable,
	Label: "Quantity",
	AccessibilityValue: func() core.AccessibilityValue {
		return core.AccessibilityValue{Max: 10, Now: float32(n.Get()), Text: fmt.Sprintf("%d items", n.Get())}
	},
	AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
	OnAccessibilityAction: func(name string) {
		if name == core.ActionIncrement {
			n.Update(func(v int) int { return v + 1 })
		} else {
			n.Update(func(v int) int { return v - 1 })
		}
	},
	Children: quantityControls,
}
```

`MultiSelectable` birden fazla seçili öğeye izin veren bir ızgara, liste kutusu veya ağacı işaretler. `base.DataTable` bunu çoklu satır seçimi için ayarlar; web, Windows ve Linux karşılık gelen erişilebilirlik bayrağını sunar.

`Expanded` bir açılır bölümün veya menü düğmesinin açık olup olmadığını, `Busy` içeriğin yüklenmekte olduğunu, `Invalid` uygulamanın bir alanın içeriğini veya bir denetimin değerini reddettiğini (`aria-invalid:` ile stillendirilir) bildirir; `Checked`, `Selected` ve `Pressed` ise bir denetimin durumunu raporlar. Bir denetimin parçaları kendi durumunu `PartStates` aracılığıyla alabilir; böylece bir gösterge kendi başına bir durak olmadan onay kutusunu izler.

`SelectionItem`, geçerli `Selected` değerinden bağımsız olarak bir görünümü seçilebilir bir koleksiyondaki bir öğe olarak işaretler. `base.DataTable` bunu tablo satır veya hücre seçtiğinde ayarlar, `base.Tree` ise her satıra ayarlar; Windows SelectionItem desenini korur ve web `aria-selected="true"` veya `"false"` yazar. Seçimi olmayan bir tablo satırları her iki işaret olmadan bırakır.

## Canlı değişiklikler

`LiveRegion` bir görünümün etiketi ve içindeki metin değiştikçe ekran okuyucuların bunları duyurmasını sağlar: `core.LiveRegionPolite` kullanıcının boşta kalmasını bekler, `LiveRegionAssertive` sözü keser. `cx.Announce` tek bir iletiyi bir kez söyler.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("%d items in the basket", count.Get()) },
	LiveRegion: core.LiveRegionPolite}
```

## Odak ve klavye

Araç ipuçları (tooltip) tetikleyicilerini odak almadan önce açıklar. `View.Tooltip`, `base.Tooltip` ve `ui.Tooltip` denetimin adını tekrarlamadığı, uygulama `AccessibilityDescription` sağlamadığı veya araç ipucu `Disabled` olmadığı sürece geçerli metinlerini denetimin erişilebilirlik açıklaması olarak kullanır. Bir sarmalayıcı açıklamayı kendi denetimlerine iletir. Balon ve alt öğeleri ekran okuyuculardan gizlenir ve asla bir uyarı duyurmaz; bu nedenle `ui.IconButton`'ın varsayılan araç ipucu etiketini tekrarlamaz.

Bir `base.DataTable` klavye ve erişilebilirlik odak durağı olarak adlandırılmış ızgarasını sunar; işaretçi vuruş katmanı yardımcı teknolojilerden gizlenir.

Masaüstlerinde, web'de ve Apple TV'de basmaları alan görünümler metin alanlarının yanında birer Tab durağıdır; `NoFocus` bir görünümü dışarıda tutar ve `Focusable` basma özelliği olmayan bir görünümü durak haline getirir. `Style.FocusVisible` odak halkasını yalnızca klavye odağı taşıdığında (`focus-visible:`), `Style.Focus` her türlü odakta ve `Style.FocusWithin` bir görünümün içindeki odakta çizer; `OnFocus` ve `OnBlur` bunu dinler (`OnBlur` odaklanmış görünüm ağaçtan ayrıldığında da çalışır), `Ref.Focus()` ve `Ref.SetAccessibilityFocus()` odağı taşır; `core.FocusedRef(cx)` o anda odağa sahip görünüme bir başvurudur ve bir tuşla açılan bulma çubuğu gibi tetikleyicisiz açılan öğelerin kapanırken odağı geri vermesini sağlar. Return ve Space odaklanmış görünüme basar, ok tuşları ise sekme listeleri, radyo grupları ve menüler arasında gezinir. Klavye odağı bir görünüme taşıdığında o görünümün `Tooltip`'i hemen gösterilir ve odak ayrılana veya Escape'e basılana kadar kalır. Bir açılır pencerenin `OnOpen` geri çağrısı içeriği yerleştikten sonra çalışır; böylece oradaki bir `Focus` odağı içeri taşır (kalıcı olmayan bir iletişim kutusunun yapması gerektiği gibi); bir popover içeriği Tab sırasında tetikleyicisini izler ve açılır pencere kapandığında odak tetikleyiciye geri döner.

Android, iPhone ve iPad'de `Ref.Focus()` ve bir düğmenin `AutoFocus` özelliği donanım klavyesi için etkin bir basılabilir veya `Focusable` görünüme odaklanabilir. Bir denetime dokunmak bu odağı taşımaz. Space ve Return, tuş eşleminden sonra odaklanmış basılabilir öğeyi etkinleştirir ve her basma aşamasında Control, Alt, Shift ve platform tuşunu korur. `NoFocus`, devre dışı bırakılmış veya gizlenmiş denetimler ve bir modalın arkasındaki denetimler bu odağı alamaz. Bir görünüme odaklanmak yazılım klavyesini açmaz; yerel alanlar düzenleme davranışlarını korur. watchOS ve mini programlarda fiziksel klavye etkinleştirme yolu yoktur.

`cx.Env().ScreenReader()` bir ekran okuyucunun çalışıp çalışmadığını, `cx.Env().ReduceMotion()` kullanıcının daha az hareket isteyip istemediğini, `cx.Env().IncreaseContrast()` kullanıcının daha fazla karşıtlık isteyip istemediğini (macOS, iOS ve watchOS'ta Karşıtlığı Artır, bir Windows karşıtlık teması, GNOME Yüksek Karşıtlık, GTK 4.20 karşıtlık ayarı veya yüksek karşıtlıklı bir GTK teması, Android 14'ün standart üstü karşıtlık ayarı, web'de `prefers-contrast: more` veya `forced-colors: active`), `cx.Env().FontScale()` metin boyutunu ve `cx.Env().RTL()` yerleşim yönünü bildirir. `NoFontScaling` ayarlanmadığı sürece metinler metin boyutunu izler ve sağdan sola yerleşim, mantıksal kenarları kullandığında fiziksel kenarlarını aynalar.

## Sonraki adımlar

- [Stiller ve temalar](styles.md): odak ve devre dışı durumlar.
- [Formlar](forms.md): alanlar için etiketler ve değerler.
- [Tuval ve gölgelendiriciler](canvas.md): bir çizimin parçalarını erişilebilir kılma.
