---
title: 應用程式網址
description: 透過瀏覽器歷史記錄讀取並更新網頁應用程式的網址。
group: 核心概念
order: 59
---

`cx.AppURL()` 會以可追蹤值（tracked value）的形式讀取應用程式的網址。在網頁平台上，初始頁面 URL 在根元件算繪之前即可取得，因此重新載入或分享連結時能選擇顯示相同的畫面。應用程式可自行決定如何將 URL 對應至其檢視。

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

`SetAppURL` 會立即更新追蹤中的網址，而不會重新請求頁面或導覽堆疊。預設會取代目前的瀏覽器歷史記錄項目；指定 `HistoryPush` 則會請求新增一筆歷史記錄。若在同一次提交（commit）中同時執行導覽推入或強制回應推送，則會改由該動作提供歷史記錄項目，確保單一操作不會產生兩筆返回步驟。同一次提交中的多次寫入會保留最後一個網址，並保留任何推入請求。

相對網址會相對於目前的網址或明確設定的 HTML base 解析。瀏覽器網址必須維持在同一個頁面來源（origin）內。無效網址、空白字元、控制字元、反斜線，以及超過 16 KiB 的網址會回傳錯誤且不會變更目前網址。

`cx.OnAppURLChange(func(change core.AppURLChange) { ... })` 會監聽瀏覽器的前進後退操作與 hash 編輯。`change.Reason` 的值為 `"back"`、`"forward"` 或 `"address"`；回傳的函式可用於取消訂閱，而作用域（scope）處置時也會自動移除監聽器。初始網址請直接讀取 `AppURL`。應用程式自行寫入網址時會更新繫結，但不會發送變更通知。維持相同網址的歷史記錄遍歷也不會發送變更通知。

瀏覽器的「上一頁」仍會關閉強制回應視窗或彈出導覽畫面。若應用程式拒絕返回，則會還原現有的歷史記錄項目與網址。關閉處理函式（dismissal handler）可自行設定取代網址。程式化的彈出（pop）操作會在非同步歷史記錄遍歷完成後更新網址，且在遍歷期間到達的寫入請求會等待其完成。若返回超過此播放器的歷史記錄項目（包含先前載入時保留的項目），則會離開應用程式。

若要使用如 `/films/42` 這類路徑，請設定部署的應用程式目錄，並讓伺服器為應用程式路由提供其 `index.html`：

```yaml
web:
  base: /        # /my-app/ when deployed below that directory
```

`web.base` 會寫入 HTML base，讓直接開啟深層網址時仍能正確載入腳本、樣式、Wasm 與資源。若未設定此項目，則會保留現有的相對資源路徑；使用查詢字串（query）或雜湊（hash）網址的應用程式可維持原樣。伺服器的後備（fallback）規則必須略過真實的資源請求。嚴格的內容安全政策（CSP）必須允許此同源 base（`base-uri 'self'`）。

嵌入於其他頁面中的應用程式會在內部保存自己的網址，不會更動外層頁面的歷史記錄。同源的 iframe 可透過設定 `data-nsgo-history` 明確啟用整合。受限制的子樹（confined subtree）無法讀取或變更應用程式網址。

在其他目標平台上，`AppURL` 會保存開啟該應用程式的最後一個連結或 `SetAppURL` 所儲存的值；設定該值不會變更原生視窗或系統瀏覽器。小程式同樣會保存此值，而不會變更其宿主平台的路由。

TSX 使用相同的狀態介面：

```tsx
import { UrlScheme } from "nsgo";

UrlScheme.url();
UrlScheme.setUrl("/films/42", { history: "push" });
const stop = UrlScheme.on("change", ({ url, reason }) => {
  // Update the app's route from the browser's choice.
});
```
