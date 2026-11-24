---
title: Dokunsal geri bildirim
description: Hazır dokunsal geri bildirim veya zamanlanmış bir darbe dizisi çalın.
group: Platform özellikleri
order: 46
---

## Tek bir geri bildirim çalmak

`mobile.PlayHaptic`, iOS ve Android'de kullanıcının parmağının altında geri bildirim çalar, başka yerlerde hiçbir şey yapmaz; bu yüzden onu kullanıcının gördüğü şeyle eşleştirin. `HapticSelection` bir değerin değiştiğini, yani bir seçicinin, bir segmentin veya bir kaydırıcının adımını işaret eder; `HapticLight` ile `HapticRigid` arasındakiler öğelerin ağırlıklarına ve esnekliklerine göre çarpışmasını işaret eder; `HapticSuccess`, `HapticWarning` ve `HapticError` ise bir görevin bu şekilde sona erdiğini işaret eder.

```go
mobile.PlayHaptic(cx, mobile.HapticSelection)
```

## Bir desen çalmak

`cx.Env().CanPlayHapticPattern()`, zamanlanmış dokunsal geri bildirimin kullanılabilir olup olmadığını bildirir. Bir desen, desene göre bir `At` başlangıç zamanına sahip darbeler içerir. Sıfır `Duration` anlık bir darbe oluşturur; pozitif bir süre ise darbeyi 30 saniyeye kadar sürdürür. `Intensity` ve `Sharpness` 0 ile 1 arasında değişir. Yeni bir desen öncekinin yerini alır. Oynatmayı durdurmak için boş bir desen verin. Uygulama arka plana geçtiğinde desen durur.

```go
mobile.PlayHapticPattern(cx, mobile.HapticPattern{
    {Intensity: 0.25, Sharpness: 0.4},
    {At: 100 * time.Millisecond, Intensity: 0.55, Sharpness: 0.6},
    {At: 200 * time.Millisecond, Intensity: 1, Sharpness: 0.8},
})
```

iOS, desteklenen donanımda olayları Core Haptics üzerinden çalar. Android zamanlamaya uyar ve titreşim motoru genlik denetimini desteklediğinde yoğunluğu kullanır. `navigator.vibrate` destekleyen Android tarayıcıları, bir kullanıcı etkileşiminden sonra açık ve kapalı zamanlamalarına uyar. Android ve web, anlık bir darbeyi 20 ms'lik bir darbe olarak çalar; keskinlik ise iOS'ta kullanılabilir. Desteklenmeyen bir platformda çağrının hiçbir etkisi olmaz.

Testlerde `nsgotest.Environment` üzerinden `HapticPatterns: true` ayarlayın ve `app.HapticPatterns` değerini okuyun. Uygulama oynatmayı iptal ettiğinde geçmiş bir nil girdi içerir.

## Ne zaman

Geri bildirimi yanıt geldiğinde değil, hareket gerçekleşirken çalın: bir değer geçerken bir seçim, bir görünüm yerine oturduğunda bir çarpma, bir görev sona erdiğinde bir sonuç. Kullanıcının neden olduğu bir değişiklik için bir tane çalın, uygulamanın kendi başına yaptığı şeyler için hiç çalmayın.

## Sonraki adımlar

- [Bildirimler](notifications.md): aynı haberi taşıyan bildirimler ve rozetler.
- [Platform hizmetleri](services.md): sistemin sunduğu diğer her şey.
- [Erişilebilirlik](accessibility.md): hiçbir şey hissetmeyen bir kullanıcının duyduğu duyuru.
