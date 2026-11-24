---
title: Giriş alanı
description: Platformun metin alanı; denetimli veya denetimsiz, odak ve yazılım klavyesiyle.
group: Bileşenler
order: 33
---

`ui.Input`, `ui`'nin görünümünde platformun metin alanıdır; `Multiline` olmadıkça tek satırdır. Her stilli alan gibi `base.Input`'un alanlarına sahiptir; `ui.Field` kendine ait bir boyutu olmayan, işaretçiyi veya odağı izlemeyen bir kenar içindeki aynı alandır; `Background` içinde `core.Transparent` ve `BorderWidth` içinde `core.Zero`, düz metin gibi okunan ama düzenlenebilen bir metin için kutuyu kapatır.

```go preview=input/basic
name := core.NewSignal(cx, "")
ui.Input{
	Value:       name.Get,
	OnChange:    name.Set,
	Placeholder: "Your name",
	TestID:      "name",
}
```

## Denetimli ve denetimsiz

`Value` ile alan denetimlidir: değeri uygulama tutar ve `OnChange` içinde günceller. `DefaultValue` ile alan kendi değerini tutar. Denetimli bir alan, değeri `""` olarak ayarlanarak temizlenebilir.

```go preview=input/submit
draft := core.NewSignal(cx, "")
ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "New task",
	OnSubmit: func(string) { add(); draft.Set("") }}
```

## Alanlar

`Multiline` satır kaydırır, üç satır yüksekliğinde başlar ve stili `Height` veya `MinHeight` ayarlamadıkça metniyle birlikte büyür. `Masked` karakterleri gizler, `Readonly` metin seçilip kopyalanabilirken düzenlemeleri durdurur, `Disabled` alanı kapatır ve `MaxLength` kullanıcının girdiği UTF-16 birimlerini sınırlar. `Placeholder`, `PlaceholderFunc` ile bir işlev olabilir.

```go
ui.Input{Masked: true, Value: password.Get, OnChange: password.Set,
	AutoComplete: core.AutoCompletePassword}
```

`KeyboardType` bir yazılım klavyesi seçer (`KeyboardEmail`, `KeyboardNumber`, `KeyboardURL` ve diğerleri), `ReturnKeyType` onun return tuşunu etiketler ve `SubmitBehavior`, Return'ün gönderip göndermediğini veya çok satırlı bir satırı bölüp bölmediğini söyler. `AutoCapitalize`, `NoAutoCorrect` ve `AutoComplete` sistemin yardımını ayarlar.

`InputSource`, alan odaktayken bir klavye giriş kaynağı seçer ve `KeepInputSource` olmadıkça alan odağı kaybederken kullanıcınınkini geri yükler: `"com.apple.keylayout.ABC"`, macOS'ta kodlar veya komutlar için bir alanı İngilizcede tutar. Kimlik platformundur: Windows'ta `"00000409"` gibi bir klavye düzeni kimliği, Linux'ta ise bir Fcitx 5 veya IBus giriş yöntemi; etkin olmayan bir kaynak, telefonlar ve web hiçbir şeyi değiştirmez.

```go
ui.Input{Placeholder: "Bundle ID", InputSource: "com.apple.keylayout.ABC"}
```

`Style` alanın görünümünün üzerine, `StyleFunc` ise onun üzerine birleştirilir: yükseklik, dolgu, yazı tipi veya bir renk gibi ayarladıkları `ui`'ninkinin yerini alır, geri kalanı aynı kalır. Hiçbiri `BorderColor` veya `Hover` ayarlamadığı sürece kenar işaretçinin altında koyulaşır ve odaktayken vurgu rengini alır. Sıfır bir `Padding`, `ui`'ninkini bırakır; ayarlanmış bir uzunluk olan `core.Zero` ise onu kaldırır (`Padding: core.All(core.Zero)`).

```go
ui.Input{Placeholder: "Filter", Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
ui.Field{DefaultValue: "Untitled", Style: core.Style{FontSize: 24, FontWeight: ui.Bold,
	Background: core.Transparent, BorderWidth: core.Zero}}
```

## Odak ve olaylar

`Ref` alanı odaklar ve odaktan çıkarır. `OnFocus` ve `OnBlur` odağı, `OnSelectionChange` ve `OnCaretMove` seçimi ve imleci, `OnKeyPress` yazılan her tuşu, `OnPaste` yapıştırılanı ve `OnCompositionChange` bir giriş yönteminin oluşturmasını izler.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Yazılım klavyesi bir telefonun alt kısmını kaplar. İçeriğin klavyenin üzerinde kalması için içeriğin alt dolgusuna veya kenar boşluğuna `core.EnvKeyboard` koyun; hareket klavyeyle birlikte canlandırılır. `cx.DismissKeyboard()` klavyeyi kapatır, `cx.Env().KeyboardHeight()` ise yüksekliğini okur.

## Doğrulama

Kurallar uygulamaya aittir. Bir sinyal hatayı tutar, `OnBlur` veya `OnSubmit` onu ayarlar ve alanın altındaki bir mesaj onu gösterir ([Formlar](../forms.md#doğrulama)).

```go preview=input/validation
err := core.NewSignal(cx, "")
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{Value: email.Get, OnChange: email.Set, Label: "Email",
		OnSubmit: func(string) {
			if !strings.Contains(email.Get(), "@") {
				err.Set("Enter an email address.")
			} else {
				err.Set("")
			}
		}},
	ui.Textf(err.Get, 12, ui.Regular, colors.Danger),
}}
```

## Erişilebilirlik

Alan `RoleTextField` rolünü, ya da `Role` öyle söylediğinde bir arama kutusu için `RoleSearchField` rolünü bildirir ve `Label`'ını okur; `Placeholder` bir etiket değildir, bu yüzden görünür etiketi olmayan bir alana bir `Label` verin. `Readonly` ve `Disabled` bildirilir, `OnSubmit` ise klavyenin eylemidir. Tek satırlı, maskeli ve çok satırlı alanlar macOS, iOS ve Linux'ta geçerli `Placeholder`'ı yerel erişilebilirlik yer tutucusu veya ipucu olarak sunar; çok satırlı bir alanın çizilen yer tutucu etiketi için ikinci bir odak durağı olmadan güncellenir ve temizlenir.

## Platform

iOS, Android, Windows ve Linux kendi yerel alanlarını kullanır; web ise bir `<input>` veya `<textarea>` kullanır. Yazılım klavyesi, otomatik doldurma, giriş yöntemleri ve metin yönü platformundur. Odak ve yazma her hedefte aynı şekilde test edilir.

## İlgili

- [Formlar](../forms.md): alanlar, doğrulama, seçimler ve klavye.
- [Erişilebilirlik](../accessibility.md): etiketler ve odak.
- [Yerleşim](../layout.md): içeriği klavyenin üzerinde tutmak.
- Galerinin Input ve Form sayfaları.
