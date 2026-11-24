---
title: Formlar
description: Metin alanları, odak, yazılım klavyesi, anahtarlar, seçiciler ve doğrulama.
group: Temel kavramlar
order: 57
---

## Metin alanları

`ui.Input`, `base.Input` alanlarıyla birlikte `ui` görünümündeki platformun kendi metin alanıdır. `Value` ile denetimli (controlled) olur: uygulama değeri tutar ve `OnChange` içinde günceller; `DefaultValue` ile kendi değerini tutar.

```go
name, email := core.NewSignal(cx, ""), &core.Ref{}

ui.Input{
	Value:         name.Get,
	OnChange:      name.Set,
	Placeholder:   "Your name",
	ReturnKeyType: core.ReturnKeyNext,
	OnSubmit:      func(string) { email.Focus() },
}
```

`Masked` karakterleri gizler, `Readonly` metin hâlâ seçilip kopyalanabilirken düzenlemeleri durdurur (mini programlarda yalnızca WeChat tek satırlı alanda: bu barındırıcılar bir düzenlemeyi geri alamadığından, bir WeChat çok satırlı alanı ve Alipay'in alanları `Disabled` gibi kapanır), `Disabled` alanı kapatır ve `Multiline` metni kaydırır ve stili boyutlandırmadığı sürece onunla birlikte büyür. Çok satırlı bir alanın satırları ve yer tutucusunun satırları stilinin `LineHeight` değeri kadar uzundur; yazı tipi, tıpkı bir `core.Text` içinde olduğu gibi her birinde ortalanır; tek satırlı bir alan satırını yüksekliğinde ortalanmış tutar. `KeyboardType` yazılım klavyesini seçer (`core.KeyboardEmail`, `core.KeyboardNumber`, `core.KeyboardURL` ve diğerleri), `AutoComplete` otomatik doldurma için sisteme alanın ne tuttuğunu söyler ve `MaxLength` kullanıcının girdiği UTF-16 birimlerini sınırlar. `AutoCapitalize` ve `NoAutoCorrect` klavyenin davranışını ayarlar.

`OnSelectionChange` ve `OnCaretMove` seçimi ve yanında açılacak bir menü için imlecin (caret) nerede olduğunu bildirir. Bir `core.Suggest` olan `Suggest`, `@` gibi bir tetikleyicinin ardından bir sorguyu izler ve alanın yanındaki bir `ui.SuggestList`, tıpkı `chat.Composer` bileşeninin `Suggest` alanının bahsetmeler (mentions) için yaptığı gibi, imleçte bunun için Yukarı, Aşağı, Return ve bir basışla seçilen satırlar sunar. `OnKeyPress` her tuşu, `OnPaste` kullanıcının yapıştırdığını dinler ve `OnCompositionChange` bir giriş yöntemini izler.

iOS ve Android'de `cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, handler)`, alan göndermeden, odağı kaybetmeden veya yeni satır eklemeden önce yazılım klavyesinin Return tuşunu yakalar. `KeyAuto` ve `KeyNotEditing` bunu `SubmitBehavior`'a bırakır; iOS'ta işaretli metni onaylayan bir Return giriş yönteminde kalır.

## Odak ve klavye

`Ref.Focus()` bir alanı veya masaüstlerinde, web'de ve Apple TV'de etkin bir basılabilir ya da `Focusable` görünümü odaklar. `Ref.Blur()` yalnızca kendi görünümünün odağını temizler ve `cx.DismissKeyboard()` yazılım klavyesini kapatır. `OnFocus` ve `OnBlur` odağı izler ve `View.OnFocus`/`OnBlur` odaklanabilir herhangi bir görünüm için aynısını yapar. `Show` kapandığında veya satırı gittiğinde ağaçtan ayrılan odaklanmış bir görünüm, onu kaldıran işlemeden sonra — veya platform önce bir sonraki odağı bildirirse o görünümün `OnFocus` olayından önce — bir kez `OnBlur` duyar ve çevresindeki kalan görünümler `OnFocusOut` duyar.

Bir `Ref`'in odaklama, bulanıklaştırma (blur), kaydırma ve yakınlaştırma istekleri, öğesi hiçbir yerin göstermediği bir `Holder`'ın içeriğindeyken hiçbir şey yapmaz ve öğe ağaçtan ayrıldığında hâlâ kuyrukta olanlar atılır. İçeriği tekrar göstermek bunları yeniden oynatmaz: yerleştirildikten sonra tekrar isteyin.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Yazılım klavyeleri telefonun alt kısmını kaplar. Alt iç veya dış boşluk olarak `core.EnvKeyboard` içeriği bunun üzerinde tutar ve hareketi klavyeyle birlikte canlandırılır:

```go
core.View{Style: core.Style{
	Padding:    core.All(16),
	PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard},
	Gap:        12,
}, Children: form}
```

Bir formun etrafındaki kaydırma görünümü sürüklendiğinde klavyeyi kapatabilir ve klavye görünürken bir dokunuşun ne yapacağına karar verebilir:

```go
core.Scroll{Behavior: core.ScrollBehavior{
	KeyboardDismissMode:       core.KeyboardDismissOnDrag,
	KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
}, Children: []core.Node{form}}
```

`cx.Env().KeyboardHeight()` ve `cx.Env().KeyboardDuration()` kod içinde klavyenin boyutunu ve zamanlamasını, `cx.Env().SafeArea()` ise ekranın etrafındaki iç boşlukları (insets) okur.

## Seçimler

`ui.Switch` hemen etkili olan bir ayardır, `ui.Checkbox` işaretli, işaretsiz veya belirsizdir ve `ui.Radio` bir `ui.RadioGroup`'un parçasıdır. `ui.Select[T]` tetikleyicisinin altında veya `Presentation` istediğinde bir sayfa (sheet) olarak bir seçenek listesi açar ve `ui.Tabs[T]` aynı anda birkaç tanesini gösterir.

```go
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}

size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options:  []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:    size.Get,
	OnChange: size.Set,
	Label:    "Size",
}
```

`ui.NumberInput` ve `base.NumberInput`, değişen bir erişilebilir ad için `LabelFunc` kabul eder. Özel bir `Format`, alan düzenlenmiyorken okuduğu sinyalleri izler; bir dil değişikliği düzenleme bitene kadar etkin taslağı korur. `ui.NumberInput` sınırlarla bir sayıyı adımlar ve kaydırarak değiştirir, `ui.DatePicker` ve `ui.TimeField` tarih ve saatleri düzenler, `ui.ColorPicker` bir rengi düzenler ve `ui.TagInput` bir etiket listesini düzenler.

`ui.OtpInput` tek seferlik bir kodu hücreler halinde alır. Kendisi ve `base.OtpInput`, odaklanmış alanı veya kodunu değiştirmeden `LabelFunc` kabul eder. `Size` masaüstlerinde ve TV'lerde 36, dokunmatik cihazlarda 44 nokta olan varsayılan yüksekliği ve genişliği ayarlar. `CellWidth` ve `CellHeight` her iki boyutu bağımsız olarak geçersiz kılar. `TextStyle` hücre tipografisini geçersiz kılar, `CornerRadius` köşelerini değiştirir ve `CaretHeight` satır yüksekliğinden türetilen yüksekliği geçersiz kılar.

```go
quantity := core.NewSignal(cx, 1.0)
ui.NumberInput{Value: quantity.Get, OnChange: quantity.Set, Min: 1, Max: 99, Step: 1, Label: "Quantity"}
```

## Gruplandırılmış satırlar

`ui.FormSection`, satırlarını üstte başlık ve altta alt bilgiyle bir kart üzerine koyar. Kartın arka planı, kenarlığı veya köşeleri için `CardStyle` ayarlayın. `CardStyleFunc` bunun üzerine birleştirilir ve okuduğu sinyalleri izler. `DividerInset`, `core.Each` tarafından oluşturulan satırlar dahil her alt satırı alır: ayırıcısını geçersiz kılmak için bir iç girinti ve `true`, otomatik başlık hizalamasını (diğer düğümler için 12 nokta) korumak için `false` döndürün. Sıfır iç girinti tam genişlikte bir ayırıcı yapar.

`ui.FormRow`, başlıktan önce `Leading` veya bir simge kutucuğu yerleştirir. Düğümünüz için yer ayırmak ve altındaki ayırıcıyı başlıkla hizalamak için `LeadingWidth` ayarlayın. Yuva varsayılan olarak 24 noktadır. Simge kutucuğunu, glifi ve köşeleri birlikte ölçeklendirmek için `TileSize` ayarlayın. `TitleStyle` ve `DetailStyle` başlık ve ayrıntı tipografisini geçersiz kılar. `TitleMaxLines` başlığı sınırlar (sıfır sınırsızdır), `MinHeight` satırın nokta cinsinden en düşük yüksekliğini geçersiz kılar ve `Below` başlık ve ayrıntıdan sonra bunların metin sütununa bir düğüm yerleştirir. Sıfır `MinHeight` varsayılanı korur: 40 nokta veya dokunmatik cihazlarda 48.

Tek seçimli bir küme için bölümü `base.RadioGroup` ile sarın ve her satırın `Role: core.RoleRadio`, `Checked` ve `OnPress` alanlarını ayarlayın. Satır, işaretli ve seçili erişilebilirlik durumları, tek Tab durağı ve ok tuşuyla seçim için `base.Radio` kullanır; zaten işaretli bir satıra basmak hiçbir şey yapmaz. Seçilen değerin sahibi uygulamadır.

`ui.FormSection.TitleFunc` ve `FooterFunc` ile `ui.FormRow.TitleFunc`, `DetailFunc` ve `LabelFunc`, satırı yeniden oluşturmadan sözcüklerini günceller. Boş bir ayrıntı, bölüm başlığı veya alt bilgi bu isteğe bağlı metni kaldırır; boş bir satır etiketi geçerli başlığını kullanır. Aranabilir bir `ui.Select`, açık aramasını ve seçimini korurken `SearchPlaceholderFunc` ve `EmptyFunc` alanlarını bağlar.

## Doğrulama

Kuralların sahibi uygulamadır: bir sinyal hatayı tutar, `OnBlur` veya `OnSubmit` bunu ayarlar ve bir `core.Show` hata varken alanın altına bir ileti ekler.

```go
address := core.NewSignal(cx, "")
err := core.NewSignal(cx, "")
colors := ui.ColorsOf(cx)
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{
		Value:    address.Get,
		OnChange: func(v string) { address.Set(v); err.Set("") },
		Label:    "Email",
		OnSubmit: func(string) {
			if !strings.Contains(address.Get(), "@") {
				err.Set("Enter an email address.")
			}
		},
	},
	core.Show{
		When: func() bool { return err.Get() != "" },
		Then: func(*core.Scope) core.Node { return ui.Textf(err.Get, 12, ui.Regular, colors.Danger) },
	},
}}
```

`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` beliren bir iletiyi canlandırır ve `ReturnKeyNext` ile `OnSubmit` klavyeyi bir sonraki alana taşır.

## Sonraki adımlar

- [Gezinme](navigation.md): bir formun yaşadığı yer.
- [Metin](text.md): bir alanın metninin nasıl stillendirildiği.
- [Erişilebilirlik](accessibility.md): ekran okuyucuların ihtiyaç duyduğu etiketler ve değerler.
