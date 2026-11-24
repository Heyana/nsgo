---
title: Terminal
description: Bir terminal ızgarasında bir program çalıştırın veya uzak bir oturum gösterin.
group: Temel kavramlar
order: 63
---

`base/terminal` bir oturum (`Terminal`) ve onun `View`'ını sağlar. Masaüstleri yerel programlar çalıştırır; iOS ve tarayıcılar, uzak bir programa bağlı bir `io.ReadWriteCloser` olan `Options.Conn`'u kullanır. Oturumun sahibi uygulamadır ve iş bittiğinde onu kapatır.

```go
t, err := terminal.New(terminal.Options{CursorBlink: true})
if err != nil {
	return core.Text{Content: err.Error()}
}
cx.Cleanup(func() { t.Close() })
return terminal.View{
	Terminal: t, AutoFocus: true,
	MinimumContrast: 4.5, PaddingColor: terminal.PaddingExtend,
	Style: core.Style{FlexGrow: 1},
}
```

`Options.Cursor` varsayılan blok, çubuk veya alt çizgi imlecini seçer. `CursorBlink` varsayılan yanıp sönmeyi etkinleştirir; sıfır değeri imleci sabit tutar. Programlar şekli ve yanıp sönmeyi DECSCUSR veya DEC modu 12 aracılığıyla geçersiz kılar. Odakta olmayan bir imleç içi boş ve sabittir; birleştirme (composition) ve azaltılmış hareket de imleci sabit tutar. Yanıp sönmeyi, terminal metni motor tarafından yeniden çizilmeden platformun animasyon saati yürütür.

`View.Font`, `Theme` ve `DarkTheme` yazı tipini ve paleti ayarlar. Izgara, Unicode kutu çizim ve blok karakterlerini ve U+E0B0–U+E0B7 Powerline ayırıcılarını kendisi çizer ve bitişik hücreleri cihaz piksellerine hizalar. Diğer metinler platform yazı tiplerini ve bunların yedeklerini kullanır.

`MinimumContrast`, ters renkler, soluk metin ve seçim dahil olmak üzere sıradan metnin gerçek hücre arka planına karşı kontrastını artırır. 1–21 arası değerler WCAG'nin kontrast oranını kullanır; sıfır renkleri korur. Grafikler ve emojiler renklerini korur. `Transparent` ile kontrast yine de terminalin yapılandırılmış arka planını kullanır.

`Padding`, ızgaranın dışında nokta cinsinden boşluk bırakır. `PaddingColor` bu boşluğun arka planını seçer:

| Değer | Davranış |
| --- | --- |
| `PaddingBackground` | OSC 11 değişiklikleri dahil terminal arka planı |
| `PaddingExtend` | En yakın hücrenin arka planı; birincil ekranda, istem (prompt) satırlarının, Powerline satırlarının veya varsayılan arka planlı bir hücre içeren satırların yanında üst/alt genişletmeyi bastırır |
| `PaddingExtendAlways` | Bu istisnalar olmadan her kenarı genişletir |

TSX uygulamaları, `Terminal`'i `nsgo/base` içinde kaydetme etkisi için `tsx/terminal`'i içe aktarır. Özellikler (prop) `cursorBlink`, `minimumContrast` ve `paddingColor`'dır; sonuncusu `background`, `extend` ve `extend-always` değerlerini kabul eder. Görünüm özellikleri oturum devam ederken değişebilir.

## Tarayıcıda

Uygulama, sabitlenmiş libghostty-vt WebAssembly modülünü gömer ve aynı terminal görünümünü ve giriş yolunu çalıştırır. Uygulamayı derlemek için Zig, yerel kütüphane kurulumu veya çalışma zamanında indirme gerekmez.

Tarayıcının WebSocket'ini `io.ReadWriteCloser`'a uyarlayın: ikili baytlar gönderin ve gelen mesajları bir akış olarak birleştirin veya bölün. Kimlik doğrulama, yeniden bağlanma ve her türlü yeniden boyutlandırma/denetim mesajları uygulamanın ve sunucusunun sorumluluğundadır; NSGo terminal baytlarını tüketir. [Tarayıcı kabul fikstürü](https://github.com/nsgo-dev/nsgo/blob/main/internal/testhost/testdata/terminalweb/socket_js_test.go), kapatma işlemeyle birlikte eksiksiz bir bayt akışı bağdaştırıcısını gösterir.

## Eylemler ve yakınlaştırma

`View.Keybinds`, varsayılanları `Keybind{Key, Action, Amount}` ile geçersiz kılar. Tuşlar `Scope.OnKey` ile aynı adları kullanır; örneğin `mod+=` veya `ctrl+shift+c`. Sonraki girdiler kazanır. Bağlamalar, bu terminalin girişi odaktayken ve hiçbir giriş yöntemi birleştirme yapmıyorken çalışır.

Eylemler `CopyToClipboard`, `PasteFromClipboard`, `SelectAll`, `ClearScreen`, `IncreaseFontSize`, `DecreaseFontSize`, `ResetFontSize`, `SetFontSize` ve `Ignore`'dur. Pencere ve sekme eylemleri uygulamanın tuş haritasına aittir. `Ignore` bir tuşu programa göndermeden tüketir.

Varsayılan olarak `mod+=` ve `mod+shift+=` yazı tipini bir punto büyütür, `mod+-` küçültür ve `mod+0` `View.Font.Size` değerine (ayarlanmamışsa 13) sıfırlar. `Amount`, büyütme/küçültme için adımı (sıfırsa bir) veya `SetFontSize` için boyutu ayarlar. Boyutlar 1–255 punto aralığına sıkıştırılır; sonlu olmayan istekler yok sayılır. Yakınlaştırma oturumu ve dış görünümü korur, ızgaranın satır ve sütunlarını günceller ve programı yeniden boyutlandırır.

```go
ref := &terminal.Ref{}
view := terminal.View{
	Terminal: session, Ref: ref,
	Keybinds: []terminal.Keybind{
		{Key: "ctrl+shift+k", Action: terminal.ClearScreen},
		{Key: "mod+=", Action: terminal.IncreaseFontSize, Amount: 2},
	},
}
```

`Ref.FontSize()` onu görüntüleyen denetimler için izlenir; `Ref.SetFontSize(size)` ve `Ref.Perform(action, amount)` olay işleyicilerinin yaptığı gibi uygulamanın motorunda çalışır. `Perform` bir eylemin kullanılabilir olup olmadığını bildirir. Bağlanmamış (unmounted) bir ref sıfır boyut döndürür ve eylemleri reddeder.

## Bağlam menüsü

İkincil bir tıklama, yerelleştirilmiş Kopyala, Yapıştır, Tümünü Seç ve Temizle öğeleriyle stilsiz bir `base.ContextMenu` açar. Kopyala bir seçim gerektirir. Temizle, istemin üstündeki içeriği kaldırır ve alternatif ekranda devre dışıdır. Dokunmatik ekranlar bağlam menüsünün uzun basma sunumunu kullanır. Yüzeyi `MenuClass`/`MenuStyle` ile, öğeleri ise `MenuItemClass`/`MenuItemStyle` ile stillendirin.

`RightClickAction`'ı `RightClickPaste`, `RightClickCopy`, `RightClickCopyOrPaste` (bir seçimi kopyala, aksi halde yapıştır) veya `RightClickIgnore` olarak ayarlayın. Ignore olayı çevreleyen bir uygulama menüsüne bırakır; ref'in eylemlerini çağıran stillendirilmiş bir `ui.ContextMenu` sağlarken bunu kullanın.

`<Terminal>`'i `nsgo/base` içinde kaydetmek için `tsx/terminal`'i içe aktarın. Reaktif `keybinds` ve `rightClickAction` özellikleri oturumu korurken görünümü günceller. Eylem değerleri `increase_font_size` ve `copy_to_clipboard` gibi Ghostty adlarını kullanır; sağ tıklama değerleri `context-menu`, `paste`, `copy`, `copy-or-paste` ve `ignore`'dur. Menü stillendirme özellikleri, Go adlarını lower camel case biçiminde kullanır.

## Fare raporları

Fare izlemeyi etkinleştiren programlar, istenen basmaları, bırakmaları, sürüklemeleri ve üzerine gelmeleri, libghostty-vt tarafından seçtikleri protokolde kodlanmış olarak alır. Shift, seçim için tıklama ve sürükleme yakalamasını atlar. `View.MouseReporting` isteğe bağlı reaktif bir `func() bool`'dur ve nil olduğunda true'dur; bir program yakalama istese bile fare seçimini ve yerel geri kaydırma (scrollback) kaydırmasını korumak için false döndürün. Raporlama etkinken, bir programın sağ tıklaması çevreleyen bir bağlam menüsüne göre önceliklidir.

Alternatif ekranda, alternatif kaydırma modu etkin ve fare izlemesi istenmemişken, dikey tekerlek girişi normal veya uygulama imleç tuşu modunu izleyerek imleç tuşlarına dönüşür. `MouseScrollMultiplier{Precision: 1, Discrete: 3}`, dokunmatik yüzeyler ve tekerlekler için varsayılan rapor sayılarını ayarlar; sıfır bu varsayılanları kullanır ve pozitif değerler 0.01–10,000 aralığına sıkıştırılır. Kesirli giriş, tam bir adım kullanılabilir olana kadar birikir. Normal geri kaydırma, platformun yerel kaydırmasını ve eylemsizliğini korur.

`<Terminal>`'i `nsgo/base` içinde kaydetmek için `tsx/terminal`'i içe aktarın. `mouseReporting` özelliği varsayılan olarak true'dur; `mouseScrollPrecision` ve `mouseScrollDiscrete` çarpanları ayarlar.
