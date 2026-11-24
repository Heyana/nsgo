---
title: Parçalar ve durumlar
description: Uygulamanın sahip olduğu hiçbir durumu tutmayın, her parçayı stillendirin ve yardımcı teknolojilere bileşenin ne olduğunu bildirin.
group: Bileşen yazma
order: 71
---

Stilsiz bir bileşen bir denetimin nasıl davrandığına karar verir ve nasıl görüneceğini onu kullanana bırakır. Bu sayfada, `base`'in kendi denetimlerini oluşturduğu şekilde, açık veya kapalı olabilen bir hap düğmesi olan `Toggle` oluşturulmaktadır.

## Denetlenen değerler

Bir denetim, uygulamanın sahip olduğu hiçbir değeri tutmaz: uygulama değeri bir işlev olarak geçirir ve bir geri çağrıdan ayarlar. Bileşen değeri gösterildiği yerde okur; böylece değiştiğinde yalnızca o parça güncellenir.

```go
// Toggle is a pill that is on or off. Its parts are the pill and a dot
// inside it, then its children, the label.
type Toggle struct {
	On       func() bool
	Disabled func() bool
	// OnChange receives the value a press asks for, the opposite of On's.
	OnChange func(on bool)

	Class        string
	Style        core.Style
	StyleFunc    func() core.Style
	DotClass     string
	DotStyle     core.Style
	DotStyleFunc func() core.Style
	Children     []core.Node
	Label        string
}
```

Bir uygulama değeri bir sinyalde tutar: `chips.Toggle{On: bold.Get, OnChange: bold.Set, Label: "Bold"}`.

## Parçalar

Bir bileşenin çizdiği her parça, parçanın adını alan bir `Class`, bir `Style` ve bir `StyleFunc` alır (`DotClass`, `DotStyle`, `DotStyleFunc`). `Style` sabittir, `StyleFunc` okuduğu sinyalleri izler ve sınıf her ikisinin altında yer alır; böylece uygulama hangisini yazarsa onu kullanabilir.

Bir parça, kendi durumunu raporlamak yerine denetimin durumunu `PartStates` aracılığıyla alır; böylece yardımcı teknolojiler tek bir denetim görürken `checked:` sınıfları ve `Checked` stilleri ona uygulanır:

```go
func (t Toggle) Render(*core.Scope) core.Node {
	on := func() bool { return t.On != nil && t.On() }
	off := func() bool { return t.Disabled != nil && t.Disabled() }
	shows := func() core.PartStates { return core.PartStates{Checked: on(), Disabled: off()} }
	dot := core.View{Class: t.DotClass, Style: t.DotStyle, StyleFunc: t.DotStyleFunc, PartStates: shows}
	return core.View{
		Class: t.Class, Style: t.Style, StyleFunc: t.StyleFunc,
		Role: core.RoleSwitch, Label: t.Label, Checked: on, Disabled: t.Disabled,
		OnPress: func(core.PressEvent) {
			if t.OnChange != nil {
				t.OnChange(!on())
			}
		},
		Children: append([]core.Node{dot}, t.Children...),
	}
}
```

Bir uygulama bunu yalnızca sınıflarla stillendirir:

```go
chips.Toggle{
	On: bold.Get, OnChange: bold.Set, Label: "Bold",
	Class:    "flex-row items-center gap-2 rounded-full border border-zinc-300 px-3 py-1 checked:bg-blue-50",
	DotClass: "size-2 rounded-full bg-zinc-300 checked:bg-blue-500",
	Children: []core.Node{core.Text{Content: "Bold"}},
}
```

`PartStates.Invalid` benzer biçimde, başka bir geçersiz denetim bildirmeden dekoratif bir parçaya `aria-invalid:` sınıflarını ve `Style.AriaInvalid`'i (içindeki `group-aria-invalid:` stilleri dahil) uygular. Yardımcı teknolojilerin doğrulama durumunu alabilmesi için `Invalid` özelliğini gerçek denetimin veya yerel girişin üzerine koyun.

## Erişilebilirlik

Bir denetimin kökü onun ne olduğunu belirtir: bir `Role`, metni bunu söylemediğinde bir `Label`, etkinleştirildiğinde ne yaptığı için bir `Hint` ve uygun oldukça `Checked`, `Selected`, `Disabled`, `Invalid` veya `Expanded`. Parçaları sessiz kalır. Bir sekme listesi gibi ok tuşlarıyla öğeler arasında geçiş yapan bir denetim tek bir Tab durağıdır. [Erişilebilirlik](../accessibility.md) rolleri ve eylemleri listeler.

## Doğrudan aktarılacaklar

Kökü kendi görünümü olan bir bileşen, o görünümün `OnLayout`, `Tooltip` ve `TestID` alanlarını alır ve bunları iletir; böylece uygulamalar onu herhangi bir görünüm gibi ölçer, açıklar ve test eder. Geri çağrılar isteğe bağlıdır: `nil` bir `OnChange`, bir değeri değiştirmeden gösteren bir denetim oluşturur.
