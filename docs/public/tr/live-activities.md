---
title: Canlı Etkinlikler
description: Bir şeyi olurken Kilit Ekranı'nda ve Dynamic Island'da, Android'de ise Canlı Güncellemeler olarak izleyin.
group: Platform özellikleri
order: 48
---

## Açın

Bir Canlı Etkinlik (Live Activity), bir teslimat veya bir zamanlayıcı gibi bir şeyi olurken izler: iOS 16.1'den itibaren iPhone'ların Kilit Ekranı'nda ve Dynamic Island'ında, Android 16'da ise bildirim panelinin üstünde ve durum çubuğundaki bir çipte Canlı Güncelleme (Live Update) olarak; daha eski Android sürümleri süren bir bildirim gösterir. Bunları `nsgo.yaml` içinde açın:

```yaml
live_activities:
  enabled: true
  # frequent_updates: true   # iOS: more updates than the system's budget, as the user allows
```

## Başlatma, güncelleme ve sonlandırma

`widget.StartActivity`, neyi izlediğini adlandıran ve widget türleri gibi yazılan bir türden bir etkinlik başlatır ve bir `widget.ActivityContent` gösterir. `Activity.Update` gösterdiğini değiştirir, `Activity.Alert` de aynısını yapar ve kullanıcıyı uyarır; `Activity.End` ise etkinliği, nil değilse son içerikle sonlandırır ve `Dismissal`'ın söylediği şekilde Kilit Ekranı'ndan ayrılır. `widget.Activities` uygulamanın, önceki çalıştırmalarınınkiler de dahil olmak üzere çalışan etkinliklerini listeler ve `widget.ActivitiesEnabled` kullanıcının ve platformun bunlara izin verip vermediğini söyler.

```go
a, err := widget.StartActivity("delivery", widget.ActivityContent{
	Title: "Your order", Body: "On its way", Symbol: "box.truck.fill",
	Timer: arrival, ShowProgress: true, Progress: 0.4,
	Actions: []widget.ActivityAction{{ID: "pause", Title: "Pause", Symbol: "pause.fill"}},
})
if err != nil {
	return
}
a.Update(widget.ActivityContent{Title: "Your order", Body: "Arrived", Status: "Now"})
a.End(nil, widget.DismissAt(time.Now().Add(2*time.Hour)))
```

`StartActivity`, bunlar olmadan derlenmiş bir uygulamada `widget.ErrNoActivities`, iOS 16.1 ve Android dışındaki yerlerde `widget.ErrActivitiesUnsupported` ve kullanıcı bunları kapattığında `widget.ErrActivitiesDisabled` bildirir; `Update` ve `End` ise etkinlik sona erdikten sonra `widget.ErrActivityEnded` bildirir. Bir etkinliğin içeriği, ActivityKit ve APNs'in kabul ettiği gibi en fazla 4 KB'tır.

## Ne gösterir

Her iki platform da başlığı ve gövdeyi, durumu veya sistemin ilerlettiği bir zamanlayıcıyı, bir ilerleme çubuğunu, en fazla üç eylemi ve vurgu rengini gösterir. iOS bunları widget uzantısıyla çizer ve Kilit Ekranı ile Dynamic Island'ın her bölümü için bunların yerine `widget` görünümlerini kabul eder: genişletilmiş hâlde `LockScreen`, `Leading`, `Trailing`, `Center` ve `Bottom`, kompakt adada `CompactLeading` ve `CompactTrailing` ve birden fazla etkinlik adayı paylaştığında `Minimal`. Android, Canlı Güncellemeler uygulamanın görünümlerini kabul etmediği için yalnızca alanları uygulamanın simgesiyle birlikte gösterir.

```go
widget.ActivityContent{
	Title: "Order 412", Body: "On its way", Status: "8 min", Symbol: "box.truck.fill",
	Tint: widget.RGB(0x30d158), ShowProgress: true, Progress: 0.4,
	Trailing: widget.Text{Content: "8 min", Style: widget.Style{FontSize: 34, FontWeight: core.FontWeightBold}},
}
```

`URL` etkinliğe dokunulduğunda uygulamayla birlikte açılır, `Stale` iOS'un içeriği güncelliğini yitirmiş olarak gösterdiği zamandır ve `Background` Kilit Ekranı'ndaki etkinliği doldurur.

## Basmalar

`widget.OnAction`, bir eyleme veya bir `widget.Button`'a basıldığında etkinlik ve eylemin kimliğiyle işleyiciyi çalıştırır (bir [widget düğmesi](widgets.md#buttons) için `Activity` nil'dir). Bunu `main` içinde, `nsgo.Run`'dan önce ayarlayın: iOS'ta sistem bir basma için uygulamayı görünümleri olmadan arka planda başlatır ve işleyici dönene kadar etkinliği tutar, bu nedenle işleyici etkinliği orada günceller; Android'de bir basma, Activity'sini açmadan çalışma zamanı alıcısı üzerinden çalışan Go sürecine ulaşır. Eylemler, onları gönderen motorun çalışmaya devam etmesini gerektirir; bayatlamış bir eylem o motor sona erdikten sonra hiçbir şey yapmaz. Bildirimin kendisine dokunulduğunda yine uygulama açılır. İşleyici, uygulamanın görünümlerininkinde değil sistemin bir goroutine'inde çalışır; görünümlere bir `core.Scope`'un `Post`'u aracılığıyla ulaşır.

```go
func main() {
	widget.OnAction(func(p widget.ActivityPress) {
		if p.Action == "pause" {
			p.Activity.Update(paused)
		}
	})
	nsgo.Run(Root)
}
```

iOS 17 ve sonrasında etkinliğin bir görünümü de bunu yapar: `widget.Button{Action: "pause"}` aynı işleyiciyi uygulamayı açmadan çalıştırır ve bir basmanın yalnızca uygulamayı açtığı yerlerde yalnızca alt öğelerini gösterir.

## Anlık bildirim (push) güncellemeleri

Arka plandaki bir uygulama etkinliklerini yalnızca çalıştığı sürece günceller; bu nedenle başka yerde olanları izleyen bir uygulama etkinliklerini `widget.ActivityOptions{Push: true}` ile başlatır, `widget.OnPushToken`'ın aldığı belirteci sunucusuna gönderir ve sunucunun, `content-state` değeri içerik için `widget.ActivityState`'in yanıtı olan APNs `liveactivity` güncellemeleri göndermesini sağlar: bir push içinde görsel bulunmaz. iOS 17.2'den itibaren uygulama, `widget.ActivityAttributesType` öznitelik türü ve türün ve kimliğin öznitelikleriyle etkinlikleri push yoluyla başlatmak için de bir belirteç alır. Push'lar, uygulamanın profilinde push bildirimleri yetkisini (entitlement) gerektirir.

```go
state, err := widget.ActivityState(content)  // the push's content-state
```

## Sınırlar

Sistem bir etkinliği sekiz saat sonra sonlandırır. `widget.DismissDefault` sona ermiş bir etkinliği iOS'ta Kilit Ekranı'nda dört saate kadar bırakır ve Android'de hemen kaldırır; `widget.DismissNow` hemen, `widget.DismissAt` ise dört saat içindeki bir zamanda kaldırır. Android `POST_NOTIFICATIONS` ve `POST_PROMOTED_NOTIFICATIONS` izinlerini kendisi ister: önce kullanıcıya `cx.RequestNotificationPermission` ile sorun ([Bildirimler](notifications.md)). Galerinin Widgets demosu ve `examples/widgets` bir tane çalıştırır.

## Sonraki

- [Widget'lar](widgets.md): aynı uzantı tarafından çizilen Ana Ekran ve Kilit Ekranı zaman çizelgeleri.
- [Bildirimler](notifications.md): Android'in önce istediği izin.
- [Derleme ve yayınlama](building.md): uzantı ve ihtiyaç duyduğu yetkiler.
