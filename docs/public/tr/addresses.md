---
title: Uygulama adresleri
description: Tarayıcı geçmişi üzerinden bir web uygulamasının adresini okuma ve güncelleme.
group: Temel kavramlar
order: 59
---

`cx.AppURL()` uygulamanın adresini izlenen (tracked) bir değer olarak okur. Web'de ilk sayfa URL'si kök işlenmeden önce kullanılabilir durumdadır; bu nedenle bir yeniden yükleme veya paylaşılan bir bağlantı aynı görünümü seçebilir. Uygulama, URL'nin görünümlerine nasıl eşleneceğine kendisi karar verir.

```go
func Root(cx *core.Scope) core.Node {
    return core.View{Children: []core.Node{
        core.Text{ContentFunc: cx.AppURL},
        base.Button{Label: "Open film", OnPress: func(core.PressEvent) {
            if err := cx.SetAppURL("/films/42", core.AppURLOptions{
                History: core.HistoryPush,
            }); err != nil {
                // Show the error in the app's usual way.
            }
        }},
    }}
}
```

`SetAppURL` bir sayfa istemeden veya bir yığında gezinmeden izlenen adresi hemen günceller. Varsayılan olarak geçerli tarayıcı girdisinin yerini alır; `HistoryPush` ise yeni bir girdi ister. Aynı işlemedeki (commit) bir gezinme veya modal itmesi bunun yerine o girdiyi sağlar; böylece tek bir eylem iki geri adımı oluşturmaz. Bir işlemede birden fazla yazma yapılırsa son adres korunur ve tüm itme istekleri saklanır.

Göreli adresler geçerli adrese veya açık bir HTML tabanına (`base`) göre çözümlenir. Bir tarayıcı adresi sayfanın kökeninde (origin) kalmalıdır. Geçersiz adresler, boşluklar, denetim karakterleri, ters eğik çizgiler ve 16 KiB'ın üzerindeki adresler adresi değiştirmeden hata döndürür.

`cx.OnAppURLChange(func(change core.AppURLChange) { ... })` tarayıcı geçişlerini ve hash düzenlemelerini gözlemler. `change.Reason` değeri `"back"`, `"forward"` veya `"address"` olur; döndürülen işlev aboneliği sonlandırır ve kapsam (scope) sonlandırıldığında dinleyici kaldırılır. İlk adres için `AppURL` okuyun. Uygulamanın kendi yazmaları bağlamaları günceller ancak değişiklik bildirimi göndermez. Aynı adresi koruyan bir geçiş değişiklik göndermez.

Tarayıcının geri düğmesi yine de bir modalı kapatır veya bir gezinme ekranını yığından çıkarır. Uygulamanın reddettiği bir geri hareketi mevcut girdiyi ve adresini geri yükler. Bir kapatma işleyicisi kendi değiştirme adresini ayarlayabilir. Programatik bir çıkarma (pop) adresi eşzamansız geçmiş geçişinden sonra günceller ve bu geçiş sırasında gelen yazmalar onun bitmesini bekler. Bu oynatıcının girdilerinin gerisine gitmek, önceki bir yüklemeden kalan girdiler dahil olmak üzere uygulamadan çıkar.

`/films/42` gibi yollar için dağıtılan uygulama dizinini ayarlayın ve sunucunun uygulama rotaları için `index.html` sunmasını sağlayın:

```yaml
web:
  base: /        # /my-app/ when deployed below that directory
```

`web.base`, derin bir adres doğrudan açıldığında betiklerin, stiller, Wasm ve kaynakların yüklenmeye devam etmesi için bir HTML tabanı yazar. Bu ayar olmadan mevcut göreli varlık yolları korunur; sorgu veya hash adresleri bunları değiştirmeden kullanabilir. Sunucu geri dönüş (fallback) kuralları gerçek varlık isteklerine dokunmamalıdır. Sıkı bir İçerik Güvenliği Politikası (CSP) bu aynı kökenli tabana (`base-uri 'self'`) izin vermelidir.

Başka bir sayfaya gömülü bir uygulama adresini dahili olarak tutar ve kapsayan sayfanın geçmişine dokunmaz. Aynı kökenli bir iframe, `data-nsgo-history` ile buna açıkça katılabilir. Sınırlandırılmış (confined) alt ağaçlar uygulamanın adresini okuyamaz veya değiştiremez.

Diğer hedeflerde `AppURL` uygulama ile açılan son bağlantıyı veya `SetAppURL`'in sakladığı değeri tutar; bu değeri ayarlamak yerel bir pencereyi veya sistem tarayıcısını değiştirmez. Mini programlar da konaklarının rotalarını değiştirmeden bu değeri korur.

TSX aynı durumu kullanır:

```tsx
import { UrlScheme } from "nsgo";

UrlScheme.url();
UrlScheme.setUrl("/films/42", { history: "push" });
const stop = UrlScheme.on("change", ({ url, reason }) => {
  // Update the app's route from the browser's choice.
});
```
